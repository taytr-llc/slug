// Copyright (c) 2026 The SLUG developers. ISC license.
package gymbout

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"github.com/taytr-llc/slug/gymcard"
	"sort"
)

func indexFor(r Record) indexRow {
	return indexRow{BoutID: r.BoutID, CardID: r.CardID, Saturday: r.Source.Event.Saturday, RecordDigest: recordDigest(r), Saved: savedFor(gymcard.BoutRef{Bout: r.Source.Bout, SourceDigest: r.SourceDigest}, &r, false)}
}
func indexBound(rows []indexRow) bool {
	if len(rows) > MaxSavedBouts {
		return false
	}
	b, e := json.Marshal(rows)
	return e == nil && len(b) <= MaxIndexSize
}
func (s *Store) indexed(id string) *indexRow {
	for i := range s.index {
		if s.index[i].BoutID == id {
			return &s.index[i]
		}
	}
	return nil
}
func (s *Store) retained(id string, decodeRow bool) (Record, *indexRow, error) {
	row := s.indexed(id)
	raw, ok, e := s.raw(key('r', id), MaxRecordSize)
	if e != nil {
		return Record{}, nil, e
	}
	if !ok {
		if row != nil {
			return Record{}, nil, ErrCorrupt
		}
		return Record{}, nil, nil
	}
	if row == nil || hash("SLUG candidate bout record\x00v1\x00", lp(raw)) != row.RecordDigest {
		return Record{}, nil, ErrCorrupt
	}
	var r Record
	if decodeRow {
		if json.Unmarshal(raw, &r) != nil || r.BoutID != id {
			return Record{}, nil, ErrCorrupt
		}
	}
	s.mark("retained-row-checked")
	return r, row, nil
}
func cardBinding(r Record) string {
	s := r.Source
	return hash("private audited card binding\x00", []byte(s.StoreID), rawID(s.CardID), rawID(s.CardRecordDigest), rawID(s.CardSourceDigest), rawID(s.SnapshotDigest), rawID(s.SeedEventKey), le32(s.TargetHeight), rawID(s.TargetHash), func() []byte { b, _ := json.Marshal([]any{s.Event, s.CardCreatedAtMS, s.FinalizedAtMS}); return b }())
}
func (s *Store) audit(ctx context.Context, m marker) error {
	raw, ok, e := s.raw([]byte("meta"), MaxMetadataSize)
	if e != nil || !ok {
		return ErrCorrupt
	}
	var meta metadata
	if decode(raw, MaxMetadataSize, &meta, true) != nil || meta.Version != 1 || meta.StoreID != m.StoreID || !equal(meta.Pin, m.Pin) || meta.SourceCreatedAtMS != s.sourceCreatedAtMS || !timestamp(meta.SourceCreatedAtMS) || !timestamp(meta.CreatedAtMS) || meta.CreatedAtMS < meta.SourceCreatedAtMS || !timestamp(meta.ClockMS) || meta.ClockMS < meta.CreatedAtMS || meta.BoutCount > MaxSavedBouts {
		return ErrCorrupt
	}
	s.meta = meta
	s.clockSeen = meta.ClockMS
	rows := make([]indexRow, 0, meta.BoutCount)
	type group struct {
		binding      string
		classes, ids map[string]bool
		count        int
	}
	groups := map[string]*group{}
	commitments := map[string]string{}
	it := s.db.NewIterator(nil, nil)
	defer it.Release()
	total := 0
	for it.Next() {
		if ctx.Err() != nil {
			return ErrCancelled
		}
		k, v := it.Key(), it.Value()
		if bytes.Equal(k, []byte("meta")) {
			continue
		}
		if len(k) != 33 || k[0] != 'r' || len(v) == 0 || len(v) > MaxRecordSize || len(rows) >= MaxSavedBouts {
			return ErrCorrupt
		}
		total += len(v)
		if total > 64*1024*1024 {
			return ErrCorrupt
		}
		id := hex.EncodeToString(k[1:])
		var r Record
		if decode(v, MaxRecordSize, &r, true) != nil || r.BoutID != id || r.CreatedAtMS < meta.CreatedAtMS || r.CreatedAtMS > meta.ClockMS || r.Source.CardCreatedAtMS < meta.SourceCreatedAtMS {
			return ErrCorrupt
		}
		if e := validateRecord(ctx, meta.Pin, r); e != nil {
			if ctx.Err() != nil {
				return ErrCancelled
			}
			return ErrCorrupt
		}
		binding := cardBinding(r)
		g := groups[r.CardID]
		if g == nil {
			g = &group{binding: binding, classes: map[string]bool{}, ids: map[string]bool{}}
			groups[r.CardID] = g
		}
		if g.binding != binding || g.count >= 5 || g.classes[r.Source.Bout.Class] {
			return ErrCorrupt
		}
		g.classes[r.Source.Bout.Class] = true
		g.count++
		for _, id := range []string{r.Source.Bout.CornerA, r.Source.Bout.CornerB} {
			if g.ids[id] {
				return ErrCorrupt
			}
			g.ids[id] = true
		}
		if x, ok := commitments[r.Source.CardRecordDigest]; ok && x != binding {
			return ErrCorrupt
		}
		commitments[r.Source.CardRecordDigest] = binding
		rows = append(rows, indexFor(r))
		s.mark("audit-row")
	}
	if it.Error() != nil || uint32(len(rows)) != meta.BoutCount || !indexBound(rows) {
		return ErrCorrupt
	}
	if ctx.Err() != nil {
		return ErrCancelled
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].BoutID < rows[j].BoutID })
	s.index = rows
	return nil
}
