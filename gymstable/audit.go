// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
)

func validAssignment(a Assignment, p Pin) bool {
	if a.Version != Version1 || a.Sequence == 0 || a.Sequence > MaxAssignments || a.GymID != p.Identity.GymID || a.DescriptorID != p.Source.DescriptorID || a.BornAtMS <= 0 || a.AcceptedAtMS < a.BornAtMS || a.AcceptedAtMS > MaxTimestampMS || a.AcceptedAtMS-a.BornAtMS >= 300000 {
		return false
	}
	for _, v := range []string{a.ProspectID, a.GymID, a.WorkHash, a.DescriptorID, a.ManifestDigest, a.PortraitDigest, a.SourceRecordDigest} {
		if !hashID(v) {
			return false
		}
	}
	return true
}
func validSlots(slots [10]string) bool {
	seen := map[string]bool{}
	for _, id := range slots {
		if id == "" {
			continue
		}
		if !hashID(id) || seen[id] {
			return false
		}
		seen[id] = true
	}
	return true
}
func validOperation(o Operation, m metadata) bool {
	i := o.Intent
	if o.Version != Version1 || i.Version != Version1 || i.Sequence == 0 || i.Sequence > uint64(m.OperationCount) || !hashID(i.OperationID) || i.Generation == 0 || i.Generation > MaxSafeInteger || !hashID(i.ViewID) || i.PreparedAtMS <= 0 || i.PreparedAtMS > m.ClockMS {
		return false
	}
	if _, e := ValidateSlotRequest(i.Request); e != nil || i.Request.ExpectedRevision > m.RosterRevision {
		return false
	}
	switch o.State {
	case "prepared":
		return o.Receipt == nil && o.AbandonedAtMS == 0 && i.Request.ExpectedRevision == m.RosterRevision
	case "abandoned":
		return o.Receipt == nil && o.AbandonedAtMS >= i.PreparedAtMS && o.AbandonedAtMS <= m.ClockMS
	case "committed":
		r := o.Receipt
		if r == nil || o.AbandonedAtMS != 0 || r.Version != Version1 || r.OperationID != i.OperationID || r.Generation != i.Generation || r.ViewID != i.ViewID || r.Request != i.Request || r.PreviousRevision != i.Request.ExpectedRevision || r.Revision > m.RosterRevision || r.CommittedAtMS < i.PreparedAtMS || r.CommittedAtMS > m.ClockMS || !validSlots(r.Slots) || r.Slots[i.Request.Slot-1] != i.Request.ProspectID {
			return false
		}
		if r.PreviousProspectID != "" && !hashID(r.PreviousProspectID) {
			return false
		}
		if r.Changed {
			return r.Revision == r.PreviousRevision+1 && r.PreviousProspectID != i.Request.ProspectID
		}
		return r.Revision == r.PreviousRevision && r.PreviousProspectID == i.Request.ProspectID
	default:
		return false
	}
}
func (s *Store) audit(m marker, sourceCount uint64) error {
	raw, e := s.db.Get([]byte("meta"), nil)
	if e != nil || decode(raw, MaxMetadataSize, 5, &s.meta, true) != nil {
		return ErrCorrupt
	}
	x := s.meta
	if x.StoreID != m.StoreID || x.Pin != m.Pin || x.Checkpoint > x.ObservedSourceCount || x.ObservedSourceCount > MaxAssignments || sourceCount < x.ObservedSourceCount || x.RosterRevision > uint64(x.OperationCount) || x.OperationCount > MaxOperations || x.ClockMS <= 0 || x.ClockMS > MaxTimestampMS || x.LastAsOfMS < 0 || x.LastAsOfMS > x.ClockMS {
		return ErrCorrupt
	}
	if (x.OperationCount == 0) != (x.LastOperationID == "") || (x.LastOperationID != "" && !hashID(x.LastOperationID)) || (x.PendingOperationID != "" && !hashID(x.PendingOperationID)) {
		return ErrCorrupt
	}
	raw, e = s.db.Get([]byte("roster"), nil)
	if e != nil || decode(raw, MaxMetadataSize, 2, &s.roster, true) != nil || s.roster.Version != Version1 || s.roster.Revision != x.RosterRevision || !validSlots(s.roster.Slots) {
		return ErrCorrupt
	}
	assigns := map[uint64]Assignment{}
	ids := map[string]uint64{}
	ops := map[uint64]Operation{}
	var indexes uint64
	it := s.db.NewIterator(nil, nil)
	defer it.Release()
	for it.Next() {
		k, v := it.Key(), it.Value()
		switch {
		case bytes.Equal(k, []byte("meta")), bytes.Equal(k, []byte("roster")):
		case len(k) == 9 && k[0] == 'a':
			if len(assigns) >= MaxAssignments || len(v) > MaxAssignmentSize {
				return ErrCorrupt
			}
			var a Assignment
			if decode(v, MaxAssignmentSize, 1, &a, true) != nil || !validAssignment(a, x.Pin) || a.Sequence != binary.BigEndian.Uint64(k[1:]) || a.Sequence > x.Checkpoint || a.AcceptedAtMS > x.ClockMS {
				return ErrCorrupt
			}
			if _, ok := ids[a.ProspectID]; ok {
				return ErrCorrupt
			}
			assigns[a.Sequence] = a
			ids[a.ProspectID] = a.Sequence
		case len(k) == 33 && k[0] == 'i':
			indexes++
			if indexes > MaxAssignments || len(v) != 8 {
				return ErrCorrupt
			}
			n := binary.BigEndian.Uint64(v)
			if n == 0 || n > x.Checkpoint {
				return ErrCorrupt
			}
			row, e := s.db.Get(seqKey(n), nil)
			var a Assignment
			if e != nil || decode(row, MaxAssignmentSize, 1, &a, true) != nil || a.ProspectID != hex.EncodeToString(k[1:]) {
				return ErrCorrupt
			}
		case len(k) == 33 && k[0] == 'o':
			if len(ops) >= MaxOperations || len(v) > MaxOperationSize {
				return ErrCorrupt
			}
			var o Operation
			if decode(v, MaxOperationSize, 5, &o, true) != nil || !validOperation(o, x) || o.Intent.OperationID != hex.EncodeToString(k[1:]) {
				return ErrCorrupt
			}
			if _, ok := ops[o.Intent.Sequence]; ok {
				return ErrCorrupt
			}
			ops[o.Intent.Sequence] = o
		default:
			return ErrCorrupt
		}
	}
	if it.Error() != nil || uint64(len(assigns)) != x.Checkpoint || indexes != x.Checkpoint || len(ops) != int(x.OperationCount) {
		return ErrCorrupt
	}
	for n := uint64(1); n <= x.Checkpoint; n++ {
		a, ok := assigns[n]
		if !ok {
			return ErrCorrupt
		}
		v, e := s.db.Get(idKey('i', a.ProspectID), nil)
		if e != nil || len(v) != 8 || binary.BigEndian.Uint64(v) != n {
			return ErrCorrupt
		}
	}
	var slots [10]string
	var revision uint64
	var lastTime int64
	var pending string
	for n := uint64(1); n <= uint64(x.OperationCount); n++ {
		o, ok := ops[n]
		if !ok || o.Intent.PreparedAtMS < lastTime {
			return ErrCorrupt
		}
		lastTime = o.Intent.PreparedAtMS
		if o.Intent.Request.ExpectedRevision != revision {
			return ErrCorrupt
		}
		if id := o.Intent.Request.ProspectID; id != "" {
			if _, ok := ids[id]; !ok {
				return ErrCorrupt
			}
		}
		switch o.State {
		case "prepared":
			if n != uint64(x.OperationCount) || pending != "" {
				return ErrCorrupt
			}
			pending = o.Intent.OperationID
		case "abandoned":
			lastTime = o.AbandonedAtMS
		case "committed":
			r := o.Receipt
			if r.PreviousRevision != revision || r.PreviousProspectID != slots[r.Request.Slot-1] {
				return ErrCorrupt
			}
			changed := slots[r.Request.Slot-1] != r.Request.ProspectID
			slots[r.Request.Slot-1] = r.Request.ProspectID
			if changed {
				revision++
			}
			if r.Changed != changed || r.Revision != revision || r.Slots != slots {
				return ErrCorrupt
			}
			lastTime = r.CommittedAtMS
		}
		if n == uint64(x.OperationCount) && o.Intent.OperationID != x.LastOperationID {
			return ErrCorrupt
		}
	}
	if pending != x.PendingOperationID || revision != x.RosterRevision || slots != s.roster.Slots {
		return ErrCorrupt
	}
	for _, id := range slots {
		if id != "" {
			if _, ok := ids[id]; !ok {
				return ErrCorrupt
			}
		}
	}
	s.clockSeen = x.ClockMS
	return nil
}
