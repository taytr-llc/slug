// Copyright (c) 2026 The SLUG developers. ISC license.
package gymseed

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/seednode"
	"github.com/taytr-llc/slug/wire/v2"
	"math"
)

func (s *Store) authorize(ctx context.Context, events *gymevent.Store, entries *gymentry.Store, session gymstable.Session, cap gymaccount.Capability) error {
	return s.withSource(ctx, events, entries, session, cap, func(lower int64, check func() error) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		if e := check(); e != nil {
			return e
		}
		_, e := s.observeLocked(lower)
		return e
	})
}
func (s *Store) actionRecord(ctx context.Context, events *gymevent.Store, entries *gymentry.Store, session gymstable.Session, cap gymaccount.Capability, date string) (Record, error) {
	if e := s.authorize(ctx, events, entries, session, cap); e != nil {
		return Record{}, e
	}
	ev, e := gymevent.EventForDate(s.meta.Pin.SourcePin, date)
	if e != nil {
		return Record{}, ErrCalendar
	}
	r, ok, e := s.record(eventKey(s.meta.Pin, ev))
	if e != nil {
		return r, e
	}
	if !ok {
		return r, ErrNotPrepared
	}
	if r.IncidentAtMS == 0 && due(r, s.meta.ClockMS) {
		return s.saveAction(ctx, events, entries, session, cap, r, s.meta, nil, nil)
	}
	return r, nil
}
func (s *Store) publisher(p *seednode.Publisher) error {
	if p == nil {
		return ErrNodeOffline
	}
	want, e := seednode.ChainIdentityFor(s.meta.Pin.SourcePin)
	if e != nil || p.Identity() != want {
		return ErrForbidden
	}
	return nil
}

// A failed explicit chain attempt does not erase a previously coherent chain
// observation. It is journaled only if the original account still authorizes it.
// No observation is invented for a rejected capability or wrong publisher.
func (s *Store) failedChainAttempt(ctx context.Context, events *gymevent.Store, entries *gymentry.Store, session gymstable.Session, cap gymaccount.Capability, id string, cause error) error {
	if !errors.Is(cause, ErrNodeOffline) && !errors.Is(cause, ErrUnavailable) {
		return cause
	}
	r, ok, e := s.record(id)
	if e != nil || !ok {
		return cause
	}
	state := "unavailable"
	if errors.Is(cause, ErrNodeOffline) {
		state = "offline"
	}
	r.Observation.NodeState = state
	r.Observation.ObservedAtMS = s.now().UnixMilli()
	_, e = s.saveAction(ctx, events, entries, session, cap, r, s.meta, nil, func(r *Record, n int64) { r.Observation.ObservedAtMS = n })
	if e != nil {
		return e
	}
	return cause
}

// saveAction assumes the resource gate is held. All public semantic work is
// complete before the original-account lease; its callback only stamps/copies,
// encodes the bounded prepared batch and synchronizes it.
func (s *Store) saveAction(ctx context.Context, events *gymevent.Store, entries *gymentry.Store, session gymstable.Session, cap gymaccount.Capability, r Record, next metadata, b *leveldb.Batch, stamp func(*Record, int64)) (Record, error) {
	if validateRecord(s.meta.Pin, r) != nil || !fits(r, MaxRecordSize) {
		return Record{}, ErrInvalid
	}
	old, oldExists, loadErr := s.record(r.EventKey)
	if loadErr != nil {
		return Record{}, loadErr
	}
	if b == nil {
		b = new(leveldb.Batch)
	}
	committed := false
	e := s.withSource(ctx, events, entries, session, cap, func(lower int64, check func() error) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		if e := check(); e != nil {
			return e
		}
		if t := recordMaxTime(r); t > lower {
			lower = t
		}
		n, e := s.sampleLocked(lower)
		if e != nil {
			return e
		}
		if stamp != nil {
			stamp(&r, n)
		}
		if r.IncidentAtMS == 0 && (due(r, n) || oldExists && due(old, n)) {
			r.IncidentAtMS = n
		}
		next.ClockMS = n
		if e = put(b, key('r', r.EventKey), r, MaxRecordSize); e != nil {
			return e
		}
		s.mark("before-record-sync")
		e = s.commitLocked(b, next)
		if e != nil {
			return e
		}
		committed = true
		s.mark("record-synced")
		return nil
	})
	if e != nil || ctx.Err() != nil {
		if committed {
			return Record{}, ErrOutcomeUnknown
		}
		if e == nil {
			e = ErrCancelled
		}
		return Record{}, e
	}
	return r, nil
}
func (s *Store) replay(ctx context.Context, events *gymevent.Store, entries *gymentry.Store, session gymstable.Session, cap gymaccount.Capability, r Record) (Record, error) {
	return s.saveAction(ctx, events, entries, session, cap, r, s.meta, nil, nil)
}
func (s *Store) Prepare(ctx context.Context, events *gymevent.Store, entries *gymentry.Store, session gymstable.Session, date string) (Record, error) {
	work, done, e := s.begin(ctx)
	if e != nil {
		return Record{}, e
	}
	defer done()
	if e = s.authorize(work, events, entries, session, gymaccount.View); e != nil {
		return Record{}, e
	}
	ev, e := gymevent.EventForDate(s.meta.Pin.SourcePin, date)
	if e != nil {
		return Record{}, ErrCalendar
	}
	id := eventKey(s.meta.Pin, ev)
	if r, ok, e := s.record(id); e != nil {
		return Record{}, e
	} else if ok {
		return s.replay(work, events, entries, session, gymaccount.View, r)
	}
	if s.meta.CreatedAtMS >= ev.CutoffMS {
		return Record{}, ErrCreatedTooLate
	}
	if s.meta.EventCount >= MaxEvents {
		return Record{}, ErrCapacity
	}
	var out Record
	committed := false
	e = events.WithSeedSnapshot(work, entries, session, date, func(receipt gymevent.SnapshotReceipt) (func(gymevent.SeedSourceInfo, func() error) error, error) {
		declaration, e := declarationFor(s.meta.Pin, receipt)
		if e != nil {
			return nil, e
		}
		n := s.now().UnixMilli()
		if n < receipt.CompletedAtMS {
			n = receipt.CompletedAtMS
		}
		r := Record{Version: 1, StoreID: s.meta.StoreID, EventKey: id, SourceReceipt: receipt, Declaration: declaration, PreparedAtMS: n, Cursor: initialCursor(s.meta.Pin, id, 0), ScanRestarts: []ScanRestart{}, Conflicts: []Conflict{}, Observation: Observation{NodeState: "unobserved"}}
		if validateRecord(s.meta.Pin, r) != nil || !fits(r, MaxRecordSize) {
			return nil, ErrInvalid
		}
		return func(info gymevent.SeedSourceInfo, check func() error) error {
			if info.Version != 1 || info.StoreID != s.meta.Pin.SourceStoreID || !equal(info.Pin, s.meta.Pin.SourcePin) || check == nil {
				return ErrForbidden
			}
			s.mu.Lock()
			defer s.mu.Unlock()
			if e := check(); e != nil {
				return e
			}
			lower := info.ObservedAtMS
			if receipt.CompletedAtMS > lower {
				lower = receipt.CompletedAtMS
			}
			now, e := s.sampleLocked(lower)
			if e != nil {
				return e
			}
			r.PreparedAtMS = now
			if due(r, now) {
				r.IncidentAtMS = now
			}
			next := s.meta
			next.EventCount++
			next.ClockMS = now
			b := new(leveldb.Batch)
			if e = put(b, key('r', id), r, MaxRecordSize); e != nil {
				return e
			}
			s.mark("before-prepare")
			if e = s.commitLocked(b, next); e != nil {
				return e
			}
			committed = true
			out = r
			s.mark("prepared")
			return nil
		}, nil
	})
	if e != nil || work.Err() != nil {
		if committed {
			return Record{}, ErrOutcomeUnknown
		}
		if e == nil {
			e = ErrCancelled
		}
		return Record{}, mapped(e)
	}
	return out, nil
}
func (s *Store) Publish(ctx context.Context, events *gymevent.Store, entries *gymentry.Store, session gymstable.Session, p *seednode.Publisher, date string) (result Record, resultErr error) {
	if p == nil {
		return Record{}, ErrNodeOffline
	}
	work, done, e := s.begin(ctx)
	if e != nil {
		return Record{}, e
	}
	defer done()
	r, e := s.actionRecord(work, events, entries, session, gymaccount.SeedPublish, date)
	if e != nil {
		return Record{}, e
	}
	defer func() {
		if resultErr != nil {
			resultErr = s.failedChainAttempt(work, events, entries, session, gymaccount.SeedPublish, r.EventKey, resultErr)
		}
	}()
	if e = s.publisher(p); e != nil {
		return Record{}, e
	}
	if phase(r) == "conflict" {
		return Record{}, ErrConflict
	}
	if r.Anchor != nil {
		return s.replay(work, events, entries, session, gymaccount.SeedPublish, r)
	}
	payload, e := carrier(r.Declaration.Declaration)
	if e != nil {
		return Record{}, e
	}
	changed := false
	if r.Publication == nil {
		tip, e := p.Observe(work)
		if e != nil {
			return Record{}, mapped(e)
		}
		if !s.meta.FundingComplete || s.meta.FundingCursor.Height != int32(tip.Height) || s.meta.FundingCursor.Hash != tip.Hash {
			return Record{}, ErrScanIncomplete
		}
		coins, e := s.funding(work)
		if e != nil {
			return Record{}, e
		}
		var coin *FundingCoin
		for _, c := range coins {
			reserved, e := s.db.Has(coinKey('u', c.Outpoint), nil)
			if e != nil {
				return Record{}, ErrCorrupt
			}
			if reserved || c.ValueSatoshis < 1294 || c.BlockHeight > tip.Height || c.Coinbase && int64(tip.Height)+1-int64(c.BlockHeight) < 100 {
				continue
			}
			// Inventory reflects confirmed chain history; a mempool spend may
			// already consume its first coin. Selection is still unsealed here.
			unspent, e := p.Unspent(work, c.Outpoint)
			if e != nil {
				return Record{}, mapped(e)
			}
			if unspent == nil {
				continue
			}
			if unspent.Script != c.Script || unspent.ValueSatoshis != c.ValueSatoshis || unspent.Coinbase != c.Coinbase || unspent.Confirmations == 0 {
				return Record{}, ErrUnavailable
			}
			copy := c
			coin = &copy
			break
		}
		if coin == nil {
			return Record{}, ErrWaitingForFunds
		}
		raw, e := p.BuildPublication(work, payload, *coin)
		if e != nil {
			return Record{}, mapped(e)
		}
		if len(raw) == 0 || len(raw) > MaxRawTxSize {
			return Record{}, ErrInvalid
		}
		var tx wire.MsgTx
		if tx.Deserialize(bytes.NewReader(raw)) != nil {
			return Record{}, ErrInvalid
		}
		n := s.now().UnixMilli()
		r.Publication = &Publication{Version: 1, RawTx: hex.EncodeToString(raw), TxID: tx.TxHash().String(), Coin: *coin, BuiltAtMS: n, State: "retained"}
		if validatePublication(s.meta.Pin, *r.Publication) != nil {
			return Record{}, ErrInvalid
		}
		if h, e := p.CanonicalHash(work, tip.Height); e != nil || h != tip.Hash {
			return Record{}, ErrUnavailable
		}
		if _, e = p.Observe(work); e != nil {
			return Record{}, mapped(e)
		}
		r.Observation = observationFor(r, tip, n, false)
		b := new(leveldb.Batch)
		b.Put(coinKey('u', coin.Outpoint), rawID(r.EventKey))
		r, e = s.saveAction(work, events, entries, session, gymaccount.SeedPublish, r, s.meta, b, func(r *Record, n int64) {
			r.Publication.BuiltAtMS = n
			r.Observation.ObservedAtMS = n
			r.Observation.ChainObservedAtMS = n
		})
		if e != nil {
			return Record{}, e
		}
		changed = true
		s.mark("transaction-retained")
	}
	// No signer/RPC runs inside the final account lease. The adapter rechecks
	// this same fixed coin/raw transaction immediately before its one dispatch.
	if r.Publication.DispatchCount == math.MaxUint32 {
		return Record{}, ErrCapacity
	}
	n := s.now().UnixMilli()
	out, e := p.Unspent(work, r.Publication.Coin.Outpoint)
	if e != nil || out == nil {
		if changed {
			return Record{}, ErrOutcomeUnknown
		}
		if e != nil {
			return Record{}, mapped(e)
		}
		return Record{}, ErrUnavailable
	}
	if out.Script != r.Publication.Coin.Script || out.ValueSatoshis != r.Publication.Coin.ValueSatoshis || out.Coinbase != r.Publication.Coin.Coinbase {
		if changed {
			return Record{}, ErrOutcomeUnknown
		}
		return Record{}, ErrUnavailable
	}
	r.Publication.DispatchCount++
	r.Publication.State = "unknown"
	if r.Publication.FirstDispatchAtMS == 0 {
		r.Publication.FirstDispatchAtMS = n
	}
	r.Publication.LastDispatchAtMS = n
	r, e = s.saveAction(work, events, entries, session, gymaccount.SeedPublish, r, s.meta, nil, func(r *Record, n int64) {
		if r.Publication.DispatchCount == 1 {
			r.Publication.FirstDispatchAtMS = n
		}
		r.Publication.LastDispatchAtMS = n
	})
	if e != nil {
		if changed {
			return Record{}, ErrOutcomeUnknown
		}
		return Record{}, e
	}
	s.mark("dispatch-armed")
	if work.Err() != nil {
		return Record{}, ErrOutcomeUnknown
	}
	txid, e := p.SubmitPublication(work, payload, r.Publication.Coin, rawID(r.Publication.RawTx))
	if e != nil || txid != r.Publication.TxID {
		return Record{}, ErrOutcomeUnknown
	}
	s.mark("dispatch-acknowledged")
	r.Publication.State = "submitted"
	r, e = s.saveAction(work, events, entries, session, gymaccount.SeedPublish, r, s.meta, nil, nil)
	if e != nil {
		return Record{}, ErrOutcomeUnknown
	}
	return r, nil
}
func addConflict(r *Record, c Conflict) {
	k := conflictKey(c)
	for _, old := range r.Conflicts {
		if conflictKey(old) == k {
			return
		}
	}
	if len(r.Conflicts) < MaxConflicts {
		r.Conflicts = append(r.Conflicts, c)
	} else {
		r.ConflictOverflow = true
	}
	r.Observation.CurrentFinality = false
}
func (s *Store) ObserveCheckpoint(ctx context.Context, events *gymevent.Store, entries *gymentry.Store, session gymstable.Session, date string, c chaincfg.SignedCheckpoint) (Record, error) {
	if len(c.Signature) != 64 || !validCheckpoint(safePin(s), c) {
		return Record{}, ErrInvalid
	}
	c.Signature = append([]byte(nil), c.Signature...)
	work, done, e := s.begin(ctx)
	if e != nil {
		return Record{}, e
	}
	defer done()
	r, e := s.actionRecord(work, events, entries, session, gymaccount.View, date)
	if e != nil {
		return Record{}, e
	}
	if r.Target == nil {
		return Record{}, ErrTargetUnavailable
	}
	if c.Height != int32(r.Target.Height) {
		return Record{}, ErrInvalid
	}
	n := s.now().UnixMilli()
	if c.Hash.String() != r.Target.Hash {
		addConflict(&r, Conflict{Kind: "checkpoint", ObservedAtMS: n, Checkpoint: &c})
	} else if r.Checkpoint == nil {
		r.Checkpoint = &c
	}
	return s.saveAction(work, events, entries, session, gymaccount.View, r, s.meta, nil, nil)
}
func safePin(s *Store) Pin {
	if s == nil {
		return Pin{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.meta.Pin
}
func (s *Store) ObserveEquivocation(ctx context.Context, events *gymevent.Store, entries *gymentry.Store, session gymstable.Session, date string, v SignedDeclaration) (Record, error) {
	p := safePin(s)
	v, e := ValidateSignedDeclaration(p, v)
	if e != nil {
		return Record{}, e
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
	if v.Declaration.EventKey != r.EventKey {
		return Record{}, ErrInvalid
	}
	if !equal(v.Declaration, r.Declaration.Declaration) {
		addConflict(&r, Conflict{Kind: "declaration", ObservedAtMS: s.now().UnixMilli(), Declaration: &v})
	}
	return s.saveAction(work, events, entries, session, gymaccount.View, r, s.meta, nil, nil)
}
func (s *Store) Attest(ctx context.Context, events *gymevent.Store, entries *gymentry.Store, session gymstable.Session, p *seednode.Publisher, date string) (result Record, resultErr error) {
	if p == nil {
		return Record{}, ErrNodeOffline
	}
	work, done, e := s.begin(ctx)
	if e != nil {
		return Record{}, e
	}
	defer done()
	r, e := s.actionRecord(work, events, entries, session, gymaccount.SeedPublish, date)
	if e != nil {
		return Record{}, e
	}
	defer func() {
		if resultErr != nil {
			resultErr = s.failedChainAttempt(work, events, entries, session, gymaccount.SeedPublish, r.EventKey, resultErr)
		}
	}()
	if e = s.publisher(p); e != nil {
		return Record{}, e
	}
	if phase(r) == "conflict" {
		return Record{}, ErrConflict
	}
	if r.Anchor == nil || r.Target == nil {
		return Record{}, ErrTargetUnavailable
	}
	cp, tip, e := p.AttestTarget(work, *r.Anchor, seednode.HeaderRef{Height: r.Target.Height, Hash: r.Target.Hash, Header: r.Target.Header})
	if e != nil {
		return Record{}, mapped(e)
	}
	if !validCheckpoint(s.meta.Pin, cp) || cp.Height != int32(r.Target.Height) || cp.Hash.String() != r.Target.Hash || tip.Height < r.Target.Height || tip.Height-r.Target.Height < 60 {
		return Record{}, ErrInvalid
	}
	n := s.now().UnixMilli()
	newFinality := retainAttestedCheckpoint(&r, cp, tip, n)
	r.Observation = Observation{ObservedAtMS: n, ChainObservedAtMS: n, Tip: &tip, NodeState: "online", TargetDescendants: tip.Height - r.Target.Height, CurrentFinality: true}
	return s.saveAction(work, events, entries, session, gymaccount.SeedPublish, r, s.meta, nil, func(r *Record, n int64) {
		r.Observation.ObservedAtMS = n
		r.Observation.ChainObservedAtMS = n
		if newFinality {
			r.Finality.ObservedAtMS = n
		}
	})
}

// retainAttestedCheckpoint applies an already verified exact-target observation.
// The caller has verified the concrete target, ancestry, signature and depth.
func retainAttestedCheckpoint(r *Record, cp chaincfg.SignedCheckpoint, tip Tip, n int64) bool {
	newFinality := r.Finality == nil
	// Equivalent valid signatures are not interchangeable journal evidence.
	// Once retained, the original certificate and historical finality stay exact.
	if r.Checkpoint == nil {
		r.Checkpoint = &cp
	}
	if newFinality {
		r.Finality = &Finality{Checkpoint: *r.Checkpoint, ObservedAtMS: n, ObservedTip: tip, TargetDescendants: tip.Height - r.Target.Height}
	}
	return newFinality
}
