//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymseed

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymevent"
)

func TestCardPreparedViewAuthorityAndCallbackLifetime(t *testing.T) {
	x, s, _ := seedFixture(t)
	base := x.session[0].Lease
	inLease := false
	x.session[0].Lease = func(ctx context.Context, cap gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
		return base(ctx, cap, func(a gymaccount.Account, check func() error) error {
			inLease = true
			defer func() { inLease = false }()
			return fn(a, check)
		})
	}
	prepared, committed := 0, 0
	prepare := func(info Info) ([]string, func(CardView, func() error) error, error) {
		prepared++
		if inLease || info.StoreID != s.meta.StoreID || info.CreatedAtMS != s.meta.CreatedAtMS || !equal(info.Pin, s.meta.Pin) {
			t.Fatal("preparation must receive bound owned metadata outside lease")
		}
		// ValidateInfo owns the nested participant slice; changing the consumer's
		// copy cannot change the retained source or final View.
		info.Pin.SourcePin.Participants[0].StablePin.AccountID = "consumer edit"
		return []string{"2026-11-21"}, func(v CardView, check func() error) error {
			committed++
			if !inLease || !equal(v.Pin, s.meta.Pin) || len(v.States) != 1 || v.States[0].Status.Exists {
				t.Fatal("final source binding/lease")
			}
			return check()
		}, nil
	}
	x.revoke[0] = true
	if e := s.WithCardPreparedView(t.Context(), x.s, x.entry[0], x.session[0], prepare); !errors.Is(e, ErrUnauthorized) || prepared != 0 || committed != 0 {
		t.Fatal("unauthorized callback", e, prepared, committed)
	}
	x.revoke[0] = false
	if e := s.WithCardPreparedView(t.Context(), x.s, x.entry[0], x.session[0], prepare); e != nil || prepared != 1 || committed != 1 {
		t.Fatal("authorized metadata read", e, prepared, committed)
	}
	for _, cancelWork := range []bool{false, true} {
		ctx, cancel := context.WithCancel(t.Context())
		before := committed
		e := s.WithCardPreparedView(ctx, x.s, x.entry[0], x.session[0], func(info Info) ([]string, func(CardView, func() error) error, error) {
			dates, commit, err := prepare(info)
			if cancelWork {
				cancel()
			} else {
				x.revoke[0] = true
			}
			return dates, commit, err
		})
		cancel()
		want := ErrUnauthorized
		if cancelWork {
			want = ErrCancelled
		}
		if !errors.Is(e, want) || committed != before {
			t.Fatal("stale prepared callback executed", e, want, committed, before)
		}
		x.revoke[0] = false
	}
	for _, cancelWork := range []bool{false, true} {
		ctx, cancel := context.WithCancel(t.Context())
		x.now++
		e := s.WithCardPreparedView(ctx, x.s, x.entry[0], x.session[0], func(info Info) ([]string, func(CardView, func() error) error, error) {
			return []string{}, func(CardView, func() error) error {
				if cancelWork {
					cancel()
				} else {
					x.revoke[0] = true
				}
				return nil
			}, nil
		})
		cancel()
		if !errors.Is(e, ErrOutcomeUnknown) || s.meta.ClockMS != x.now {
			t.Fatal("durable observation lost uncertainty", e)
		}
		x.revoke[0] = false
	}
	before := s.meta.ClockMS
	if e := s.WithCardPreparedView(t.Context(), nil, x.entry[0], x.session[0], prepare); !errors.Is(e, ErrInvalid) || s.meta.ClockMS != before {
		t.Fatal("nil source had effects", e)
	}
	for _, dates := range [][]string{nil, {"2026-11-21", "2026-11-21"}, {"2026-11-28", "2026-11-21"}} {
		before := committed
		e := s.WithCardPreparedView(t.Context(), x.s, x.entry[0], x.session[0], func(Info) ([]string, func(CardView, func() error) error, error) {
			return dates, func(CardView, func() error) error { committed++; return nil }, nil
		})
		if !errors.Is(e, ErrInvalid) || committed != before {
			t.Fatal("invalid date selection", e)
		}
	}
}

func TestCardPreparedViewCloseJoinsAndPreservesUncertainty(t *testing.T) {
	foreignUnknown := errors.New("outcome_unknown") // A different package's opaque error identity.
	for _, c := range []struct {
		name             string
		afterObservation bool
		prepareError     error
		want             error
	}{
		{"during_preparation", false, nil, ErrClosed},
		{"preparation_returns_cancellation", false, context.Canceled, ErrClosed},
		{"preparation_returns_foreign_unknown", false, foreignUnknown, foreignUnknown},
		{"after_observation", true, nil, ErrOutcomeUnknown},
	} {
		t.Run(c.name, func(t *testing.T) {
			x, s, _ := seedFixture(t)
			x.now++
			entered, release := make(chan struct{}), make(chan struct{})
			result, closed := make(chan error, 1), make(chan error, 1)
			commits := 0
			go func() {
				result <- s.WithCardPreparedView(t.Context(), x.s, x.entry[0], x.session[0], func(Info) ([]string, func(CardView, func() error) error, error) {
					if !c.afterObservation {
						close(entered)
						<-release
					}
					if c.prepareError != nil {
						return nil, nil, c.prepareError
					}
					return []string{}, func(CardView, func() error) error {
						commits++
						if c.afterObservation {
							close(entered)
							<-release
						}
						return nil
					}, nil
				})
			}()
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("preparation did not start")
			}
			go func() { closed <- s.Close() }()
			<-s.life.Done()
			select {
			case e := <-closed:
				t.Fatal("Close returned before owned operation joined", e)
			default:
			}
			close(release)
			var e error
			select {
			case e = <-result:
			case <-time.After(3 * time.Second):
				t.Fatal("operation failed to join")
			}
			count := 0
			if c.afterObservation {
				count = 1
			}
			if !errors.Is(e, c.want) || commits != count {
				t.Fatal("source closure classification", e, c.want, commits, count)
			}
			select {
			case e = <-closed:
				if e != nil {
					t.Fatal(e)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("Close failed to join")
			}
		})
	}
}

func TestCardPreparedViewDirectLifetimeAndForeignError(t *testing.T) {
	t.Run("lifetime_guard_precedes_async_context_callback", func(t *testing.T) {
		x, s, _ := seedFixture(t)
		x.now++
		// Exercise the final body with a still-live task context, deterministically
		// modeling the interval before context.AfterFunc propagates source closure.
		s.gate <- struct{}{}
		e := s.cardViewAuthorized(t.Context(), x.s, x.entry[0], x.session[0], []gymevent.Event{}, func(CardView, func() error) error {
			s.cancel()
			if t.Context().Err() != nil {
				t.Fatal("task context must remain live")
			}
			return nil
		})
		<-s.gate
		if !errors.Is(e, ErrOutcomeUnknown) || s.meta.ClockMS != x.now {
			t.Fatal("closed source concealed committed observation", e)
		}
	})
	t.Run("foreign_commit_error_is_opaque", func(t *testing.T) {
		x, s, _ := seedFixture(t)
		if e := s.WithCardView(t.Context(), x.s, x.entry[0], x.session[0], []string{}, func(CardView, func() error) error { return nil }); e != nil {
			t.Fatal(e)
		}
		foreignUnknown := errors.New("outcome_unknown")
		e := s.WithCardPreparedView(t.Context(), x.s, x.entry[0], x.session[0], func(Info) ([]string, func(CardView, func() error) error, error) {
			return []string{}, func(CardView, func() error) error { s.cancel(); return foreignUnknown }, nil
		})
		if e != foreignUnknown {
			t.Fatal("foreign uncertainty was reinterpreted", e)
		}
	})
}
