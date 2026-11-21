// Copyright (c) 2026 The SLUG developers. ISC license.
package pooloperator

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/taytr-llc/slug/poolround"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/registry"
)

type intakeBusyTree struct{ children []error }

func (e intakeBusyTree) Error() string   { return "conditional error tree" }
func (e intakeBusyTree) Unwrap() []error { return e.children }

type intakeBusyWrapper struct{ child error }

func (e intakeBusyWrapper) Error() string { return "conditional single wrapper" }
func (e intakeBusyWrapper) Unwrap() error { return e.child }

// Hand-selected error categories, not generated production expected values.
// Only an exact nonempty Busy tree is safe to repeat as the SAME invocation.
func TestOperatorIntakeSourceBusyErrorTrees(t *testing.T) {
	busy := poolshare.ErrBusy
	cases := []struct {
		name  string
		err   error
		retry bool
	}{
		{"nil", nil, false},
		{"exact source busy", busy, true},
		{"wrapped source busy", fmt.Errorf("original source: %w", busy), true},
		{"joined source busy", errors.Join(busy, busy), true},
		{"nested source busy", fmt.Errorf("pass: %w", errors.Join(busy, fmt.Errorf("source: %w", busy))), true},
		{"single wrapper", intakeBusyWrapper{busy}, true},
		{"registry busy", registry.ErrBusy, false},
		{"round busy", poolround.ErrBusy, false},
		{"unavailable", ErrUnavailable, false},
		{"corrupt", poolshare.ErrCorrupt, false},
		{"poisoned", poolshare.ErrPoisoned, false},
		{"capacity", poolshare.ErrCapacity, false},
		{"cancel", context.Canceled, false},
		{"deadline", context.DeadlineExceeded, false},
		{"same text is not sentinel", errors.New(busy.Error()), false},
		{"busy and integrity", errors.Join(busy, poolshare.ErrCorrupt), false},
		{"busy and unavailable", errors.Join(busy, ErrUnavailable), false},
		{"busy and cancel", errors.Join(busy, context.Canceled), false},
		{"busy and deadline", errors.Join(busy, context.DeadlineExceeded), false},
		{"wrapped mixed fatal", fmt.Errorf("pass: %w", errors.Join(fmt.Errorf("source: %w", busy), poolshare.ErrPoisoned)), false},
		{"empty multi unwrap", intakeBusyTree{}, false},
		{"nil child", intakeBusyTree{[]error{nil}}, false},
		{"nil mixed child", intakeBusyTree{[]error{busy, nil}}, false},
		{"empty single unwrap", intakeBusyWrapper{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := intakeSourceBusyOnly(tc.err); got != tc.retry {
				t.Fatalf("retry=%v want=%v", got, tc.retry)
			}
		})
	}
}

// A blocked intakeMu is still a joined owner: cancellation does not pretend to
// preempt the holder. After its release the original expired context must win
// before any controller dereference or enabling side effect.
func TestOperatorIntakeMutexCancellationJoinsHolder(t *testing.T) {
	r := &Runtime{}
	r.intakeMu.Lock()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- r.updateIntake(ctx, true, true, "public_capacity") }()
	// No pool/server exist: reaching the transition would panic. This condition
	// specifically exercises the post-acquisition cancellation check.
	cancel()
	select {
	case err := <-done:
		r.intakeMu.Unlock()
		t.Fatal("canceled waiter did not join mutex holder", err)
	case <-time.After(20 * time.Millisecond):
	}
	r.intakeMu.Unlock()
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("mutex waiter failed to join")
	}
	if r.intakePublicReady || r.intakeRoundReady || r.intakeReason != "" {
		t.Fatal("expired waiter changed original transition state")
	}
}
