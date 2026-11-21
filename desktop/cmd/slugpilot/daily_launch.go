// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/taytr-llc/slug/desktop/accountpilot"
	"github.com/taytr-llc/slug/desktop/pilot"
)

// dailyCall uses only the original selected controller. The local snapshot is
// not a remote cumulative-money DTO and is never served on the legacy wire.
func (o *remoteLaunchOwner) dailyCall(ctx context.Context, command, arg string) (out accountpilot.RemotePoolControllerSnapshot, resultErr error) {
	if o == nil {
		return out, errRemoteLaunchUnavailable
	}
	o.mu.Lock()
	e := o.checkLocked(ctx)
	if e == nil && o.controller == nil {
		e = errRemoteLaunchUnavailable
	}
	if e != nil {
		o.mu.Unlock()
		return out, errors.Join(e, o.Close())
	}
	o.active.Add(1)
	work, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	stop := context.AfterFunc(o.life, func() { defer close(done); cancel() })
	o.mu.Unlock()
	defer func() {
		if !stop() {
			<-done
		}
		cancel()
		o.mu.Lock()
		checkErr := o.checkLocked(ctx)
		o.mu.Unlock()
		o.active.Done()
		if checkErr != nil {
			out = accountpilot.RemotePoolControllerSnapshot{}
			resultErr = errors.Join(resultErr, checkErr, o.Close())
		}
	}()
	switch command {
	case "snapshot":
		return o.controller.Snapshot(work)
	case "login_fixture":
		return out, o.controller.LoginFixture(work, arg)
	case "start":
		return out, o.controller.Start(work, arg)
	case "stop":
		return out, o.controller.Stop(work, arg)
	case "logout":
		return out, o.controller.Logout(work, arg)
	default:
		return out, errRemoteLaunchInvalid
	}
}

// Fixed daily statuses are at most 270 bytes including LF; cap complete frames at
// the POSIX minimum PIPE_BUF of 512 rather than assuming Darwin has 4096.
// An actual output descriptor has a joined cancellation interrupt, so a blocked
// status consumer cannot strand the original Mining lease and child owners.
type dailyLaunchOutput struct {
	file           *os.File
	mu             sync.Mutex
	interrupted    bool
	interruptCause error
	closeErr       error
}

func (w *dailyLaunchOutput) interrupt(cause error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.interrupted {
		return
	}
	w.interrupted = true
	w.interruptCause = cause
	if e := w.file.Close(); e != nil {
		w.closeErr = errors.Join(errRemoteLaunchCleanup, e)
	}
}
func (w *dailyLaunchOutput) write(ctx context.Context, value any) error {
	if w == nil || w.file == nil || ctx == nil {
		return errRemoteLaunchUnavailable
	}
	raw, e := json.Marshal(value)
	if e != nil || len(raw)+1 > 512 {
		return errors.Join(errRemoteLaunchUnavailable, e)
	}
	if e = ctx.Err(); e != nil {
		return e // No write was attempted; preserve the exact caller cause.
	}
	work, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	joined := make(chan struct{})
	stop := context.AfterFunc(work, func() { defer close(joined); w.interrupt(work.Err()) })
	raw = append(raw, '\n')
	n, writeErr := w.file.Write(raw)
	if !stop() {
		<-joined
	}
	w.mu.Lock()
	ce, interrupted, cause := w.closeErr, w.interrupted, w.interruptCause
	w.mu.Unlock()
	return dailyLaunchWriteResult(ctx.Err(), work.Err(), writeErr, n, len(raw), interrupted, cause, ce)
}

// Only facts sampled after the original write and its interrupt have joined
// enter this private projection. A partial frame, EPIPE, failed close, local
// deadline, or unrelated error is never a clean parent-cancellation result.
func dailyLaunchWriteResult(parentErr, workErr, writeErr error, n, size int, interrupted bool, cause, closeErr error) error {
	if parentErr == context.Canceled && workErr == context.Canceled && closeErr == nil {
		if n == size && writeErr == nil && (!interrupted || cause == context.Canceled) {
			return context.Canceled
		}
		if n == 0 && interrupted && cause == context.Canceled && errors.Is(writeErr, os.ErrClosed) {
			// os.File.Write supplies either its own exact error or a PathError;
			// accept no arbitrary joined wrapper that also carries a fatal cause.
			if writeErr == os.ErrClosed {
				return context.Canceled
			}
			if e, ok := writeErr.(*os.PathError); ok && e.Op == "write" && e.Err == os.ErrClosed {
				return context.Canceled
			}
		}
	}
	if n == size && writeErr == nil && !interrupted && closeErr == nil && workErr == nil && parentErr == nil {
		return nil
	}
	var short error
	if n != size {
		short = io.ErrShortWrite
	}
	return errors.Join(errRemoteLaunchUnavailable, writeErr, short, closeErr, workErr, parentErr)
}

// This is used only after the original owner was successfully opened and after
// its Close plus the original output cleanup have returned successfully. It
// cannot normalize a constructor refusal or an erased generic error.
func dailyLaunchParentCancellation(ctx context.Context, result error) bool {
	if ctx == nil || ctx.Err() != context.Canceled || result == nil {
		return false
	}
	return dailyLaunchCancellationOnly(result)
}
func dailyLaunchCancellationOnly(e error) bool {
	if e == context.Canceled {
		return true
	}
	if owner, ok := e.(*remoteLaunchError); ok {
		return owner.retained == nil && dailyLaunchCancellationOnly(owner.cause)
	}
	if _, cleanup := e.(*dailyLaunchCleanupError); cleanup {
		return false
	}
	if many, ok := e.(interface{ Unwrap() []error }); ok {
		children := many.Unwrap()
		if len(children) == 0 {
			return false
		}
		for _, child := range children {
			if !dailyLaunchCancellationOnly(child) {
				return false
			}
		}
		return true
	}
	return false
}

type dailyLaunchStatus struct {
	Version      uint32 `json:"version"`
	ObservedAtMS int64  `json:"observed_at_ms"`
	ConfigID     string `json:"config_id"`
	State        string `json:"state"`
	WorkerState  string `json:"worker_state"`
	Code         string `json:"code"`
	Records      uint64 `json:"records"`
	Pending      uint64 `json:"pending"`
}

// runDailyRemoteLaunch is an explicit existing-session headless worker. It does
// not serve helper commands, start a node, create a wallet, or retry a failed
// operation. Close performs the original controller's joined stop/logout path.
func runDailyRemoteLaunch(ctx context.Context, output *os.File, paths remoteLaunchPaths, options pilot.PoolOptions) (result error) {
	if output == nil {
		return errRemoteLaunchInvalid
	}
	signal.Ignore(syscall.SIGPIPE)
	o, expected, e := openRemoteLaunchMode(ctx, paths, options, true)
	if e != nil {
		return e
	}
	w := &dailyLaunchOutput{file: output}
	defer func() { result = finishDailyLaunch(ctx, o, w, result) }()
	if _, e = o.dailyCall(ctx, "login_fixture", expected.ExpectedLocal.Recipient.Config.Fixture); e != nil {
		return e
	}
	snapshot, e := o.dailyCall(ctx, "snapshot", "")
	if e != nil {
		return e
	}
	if snapshot.View == nil || snapshot.View.ViewID == "" || snapshot.View.GymID != expected.ExpectedLocal.Recipient.Config.Identity.GymID {
		return errRemoteLaunchIntegrity
	}
	view := snapshot.View.ViewID
	if _, e = o.dailyCall(ctx, "start", view); e != nil {
		return e
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		snapshot, e = o.dailyCall(ctx, "snapshot", "")
		if e != nil {
			return e
		}
		if snapshot.View == nil || snapshot.View.ViewID != view || snapshot.Remote == nil {
			return errRemoteLaunchUnavailable
		}
		r := snapshot.Remote
		if e = w.write(ctx, dailyLaunchStatus{2, time.Now().UTC().UnixMilli(), snapshot.ConfigID, snapshot.State, r.WorkerState, r.LastError, r.Records, r.Pending}); e != nil {
			return e
		}
		// Held issuance remains visible and nonissuing. The original worker can still
		// reconcile retained pending deliveries. No replacement controller is made.
		select {
		case <-ctx.Done():
			if ctx.Err() == context.Canceled {
				return nil
			}
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func finishDailyLaunch(ctx context.Context, o *remoteLaunchOwner, w *dailyLaunchOutput, result error) error {
	if o == nil || w == nil {
		return errors.Join(errRemoteLaunchUnavailable, result)
	}
	ce := o.Close()
	w.mu.Lock()
	oe := w.closeErr
	w.mu.Unlock()
	if ce == nil && oe == nil && dailyLaunchParentCancellation(ctx, result) {
		result = nil
	}
	if result != nil || ce != nil || oe != nil {
		failure := &remoteLaunchError{cause: errors.Join(result, ce, oe)}
		if ce != nil {
			failure.retained = o
		}
		if oe != nil {
			result = &dailyLaunchCleanupError{owner: o, output: w, cause: failure}
		} else {
			result = failure
		}
	}
	return result
}

type dailyLaunchCleanupError struct {
	owner  *remoteLaunchOwner
	output *dailyLaunchOutput
	cause  error
}

func (*dailyLaunchCleanupError) Error() string   { return "daily remote worker cleanup incomplete" }
func (e *dailyLaunchCleanupError) Unwrap() error { return e.cause }
