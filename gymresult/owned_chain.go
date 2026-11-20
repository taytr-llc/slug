// Copyright (c) 2026 The SLUG developers. ISC license.
package gymresult

import (
	"bytes"
	"context"
	"encoding/hex"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/seednode"
	"github.com/taytr-llc/slug/wire/v2"
	"math"
)

func (s *OwnedStore) Publish(ctx context.Context, bouts *gymbout.OwnedStore, cards *gymcard.OwnedStore, seeds *gymseed.OwnedStore, events *gymevent.OwnedStore, entries *gymentry.OwnedStore, session gymstable.Session, saturday, boutID string, p *seednode.ResultPublisher) (OwnedActionResult, error) {
	if p == nil {
		return OwnedActionResult{}, ErrInvalid
	}
	c, done, e := s.call(ownedSourceCall{ctx: ctx, bouts: bouts, cards: cards, seeds: seeds, events: events, entries: entries, session: session, date: saturday, id: boutID})
	if e != nil {
		return OwnedActionResult{}, e
	}
	defer done()
	if e = s.publisher(p); e != nil {
		return OwnedActionResult{}, e
	}
	return s.publish(c, p)
}
func (s *OwnedStore) Reconcile(ctx context.Context, bouts *gymbout.OwnedStore, cards *gymcard.OwnedStore, seeds *gymseed.OwnedStore, events *gymevent.OwnedStore, entries *gymentry.OwnedStore, session gymstable.Session, saturday, boutID string, p *seednode.ResultPublisher) (OwnedActionResult, error) {
	if p == nil {
		return OwnedActionResult{}, ErrInvalid
	}
	c, done, e := s.call(ownedSourceCall{ctx: ctx, bouts: bouts, cards: cards, seeds: seeds, events: events, entries: entries, session: session, date: saturday, id: boutID})
	if e != nil {
		return OwnedActionResult{}, e
	}
	defer done()
	if e = s.publisher(p); e != nil {
		return OwnedActionResult{}, e
	}
	return s.reconcile(c, p)
}
func (s *OwnedStore) Attest(ctx context.Context, bouts *gymbout.OwnedStore, cards *gymcard.OwnedStore, seeds *gymseed.OwnedStore, events *gymevent.OwnedStore, entries *gymentry.OwnedStore, session gymstable.Session, saturday, boutID string, p *seednode.ResultPublisher) (OwnedActionResult, error) {
	if p == nil {
		return OwnedActionResult{}, ErrInvalid
	}
	c, done, e := s.call(ownedSourceCall{ctx: ctx, bouts: bouts, cards: cards, seeds: seeds, events: events, entries: entries, session: session, date: saturday, id: boutID})
	if e != nil {
		return OwnedActionResult{}, e
	}
	defer done()
	if e = s.publisher(p); e != nil {
		return OwnedActionResult{}, e
	}
	return s.attest(c, p)
}
func (s *OwnedStore) publisher(p *seednode.ResultPublisher) error {
	if p == nil {
		return ErrInvalid
	}
	chain, e := seednode.OwnershipChainIdentityFor(s.meta.Pin.SourcePin.SourcePin.SourcePin.SourcePin)
	if e != nil {
		return ErrCorrupt
	}
	v := p.Identity()
	if v.Chain != chain || v.Purpose != "candidate_result_v1" || v.FundingPublicKey != s.meta.Pin.FundingPublicKey || v.FundingScript != s.meta.Pin.FundingScript {
		return ErrForbidden
	}
	return nil
}
func ownedAddConflict(r *OwnedRecord, c OwnedConflict) {
	k := ownedConflictKey(c)
	for _, v := range r.Conflicts {
		if ownedConflictKey(v) == k {
			return
		}
	}
	if len(r.Conflicts) < 8 {
		r.Conflicts = append(r.Conflicts, c)
	} else {
		r.ConflictOverflow = true
	}
	r.Observation.CurrentFinality = false
}
func ownedOwnConflict(r OwnedRecord) bool {
	return len(r.Conflicts) > 0 || r.ConflictOverflow || r.Equivocation != nil
}
func ownedObservationFor(r OwnedRecord, tip Tip, n int64, verified bool) Observation {
	o := Observation{ObservedAtMS: n, ChainObservedAtMS: n, Tip: &tip, NodeState: "online"}
	if r.Inclusion != nil && tip.Height >= r.Inclusion.Anchor.Height {
		o.Descendants = tip.Height - r.Inclusion.Anchor.Height
	}
	o.CurrentFinality = verified && r.Checkpoint != nil && o.Descendants >= 60 && !ownedOwnConflict(r)
	return o
}
func ownedChainStamp(old OwnedRecord, oldMeta ownedMetadata) func(*OwnedRecord, *ownedMetadata, int64) {
	return func(r *OwnedRecord, m *ownedMetadata, n int64) {
		r.Observation.ObservedAtMS = n
		r.Observation.ChainObservedAtMS = n
		if old.Inclusion == nil && r.Inclusion != nil {
			r.Inclusion.ObservedAtMS = n
		}
		if old.Finality == nil && r.Finality != nil {
			r.Finality.ObservedAtMS = n
			r.Finality.Inclusion = *r.Inclusion
		}
		for i := len(old.ScanRestarts); i < len(r.ScanRestarts); i++ {
			r.ScanRestarts[i].ObservedAtMS = n
		}
		for i := len(oldMeta.FundingRestarts); i < len(m.FundingRestarts); i++ {
			m.FundingRestarts[i].ObservedAtMS = n
		}
		for i := len(old.PriorInclusions); i < len(r.PriorInclusions); i++ {
			r.PriorInclusions[i].OrphanedAtMS = n
		}
		for i := len(old.Conflicts); i < len(r.Conflicts); i++ {
			r.Conflicts[i].ObservedAtMS = n
		}
	}
}
func (s *OwnedStore) reconcile(c ownedSourceCall, p observedChain) (OwnedActionResult, error) {
	r, st, e := s.read(c, gymaccount.View, true)
	if e != nil {
		return OwnedActionResult{}, e
	}
	old := r
	oldMeta := s.meta
	tip, e := p.Observe(c.ctx)
	if e != nil {
		return OwnedActionResult{}, ownedMapped(e)
	}
	n := max(s.now().UnixMilli(), s.meta.ClockMS, r.PreparedAtMS)
	next := s.meta
	next.FundingRestarts = append([]ScanRestart{}, next.FundingRestarts...)
	next.LastFundingTip = &tip
	src := r.Statement.Statement.Source.Source
	h, e := canonicalAt(c.ctx, p, tip, src.TargetHeight)
	if e != nil {
		return OwnedActionResult{}, ownedMapped(e)
	}
	if h != src.TargetHash {
		ownedAddConflict(&r, OwnedConflict{Kind: "source_chain_conflict", ObservedAtMS: n, ExpectedHeight: src.TargetHeight, ExpectedHash: src.TargetHash, ObservedHash: h, ObservedTip: &tip})
	}
	restarted := false
	if r.Inclusion != nil {
		a := r.Inclusion.Anchor
		h, e := canonicalAt(c.ctx, p, tip, a.Height)
		if e != nil {
			return OwnedActionResult{}, ownedMapped(e)
		}
		if h != a.Hash {
			if r.Checkpoint != nil {
				ownedAddConflict(&r, OwnedConflict{Kind: "checkpoint_chain_conflict", ObservedAtMS: n, ExpectedHeight: a.Height, ExpectedHash: a.Hash, ObservedHash: h, ObservedTip: &tip})
			} else {
				if len(r.PriorInclusions) >= 16 || len(r.ScanRestarts) >= 16 {
					return OwnedActionResult{}, ErrCapacity
				}
				r.PriorInclusions = append(r.PriorInclusions, PriorInclusion{1, *r.Inclusion, n, tip, h})
				r.Inclusion = nil
				if r.Cursor.Height >= 0 {
					repl, e := canonicalAt(c.ctx, p, tip, uint32(r.Cursor.Height))
					if e != nil {
						return OwnedActionResult{}, ownedMapped(e)
					}
					g := r.Cursor.Generation + 1
					r.ScanRestarts = append(r.ScanRestarts, ScanRestart{Generation: g, Previous: r.Cursor, ObservedAtMS: n, ObservedTip: tip, ReplacementHash: repl})
					r.Cursor = ownedInitialCursor(s.meta.Pin, r.ResultKey, g)
				}
				restarted = true
			}
		}
	}
	oldCoins, e := s.funding(c.ctx)
	if e != nil {
		return OwnedActionResult{}, e
	}
	coins := map[string]FundingCoin{}
	for _, x := range oldCoins {
		coins[string(coinKey('c', x.Outpoint))] = x
	}
	if next.FundingCursor.Height >= 0 {
		h, e := canonicalAt(c.ctx, p, tip, uint32(next.FundingCursor.Height))
		if e != nil {
			return OwnedActionResult{}, ownedMapped(e)
		}
		if h != next.FundingCursor.Hash {
			if len(next.FundingRestarts) >= 16 {
				return OwnedActionResult{}, ErrCapacity
			}
			g := next.FundingCursor.Generation + 1
			next.FundingRestarts = append(next.FundingRestarts, ScanRestart{Generation: g, Previous: next.FundingCursor, ObservedAtMS: n, ObservedTip: tip, ReplacementHash: h})
			next.FundingCursor = ownedInitialCursor(s.meta.Pin, "", g)
			next.FundingComplete = false
			coins = map[string]FundingCoin{}
			restarted = true
		}
	}
	if r.Publication != nil && r.Inclusion == nil && r.Cursor.Height >= 0 && !restarted {
		h, e := canonicalAt(c.ctx, p, tip, uint32(r.Cursor.Height))
		if e != nil {
			return OwnedActionResult{}, ownedMapped(e)
		}
		if h != r.Cursor.Hash {
			if len(r.ScanRestarts) >= 16 {
				return OwnedActionResult{}, ErrCapacity
			}
			g := r.Cursor.Generation + 1
			r.ScanRestarts = append(r.ScanRestarts, ScanRestart{Generation: g, Previous: r.Cursor, ObservedAtMS: n, ObservedTip: tip, ReplacementHash: h})
			r.Cursor = ownedInitialCursor(s.meta.Pin, r.ResultKey, g)
			restarted = true
		}
	}
	checked := map[uint32]string{}
	payload, _ := OwnedCarrier(r.Statement.Statement)
	verified := false
	fetch := func(height uint32) (*wire.MsgBlock, string, error) {
		if _, ok := checked[height]; !ok && len(checked) >= 8 {
			return nil, "", ErrCapacity
		}
		h, e := p.CanonicalHash(c.ctx, height)
		if e != nil {
			return nil, "", ownedMapped(e)
		}
		b, e := p.Block(c.ctx, height)
		if e != nil {
			return nil, "", ownedMapped(e)
		}
		again, e := p.CanonicalHash(c.ctx, height)
		if e != nil || again != h {
			return nil, "", ErrUnavailable
		}
		checked[height] = h
		return b, h, nil
	}
	if r.Inclusion != nil && !ownedOwnConflict(r) {
		a := r.Inclusion.Anchor
		b, h, e := fetch(a.Height)
		if e != nil {
			return OwnedActionResult{}, e
		}
		got, e := anchorAt(b, a.Height, h, a.TxIndex, 0, payload)
		if e != nil || !equal(got, a) {
			return OwnedActionResult{}, ErrUnavailable
		}
		verified = true
	}
	if !restarted && !ownedOwnConflict(r) {
		for len(checked) < 8 {
			height := int64(tip.Height) + 1
			if next.FundingCursor.Height < int32(tip.Height) {
				height = int64(next.FundingCursor.Height) + 1
			}
			if r.Publication != nil && r.Inclusion == nil && r.Cursor.Height < int32(tip.Height) && int64(r.Cursor.Height)+1 < height {
				height = int64(r.Cursor.Height) + 1
			}
			if height > int64(tip.Height) {
				break
			}
			b, h, e := fetch(uint32(height))
			if e != nil {
				return OwnedActionResult{}, e
			}
			if height == 0 && h != s.meta.Pin.Genesis {
				return OwnedActionResult{}, ErrUnavailable
			}
			if next.FundingCursor.Height+1 == int32(height) {
				if height > 0 && b.Header.PrevBlock.String() != next.FundingCursor.Hash {
					return OwnedActionResult{}, ErrUnavailable
				}
				if e = updateFunding(c.ctx, physicalOwnedPin(s.meta.Pin), coins, b, uint32(height), h); e != nil {
					return OwnedActionResult{}, e
				}
				next.FundingCursor.ScanDigest = stepDigest(next.FundingCursor.ScanDigest, uint32(height), h)
				next.FundingCursor.Height = int32(height)
				next.FundingCursor.Hash = h
			}
			if r.Publication != nil && r.Inclusion == nil && r.Cursor.Height+1 == int32(height) {
				if height > 0 && b.Header.PrevBlock.String() != r.Cursor.Hash {
					return OwnedActionResult{}, ErrUnavailable
				}
				r.Cursor.ScanDigest = stepDigest(r.Cursor.ScanDigest, uint32(height), h)
				r.Cursor.Height = int32(height)
				r.Cursor.Hash = h
				for i, tx := range b.Transactions {
					if c.ctx.Err() != nil {
						return OwnedActionResult{}, ErrCancelled
					}
					if tx.TxHash().String() != r.Publication.TxID {
						continue
					}
					var raw bytes.Buffer
					if tx.Serialize(&raw) != nil {
						return OwnedActionResult{}, ErrCorrupt
					}
					if !bytes.Equal(raw.Bytes(), rawID(r.Publication.RawTx)) {
						continue
					}
					a, e := anchorAt(b, uint32(height), h, uint32(i), 0, payload)
					if e != nil {
						return OwnedActionResult{}, e
					}
					r.Inclusion = &Inclusion{1, uint32(len(r.PriorInclusions)), a, n, tip}
					verified = true
					break
				}
			}
		}
	}
	next.FundingComplete = next.FundingCursor.Height == int32(tip.Height) && next.FundingCursor.Hash == tip.Hash
	if next.FundingComplete {
		next.FundingCursor.CompleteToHeight = int32(tip.Height)
		next.FundingCursor.CompleteToHash = tip.Hash
	}
	if r.Cursor.Height == int32(tip.Height) && r.Cursor.Hash == tip.Hash {
		r.Cursor.CompleteToHeight = r.Cursor.Height
		r.Cursor.CompleteToHash = r.Cursor.Hash
	}
	if e = bracket(c.ctx, p, tip, checked); e != nil {
		return OwnedActionResult{}, e
	}
	r.Observation = ownedObservationFor(r, tip, n, verified && st.Source.Saved.Usable)
	if r.Observation.CurrentFinality && r.Finality == nil {
		r.Finality = &Finality{*r.Checkpoint, *r.Inclusion, n, tip, r.Observation.Descendants}
	}
	b := new(leveldb.Batch)
	for _, x := range oldCoins {
		b.Delete(coinKey('c', x.Outpoint))
	}
	for _, x := range coins {
		if e = put(b, coinKey('c', x.Outpoint), x, 1024); e != nil {
			return OwnedActionResult{}, e
		}
	}
	return s.save(c, gymaccount.View, r, next, b, ownedChainStamp(old, oldMeta), false)
}
func (s *OwnedStore) publish(c ownedSourceCall, p publicationNetwork) (OwnedActionResult, error) {
	r, st, e := s.read(c, gymaccount.ResultPublish, true)
	if e != nil {
		return OwnedActionResult{}, e
	}
	if !st.Source.Saved.Usable {
		return OwnedActionResult{}, ErrSourceConflict
	}
	if ownedOwnConflict(r) {
		return OwnedActionResult{}, ErrConflict
	}
	tip, e := s.checkSavedTarget(c, r, p)
	if e != nil {
		return OwnedActionResult{}, e
	}
	if r.Inclusion != nil {
		row, current, e := s.read(c, gymaccount.ResultPublish, true)
		if e != nil {
			return OwnedActionResult{}, e
		}
		if !current.Source.Saved.Usable {
			return OwnedActionResult{}, ErrSourceConflict
		}
		return OwnedActionResult{1, row, current}, nil
	}
	payload, _ := OwnedCarrier(r.Statement.Statement)
	if r.Publication == nil {
		if !s.meta.FundingComplete || s.meta.FundingCursor.Height != int32(tip.Height) || s.meta.FundingCursor.Hash != tip.Hash {
			return OwnedActionResult{}, ErrScanIncomplete
		}
		coins, e := s.funding(c.ctx)
		if e != nil {
			return OwnedActionResult{}, e
		}
		var selected *FundingCoin
		for _, coin := range coins {
			_, reserved, e := s.raw(coinKey('u', coin.Outpoint), 256)
			if e != nil {
				return OwnedActionResult{}, e
			}
			if reserved || coin.ValueSatoshis < 1294 || !mature(coin, tip) {
				continue
			}
			h, e := p.CanonicalHash(c.ctx, coin.BlockHeight)
			if e != nil {
				return OwnedActionResult{}, ownedMapped(e)
			}
			if h != coin.BlockHash {
				return OwnedActionResult{}, ErrScanIncomplete
			}
			unspent, e := p.Unspent(c.ctx, coin.Outpoint)
			if e != nil {
				return OwnedActionResult{}, ownedMapped(e)
			}
			if unspent == nil {
				continue
			}
			if unspent.Script != coin.Script || unspent.ValueSatoshis != coin.ValueSatoshis || unspent.Coinbase != coin.Coinbase || unspent.Confirmations == 0 {
				return OwnedActionResult{}, ErrUnavailable
			}
			x := coin
			selected = &x
			break
		}
		if selected == nil {
			return OwnedActionResult{}, ErrWaitingForFunds
		}
		raw, e := p.BuildPublication(c.ctx, payload, *selected)
		if e != nil {
			return OwnedActionResult{}, ownedMapped(e)
		}
		if len(raw) == 0 || len(raw) > MaxRawTxSize {
			return OwnedActionResult{}, ErrInvalid
		}
		var tx wire.MsgTx
		if tx.Deserialize(bytes.NewReader(raw)) != nil {
			return OwnedActionResult{}, ErrInvalid
		}
		if _, e = s.checkSavedTarget(c, r, p); e != nil {
			return OwnedActionResult{}, e
		}
		n := max(s.now().UnixMilli(), s.meta.ClockMS)
		r.Publication = &Publication{Version: 1, RawTx: hex.EncodeToString(raw), TxID: tx.TxHash().String(), Coin: *selected, BuiltAtMS: n, State: "built"}
		r.Observation = ownedObservationFor(r, tip, n, false)
		if e = bracket(c.ctx, p, tip, nil); e != nil {
			return OwnedActionResult{}, e
		}
		b := new(leveldb.Batch)
		if e = put(b, coinKey('u', selected.Outpoint), reservation{1, r.ResultKey, r.Publication.TxID}, 256); e != nil {
			return OwnedActionResult{}, e
		}
		out, e := s.save(c, gymaccount.ResultPublish, r, s.meta, b, func(r *OwnedRecord, _ *ownedMetadata, n int64) {
			r.Publication.BuiltAtMS = n
			r.Observation.ObservedAtMS = n
			r.Observation.ChainObservedAtMS = n
		}, true)
		if e != nil {
			return OwnedActionResult{}, e
		}
		r = out.Record
		s.mark("transaction-retained")
	}
	if r.Publication.DispatchCount == math.MaxUint32 {
		return OwnedActionResult{}, ErrCapacity
	}
	// An exact retry preserves the one retained input/raw transaction. It does not
	// re-sign even when the retained valid signature uses another nonce.
	unspent, e := p.Unspent(c.ctx, r.Publication.Coin.Outpoint)
	if e != nil || unspent == nil {
		if e != nil {
			return OwnedActionResult{}, ownedMapped(e)
		}
		return OwnedActionResult{}, ErrUnavailable
	}
	coin := r.Publication.Coin
	if unspent.Script != coin.Script || unspent.ValueSatoshis != coin.ValueSatoshis || unspent.Coinbase != coin.Coinbase || unspent.Confirmations == 0 {
		return OwnedActionResult{}, ErrUnavailable
	}
	freshTip, e := s.checkSavedTarget(c, r, p)
	if e != nil {
		return OwnedActionResult{}, e
	}
	if !mature(coin, freshTip) {
		return OwnedActionResult{}, ErrNotMature
	}
	if h, e := p.CanonicalHash(c.ctx, coin.BlockHeight); e != nil {
		return OwnedActionResult{}, ownedMapped(e)
	} else if h != coin.BlockHash {
		return OwnedActionResult{}, ErrUnavailable
	}
	if e = bracket(c.ctx, p, freshTip, map[uint32]string{coin.BlockHeight: coin.BlockHash}); e != nil {
		return OwnedActionResult{}, e
	}
	n := max(s.now().UnixMilli(), s.meta.ClockMS)
	r.Publication.DispatchCount++
	r.Publication.State = "dispatch_unknown"
	if r.Publication.FirstDispatchAtMS == 0 {
		r.Publication.FirstDispatchAtMS = n
	}
	r.Publication.LastDispatchAtMS = n
	r.Observation.ObservedAtMS = n
	out, e := s.save(c, gymaccount.ResultPublish, r, s.meta, nil, func(r *OwnedRecord, _ *ownedMetadata, n int64) {
		if r.Publication.DispatchCount == 1 {
			r.Publication.FirstDispatchAtMS = n
		}
		r.Publication.LastDispatchAtMS = n
		r.Observation.ObservedAtMS = n
	}, true)
	if e != nil {
		return OwnedActionResult{}, e
	}
	r = out.Record
	s.mark("dispatch-armed")
	if c.ctx.Err() != nil || s.life.Err() != nil {
		return OwnedActionResult{}, ErrOutcomeUnknown
	}
	txid, e := p.SubmitPublication(c.ctx, payload, r.Publication.Coin, rawID(r.Publication.RawTx))
	if e != nil || txid != r.Publication.TxID {
		return OwnedActionResult{}, ErrOutcomeUnknown
	}
	s.mark("dispatch-acknowledged")
	if _, e = s.checkSavedTarget(c, r, p); e != nil {
		return OwnedActionResult{}, ErrOutcomeUnknown
	}
	r.Publication.State = "submitted"
	out, e = s.save(c, gymaccount.ResultPublish, r, s.meta, nil, func(r *OwnedRecord, _ *ownedMetadata, n int64) { r.Observation.ObservedAtMS = n }, true)
	if e != nil {
		return OwnedActionResult{}, ErrOutcomeUnknown
	}
	return out, nil
}
func (s *OwnedStore) attest(c ownedSourceCall, p attestationNetwork) (OwnedActionResult, error) {
	r, st, e := s.read(c, gymaccount.ResultPublish, true)
	if e != nil {
		return OwnedActionResult{}, e
	}
	if !st.Source.Saved.Usable {
		return OwnedActionResult{}, ErrSourceConflict
	}
	if ownedOwnConflict(r) {
		return OwnedActionResult{}, ErrConflict
	}
	if _, e = s.checkSavedTarget(c, r, p); e != nil {
		return OwnedActionResult{}, e
	}
	if r.Inclusion == nil {
		return OwnedActionResult{}, ErrNotMature
	}
	if r.Finality != nil {
		row, current, e := s.read(c, gymaccount.ResultPublish, true)
		if e != nil {
			return OwnedActionResult{}, e
		}
		if !current.Source.Saved.Usable {
			return OwnedActionResult{}, ErrSourceConflict
		}
		return OwnedActionResult{1, row, current}, nil
	}
	cp, tip, e := p.AttestInclusion(c.ctx, r.Inclusion.Anchor)
	if e != nil {
		return OwnedActionResult{}, ownedMapped(e)
	}
	if !ownedValidCheckpoint(s.meta.Pin, cp) || cp.Height != int32(r.Inclusion.Anchor.Height) || cp.Hash.String() != r.Inclusion.Anchor.Hash || tip.Height < r.Inclusion.Anchor.Height || tip.Height-r.Inclusion.Anchor.Height < 60 {
		return OwnedActionResult{}, ErrUnavailable
	}
	if _, e = s.checkSavedTarget(c, r, p); e != nil {
		return OwnedActionResult{}, e
	}
	src := r.Statement.Statement.Source.Source
	if e = bracket(c.ctx, p, tip, map[uint32]string{src.TargetHeight: src.TargetHash, r.Inclusion.Anchor.Height: r.Inclusion.Anchor.Hash}); e != nil {
		return OwnedActionResult{}, e
	}
	old := r
	n := max(s.now().UnixMilli(), s.meta.ClockMS)
	if r.Checkpoint == nil {
		r.Checkpoint = &cp
	}
	r.Finality = &Finality{*r.Checkpoint, *r.Inclusion, n, tip, tip.Height - r.Inclusion.Anchor.Height}
	r.Observation = ownedObservationFor(r, tip, n, true)
	return s.save(c, gymaccount.ResultPublish, r, s.meta, nil, ownedChainStamp(old, s.meta), true)
}

// The exported action admits only the concrete ownedValue publisher. This private
// read boundary keeps target observation and its evidence commit testable without
// introducing an imported authority or a public callback capability.
// observedChain is private to the journal algorithm. Public entry points still
// require and pin the concrete, ownedValue ResultPublisher before invoking it.

func (s *OwnedStore) checkSavedTarget(c ownedSourceCall, r OwnedRecord, p targetReader) (Tip, error) {
	tip, e := p.Observe(c.ctx)
	if e != nil {
		return Tip{}, ownedMapped(e)
	}
	src := r.Statement.Statement.Source.Source
	observed := ""
	if src.TargetHeight <= tip.Height {
		observed, e = p.CanonicalHash(c.ctx, src.TargetHeight)
		if e != nil {
			return Tip{}, ownedMapped(e)
		}
	}
	// A failed captured-tip bracket is unavailable evidence, never a conflict.
	current, e := p.Observe(c.ctx)
	if e != nil {
		return Tip{}, ownedMapped(e)
	}
	if current.Height < tip.Height {
		return Tip{}, ErrUnavailable
	}
	h, e := p.CanonicalHash(c.ctx, tip.Height)
	if e != nil || h != tip.Hash {
		return Tip{}, ErrUnavailable
	}
	if observed == src.TargetHash {
		return tip, nil
	}
	// Recheck the mismatch under the same captured canonical branch before it is
	// latched; RPC failure and a moving branch are not signed conflict facts.
	again := ""
	if src.TargetHeight <= tip.Height {
		again, e = p.CanonicalHash(c.ctx, src.TargetHeight)
		if e != nil {
			return Tip{}, ownedMapped(e)
		}
	}
	if again != observed {
		return Tip{}, ErrUnavailable
	}
	// OwnedConflict evidence may retain only the durable row. Tentative signing work
	// must never smuggle an unreserved publication into this evidence-only sync.
	durable, ok, e := s.record(r.ResultKey)
	if e != nil {
		return Tip{}, e
	}
	if !ok || !equal(durable.Statement, r.Statement) {
		return Tip{}, ErrSourceChanged
	}
	r = durable
	n := max(s.now().UnixMilli(), s.meta.ClockMS)
	old := r
	ownedAddConflict(&r, OwnedConflict{Kind: "source_chain_conflict", ObservedAtMS: n, ExpectedHeight: src.TargetHeight, ExpectedHash: src.TargetHash, ObservedHash: observed, ObservedTip: &tip})
	r.Observation = ownedObservationFor(r, tip, n, false)
	if _, e = s.save(c, gymaccount.View, r, s.meta, nil, ownedChainStamp(old, s.meta), false); e != nil {
		return Tip{}, e
	}
	return Tip{}, ErrSourceConflict
}
