// Copyright (c) 2026 The SLUG developers. ISC license.
package poolround

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/prospectnode"
)

func TestRoundOrphanReturnConfirmationAndFundingConflict(t *testing.T) {
	h := newRoundHarness(t, false)
	ctx := context.Background()
	summary, err := h.store.Sync(ctx)
	if err != nil || summary != (Summary{Through: 3, Rounds: 1, PendingShares: 0}) {
		t.Fatal("wrong concrete round", summary, err)
	}
	initial, err := h.store.Get(ctx, h.winner.Hash)
	if err != nil || initial.Status() != "unobserved" || initial.Confirmation != nil || len(initial.ConfirmedEarnings()) != 0 || len(initial.ProvisionalEarnings()) != 0 {
		t.Fatal("new round manufactured earnings", err)
	}
	if initial.Round.FirstAccepted != 1 || initial.Round.LastAccepted != 3 || len(initial.Round.ShareKeys) != 3 || initial.Round.ShareKeys[2] != h.winner.Key || initial.Round.RewardAtoms != 1000000000 || len(initial.Round.Allocations) != 1 || initial.Round.Allocations[0] != (Allocation{GymID: h.input.Job.GymID, Shares: 3, Numerator: "1000000000", Denominator: "1"}) {
		t.Fatal("fixed source membership or exact reward incorrect", initial.Round)
	}
	provisional, err := h.store.Refresh(ctx, h.winner.Hash)
	if err != nil || provisional.Status() != "provisional" || provisional.Confirmation != nil || len(provisional.ConfirmedEarnings()) != 0 || !reflect.DeepEqual(provisional.ProvisionalEarnings(), initial.Round.Allocations) {
		t.Fatal("59 descendants crossed confirmation boundary", err)
	}
	h.observer.canonical = false
	h.observer.depth = 70
	orphan, err := h.store.Refresh(ctx, h.winner.Hash)
	if err != nil || orphan.Status() != "orphaned" || len(orphan.ProvisionalEarnings()) != 0 || len(orphan.ConfirmedEarnings()) != 0 || !reflect.DeepEqual(orphan.Round, initial.Round) {
		t.Fatal("orphan changed membership or retained unconfirmed earnings", err)
	}
	h.reopen(t)
	h.observer.canonical = true
	h.observer.depth = 59
	returned, err := h.store.Refresh(ctx, h.winner.Hash)
	if err != nil || returned.Status() != "provisional" || !reflect.DeepEqual(returned.Round, initial.Round) || returned.Confirmation != nil {
		t.Fatal("exact return changed round or confirmed early", err)
	}
	h.observer.depth = 60
	confirmed, err := h.store.Refresh(ctx, h.winner.Hash)
	if err != nil || confirmed.Status() != "confirmed" || confirmed.Confirmation == nil || confirmed.Confirmation.Descendants != 60 || !reflect.DeepEqual(confirmed.ConfirmedEarnings(), initial.Round.Allocations) || len(confirmed.ProvisionalEarnings()) != 0 {
		t.Fatal("60 descendants did not lock exact earnings", err)
	}
	firstConfirmation := *confirmed.Confirmation
	h.reopen(t)
	h.observer.canonical = false
	h.observer.depth = 80
	conflict, err := h.store.Refresh(ctx, h.winner.Hash)
	if err != nil || conflict.Status() != "funding-conflict" || conflict.Confirmation == nil || *conflict.Confirmation != firstConfirmation || !reflect.DeepEqual(conflict.Round, initial.Round) || !reflect.DeepEqual(conflict.ConfirmedEarnings(), initial.Round.Allocations) {
		t.Fatal("post-lock orphan erased or rewrote locked earnings", err)
	}
	h.reopen(t)
	h.observer.canonical = true
	h.observer.depth = 20
	again, err := h.store.Refresh(ctx, h.winner.Hash)
	if err != nil || again.Status() != "confirmed" || again.Confirmation == nil || *again.Confirmation != firstConfirmation {
		t.Fatal("return below original depth reset confirmed earnings", err)
	}
	rows, err := h.store.List(ctx, 0, 8)
	if err != nil || len(rows) != 1 || !reflect.DeepEqual(rows[0], again) {
		t.Fatal("recovery duplicated round", err)
	}
	rows[0].Round.ShareKeys[0] = "mutated"
	rows[0].Confirmation.TipHash = "mutated"
	copy, err := h.store.Get(ctx, h.winner.Hash)
	if err != nil || copy.Round.ShareKeys[0] == "mutated" || copy.Confirmation.TipHash == "mutated" {
		t.Fatal("returned data aliases durable round", err)
	}
}

func TestRoundLateWinnerAppendsAfterPendingIntent(t *testing.T) {
	h := newRoundHarness(t, true)
	ctx := context.Background()
	summary, err := h.store.Sync(ctx)
	if err != nil || summary != (Summary{Through: 2, Rounds: 0, PendingShares: 2}) {
		t.Fatal("unaccepted winning intent consumed ordinary shares", summary, err)
	}
	h.reopen(t)
	h.poolNode.canonical = true
	late, err := h.source.Submit(ctx, roundToken, h.input.Job.ID, h.winner.Nonce)
	if err != nil || late.Sequence != 1 || late.AcceptedSequence != 3 || late.Acceptance != "canonical-late-winner" {
		t.Fatal("fixture did not append late canonical acceptance", late, err)
	}
	summary, err = h.store.Sync(ctx)
	if err != nil || summary != (Summary{Through: 3, Rounds: 1, PendingShares: 0}) {
		t.Fatal("late winner did not close current window", summary, err)
	}
	r, err := h.store.Get(ctx, h.winner.Hash)
	if err != nil || r.Round.FirstAccepted != 1 || r.Round.LastAccepted != 3 || r.Round.Winner.Share.Sequence != 1 || r.Round.Winner.Share.AcceptedSequence != 3 || r.Round.ShareKeys[2] != h.winner.Key {
		t.Fatal("raw submission order became round boundary", err)
	}
	h.reopen(t)
	before := r.Round
	if _, err := h.source.Submit(ctx, roundToken, h.input.Job.ID, h.winner.Nonce); err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	r, err = h.store.Get(ctx, h.winner.Hash)
	if err != nil || !reflect.DeepEqual(r.Round, before) {
		t.Fatal("duplicate late winner rewrote round", err)
	}
}

func TestRoundObservationErrorsNeverInventOrphanOrConfirmation(t *testing.T) {
	h := newRoundHarness(t, false)
	ctx := context.Background()
	if _, err := h.store.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	before, err := h.store.Refresh(ctx, h.winner.Hash)
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"offline", "wrong-block", "wrong-height", "wrong-depth", "nil-context"} {
		t.Run(mode, func(t *testing.T) {
			h.observer.err = nil
			h.observer.change = nil
			h.observer.depth = 60
			callCtx := ctx
			switch mode {
			case "offline":
				h.observer.err = errors.New("untrusted-secret-node-error")
			case "wrong-block":
				h.observer.change = func(o *prospectnode.PoolBlockObservation) { o.BlockHash = strings.Repeat("1", 64) }
			case "wrong-height":
				h.observer.change = func(o *prospectnode.PoolBlockObservation) { o.BlockHeight++ }
			case "wrong-depth":
				h.observer.change = func(o *prospectnode.PoolBlockObservation) { o.Descendants = 59 }
			case "nil-context":
				callCtx = nil
			}
			if _, err := h.store.Refresh(callCtx, h.winner.Hash); err == nil || strings.Contains(err.Error(), "untrusted-secret") {
				t.Fatal("invalid observation accepted or leaked", err)
			}
			saved, err := h.store.Get(ctx, h.winner.Hash)
			if err != nil || !reflect.DeepEqual(saved, before) {
				t.Fatal("failed observation rewrote prior evidence", err)
			}
		})
	}
}
