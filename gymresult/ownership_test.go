//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymresult

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymcard"
	"strings"
	"testing"
)

// Concrete source stores and the explicitly modeled fixture lease exercise
// owner shutdown during lower authorization without claiming real account auth.
func TestResultOwnerCloseDuringLowerAuthorization(t *testing.T) {
	for _, owner := range []string{"result", "card"} {
		t.Run(owner, func(t *testing.T) {
			f := newResultFixture(t)
			session := f.session
			closed := make(chan error, 1)
			entered, callbacks := 0, 0
			session.Lease = func(ctx context.Context, _ gymaccount.Capability, _ func(gymaccount.Account, func() error) error) error {
				entered++
				go func() {
					if owner == "result" {
						closed <- f.s.Close()
					} else {
						closed <- f.card.Close()
					}
				}()
				<-ctx.Done() // the concrete owner's lifetime reaches the lower traversal
				return ctx.Err()
			}
			var got, want error
			if owner == "result" {
				call := f.call()
				call.session = session
				c, done, e := f.s.call(call)
				if e != nil {
					t.Fatal(e)
				}
				r, st, e := f.s.read(c, gymaccount.View, false)
				done()
				got, want = e, ErrClosed
				if r.ResultKey != "" || st.ResultKey != "" {
					t.Fatal("closed result disclosed payload")
				}
			} else {
				got = f.card.WithResultPreparedView(context.Background(), f.seed, f.event, f.entry, session, gymaccount.View, f.record.Source.Event.Saturday, func(gymcard.Info, []gymcard.BoutSource) (func(gymcard.BoutView, func() error) error, error) {
					callbacks++
					return func(gymcard.BoutView, func() error) error { callbacks++; return nil }, nil
				})
				want = gymcard.ErrClosed
			}
			if entered != 1 {
				t.Fatalf("unexpected lower admissions %d: %v", entered, got)
			}
			if e := <-closed; e != nil {
				t.Fatal(e)
			}
			if !errors.Is(got, want) || callbacks != 0 {
				t.Fatalf("owner=%s error=%v want=%v callbacks=%d", owner, got, want, callbacks)
			}
		})
	}
}

func TestResultReturnedConflictDoesNotAliasFundingMetadata(t *testing.T) {
	f, c, done, r := preparedFixture(t)
	defer done()
	m := modelTip(r)
	m.hashes[r.Statement.Statement.Source.Source.TargetHeight] = strings.Repeat("e", 64)
	got, e := f.s.reconcile(c, m)
	if e != nil || len(got.Record.Conflicts) != 1 || got.Record.Conflicts[0].ObservedTip == nil {
		t.Fatal("conflict transition", e)
	}
	raw, e := f.s.db.Get([]byte("meta"), nil)
	if e != nil {
		t.Fatal(e)
	}
	var durable metadata
	if json.Unmarshal(raw, &durable) != nil || durable.LastFundingTip == nil {
		t.Fatal("durable tip missing")
	}
	if got.Record.Conflicts[0].ObservedTip == f.s.meta.LastFundingTip {
		t.Fatal("returned conflict aliases retained metadata")
	}
	got.Record.Conflicts[0].ObservedTip.Hash = strings.Repeat("f", 64)
	got.Record.Conflicts[0].ObservedTip.Height = 0
	if !equal(f.s.meta, durable) {
		t.Fatal("returned record changed retained metadata")
	}
	again, e := f.s.db.Get([]byte("meta"), nil)
	if e != nil || string(again) != string(raw) {
		t.Fatal("returned mutation changed durable metadata", e)
	}
	row, exists, e := f.s.record(r.ResultKey)
	if e != nil || !exists || row.Conflicts[0].ObservedTip.Hash != durable.LastFundingTip.Hash {
		t.Fatal("returned mutation changed retained row", e)
	}
	auditCurrent(t, f)
}
