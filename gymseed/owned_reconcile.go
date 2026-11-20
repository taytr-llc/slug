// Copyright (c) 2026 The SLUG developers. ISC license.
package gymseed

import (
	"context"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/seednode"
	"github.com/taytr-llc/slug/wire/v2"
	"math"
)

func (s *OwnedStore) Reconcile(ctx context.Context, events *gymevent.OwnedStore, entries *gymentry.OwnedStore, session gymstable.Session, p *seednode.Publisher, date string) (result OwnedRecord, resultErr error) {
	if p == nil {
		return OwnedRecord{}, ErrNodeOffline
	}
	work, done, e := s.begin(ctx)
	if e != nil {
		return OwnedRecord{}, e
	}
	defer done()
	r, e := s.actionRecord(work, events, entries, session, gymaccount.View, date)
	if e != nil {
		return OwnedRecord{}, e
	}
	id := r.EventKey
	defer func() {
		if resultErr != nil {
			resultErr = s.failedChainAttempt(work, events, entries, session, gymaccount.View, id, resultErr)
		}
	}()
	if e = s.publisher(p); e != nil {
		return OwnedRecord{}, e
	}
	if ownedPhase(r) == "conflict" {
		return s.replay(work, events, entries, session, gymaccount.View, r)
	}
	original := r
	stamp := func(r *OwnedRecord, n int64) {
		r.Observation.ObservedAtMS = n
		r.Observation.ChainObservedAtMS = n
		if original.Target == nil && r.Target != nil {
			r.Target.ObservedAtMS = n
		}
		if original.Finality == nil && r.Finality != nil {
			r.Finality.ObservedAtMS = n
		}
		for i := len(original.ScanRestarts); i < len(r.ScanRestarts); i++ {
			r.ScanRestarts[i].ObservedAtMS = n
		}
		for i := len(original.Conflicts); i < len(r.Conflicts); i++ {
			r.Conflicts[i].ObservedAtMS = n
		}
	}
	tip, e := p.Observe(work)
	if e != nil {
		return OwnedRecord{}, ownedMapped(e)
	}
	n := s.now().UnixMilli()
	next := s.meta
	next.FundingRestarts = append([]ScanRestart{}, s.meta.FundingRestarts...)
	next.LastFundingTip = &tip
	// Only an actual pinned anchor/target mismatch conflicts this event.
	for _, ref := range []*seednode.HeaderRef{anchorRef(r.Anchor), targetRef(r.Target)} {
		if ref == nil {
			continue
		}
		h, e := canonicalAt(work, p, tip, ref.Height)
		if e != nil {
			return OwnedRecord{}, ownedMapped(e)
		}
		if h != ref.Hash {
			if e = bracket(work, p, tip, nil); e != nil {
				return OwnedRecord{}, e
			}
			ownedAddConflict(&r, OwnedConflict{Kind: "chain", ObservedAtMS: n, ExpectedHeight: ref.Height, ExpectedHash: ref.Hash, ObservedHash: h, ObservedTip: &tip})
			r.Observation = ownedObservationFor(r, tip, n, false)
			return s.saveAction(work, events, entries, session, gymaccount.View, r, s.meta, nil, stamp)
		}
	}
	oldCoins, e := s.funding(work)
	if e != nil {
		return OwnedRecord{}, e
	}
	coins := map[string]FundingCoin{}
	for _, c := range oldCoins {
		coins[string(coinKey('f', c.Outpoint))] = c
	}
	restarted := false
	fundingRestarted := false
	if next.FundingCursor.Height >= 0 {
		h, e := canonicalAt(work, p, tip, uint32(next.FundingCursor.Height))
		if e != nil {
			return OwnedRecord{}, ownedMapped(e)
		}
		if h != next.FundingCursor.Hash {
			if len(next.FundingRestarts) >= 16 {
				return OwnedRecord{}, ErrCapacity
			}
			g := next.FundingCursor.Generation + 1
			next.FundingRestarts = append(next.FundingRestarts, ScanRestart{Generation: g, Previous: next.FundingCursor, ObservedAtMS: n, ObservedTip: tip, ReplacementHash: h})
			next.FundingCursor = initialOwnedCursor(s.meta.Pin, "", g)
			next.FundingComplete = false
			coins = map[string]FundingCoin{}
			fundingRestarted = true
			restarted = true
		}
	}
	if r.Anchor == nil && r.Cursor.Height >= 0 {
		h, e := canonicalAt(work, p, tip, uint32(r.Cursor.Height))
		if e != nil {
			return OwnedRecord{}, ownedMapped(e)
		}
		if h != r.Cursor.Hash {
			if len(r.ScanRestarts) >= 16 {
				return OwnedRecord{}, ErrCapacity
			}
			g := r.Cursor.Generation + 1
			r.ScanRestarts = append(r.ScanRestarts, ScanRestart{Generation: g, Previous: r.Cursor, ObservedAtMS: n, ObservedTip: tip, ReplacementHash: h})
			r.Cursor = initialOwnedCursor(s.meta.Pin, r.EventKey, g)
			restarted = true
		}
	}
	if restarted {
		if e = bracket(work, p, tip, nil); e != nil {
			return OwnedRecord{}, e
		}
		r.Observation = ownedObservationFor(r, tip, n, false)
		b := new(leveldb.Batch)
		if fundingRestarted {
			for _, c := range oldCoins {
				b.Delete(coinKey('f', c.Outpoint))
			}
		}
		next.FundingComplete = next.FundingCursor.Height == int32(tip.Height) && next.FundingCursor.Hash == tip.Hash
		if !next.FundingComplete {
			next.LastFundingTip = &tip
		}
		return s.saveAction(work, events, entries, session, gymaccount.View, r, next, b, stamp)
	}
	payload, e := ownedCarrier(r.Declaration.Declaration)
	if e != nil {
		return OwnedRecord{}, e
	}
	checked := map[uint32]string{}
	verifiedAnchor, verifiedTarget := false, false
	fetch := func(height uint32) (*wire.MsgBlock, string, error) {
		if _, ok := checked[height]; !ok && len(checked) >= MaxScanBlocks {
			return nil, "", ErrCapacity
		}
		h, e := p.CanonicalHash(work, height)
		if e != nil {
			return nil, "", ownedMapped(e)
		}
		block, e := p.Block(work, height)
		if e != nil {
			return nil, "", ownedMapped(e)
		}
		if again, e := p.CanonicalHash(work, height); e != nil || again != h {
			return nil, "", ErrUnavailable
		}
		checked[height] = h
		return block, h, nil
	}
	if r.Anchor != nil {
		b, h, e := fetch(r.Anchor.Height)
		if e != nil {
			return OwnedRecord{}, e
		}
		a, e := anchorAt(b, r.Anchor.Height, h, r.Anchor.TxIndex, r.Anchor.OutputIndex, payload)
		if e != nil || !equal(a, *r.Anchor) {
			return OwnedRecord{}, ErrUnavailable
		}
		verifiedAnchor = true
		b = nil
	}
	if r.Target != nil {
		b, h, e := fetch(r.Target.Height)
		if e != nil {
			return OwnedRecord{}, e
		}
		if h != r.Target.Hash || headerString(b.Header) != r.Target.Header || b.Header.PrevBlock.String() != r.Anchor.Hash {
			return OwnedRecord{}, ErrUnavailable
		}
		verifiedTarget = true
		b = nil
	}
	for len(checked) < MaxScanBlocks {
		height := int64(tip.Height) + 1
		if next.FundingCursor.Height < int32(tip.Height) {
			height = int64(next.FundingCursor.Height) + 1
		}
		if r.Anchor == nil && r.Cursor.Height < int32(tip.Height) && int64(r.Cursor.Height)+1 < height {
			height = int64(r.Cursor.Height) + 1
		}
		if height > int64(tip.Height) {
			break
		}
		b, h, e := fetch(uint32(height))
		if e != nil {
			return OwnedRecord{}, e
		}
		if next.FundingCursor.Height+1 == int32(height) {
			if height > 0 && b.Header.PrevBlock.String() != next.FundingCursor.Hash {
				return OwnedRecord{}, ErrUnavailable
			}
			if e = updateFunding(work, physicalOwnedPin(s.meta.Pin), coins, b, uint32(height), h); e != nil {
				return OwnedRecord{}, e
			}
			advanceCursor(&next.FundingCursor, uint32(height), h, false)
		}
		if r.Anchor == nil && r.Cursor.Height+1 == int32(height) {
			if height > 0 && b.Header.PrevBlock.String() != r.Cursor.Hash {
				return OwnedRecord{}, ErrUnavailable
			}
			a, e := firstAnchor(work, b, uint32(height), h, payload)
			if e != nil {
				return OwnedRecord{}, e
			}
			advanceCursor(&r.Cursor, uint32(height), h, true)
			if a != nil {
				r.Anchor = a
				r.Cursor.CompleteToHeight = r.Cursor.Height
				r.Cursor.CompleteToHash = r.Cursor.Hash
				verifiedAnchor = true
			}
		}
		b = nil
	}
	if next.FundingCursor.Height == int32(tip.Height) && next.FundingCursor.Hash == tip.Hash {
		next.FundingCursor.CompleteToHeight = int32(tip.Height)
		next.FundingCursor.CompleteToHash = tip.Hash
		next.FundingComplete = true
	} else {
		next.FundingComplete = false
	}
	if r.Anchor == nil && r.Cursor.Height == int32(tip.Height) && r.Cursor.Hash == tip.Hash {
		r.Cursor.CompleteToHeight = r.Cursor.Height
		r.Cursor.CompleteToHash = r.Cursor.Hash
	}
	if r.Anchor != nil && r.Target == nil && r.Anchor.Height < math.MaxInt32 && r.Anchor.Height < tip.Height {
		height := r.Anchor.Height + 1
		if _, ok := checked[height]; ok || len(checked) < MaxScanBlocks {
			b, h, e := fetch(height)
			if e != nil {
				return OwnedRecord{}, e
			}
			if b.Header.PrevBlock.String() != r.Anchor.Hash {
				return OwnedRecord{}, ErrUnavailable
			}
			r.Target = &Target{Height: height, Hash: h, Header: headerString(b.Header), ObservedAtMS: n, ObservedTip: tip}
			verifiedTarget = true
			b = nil
		}
	}
	if e = bracket(work, p, tip, checked); e != nil {
		return OwnedRecord{}, e
	}
	if r.Anchor != nil && !verifiedAnchor {
		return OwnedRecord{}, ErrUnavailable
	}
	r.Observation = ownedObservationFor(r, tip, n, verifiedAnchor && verifiedTarget)
	if r.Observation.CurrentFinality && r.Finality == nil {
		r.Finality = &Finality{Checkpoint: *r.Checkpoint, ObservedAtMS: n, ObservedTip: tip, TargetDescendants: tip.Height - r.Target.Height}
	}
	b := new(leveldb.Batch)
	for _, c := range oldCoins {
		b.Delete(coinKey('f', c.Outpoint))
	}
	for _, c := range coins {
		if e = put(b, coinKey('f', c.Outpoint), c, 512); e != nil {
			return OwnedRecord{}, e
		}
	}
	return s.saveAction(work, events, entries, session, gymaccount.View, r, next, b, stamp)
}
func ownedObservationFor(r OwnedRecord, tip Tip, n int64, verified bool) Observation {
	v := Observation{ObservedAtMS: n, ChainObservedAtMS: n, Tip: &tip, NodeState: "online"}
	if r.Target != nil && tip.Height >= r.Target.Height {
		v.TargetDescendants = tip.Height - r.Target.Height
	}
	v.CurrentFinality = verified && r.Checkpoint != nil && v.TargetDescendants >= 60 && len(r.Conflicts) == 0 && !r.ConflictOverflow
	return v
}
