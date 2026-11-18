// Copyright (c) 2026 The SLUG developers. ISC license.
package gymresult

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymseed"
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
func coinbase(tx *wire.MsgTx) bool {
	return len(tx.TxIn) == 1 && tx.TxIn[0].PreviousOutPoint.Hash == (chainhash.Hash{}) && tx.TxIn[0].PreviousOutPoint.Index == math.MaxUint32
}
func updateFunding(ctx context.Context, p Pin, coins map[string]FundingCoin, b *wire.MsgBlock, height uint32, hashValue string) error {
	for _, tx := range b.Transactions {
		if ctx.Err() != nil {
			return ErrCancelled
		}
		for _, in := range tx.TxIn {
			delete(coins, string(coinKey('c', Outpoint{TxID: in.PreviousOutPoint.Hash.String(), Index: in.PreviousOutPoint.Index})))
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
				k := string(coinKey('c', c.Outpoint))
				if _, exists := coins[k]; !exists && len(coins) >= MaxFundingCoins {
					return ErrCapacity
				}
				coins[k] = c
			}
		}
	}
	return nil
}
func (s *Store) Publish(ctx context.Context, bouts *gymbout.Store, cards *gymcard.Store, seeds *gymseed.Store, events *gymevent.Store, entries *gymentry.Store, session gymstable.Session, saturday, boutID string, p *seednode.ResultPublisher) (ActionResult, error) {
	if p == nil {
		return ActionResult{}, ErrInvalid
	}
	c, done, e := s.call(sourceCall{ctx: ctx, bouts: bouts, cards: cards, seeds: seeds, events: events, entries: entries, session: session, date: saturday, id: boutID})
	if e != nil {
		return ActionResult{}, e
	}
	defer done()
	if e = s.publisher(p); e != nil {
		return ActionResult{}, e
	}
	return s.publish(c, p)
}
func (s *Store) Reconcile(ctx context.Context, bouts *gymbout.Store, cards *gymcard.Store, seeds *gymseed.Store, events *gymevent.Store, entries *gymentry.Store, session gymstable.Session, saturday, boutID string, p *seednode.ResultPublisher) (ActionResult, error) {
	if p == nil {
		return ActionResult{}, ErrInvalid
	}
	c, done, e := s.call(sourceCall{ctx: ctx, bouts: bouts, cards: cards, seeds: seeds, events: events, entries: entries, session: session, date: saturday, id: boutID})
	if e != nil {
		return ActionResult{}, e
	}
	defer done()
	if e = s.publisher(p); e != nil {
		return ActionResult{}, e
	}
	return s.reconcile(c, p)
}
func (s *Store) Attest(ctx context.Context, bouts *gymbout.Store, cards *gymcard.Store, seeds *gymseed.Store, events *gymevent.Store, entries *gymentry.Store, session gymstable.Session, saturday, boutID string, p *seednode.ResultPublisher) (ActionResult, error) {
	if p == nil {
		return ActionResult{}, ErrInvalid
	}
	c, done, e := s.call(sourceCall{ctx: ctx, bouts: bouts, cards: cards, seeds: seeds, events: events, entries: entries, session: session, date: saturday, id: boutID})
	if e != nil {
		return ActionResult{}, e
	}
	defer done()
	if e = s.publisher(p); e != nil {
		return ActionResult{}, e
	}
	return s.attest(c, p)
}
func (s *Store) publisher(p *seednode.ResultPublisher) error {
	if p == nil {
		return ErrInvalid
	}
	chain, e := seednode.ChainIdentityFor(s.meta.Pin.SourcePin.SourcePin.SourcePin.SourcePin)
	if e != nil {
		return ErrCorrupt
	}
	v := p.Identity()
	if v.Chain != chain || v.Purpose != "candidate_result_v1" || v.FundingPublicKey != s.meta.Pin.FundingPublicKey || v.FundingScript != s.meta.Pin.FundingScript {
		return ErrForbidden
	}
	return nil
}
func canonicalAt(ctx context.Context, p targetReader, tip Tip, height uint32) (string, error) {
	if height > tip.Height {
		return "", nil
	}
	return p.CanonicalHash(ctx, height)
}
func bracket(ctx context.Context, p targetReader, tip Tip, checks map[uint32]string) error {
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
	for h, want := range checks {
		if got, e := p.CanonicalHash(ctx, h); e != nil || got != want {
			return ErrUnavailable
		}
	}
	return nil
}
func addConflict(r *Record, c Conflict) {
	k := conflictKey(c)
	for _, v := range r.Conflicts {
		if conflictKey(v) == k {
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
func ownConflict(r Record) bool {
	return len(r.Conflicts) > 0 || r.ConflictOverflow || r.Equivocation != nil
}
func observationFor(r Record, tip Tip, n int64, verified bool) Observation {
	o := Observation{ObservedAtMS: n, ChainObservedAtMS: n, Tip: &tip, NodeState: "online"}
	if r.Inclusion != nil && tip.Height >= r.Inclusion.Anchor.Height {
		o.Descendants = tip.Height - r.Inclusion.Anchor.Height
	}
	o.CurrentFinality = verified && r.Checkpoint != nil && o.Descendants >= 60 && !ownConflict(r)
	return o
}
func chainStamp(old Record, oldMeta metadata) func(*Record, *metadata, int64) {
	return func(r *Record, m *metadata, n int64) {
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
func (s *Store) reconcile(c sourceCall, p observedChain) (ActionResult, error) {
	r, st, e := s.read(c, gymaccount.View, true)
	if e != nil {
		return ActionResult{}, e
	}
	old := r
	oldMeta := s.meta
	tip, e := p.Observe(c.ctx)
	if e != nil {
		return ActionResult{}, mapped(e)
	}
	n := max(s.now().UnixMilli(), s.meta.ClockMS, r.PreparedAtMS)
	next := s.meta
	next.FundingRestarts = append([]ScanRestart{}, next.FundingRestarts...)
	next.LastFundingTip = &tip
	src := r.Statement.Statement.Source.Source
	h, e := canonicalAt(c.ctx, p, tip, src.TargetHeight)
	if e != nil {
		return ActionResult{}, mapped(e)
	}
	if h != src.TargetHash {
		addConflict(&r, Conflict{Kind: "source_chain_conflict", ObservedAtMS: n, ExpectedHeight: src.TargetHeight, ExpectedHash: src.TargetHash, ObservedHash: h, ObservedTip: &tip})
	}
	restarted := false
	if r.Inclusion != nil {
		a := r.Inclusion.Anchor
		h, e := canonicalAt(c.ctx, p, tip, a.Height)
		if e != nil {
			return ActionResult{}, mapped(e)
		}
		if h != a.Hash {
			if r.Checkpoint != nil {
				addConflict(&r, Conflict{Kind: "checkpoint_chain_conflict", ObservedAtMS: n, ExpectedHeight: a.Height, ExpectedHash: a.Hash, ObservedHash: h, ObservedTip: &tip})
			} else {
				if len(r.PriorInclusions) >= 16 || len(r.ScanRestarts) >= 16 {
					return ActionResult{}, ErrCapacity
				}
				r.PriorInclusions = append(r.PriorInclusions, PriorInclusion{1, *r.Inclusion, n, tip, h})
				r.Inclusion = nil
				if r.Cursor.Height >= 0 {
					repl, e := canonicalAt(c.ctx, p, tip, uint32(r.Cursor.Height))
					if e != nil {
						return ActionResult{}, mapped(e)
					}
					g := r.Cursor.Generation + 1
					r.ScanRestarts = append(r.ScanRestarts, ScanRestart{Generation: g, Previous: r.Cursor, ObservedAtMS: n, ObservedTip: tip, ReplacementHash: repl})
					r.Cursor = initialCursor(s.meta.Pin, r.ResultKey, g)
				}
				restarted = true
			}
		}
	}
	oldCoins, e := s.funding(c.ctx)
	if e != nil {
		return ActionResult{}, e
	}
	coins := map[string]FundingCoin{}
	for _, x := range oldCoins {
		coins[string(coinKey('c', x.Outpoint))] = x
	}
	if next.FundingCursor.Height >= 0 {
		h, e := canonicalAt(c.ctx, p, tip, uint32(next.FundingCursor.Height))
		if e != nil {
			return ActionResult{}, mapped(e)
		}
		if h != next.FundingCursor.Hash {
			if len(next.FundingRestarts) >= 16 {
				return ActionResult{}, ErrCapacity
			}
			g := next.FundingCursor.Generation + 1
			next.FundingRestarts = append(next.FundingRestarts, ScanRestart{Generation: g, Previous: next.FundingCursor, ObservedAtMS: n, ObservedTip: tip, ReplacementHash: h})
			next.FundingCursor = initialCursor(s.meta.Pin, "", g)
			next.FundingComplete = false
			coins = map[string]FundingCoin{}
			restarted = true
		}
	}
	if r.Publication != nil && r.Inclusion == nil && r.Cursor.Height >= 0 && !restarted {
		h, e := canonicalAt(c.ctx, p, tip, uint32(r.Cursor.Height))
		if e != nil {
			return ActionResult{}, mapped(e)
		}
		if h != r.Cursor.Hash {
			if len(r.ScanRestarts) >= 16 {
				return ActionResult{}, ErrCapacity
			}
			g := r.Cursor.Generation + 1
			r.ScanRestarts = append(r.ScanRestarts, ScanRestart{Generation: g, Previous: r.Cursor, ObservedAtMS: n, ObservedTip: tip, ReplacementHash: h})
			r.Cursor = initialCursor(s.meta.Pin, r.ResultKey, g)
			restarted = true
		}
	}
	checked := map[uint32]string{}
	payload, _ := Carrier(r.Statement.Statement)
	verified := false
	fetch := func(height uint32) (*wire.MsgBlock, string, error) {
		if _, ok := checked[height]; !ok && len(checked) >= 8 {
			return nil, "", ErrCapacity
		}
		h, e := p.CanonicalHash(c.ctx, height)
		if e != nil {
			return nil, "", mapped(e)
		}
		b, e := p.Block(c.ctx, height)
		if e != nil {
			return nil, "", mapped(e)
		}
		again, e := p.CanonicalHash(c.ctx, height)
		if e != nil || again != h {
			return nil, "", ErrUnavailable
		}
		checked[height] = h
		return b, h, nil
	}
	if r.Inclusion != nil && !ownConflict(r) {
		a := r.Inclusion.Anchor
		b, h, e := fetch(a.Height)
		if e != nil {
			return ActionResult{}, e
		}
		got, e := anchorAt(b, a.Height, h, a.TxIndex, 0, payload)
		if e != nil || !equal(got, a) {
			return ActionResult{}, ErrUnavailable
		}
		verified = true
	}
	if !restarted && !ownConflict(r) {
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
				return ActionResult{}, e
			}
			if height == 0 && h != s.meta.Pin.Genesis {
				return ActionResult{}, ErrUnavailable
			}
			if next.FundingCursor.Height+1 == int32(height) {
				if height > 0 && b.Header.PrevBlock.String() != next.FundingCursor.Hash {
					return ActionResult{}, ErrUnavailable
				}
				if e = updateFunding(c.ctx, s.meta.Pin, coins, b, uint32(height), h); e != nil {
					return ActionResult{}, e
				}
				next.FundingCursor.ScanDigest = stepDigest(next.FundingCursor.ScanDigest, uint32(height), h)
				next.FundingCursor.Height = int32(height)
				next.FundingCursor.Hash = h
			}
			if r.Publication != nil && r.Inclusion == nil && r.Cursor.Height+1 == int32(height) {
				if height > 0 && b.Header.PrevBlock.String() != r.Cursor.Hash {
					return ActionResult{}, ErrUnavailable
				}
				r.Cursor.ScanDigest = stepDigest(r.Cursor.ScanDigest, uint32(height), h)
				r.Cursor.Height = int32(height)
				r.Cursor.Hash = h
				for i, tx := range b.Transactions {
					if c.ctx.Err() != nil {
						return ActionResult{}, ErrCancelled
					}
					if tx.TxHash().String() != r.Publication.TxID {
						continue
					}
					var raw bytes.Buffer
					if tx.Serialize(&raw) != nil {
						return ActionResult{}, ErrCorrupt
					}
					if !bytes.Equal(raw.Bytes(), rawID(r.Publication.RawTx)) {
						continue
					}
					a, e := anchorAt(b, uint32(height), h, uint32(i), 0, payload)
					if e != nil {
						return ActionResult{}, e
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
		return ActionResult{}, e
	}
	r.Observation = observationFor(r, tip, n, verified && st.Source.Saved.Usable)
	if r.Observation.CurrentFinality && r.Finality == nil {
		r.Finality = &Finality{*r.Checkpoint, *r.Inclusion, n, tip, r.Observation.Descendants}
	}
	b := new(leveldb.Batch)
	for _, x := range oldCoins {
		b.Delete(coinKey('c', x.Outpoint))
	}
	for _, x := range coins {
		if e = put(b, coinKey('c', x.Outpoint), x, 1024); e != nil {
			return ActionResult{}, e
		}
	}
	return s.save(c, gymaccount.View, r, next, b, chainStamp(old, oldMeta), false)
}
func (s *Store) publish(c sourceCall, p publicationNetwork) (ActionResult, error) {
	r, st, e := s.read(c, gymaccount.ResultPublish, true)
	if e != nil {
		return ActionResult{}, e
	}
	if !st.Source.Saved.Usable {
		return ActionResult{}, ErrSourceConflict
	}
	if ownConflict(r) {
		return ActionResult{}, ErrConflict
	}
	tip, e := s.checkSavedTarget(c, r, p)
	if e != nil {
		return ActionResult{}, e
	}
	if r.Inclusion != nil {
		row, current, e := s.read(c, gymaccount.ResultPublish, true)
		if e != nil {
			return ActionResult{}, e
		}
		if !current.Source.Saved.Usable {
			return ActionResult{}, ErrSourceConflict
		}
		return ActionResult{1, row, current}, nil
	}
	payload, _ := Carrier(r.Statement.Statement)
	if r.Publication == nil {
		if !s.meta.FundingComplete || s.meta.FundingCursor.Height != int32(tip.Height) || s.meta.FundingCursor.Hash != tip.Hash {
			return ActionResult{}, ErrScanIncomplete
		}
		coins, e := s.funding(c.ctx)
		if e != nil {
			return ActionResult{}, e
		}
		var selected *FundingCoin
		for _, coin := range coins {
			_, reserved, e := s.raw(coinKey('u', coin.Outpoint), 256)
			if e != nil {
				return ActionResult{}, e
			}
			if reserved || coin.ValueSatoshis < 1294 || !mature(coin, tip) {
				continue
			}
			h, e := p.CanonicalHash(c.ctx, coin.BlockHeight)
			if e != nil {
				return ActionResult{}, mapped(e)
			}
			if h != coin.BlockHash {
				return ActionResult{}, ErrScanIncomplete
			}
			unspent, e := p.Unspent(c.ctx, coin.Outpoint)
			if e != nil {
				return ActionResult{}, mapped(e)
			}
			if unspent == nil {
				continue
			}
			if unspent.Script != coin.Script || unspent.ValueSatoshis != coin.ValueSatoshis || unspent.Coinbase != coin.Coinbase || unspent.Confirmations == 0 {
				return ActionResult{}, ErrUnavailable
			}
			x := coin
			selected = &x
			break
		}
		if selected == nil {
			return ActionResult{}, ErrWaitingForFunds
		}
		raw, e := p.BuildPublication(c.ctx, payload, *selected)
		if e != nil {
			return ActionResult{}, mapped(e)
		}
		if len(raw) == 0 || len(raw) > MaxRawTxSize {
			return ActionResult{}, ErrInvalid
		}
		var tx wire.MsgTx
		if tx.Deserialize(bytes.NewReader(raw)) != nil {
			return ActionResult{}, ErrInvalid
		}
		if _, e = s.checkSavedTarget(c, r, p); e != nil {
			return ActionResult{}, e
		}
		n := max(s.now().UnixMilli(), s.meta.ClockMS)
		r.Publication = &Publication{Version: 1, RawTx: hex.EncodeToString(raw), TxID: tx.TxHash().String(), Coin: *selected, BuiltAtMS: n, State: "built"}
		r.Observation = observationFor(r, tip, n, false)
		if e = bracket(c.ctx, p, tip, nil); e != nil {
			return ActionResult{}, e
		}
		b := new(leveldb.Batch)
		if e = put(b, coinKey('u', selected.Outpoint), reservation{1, r.ResultKey, r.Publication.TxID}, 256); e != nil {
			return ActionResult{}, e
		}
		out, e := s.save(c, gymaccount.ResultPublish, r, s.meta, b, func(r *Record, _ *metadata, n int64) {
			r.Publication.BuiltAtMS = n
			r.Observation.ObservedAtMS = n
			r.Observation.ChainObservedAtMS = n
		}, true)
		if e != nil {
			return ActionResult{}, e
		}
		r = out.Record
		s.mark("transaction-retained")
	}
	if r.Publication.DispatchCount == math.MaxUint32 {
		return ActionResult{}, ErrCapacity
	}
	// An exact retry preserves the one retained input/raw transaction. It does not
	// re-sign even when the retained valid signature uses another nonce.
	unspent, e := p.Unspent(c.ctx, r.Publication.Coin.Outpoint)
	if e != nil || unspent == nil {
		if e != nil {
			return ActionResult{}, mapped(e)
		}
		return ActionResult{}, ErrUnavailable
	}
	coin := r.Publication.Coin
	if unspent.Script != coin.Script || unspent.ValueSatoshis != coin.ValueSatoshis || unspent.Coinbase != coin.Coinbase || unspent.Confirmations == 0 {
		return ActionResult{}, ErrUnavailable
	}
	freshTip, e := s.checkSavedTarget(c, r, p)
	if e != nil {
		return ActionResult{}, e
	}
	if !mature(coin, freshTip) {
		return ActionResult{}, ErrNotMature
	}
	if h, e := p.CanonicalHash(c.ctx, coin.BlockHeight); e != nil {
		return ActionResult{}, mapped(e)
	} else if h != coin.BlockHash {
		return ActionResult{}, ErrUnavailable
	}
	if e = bracket(c.ctx, p, freshTip, map[uint32]string{coin.BlockHeight: coin.BlockHash}); e != nil {
		return ActionResult{}, e
	}
	n := max(s.now().UnixMilli(), s.meta.ClockMS)
	r.Publication.DispatchCount++
	r.Publication.State = "dispatch_unknown"
	if r.Publication.FirstDispatchAtMS == 0 {
		r.Publication.FirstDispatchAtMS = n
	}
	r.Publication.LastDispatchAtMS = n
	r.Observation.ObservedAtMS = n
	out, e := s.save(c, gymaccount.ResultPublish, r, s.meta, nil, func(r *Record, _ *metadata, n int64) {
		if r.Publication.DispatchCount == 1 {
			r.Publication.FirstDispatchAtMS = n
		}
		r.Publication.LastDispatchAtMS = n
		r.Observation.ObservedAtMS = n
	}, true)
	if e != nil {
		return ActionResult{}, e
	}
	r = out.Record
	s.mark("dispatch-armed")
	if c.ctx.Err() != nil || s.life.Err() != nil {
		return ActionResult{}, ErrOutcomeUnknown
	}
	txid, e := p.SubmitPublication(c.ctx, payload, r.Publication.Coin, rawID(r.Publication.RawTx))
	if e != nil || txid != r.Publication.TxID {
		return ActionResult{}, ErrOutcomeUnknown
	}
	s.mark("dispatch-acknowledged")
	if _, e = s.checkSavedTarget(c, r, p); e != nil {
		return ActionResult{}, ErrOutcomeUnknown
	}
	r.Publication.State = "submitted"
	out, e = s.save(c, gymaccount.ResultPublish, r, s.meta, nil, func(r *Record, _ *metadata, n int64) { r.Observation.ObservedAtMS = n }, true)
	if e != nil {
		return ActionResult{}, ErrOutcomeUnknown
	}
	return out, nil
}
func (s *Store) attest(c sourceCall, p attestationNetwork) (ActionResult, error) {
	r, st, e := s.read(c, gymaccount.ResultPublish, true)
	if e != nil {
		return ActionResult{}, e
	}
	if !st.Source.Saved.Usable {
		return ActionResult{}, ErrSourceConflict
	}
	if ownConflict(r) {
		return ActionResult{}, ErrConflict
	}
	if _, e = s.checkSavedTarget(c, r, p); e != nil {
		return ActionResult{}, e
	}
	if r.Inclusion == nil {
		return ActionResult{}, ErrNotMature
	}
	if r.Finality != nil {
		row, current, e := s.read(c, gymaccount.ResultPublish, true)
		if e != nil {
			return ActionResult{}, e
		}
		if !current.Source.Saved.Usable {
			return ActionResult{}, ErrSourceConflict
		}
		return ActionResult{1, row, current}, nil
	}
	cp, tip, e := p.AttestInclusion(c.ctx, r.Inclusion.Anchor)
	if e != nil {
		return ActionResult{}, mapped(e)
	}
	if !validCheckpoint(s.meta.Pin, cp) || cp.Height != int32(r.Inclusion.Anchor.Height) || cp.Hash.String() != r.Inclusion.Anchor.Hash || tip.Height < r.Inclusion.Anchor.Height || tip.Height-r.Inclusion.Anchor.Height < 60 {
		return ActionResult{}, ErrUnavailable
	}
	if _, e = s.checkSavedTarget(c, r, p); e != nil {
		return ActionResult{}, e
	}
	src := r.Statement.Statement.Source.Source
	if e = bracket(c.ctx, p, tip, map[uint32]string{src.TargetHeight: src.TargetHash, r.Inclusion.Anchor.Height: r.Inclusion.Anchor.Hash}); e != nil {
		return ActionResult{}, e
	}
	old := r
	n := max(s.now().UnixMilli(), s.meta.ClockMS)
	if r.Checkpoint == nil {
		r.Checkpoint = &cp
	}
	r.Finality = &Finality{*r.Checkpoint, *r.Inclusion, n, tip, tip.Height - r.Inclusion.Anchor.Height}
	r.Observation = observationFor(r, tip, n, true)
	return s.save(c, gymaccount.ResultPublish, r, s.meta, nil, chainStamp(old, s.meta), true)
}

// The exported action admits only the concrete owned publisher. This private
// read boundary keeps target observation and its evidence commit testable without
// introducing an imported authority or a public callback capability.
// observedChain is private to the journal algorithm. Public entry points still
// require and pin the concrete, owned ResultPublisher before invoking it.
type publicationNetwork interface {
	targetReader
	Unspent(context.Context, Outpoint) (*seednode.UnspentOutput, error)
	BuildPublication(context.Context, [73]byte, FundingCoin) ([]byte, error)
	SubmitPublication(context.Context, [73]byte, FundingCoin, []byte) (string, error)
}
type attestationNetwork interface {
	targetReader
	AttestInclusion(context.Context, Anchor) (chaincfg.SignedCheckpoint, Tip, error)
}
type observedChain interface {
	targetReader
	Block(context.Context, uint32) (*wire.MsgBlock, error)
}
type targetReader interface {
	Observe(context.Context) (Tip, error)
	CanonicalHash(context.Context, uint32) (string, error)
}

func (s *Store) checkSavedTarget(c sourceCall, r Record, p targetReader) (Tip, error) {
	tip, e := p.Observe(c.ctx)
	if e != nil {
		return Tip{}, mapped(e)
	}
	src := r.Statement.Statement.Source.Source
	observed := ""
	if src.TargetHeight <= tip.Height {
		observed, e = p.CanonicalHash(c.ctx, src.TargetHeight)
		if e != nil {
			return Tip{}, mapped(e)
		}
	}
	// A failed captured-tip bracket is unavailable evidence, never a conflict.
	current, e := p.Observe(c.ctx)
	if e != nil {
		return Tip{}, mapped(e)
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
			return Tip{}, mapped(e)
		}
	}
	if again != observed {
		return Tip{}, ErrUnavailable
	}
	// Conflict evidence may retain only the durable row. Tentative signing work
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
	addConflict(&r, Conflict{Kind: "source_chain_conflict", ObservedAtMS: n, ExpectedHeight: src.TargetHeight, ExpectedHash: src.TargetHash, ObservedHash: observed, ObservedTip: &tip})
	r.Observation = observationFor(r, tip, n, false)
	if _, e = s.save(c, gymaccount.View, r, s.meta, nil, chainStamp(old, s.meta), false); e != nil {
		return Tip{}, e
	}
	return Tip{}, ErrSourceConflict
}
