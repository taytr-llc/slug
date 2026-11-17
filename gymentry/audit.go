// Copyright (c) 2026 The SLUG developers. ISC license.
package gymentry

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"github.com/taytr-llc/slug/gymstable"
)

func validateEventRow(r eventRow, m metadata) error {
	ev, e := EventForDate(m.Pin, r.Event.Saturday)
	id, _ := EntryID(m.StoreID, r.Event.EventID, m.Pin.StablePin.Identity.GymID)
	if e != nil || r.Version != Version1 || r.Event != ev || r.EntryID != id || !timestamp(r.CreatedAtMS) || r.CreatedAtMS >= ev.CutoffMS || r.CreatedAtMS > m.ClockMS || r.Revision > uint64(m.OperationCount) || !optionalID(r.LatestOperationID) || (r.Revision == 0) != (r.LatestOperationID == "") {
		return ErrCorrupt
	}
	switch r.Phase {
	case "accepting":
		if r.ClosedObservedAtMS != 0 {
			return ErrCorrupt
		}
	case "closed_pending_lock":
		if r.ClosedObservedAtMS < ev.CutoffMS || r.ClosedObservedAtMS > m.ClockMS {
			return ErrCorrupt
		}
	default:
		return ErrCorrupt
	}
	return nil
}
func validateBoundOperation(o Operation, m metadata) error {
	i := o.Intent
	ev, e := EventForDate(m.Pin, i.Request.Saturday)
	if e != nil || i.Request.EventID != ev.EventID || i.Sequence > uint64(m.OperationCount) || i.PreparedAtMS > m.ClockMS || o.AbandonedAtMS > m.ClockMS {
		return ErrCorrupt
	}
	if o.Receipt != nil {
		r := o.Receipt
		eid, _ := EntryID(m.StoreID, ev.EventID, m.Pin.StablePin.Identity.GymID)
		if r.StoreID != m.StoreID || r.EntryID != eid || r.Event != ev || r.CommittedAtMS > m.ClockMS || r.StableCheckpoint > m.ObservedCheckpoint || r.ObservedSourceCount > m.ObservedSourceCount || r.RosterRevision > m.ObservedRosterRevision {
			return ErrCorrupt
		}
		for _, x := range r.Members {
			if x.Assignment.GymID != m.Pin.StablePin.Identity.GymID || x.Assignment.DescriptorID != m.Pin.StablePin.Source.DescriptorID {
				return ErrCorrupt
			}
		}
	}
	return nil
}
func (s *Store) audit(ctx context.Context, mark marker, source gymstable.EntrySourceInfo) error {
	// Inspect the iterator-owned value before any Get-sized allocation.
	metaIt := s.db.NewIterator(nil, nil)
	found := metaIt.Seek([]byte("meta"))
	if !found || !bytes.Equal(metaIt.Key(), []byte("meta")) || len(metaIt.Value()) > MaxMetadataSize {
		metaIt.Release()
		return ErrCorrupt
	}
	decodeErr := decode(metaIt.Value(), MaxMetadataSize, &s.meta, true)
	iterErr := metaIt.Error()
	metaIt.Release()
	if decodeErr != nil || iterErr != nil {
		return ErrCorrupt
	}

	m := s.meta
	if m.Version != Version1 || m.StoreID != mark.StoreID || m.Pin != mark.Pin || m.EventCount > MaxEvents || m.OperationCount > MaxOperations || m.ObservedCheckpoint > m.ObservedSourceCount || m.ObservedSourceCount > gymstable.MaxAssignments || m.ObservedRosterRevision > gymstable.MaxOperations || !timestamp(m.ClockMS) || !optionalID(m.PendingOperationID) || !optionalID(m.LastOperationID) || (m.OperationCount == 0) != (m.LastOperationID == "") {
		return ErrCorrupt
	}
	next := m
	if mergeSource(&next, source) != nil {
		return ErrCorrupt
	}
	events := map[string]eventRow{}
	ops := map[uint64]Operation{}
	indexes := map[uint64]string{}
	it := s.db.NewIterator(nil, nil)
	defer it.Release()
	for it.Next() {
		if ctx.Err() != nil {
			return ErrCancelled
		}
		k, v := it.Key(), it.Value()
		switch {
		case bytes.Equal(k, []byte("meta")):
		case len(k) == 66 && bytes.Equal(k[:2], []byte("e:")):
			if len(events) >= MaxEvents || len(v) > MaxEventSize || !hashID(string(k[2:])) {
				return ErrCorrupt
			}
			var r eventRow
			if decode(v, MaxEventSize, &r, true) != nil || validateEventRow(r, m) != nil || r.Event.EventID != string(k[2:]) {
				return ErrCorrupt
			}
			events[r.Event.EventID] = r
		case len(k) == 66 && bytes.Equal(k[:2], []byte("o:")):
			if len(ops) >= MaxOperations || len(v) > MaxOperationSize || !hashID(string(k[2:])) {
				return ErrCorrupt
			}
			var o Operation
			if decode(v, MaxOperationSize, &o, true) != nil || validateOperation(o) != nil || validateBoundOperation(o, m) != nil || o.Intent.OperationID != string(k[2:]) {
				return ErrCorrupt
			}
			if _, ok := ops[o.Intent.Sequence]; ok {
				return ErrCorrupt
			}
			ops[o.Intent.Sequence] = o
		case len(k) == 10 && bytes.Equal(k[:2], []byte("s:")):
			if len(indexes) >= MaxOperations || len(v) != 32 {
				return ErrCorrupt
			}
			n := binary.BigEndian.Uint64(k[2:])
			if n == 0 || n > uint64(m.OperationCount) {
				return ErrCorrupt
			}
			indexes[n] = hex.EncodeToString(v)
		default:
			return ErrCorrupt
		}
	}
	if it.Error() != nil || len(events) != int(m.EventCount) || len(ops) != int(m.OperationCount) || len(indexes) != int(m.OperationCount) {
		return ErrCorrupt
	}
	revisions := map[string]uint64{}
	latest := map[string]string{}
	first := map[string]int64{}
	pending := ""
	var lastTime int64
	var lastCheckpoint, lastCount, lastRosterRevision uint64
	for n := uint64(1); n <= uint64(m.OperationCount); n++ {
		o, ok := ops[n]
		if !ok || indexes[n] != o.Intent.OperationID {
			return ErrCorrupt
		}
		i := o.Intent
		r, ok := events[i.Request.EventID]
		if !ok || i.PreparedAtMS < lastTime || i.Request.ExpectedRevision != revisions[r.Event.EventID] {
			return ErrCorrupt
		}
		if first[r.Event.EventID] == 0 {
			first[r.Event.EventID] = i.PreparedAtMS
		}
		lastTime = i.PreparedAtMS
		switch o.State {
		case "prepared":
			if pending != "" || n != uint64(m.OperationCount) {
				return ErrCorrupt
			}
			pending = i.OperationID
		case "abandoned":
			lastTime = o.AbandonedAtMS
		case "committed":
			v := o.Receipt
			if v.PreviousRevision != revisions[r.Event.EventID] || v.StableCheckpoint < lastCheckpoint || v.ObservedSourceCount < lastCount {
				return ErrCorrupt
			}
			if v.Action == "replace" {
				if v.RosterRevision < lastRosterRevision {
					return ErrCorrupt
				}
				lastRosterRevision = v.RosterRevision
			}
			lastTime = v.CommittedAtMS
			lastCheckpoint = v.StableCheckpoint
			lastCount = v.ObservedSourceCount
			revisions[r.Event.EventID] = v.Revision
			latest[r.Event.EventID] = i.OperationID
		}
		if n == uint64(m.OperationCount) && m.LastOperationID != i.OperationID {
			return ErrCorrupt
		}
	}
	if pending != m.PendingOperationID {
		return ErrCorrupt
	}
	for id, r := range events {
		if first[id] != r.CreatedAtMS || r.Revision != revisions[id] || r.LatestOperationID != latest[id] {
			return ErrCorrupt
		}
	}
	s.clockSeen = m.ClockMS
	return nil
}
