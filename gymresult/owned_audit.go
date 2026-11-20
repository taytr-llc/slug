// Copyright (c) 2026 The SLUG developers. ISC license.
package gymresult

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/taytr-llc/slug/internal/careerstate"
	"sort"
)

func ownedIndexFor(r OwnedRecord) indexRow {
	return indexRow{r.ResultKey, r.Statement.Statement.Source.Source.Event.Saturday, r.Statement.Statement.Source.Source.Bout.BoutID, ownedRecordDigest(r)}
}
func (s *OwnedStore) indexed(id string) *indexRow {
	for i := range s.index {
		if s.index[i].ResultKey == id {
			return &s.index[i]
		}
	}
	return nil
}
func (s *OwnedStore) record(id string) (OwnedRecord, bool, error) {
	row := s.indexed(id)
	raw, ok, e := s.raw(key('r', id), OwnedMaxRecordSize)
	if e != nil {
		return OwnedRecord{}, false, e
	}
	if !ok {
		if row != nil {
			return OwnedRecord{}, false, ErrCorrupt
		}
		return OwnedRecord{}, false, nil
	}
	if row == nil || hash("SLUG owned result record\x00v1\x00", lp(raw)) != row.RecordDigest {
		return OwnedRecord{}, false, ErrCorrupt
	}
	var r OwnedRecord
	if json.Unmarshal(raw, &r) != nil || r.ResultKey != id {
		return OwnedRecord{}, false, ErrCorrupt
	}
	s.mark("retained-row-checked")
	return r, true, nil
}
func ownedIndexBound(rows []indexRow) bool {
	return len(rows) <= MaxResults && fits(rows, MaxIndexSize)
}
func (s *OwnedStore) installIndex(r OwnedRecord) {
	x := ownedIndexFor(r)
	for i := range s.index {
		if s.index[i].ResultKey == x.ResultKey {
			s.index[i] = x
			return
		}
	}
	s.index = append(s.index, x)
	sort.Slice(s.index, func(i, j int) bool { return s.index[i].ResultKey < s.index[j].ResultKey })
}
func ownedValidMeta(m ownedMetadata, p OwnedPin, id string) bool {
	if (m.Version != 1 && m.Version != 2) || m.Version == 1 && m.Career != nil || m.Version == 2 && (m.Career == nil || careerstate.ValidateActivation(*m.Career) != nil || m.Career.ResultStoreID != m.StoreID || m.Career.ResultCreatedAtMS != m.CreatedAtMS || m.Career.SourceCreatedAtMS != m.SourceCreatedAtMS || m.Career.ActivatedAtMS > m.ClockMS) || m.StoreID != id || !equal(m.Pin, p) || !ownedTimestamp(m.SourceCreatedAtMS) || !ownedTimestamp(m.CreatedAtMS) || !ownedTimestamp(m.ClockMS) || m.SourceCreatedAtMS > m.CreatedAtMS || m.CreatedAtMS > m.ClockMS || m.ResultCount > MaxResults || ownedValidateCursor(p, m.FundingCursor) != nil || m.FundingRestarts == nil || len(m.FundingRestarts) > 16 || uint32(len(m.FundingRestarts)) != m.FundingCursor.Generation {
		return false
	}
	if m.FundingCursor.Height < 0 && !equal(m.FundingCursor, ownedInitialCursor(p, "", m.FundingCursor.Generation)) {
		return false
	}
	n := m.CreatedAtMS
	for i, x := range m.FundingRestarts {
		if ownedValidateScanRestart(p, x) != nil || x.Generation != uint32(i+1) || x.ObservedAtMS < n || x.ObservedAtMS > m.ClockMS {
			return false
		}
		n = x.ObservedAtMS
	}
	if m.LastFundingTip != nil && ownedValidateTip(p, *m.LastFundingTip) != nil {
		return false
	}
	return !m.FundingComplete || (m.LastFundingTip != nil && int64(m.LastFundingTip.Height) == int64(m.FundingCursor.Height) && m.LastFundingTip.Hash == m.FundingCursor.Hash)
}
func ownedRecordMaxTime(r OwnedRecord) int64 {
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
func (s *OwnedStore) audit(ctx context.Context, m ownedMarker) error {
	raw, ok, e := s.raw([]byte("meta"), OwnedMaxMetadataSize)
	if e != nil || !ok {
		return ErrCorrupt
	}
	var meta ownedMetadata
	if ownedDecodeCareerMetadata(raw, OwnedMaxMetadataSize, &meta, true) != nil || !ownedValidMeta(meta, m.Pin, m.StoreID) || meta.SourceCreatedAtMS != s.sourceCreatedAtMS {
		return ErrCorrupt
	}
	if !s.careerOpening && (m.Version != 1 || meta.Version != 1) {
		return ErrCorrupt
	}
	careerEvents := []careerstate.Event{}
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
		if bytes.Equal(k, []byte("career_init")) {
			if !s.careerOpening {
				return ErrCorrupt
			}
			var intent ownedCareerIntent
			if ownedDecode(v, 2048, &intent, true) != nil || intent != ownedIntentFor(meta) {
				return ErrCorrupt
			}
			continue
		}
		if len(k) == 0 {
			return ErrCorrupt
		}
		switch k[0] {
		case 'r':
			if len(index) >= MaxResults || len(k) != 33 || len(v) == 0 || len(v) > OwnedMaxRecordSize {
				return ErrCorrupt
			}
			var r OwnedRecord
			if ownedDecode(v, OwnedMaxRecordSize, &r, true) != nil || ownedValidateRecord(m.Pin, r) != nil || !bytes.Equal(k, key('r', r.ResultKey)) || r.PreparedAtMS < meta.CreatedAtMS || r.Statement.Statement.Source.StoreCreatedAtMS != meta.SourceCreatedAtMS || ownedRecordMaxTime(r) > meta.ClockMS {
				return ErrCorrupt
			}
			index = append(index, ownedIndexFor(r))
			if meta.Career != nil && r.Finality != nil {
				event, e := ownedCareerEvent(*meta.Career, r)
				if e != nil {
					return e
				}
				careerEvents = append(careerEvents, event)
				s.mark("career-audit-row")
			}
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
			if ownedDecode(v, 1024, &c, true) != nil || ownedValidateFundingCoin(m.Pin, c) != nil || !bytes.Equal(k, coinKey('c', c.Outpoint)) || int64(c.BlockHeight) > int64(meta.FundingCursor.Height) || int64(c.BlockHeight) == int64(meta.FundingCursor.Height) && c.BlockHash != meta.FundingCursor.Hash {
				return ErrCorrupt
			}
		case 'u':
			if len(reserved) >= MaxReservations || len(k) != 37 || len(v) > 256 {
				return ErrCorrupt
			}
			var u reservation
			if ownedDecode(v, 256, &u, true) != nil || u.Version != 1 || !hashID(u.ResultKey) || !hashID(u.TxID) {
				return ErrCorrupt
			}
			reserved[string(k)] = u
		default:
			return ErrCorrupt
		}
	}
	if it.Error() != nil || len(index) != int(meta.ResultCount) || len(expected) != len(reserved) || !ownedIndexBound(index) {
		return ErrCorrupt
	}
	for k, w := range expected {
		if reserved[k] != w {
			return ErrCorrupt
		}
	}
	sort.Slice(index, func(i, j int) bool { return index[i].ResultKey < index[j].ResultKey })
	if meta.Career != nil {
		var e error
		careerEvents, e = careerstate.ValidateIndex(*meta.Career, careerEvents)
		if e != nil {
			return ErrCorrupt
		}
	}
	s.careerIndex = careerEvents
	s.meta = meta
	s.clockSeen = meta.ClockMS
	s.index = index
	return nil
}
func (s *OwnedStore) funding(ctx context.Context) ([]FundingCoin, error) {
	out := []FundingCoin{}
	it := s.db.NewIterator(nil, nil)
	defer it.Release()
	for ok := it.Seek([]byte{'c'}); ok && len(it.Key()) > 0 && it.Key()[0] == 'c'; ok = it.Next() {
		if ctx.Err() != nil {
			return nil, ErrCancelled
		}
		// The durable activation intent shares the coin prefix. Only its exact
		// key and validated active-store identity are ownedMetadata, never a coin.
		if bytes.Equal(it.Key(), []byte("career_init")) {
			var intent ownedCareerIntent
			if s.meta.Version != 2 || s.meta.Career == nil || careerstate.ValidateActivation(*s.meta.Career) != nil || ownedDecode(it.Value(), 2048, &intent, true) != nil || intent != ownedIntentFor(s.meta) {
				return nil, ErrCorrupt
			}
			continue
		}
		var c FundingCoin
		if len(out) >= MaxFundingCoins || len(it.Key()) != 37 || ownedDecode(it.Value(), 1024, &c, true) != nil || ownedValidateFundingCoin(s.meta.Pin, c) != nil || !bytes.Equal(it.Key(), coinKey('c', c.Outpoint)) {
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
