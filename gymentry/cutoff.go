// Copyright (c) 2026 The SLUG developers. ISC license.
package gymentry

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"github.com/syndtr/goleveldb/leveldb/util"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymstable"
)

func (s *Store) cutoffBytes(key []byte, limit int) ([]byte, error) {
	it := s.db.NewIterator(nil, nil)
	defer it.Release()
	if !it.Seek(key) || !bytes.Equal(it.Key(), key) || len(it.Value()) > limit || it.Error() != nil {
		return nil, ErrCorrupt
	}
	return append([]byte{}, it.Value()...), nil
}
func (s *Store) cutoffEntry(ctx context.Context, ev Event) (CutoffEntry, error) {
	eid, _ := EntryID(s.meta.StoreID, ev.EventID, s.meta.Pin.StablePin.Identity.GymID)
	out := CutoffEntry{Version: 1, StoreID: s.meta.StoreID, Event: ev, EntryID: eid, HistoryDigest: hash("SLUG entry cutoff history\x00v1\x00", rawID(s.meta.StoreID), rawID(ev.EventID), le64(uint64(ev.CutoffMS)))}
	if s.meta.OperationCount > MaxOperations || s.meta.EventCount > MaxEvents {
		return out, ErrCorrupt
	}
	// Check row inventories without retaining the potentially32KiB operation bodies.
	for _, prefix := range []string{"o:", "s:"} {
		it := s.db.NewIterator(util.BytesPrefix([]byte(prefix)), nil)
		var n uint32
		for it.Next() {
			if ctx.Err() != nil {
				it.Release()
				return out, ErrCancelled
			}
			n++
			k, v := it.Key(), it.Value()
			if n > s.meta.OperationCount || (prefix == "o:" && (len(k) != 66 || len(v) > MaxOperationSize)) || (prefix == "s:" && (len(k) != 10 || len(v) != 32)) {
				it.Release()
				return out, ErrCorrupt
			}
		}
		ie := it.Error()
		it.Release()
		if ie != nil || n != s.meta.OperationCount {
			return out, ErrCorrupt
		}
	}
	events := map[string]eventRow{}
	it := s.db.NewIterator(util.BytesPrefix([]byte("e:")), nil)
	for it.Next() {
		if ctx.Err() != nil {
			it.Release()
			return out, ErrCancelled
		}
		k, v := it.Key(), it.Value()
		if len(events) >= MaxEvents || len(k) != 66 || len(v) > MaxEventSize {
			it.Release()
			return out, ErrCorrupt
		}
		var r eventRow
		if decode(v, MaxEventSize, &r, true) != nil || validateEventRow(r, s.meta) != nil || r.Event.EventID != string(k[2:]) {
			it.Release()
			return out, ErrCorrupt
		}
		events[r.Event.EventID] = r
	}
	ie := it.Error()
	it.Release()
	if ie != nil || len(events) != int(s.meta.EventCount) {
		return out, ErrCorrupt
	}
	revisions := map[string]uint64{}
	latest := map[string]string{}
	first := map[string]int64{}
	var last int64
	var cp, sourceCount, roster uint64
	pending := ""
	for n := uint64(1); n <= uint64(s.meta.OperationCount); n++ {
		if ctx.Err() != nil {
			return out, ErrCancelled
		}
		idx, e := s.cutoffBytes(seqKey(n), 32)
		if e != nil || len(idx) != 32 {
			return out, ErrCorrupt
		}
		id := hex.EncodeToString(idx)
		raw, e := s.cutoffBytes(key("o", id), MaxOperationSize)
		if e != nil {
			return out, e
		}
		var o Operation
		if decode(raw, MaxOperationSize, &o, true) != nil || validateOperation(o) != nil || validateBoundOperation(o, s.meta) != nil || o.Intent.Sequence != n || o.Intent.OperationID != id {
			return out, ErrCorrupt
		}
		i := o.Intent
		r, ok := events[i.Request.EventID]
		if !ok || i.PreparedAtMS < last || i.Request.ExpectedRevision != revisions[r.Event.EventID] {
			return out, ErrCorrupt
		}
		if first[r.Event.EventID] == 0 {
			first[r.Event.EventID] = i.PreparedAtMS
		}
		last = i.PreparedAtMS
		switch o.State {
		case "prepared":
			if n != uint64(s.meta.OperationCount) {
				return out, ErrCorrupt
			}
			pending = id
		case "abandoned":
			last = o.AbandonedAtMS
		case "committed":
			v := o.Receipt
			if v.PreviousRevision != revisions[r.Event.EventID] || v.StableCheckpoint < cp || v.ObservedSourceCount < sourceCount {
				return out, ErrCorrupt
			}
			if v.Action == "replace" {
				if v.RosterRevision < roster {
					return out, ErrCorrupt
				}
				roster = v.RosterRevision
			}
			last = v.CommittedAtMS
			cp = v.StableCheckpoint
			sourceCount = v.ObservedSourceCount
			revisions[r.Event.EventID] = v.Revision
			latest[r.Event.EventID] = id
			if v.Event.EventID == ev.EventID && v.CommittedAtMS < ev.CutoffMS {
				out.CommittedCount++
				out.LastSequence = n
				out.LatestReceipt = v
				out.HistoryDigest = hash("SLUG entry cutoff row\x00v1\x00", rawID(out.HistoryDigest), le64(n), append(le32(uint32(len(raw))), raw...))
			}
		}
		if n == uint64(s.meta.OperationCount) && id != s.meta.LastOperationID {
			return out, ErrCorrupt
		}
	}
	if pending != s.meta.PendingOperationID {
		return out, ErrCorrupt
	}
	for id, r := range events {
		if r.Revision != revisions[id] || r.LatestOperationID != latest[id] || r.CreatedAtMS != first[id] {
			return out, ErrCorrupt
		}
	}
	return out, nil
}

// WithSnapshotView is a short original-View metadata lease; it grants no source
// proof, ownership or historical-cutoff verdict. The callback may not reenter.
func (s *Store) WithSnapshotView(ctx context.Context, session Session, fn func(SnapshotSourceInfo, func() error) error) error {
	if fn == nil {
		return ErrInvalid
	}
	ctx, done, e := s.begin(ctx, session, gymaccount.View)
	if e != nil {
		return e
	}
	defer done()
	return s.withLease(ctx, session, gymaccount.View, func(check func() error) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		now, e := s.observeLocked(nil, 0, nil)
		if e != nil {
			return e
		}
		if e = check(); e != nil {
			return e
		}
		return fn(SnapshotSourceInfo{1, s.meta.StoreID, s.meta.Pin, now}, check)
	})
}

// WithCutoffSnapshot retains entry→stable gate order. Preparation runs outside
// the account lease; the final callback sees already synchronized source fences.
func (s *Store) WithCutoffSnapshot(ctx context.Context, session Session, saturday string, prepare func(CutoffSnapshot) (func(CutoffFences, func() error) error, error)) error {
	if prepare == nil {
		return ErrInvalid
	}
	ctx, done, e := s.begin(ctx, session, gymaccount.View)
	if e != nil {
		return e
	}
	defer done()
	ev, e := EventForDate(s.meta.Pin, saturday)
	if e != nil {
		return e
	}
	if s.meta.ClockMS < ev.CutoffMS {
		return ErrClock
	}
	history, e := s.cutoffEntry(ctx, ev)
	if e != nil {
		return e
	}
	count := s.meta.OperationCount
	ids := []string{}
	if history.LatestReceipt != nil {
		for _, m := range history.LatestReceipt.Members {
			ids = append(ids, m.Assignment.ProspectID)
		}
	}
	invoked := false
	e = s.source.WithCutoffSelection(ctx, session, ev.CutoffMS, ids, func(selection gymstable.CutoffSelection) (func(gymstable.CutoffFence, func() error) error, error) {
		if selection.Version != 1 || selection.Roster.StoreID != s.meta.Pin.StableStoreID || selection.Roster.CutoffMS != ev.CutoffMS || len(selection.Members) != len(ids) {
			return nil, ErrCorrupt
		}
		for i, m := range selection.Members {
			a := m.Assignment
			if a != history.LatestReceipt.Members[i].Assignment || a.AcceptedAtMS > history.LatestReceipt.CommittedAtMS || m.ConditionAtCutoff.AsOfMS != ev.CutoffMS {
				return nil, ErrCorrupt
			}
		}
		snapshot := CutoffSnapshot{1, history, selection.Roster, selection.Members}
		raw, err := json.Marshal(snapshot)
		if err != nil || len(raw) > 61440 {
			return nil, ErrCapacity
		}
		commit, err := prepare(snapshot)
		if err != nil {
			return nil, err
		}
		if commit == nil {
			return nil, ErrInvalid
		}
		return func(f gymstable.CutoffFence, check func() error) error {
			s.mu.Lock()
			defer s.mu.Unlock()
			if s.meta.OperationCount != count || f.StoreID != s.meta.Pin.StableStoreID || f.CutoffMS != ev.CutoffMS || f.ObservedAtMS < ev.CutoffMS {
				return ErrConflict
			}
			now, err := s.observeLocked(&ev, f.ObservedAtMS, nil)
			if err != nil {
				return err
			}
			if err = check(); err != nil {
				return err
			}
			s.mark("cutoff-fenced")
			invoked = true
			return commit(CutoffFences{1, f, gymstable.CutoffFence{Version: 1, StoreID: s.meta.StoreID, CutoffMS: ev.CutoffMS, ObservedAtMS: now, OperationCount: s.meta.OperationCount}}, check)
		}, nil
	})
	// A slow/failed proof still advances the entry clock. Never write after the
	// final foreign callback, whose commit may already have succeeded.
	if e != nil && !invoked {
		s.mu.Lock()
		_, clockErr := s.observeLocked(&ev, 0, nil)
		s.mu.Unlock()
		if clockErr != nil {
			return clockErr
		}
	}
	return mapped(e)
}
