//go:build !slugtestpow && (linux || (darwin && cgo))

// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettle

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb/opt"
)

// This counts calls to the original controlled fixture backend. No replacement
// source, signing result or ledger state is supplied to production.
type dailyRefreshBackend struct {
	Backend
	calls int
}

func (b *dailyRefreshBackend) Inspect(ctx context.Context, c []Coin) ([]Funding, error) {
	b.calls++
	return b.Backend.Inspect(ctx, c)
}
func (b *dailyRefreshBackend) EstimateFee(ctx context.Context, c []Coin, p []Payment, script string) (int64, error) {
	b.calls++
	return b.Backend.EstimateFee(ctx, c, p, script)
}
func (b *dailyRefreshBackend) Sign(ctx context.Context, c []Coin, raw []byte) ([]byte, error) {
	b.calls++
	return b.Backend.Sign(ctx, c, raw)
}
func (b *dailyRefreshBackend) ObserveTransaction(ctx context.Context, id string) (TransactionObservation, error) {
	b.calls++
	return b.Backend.ObserveTransaction(ctx, id)
}
func (b *dailyRefreshBackend) Broadcast(ctx context.Context, raw []byte) (string, error) {
	b.calls++
	return b.Backend.Broadcast(ctx, raw)
}

func TestDailyCompletedRefreshGuardsBeforeEffects(t *testing.T) {
	h := dailyIntegrationFixture(t)
	ctx := context.Background()
	s := h.settlement
	backend := &dailyRefreshBackend{Backend: h.backend}
	s.backend = backend
	day := h.clock.UTC().Format("2006-01-02")
	refuse := func(label, expected string, callCtx context.Context, want error) {
		t.Helper()
		before, calls := dailyRows(t, s), backend.calls
		report, err := s.RefreshCompletedDaily(callCtx, expected)
		if !errors.Is(err, want) || !reflect.DeepEqual(report, Report{}) {
			t.Fatal(label, report, err)
		}
		if backend.calls != calls || !reflect.DeepEqual(before, dailyRows(t, s)) {
			t.Fatal(label, "refusal changed journal or called backend")
		}
	}
	refuse("empty original owner", day, ctx, ErrInvalid)
	for _, invalid := range []string{"", "2030-1-01", "2030-02-30", day + "\n"} {
		refuse("invalid admitted day", invalid, ctx, ErrInvalid)
	}
	refuse("nil context", day, nil, ErrInvalid)
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	refuse("cancelled", day, cancelled, context.Canceled)
	work, err := h.pool.Issue(ctx, "gym-a")
	if err != nil {
		t.Fatal(err)
	}
	winner, _ := lateWinnerSearch(t, h, work, false)
	h.node.canonical[winner.WorkHash] = true
	h.node.tip = work.Job.Context.ParentHeight + 61
	if _, err = h.pool.Submit(ctx, "gym-a", work.Job.ID, winner.Nonce); err != nil {
		t.Fatal(err)
	}
	if _, err = h.rounds.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = h.rounds.Refresh(ctx, winner.WorkHash); err != nil {
		t.Fatal(err)
	}
	base := h.backend.Backend.(*settlementBackend)
	base.loseAck = true
	if report, err := s.RunDaily(ctx); err != nil || report.State != "pending" || report.Reason != "broadcast-unknown" {
		t.Fatal("original reserved A", report, err)
	}
	original := dailyIntegrationRaw(t, s, dailyNumber("b/", 1))
	refuse("pending A is not a completed slot", day, ctx, ErrInvalid)
	base.confirmations = 1
	if report, err := s.RecoverPending(ctx); err != nil || report.State != "complete" || report.Reason != "" {
		t.Fatal("original A recovery", report, err)
	}
	refuse("expired prior admitted day", h.clock.Add(-24*time.Hour).Format("2006-01-02"), ctx, ErrDailyRefreshExpired)
	refuse("future admitted day", h.clock.Add(24*time.Hour).Format("2006-01-02"), ctx, ErrDailyRefreshExpired)
	savedClock := h.clock
	h.clock = h.clock.Add(-time.Millisecond)
	refuse("original clock rollback", day, ctx, ErrClock)
	h.clock = savedClock.Add(24 * time.Hour)
	refuse("old completion cannot authorize new day", h.clock.Format("2006-01-02"), ctx, ErrInvalid)
	refuse("midnight expiry cannot authorize new day", day, ctx, ErrDailyRefreshExpired)
	h.clock = savedClock
	if report, err := s.RefreshCompletedDaily(ctx, day); err != nil || report.Reason != "daily-batch-complete" || report.State != "complete" {
		t.Fatal("same original completed slot", report, err)
	}
	if !reflect.DeepEqual(original, dailyIntegrationRaw(t, s, dailyNumber("b/", 1))) || base.signCalls != 1 || base.broadcasts != 1 || h.backend.quotes != 1 {
		t.Fatal("refresh changed A or performed another payment")
	}
	// Even an expired request must not mask unrelated namespace corruption.
	// Restore only this test's injected key before testing original close/open.
	if err = s.db.Put([]byte("unexpected-refresh-tail"), []byte("true"), &opt.WriteOptions{Sync: true}); err != nil {
		t.Fatal(err)
	}
	refuse("full integrity precedes expiry", h.clock.Add(-24*time.Hour).Format("2006-01-02"), ctx, ErrCorrupt)
	if err = s.db.Delete([]byte("unexpected-refresh-tail"), &opt.WriteOptions{Sync: true}); err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	if report, err := s.RefreshCompletedDaily(ctx, day); !errors.Is(err, ErrClosed) || !reflect.DeepEqual(report, Report{}) {
		t.Fatal("closed original", report, err)
	}
	audit, err := OpenDailyAudit(ctx, filepath.Join(h.root, "settlement"), h.settleConfig, h.rounds)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := audit.Close(); err != nil {
			t.Error(err)
		}
	}()
	before := dailyRows(t, audit)
	if report, err := audit.RefreshCompletedDaily(ctx, day); !errors.Is(err, ErrInvalid) || !reflect.DeepEqual(report, Report{}) || !reflect.DeepEqual(before, dailyRows(t, audit)) {
		t.Fatal("read-only owner admitted", report, err)
	}
}

func TestDailyCompletedRefreshRefusesOriginalV1(t *testing.T) {
	h := newSettlementHarness(t)
	backend := &dailyRefreshBackend{Backend: h.store.backend}
	h.store.backend = backend
	before := clone(h.store.state)
	if report, err := h.store.RefreshCompletedDaily(context.Background(), time.Now().UTC().Format("2006-01-02")); !errors.Is(err, ErrInvalid) || !reflect.DeepEqual(report, Report{}) {
		t.Fatal(report, err)
	}
	if !reflect.DeepEqual(before, h.store.state) || backend.calls != 0 || h.backend.signCalls != 0 || h.backend.broadcasts != 0 {
		t.Fatal("daily refresh affected V1 owner")
	}
}
