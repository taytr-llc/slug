// Copyright (c) 2026 The SLUG developers. ISC license.
package gymevent

import (
	"context"
	"github.com/syndtr/goleveldb/leveldb/util"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/internal/ownedcodec"
	"sort"
)

func (s *OwnedStore) status(ev Event, now int64) (OwnedEventStatus, error) {
	row, exists, e := s.event(ev)
	if e != nil {
		return OwnedEventStatus{}, e
	}
	out := OwnedEventStatus{Version: 1, StoreID: s.meta.StoreID, Event: ev, Exists: exists, Phase: "before_cutoff", CapturedParticipants: []string{}, CoordinatorCreatedAtMS: s.meta.CreatedAtMS, ObservedAtMS: now, SeedAuthority: "unavailable"}
	if now >= ev.CutoffMS {
		out.Phase = "awaiting_capture"
	}
	if exists {
		out.Phase = row.Phase
		out.CreatedAtMS = row.CreatedAtMS
		out.CompletedAtMS = row.CompletedAtMS
		out.SnapshotDigest = row.SnapshotDigest
		for _, p := range s.meta.Pin.Participants {
			id, _ := OwnedParticipantID(s.meta.Pin, p)
			_, ok, e := s.raw(key('c', ev.EventID, id), OwnedMaxParticipantSize+4096)
			if e != nil {
				return out, e
			}
			if ok {
				out.CapturedParticipants = append(out.CapturedParticipants, id)
			}
		}
		sort.Strings(out.CapturedParticipants)
	}
	return ValidateOwnedEventStatus(s.meta.Pin, out)
}
func ValidateOwnedEventStatus(p OwnedPin, v OwnedEventStatus) (OwnedEventStatus, error) {
	id, e := OwnedID(p)
	ev, ee := OwnedEventForDate(p, v.Event.Saturday)
	if e != nil || ee != nil || v.Version != 1 || v.StoreID != id || v.Event != ev || v.CapturedParticipants == nil || len(v.CapturedParticipants) > 2 || !optionalID(v.SnapshotDigest) || !timestamp(v.CoordinatorCreatedAtMS) || v.ObservedAtMS < v.CoordinatorCreatedAtMS || !timestamp(v.ObservedAtMS) || v.SeedAuthority != "unavailable" {
		return v, ErrInvalid
	}
	for n, id := range v.CapturedParticipants {
		if _, e := ownedParticipant(p, id); e != nil || n > 0 && v.CapturedParticipants[n-1] >= id {
			return v, ErrInvalid
		}
	}
	if !v.Exists {
		phase := "before_cutoff"
		if v.ObservedAtMS >= ev.CutoffMS {
			phase = "awaiting_capture"
		}
		if v.Phase != phase || len(v.CapturedParticipants) != 0 || v.CreatedAtMS != 0 || v.CompletedAtMS != 0 || v.SnapshotDigest != "" {
			return v, ErrInvalid
		}
	} else {
		if !timestamp(v.CreatedAtMS) || v.CreatedAtMS < ev.CutoffMS || v.CreatedAtMS > v.ObservedAtMS || v.CoordinatorCreatedAtMS >= ev.CutoffMS {
			return v, ErrInvalid
		}
		switch v.Phase {
		case "awaiting_capture":
			if len(v.CapturedParticipants) != 0 {
				return v, ErrInvalid
			}
		case "capturing":
			if len(v.CapturedParticipants) != 1 {
				return v, ErrInvalid
			}
		case "snapshotted":
			if len(v.CapturedParticipants) != 2 || !hashID(v.SnapshotDigest) || v.CompletedAtMS < v.CreatedAtMS || v.CompletedAtMS > v.ObservedAtMS {
				return v, ErrInvalid
			}
		default:
			return v, ErrInvalid
		}
		if v.Phase != "snapshotted" && (v.CompletedAtMS != 0 || v.SnapshotDigest != "") {
			return v, ErrInvalid
		}
	}
	v.CapturedParticipants = append([]string{}, v.CapturedParticipants...)
	return v, nil
}
func (s *OwnedStore) Observe(ctx context.Context, entry *gymentry.OwnedStore, session gymstable.Session, saturday string) (OwnedEventStatus, error) {
	ctx, done, e := s.begin(ctx)
	if e != nil {
		return OwnedEventStatus{}, e
	}
	defer done()
	ev, e := OwnedEventForDate(s.meta.Pin, saturday)
	if e != nil {
		return OwnedEventStatus{}, e
	}
	var now int64
	e = s.sourceView(ctx, entry, session, func(info gymentry.OwnedSnapshotSourceInfo, check func() error) error {
		var e error
		now, e = s.clock(info.ObservedAtMS)
		if e != nil {
			return e
		}
		return check()
	})
	if e != nil {
		return OwnedEventStatus{}, e
	}
	return s.status(ev, now)
}
func (s *OwnedStore) Read(ctx context.Context, entry *gymentry.OwnedStore, session gymstable.Session, saturday string) (OwnedSnapshotReceipt, error) {
	ctx, done, e := s.begin(ctx)
	if e != nil {
		return OwnedSnapshotReceipt{}, e
	}
	defer done()
	ev, e := OwnedEventForDate(s.meta.Pin, saturday)
	if e != nil {
		return OwnedSnapshotReceipt{}, e
	}
	if e = s.sourceView(ctx, entry, session, func(_ gymentry.OwnedSnapshotSourceInfo, check func() error) error { return check() }); e != nil {
		return OwnedSnapshotReceipt{}, e
	}
	v, ok, e := s.receipt(ev)
	if e != nil {
		return v, e
	}
	if !ok {
		return v, ErrIncomplete
	}
	e = s.sourceView(ctx, entry, session, func(_ gymentry.OwnedSnapshotSourceInfo, check func() error) error { return check() })
	if e != nil {
		return OwnedSnapshotReceipt{}, e
	}
	return v, nil
}
func (s *OwnedStore) History(ctx context.Context, entry *gymentry.OwnedStore, session gymstable.Session, after string, limit uint32) (OwnedHistoryPage, error) {
	if !optionalID(after) || limit < 1 || limit > 8 {
		return OwnedHistoryPage{}, ErrInvalid
	}
	ctx, done, e := s.begin(ctx)
	if e != nil {
		return OwnedHistoryPage{}, e
	}
	defer done()
	if e = s.sourceView(ctx, entry, session, func(_ gymentry.OwnedSnapshotSourceInfo, check func() error) error { return check() }); e != nil {
		return OwnedHistoryPage{}, e
	}
	rows := make([]eventRow, 0, s.meta.EventCount)
	it := s.db.NewIterator(util.BytesPrefix([]byte{'e'}), nil)
	for it.Next() {
		if len(rows) >= 256 || len(it.Value()) > MaxEventSize {
			it.Release()
			return OwnedHistoryPage{}, ErrCorrupt
		}
		var r eventRow
		if ownedcodec.Decode(it.Value(), MaxEventSize, &r) != nil {
			it.Release()
			return OwnedHistoryPage{}, ErrCorrupt
		}
		rows = append(rows, r)
	}
	e = it.Error()
	it.Release()
	if e != nil || len(rows) != int(s.meta.EventCount) {
		return OwnedHistoryPage{}, ErrCorrupt
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Event.CutoffMS < rows[j].Event.CutoffMS })
	start := 0
	if after != "" {
		found := false
		for n, r := range rows {
			if r.Event.EventID == after {
				start = n + 1
				found = true
				break
			}
		}
		if !found {
			return OwnedHistoryPage{}, ErrNotFound
		}
	}
	now, e := s.clock(0)
	if e != nil {
		return OwnedHistoryPage{}, e
	}
	out := OwnedHistoryPage{Version: 1, StoreID: s.meta.StoreID, AfterEventID: after, ObservedAtMS: now, Events: []OwnedEventStatus{}}
	end := min(len(rows), start+int(limit))
	for _, r := range rows[start:end] {
		v, e := s.status(r.Event, now)
		if e != nil {
			return out, e
		}
		out.Events = append(out.Events, v)
	}
	out.HasMore = end < len(rows)
	if out.HasMore {
		out.NextEventID = rows[end-1].Event.EventID
	}
	e = s.sourceView(ctx, entry, session, func(_ gymentry.OwnedSnapshotSourceInfo, check func() error) error { return check() })
	if e != nil {
		return OwnedHistoryPage{}, e
	}
	return out, nil
}
