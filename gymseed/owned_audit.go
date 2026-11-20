// Copyright (c) 2026 The SLUG developers. ISC license.
package gymseed

import (
	"bytes"
	"context"
	"encoding/hex"
	"sort"
)

func (s *OwnedStore) record(id string) (OwnedRecord, bool, error) {
	raw, ok, e := s.raw(key('r', id), OwnedMaxRecordSize)
	if e != nil || !ok {
		return OwnedRecord{}, ok, e
	}
	var r OwnedRecord
	if ownedDecode(raw, OwnedMaxRecordSize, &r, true) != nil || r.EventKey != id || validateOwnedRecord(s.meta.Pin, r) != nil {
		return OwnedRecord{}, false, ErrCorrupt
	}
	return r, true, nil
}
func (s *OwnedStore) recordIDs(ctx context.Context) ([]string, error) {
	out := make([]string, 0, s.meta.EventCount)
	it := s.db.NewIterator(nil, nil)
	defer it.Release()
	for ok := it.Seek([]byte{'r'}); ok && len(it.Key()) > 0 && it.Key()[0] == 'r'; ok = it.Next() {
		if ctx.Err() != nil {
			return nil, ErrCancelled
		}
		if len(out) >= MaxEvents || len(it.Key()) != 33 || len(it.Value()) == 0 || len(it.Value()) > OwnedMaxRecordSize {
			return nil, ErrCorrupt
		}
		out = append(out, hex.EncodeToString(it.Key()[1:]))
	}
	if it.Error() != nil || len(out) != int(s.meta.EventCount) {
		return nil, ErrCorrupt
	}
	return out, nil
}
func (s *OwnedStore) funding(ctx context.Context) ([]FundingCoin, error) {
	out := []FundingCoin{}
	it := s.db.NewIterator(nil, nil)
	defer it.Release()
	for ok := it.Seek([]byte{'f'}); ok && len(it.Key()) > 0 && it.Key()[0] == 'f'; ok = it.Next() {
		if ctx.Err() != nil {
			return nil, ErrCancelled
		}
		if len(out) >= MaxFundingCoins || len(it.Key()) != 37 || len(it.Value()) == 0 || len(it.Value()) > 512 {
			return nil, ErrCorrupt
		}
		var c FundingCoin
		if ownedDecode(it.Value(), 512, &c, true) != nil || validateFundingCoin(physicalOwnedPin(s.meta.Pin), c) != nil || !bytes.Equal(it.Key(), coinKey('f', c.Outpoint)) {
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
func ownedValidMeta(m ownedMetadata, p OwnedPin, id string) bool {
	if m.Version != 1 || m.StoreID != id || !equal(m.Pin, p) || !ownedTimestamp(m.CreatedAtMS) || !ownedTimestamp(m.ClockMS) || m.ClockMS < m.CreatedAtMS || m.EventCount > MaxEvents || validateCursor(physicalOwnedPin(p), m.FundingCursor) != nil || m.FundingRestarts == nil || len(m.FundingRestarts) > 16 || uint32(len(m.FundingRestarts)) != m.FundingCursor.Generation {
		return false
	}
	n := m.CreatedAtMS
	for i, x := range m.FundingRestarts {
		if validateScanRestart(physicalOwnedPin(p), x) != nil || x.Generation != uint32(i+1) || x.ObservedAtMS < n || x.ObservedAtMS > m.ClockMS {
			return false
		}
		n = x.ObservedAtMS
	}
	if m.FundingCursor.Height < 0 && !equal(m.FundingCursor, initialOwnedCursor(p, "", m.FundingCursor.Generation)) {
		return false
	}
	if m.LastFundingTip == nil {
		return m.FundingCursor.Height < 0 && !m.FundingComplete
	}
	if validateTip(physicalOwnedPin(p), *m.LastFundingTip) != nil || m.FundingCursor.Height > int32(m.LastFundingTip.Height) {
		return false
	}
	return m.FundingComplete == (m.FundingCursor.Height == int32(m.LastFundingTip.Height) && m.FundingCursor.Hash == m.LastFundingTip.Hash)
}
func (s *OwnedStore) audit(ctx context.Context, m ownedMarker) error {
	raw, ok, e := s.raw([]byte("meta"), OwnedMaxMetadataSize)
	if e != nil || !ok {
		return ErrCorrupt
	}
	var meta ownedMetadata
	if ownedDecode(raw, OwnedMaxMetadataSize, &meta, true) != nil || !ownedValidMeta(meta, m.Pin, m.StoreID) || meta.CreatedAtMS < s.sourceCreatedAtMS {
		return ErrCorrupt
	}
	s.meta = meta
	s.clockSeen = meta.ClockMS
	it := s.db.NewIterator(nil, nil)
	defer it.Release()
	records := map[string]Outpoint{}
	reservations := map[string]string{}
	recordCount, fundCount := 0, 0
	total := 0
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
			recordCount++
			if recordCount > MaxEvents || len(k) != 33 || len(v) == 0 || len(v) > OwnedMaxRecordSize {
				return ErrCorrupt
			}
			var r OwnedRecord
			if ownedDecode(v, OwnedMaxRecordSize, &r, true) != nil || !bytes.Equal(k, key('r', r.EventKey)) || validateOwnedRecord(m.Pin, r) != nil || r.PreparedAtMS < meta.CreatedAtMS || meta.CreatedAtMS >= r.SourceReceipt.Input.Event.CutoffMS || ownedRecordMaxTime(r) > meta.ClockMS {
				return ErrCorrupt
			}
			if r.Publication != nil {
				records[r.EventKey] = r.Publication.Coin.Outpoint
			}
		case 'f':
			fundCount++
			if fundCount > MaxFundingCoins || len(k) != 37 || len(v) == 0 || len(v) > 512 {
				return ErrCorrupt
			}
			var c FundingCoin
			if ownedDecode(v, 512, &c, true) != nil || validateFundingCoin(physicalOwnedPin(m.Pin), c) != nil || !bytes.Equal(k, coinKey('f', c.Outpoint)) || int64(c.BlockHeight) > int64(meta.FundingCursor.Height) {
				return ErrCorrupt
			}
		case 'u':
			if len(reservations) >= MaxEvents || len(k) != 37 || len(v) != 32 {
				return ErrCorrupt
			}
			reservations[string(k)] = hex.EncodeToString(v)
		default:
			return ErrCorrupt
		}
	}
	if it.Error() != nil || recordCount != int(meta.EventCount) || len(records) != len(reservations) {
		return ErrCorrupt
	}
	for id, c := range records {
		if reservations[string(coinKey('u', c))] != id {
			return ErrCorrupt
		}
	}
	return nil
}
func ownedRecordMaxTime(r OwnedRecord) int64 {
	n := r.PreparedAtMS
	max := func(x int64) {
		if x > n {
			n = x
		}
	}
	max(r.IncidentAtMS)
	max(r.Observation.ObservedAtMS)
	if r.Publication != nil {
		max(r.Publication.BuiltAtMS)
		max(r.Publication.LastDispatchAtMS)
	}
	if r.Target != nil {
		max(r.Target.ObservedAtMS)
	}
	if r.Finality != nil {
		max(r.Finality.ObservedAtMS)
	}
	for _, x := range r.ScanRestarts {
		max(x.ObservedAtMS)
	}
	for _, x := range r.Conflicts {
		max(x.ObservedAtMS)
	}
	return n
}
