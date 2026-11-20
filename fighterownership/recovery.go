// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterownership

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"time"

	"github.com/taytr-llc/slug/fightertransfer"
	"github.com/taytr-llc/slug/wire/v2"
)

// PreparedRecovery proves an exact canonical commit and authenticated historical
// predecessor under one retained owner. It does not grant present ownership or
// decide whether a later reveal/refund is eligible. No caller DTO constructs it.
type PreparedRecovery struct {
	store      *Store
	source     *PublicSource
	node       *OwnedChainSource
	generation uint64
	head       HeadRef
	commit     TransactionWitness
	prior      EffectiveState
	intent     TransferIntent
	inspection fightertransfer.CommitInspection
}

func (p *PreparedRecovery) Head() HeadRef {
	if p == nil {
		return HeadRef{}
	}
	return p.head
}
func (p *PreparedRecovery) Commit() TransactionWitness {
	if p == nil {
		return TransactionWitness{}
	}
	v := p.commit
	v.MerkleBranch = append([]string{}, v.MerkleBranch...)
	return v
}
func (p *PreparedRecovery) CommitRaw() []byte {
	if p == nil {
		return nil
	}
	v, _ := hex.DecodeString(p.commit.Raw)
	return v
}
func (p *PreparedRecovery) Prior() EffectiveState {
	if p == nil {
		return EffectiveState{}
	}
	return cloneState(p.prior)
}
func (p *PreparedRecovery) Intent() TransferIntent {
	if p == nil {
		return TransferIntent{}
	}
	v := p.intent
	v.Funding = append(FundingTuple{}, v.Funding...)
	return v
}
func (p *PreparedRecovery) Inspection() fightertransfer.CommitInspection {
	if p == nil {
		return fightertransfer.CommitInspection{}
	}
	v := p.inspection
	v.Funding = append([]wire.OutPoint{}, v.Funding...)
	return v
}
func (p *PreparedRecovery) current() (*PublicSource, ChainObservation, error) {
	if p == nil || p.store == nil {
		return nil, ChainObservation{}, ErrInvalid
	}
	s := p.store
	if !s.mu.TryLock() {
		return nil, ChainObservation{}, ErrBusy
	}
	defer s.mu.Unlock()
	if e := s.state(); e != nil {
		return nil, ChainObservation{}, e
	}
	if !s.ready || s.generation != p.generation || s.node != p.node || s.source == nil || !p.source.sameOwner(s.source) || s.canonical[p.commit.Height] != p.commit.BlockHash {
		return nil, ChainObservation{}, ErrStale
	}
	prior, ok := s.states[p.prior.StateID]
	if !ok || !s.portable[prior.StateID] || prior.ProspectID != p.prior.ProspectID || prior.OwnerPublicKey != p.prior.OwnerPublicKey {
		return nil, ChainObservation{}, ErrStale
	}
	return s.source, s.observation, nil
}
func (p *PreparedRecovery) Check(ctx context.Context) error {
	if ctx == nil {
		return ErrInvalid
	}
	source, obs, e := p.current()
	if e != nil {
		return e
	}
	if e = source.Check(ctx); e != nil {
		return e
	}
	if e = p.node.Check(ctx, obs); e != nil {
		return e
	}
	if e = p.node.CheckUnspent(ctx, p.commit.TxID, 0, fightertransfer.CommitAtoms, p.inspection.CommitScript[:]); e != nil {
		return e
	}
	if e = p.node.Check(ctx, obs); e != nil {
		return e
	}
	again, next, e := p.current()
	if e != nil {
		return e
	}
	if again != source || next != obs {
		return ErrStale
	}
	return ctx.Err()
}
func (s *Store) PrepareRecovery(ctx context.Context, h *HeadLease, commitID string) (*PreparedRecovery, error) {
	if ctx == nil || h == nil || h.store != s || !hexValue(commitID, 32, true) {
		return nil, ErrInvalid
	}
	if e := h.Check(); e != nil {
		return nil, e
	}
	done, e := s.project(ctx)
	if e != nil {
		return nil, e
	}
	defer done()
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	p, e := s.recoverySnapshot(h, commitID)
	if e != nil {
		return nil, e
	}
	commit := p.commit
	raw, _ := hex.DecodeString(commit.Raw)
	c, e := boundedTx(ctx, raw, 4096)
	if e != nil {
		return nil, e
	}
	if len(c.TxOut) != 3 || len(c.TxIn) < 1 || len(c.TxIn) > 16 {
		return nil, ErrInvalid
	}
	script := c.TxOut[1].PkScript
	if len(script) != 76 || script[0] != 0x6a || script[1] != 0x4a || string(script[2:11]) != "SLUGFCM1\x01" {
		return nil, ErrInvalid
	}
	var pin, prior [32]byte
	var recipient [33]byte
	decoded, _ := hex.DecodeString(s.pinID)
	copy(pin[:], decoded)
	copy(prior[:], script[11:43])
	copy(recipient[:], script[43:])
	priorID := hex.EncodeToString(prior[:])
	if !s.mu.TryLock() {
		return nil, ErrBusy
	}
	state, ok := s.states[priorID]
	s.mu.Unlock()
	if !ok {
		return nil, ErrUnavailable
	}
	if state.Generation > 0 {
		var ev FinalStateEvidence
		json.Unmarshal(state.Evidence, &ev)
		if ev.SixtiethDescendantHeight >= commit.Height {
			return nil, ErrInvalid
		}
	}
	p.prior = cloneState(state)
	parents := make([][]byte, 0, len(c.TxIn))
	seen := map[string]bool{}
	total := len(raw)
	for _, in := range c.TxIn {
		id := in.PreviousOutPoint.Hash.String()
		if seen[id] {
			continue
		}
		seen[id] = true
		b, e := p.node.Transaction(ctx, id)
		if e != nil {
			return nil, e
		}
		if len(b) > fightertransfer.MaxEvidenceSize-total {
			return nil, ErrCapacity
		}
		total += len(b)
		parents = append(parents, b)
	}
	inspection, e := fightertransfer.InspectCommit(ctx, pin, prior, recipient, raw, parents)
	if e != nil {
		return nil, e
	}
	if inspection.CommitTxID.String() != commitID {
		return nil, ErrCorrupt
	}
	p.inspection = inspection
	tuple := make(FundingTuple, len(inspection.Funding))
	for n, op := range inspection.Funding {
		tuple[n] = FundingOutpoint{op.Hash.String(), op.Index}
	}
	intentRaw, _ := fightertransfer.MarshalIntent(inspection.Intent)
	tid, _ := fightertransfer.TransferID(inspection.Intent)
	p.intent = TransferIntent{1, s.pinID, state.ProspectID, state.StateID, state.OwnerPublicKey, hex.EncodeToString(recipient[:]), tuple, hex.EncodeToString(inspection.Intent.FundingDigest[:]), hex.EncodeToString(tid[:]), hex.EncodeToString(intentRaw)}
	if e = p.Check(ctx); e != nil {
		return nil, e
	}
	return p, nil
}

func (s *Store) recoverySnapshot(h *HeadLease, commitID string) (*PreparedRecovery, error) {
	if !s.mu.TryLock() {
		return nil, ErrBusy
	}
	if e := h.checkLocked(); e != nil {
		s.mu.Unlock()
		return nil, e
	}
	commit, ok := s.commits[commitID]
	if !ok {
		s.mu.Unlock()
		return nil, ErrNotFound
	}
	p := &PreparedRecovery{store: s, source: s.source, node: s.node, generation: s.generation, head: h.head, commit: commit}
	s.mu.Unlock()
	return p, nil
}
