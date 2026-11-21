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

// DailyWalletContext2 cannot be manufactured from a public view. Both the exact
// context pointer and the original process owner must remain current.
type DailyWalletContext2 struct {
	helper *DailyWalletBridge2
	view   pilot.RemotePoolView
}

func (v *DailyWalletContext2) View() pilot.RemotePoolView {
	if v == nil {
		return pilot.RemotePoolView{}
	}
	return v.view
}

type DailyWalletBridge2 struct {
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
	view                         *DailyWalletContext2
	lastViewID                   string
	nextID, awaiting             uint64
	delivered                    bool
	callDone                     chan struct{}
}

func (b *DailyWalletBridge2) Context() (*DailyWalletContext2, error) {
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
func (b *DailyWalletBridge2) ContextMatches(v *DailyWalletContext2) bool {
	if b == nil || v == nil {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return !b.retired && b.view == v && v.helper == b
}

func (b *DailyWalletBridge2) Snapshot(ctx context.Context, original *DailyWalletContext2) (pilot.DailyWalletResponse2, error) {
	return b.requestDailyWallet2(ctx, original, "snapshot", "", 0, 0, 0, 0, 0, "", 0, 0)
}
func (b *DailyWalletBridge2) LoginFixture(ctx context.Context, fixture string) (pilot.DailyWalletResponse2, error) {
	return b.requestDailyWallet2(ctx, nil, "login_fixture", fixture, 0, 0, 0, 0, 0, "", 0, 0)
}
func (b *DailyWalletBridge2) Start(ctx context.Context, original *DailyWalletContext2) (pilot.DailyWalletResponse2, error) {
	return b.requestDailyWallet2(ctx, original, "start", "", 0, 0, 0, 0, 0, "", 0, 0)
}
func (b *DailyWalletBridge2) Stop(ctx context.Context, original *DailyWalletContext2) (pilot.DailyWalletResponse2, error) {
	return b.requestDailyWallet2(ctx, original, "stop", "", 0, 0, 0, 0, 0, "", 0, 0)
}
func (b *DailyWalletBridge2) Logout(ctx context.Context, original *DailyWalletContext2) (pilot.DailyWalletResponse2, error) {
	return b.requestDailyWallet2(ctx, original, "logout", "", 0, 0, 0, 0, 0, "", 0, 0)
}

func (b *DailyWalletBridge2) requestDailyWallet2(ctx context.Context, original *DailyWalletContext2, command, fixture string, sequence, captureSequence, beforeBatch uint64, after, limit uint32, txid string, index uint32, batch uint64) (out pilot.DailyWalletResponse2, resultErr error) {
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
		return out, b.retireDailyWalletRequest22(false, ErrUnavailable)
	}
	var prior *pilot.RemotePoolView
	viewID := ""
	if original != nil {
		copy := original.view
		prior = &copy
		viewID = copy.ViewID
	}
	r := pilot.DailyWalletRequest2{Version: 2, Application: pilot.DailyWalletApplication2, ID: b.nextID + 1, Command: command, ConfigID: b.configID, ViewID: viewID, Fixture: fixture, Sequence: sequence, CaptureSequence: captureSequence, BeforeBatch: beforeBatch, After: after, Limit: limit, TxID: txid, OutputIndex: index, BatchSequence: batch}
	callDone := make(chan struct{})
	b.callDone = callDone
	b.mu.Unlock()
	defer func() {
		b.mu.Lock()
		b.callDone = nil
		close(callDone)
		b.mu.Unlock()
	}()

	raw, e := pilot.MarshalDailyWalletRequest2(r, b.config)
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
		b.retireDailyWallet2(work.Err())
	})
	raw = append(raw, '\n')
	n, writeErr := b.input.Write(raw)
	if !stopWrite() {
		<-writeCancelled
	}
	if writeErr != nil || n != len(raw) || work.Err() != nil {
		return out, b.retireDailyWalletRequest22(mutation, errors.Join(writeErr, work.Err(), ErrUnavailable))
	}
	select {
	case raw = <-b.lines:
	case <-work.Done():
		return out, b.retireDailyWalletRequest22(mutation, work.Err())
	case <-b.failed:
		return out, b.retireDailyWalletRequest22(mutation, ErrUnavailable)
	}
	if e = work.Err(); e != nil {
		return out, b.retireDailyWalletRequest22(mutation, e)
	}
	response, e := pilot.ParseDailyWalletResponse2(raw, r, b.config, prior)
	if e != nil {
		return out, b.retireDailyWalletRequest22(mutation, e)
	}
	if e = work.Err(); e != nil {
		return out, b.retireDailyWalletRequest22(mutation, e)
	}
	b.mu.Lock()
	dead := b.retired || b.awaiting != r.ID || !b.delivered
	b.mu.Unlock()
	if dead {
		return out, b.retireDailyWalletRequest22(mutation, ErrUnavailable)
	}
	if !response.OK {
		if response.Error == "outcome_unknown" || response.Error == "cleanup_incomplete" {
			cause := error(ErrRemotePoolOutcomeUnknown)
			if response.Error == "cleanup_incomplete" {
				cause = ErrRemotePoolBridgeCleanup
			}
			b.retireDailyWallet2(cause)
			cleanup := b.joinDailyWalletCleanup2()
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
		valid = valid && original != nil && (command == "daily_payout_begin" || command == "daily_payout_read" || command == "daily_payout_cancel" || command == "wallet_activity_begin" || command == "wallet_activity_read" || command == "wallet_activity_cancel" || command == "wallet_activity_page" || command == "wallet_activity_detail" || command == "daily_payout_observe")
	} else {
		current := out.Snapshot
		if valid && r.ID == 1 {
			valid = command == "snapshot" && current.State == "signed_out" && current.View == nil && current.Remote == nil && current.Error == ""
		}
		if valid && command == "login_fixture" {
			valid = current.View != nil && current.Remote != nil && current.State == "signed_in" && current.Remote.State == "stopped" && current.View.ViewID != b.lastViewID
			if valid {
				b.view = &DailyWalletContext2{helper: b, view: *current.View}
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
		return pilot.DailyWalletResponse2{}, b.retireDailyWalletRequest22(mutation, ErrUnavailable)
	}
	// The returned nested pointers are response-owned, not the retained context.
	return out, nil
}

func (b *DailyWalletBridge2) retireDailyWalletRequest22(mutation bool, cause error) error {
	b.retireDailyWallet2(cause)
	cleanup := b.joinDailyWalletCleanup2()
	if mutation {
		return remotePoolFailure("outcome_unknown", errors.Join(ErrRemotePoolOutcomeUnknown, cause, cleanup))
	}
	if cleanup != nil {
		return remotePoolFailure("cleanup_incomplete", errors.Join(cause, cleanup))
	}
	return remotePoolFailure("remote_unavailable", errors.Join(ErrUnavailable, cause))
}

func (b *DailyWalletBridge2) joinDailyWalletCleanup2() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return b.joinDailyWallet2(ctx, false)
}

// Close retires the one original process. Timeout/failure retains that exact
// owner and its descriptors; a subsequent call cannot attach a replacement.
func (b *DailyWalletBridge2) Close(ctx context.Context) error {
	if b == nil {
		return nil
	}
	if ctx == nil || b.failed == nil || b.waitDone == nil || b.readDone == nil {
		return ErrUnavailable
	}
	b.retireDailyWallet2(ErrClosed)
	return b.joinDailyWallet2(ctx, true)
}

func (b *DailyWalletBridge2) joinDailyWallet2(ctx context.Context, joinCall bool) error {
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

func (b *DailyWalletBridge2) closeDailyWalletPipe2(index int) {
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

func (b *DailyWalletBridge2) retireDailyWallet2(cause error) {
	b.retireOnce.Do(func() {
		b.mu.Lock()
		b.retired, b.failure, b.view = true, cause, nil
		close(b.failed)
		b.mu.Unlock()
		for i := range b.pipes {
			b.closeDailyWalletPipe2(i)
		}
	})
}

// Fixed read commands retain the same opaque original process/View context.
func (b *DailyWalletBridge2) BeginDailyPayoutRead(ctx context.Context, v *DailyWalletContext2, sequence, capture, before uint64) (pilot.DailyWalletResponse2, error) {
	return b.requestDailyWallet2(ctx, v, "daily_payout_begin", "", sequence, capture, before, 0, 0, "", 0, 0)
}
func (b *DailyWalletBridge2) DailyPayoutRead(ctx context.Context, v *DailyWalletContext2, sequence uint64) (pilot.DailyWalletResponse2, error) {
	return b.requestDailyWallet2(ctx, v, "daily_payout_read", "", sequence, 0, 0, 0, 0, "", 0, 0)
}
func (b *DailyWalletBridge2) CancelDailyPayoutRead(ctx context.Context, v *DailyWalletContext2, sequence uint64) (pilot.DailyWalletResponse2, error) {
	return b.requestDailyWallet2(ctx, v, "daily_payout_cancel", "", sequence, 0, 0, 0, 0, "", 0, 0)
}

func (b *DailyWalletBridge2) BeginWalletActivityRead(ctx context.Context, v *DailyWalletContext2, sequence uint64) (pilot.DailyWalletResponse2, error) {
	return b.requestDailyWallet2(ctx, v, "wallet_activity_begin", "", sequence, 0, 0, 0, 0, "", 0, 0)
}
func (b *DailyWalletBridge2) WalletActivityRead(ctx context.Context, v *DailyWalletContext2, sequence uint64) (pilot.DailyWalletResponse2, error) {
	return b.requestDailyWallet2(ctx, v, "wallet_activity_read", "", sequence, 0, 0, 0, 0, "", 0, 0)
}
func (b *DailyWalletBridge2) CancelWalletActivityRead(ctx context.Context, v *DailyWalletContext2, sequence uint64) (pilot.DailyWalletResponse2, error) {
	return b.requestDailyWallet2(ctx, v, "wallet_activity_cancel", "", sequence, 0, 0, 0, 0, "", 0, 0)
}
func (b *DailyWalletBridge2) WalletActivityPage(ctx context.Context, v *DailyWalletContext2, capture uint64, after, limit uint32) (pilot.DailyWalletResponse2, error) {
	return b.requestDailyWallet2(ctx, v, "wallet_activity_page", "", 0, capture, 0, after, limit, "", 0, 0)
}
func (b *DailyWalletBridge2) WalletActivityDetail(ctx context.Context, v *DailyWalletContext2, capture uint64, txid string) (pilot.DailyWalletResponse2, error) {
	return b.requestDailyWallet2(ctx, v, "wallet_activity_detail", "", 0, capture, 0, 0, 0, txid, 0, 0)
}
func (b *DailyWalletBridge2) ObserveDailyPayout(ctx context.Context, v *DailyWalletContext2, capture, batch uint64, txid string, index uint32) (pilot.DailyWalletResponse2, error) {
	return b.requestDailyWallet2(ctx, v, "daily_payout_observe", "", 0, capture, 0, 0, 0, txid, index, batch)
}
