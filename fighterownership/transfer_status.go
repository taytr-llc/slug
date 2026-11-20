// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterownership

import (
	"context"
	"encoding/json"
)

// recordReveal runs only after the exact canonical carrier and prior signature
// have been verified. Refunds never enter this ownership disposition map.
func (b *replayBranch) recordReveal(priorID, id string, eligible bool) bool {
	if !eligible {
		b.transfers[id] = TransferStatus{TransferID: id, State: "void"}
		return false
	}
	if winner, exists := b.winners[priorID]; exists && winner != id {
		b.transfers[id] = TransferStatus{TransferID: id, State: "void"}
		return false
	}
	b.winners[priorID] = id
	b.transfers[id] = TransferStatus{TransferID: id, State: "pending"}
	return true
}

// completedTransferStatuses projects only this completed branch and its
// official effective lineages. Retained fork/partial-state rows are evidence,
// not a current status. Official ancestors remain finalized after later sends
// or a checkpoint-crossing reorg.
func completedTransferStatuses(ctx context.Context, b *replayBranch, states map[string]EffectiveState) (map[string]TransferStatus, error) {
	out := make(map[string]TransferStatus, len(b.transfers))
	for id, v := range b.transfers {
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		out[id] = v
	}
	visited := make(map[string]bool)
	for _, endpoint := range b.effective {
		for state := endpoint; state.Generation > 0; {
			if e := ctx.Err(); e != nil {
				return nil, e
			}
			if visited[state.StateID] || len(visited) >= 4096 {
				return nil, ErrCorrupt
			}
			visited[state.StateID] = true
			var ev FinalStateEvidence
			if json.Unmarshal(state.Evidence, &ev) != nil {
				return nil, ErrCorrupt
			}
			prior, ok := states[ev.PriorStateID]
			if !ok || prior.ProspectID != state.ProspectID || prior.Generation+1 != state.Generation {
				return nil, ErrCorrupt
			}
			owned := cloneState(state)
			out[ev.TransferID] = TransferStatus{TransferID: ev.TransferID, State: "finalized", EffectiveState: &owned}
			state = prior
		}
	}
	return out, nil
}

// TransferStatus reports ownership status at the completed ready head. An ID
// known only on an orphan branch or as a refund has no current reveal status.
func (s *Store) TransferStatus(ctx context.Context, id string) (TransferStatus, error) {
	if !hexValue(id, 32, true) {
		return TransferStatus{}, ErrInvalid
	}
	if e := s.readLock(ctx); e != nil {
		return TransferStatus{}, e
	}
	defer s.mu.Unlock()
	out, ok := s.transfers[id]
	if !ok {
		return TransferStatus{}, ErrNotFound
	}
	out.ConflictID = s.head.ConflictID
	if out.EffectiveState != nil {
		owned := cloneState(*out.EffectiveState)
		out.EffectiveState = &owned
		out.PublicEvidencePending = !s.portable[owned.StateID]
	}
	return out, ctx.Err()
}
