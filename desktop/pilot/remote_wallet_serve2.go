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
)

func dailyWalletFailure2(q DailyWalletRequest2, code string) DailyWalletResponse2 {
	return DailyWalletResponse2{Version: q.Version, Application: q.Application, ID: q.ID, Command: q.Command, ConfigID: q.ConfigID, ViewID: q.ViewID, Error: code}
}
func dailyWalletNativeError2(e error) string {
	if e == ErrActivityNotFound {
		return "not_found"
	}
	if e == ErrActivityUnavailable {
		return "unavailable"
	}
	return dailyWalletReadError(e)
}
func dailyWalletStep2(ctx context.Context, q DailyWalletRequest2, c DailyWalletControllerAPI2, expected RecoverableRemotePoolConfig, prior *RemotePoolView) (DailyWalletResponse2, error) {
	if !dailyWalletCommand2(q.Command) {
		r, e := dailyWalletStep(ctx, dailyWalletRequest1(q), c, expected, prior)
		return DailyWalletResponse2{Version: q.Version, Application: q.Application, ID: r.ID, Command: r.Command, ConfigID: r.ConfigID, ViewID: r.ViewID, OK: r.OK, Snapshot: r.Snapshot, Job: r.Job, Read: r.Read, Error: r.Error}, e
	}
	before, e := c.DailyWalletSnapshot(ctx)
	if e != nil {
		return dailyWalletFailure2(q, dailyWalletNativeError2(e)), nil
	}
	if ValidateDailyWalletSnapshot(before, expected) != nil {
		return DailyWalletResponse2{}, ErrDailyWalletProtocol
	}
	if before.View != nil && (prior == nil || *before.View != *prior) {
		return DailyWalletResponse2{}, ErrDailyWalletProtocol
	}
	if prior == nil || q.ViewID != prior.ViewID || before.View == nil || before.View.ViewID != q.ViewID {
		return dailyWalletFailure2(q, "stale_view"), nil
	}
	out := DailyWalletResponse2{Version: q.Version, Application: q.Application, ID: q.ID, Command: q.Command, ConfigID: q.ConfigID, ViewID: q.ViewID, OK: true}
	switch q.Command {
	case "wallet_activity_begin":
		var j RemoteWalletActivityJob
		j, e = c.BeginWalletActivityRead(ctx, q.ViewID, q.Sequence)
		v := DailyWalletActivityJobProjection(j)
		out.ActivityJob = &v
	case "wallet_activity_read", "wallet_activity_cancel":
		if q.Command == "wallet_activity_cancel" {
			e = c.CancelWalletActivityRead(ctx, q.ViewID, q.Sequence)
		}
		if e == nil {
			var r RemoteWalletActivityRead
			r, e = c.WalletActivityRead(ctx, q.ViewID, q.Sequence)
			v := DailyWalletActivityReadProjection(r)
			out.ActivityRead = &v
		}
	case "wallet_activity_page":
		var p RemoteWalletActivityPage
		p, e = c.WalletActivityPage(ctx, q.ViewID, q.CaptureSequence, q.After, q.Limit)
		v := DailyWalletActivityPageProjection(p)
		out.ActivityPage = &v
	case "wallet_activity_detail":
		var d RemoteWalletActivityDetail
		d, e = c.WalletActivityDetail(ctx, q.ViewID, q.CaptureSequence, q.TxID)
		v := DailyWalletActivityDetailProjection(d)
		out.ActivityDetail = &v
	case "daily_payout_observe":
		var o RemoteDailyPayoutObservation
		o, e = c.ObserveDailyPayout(ctx, q.ViewID, q.CaptureSequence, q.BatchSequence, q.TxID, q.OutputIndex)
		v := DailyWalletObservationProjection(o)
		out.PayoutObservation = &v
	default:
		return DailyWalletResponse2{}, ErrDailyWalletProtocol
	}
	if e != nil {
		return dailyWalletFailure2(q, dailyWalletNativeError2(e)), nil
	}
	if ValidateDailyWalletResponse2(out, q, expected, prior) != nil {
		return DailyWalletResponse2{}, ErrDailyWalletProtocol
	}
	return out, nil
}

// ServeDailyWallet2 serves one original concrete owner on one bounded pipe.
// Cancellation closes both pipes before joining controller work. Failed Close
// is returned without replacing the controller; its caller retains that owner.
func ServeDailyWallet2(ctx context.Context, input io.ReadCloser, output io.WriteCloser, c DailyWalletControllerAPI2, expected RecoverableRemotePoolConfig) (resultErr error) {
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
		q, e := ParseDailyWalletRequest2(line, expected)
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
		r, stepErr := dailyWalletStep2(work, q, c, expected, prior)
		if stepErr == nil {
			if !dailyWalletFits2(r, DailyWalletResponseLimit2(q.Command)) || !r.OK && (r.Snapshot != nil || r.Job != nil || r.Read != nil || r.ActivityJob != nil || r.ActivityRead != nil || r.ActivityPage != nil || r.ActivityDetail != nil || r.PayoutObservation != nil || !dailyWalletErrorCode(r.Error)) {
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
