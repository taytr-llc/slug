// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"sync"
	"time"

	"github.com/taytr-llc/slug/desktop/pilot"
)

var (
	ErrRemotePoolOutcomeUnknown = errors.New("Remote pool request outcome is unknown.")
	ErrRemotePoolBridgeCleanup  = errors.New("Remote pool helper cleanup is incomplete.")
)

// RemotePoolFailure exposes only a closed public code. Retained causes and
// concrete failed owners never become helper output or frontend data.
type RemotePoolFailure struct {
	Code  string
	cause error
}

func (e *RemotePoolFailure) Error() string {
	return "The remote pool request could not complete (" + e.Code + ")."
}
func (e *RemotePoolFailure) Unwrap() error { return e.cause }

// RemotePoolContext cannot be manufactured from a public view. Both the exact
// context pointer and the original process owner must remain current.
type RemotePoolContext struct {
	helper *RemotePoolBridge
	view   pilot.RemotePoolView
}

func (v *RemotePoolContext) View() pilot.RemotePoolView {
	if v == nil {
		return pilot.RemotePoolView{}
	}
	return v.view
}

type remotePoolPipe struct {
	owner  io.Closer
	closed bool
}

type RemotePoolBridge struct {
	cmd                          *exec.Cmd
	input                        io.WriteCloser
	output                       io.ReadCloser
	pipes                        []remotePoolPipe
	gate                         chan struct{}
	lines                        chan []byte
	failed, readDone, waitDone   chan struct{}
	retireOnce                   sync.Once
	mu                           sync.Mutex
	retired                      bool
	failure, cleanupErr, waitErr error
	config                       pilot.RemotePoolConfig
	configID                     string
	view                         *RemotePoolContext
	lastViewID                   string
	nextID, awaiting             uint64
	delivered                    bool
	callDone                     chan struct{}
}

func (b *RemotePoolBridge) Context() (*RemotePoolContext, error) {
	if b == nil {
		return nil, ErrUnavailable
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.retired || b.view == nil {
		return nil, ErrUnavailable
	}
	return b.view, nil
}
func (b *RemotePoolBridge) ContextMatches(v *RemotePoolContext) bool {
	if b == nil || v == nil {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return !b.retired && b.view == v && v.helper == b
}

func (b *RemotePoolBridge) Snapshot(ctx context.Context, original *RemotePoolContext) (pilot.RemotePoolWireSnapshot, error) {
	return b.requestRemote(ctx, original, "snapshot", "")
}
func (b *RemotePoolBridge) LoginFixture(ctx context.Context, fixture string) (pilot.RemotePoolWireSnapshot, error) {
	return b.requestRemote(ctx, nil, "login_fixture", fixture)
}
func (b *RemotePoolBridge) Start(ctx context.Context, original *RemotePoolContext) (pilot.RemotePoolWireSnapshot, error) {
	return b.requestRemote(ctx, original, "start", "")
}
func (b *RemotePoolBridge) Stop(ctx context.Context, original *RemotePoolContext) (pilot.RemotePoolWireSnapshot, error) {
	return b.requestRemote(ctx, original, "stop", "")
}
func (b *RemotePoolBridge) Logout(ctx context.Context, original *RemotePoolContext) (pilot.RemotePoolWireSnapshot, error) {
	return b.requestRemote(ctx, original, "logout", "")
}

func remotePoolFailure(code string, cause error) error {
	return &RemotePoolFailure{Code: code, cause: cause}
}

func (b *RemotePoolBridge) requestRemote(ctx context.Context, original *RemotePoolContext, command, fixture string) (out pilot.RemotePoolWireSnapshot, resultErr error) {
	if b == nil || ctx == nil {
		return out, ErrUnavailable
	}
	if e := ctx.Err(); e != nil {
		return out, e
	}
	select {
	case b.gate <- struct{}{}:
		defer func() { <-b.gate }()
	default:
		return out, ErrBusy
	}
	b.mu.Lock()
	if b.retired {
		b.mu.Unlock()
		return out, ErrClosed
	}
	if e := ctx.Err(); e != nil {
		b.mu.Unlock()
		return out, e
	}
	if (original == nil) != (b.view == nil) || original != nil && (original.helper != b || original != b.view) {
		b.mu.Unlock()
		return out, remotePoolFailure("stale_view", nil)
	}
	if command == "login_fixture" && (b.view != nil || fixture != b.config.ExpectedLocal.Recipient.Config.Fixture) {
		b.mu.Unlock()
		return out, remotePoolFailure("invalid_request", nil)
	}
	if command != "snapshot" && command != "login_fixture" && b.view == nil {
		b.mu.Unlock()
		return out, remotePoolFailure("stale_view", nil)
	}
	if b.nextID >= 9007199254740991 {
		b.mu.Unlock()
		return out, b.retireRemoteRequest(false, ErrUnavailable)
	}
	var prior *pilot.RemotePoolView
	viewID := ""
	if original != nil {
		copy := original.view
		prior = &copy
		viewID = copy.ViewID
	}
	r := pilot.RemotePoolRequest{Version: 1, ID: b.nextID + 1, Command: command, ConfigID: b.configID, ViewID: viewID, Fixture: fixture}
	callDone := make(chan struct{})
	b.callDone = callDone
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		b.callDone = nil
		close(callDone)
		b.mu.Unlock()
	}()

	raw, e := pilot.MarshalRemotePoolRequest(r, b.config)
	if e != nil {
		return out, remotePoolFailure("invalid_request", e)
	}
	work, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if e = work.Err(); e != nil {
		return out, e
	}
	b.mu.Lock()
	if b.retired {
		b.mu.Unlock()
		return out, ErrClosed
	}
	b.nextID, b.awaiting, b.delivered = r.ID, r.ID, false
	b.mu.Unlock()
	mutation := command != "snapshot"
	// From this point a write may have reached the original helper. Cancellation
	// closes the concrete pipe, and we synchronously join that write callback.
	writeCancelled := make(chan struct{})
	stopWrite := context.AfterFunc(work, func() {
		defer close(writeCancelled)
		b.retireRemote(work.Err())
	})
	raw = append(raw, '\n')
	n, writeErr := b.input.Write(raw)
	if !stopWrite() {
		<-writeCancelled
	}
	if writeErr != nil || n != len(raw) || work.Err() != nil {
		return out, b.retireRemoteRequest(mutation, errors.Join(writeErr, work.Err(), ErrUnavailable))
	}
	select {
	case raw = <-b.lines:
	case <-work.Done():
		return out, b.retireRemoteRequest(mutation, work.Err())
	case <-b.failed:
		return out, b.retireRemoteRequest(mutation, ErrUnavailable)
	}
	if e = work.Err(); e != nil {
		return out, b.retireRemoteRequest(mutation, e)
	}
	response, e := pilot.ParseRemotePoolResponse(raw, r, b.config, prior)
	if e != nil {
		return out, b.retireRemoteRequest(mutation, e)
	}
	if e = work.Err(); e != nil {
		return out, b.retireRemoteRequest(mutation, e)
	}
	b.mu.Lock()
	dead := b.retired || b.awaiting != r.ID || !b.delivered
	b.mu.Unlock()
	if dead {
		return out, b.retireRemoteRequest(mutation, ErrUnavailable)
	}
	if !response.OK {
		if response.Error == "outcome_unknown" || response.Error == "cleanup_incomplete" {
			cause := error(ErrRemotePoolOutcomeUnknown)
			if response.Error == "cleanup_incomplete" {
				cause = ErrRemotePoolBridgeCleanup
			}
			b.retireRemote(cause)
			cleanup := b.joinRemoteCleanup()
			return out, remotePoolFailure(response.Error, errors.Join(cause, cleanup))
		}
		b.mu.Lock()
		b.awaiting = 0
		b.mu.Unlock()
		return out, remotePoolFailure(response.Error, nil)
	}
	if response.Result == nil {
		return out, b.retireRemoteRequest(mutation, ErrUnavailable)
	}
	out = *response.Result
	b.mu.Lock()
	valid := !b.retired && b.awaiting == r.ID && b.view == original
	if valid && r.ID == 1 {
		valid = command == "snapshot" && out.State == "signed_out" && out.View == nil && out.Remote == nil && out.Error == ""
	}
	if valid && command == "login_fixture" {
		valid = out.View != nil && out.Remote != nil && out.State == "signed_in" && out.Remote.State == "stopped" && out.View.ViewID != b.lastViewID
		if valid {
			b.view = &RemotePoolContext{helper: b, view: *out.View}
			b.lastViewID = out.View.ViewID
		}
	} else if valid && out.View != nil {
		valid = original != nil && *out.View == original.view
	} else if valid {
		valid = command == "snapshot" || command == "logout"
		if valid {
			b.view = nil
		}
	}
	if valid {
		b.awaiting = 0
	}
	b.mu.Unlock()
	if !valid {
		return pilot.RemotePoolWireSnapshot{}, b.retireRemoteRequest(mutation, ErrUnavailable)
	}
	// The returned nested pointers are response-owned, not the retained context.
	return out, nil
}

func (b *RemotePoolBridge) retireRemoteRequest(mutation bool, cause error) error {
	b.retireRemote(cause)
	cleanup := b.joinRemoteCleanup()
	if mutation {
		return remotePoolFailure("outcome_unknown", errors.Join(ErrRemotePoolOutcomeUnknown, cause, cleanup))
	}
	if cleanup != nil {
		return remotePoolFailure("cleanup_incomplete", errors.Join(cause, cleanup))
	}
	return remotePoolFailure("remote_unavailable", errors.Join(ErrUnavailable, cause))
}

func (b *RemotePoolBridge) joinRemoteCleanup() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return b.joinRemote(ctx, false)
}

// Close retires the one original process. Timeout/failure retains that exact
// owner and its descriptors; a subsequent call cannot attach a replacement.
func (b *RemotePoolBridge) Close(ctx context.Context) error {
	if b == nil {
		return nil
	}
	if ctx == nil || b.failed == nil || b.waitDone == nil || b.readDone == nil {
		return ErrUnavailable
	}
	b.retireRemote(ErrClosed)
	return b.joinRemote(ctx, true)
}

func (b *RemotePoolBridge) joinRemote(ctx context.Context, joinCall bool) error {
	b.mu.Lock()
	call := b.callDone
	b.mu.Unlock()
	wait := []<-chan struct{}{b.waitDone, b.readDone}
	if joinCall && call != nil {
		wait = append(wait, call)
	}
	for _, done := range wait {
		select {
		case <-done:
		case <-ctx.Done():
			b.mu.Lock()
			b.cleanupErr = errors.Join(b.cleanupErr, ErrRemotePoolBridgeCleanup, ctx.Err())
			e := b.cleanupErr
			b.mu.Unlock()
			return remotePoolFailure("cleanup_incomplete", e)
		}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.cleanupErr != nil || b.waitErr != nil {
		return remotePoolFailure("cleanup_incomplete", errors.Join(ErrRemotePoolBridgeCleanup, b.cleanupErr, b.waitErr))
	}
	return nil
}

func (b *RemotePoolBridge) closeRemotePipe(index int) {
	p := &b.pipes[index]
	if p.closed {
		return
	}
	p.closed = true // A failed OS Close is never retried on a reused descriptor.
	if e := p.owner.Close(); e != nil {
		b.mu.Lock()
		b.cleanupErr = errors.Join(b.cleanupErr, ErrRemotePoolBridgeCleanup, e)
		b.mu.Unlock()
	}
}

func (b *RemotePoolBridge) retireRemote(cause error) {
	b.retireOnce.Do(func() {
		b.mu.Lock()
		b.retired, b.failure, b.view = true, cause, nil
		close(b.failed)
		b.mu.Unlock()
		for i := range b.pipes {
			b.closeRemotePipe(i)
		}
	})
}
