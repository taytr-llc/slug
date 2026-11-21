// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettle

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/prospectwork"
)

// Extend the existing published-key/normal-PoW fixture, not an injected
// settlement source. Each one-share round has an independent 1e9-atom reward,
// hence exactly 990e6 miner atoms and 10e6 operator atoms under unchanged D58.
// The controlled node supplies confirmation evidence; this is not node or
// real-expiry acceptance, which remains the separate actual harness.
func streamingAddRound(t *testing.T, h *settlementHarness, sequence uint64) string {
	t.Helper()
	ctx := context.Background()
	h.node.template.Context.ParentHash = h.node.winner.WorkHash
	h.node.template.Context.ParentHeight++
	h.node.template.Block.PreviousHash = h.node.winner.WorkHash
	h.node.template.Block.Height++
	work, err := h.pool.Issue(ctx, settlementTestToken)
	if err != nil {
		t.Fatal(err)
	}
	key, _, payout := settlementFixtureKey(3)
	key.Zero()
	prepared, err := prospectwork.PrepareManifestWork(work.Job, h.node.template, payout, 0, strings.Repeat("f", 64))
	if err != nil {
		t.Fatal(err)
	}
	h.node.winner = prospectwork.ShareCheck{}
	for nonce := uint32(0); nonce < 64; nonce++ {
		check, err := prepared.CheckNonce(ctx, nonce)
		if err != nil {
			t.Fatal(err)
		}
		if check.Block {
			h.node.winner = check
			break
		}
	}
	if h.node.winner.Candidate == nil {
		t.Fatal("bounded normal-PoW fixture search exhausted")
	}
	receipt, err := h.pool.Submit(ctx, settlementTestToken, work.Job.ID, h.node.winner.Nonce)
	if err != nil || receipt.AcceptedSequence != sequence {
		t.Fatalf("accepted sequence %d: %+v %v", sequence, receipt, err)
	}
	if _, err = h.source.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	round, err := h.source.Refresh(ctx, h.node.winner.WorkHash)
	if err != nil || round.Confirmation == nil {
		t.Fatalf("confirmed source round: %+v %v", round, err)
	}
	return h.node.winner.WorkHash
}

func streamingCredits(t *testing.T, h *settlementHarness, credits []Credit, hashes []string) {
	t.Helper()
	if len(credits) != len(hashes) {
		t.Fatalf("credits=%d, want %d", len(credits), len(hashes))
	}
	outpoints := make(map[string]bool)
	for i, credit := range credits {
		coin := credit.Coin
		if credit.RoundHash != hashes[i] || coin.BlockHash != hashes[i] || coin.Index != 0 || !coin.Coinbase || coin.Atoms != 1000000000 || coin.Script != h.config.ChangeScript || outpoints[coin.TxID] {
			t.Fatalf("wrong or duplicated source credit %d: %+v", i, credit)
		}
		outpoints[coin.TxID] = true
		if len(coin.Claims) != 2 {
			t.Fatal("one-share round must retain miner and operator claims")
		}
		claims := make(map[string]Fraction)
		for _, claim := range coin.Claims {
			if claim.RoundHash != hashes[i] || claim.RoundHeight != coin.BlockHeight {
				t.Fatal("claim lost its original round")
			}
			if _, duplicate := claims[claim.GymID]; duplicate {
				t.Fatal("duplicate gym claim")
			}
			claims[claim.GymID] = claim.Amount
		}
		if claims[h.gym] != (Fraction{Numerator: "990000000", Denominator: "1"}) || claims[""] != (Fraction{Numerator: "10000000", Denominator: "1"}) {
			t.Fatal("credit changed the independently specified 99/1 allocation", claims)
		}
	}
}

func streamingBatch(t *testing.T, got, original Batch, state string) {
	t.Helper()
	if got.State != state {
		t.Fatalf("batch state %q, want %q", got.State, state)
	}
	got.State = original.State
	if !reflect.DeepEqual(got, original) {
		t.Fatal("original signed batch changed beyond its delivery observation")
	}
}

func TestSettlementStreamingPendingRecoveryAndCompleteImport(t *testing.T) {
	h := newSettlementHarness(t)
	ctx := context.Background()
	h.backend.loseAck = true
	if r, err := h.store.RunDaily(ctx); err != nil || r.Reason != "broadcast-unknown" {
		t.Fatal("fixture did not retain original pending payment", err)
	}
	if len(h.store.state.Batches) != 1 || len(h.store.state.Credits) != 1 {
		t.Fatal("fixture payment/credit cardinality")
	}
	original := clone(h.store.state.Batches[0])
	hashes := []string{h.node.winner.WorkHash}
	hashes = append(hashes, streamingAddRound(t, h, 2))
	hashes = append(hashes, streamingAddRound(t, h, 3))
	h.reopen(t)
	watch := recoveryWatch(h)
	if r, err := h.store.RecoverPending(ctx); err != nil || r.State != "pending" || r.Reason != "payout-pending" {
		t.Fatal("original lost acknowledgment did not recover", err)
	}
	if len(h.store.state.Credits) != 1 || len(h.store.state.Batches) != 1 || watch.inspects != 0 || watch.quotes != 0 || watch.signs != 0 || watch.broadcasts != 0 {
		t.Fatal("pending recovery imported, selected or signed new funding")
	}
	streamingBatch(t, h.store.state.Batches[0], original, "broadcast")

	// The existing daily slot remains occupied. Confirmation permits importing
	// both new rounds, but cannot authorize another payment on the same day.
	h.backend.confirmations = 1
	r, err := h.store.RunDaily(ctx)
	if err != nil || r.State != "complete" || r.Reason != "daily-batch-complete" {
		t.Fatal("same-day import did not preserve daily slot", err)
	}
	streamingCredits(t, h, h.store.state.Credits, hashes)
	if len(h.store.state.Batches) != 1 || watch.quotes != 0 || watch.signs != 0 || watch.broadcasts != 0 || h.backend.signCalls != 1 || h.backend.broadcasts != 1 {
		t.Fatal("new confirmations caused a duplicate debit or payment")
	}
	streamingBatch(t, h.store.state.Batches[0], original, "confirmed")
	balance := settlementBalance(t, r, h.gym)
	if balance.Confirmed != (Fraction{Numerator: "1980000000", Denominator: "1"}) || balance.ReservedAtoms != 0 || balance.PaidAtoms != 990000000 {
		t.Fatal("streamed import lost or duplicated earnings", balance)
	}
	credits := clone(h.store.state.Credits)
	h.reopen(t)
	watch = recoveryWatch(h)
	if _, err = h.store.RunDaily(ctx); err != nil || !reflect.DeepEqual(credits, h.store.state.Credits) || len(h.store.state.Batches) != 1 || watch.quotes != 0 || watch.signs != 0 || watch.broadcasts != 0 {
		t.Fatal("reopen/retry duplicated import or payment", err)
	}
	streamingBatch(t, h.store.state.Batches[0], original, "confirmed")
}

func TestSettlementStreamingLateRefusalLeavesCandidateUnchanged(t *testing.T) {
	h := newSettlementHarness(t)
	ctx := context.Background()
	hashes := []string{h.node.winner.WorkHash}
	hashes = append(hashes, streamingAddRound(t, h, 2))
	hashes = append(hashes, streamingAddRound(t, h, 3))
	// Capture authentic credits only as fault-injection inputs. The expected
	// outcome is independent: a refusal must preserve every supplied byte.
	authentic := clone(h.store.state)
	if err := h.store.sourceState(ctx, &authentic, true); err != nil {
		t.Fatal(err)
	}
	streamingCredits(t, h, authentic.Credits, hashes)
	durable := append([]byte(nil), recoveryDisk(t, h.store)...)
	ownerState := clone(h.store.state)
	for _, name := range []string{"late-old-digest", "missing-old-round", "source-binding", "credit-capacity"} {
		t.Run(name, func(t *testing.T) {
			candidate := clone(ownerState)
			wantErr := ErrSource
			switch name {
			case "late-old-digest":
				candidate.Credits = []Credit{clone(authentic.Credits[2])}
				if candidate.Credits[0].RoundDigest == strings.Repeat("0", 64) {
					t.Fatal("fault value unexpectedly matches original digest")
				}
				candidate.Credits[0].RoundDigest = strings.Repeat("0", 64)
			case "missing-old-round":
				candidate.Credits = []Credit{clone(authentic.Credits[2])}
				for _, hash := range hashes {
					if hash == strings.Repeat("0", 64) {
						t.Fatal("fault hash unexpectedly names an original round")
					}
				}
				candidate.Credits[0].RoundHash = strings.Repeat("0", 64)
			case "source-binding":
				candidate.Source.MaxBytes++
			case "credit-capacity":
				candidate.Config.MaxRounds = 2
				wantErr = ErrCapacity
			}
			before := clone(candidate)
			if err := h.store.sourceState(ctx, &candidate, true); !errors.Is(err, wantErr) {
				t.Fatalf("late refusal: %v, want %v", err, wantErr)
			}
			if !reflect.DeepEqual(candidate, before) || !reflect.DeepEqual(h.store.state, ownerState) || !bytes.Equal(durable, recoveryDisk(t, h.store)) {
				t.Fatal("late refusal published a partial import")
			}
		})
	}
}

func TestSettlementStreamingCanceledFundingLeavesNoImport(t *testing.T) {
	h := newSettlementHarness(t)
	hashes := []string{h.node.winner.WorkHash}
	hashes = append(hashes, streamingAddRound(t, h, 2))
	hashes = append(hashes, streamingAddRound(t, h, 3))
	before, durable := clone(h.store.state), append([]byte(nil), recoveryDisk(t, h.store)...)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	watch := recoveryWatch(h)
	watch.inspect = func(_ context.Context, coins []Coin) ([]Funding, error) {
		// This boundary is reached only after all three source credits have
		// been derived. Cancel before the financial state can be committed.
		if len(coins) != 3 {
			t.Fatal("funding inspection omitted staged source credits")
		}
		seen := make(map[string]bool)
		for _, coin := range coins {
			seen[coin.BlockHash] = true
		}
		for _, hash := range hashes {
			if !seen[hash] {
				t.Fatal("funding inspection changed original source")
			}
		}
		cancel()
		return nil, context.Canceled
	}
	if _, err := h.store.RunDaily(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal("late original cancellation was not preserved", err)
	}
	if !reflect.DeepEqual(before, h.store.state) || !bytes.Equal(durable, recoveryDisk(t, h.store)) || watch.inspects != 1 || watch.observes != 0 || watch.quotes != 0 || watch.signs != 0 || watch.broadcasts != 0 {
		t.Fatal("cancellation retained a partial import or payment")
	}
	watch.inspect = nil
	h.backend.funding = "immature"
	if _, err := h.store.RunDaily(context.Background()); err != nil {
		t.Fatal(err)
	}
	streamingCredits(t, h, h.store.state.Credits, hashes)
	if len(h.store.state.Batches) != 0 || watch.quotes != 0 || watch.signs != 0 || watch.broadcasts != 0 {
		t.Fatal("fresh retry treated immature imports as payable")
	}
	credits := clone(h.store.state.Credits)
	h.reopen(t)
	if _, err := h.store.RunDaily(context.Background()); err != nil || !reflect.DeepEqual(credits, h.store.state.Credits) || len(h.store.state.Batches) != 0 {
		t.Fatal("reopened retry duplicated imported earnings", err)
	}
}
