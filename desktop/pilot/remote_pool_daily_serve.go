// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/poolsettle"
	"io"
	"sync"
	"time"
)

func dailyWalletFailure(q DailyWalletRequest, code string) DailyWalletResponse {
	return DailyWalletResponse{Version: q.Version, Application: q.Application, ID: q.ID, Command: q.Command, ConfigID: q.ConfigID, ViewID: q.ViewID, Error: code}
}
func dailyWalletReadError(e error) string {
	switch e {
	case ErrDailyPayoutReadInvalid, ErrDailyWalletProtocol:
		return "invalid_request"
	case ErrDailyPayoutReadNotFound:
		return "not_found"
	case poolsettle.ErrStaleCapture:
		return "stale_capture"
	}
	code := remoteWireReadError(e)
	if code == "remote_unavailable" {
		return "unavailable"
	}
	return code
}
func dailyWalletStep(ctx context.Context, q DailyWalletRequest, c DailyWalletControllerAPI, expected RecoverableRemotePoolConfig, prior *RemotePoolView) (DailyWalletResponse, error) {
	before, e := c.DailyWalletSnapshot(ctx)
	if e != nil {
		return dailyWalletFailure(q, dailyWalletReadError(e)), nil
	}
	if ValidateDailyWalletSnapshot(before, expected) != nil {
		return DailyWalletResponse{}, ErrDailyWalletProtocol
	}
	if before.View != nil && (prior == nil || *before.View != *prior) {
		return DailyWalletResponse{}, ErrDailyWalletProtocol
	}
	if q.ViewID == "" && prior != nil || q.ViewID != "" && (prior == nil || q.ViewID != prior.ViewID) {
		return dailyWalletFailure(q, "stale_view"), nil
	}
	if q.Command == "login_fixture" {
		if before.View != nil {
			return dailyWalletFailure(q, "stale_view"), nil
		}
	} else if q.Command != "snapshot" && (before.View == nil || before.View.ViewID != q.ViewID) {
		return dailyWalletFailure(q, "stale_view"), nil
	}
	out := DailyWalletResponse{Version: q.Version, Application: q.Application, ID: q.ID, Command: q.Command, ConfigID: q.ConfigID, ViewID: q.ViewID, OK: true}
	if dailyWalletMoneyCommand(q.Command) {
		switch q.Command {
		case "daily_payout_begin":
			var j RemoteDailyPayoutJob
			j, e = c.BeginDailyPayoutRead(ctx, q.ViewID, q.Sequence, q.CaptureSequence, q.BeforeBatch)
			v := DailyWalletJobProjection(j)
			out.Job = &v
		case "daily_payout_cancel":
			e = c.CancelDailyPayoutRead(ctx, q.ViewID, q.Sequence)
			if e == nil {
				var r RemoteDailyPayoutRead
				r, e = c.DailyPayoutRead(ctx, q.ViewID, q.Sequence)
				v := DailyWalletReadProjection(r)
				out.Read = &v
			}
		case "daily_payout_read":
			var r RemoteDailyPayoutRead
			r, e = c.DailyPayoutRead(ctx, q.ViewID, q.Sequence)
			v := DailyWalletReadProjection(r)
			out.Read = &v
		}
		if e != nil {
			return dailyWalletFailure(q, dailyWalletReadError(e)), nil
		}
	} else {
		after := before
		if q.Command != "snapshot" {
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
				return DailyWalletResponse{}, ErrDailyWalletProtocol
			}
			if e == nil {
				after, e = c.DailyWalletSnapshot(ctx)
			}
			if e != nil {
				if errors.Is(e, ErrRemotePoolCleanup) {
					return dailyWalletFailure(q, "cleanup_incomplete"), nil
				}
				return dailyWalletFailure(q, "outcome_unknown"), nil
			}
		}
		out.Snapshot = &after
	}
	if ValidateDailyWalletResponse(out, q, expected, prior) != nil {
		if q.Command != "snapshot" && !dailyWalletMoneyCommand(q.Command) {
			return dailyWalletFailure(q, "outcome_unknown"), nil
		}
		return DailyWalletResponse{}, ErrDailyWalletProtocol
	}
	return out, nil
}

// ServeDailyWallet serves one original concrete owner on one bounded pipe.
// Cancellation closes both pipes before joining controller work. Failed Close
// is returned without replacing the controller; its caller retains that owner.
func ServeDailyWallet(ctx context.Context, input io.ReadCloser, output io.WriteCloser, c DailyWalletControllerAPI, expected RecoverableRemotePoolConfig) (resultErr error) {
	if ctx == nil || input == nil || output == nil || c == nil {
		return ErrDailyWalletProtocol
	}
	raw, e := MarshalRecoverableRemotePoolConfig(expected)
	if e != nil {
		return e
	}
	expected, e = ParseRecoverableRemotePoolConfig(raw) // owned, immutable nested slices
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
		reader := bufio.NewReaderSize(input, MaxDailyWalletRequestSize-1+2)
		for {
			line, readErr := reader.ReadSlice('\n')
			if readErr == io.EOF && len(line) == 0 {
				fail(nil)
				return
			}
			if readErr != nil || len(line) < 2 || len(line) > MaxDailyWalletRequestSize-1+1 || line[len(line)-1] != '\n' || bytes.IndexByte(line, '\r') >= 0 {
				fail(errors.Join(ErrDailyWalletProtocol, readErr))
				return
			}
			admission.Lock()
			if !accepting || life.Err() != nil {
				admission.Unlock()
				fail(ErrDailyWalletProtocol)
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
				fail(ErrDailyWalletProtocol)
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
		q, e := ParseDailyWalletRequest(line, expected)
		if e != nil || q.ID <= last || last == 0 && (q.ID != 1 || q.Command != "snapshot" || q.ViewID != "") {
			return ErrDailyWalletProtocol
		}
		last = q.ID
		if e = life.Err(); e != nil {
			return shutdownError(e)
		}
		work, stop := context.WithTimeout(life, 2*time.Minute)
		timed := make(chan struct{})
		stopDeadline := context.AfterFunc(work, func() { defer close(timed); cancel() })
		r, stepErr := dailyWalletStep(work, q, c, expected, prior)
		if stepErr == nil {
			if !dailyWalletFits(r, MaxDailyWalletResponseSize) || !r.OK && (r.Snapshot != nil || r.Job != nil || r.Read != nil || !dailyWalletErrorCode(r.Error)) {
				stepErr = ErrDailyWalletProtocol
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
				// EOF requests shutdown, but it cannot erase an observed protocol,
				// marshal, write or short-write failure from the admitted operation.
				return errors.Join(stepErr, shutdownError(contextErr))
			}
			return e
		}
		if !r.OK {
			if r.Error == "outcome_unknown" || r.Error == "cleanup_incomplete" {
				return ErrDailyWalletProtocol
			}
			continue
		}
		if r.Snapshot == nil {
			continue
		}
		if r.Snapshot.View == nil {
			prior = nil
		} else {
			v := *r.Snapshot.View
			prior = &v
		}
	}
}
