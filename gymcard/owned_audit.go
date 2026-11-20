// Copyright (c) 2026 The SLUG developers. ISC license.
package gymcard

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

func (s *OwnedStore) record(id string) (OwnedRecord, bool, error) {
	var indexed *indexRow
	for i := range s.index {
		if s.index[i].id == id {
			indexed = &s.index[i]
			break
		}
	}
	raw, ok, e := s.raw(key('r', id), OwnedMaxRecordSize)
	if e != nil {
		return OwnedRecord{}, ok, e
	}
	if !ok {
		if indexed != nil {
			return OwnedRecord{}, false, ErrCorrupt
		}
		return OwnedRecord{}, false, nil
	}
	if indexed == nil || sha256.Sum256(raw) != indexed.digest {
		return OwnedRecord{}, false, ErrCorrupt
	}
	// These exact bounded canonical bytes passed the full Open audit or the
	// owned Build validation before their digest was published after Sync.
	// Current source semantics are still checked by the concrete seed seam.
	var r OwnedRecord
	if json.Unmarshal(raw, &r) != nil || r.CardID != id || r.Source.SourceReceipt.Input.Event.CutoffMS != indexed.cutoff {
		return OwnedRecord{}, false, ErrCorrupt
	}
	s.mark("record-validated")
	return r, true, nil
}
func (s *OwnedStore) audit(ctx context.Context, m ownedMarker) error {
	raw, ok, e := s.raw([]byte("meta"), OwnedMaxMetadataSize)
	if e != nil || !ok {
		return ErrCorrupt
	}
	var meta ownedMetadata
	if ownedDecode(raw, OwnedMaxMetadataSize, &meta, true) != nil || meta.Version != 1 || meta.StoreID != m.StoreID || !equal(meta.Pin, m.Pin) || meta.SourceCreatedAtMS != s.sourceCreatedAtMS || !ownedTimestamp(meta.SourceCreatedAtMS) || !ownedTimestamp(meta.CreatedAtMS) || meta.CreatedAtMS < meta.SourceCreatedAtMS || !ownedTimestamp(meta.ClockMS) || meta.ClockMS < meta.CreatedAtMS || meta.EventCount > 256 {
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
		if len(k) != 33 || k[0] != 'r' || len(v) == 0 || len(v) > OwnedMaxRecordSize || count >= MaxEvents {
			return ErrCorrupt
		}
		id := hex.EncodeToString(k[1:])
		var r OwnedRecord
		if ownedDecode(v, OwnedMaxRecordSize, &r, true) != nil || r.CardID != id || validateOwnedRecord(meta.Pin, r) != nil || r.CreatedAtMS < meta.CreatedAtMS || r.CreatedAtMS > meta.ClockMS {
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
