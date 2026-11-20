// Copyright (c) 2026 The SLUG developers. ISC license.
package gymentry

import (
	"context"
	"encoding/hex"
	"github.com/taytr-llc/slug/internal/ownedcodec"
)

func (s *OwnedStore) audit(ctx context.Context, marker ownedMarker) error {
	raw, e := s.row([]byte("meta"), OwnedMaxMetadataSize)
	if e != nil || ownedcodec.Decode(raw, OwnedMaxMetadataSize, &s.meta) != nil {
		return ErrCorrupt
	}
	m := s.meta
	if m.Version != 1 || m.StoreID != marker.StoreID || m.Pin != marker.Pin || !timestamp(m.ClockMS) || m.EventCount > MaxEvents || m.OperationCount > MaxOperations || !optionalID(m.PendingOperationID) || !optionalID(m.LastOperationID) {
		return ErrCorrupt
	}
	events := map[string]eventRow{}
	var ops, preps, seqs uint32
	it := s.db.NewIterator(nil, nil)
	for it.Next() {
		if ctx.Err() != nil {
			it.Release()
			return ErrCancelled
		}
		k, v := it.Key(), it.Value()
		switch {
		case string(k) == "meta":
		case len(k) == 66 && string(k[:2]) == "e:":
			if len(events) >= MaxEvents || len(v) > MaxEventSize {
				it.Release()
				return ErrCorrupt
			}
			var r eventRow
			if ownedcodec.Decode(v, MaxEventSize, &r) != nil {
				it.Release()
				return ErrCorrupt
			}
			ev, e := OwnedEventForDate(m.Pin, r.Event.Saturday)
			eid, _ := OwnedEntryID(m.StoreID, ev.EventID, m.Pin.StablePin.Stable.Identity.GymID)
			if e != nil || r.Version != 1 || r.Event != ev || r.EntryID != eid || ev.EventID != string(k[2:]) || !timestamp(r.CreatedAtMS) || r.CreatedAtMS >= ev.CutoffMS || r.CreatedAtMS > m.ClockMS || r.Revision > MaxOperations || !optionalID(r.LatestOperationID) || r.ClosedObservedAtMS < 0 || r.ClosedObservedAtMS > m.ClockMS || r.Phase != "accepting" && r.Phase != "closed_pending_lock" || r.Phase == "accepting" && r.ClosedObservedAtMS != 0 || r.Phase == "closed_pending_lock" && r.ClosedObservedAtMS < ev.CutoffMS {
				it.Release()
				return ErrCorrupt
			}
			events[ev.EventID] = r
		case len(k) == 66 && string(k[:2]) == "o:":
			ops++
			if ops > m.OperationCount || len(v) > OwnedMaxOperationSize {
				it.Release()
				return ErrCorrupt
			}
		case len(k) == 66 && string(k[:2]) == "p:":
			preps++
			if preps > m.OperationCount || len(v) > 4096 {
				it.Release()
				return ErrCorrupt
			}
		case len(k) == 10 && string(k[:2]) == "s:":
			seqs++
			if seqs > m.OperationCount || len(v) != 32 {
				it.Release()
				return ErrCorrupt
			}
		default:
			it.Release()
			return ErrCorrupt
		}
	}
	e = it.Error()
	it.Release()
	if e != nil || ops != m.OperationCount || preps != ops || seqs != ops || len(events) != int(m.EventCount) {
		return ErrCorrupt
	}
	revisions := map[string]uint64{}
	latest := map[string]string{}
	first := map[string]int64{}
	seen := map[string]bool{}
	var stamp int64
	pending, last := "", ""
	for n := uint64(1); n <= uint64(ops); n++ {
		if ctx.Err() != nil {
			return ErrCancelled
		}
		raw, e = s.row(seqKey(n), 32)
		if e != nil || len(raw) != 32 {
			return ErrCorrupt
		}
		id := hex.EncodeToString(raw)
		if seen[id] {
			return ErrCorrupt
		}
		seen[id] = true
		o, e := s.operation(id)
		if e != nil || o.Intent.Sequence != n {
			return ErrCorrupt
		}
		p, e := s.preparation(o)
		if e != nil {
			return e
		}
		evID := o.Intent.Request.EventID
		if _, ok := events[evID]; !ok {
			return ErrCorrupt
		}
		if o.Intent.PreparedAtMS < stamp || o.Intent.Request.ExpectedRevision != revisions[evID] {
			return ErrCorrupt
		}
		if first[evID] == 0 {
			first[evID] = o.Intent.PreparedAtMS
		}
		stamp = o.Intent.PreparedAtMS
		last = id
		switch o.State {
		case "prepared":
			if n != uint64(ops) {
				return ErrCorrupt
			}
			pending = id
		case "abandoned":
			stamp = o.AbandonedAtMS
		case "committed":
			r := o.Receipt
			for i, member := range r.Members {
				if member.Ownership.StateID != p.StateIDs[i] {
					return ErrCorrupt
				}
			}
			if r.OwnershipHead.AdmissionSequence < p.Head.AdmissionSequence {
				return ErrCorrupt
			}
			revisions[evID] = r.Revision
			latest[evID] = id
			stamp = r.CommittedAtMS
		}
	}
	if stamp > m.ClockMS || pending != m.PendingOperationID || last != m.LastOperationID {
		return ErrCorrupt
	}
	for id, r := range events {
		if r.Revision != revisions[id] || r.LatestOperationID != latest[id] || r.CreatedAtMS != first[id] {
			return ErrCorrupt
		}
	}
	return nil
}
