// Copyright (c) 2026 The SLUG developers. ISC license.
package gymentry

import (
	"context"
	"encoding/hex"
	"errors"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/internal/ownedcodec"
)

func (s *OwnedStore) cutoffEntry(ctx context.Context, ev Event) (OwnedCutoffEntry, error) {
	eid, _ := OwnedEntryID(s.meta.StoreID, ev.EventID, s.meta.Pin.StablePin.Stable.Identity.GymID)
	out := OwnedCutoffEntry{Version: 1, StoreID: s.meta.StoreID, Event: ev, EntryID: eid}
	out.HistoryDigest, _ = ownedcodec.Hash("entry cutoff history", struct {
		Version  uint32 `json:"version"`
		StoreID  string `json:"store_id"`
		EventID  string `json:"event_id"`
		CutoffMS int64  `json:"cutoff_ms"`
	}{1, s.meta.StoreID, ev.EventID, ev.CutoffMS})
	for n := uint64(1); n <= uint64(s.meta.OperationCount); n++ {
		if ctx.Err() != nil {
			return out, ErrCancelled
		}
		raw, e := s.row(seqKey(n), 32)
		if e != nil || len(raw) != 32 {
			return out, ErrCorrupt
		}
		o, e := s.operation(hex.EncodeToString(raw))
		if e != nil || o.Intent.Sequence != n {
			return out, ErrCorrupt
		}
		if o.State != "committed" || o.Receipt.Event.EventID != ev.EventID || o.Receipt.CommittedAtMS >= ev.CutoffMS {
			continue
		}
		out.CommittedCount++
		out.LastSequence = n
		out.LatestReceipt = o.Receipt
		out.HistoryDigest, e = ownedcodec.Hash("entry cutoff history", struct {
			Previous  string         `json:"previous_digest"`
			Operation OwnedOperation `json:"operation"`
		}{out.HistoryDigest, o})
		if e != nil {
			return out, e
		}
	}
	return out, nil
}

// WithCutoffSnapshot uses the event coordinator's exact signed CutoffID. It
// retains entry→stable lanes; no latest owner head supplies historical facts.
func (s *OwnedStore) WithCutoffSnapshot(ctx context.Context, session Session, saturday, cutoffID string, prepare func(OwnedCutoffSnapshot) (func(CutoffFences, func() error) error, error)) error {
	if prepare == nil || !hashID(cutoffID) {
		return ErrInvalid
	}
	ctx, done, e := s.begin(ctx, session, gymaccount.View)
	if e != nil {
		return e
	}
	defer done()
	ev, e := OwnedEventForDate(s.meta.Pin, saturday)
	if e != nil {
		return e
	}
	now, e := s.clock()
	if e != nil {
		return e
	}
	if now < ev.CutoffMS {
		return ErrClock
	}
	history, e := s.cutoffEntry(ctx, ev)
	if e != nil {
		return e
	}
	ids := []string{}
	if history.LatestReceipt != nil {
		for _, m := range history.LatestReceipt.Members {
			ids = append(ids, m.Birth.ProspectID)
		}
	}
	committed := false
	e = s.source.WithCutoffSelection(ctx, session, cutoffID, ids, func(v gymstable.OwnedCutoffSelection) (func(gymstable.CutoffFence, func() error) error, error) {
		if v.Cutoff.CutoffMS != ev.CutoffMS || v.Roster.StoreID != s.meta.Pin.StableStoreID || len(v.Members)+len(v.Excluded) != len(ids) {
			return nil, ErrCorrupt
		}
		saved := map[string]OwnedMember{}
		if history.LatestReceipt != nil {
			for _, m := range history.LatestReceipt.Members {
				saved[m.Birth.ProspectID] = m
			}
		}
		for _, m := range v.Members {
			old, ok := saved[m.Birth.ProspectID]
			if !ok || old.Birth != m.Birth {
				return nil, ErrCorrupt
			}
		}
		snapshot := OwnedCutoffSnapshot{1, v.Cutoff, history, v.Roster, v.Members, v.Excluded}
		if !ownedcodec.Fits(snapshot, 104*1024) {
			return nil, ErrCapacity
		}
		commit, e := prepare(snapshot)
		if e != nil {
			return nil, e
		}
		if commit == nil {
			return nil, ErrInvalid
		}
		now, e := s.clock()
		if e != nil {
			return nil, e
		}
		m := s.meta
		m.ClockMS = now
		b := new(leveldb.Batch)
		row, exists, e := s.event(ev)
		if e != nil {
			return nil, e
		}
		if exists && row.ClosedObservedAtMS == 0 {
			row.Phase = "closed_pending_lock"
			row.ClosedObservedAtMS = now
			if e = put(b, key("e", ev.EventID), row, MaxEventSize); e != nil {
				return nil, e
			}
		}
		return func(f gymstable.CutoffFence, check func() error) error {
			if f.StoreID != s.meta.Pin.StableStoreID || f.CutoffMS != ev.CutoffMS || f.ObservedAtMS < ev.CutoffMS {
				return ErrConflict
			}
			if e := check(); e != nil {
				return e
			}
			if e := s.persist(b, m); e != nil {
				return e
			}
			committed = true
			return commit(CutoffFences{1, f, gymstable.CutoffFence{Version: 1, StoreID: s.meta.StoreID, CutoffMS: ev.CutoffMS, ObservedAtMS: now, OperationCount: s.meta.OperationCount}}, check)
		}, nil
	})
	if e != nil && committed {
		return errors.Join(ErrOutcomeUnknown, e)
	}
	return e
}
