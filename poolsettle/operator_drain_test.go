// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettle

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/taytr-llc/slug/poolshare"
)

// Two independently specified root allocations leave B below minimum on each
// change coin. Their one merge reaches B's minimum, attaining 2R-1 batches for
// R=2 without a new reward. This protects the lineage premise rather than a
// particular transaction encoding (existing transaction references cover that).
func TestSettlementCapacityRootAndChangeOnlyLineage(t *testing.T) {
	root := func(hash string) Coin {
		return modelCoin(hash, 300000000, []Claim{{Amount: Fraction{"3000000", "1"}}, {GymID: modelGymA, Amount: Fraction{"198000000", "1"}}, {GymID: modelGymB, Amount: Fraction{"99000000", "1"}}})
	}
	a, b := root(strings.Repeat("aa", 32)), root(strings.Repeat("bb", 32))
	first := modelBatch(t, []Coin{a}, "2026-11-18")
	second := modelBatch(t, []Coin{b}, "2026-11-19")
	for _, batch := range []Batch{first, second} {
		if batch.Change == nil || len(batch.Payments) != 1 || batch.Payments[0].GymID != modelGymA {
			t.Fatal("root boundary fixture changed")
		}
		payments, change, reason, err := plan([]Coin{*batch.Change}, modelConfig(), 1000)
		if err != nil || reason != "below-minimum" || len(payments) != 0 || change != nil {
			t.Fatal("one residual change generated another payment", reason, err)
		}
	}
	// liveCoins prepends each new change: after the two independent root
	// batches, its original input order is second.Change, then first.Change.
	merged := modelBatch(t, []Coin{*second.Change, *first.Change}, "2026-11-20")
	if len(merged.Payments) != 1 || merged.Payments[0].GymID != modelGymB || merged.Payments[0].Atoms != 198000000 {
		t.Fatal("change merge did not preserve exact carry")
	}
	history := []Batch{first, second, merged}
	state := state{Config: modelConfig(), Credits: []Credit{{a.BlockHash, strings.Repeat("01", 32), a}, {b.BlockHash, strings.Repeat("02", 32), b}}, Batches: history, LastClockMS: merged.CreatedAtMS}
	live, err := liveCoins(state)
	if err != nil || len(live) != 1 {
		t.Fatal("merged lineage was not unique", err)
	}
	// Preserve the failed ordering as a negative control: valid signed bytes
	// cannot authorize a reordered subsequence of the original live coins.
	reversed := modelBatch(t, []Coin{*first.Change, *second.Change}, "2026-11-20")
	badOrder := state
	badOrder.Batches = []Batch{first, second, reversed}
	if _, err = liveCoins(badOrder); err == nil {
		t.Fatal("reversed live-input order was accepted")
	}
	inputs, copies := 0, 0
	for _, batch := range history {
		inputs += len(batch.Inputs)
		for _, coin := range batch.Inputs {
			copies += len(coin.Claims)
		}
		if batch.Change != nil {
			copies += len(batch.Change.Claims)
		}
	}
	if len(history) != 2*2-1 || inputs > 2+len(history) || copies > 2*6*2 {
		t.Fatal("conservative lineage inequality violated", inputs, copies)
	}
	// Reusing any original input is conflict/corruption, never another branch
	// that can enlarge the reserved payment history.
	state.Batches = append(state.Batches, first)
	if _, err = liveCoins(state); err == nil {
		t.Fatal("reused root created a second lineage")
	}
}

// This uses the existing actual normal-PoW source and published synthetic
// native84 transaction fixture. No receipt DTO or fabricated credit is imported.
func TestSettlementOriginalPoolDrainKeepsFundingAndExactRecovery(t *testing.T) {
	h := newSettlementHarness(t)
	ctx := context.Background()
	page, err := h.pool.AccountingPage(ctx, 0, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	intake, err := h.pool.AttachOperatorAdmission(ctx, page.Config, 1)
	if err != nil {
		t.Fatal(err)
	}
	status, err := intake.Status(ctx)
	if err != nil || status.Mode != "draining" || status.OverBound {
		t.Fatal(status, err)
	}
	if _, err = h.pool.Issue(ctx, settlementTestToken); err != poolshare.ErrCapacity {
		t.Fatal("drain issued new work", err)
	}
	if _, err = h.source.Refresh(ctx, h.node.winner.WorkHash); err != nil {
		t.Fatal("drain closed retained round observation", err)
	}
	h.backend.funding = "immature"
	r, err := h.store.RunDaily(ctx)
	if err != nil || r.State != "deferred" || h.backend.signCalls != 0 {
		t.Fatal("immature drain changed readiness", r, err)
	}
	h.clock = h.clock.Add(24 * time.Hour)
	h.backend.funding = "ready"
	h.backend.loseAck = true
	r, err = h.store.RunDaily(ctx)
	if err != nil || h.backend.signCalls != 1 || len(h.store.state.Batches) != 1 || h.backend.broadcasts != 1 {
		t.Fatal("drain canceled due payment", r, err)
	}
	batch := h.store.state.Batches[0]
	first := append([]byte(nil), h.backend.broadcastBytes[0]...)
	// Exact observed-unknown loss forces rebroadcast after reopen; no new fee,
	// signature, inputs or payment identity may replace this retained batch.
	delete(h.backend.txs, batch.TxID)
	h.reopen(t)
	if _, err = h.store.RecoverPending(ctx); err != nil {
		t.Fatal(err)
	}
	if h.backend.signCalls != 1 || h.backend.broadcasts != 2 || len(h.store.state.Batches) != 1 || !bytes.Equal(first, h.backend.broadcastBytes[1]) || h.store.state.Batches[0].TxID != batch.TxID {
		t.Fatal("drain recovery replaced signed payment")
	}
	h.backend.confirmations = 1
	if _, err = h.store.RecoverPending(ctx); err != nil {
		t.Fatal(err)
	}
	if h.store.state.Batches[0].State != "confirmed" {
		t.Fatal("drain lost confirmation")
	}
	if status, err = intake.Status(ctx); err != nil || status.Mode != "draining" {
		t.Fatal("financial completion reopened intake", status, err)
	}
}
