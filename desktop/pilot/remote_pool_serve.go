// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"sync"
	"time"

	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymstable"
)

func remoteWireFailure(q RemotePoolRequest, code string) RemotePoolResponse {
	return RemotePoolResponse{Version: q.Version, ID: q.ID, Command: q.Command, ConfigID: q.ConfigID, ViewID: q.ViewID, Error: code}
}

// Narrow read/pre-dispatch categories only. Joined errors are never treated as
// exact Busy; failed cleanup takes precedence and retires the helper.
func remoteWireReadError(e error) string {
	switch {
	case errors.Is(e, ErrRemotePoolCleanup):
		return "cleanup_incomplete"
	case errors.Is(e, gymstable.ErrOutcomeUnknown):
		return "outcome_unknown"
	case e == ErrRemotePoolBusy || e == gymstable.ErrBusy:
		return "busy"
	case errors.Is(e, context.Canceled), errors.Is(e, context.DeadlineExceeded):
		return "cancelled"
	case errors.Is(e, gymstable.ErrStaleView):
		return "stale_view"
	case errors.Is(e, gymaccount.ErrUnauthorized):
		return "unauthorized"
	case errors.Is(e, gymaccount.ErrForbidden), errors.Is(e, gymaccount.ErrHeld):
		return "forbidden"
	default:
		return "remote_unavailable"
	}
}

func remoteWireStep(ctx context.Context, q RemotePoolRequest, c RemotePoolControllerAPI, expected RemotePoolConfig, prior *RemotePoolView) (RemotePoolResponse, error) {
	before, e := c.WireSnapshot(ctx)
	if e != nil {
		return remoteWireFailure(q, remoteWireReadError(e)), nil
	}
	if ValidateRemotePoolWireSnapshot(before, expected) != nil {
		return RemotePoolResponse{}, ErrRemotePoolProtocol
	}
	// A signed-in view can only originate in this pipe's successful login.
	if before.View != nil && (prior == nil || *before.View != *prior) {
		return RemotePoolResponse{}, ErrRemotePoolProtocol
	}
	if q.ViewID == "" && prior != nil || q.ViewID != "" && (prior == nil || q.ViewID != prior.ViewID) {
		return remoteWireFailure(q, "stale_view"), nil
	}
	mutating := q.Command != "snapshot"
	if q.Command == "login_fixture" {
		if before.View != nil {
			return remoteWireFailure(q, "stale_view"), nil
		}
	} else if mutating && (before.View == nil || before.View.ViewID != q.ViewID) {
		return remoteWireFailure(q, "stale_view"), nil
	}
	after := before
	if mutating {
		switch q.Command {
		case "login_fixture":
			e = c.LoginFixture(ctx, q.Fixture)
		case "start":
			e = c.Start(ctx, q.ViewID)
		case "stop":
			e = c.Stop(ctx, q.ViewID)
		case "logout":
			e = c.Logout(ctx, q.ViewID)
		default:
			return RemotePoolResponse{}, ErrRemotePoolProtocol
		}
		if e != nil {
			if errors.Is(e, ErrRemotePoolCleanup) {
				return remoteWireFailure(q, "cleanup_incomplete"), nil
			}
			return remoteWireFailure(q, "outcome_unknown"), nil
		}
		after, e = c.WireSnapshot(ctx)
		if e != nil {
			if errors.Is(e, ErrRemotePoolCleanup) {
				return remoteWireFailure(q, "cleanup_incomplete"), nil
			}
			return remoteWireFailure(q, "outcome_unknown"), nil
		}
	}
	r := RemotePoolResponse{Version: q.Version, ID: q.ID, Command: q.Command, ConfigID: q.ConfigID, ViewID: q.ViewID, OK: true, Result: &after}
	if ValidateRemotePoolWireSnapshot(after, expected) != nil || !remoteWireFits(r, MaxRemotePoolResponseSize) {
		if mutating {
			return remoteWireFailure(q, "outcome_unknown"), nil
		}
		return RemotePoolResponse{}, ErrRemotePoolProtocol
	}
	raw, e := json.Marshal(r)
	if e == nil {
		_, e = ParseRemotePoolResponse(raw, q, expected, prior)
	}
	if e != nil {
		if mutating {
			return remoteWireFailure(q, "outcome_unknown"), nil
		}
		return RemotePoolResponse{}, ErrRemotePoolProtocol
	}
	return r, nil
}

// ServeRemotePool serves one original concrete owner on one bounded pipe.
// Cancellation closes both pipes before joining controller work. Failed Close
// is returned without replacing the controller; its caller retains that owner.
func ServeRemotePool(ctx context.Context, input io.ReadCloser, output io.WriteCloser, c RemotePoolControllerAPI, expected RemotePoolConfig) (resultErr error) {
	if ctx == nil || input == nil || output == nil || c == nil {
		return ErrRemotePoolProtocol
	}
	raw, e := MarshalRemotePoolConfig(expected)
	if e != nil {
		return e
	}
	expected, e = ParseRemotePoolConfig(raw) // owned, immutable nested slices
	if e != nil {
		return e
	}
	life, cancel := context.WithCancel(ctx)
	joined := make(chan struct{})
	readDone := make(chan struct{})
	frames := make(chan []byte, 1)
	readFailure := make(chan error, 1)
	var admission sync.Mutex
	accepting := true
	var cleanupErr error
	go func() {
		defer close(joined)
		<-life.Done()
		cleanupErr = errors.Join(input.Close(), output.Close(), c.Close())
	}()
	// One reader owns the original pipe. It keeps observing EOF during an
	// admitted controller action; at most one complete frame can be pending.
	go func() {
		defer close(readDone)
		fail := func(e error) { readFailure <- e; cancel() }
		reader := bufio.NewReaderSize(input, MaxRemotePoolRequestSize+2)
		for {
			line, readErr := reader.ReadSlice('\n')
			if readErr == io.EOF && len(line) == 0 {
				fail(nil)
				return
			}
			if readErr != nil || len(line) < 2 || len(line) > MaxRemotePoolRequestSize+1 || line[len(line)-1] != '\n' || bytes.IndexByte(line, '\r') >= 0 {
				fail(errors.Join(ErrRemotePoolProtocol, readErr))
				return
			}
			admission.Lock()
			if !accepting || life.Err() != nil {
				admission.Unlock()
				fail(ErrRemotePoolProtocol)
				return
			}
			accepting = false
			admission.Unlock()
			frame := append([]byte(nil), line[:len(line)-1]...)
			select {
			case frames <- frame:
			case <-life.Done():
				return
			default:
				fail(ErrRemotePoolProtocol)
				return
			}
		}
	}()
	defer func() { cancel(); <-readDone; <-joined; resultErr = errors.Join(resultErr, cleanupErr) }()
	shutdownError := func(fallback error) error {
		if e := ctx.Err(); e != nil {
			return errors.Join(fallback, e)
		}
		select {
		case e := <-readFailure:
			// Exact stdin EOF requests joined shutdown, including during an
			// action. No successful action response is invented; the peer
			// already lost that response and retains its uncertainty.
			if e == nil {
				return nil
			}
			return errors.Join(fallback, e)
		default:
			return fallback
		}
	}
	var last uint64
	var prior *RemotePoolView
	for {
		var line []byte
		select {
		case line = <-frames:
		case <-life.Done():
			return shutdownError(life.Err())
		}
		q, e := ParseRemotePoolRequest(line, expected)
		if e != nil || q.ID <= last || last == 0 && (q.ID != 1 || q.Command != "snapshot" || q.ViewID != "") {
			return ErrRemotePoolProtocol
		}
		last = q.ID
		if e = life.Err(); e != nil {
			return shutdownError(e)
		}
		work, stop := context.WithTimeout(life, 2*time.Minute)
		timed := make(chan struct{})
		stopDeadline := context.AfterFunc(work, func() { defer close(timed); cancel() })
		r, stepErr := remoteWireStep(work, q, c, expected, prior)
		if stepErr == nil {
			if !remoteWireFits(r, MaxRemotePoolResponseSize) || !r.OK && (r.Result != nil || !remoteWireErrorCode(r.Error)) {
				stepErr = ErrRemotePoolProtocol
			} else {
				var encoded []byte
				encoded, stepErr = json.Marshal(r)
				if stepErr == nil {
					encoded = append(encoded, '\n')
					var n int
					// A command arriving during dispatch is refused by the
					// reader. During this final response write, serialize the
					// next admission so a peer that just received the complete
					// reply cannot race the local write-return bookkeeping.
					admission.Lock()
					n, stepErr = output.Write(encoded)
					if n != len(encoded) {
						stepErr = errors.Join(stepErr, io.ErrShortWrite)
					}
					if stepErr == nil {
						accepting = true
					}
					admission.Unlock()
				}
			}
		}
		if !stopDeadline() {
			<-timed
		}
		contextErr := work.Err()
		stop()
		if stepErr != nil || contextErr != nil {
			e := errors.Join(stepErr, contextErr)
			if life.Err() != nil {
				return shutdownError(e)
			}
			return e
		}
		if !r.OK {
			if r.Error == "outcome_unknown" || r.Error == "cleanup_incomplete" {
				return ErrRemotePoolProtocol
			}
			continue
		}
		if r.Result.View == nil {
			prior = nil
		} else {
			v := *r.Result.View
			prior = &v
		}
	}
}
