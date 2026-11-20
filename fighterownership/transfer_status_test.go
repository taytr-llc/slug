//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterownership

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestOwnershipTransferStatusCanonicalCompetition(t *testing.T) {
	// Conditional post-authentication classification, not a normal-work claim.
	b := &replayBranch{winners: map[string]string{}, transfers: map[string]TransferStatus{}, effective: map[string]EffectiveState{}}
	prior, winner, loser := strings.Repeat("11", 32), strings.Repeat("22", 32), strings.Repeat("33", 32)
	if !b.recordReveal(prior, winner, true) || b.recordReveal(prior, loser, true) || !b.recordReveal(prior, winner, true) {
		t.Fatal("first valid semantic transfer did not win")
	}
	statuses, e := completedTransferStatuses(context.Background(), b, nil)
	if e != nil || statuses[winner].State != "pending" || statuses[loser].State != "void" {
		t.Fatal("canonical competitor statuses", statuses, e)
	}
	copy := cloneBranch(b)
	copy.recordReveal(prior, strings.Repeat("44", 32), true)
	if len(b.transfers) != 2 {
		t.Fatal("uncommitted block changed original dispositions")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, e = completedTransferStatuses(ctx, b, nil); !errors.Is(e, context.Canceled) {
		t.Fatal("classification ignored cancellation", e)
	}
}

func TestOwnershipTransferStatusFinalizedLineageAndOrphans(t *testing.T) {
	corpus, _ := coreReference(t)
	var initial, final, reacquired EffectiveState
	for _, v := range corpus.Positives {
		if v.Family != "state" {
			continue
		}
		if v.Name == "v1/zero256-unawarded/initial" {
			json.Unmarshal(v.Value, &initial)
		}
		if v.Name == "v1/zero256-unawarded/final" {
			json.Unmarshal(v.Value, &final)
		}
		if v.Name == "v1/zero256-unawarded/reacquired" {
			json.Unmarshal(v.Value, &reacquired)
		}
	}
	if initial.StateID == "" || final.Generation != 1 {
		t.Fatal("independent lineage fixture absent")
	}
	var ev FinalStateEvidence
	json.Unmarshal(final.Evidence, &ev)
	loser := strings.Repeat("33", 32)
	b := &replayBranch{winners: map[string]string{}, transfers: map[string]TransferStatus{}, effective: map[string]EffectiveState{final.ProspectID: final}}
	// Replaying with an official final state sees the old predecessor consumed.
	// The official lineage restores only the winner's finalized status.
	b.recordReveal(initial.StateID, ev.TransferID, false)
	b.recordReveal(initial.StateID, loser, false)
	states := map[string]EffectiveState{initial.StateID: initial, final.StateID: final}
	statuses, e := completedTransferStatuses(context.Background(), b, states)
	if e != nil || statuses[ev.TransferID].State != "finalized" || statuses[loser].State != "void" {
		t.Fatal("finalized competition", statuses, e)
	}
	life, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := &Store{life: life, ready: true, transfers: statuses, states: states, portable: map[string]bool{final.StateID: true}, head: HeadRef{ConflictID: strings.Repeat("55", 32)}}
	got, e := s.TransferStatus(context.Background(), ev.TransferID)
	if e != nil || got.State != "finalized" || got.EffectiveState.StateID != final.StateID || got.ConflictID != s.head.ConflictID || got.PublicEvidencePending {
		t.Fatal("official state lost after conflict", got, e)
	}
	got.EffectiveState.Evidence[0] = '!'
	if statuses[ev.TransferID].EffectiveState.Evidence[0] == '!' {
		t.Fatal("status did not own its evidence")
	}
	// A later finalized successor does not erase the earlier finalized transfer.
	if reacquired.Generation != 2 {
		t.Fatal("independent successor absent")
	}
	states[reacquired.StateID] = reacquired
	b.effective[final.ProspectID] = reacquired
	later, e := completedTransferStatuses(context.Background(), b, states)
	if e != nil || later[ev.TransferID].State != "finalized" || later[ev.TransferID].EffectiveState.StateID != final.StateID {
		t.Fatal("finalized historical ancestor lost", e)
	}
	b.effective[final.ProspectID] = final
	// Stored evidence alone must not promote a discarded branch or a refund.
	orphan, refund := strings.Repeat("66", 32), strings.Repeat("77", 32)
	s.witnesses = map[string]CarrierWitness{"orphan": {TransferID: orphan, Kind: "reveal"}, "refund": {TransferID: refund, Kind: "refund"}}
	orphanState := final
	orphanEv := ev
	orphanEv.TransferID = orphan
	orphanState.StateID = strings.Repeat("88", 32)
	orphanState.Evidence, _ = json.Marshal(orphanEv)
	s.states[orphanState.StateID] = orphanState
	for _, id := range []string{orphan, refund, strings.Repeat("99", 32)} {
		if _, e = s.TransferStatus(context.Background(), id); !errors.Is(e, ErrNotFound) {
			t.Fatal("non-current reveal became status", id, e)
		}
	}
	// A prefinal reorg creates a new completed branch; old pending/void rows are
	// not copied. This is distinct from the retained finalized lineage above.
	b.effective = map[string]EffectiveState{initial.ProspectID: initial}
	b.transfers = map[string]TransferStatus{}
	s.transfers, e = completedTransferStatuses(context.Background(), b, states)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.TransferStatus(context.Background(), loser); !errors.Is(e, ErrNotFound) {
		t.Fatal("orphan loser remained void", e)
	}
	s.ready = false
	if _, e = s.TransferStatus(context.Background(), loser); !errors.Is(e, ErrUnavailable) {
		t.Fatal("incomplete head asserted absence", e)
	}
	delete(states, initial.StateID)
	b.effective[final.ProspectID] = final
	if _, e = completedTransferStatuses(context.Background(), b, states); !errors.Is(e, ErrCorrupt) {
		t.Fatal("missing lineage became finalized", e)
	}
}
