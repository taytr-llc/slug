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

// DailyWalletContext cannot be manufactured from a public view. Both the exact
// context pointer and the original process owner must remain current.
type DailyWalletContext struct {
	helper *DailyWalletBridge
	view   pilot.RemotePoolView
}

func (v *DailyWalletContext) View() pilot.RemotePoolView {
	if v == nil {
		return pilot.RemotePoolView{}
	}
	return v.view
}

type DailyWalletBridge struct {
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
	config                       pilot.RecoverableRemotePoolConfig
	configID                     string
	view                         *DailyWalletContext
	lastViewID                   string
	nextID, awaiting             uint64
	delivered                    bool
	callDone                     chan struct{}
}

func (b *DailyWalletBridge) Context() (*DailyWalletContext, error) {
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
func (b *DailyWalletBridge) ContextMatches(v *DailyWalletContext) bool {
	if b == nil || v == nil {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return !b.retired && b.view == v && v.helper == b
}

func (b *DailyWalletBridge) Snapshot(ctx context.Context, original *DailyWalletContext) (pilot.DailyWalletResponse, error) {
	return b.requestDailyWallet(ctx, original, "snapshot", "", 0, 0, 0)
}
func (b *DailyWalletBridge) LoginFixture(ctx context.Context, fixture string) (pilot.DailyWalletResponse, error) {
	return b.requestDailyWallet(ctx, nil, "login_fixture", fixture, 0, 0, 0)
}
func (b *DailyWalletBridge) Start(ctx context.Context, original *DailyWalletContext) (pilot.DailyWalletResponse, error) {
	return b.requestDailyWallet(ctx, original, "start", "", 0, 0, 0)
}
func (b *DailyWalletBridge) Stop(ctx context.Context, original *DailyWalletContext) (pilot.DailyWalletResponse, error) {
	return b.requestDailyWallet(ctx, original, "stop", "", 0, 0, 0)
}
func (b *DailyWalletBridge) Logout(ctx context.Context, original *DailyWalletContext) (pilot.DailyWalletResponse, error) {
	return b.requestDailyWallet(ctx, original, "logout", "", 0, 0, 0)
}

func (b *DailyWalletBridge) requestDailyWallet(ctx context.Context, original *DailyWalletContext, command, fixture string, sequence, captureSequence, beforeBatch uint64) (out pilot.DailyWalletResponse, resultErr error) {
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
	if command == "login_fixture" && (b.view != nil || fixture != b.config.Base.ExpectedLocal.Recipient.Config.Fixture) {
		b.mu.Unlock()
		return out, remotePoolFailure("invalid_request", nil)
	}
	if command != "snapshot" && command != "login_fixture" && b.view == nil {
		b.mu.Unlock()
		return out, remotePoolFailure("stale_view", nil)
	}
	if b.nextID == ^uint64(0) {
		b.mu.Unlock()
		return out, b.retireDailyWalletRequest(false, ErrUnavailable)
	}
	var prior *pilot.RemotePoolView
	viewID := ""
	if original != nil {
		copy := original.view
		prior = &copy
		viewID = copy.ViewID
	}
	r := pilot.DailyWalletRequest{Version: 1, Application: pilot.DailyWalletApplication, ID: b.nextID + 1, Command: command, ConfigID: b.configID, ViewID: viewID, Fixture: fixture, Sequence: sequence, CaptureSequence: captureSequence, BeforeBatch: beforeBatch}
	callDone := make(chan struct{})
	b.callDone = callDone
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		b.callDone = nil
		close(callDone)
		b.mu.Unlock()
	}()

	raw, e := pilot.MarshalDailyWalletRequest(r, b.config)
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
		b.retireDailyWallet(work.Err())
	})
	raw = append(raw, '\n')
	n, writeErr := b.input.Write(raw)
	if !stopWrite() {
		<-writeCancelled
	}
	if writeErr != nil || n != len(raw) || work.Err() != nil {
		return out, b.retireDailyWalletRequest(mutation, errors.Join(writeErr, work.Err(), ErrUnavailable))
	}
	select {
	case raw = <-b.lines:
	case <-work.Done():
		return out, b.retireDailyWalletRequest(mutation, work.Err())
	case <-b.failed:
		return out, b.retireDailyWalletRequest(mutation, ErrUnavailable)
	}
	if e = work.Err(); e != nil {
		return out, b.retireDailyWalletRequest(mutation, e)
	}
	response, e := pilot.ParseDailyWalletResponse(raw, r, b.config, prior)
	if e != nil {
		return out, b.retireDailyWalletRequest(mutation, e)
	}
	if e = work.Err(); e != nil {
		return out, b.retireDailyWalletRequest(mutation, e)
	}
	b.mu.Lock()
	dead := b.retired || b.awaiting != r.ID || !b.delivered
	b.mu.Unlock()
	if dead {
		return out, b.retireDailyWalletRequest(mutation, ErrUnavailable)
	}
	if !response.OK {
		if response.Error == "outcome_unknown" || response.Error == "cleanup_incomplete" {
			cause := error(ErrRemotePoolOutcomeUnknown)
			if response.Error == "cleanup_incomplete" {
				cause = ErrRemotePoolBridgeCleanup
			}
			b.retireDailyWallet(cause)
			cleanup := b.joinDailyWalletCleanup()
			return out, remotePoolFailure(response.Error, errors.Join(cause, cleanup))
		}
		b.mu.Lock()
		b.awaiting = 0
		b.mu.Unlock()
		return out, remotePoolFailure(response.Error, nil)
	}
	out = response
	b.mu.Lock()
	valid := !b.retired && b.awaiting == r.ID && b.view == original
	if out.Snapshot == nil {
		valid = valid && original != nil && (command == "daily_payout_begin" || command == "daily_payout_read" || command == "daily_payout_cancel")
	} else {
		current := out.Snapshot
		if valid && r.ID == 1 {
			valid = command == "snapshot" && current.State == "signed_out" && current.View == nil && current.Remote == nil && current.Error == ""
		}
		if valid && command == "login_fixture" {
			valid = current.View != nil && current.Remote != nil && current.State == "signed_in" && current.Remote.State == "stopped" && current.View.ViewID != b.lastViewID
			if valid {
				b.view = &DailyWalletContext{helper: b, view: *current.View}
				b.lastViewID = current.View.ViewID
			}
		} else if valid && current.View != nil {
			valid = original != nil && *current.View == original.view
		} else if valid {
			valid = command == "snapshot" || command == "logout"
			if valid {
				b.view = nil
			}
		}
	}
	if valid {
		b.awaiting = 0
	}
	b.mu.Unlock()
	if !valid {
		return pilot.DailyWalletResponse{}, b.retireDailyWalletRequest(mutation, ErrUnavailable)
	}
	// The returned nested pointers are response-owned, not the retained context.
	return out, nil
}

func (b *DailyWalletBridge) retireDailyWalletRequest(mutation bool, cause error) error {
	b.retireDailyWallet(cause)
	cleanup := b.joinDailyWalletCleanup()
	if mutation {
		return remotePoolFailure("outcome_unknown", errors.Join(ErrRemotePoolOutcomeUnknown, cause, cleanup))
	}
	if cleanup != nil {
		return remotePoolFailure("cleanup_incomplete", errors.Join(cause, cleanup))
	}
	return remotePoolFailure("remote_unavailable", errors.Join(ErrUnavailable, cause))
}

func (b *DailyWalletBridge) joinDailyWalletCleanup() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return b.joinDailyWallet(ctx, false)
}

// Close retires the one original process. Timeout/failure retains that exact
// owner and its descriptors; a subsequent call cannot attach a replacement.
func (b *DailyWalletBridge) Close(ctx context.Context) error {
	if b == nil {
		return nil
	}
	if ctx == nil || b.failed == nil || b.waitDone == nil || b.readDone == nil {
		return ErrUnavailable
	}
	b.retireDailyWallet(ErrClosed)
	return b.joinDailyWallet(ctx, true)
}

func (b *DailyWalletBridge) joinDailyWallet(ctx context.Context, joinCall bool) error {
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

func (b *DailyWalletBridge) closeDailyWalletPipe(index int) {
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

func (b *DailyWalletBridge) retireDailyWallet(cause error) {
	b.retireOnce.Do(func() {
		b.mu.Lock()
		b.retired, b.failure, b.view = true, cause, nil
		close(b.failed)
		b.mu.Unlock()
		for i := range b.pipes {
			b.closeDailyWalletPipe(i)
		}
	})
}

// Fixed read commands retain the same opaque original process/View context.
func (b *DailyWalletBridge) BeginDailyPayoutRead(ctx context.Context, v *DailyWalletContext, sequence, capture, before uint64) (pilot.DailyWalletResponse, error) {
	return b.requestDailyWallet(ctx, v, "daily_payout_begin", "", sequence, capture, before)
}
func (b *DailyWalletBridge) DailyPayoutRead(ctx context.Context, v *DailyWalletContext, sequence uint64) (pilot.DailyWalletResponse, error) {
	return b.requestDailyWallet(ctx, v, "daily_payout_read", "", sequence, 0, 0)
}
func (b *DailyWalletBridge) CancelDailyPayoutRead(ctx context.Context, v *DailyWalletContext, sequence uint64) (pilot.DailyWalletResponse, error) {
	return b.requestDailyWallet(ctx, v, "daily_payout_cancel", "", sequence, 0, 0)
}
