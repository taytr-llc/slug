// Copyright (c) 2026 The SLUG developers. ISC license.
package gymcard

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

func (s *Store) record(id string) (Record, bool, error) {
	var indexed *indexRow
	for i := range s.index {
		if s.index[i].id == id {
			indexed = &s.index[i]
			break
		}
	}
	raw, ok, e := s.raw(key('r', id), MaxRecordSize)
	if e != nil {
		return Record{}, ok, e
	}
	if !ok {
		if indexed != nil {
			return Record{}, false, ErrCorrupt
		}
		return Record{}, false, nil
	}
	if indexed == nil || sha256.Sum256(raw) != indexed.digest {
		return Record{}, false, ErrCorrupt
	}
	// These exact bounded canonical bytes passed the full Open audit or the
	// owned Build validation before their digest was published after Sync.
	// Current source semantics are still checked by the concrete seed seam.
	var r Record
	if json.Unmarshal(raw, &r) != nil || r.CardID != id || r.Source.SourceReceipt.Input.Event.CutoffMS != indexed.cutoff {
		return Record{}, false, ErrCorrupt
	}
	s.mark("record-validated")
	return r, true, nil
}
func sortIndex(v []indexRow) {
	sort.Slice(v, func(i, j int) bool {
		if v[i].cutoff == v[j].cutoff {
			return v[i].id < v[j].id
		}
		return v[i].cutoff < v[j].cutoff
	})
}
func (s *Store) audit(ctx context.Context, m marker) error {
	raw, ok, e := s.raw([]byte("meta"), MaxMetadataSize)
	if e != nil || !ok {
		return ErrCorrupt
	}
	var meta metadata
	if decode(raw, MaxMetadataSize, &meta, true) != nil || meta.Version != 1 || meta.StoreID != m.StoreID || !equal(meta.Pin, m.Pin) || meta.SourceCreatedAtMS != s.sourceCreatedAtMS || !timestamp(meta.SourceCreatedAtMS) || !timestamp(meta.CreatedAtMS) || meta.CreatedAtMS < meta.SourceCreatedAtMS || !timestamp(meta.ClockMS) || meta.ClockMS < meta.CreatedAtMS || meta.EventCount > 256 {
		return ErrCorrupt
	}
	s.meta = meta
	s.clockSeen = meta.ClockMS
	s.index = make([]indexRow, 0, meta.EventCount)
	it := s.db.NewIterator(nil, nil)
	defer it.Release()
	count := 0
	for it.Next() {
		if ctx.Err() != nil {
			return ErrCancelled
		}
		k, v := it.Key(), it.Value()
		if handled, e := s.career.Audit(k, v); handled {
			if e != nil {
				return ErrCorrupt
			}
			continue
		}
		if bytes.Equal(k, []byte("meta")) {
			continue
		}
		if len(k) != 33 || k[0] != 'r' || len(v) == 0 || len(v) > MaxRecordSize || count >= MaxEvents {
			return ErrCorrupt
		}
		id := hex.EncodeToString(k[1:])
		var r Record
		if decode(v, MaxRecordSize, &r, true) != nil || r.CardID != id || validateRecord(meta.Pin, r) != nil || r.CreatedAtMS < meta.CreatedAtMS || r.CreatedAtMS > meta.ClockMS {
			return ErrCorrupt
		}
		s.index = append(s.index, indexRow{id: id, cutoff: r.Source.SourceReceipt.Input.Event.CutoffMS, digest: sha256.Sum256(v)})
		count++
	}
	if it.Error() != nil || s.career.Audited(meta.ClockMS) != nil || uint32(count) != meta.EventCount {
		return ErrCorrupt
	}
	sortIndex(s.index)
	return nil
}
