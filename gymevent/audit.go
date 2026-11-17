// Copyright (c) 2026 The SLUG developers. ISC license.
package gymevent

import (
	"bytes"
	"context"
	"encoding/hex"
	"sort"
)

func (s *Store) event(ev Event) (eventRow, bool, error) {
	raw, ok, e := s.raw(key('e', ev.EventID), MaxEventSize)
	if e != nil || !ok {
		return eventRow{}, ok, e
	}
	var v eventRow
	if decode(raw, MaxEventSize, &v, true) != nil || v.Version != 1 || v.Event != ev {
		return v, false, ErrCorrupt
	}
	return v, true, nil
}
func (s *Store) capture(ev Event, id string) (ParticipantCapture, bool, error) {
	raw, ok, e := s.raw(key('c', ev.EventID, id), MaxCaptureSize)
	if e != nil || !ok {
		return ParticipantCapture{}, ok, e
	}
	var v ParticipantCapture
	if decode(raw, MaxCaptureSize, &v, true) != nil || v.Version != 1 || v.Snapshot.ParticipantID != id || v.Snapshot.Entry.Event.EventID != ev.SourceEventID {
		return v, false, ErrCorrupt
	}
	return v, true, nil
}
func (s *Store) receipt(ev Event) (SnapshotReceipt, bool, error) {
	raw, ok, e := s.raw(key('s', ev.EventID), MaxReceiptSize)
	if e != nil || !ok {
		return SnapshotReceipt{}, ok, e
	}
	var v SnapshotReceipt
	if decode(raw, MaxReceiptSize, &v, true) != nil || v.Version != 1 || v.Input.Event != ev {
		return v, false, ErrCorrupt
	}
	return v, true, nil
}
func (s *Store) rows(ctx context.Context) ([]eventRow, error) {
	out := make([]eventRow, 0, s.meta.EventCount)
	it := s.db.NewIterator(nil, nil)
	defer it.Release()
	for ok := it.Seek([]byte{'e'}); ok && len(it.Key()) > 0 && it.Key()[0] == 'e'; ok = it.Next() {
		if ctx.Err() != nil {
			return nil, ErrCancelled
		}
		if len(out) >= MaxEvents || len(it.Key()) != 33 || len(it.Value()) == 0 || len(it.Value()) > MaxEventSize {
			return nil, ErrCorrupt
		}
		var v eventRow
		if decode(it.Value(), MaxEventSize, &v, true) != nil || !bytes.Equal(it.Key(), key('e', v.Event.EventID)) {
			return nil, ErrCorrupt
		}
		out = append(out, v)
	}
	if it.Error() != nil || len(out) != int(s.meta.EventCount) {
		return nil, ErrCorrupt
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Event.CutoffMS == out[j].Event.CutoffMS {
			return out[i].Event.EventID < out[j].Event.EventID
		}
		return out[i].Event.CutoffMS < out[j].Event.CutoffMS
	})
	return out, nil
}
func (s *Store) status(ev Event, observed int64) (EventStatus, error) {
	v := EventStatus{Version: 1, StoreID: s.meta.StoreID, Event: ev, Phase: "before_cutoff", CapturedParticipants: []string{}, CoordinatorCreatedAtMS: s.meta.CreatedAtMS, ObservedAtMS: observed, SeedAuthority: "unavailable"}
	if observed >= ev.CutoffMS {
		v.Phase = "awaiting_capture"
	}
	r, ok, e := s.event(ev)
	if e != nil {
		return v, e
	}
	if !ok {
		return v, nil
	}
	v.Exists = true
	v.Phase = r.Phase
	v.CreatedAtMS = r.CreatedAtMS
	v.CompletedAtMS = r.CompletedAtMS
	v.SnapshotDigest = r.SnapshotDigest
	for _, p := range s.meta.Pin.Participants {
		id := p.StablePin.Identity.GymID
		_, exists, e := s.raw(key('c', ev.EventID, id), MaxCaptureSize)
		if e != nil {
			return v, e
		}
		if exists {
			v.CapturedParticipants = append(v.CapturedParticipants, id)
		}
	}
	if validateEventStatus(s.meta.Pin, v) != nil {
		return EventStatus{}, ErrCorrupt
	}
	return v, nil
}

// audit is structural authority-store validation, not a replacement for source
// proof verification. It never invokes Argon, source reads, or portrait rendering.
func (s *Store) audit(ctx context.Context, m marker) error {
	it := s.db.NewIterator(nil, nil)
	counts := map[byte]int{}
	metaSeen := false
	for it.Next() {
		if ctx.Err() != nil {
			it.Release()
			return ErrCancelled
		}
		k, v := it.Key(), it.Value()
		if bytes.Equal(k, []byte("meta")) {
			if metaSeen || len(v) == 0 || len(v) > MaxMetadataSize {
				it.Release()
				return ErrCorrupt
			}
			metaSeen = true
			if decode(v, MaxMetadataSize, &s.meta, true) != nil {
				it.Release()
				return ErrCorrupt
			}
			continue
		}
		if len(k) == 0 {
			it.Release()
			return ErrCorrupt
		}
		limit, maxCount, keyLen := 0, 0, 0
		switch k[0] {
		case 'e':
			limit, maxCount, keyLen = MaxEventSize, MaxEvents, 33
		case 'c':
			limit, maxCount, keyLen = MaxCaptureSize, 2*MaxEvents, 65
		case 's':
			limit, maxCount, keyLen = MaxReceiptSize, MaxEvents, 33
		default:
			it.Release()
			return ErrCorrupt
		}
		counts[k[0]]++
		if len(k) != keyLen || len(v) == 0 || len(v) > limit || counts[k[0]] > maxCount {
			it.Release()
			return ErrCorrupt
		}
	}
	e := it.Error()
	it.Release()
	if e != nil || !metaSeen || s.meta.Version != 1 || s.meta.StoreID != m.StoreID || !equal(s.meta.Pin, m.Pin) || !timestamp(s.meta.CreatedAtMS) || s.meta.ClockMS < s.meta.CreatedAtMS || !timestamp(s.meta.ClockMS) || s.meta.EventCount > MaxEvents || counts['e'] != int(s.meta.EventCount) {
		return ErrCorrupt
	}
	rows, e := s.rows(ctx)
	if e != nil {
		return e
	}
	captures, receipts := 0, 0
	for _, r := range rows {
		if ctx.Err() != nil {
			return ErrCancelled
		}
		if validateEvent(s.meta.Pin, r.Event) != nil {
			return ErrCorrupt
		}
		status, e := s.status(r.Event, s.meta.ClockMS)
		if e != nil || !status.Exists {
			return ErrCorrupt
		}
		in := SnapshotInput{Version: 1, Event: r.Event, Participants: []ParticipantSnapshot{}}
		evidence := []CaptureEvidence{}
		for _, p := range s.meta.Pin.Participants {
			cap, ok, e := s.capture(r.Event, p.StablePin.Identity.GymID)
			if e != nil {
				return e
			}
			if !ok {
				continue
			}
			captures++
			if validateParticipantCapture(s.meta.Pin, cap) != nil || cap.Evidence.CapturedAtMS < r.CreatedAtMS || cap.Evidence.CapturedAtMS > s.meta.ClockMS {
				return ErrCorrupt
			}
			in.Participants = append(in.Participants, cap.Snapshot)
			evidence = append(evidence, cap.Evidence)
		}
		receipt, ok, e := s.receipt(r.Event)
		if e != nil {
			return e
		}
		if r.Phase == "snapshotted" {
			receipts++
			if !ok || validateSnapshotReceipt(s.meta.Pin, receipt) != nil || !equal(receipt.Input, in) || !equal(receipt.Captures, evidence) || receipt.SnapshotDigest != r.SnapshotDigest || receipt.CompletedAtMS != r.CompletedAtMS {
				return ErrCorrupt
			}
			// Atomic completion uses the second capture's exact actual timestamp.
			if receipt.CompletedAtMS != max(evidence[0].CapturedAtMS, evidence[1].CapturedAtMS) {
				return ErrCorrupt
			}
		} else if ok {
			return ErrCorrupt
		}
	}
	if captures != counts['c'] || receipts != counts['s'] {
		return ErrCorrupt
	}
	// Count equality plus exact expected keys rules out orphan/crosslinked rows.
	s.clockSeen = s.meta.ClockMS
	return nil
}

func keyID(raw []byte) string { return hex.EncodeToString(raw) }
