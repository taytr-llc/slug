// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/taytr-llc/slug/gymaccount"
)

// Hold only the scheduling of the real lifetime's AfterFunc notification.
// Done and Err still observe the original owner. Hiding Value prevents the
// standard context implementation from bypassing this explicit scheduling seam.
type heldStableAfterFunc struct {
	context.Context
	mu       sync.Mutex
	callback func()
	stopped  bool
}

func (*heldStableAfterFunc) Value(any) any { return nil }
func (h *heldStableAfterFunc) AfterFunc(f func()) func() bool {
	h.mu.Lock()
	h.callback = f
	h.mu.Unlock()
	return func() bool {
		h.mu.Lock()
		defer h.mu.Unlock()
		pending := h.callback != nil && !h.stopped
		h.stopped = true
		return pending
	}
}
func (h *heldStableAfterFunc) release() {
	h.mu.Lock()
	f := h.callback
	if h.stopped {
		f = nil
	}
	h.stopped = true
	h.mu.Unlock()
	if f != nil {
		f()
	}
}

// Expected from the original lifecycle contract, before the fix: Close joins
// the entered proof, then no second source read or stable assignment may occur.
// A later caller sees ErrClosed; the already-entered operation sees cancellation.
func TestSharedStableCloseObservesLifetimeBeforeCallback(t *testing.T) {
	x := newSharedFixture(t, "BA")
	a := x.gyms[0]
	held := &heldStableAfterFunc{Context: a.s.life}
	a.s.life = held
	t.Cleanup(held.release)
	entered, release := make(chan struct{}), make(chan struct{})
	reads := 0
	a.s.proofRead = func() {
		reads++
		if reads == 1 {
			close(entered)
			<-release
		}
	}
	var work context.Context
	session := a.session
	originalLease := session.Lease
	session.Lease = func(ctx context.Context, cap gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
		work = ctx
		return originalLease(ctx, cap, fn)
	}
	syncDone := make(chan error, 1)
	go func() { _, e := a.s.Sync(context.Background(), session); syncDone <- e }()
	<-entered
	closed := make(chan error, 1)
	go func() { closed <- a.s.Close() }()
	<-held.Done()
	// The source hook is blocked, so neither this observation nor reads races
	// another session/proof operation. The scheduler callback is still held.
	delayed := work != nil && work.Err() == nil
	earlyClose := false
	var closeErr error
	select {
	case closeErr = <-closed:
		earlyClose = true
	default:
	}
	close(release)
	syncErr := <-syncDone
	if !earlyClose {
		closeErr = <-closed
	}
	if !delayed || earlyClose || closeErr != nil {
		t.Fatal("original owner/callback scheduling or join", delayed, earlyClose, closeErr)
	}
	if reads != 1 || !errors.Is(syncErr, ErrCancelled) {
		t.Fatalf("owner closure allowed another proof or commit: reads=%d error=%v", reads, syncErr)
	}
	if _, e := a.s.Sync(context.Background(), a.session); !errors.Is(e, ErrClosed) {
		t.Fatal("later call lost closed-owner precedence", e)
	}
	reopenSharedFixture(t, a)
	status, e := a.s.SharedStatus(context.Background(), a.session)
	if e != nil || status.Status.Checkpoint != 0 || status.AssignmentCount != 0 {
		t.Fatal("close published incomplete source work", status, e)
	}
}
