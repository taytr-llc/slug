// Copyright (c) 2026 The SLUG developers. ISC license.
package gymresult

import (
	"bytes"
	"context"
	"encoding/json"
	"sort"
)

func coinKey(prefix byte, v Outpoint) []byte {
	return append(append([]byte{prefix}, rawID(v.TxID)...), le32(v.Index)...)
}
func indexFor(r Record) indexRow {
	return indexRow{r.ResultKey, r.Statement.Statement.Source.Source.Event.Saturday, r.Statement.Statement.Source.Source.Bout.BoutID, recordDigest(r)}
}
func (s *Store) indexed(id string) *indexRow {
	for i := range s.index {
		if s.index[i].ResultKey == id {
			return &s.index[i]
		}
	}
	return nil
}
func (s *Store) record(id string) (Record, bool, error) {
	row := s.indexed(id)
	raw, ok, e := s.raw(key('r', id), MaxRecordSize)
	if e != nil {
		return Record{}, false, e
	}
	if !ok {
		if row != nil {
			return Record{}, false, ErrCorrupt
		}
		return Record{}, false, nil
	}
	if row == nil || hash("SLUG candidate result record\x00v1\x00", lp(raw)) != row.RecordDigest {
		return Record{}, false, ErrCorrupt
	}
	var r Record
	if json.Unmarshal(raw, &r) != nil || r.ResultKey != id {
		return Record{}, false, ErrCorrupt
	}
	s.mark("retained-row-checked")
	return r, true, nil
}
func indexBound(rows []indexRow) bool { return len(rows) <= MaxResults && fits(rows, MaxIndexSize) }
func (s *Store) installIndex(r Record) {
	x := indexFor(r)
	for i := range s.index {
		if s.index[i].ResultKey == x.ResultKey {
			s.index[i] = x
			return
		}
	}
	s.index = append(s.index, x)
	sort.Slice(s.index, func(i, j int) bool { return s.index[i].ResultKey < s.index[j].ResultKey })
}
func validMeta(m metadata, p Pin, id string) bool {
	if m.Version != 1 || m.StoreID != id || !equal(m.Pin, p) || !timestamp(m.SourceCreatedAtMS) || !timestamp(m.CreatedAtMS) || !timestamp(m.ClockMS) || m.SourceCreatedAtMS > m.CreatedAtMS || m.CreatedAtMS > m.ClockMS || m.ResultCount > MaxResults || validateCursor(p, m.FundingCursor) != nil || m.FundingRestarts == nil || len(m.FundingRestarts) > 16 || uint32(len(m.FundingRestarts)) != m.FundingCursor.Generation {
		return false
	}
	if m.FundingCursor.Height < 0 && !equal(m.FundingCursor, initialCursor(p, "", m.FundingCursor.Generation)) {
		return false
	}
	n := m.CreatedAtMS
	for i, x := range m.FundingRestarts {
		if validateScanRestart(p, x) != nil || x.Generation != uint32(i+1) || x.ObservedAtMS < n || x.ObservedAtMS > m.ClockMS {
			return false
		}
		n = x.ObservedAtMS
	}
	if m.LastFundingTip != nil && validateTip(p, *m.LastFundingTip) != nil {
		return false
	}
	return !m.FundingComplete || (m.LastFundingTip != nil && int64(m.LastFundingTip.Height) == int64(m.FundingCursor.Height) && m.LastFundingTip.Hash == m.FundingCursor.Hash)
}
func recordMaxTime(r Record) int64 {
	n := max(r.PreparedAtMS, r.Observation.ObservedAtMS)
	if r.Publication != nil {
		n = max(n, r.Publication.BuiltAtMS, r.Publication.LastDispatchAtMS)
	}
	if r.Inclusion != nil {
		n = max(n, r.Inclusion.ObservedAtMS)
	}
	if r.Finality != nil {
		n = max(n, r.Finality.ObservedAtMS)
	}
	for _, x := range r.ScanRestarts {
		n = max(n, x.ObservedAtMS)
	}
	for _, x := range r.PriorInclusions {
		n = max(n, x.OrphanedAtMS, x.Inclusion.ObservedAtMS)
	}
	for _, x := range r.Conflicts {
		n = max(n, x.ObservedAtMS)
	}
	return n
}
func (s *Store) audit(ctx context.Context, m marker) error {
	raw, ok, e := s.raw([]byte("meta"), MaxMetadataSize)
	if e != nil || !ok {
		return ErrCorrupt
	}
	var meta metadata
	if decode(raw, MaxMetadataSize, &meta, true) != nil || !validMeta(meta, m.Pin, m.StoreID) || meta.SourceCreatedAtMS != s.sourceCreatedAtMS {
		return ErrCorrupt
	}
	index := []indexRow{}
	expected := map[string]reservation{}
	reserved := map[string]reservation{}
	funds := 0
	total := 0
	it := s.db.NewIterator(nil, nil)
	defer it.Release()
	for it.Next() {
		if ctx.Err() != nil {
			return ErrCancelled
		}
		k, v := it.Key(), it.Value()
		total += len(k) + len(v)
		if total > 64<<20 {
			return ErrCorrupt
		}
		if bytes.Equal(k, []byte("meta")) {
			continue
		}
		if len(k) == 0 {
			return ErrCorrupt
		}
		switch k[0] {
		case 'r':
			if len(index) >= MaxResults || len(k) != 33 || len(v) == 0 || len(v) > MaxRecordSize {
				return ErrCorrupt
			}
			var r Record
			if decode(v, MaxRecordSize, &r, true) != nil || validateRecord(m.Pin, r) != nil || !bytes.Equal(k, key('r', r.ResultKey)) || r.PreparedAtMS < meta.CreatedAtMS || r.Statement.Statement.Source.StoreCreatedAtMS != meta.SourceCreatedAtMS || recordMaxTime(r) > meta.ClockMS {
				return ErrCorrupt
			}
			index = append(index, indexFor(r))
			if r.Publication != nil {
				key := string(coinKey('u', r.Publication.Coin.Outpoint))
				if _, ok := expected[key]; ok {
					return ErrCorrupt
				}
				expected[key] = reservation{1, r.ResultKey, r.Publication.TxID}
			}
		case 'c':
			funds++
			if funds > MaxFundingCoins || len(k) != 37 || len(v) > 1024 {
				return ErrCorrupt
			}
			var c FundingCoin
			if decode(v, 1024, &c, true) != nil || validateFundingCoin(m.Pin, c) != nil || !bytes.Equal(k, coinKey('c', c.Outpoint)) || int64(c.BlockHeight) > int64(meta.FundingCursor.Height) || int64(c.BlockHeight) == int64(meta.FundingCursor.Height) && c.BlockHash != meta.FundingCursor.Hash {
				return ErrCorrupt
			}
		case 'u':
			if len(reserved) >= MaxReservations || len(k) != 37 || len(v) > 256 {
				return ErrCorrupt
			}
			var u reservation
			if decode(v, 256, &u, true) != nil || u.Version != 1 || !hashID(u.ResultKey) || !hashID(u.TxID) {
				return ErrCorrupt
			}
			reserved[string(k)] = u
		default:
			return ErrCorrupt
		}
	}
	if it.Error() != nil || len(index) != int(meta.ResultCount) || len(expected) != len(reserved) || !indexBound(index) {
		return ErrCorrupt
	}
	for k, w := range expected {
		if reserved[k] != w {
			return ErrCorrupt
		}
	}
	sort.Slice(index, func(i, j int) bool { return index[i].ResultKey < index[j].ResultKey })
	s.meta = meta
	s.clockSeen = meta.ClockMS
	s.index = index
	return nil
}
func (s *Store) funding(ctx context.Context) ([]FundingCoin, error) {
	out := []FundingCoin{}
	it := s.db.NewIterator(nil, nil)
	defer it.Release()
	for ok := it.Seek([]byte{'c'}); ok && len(it.Key()) > 0 && it.Key()[0] == 'c'; ok = it.Next() {
		if ctx.Err() != nil {
			return nil, ErrCancelled
		}
		var c FundingCoin
		if len(out) >= MaxFundingCoins || len(it.Key()) != 37 || decode(it.Value(), 1024, &c, true) != nil || validateFundingCoin(s.meta.Pin, c) != nil || !bytes.Equal(it.Key(), coinKey('c', c.Outpoint)) {
			return nil, ErrCorrupt
		}
		out = append(out, c)
	}
	if it.Error() != nil {
		return nil, ErrCorrupt
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Outpoint.TxID == out[j].Outpoint.TxID {
			return out[i].Outpoint.Index < out[j].Outpoint.Index
		}
		return out[i].Outpoint.TxID < out[j].Outpoint.TxID
	})
	return out, nil
}
