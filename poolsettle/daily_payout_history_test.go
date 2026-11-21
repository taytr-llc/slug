//go:build !slugtestpow && (linux || (darwin && cgo))

// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettle

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/internal/poolcapacity"
)

func TestDailyPayoutPageOriginalOwnerCaptureAndRestart(t *testing.T) {
	h := dailyIntegrationFixture(t)
	ctx := context.Background()
	store := h.settlement
	// Reading an empty journal is still a real capture, and cannot advance it.
	before := dailyRows(t, store)
	first, err := store.MinerDailyPayoutPage(ctx, h.sessions, "gym-a", DailyPayoutRequest{})
	if err != nil || first.Revision == 0 || first.HighBatch != 0 || first.BeforeBatch != 1 || !first.Done || first.NextBefore != 0 || len(first.Items) != 0 || first.ObservedAtMS != 0 {
		t.Fatal("empty original capture", first, err)
	}
	if !reflect.DeepEqual(before, dailyRows(t, store)) {
		t.Fatal("read changed empty journal")
	}
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
	backend := h.backend.Backend.(*settlementBackend)
	backend.loseAck = true
	if report, err := store.RunDaily(ctx); err != nil || report.Reason != "broadcast-unknown" {
		t.Fatal("original reserved payment", report, err)
	}
	before = dailyRows(t, store)
	page, err := store.MinerDailyPayoutPage(ctx, h.sessions, "gym-a", DailyPayoutRequest{})
	if err != nil || page.HighBatch != 1 || len(page.Items) != 1 || page.Items[0].BatchSequence != 1 || page.Items[0].Payment.RecordedState != "prepared" || page.Balance.ReservedAtoms != 990000000 || page.Balance.PaidAtoms != 0 {
		t.Fatal("reserved payment projection", page, err)
	}
	other, err := store.MinerDailyPayoutPage(ctx, h.sessions, "gym-b", DailyPayoutRequest{})
	if err != nil || other.HighBatch != page.HighBatch || other.Revision != page.Revision || len(other.Items) != 0 || other.Balance.GymID == page.Balance.GymID || other.RecipientScript == page.RecipientScript || other.Balance.ReservedAtoms != 0 {
		t.Fatal("other gym capture isolation", other, err)
	}
	if !reflect.DeepEqual(before, dailyRows(t, store)) {
		t.Fatal("history read changed original rows")
	}
	request := DailyPayoutRequest{page.Revision, page.HighBatch, page.BeforeBatch}
	// Another real report commit changes the captured revision even when the
	// immutable transaction stays the same and its delivery remains unknown.
	h.clock = h.clock.Add(time.Millisecond)
	if _, err = store.RecoverPending(ctx); err != nil {
		t.Fatal(err)
	}
	if got, err := store.MinerDailyPayoutPage(ctx, h.sessions, "gym-a", request); !errors.Is(err, ErrStaleCapture) || !reflect.DeepEqual(got, DailyPayoutPage{}) {
		t.Fatal("changed capture disclosed a page", got, err)
	}
	dailyIntegrationReopen(t, h)
	if _, err = store.MinerDailyPayoutPage(ctx, h.sessions, "gym-a", DailyPayoutRequest{}); !errors.Is(err, ErrClosed) {
		t.Fatal("closed original owner", err)
	}
	store = h.settlement
	reopened, err := store.MinerDailyPayoutPage(ctx, h.sessions, "gym-a", DailyPayoutRequest{})
	if err != nil || len(reopened.Items) != 1 || reopened.Items[0].Payment.TxID != page.Items[0].Payment.TxID || reopened.Balance.ReservedAtoms != page.Balance.ReservedAtoms {
		t.Fatal("reopened exact payment", reopened, err)
	}
	// A partial requested projection cannot hide an unrelated corrupt key.
	if err = store.db.Put([]byte("unrelated-corrupt-tail"), []byte("true"), &opt.WriteOptions{Sync: true}); err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{"gym-a", "gym-b"} {
		if got, err := store.MinerDailyPayoutPage(ctx, h.sessions, token, DailyPayoutRequest{}); err == nil || !reflect.DeepEqual(got, DailyPayoutPage{}) {
			t.Fatal("partial page hid full audit corruption", token, err)
		}
	}
}

type dailyPageRevokingSessions struct {
	key   string
	calls int
}

func (s *dailyPageRevokingSessions) WithSession(ctx context.Context, token string, fn func(string, func() error) error) error {
	s.calls++
	if s.calls > 1 {
		return errors.New("original session revoked before disclosure")
	}
	return fn(s.key, ctx.Err)
}

func TestDailyPayoutPageFinalSessionFence(t *testing.T) {
	h := dailyIntegrationFixture(t)
	sessions := &dailyPageRevokingSessions{key: h.sessions["gym-a"]}
	got, err := h.settlement.MinerDailyPayoutPage(context.Background(), sessions, "gym-a", DailyPayoutRequest{})
	if err == nil || sessions.calls < 2 || !reflect.DeepEqual(got, DailyPayoutPage{}) {
		t.Fatal("revoked original session disclosed history", got, err)
	}
}

func TestDailyPayoutPageOriginalSparseSlotTraversal(t *testing.T) {
	// The selected finite journal holds eighteen real controlled-observation
	// payments. Only the oldest belongs to A; no synthetic b/ rows or DTOs are
	// imported. This checks bounded global slots rather than eight matches.
	limits := &poolcapacity.DailyInputs{SourceMax: 64, SourceBytes: 64 << 20, Rounds: 64, RoundBytes: 64 << 20, CreditRounds: 64, Batches: 32, Gyms: 2}
	h := dailyIntegrationFixtureWithWorkLimit(t, limits, 32)
	ctx := context.Background()
	backend := h.backend.Backend.(*settlementBackend)
	backend.confirmations = 1
	for i := 0; i < 18; i++ {
		token := "gym-b"
		if i == 0 {
			token = "gym-a"
		}
		work, err := h.pool.Issue(ctx, token)
		if err != nil {
			t.Fatal(i, err)
		}
		winner, _ := lateWinnerSearch(t, h, work, false)
		h.node.canonical[winner.WorkHash] = true
		h.node.tip = work.Job.Context.ParentHeight + 61
		if _, err = h.pool.Submit(ctx, token, work.Job.ID, winner.Nonce); err != nil {
			t.Fatal(i, err)
		}
		if _, err = h.rounds.Sync(ctx); err != nil {
			t.Fatal(i, err)
		}
		if _, err = h.rounds.Refresh(ctx, winner.WorkHash); err != nil {
			t.Fatal(i, err)
		}
		if r, err := h.settlement.RunDaily(ctx); err != nil || r.State != "pending" {
			t.Fatal("daily payment", i, r.State, r.Reason, err)
		}
		if r, err := h.settlement.RecoverPending(ctx); err != nil || r.State != "complete" {
			t.Fatal("recorded confirmation", i, r.State, r.Reason, err)
		}
		h.clock = h.clock.Add(24 * time.Hour)
		h.node.template.Context.ParentHash = winner.WorkHash
		h.node.template.Context.ParentHeight = h.node.tip
		h.node.template.Block.PreviousHash = winner.WorkHash
		h.node.template.Block.Height = int64(h.node.tip) + 1
	}
	before := dailyRows(t, h.settlement)
	q := DailyPayoutRequest{}
	var paid int64
	for pageIndex := 0; pageIndex < 3; pageIndex++ {
		p, err := h.settlement.MinerDailyPayoutPage(ctx, h.sessions, "gym-a", q)
		if err != nil || p.HighBatch != 18 || p.BeforeBatch != uint64(19-8*pageIndex) || p.Balance.PaidAtoms != 990000000 {
			t.Fatal("original global slot capture", pageIndex, p, err)
		}
		if pageIndex < 2 {
			if p.Done || len(p.Items) != 0 || p.NextBefore != p.BeforeBatch-8 {
				t.Fatal("empty page treated as EOF or scanned for matches", p)
			}
		} else if !p.Done || p.NextBefore != 0 || len(p.Items) != 1 || p.Items[0].BatchSequence != 1 {
			t.Fatal("oldest exact payment missing", p)
		}
		for _, row := range p.Items {
			paid += row.Payment.Atoms
		}
		q = DailyPayoutRequest{p.Revision, p.HighBatch, p.NextBefore}
	}
	if paid != 990000000 || !reflect.DeepEqual(before, dailyRows(t, h.settlement)) {
		t.Fatal("traversal did not reconcile or changed journal", paid)
	}
}
