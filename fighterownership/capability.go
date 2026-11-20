// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterownership

import (
	"context"
	"encoding/hex"
	"sync/atomic"

	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/fightertransfer"
	"github.com/taytr-llc/slug/wire/v2"
)

// OwnedCapture retains immutable membership and evidence from one exact head.
// There is no public constructor or method for substituting its original owner.
type OwnedCapture struct {
	store      *Store
	head       HeadRef
	gym        string
	items      []MapItem
	generation uint64
}

// HeadLease is a local lifetime/head fence. Check performs no network or DB I/O.
// Fresh node verification belongs to PreparedTransfer.Check outside account leases.
type HeadLease struct {
	source     *PublicSource
	node       *OwnedChainSource
	store      *Store
	head       HeadRef
	generation uint64
	closed     atomic.Bool
}

func (h *HeadLease) Check() error {
	if h == nil || h.store == nil || h.closed.Load() {
		return ErrStale
	}
	s := h.store
	if !s.mu.TryLock() {
		return ErrBusy
	}
	defer s.mu.Unlock()
	return h.checkLocked()
}

// checkLocked is repeated in the same critical section that captures the
// predecessor/commit. An earlier Check cannot authorize a newer head.
func (h *HeadLease) checkLocked() error {
	if h == nil || h.store == nil || h.closed.Load() {
		return ErrStale
	}
	s := h.store
	if e := s.state(); e != nil {
		return e
	}
	if s.source == nil || s.node == nil || s.source.Alive() != nil || s.node.Alive() != nil {
		return ErrStale
	}
	if !s.ready || h.generation != s.generation || h.head != s.head || h.source != s.source || h.node != s.node {
		return ErrStale
	}
	return nil
}
func (h *HeadLease) Close() error {
	if h != nil {
		h.closed.Store(true)
	}
	return nil
}

// PreparedTransfer is a concrete replay-authorized predecessor capability.
// Its projection is public, but copying that projection cannot recreate it.
// It does not capture, replace or authorize a hosted account session.
type PreparedTransfer struct {
	store       *Store
	source      *PublicSource
	node        *OwnedChainSource
	observation ChainObservation
	generation  uint64
	predecessor EffectiveState
	intent      TransferIntent
	head        HeadRef
}

func (p *PreparedTransfer) Head() HeadRef {
	if p == nil {
		return HeadRef{}
	}
	return p.head
}
func (p *PreparedTransfer) Intent() TransferIntent {
	if p == nil {
		return TransferIntent{}
	}
	v := p.intent
	v.Funding = append(FundingTuple(nil), v.Funding...)
	return v
}
func (p *PreparedTransfer) Check(ctx context.Context) error {
	if ctx == nil || p == nil || p.store == nil || p.node == nil || p.source == nil {
		return ErrInvalid
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	// The original concrete owner remains fixed. A completed later replay may
	// establish a newer observation only when the exact selected predecessor is
	// unchanged; the reviewed intent and original Head projection never change.
	source, observation, e := p.currentCheck()
	if e != nil {
		return e
	}
	if e = source.Check(ctx); e != nil {
		return e
	}
	if e = p.node.Check(ctx, observation); e != nil {
		return e
	}
	again, after, e := p.currentCheck()
	if e != nil {
		return e
	}
	if again != source || after != observation {
		return ErrStale
	}
	return nil
}
func (p *PreparedTransfer) checkLocal() error { _, _, e := p.currentCheck(); return e }
func (p *PreparedTransfer) currentCheck() (*PublicSource, ChainObservation, error) {
	s := p.store
	if !s.mu.TryLock() {
		return nil, ChainObservation{}, ErrBusy
	}
	defer s.mu.Unlock()
	if e := s.state(); e != nil {
		return nil, ChainObservation{}, e
	}
	if !s.ready || p.generation != s.generation || s.source == nil || !p.source.sameOwner(s.source) || p.node != s.node || s.head.ConflictID != "" {
		return nil, ChainObservation{}, ErrStale
	}
	v, ok := s.effective[p.predecessor.ProspectID]
	if !ok || v.StateID != p.predecessor.StateID || v.OwnerPublicKey != p.predecessor.OwnerPublicKey || !s.portable[v.StateID] {
		return nil, ChainObservation{}, ErrStale
	}
	return s.source, s.observation, nil
}

func (s *Store) PrepareTransfer(ctx context.Context, h *HeadLease, prospectID, fromGym, recipient string, tuple FundingTuple) (*PreparedTransfer, error) {
	if ctx == nil || h == nil || h.store != s || len(tuple) < 1 || len(tuple) > 16 || fromGym == recipient {
		return nil, ErrInvalid
	}
	if e := h.Check(); e != nil {
		return nil, e
	}
	p, e := s.transferSnapshot(h, prospectID, fromGym)
	if e != nil {
		return nil, e
	}
	predecessor := p.predecessor
	pinID := s.pinID
	var compact fightertransfer.Intent
	a, e := hex.DecodeString(pinID)
	if e != nil || len(a) != 32 {
		return nil, ErrInvalid
	}
	copy(compact.PinID[:], a)
	a, e = hex.DecodeString(predecessor.StateID)
	if e != nil || len(a) != 32 {
		return nil, ErrInvalid
	}
	copy(compact.PriorStateID[:], a)
	a, e = hex.DecodeString(recipient)
	if e != nil || len(a) != 33 {
		return nil, ErrInvalid
	}
	copy(compact.RecipientGym[:], a)
	ops := make([]wire.OutPoint, len(tuple))
	for n, v := range tuple {
		hash, e := chainhash.NewHashFromStr(v.TxID)
		if e != nil || hash.String() != v.TxID {
			return nil, ErrInvalid
		}
		ops[n] = wire.OutPoint{Hash: *hash, Index: v.Index}
	}
	funding, e := fightertransfer.FundingDigest(ops)
	if e != nil {
		return nil, ErrInvalid
	}
	compact.FundingDigest = funding
	raw, e := fightertransfer.MarshalIntent(compact)
	if e != nil {
		return nil, ErrInvalid
	}
	id, e := fightertransfer.TransferID(compact)
	if e != nil {
		return nil, ErrInvalid
	}
	p.intent = TransferIntent{Version: 1, PinID: pinID, ProspectID: prospectID, PriorStateID: predecessor.StateID, FromGym: fromGym, Recipient: recipient, Funding: append(FundingTuple(nil), tuple...), FundingDigest: hex.EncodeToString(funding[:]), TransferID: hex.EncodeToString(id[:]), IntentHex: hex.EncodeToString(raw)}
	if e := p.Check(ctx); e != nil {
		return nil, e
	}
	return p, nil
}

func (s *Store) transferSnapshot(h *HeadLease, prospectID, fromGym string) (*PreparedTransfer, error) {
	if !s.mu.TryLock() {
		return nil, ErrBusy
	}
	if e := h.checkLocked(); e != nil {
		s.mu.Unlock()
		return nil, e
	}
	predecessor, ok := s.effective[prospectID]
	if !ok || predecessor.OwnerPublicKey != fromGym || s.source == nil || s.node == nil || s.head.ConflictID != "" {
		s.mu.Unlock()
		return nil, ErrUnavailable
	}
	// An initial admission must have its signed public statement before transfer;
	// later states additionally require the retained public finality witness.
	if !s.portable[predecessor.StateID] {
		s.mu.Unlock()
		return nil, ErrUnavailable
	}
	p := &PreparedTransfer{store: s, source: s.source, node: s.node, observation: s.observation, generation: s.generation, predecessor: cloneState(predecessor), head: h.head}
	s.mu.Unlock()
	return p, nil
}
