//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymbout

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymcard"
	"testing"
)

func TestResultViewLowerCancellationUsesOwnerLifetime(t *testing.T) {
	for _, tc := range []struct {
		name       string
		closeOwner bool
		lowerError error
		want       error
	}{
		{"owner_close", true, context.Canceled, ErrClosed},
		{"caller_cancel", false, context.Canceled, ErrCancelled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newBoutFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var closed chan error
			entered, commits := 0, 0
			// Inject failure at the existing modeled account boundary. The real
			// Entry/Event/Seed/Card traversal must propagate and map it before
			// Bout classifies its own lifetime; no final consumer may run.
			session := f.session
			session.Lease = func(context.Context, gymaccount.Capability, func(gymaccount.Account, func() error) error) error {
				entered++
				if tc.closeOwner {
					closed = make(chan error, 1)
					go func() { closed <- f.s.Close() }()
					<-f.s.life.Done()
				} else {
					cancel()
				}
				return tc.lowerError
			}
			e := f.s.WithResultView(ctx, f.card, f.seed, f.event, f.entry, session, gymaccount.View, f.date(), f.id(), func(ResultView, func() error) error {
				commits++
				return nil
			})
			if closed != nil {
				if closeErr := <-closed; closeErr != nil {
					t.Fatal(closeErr)
				}
			}
			if !errors.Is(e, tc.want) || entered != 1 || commits != 0 {
				t.Fatalf("lower refusal: got=%v want=%v entered=%d commits=%d", e, tc.want, entered, commits)
			}
		})
	}
}

func TestResultViewClosePreservesConsumerErrors(t *testing.T) {
	opaque := errors.New("opaque consumer failure")
	for _, tc := range []struct {
		name string
		err  error
		want error
	}{
		{"opaque", opaque, opaque},
		{"unknown", gymcard.ErrOutcomeUnknown, ErrOutcomeUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newBoutFixture(t)
			closed := make(chan error, 1)
			commits := 0
			e := f.s.WithResultView(context.Background(), f.card, f.seed, f.event, f.entry, f.session, gymaccount.View, f.date(), f.id(), func(ResultView, func() error) error {
				commits++
				go func() { closed <- f.s.Close() }()
				<-f.s.life.Done()
				return tc.err
			})
			if commits != 1 {
				t.Fatalf("consumer not reached: %v", e)
			}
			if closeErr := <-closed; closeErr != nil {
				t.Fatal(closeErr)
			}
			if !errors.Is(e, tc.want) {
				t.Fatalf("consumer error replaced: got=%v want=%v", e, tc.want)
			}
		})
	}
}

func TestResultViewCompletedBodySurvivesLaterClose(t *testing.T) {
	f := newBoutFixture(t)
	closed := make(chan error, 1)
	completed := false
	f.s.checkpoint = func(stage string) {
		if stage != "result-view-complete" {
			return
		}
		completed = true
		go func() { closed <- f.s.Close() }()
		<-f.s.life.Done()
	}
	e := f.s.WithResultView(context.Background(), f.card, f.seed, f.event, f.entry, f.session, gymaccount.View, f.date(), f.id(), func(v ResultView, check func() error) error { return check() })
	if e != nil || !completed {
		t.Fatal("completed final guard replaced", e, completed)
	}
	if e = <-closed; e != nil {
		t.Fatal(e)
	}
}
