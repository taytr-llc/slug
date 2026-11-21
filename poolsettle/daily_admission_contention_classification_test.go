// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettle

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/taytr-llc/slug/poolround"
	"github.com/taytr-llc/slug/poolshare"
)

type dailyContentionEmptyTree struct{}

func (dailyContentionEmptyTree) Error() string   { return "empty" }
func (dailyContentionEmptyTree) Unwrap() []error { return []error{} }

type dailyContentionNilChild struct{}

func (dailyContentionNilChild) Error() string   { return "nil-child" }
func (dailyContentionNilChild) Unwrap() []error { return []error{ErrBusy, nil} }

// Hand-derived error classification only: these values are not forged source
// observations or expected financial outputs. Every observed fatal sibling wins.
func TestDailyAdmissionContentionErrorTrees(t *testing.T) {
	cases := []struct {
		name  string
		err   error
		retry bool
	}{
		{"nil", nil, false},
		{"ledger-busy", ErrBusy, true},
		{"round-busy", poolround.ErrBusy, true},
		{"wrapped-ledger", fmt.Errorf("original owner: %w", ErrBusy), true},
		{"wrapped-round", fmt.Errorf("original source: %w", poolround.ErrBusy), true},
		{"both-busy", errors.Join(ErrBusy, poolround.ErrBusy), true},
		{"nested-busy", errors.Join(fmt.Errorf("outer: %w", errors.Join(poolround.ErrBusy, ErrBusy)), poolround.ErrBusy), true},
		{"empty-tree", dailyContentionEmptyTree{}, false},
		{"nil-child", dailyContentionNilChild{}, false},
		{"text-lookalike", errors.New(ErrBusy.Error()), false},
		{"raw-source-busy", poolshare.ErrBusy, false},
		{"ledger-corrupt", ErrCorrupt, false},
		{"round-corrupt", poolround.ErrCorrupt, false},
		{"closed", ErrClosed, false},
		{"poisoned", ErrPoisoned, false},
		{"source-unavailable", poolround.ErrSourceUnavailable, false},
		{"parent-cancel", context.Canceled, false},
		{"deadline", context.DeadlineExceeded, false},
		{"busy-corrupt", errors.Join(ErrBusy, ErrCorrupt), false},
		{"busy-round-corrupt", errors.Join(poolround.ErrBusy, poolround.ErrCorrupt), false},
		{"busy-closed", errors.Join(ErrBusy, ErrClosed), false},
		{"busy-cancel", errors.Join(poolround.ErrBusy, context.Canceled), false},
		{"busy-deadline", errors.Join(ErrBusy, context.DeadlineExceeded), false},
		{"nested-fatal", fmt.Errorf("outer: %w", errors.Join(poolround.ErrBusy, errors.Join(ErrBusy, ErrPoisoned))), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := dailyContentionBusy(c.err); got != c.retry {
				t.Fatalf("retry=%v want=%v error=%v", got, c.retry, c.err)
			}
		})
	}
}

func TestDailyAdmissionContentionBudgetDoesNotRenew(t *testing.T) {
	// An expired private budget models elapsed contention without waiting 30s.
	// Production's selected bound and the financial/store clocks are untouched.
	deadline := time.Now().Add(-time.Millisecond)
	budget := dailyContention{deadline: deadline}
	for i := 0; i < 3; i++ {
		if err := budget.pause(context.Background()); err != context.DeadlineExceeded {
			t.Fatal("background contention renewed expired budget", err)
		}
		if !budget.deadline.Equal(deadline) {
			t.Fatal("shared contention deadline changed")
		}
	}
	parent, cancel := context.WithCancel(context.Background())
	cancel()
	future := time.Now().Add(time.Hour)
	budget = dailyContention{deadline: future}
	if err := budget.pause(parent); err != context.Canceled {
		t.Fatal("parent cancellation lost", err)
	}
	if !budget.deadline.Equal(future) {
		t.Fatal("cancellation changed original budget")
	}
	// Two independent read sites sharing this invocation receive one deadline.
	budget = dailyContention{}
	first, stopFirst := budget.context(context.Background())
	defer stopFirst()
	second, stopSecond := budget.context(context.Background())
	defer stopSecond()
	a, aok := first.Deadline()
	b, bok := second.Deadline()
	if !aok || !bok || !a.Equal(b) || !a.Equal(budget.deadline) {
		t.Fatal("same invocation received replacement deadlines")
	}
}
