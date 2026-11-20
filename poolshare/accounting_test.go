// Copyright (c) 2026 The SLUG developers. ISC license.
package poolshare

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/taytr-llc/slug/wire/v2"
)

func TestAccountingFixedTailAndImmutableDeliveryProjection(t *testing.T) {
	h := setupPool(t, "", 0)
	ctx := context.Background()
	if _, err := h.pool.Submit(ctx, poolTestToken, h.source.Job.ID, h.ordinary.Nonce); err != nil {
		t.Fatal(err)
	}
	first, err := h.pool.AccountingPage(ctx, 0, 0, 1)
	if err != nil || first.Through != 1 || len(first.Entries) != 1 || first.Config != h.config || first.Entries[0].Proof != nil || first.Entries[0].RewardAtoms != 0 {
		t.Fatal("wrong ordinary accounting page", err)
	}
	assertFrozenSubmission(t, first.Entries[0].Share, h.ordinary, h.source, h.config.PoolTarget)
	if _, err := h.pool.Submit(ctx, poolTestToken, h.source.Job.ID, h.winner.Nonce); err != nil {
		t.Fatal(err)
	}
	oldTail, err := h.pool.AccountingPage(ctx, 1, first.Through, 1)
	if err != nil || oldTail.Through != 1 || len(oldTail.Entries) != 0 {
		t.Fatal("new share leaked into a fixed accounting snapshot", err)
	}
	winner, err := h.pool.AccountingPage(ctx, 1, 0, 1)
	if err != nil || winner.Through != 2 || len(winner.Entries) != 1 {
		t.Fatal("missing accepted winner", err)
	}
	entry := winner.Entries[0]
	if entry.Share.AcceptedSequence != 2 || entry.Share.BlockState != "" || entry.Share.BlockObservedAtMS != 0 || entry.RewardAtoms != h.winner.Candidate.CoinbaseValue || !reflect.DeepEqual(entry.Proof, &h.winner.Candidate.Proof) {
		t.Fatal("accounting entry lost exact reward/proof or retained mutable delivery status")
	}
	h.node.canonicalErr = nil
	h.now++
	if _, err := h.pool.Submit(ctx, poolTestToken, h.source.Job.ID, h.winner.Nonce); err != nil {
		t.Fatal(err)
	}
	after, err := h.pool.AccountingPage(ctx, 1, 2, 1)
	if err != nil || !reflect.DeepEqual(after, winner) {
		t.Fatal("delivery observation changed immutable accounting evidence", err)
	}
	entry.Proof.Header = "mutated"
	h.reopen(t)
	reopened, err := h.pool.AccountingPage(ctx, 1, 2, 1)
	if err != nil || reopened.Entries[0].Proof.Header != h.winner.Header {
		t.Fatal("accounting result aliases retained evidence", err)
	}
}

func TestAccountingOmitsPendingAndRefusesInvalidAccess(t *testing.T) {
	if wire.TestPoW {
		if _, err := new(Pool).AccountingPage(context.Background(), 0, 0, 1); !errors.Is(err, ErrProfile) {
			t.Fatal("cheap accounting profile accepted", err)
		}
		return
	}
	h := setupPool(t, "", 0)
	h.now = h.source.Job.ExpiresAtMS
	if _, err := h.pool.Submit(context.Background(), poolTestToken, h.source.Job.ID, h.winner.Nonce); err != nil {
		t.Fatal(err)
	}
	page, err := h.pool.AccountingPage(context.Background(), 0, 0, 8)
	if err != nil || page.Through != 0 || len(page.Entries) != 0 {
		t.Fatal("unaccepted block intent became round evidence", err)
	}
	for _, limit := range []int{0, 9} {
		if _, err := h.pool.AccountingPage(context.Background(), 0, 0, limit); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid page bound accepted", err)
		}
	}
	if _, err := h.pool.AccountingPage(context.Background(), 0, 1, 1); !errors.Is(err, ErrInvalid) {
		t.Fatal("future acceptance tail accepted", err)
	}
	_ = h.pool.Close()
	if _, err := h.pool.AccountingPage(context.Background(), 0, 0, 1); !errors.Is(err, ErrClosed) {
		t.Fatal("closed accounting source served evidence", err)
	}
}
