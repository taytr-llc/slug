// Copyright (c) 2026 The SLUG developers. ISC license.
package gymseed

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/seednode"
	"github.com/taytr-llc/slug/txscript/v2"
	"github.com/taytr-llc/slug/wire/v2"
	"math"
)

func headerString(h wire.BlockHeader) string {
	var b bytes.Buffer
	_ = h.Serialize(&b)
	return hex.EncodeToString(b.Bytes())
}
func merklePath(txs []*wire.MsgTx, index uint32) []string {
	level := make([]chainhash.Hash, len(txs))
	for i, tx := range txs {
		level[i] = tx.TxHash()
	}
	path := []string{}
	for len(level) > 1 {
		sibling := index ^ 1
		if sibling >= uint32(len(level)) {
			sibling = index
		}
		path = append(path, level[sibling].String())
		next := make([]chainhash.Hash, (len(level)+1)/2)
		for i := range next {
			left := level[2*i]
			right := left
			if 2*i+1 < len(level) {
				right = level[2*i+1]
			}
			var pair [64]byte
			copy(pair[:32], left[:])
			copy(pair[32:], right[:])
			next[i] = chainhash.DoubleHashH(pair[:])
		}
		level = next
		index /= 2
	}
	return path
}
func anchorAt(block *wire.MsgBlock, height uint32, hash string, txIndex, outIndex uint32, payload [73]byte) (Anchor, error) {
	if block == nil || uint64(txIndex) >= uint64(len(block.Transactions)) {
		return Anchor{}, ErrInvalid
	}
	tx := block.Transactions[txIndex]
	if uint64(outIndex) >= uint64(len(tx.TxOut)) {
		return Anchor{}, ErrInvalid
	}
	want := append([]byte{txscript.OP_RETURN, 73}, payload[:]...)
	if !bytes.Equal(tx.TxOut[outIndex].PkScript, want) {
		return Anchor{}, ErrInvalid
	}
	var raw bytes.Buffer
	if tx.Serialize(&raw) != nil {
		return Anchor{}, ErrInvalid
	}
	digest := sha256.Sum256(raw.Bytes())
	return Anchor{Height: height, Hash: hash, Header: headerString(block.Header), TxID: tx.TxHash().String(), RawTxDigest: hex.EncodeToString(digest[:]), RawTxSize: uint32(raw.Len()), TxCount: uint32(len(block.Transactions)), TxIndex: txIndex, OutputIndex: outIndex, Payload: hex.EncodeToString(payload[:]), MerkleBranch: merklePath(block.Transactions, txIndex)}, nil
}
func firstAnchor(ctx context.Context, b *wire.MsgBlock, height uint32, hash string, payload [73]byte) (*Anchor, error) {
	want := append([]byte{txscript.OP_RETURN, 73}, payload[:]...)
	for i, tx := range b.Transactions {
		if ctx.Err() != nil {
			return nil, ErrCancelled
		}
		for j, out := range tx.TxOut {
			if bytes.Equal(out.PkScript, want) {
				a, e := anchorAt(b, height, hash, uint32(i), uint32(j), payload)
				return &a, e
			}
		}
	}
	return nil, nil
}
func advanceCursor(c *Cursor, height uint32, hashValue string, event bool) {
	domain := "SLUG candidate funding scan step\x00v1\x00"
	if event {
		domain = "SLUG candidate seed scan step\x00v1\x00"
	}
	c.ScanDigest = hash(domain, rawID(c.ScanDigest), le32(height), rawID(hashValue))
	c.Height = int32(height)
	c.Hash = hashValue
}
func canonicalAt(ctx context.Context, p *seednode.Publisher, tip Tip, height uint32) (string, error) {
	if height > tip.Height {
		return "", nil
	}
	return p.CanonicalHash(ctx, height)
}
func coinbase(tx *wire.MsgTx) bool {
	return len(tx.TxIn) == 1 && tx.TxIn[0].PreviousOutPoint.Hash == (chainhash.Hash{}) && tx.TxIn[0].PreviousOutPoint.Index == math.MaxUint32
}
func updateFunding(ctx context.Context, p Pin, coins map[string]FundingCoin, b *wire.MsgBlock, height uint32, hashValue string) error {
	for _, tx := range b.Transactions {
		if ctx.Err() != nil {
			return ErrCancelled
		}
		for _, in := range tx.TxIn {
			delete(coins, string(coinKey('f', Outpoint{TxID: in.PreviousOutPoint.Hash.String(), Index: in.PreviousOutPoint.Index})))
		}
		id := tx.TxHash().String()
		for i, out := range tx.TxOut {
			if bytes.Equal(out.PkScript, rawID(p.FundingScript)) {
				if out.Value <= 0 {
					continue
				}
				c := FundingCoin{Outpoint: Outpoint{TxID: id, Index: uint32(i)}, ValueSatoshis: out.Value, Script: p.FundingScript, BlockHeight: height, BlockHash: hashValue, Coinbase: coinbase(tx)}
				if validateFundingCoin(p, c) != nil {
					return ErrCorrupt
				}
				k := string(coinKey('f', c.Outpoint))
				if _, exists := coins[k]; !exists && len(coins) >= MaxFundingCoins {
					return ErrCapacity
				}
				coins[k] = c
			}
		}
	}
	return nil
}
func (s *Store) Reconcile(ctx context.Context, events *gymevent.Store, entries *gymentry.Store, session gymstable.Session, p *seednode.Publisher, date string) (result Record, resultErr error) {
	if p == nil {
		return Record{}, ErrNodeOffline
	}
	work, done, e := s.begin(ctx)
	if e != nil {
		return Record{}, e
	}
	defer done()
	r, e := s.actionRecord(work, events, entries, session, gymaccount.View, date)
	if e != nil {
		return Record{}, e
	}
	id := r.EventKey
	defer func() {
		if resultErr != nil {
			resultErr = s.failedChainAttempt(work, events, entries, session, gymaccount.View, id, resultErr)
		}
	}()
	if e = s.publisher(p); e != nil {
		return Record{}, e
	}
	if phase(r) == "conflict" {
		return s.replay(work, events, entries, session, gymaccount.View, r)
	}
	original := r
	stamp := func(r *Record, n int64) {
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
		return Record{}, mapped(e)
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
			return Record{}, mapped(e)
		}
		if h != ref.Hash {
			if e = bracket(work, p, tip, nil); e != nil {
				return Record{}, e
			}
			addConflict(&r, Conflict{Kind: "chain", ObservedAtMS: n, ExpectedHeight: ref.Height, ExpectedHash: ref.Hash, ObservedHash: h, ObservedTip: &tip})
			r.Observation = observationFor(r, tip, n, false)
			return s.saveAction(work, events, entries, session, gymaccount.View, r, s.meta, nil, stamp)
		}
	}
	oldCoins, e := s.funding(work)
	if e != nil {
		return Record{}, e
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
			return Record{}, mapped(e)
		}
		if h != next.FundingCursor.Hash {
			if len(next.FundingRestarts) >= 16 {
				return Record{}, ErrCapacity
			}
			g := next.FundingCursor.Generation + 1
			next.FundingRestarts = append(next.FundingRestarts, ScanRestart{Generation: g, Previous: next.FundingCursor, ObservedAtMS: n, ObservedTip: tip, ReplacementHash: h})
			next.FundingCursor = initialCursor(s.meta.Pin, "", g)
			next.FundingComplete = false
			coins = map[string]FundingCoin{}
			fundingRestarted = true
			restarted = true
		}
	}
	if r.Anchor == nil && r.Cursor.Height >= 0 {
		h, e := canonicalAt(work, p, tip, uint32(r.Cursor.Height))
		if e != nil {
			return Record{}, mapped(e)
		}
		if h != r.Cursor.Hash {
			if len(r.ScanRestarts) >= 16 {
				return Record{}, ErrCapacity
			}
			g := r.Cursor.Generation + 1
			r.ScanRestarts = append(r.ScanRestarts, ScanRestart{Generation: g, Previous: r.Cursor, ObservedAtMS: n, ObservedTip: tip, ReplacementHash: h})
			r.Cursor = initialCursor(s.meta.Pin, r.EventKey, g)
			restarted = true
		}
	}
	if restarted {
		if e = bracket(work, p, tip, nil); e != nil {
			return Record{}, e
		}
		r.Observation = observationFor(r, tip, n, false)
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
	payload, e := carrier(r.Declaration.Declaration)
	if e != nil {
		return Record{}, e
	}
	checked := map[uint32]string{}
	verifiedAnchor, verifiedTarget := false, false
	fetch := func(height uint32) (*wire.MsgBlock, string, error) {
		if _, ok := checked[height]; !ok && len(checked) >= MaxScanBlocks {
			return nil, "", ErrCapacity
		}
		h, e := p.CanonicalHash(work, height)
		if e != nil {
			return nil, "", mapped(e)
		}
		block, e := p.Block(work, height)
		if e != nil {
			return nil, "", mapped(e)
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
			return Record{}, e
		}
		a, e := anchorAt(b, r.Anchor.Height, h, r.Anchor.TxIndex, r.Anchor.OutputIndex, payload)
		if e != nil || !equal(a, *r.Anchor) {
			return Record{}, ErrUnavailable
		}
		verifiedAnchor = true
		b = nil
	}
	if r.Target != nil {
		b, h, e := fetch(r.Target.Height)
		if e != nil {
			return Record{}, e
		}
		if h != r.Target.Hash || headerString(b.Header) != r.Target.Header || b.Header.PrevBlock.String() != r.Anchor.Hash {
			return Record{}, ErrUnavailable
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
			return Record{}, e
		}
		if next.FundingCursor.Height+1 == int32(height) {
			if height > 0 && b.Header.PrevBlock.String() != next.FundingCursor.Hash {
				return Record{}, ErrUnavailable
			}
			if e = updateFunding(work, s.meta.Pin, coins, b, uint32(height), h); e != nil {
				return Record{}, e
			}
			advanceCursor(&next.FundingCursor, uint32(height), h, false)
		}
		if r.Anchor == nil && r.Cursor.Height+1 == int32(height) {
			if height > 0 && b.Header.PrevBlock.String() != r.Cursor.Hash {
				return Record{}, ErrUnavailable
			}
			a, e := firstAnchor(work, b, uint32(height), h, payload)
			if e != nil {
				return Record{}, e
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
				return Record{}, e
			}
			if b.Header.PrevBlock.String() != r.Anchor.Hash {
				return Record{}, ErrUnavailable
			}
			r.Target = &Target{Height: height, Hash: h, Header: headerString(b.Header), ObservedAtMS: n, ObservedTip: tip}
			verifiedTarget = true
			b = nil
		}
	}
	if e = bracket(work, p, tip, checked); e != nil {
		return Record{}, e
	}
	if r.Anchor != nil && !verifiedAnchor {
		return Record{}, ErrUnavailable
	}
	r.Observation = observationFor(r, tip, n, verifiedAnchor && verifiedTarget)
	if r.Observation.CurrentFinality && r.Finality == nil {
		r.Finality = &Finality{Checkpoint: *r.Checkpoint, ObservedAtMS: n, ObservedTip: tip, TargetDescendants: tip.Height - r.Target.Height}
	}
	b := new(leveldb.Batch)
	for _, c := range oldCoins {
		b.Delete(coinKey('f', c.Outpoint))
	}
	for _, c := range coins {
		if e = put(b, coinKey('f', c.Outpoint), c, 512); e != nil {
			return Record{}, e
		}
	}
	return s.saveAction(work, events, entries, session, gymaccount.View, r, next, b, stamp)
}
func anchorRef(a *Anchor) *seednode.HeaderRef {
	if a == nil {
		return nil
	}
	return &seednode.HeaderRef{Height: a.Height, Hash: a.Hash, Header: a.Header}
}
func targetRef(t *Target) *seednode.HeaderRef {
	if t == nil {
		return nil
	}
	return &seednode.HeaderRef{Height: t.Height, Hash: t.Hash, Header: t.Header}
}
func observationFor(r Record, tip Tip, n int64, verified bool) Observation {
	v := Observation{ObservedAtMS: n, ChainObservedAtMS: n, Tip: &tip, NodeState: "online"}
	if r.Target != nil && tip.Height >= r.Target.Height {
		v.TargetDescendants = tip.Height - r.Target.Height
	}
	v.CurrentFinality = verified && r.Checkpoint != nil && v.TargetDescendants >= 60 && len(r.Conflicts) == 0 && !r.ConflictOverflow
	return v
}
func bracket(ctx context.Context, p *seednode.Publisher, tip Tip, checks map[uint32]string) error {
	now, e := p.Observe(ctx)
	if e != nil {
		return mapped(e)
	}
	if now.Height < tip.Height {
		return ErrUnavailable
	}
	if h, e := p.CanonicalHash(ctx, tip.Height); e != nil || h != tip.Hash {
		return ErrUnavailable
	}
	for height, hash := range checks {
		if h, e := p.CanonicalHash(ctx, height); e != nil || h != hash {
			return ErrUnavailable
		}
	}
	return nil
}
