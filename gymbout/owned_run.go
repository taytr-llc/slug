// Copyright (c) 2026 The SLUG developers. ISC license.
package gymbout

import (
	"context"
	"encoding/json"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/fighterbout"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/internal/ownedcodec"
	"sort"
)

func (s *OwnedStore) Run(ctx context.Context, cards *gymcard.OwnedStore, seeds *gymseed.OwnedStore, events *gymevent.OwnedStore, entries *gymentry.OwnedStore, session gymstable.Session, date, id string) (OwnedRunResult, error) {
	if !ownedCallsValid(ctx, cards, seeds, events, entries) || !hashID(id) {
		return OwnedRunResult{}, ErrInvalid
	}
	work, done, e := s.begin(ctx)
	if e != nil {
		return OwnedRunResult{}, e
	}
	defer done()
	prior, page, e := s.observe(work, cards, seeds, events, entries, session, date, id, true)
	if e != nil {
		return OwnedRunResult{}, e
	}
	if !page.Source.CardExists {
		return OwnedRunResult{}, ErrSourceIncomplete
	}
	status, e := ownedSelectedStatus(page)
	if e != nil {
		return OwnedRunResult{}, e
	}
	if status.Saved.Exists {
		return OwnedRunResult{Version: 1, Record: prior, Status: status, Replayed: true}, nil
	}
	if !status.Source.Usable {
		return OwnedRunResult{}, ErrSourceConflict
	}
	if s.meta.BoutCount >= MaxSavedBouts {
		return OwnedRunResult{}, ErrCapacity
	}
	var out OwnedRunResult
	mutated := false
	e = cards.WithBoutSource(work, seeds, events, entries, session, date, id, func(src gymcard.OwnedBoutSource) (func(gymcard.OwnedBoutView, func() error) error, error) {
		if work.Err() != nil {
			return nil, ErrCancelled
		}
		in, e := DeriveOwnedInput(s.meta.Pin, src)
		if e != nil {
			return nil, ErrCorrupt
		}
		if ownedSourceDigest(src) != status.Saved.SourceDigest {
			return nil, ErrSourceChanged
		}
		s.mark("before-engine")
		result, e := fighterbout.Simulate(work, in)
		if e != nil {
			if work.Err() != nil {
				return nil, ErrCancelled
			}
			return nil, ErrUnavailable
		}
		s.mark("after-engine")
		r := OwnedRecord{Version: 1, StoreID: s.meta.StoreID, CardID: src.CardID, BoutID: id, Source: src, SourceDigest: ownedSourceDigest(src), Input: in, InputID: inputDigest(in), Result: result, ResultDigest: resultDigest(result), CreatedAtMS: MaxTimestampMS}
		// Engine-ownedValue result: bounded shape/byte preflight, never public validation
		// or Digest/MarshalResult (which would execute the engine again).
		if !ownedcodec.Fits(r, OwnedMaxRecordSize) || r.Result.InputID != r.InputID {
			return nil, ErrCorrupt
		}
		b, e := json.Marshal(r)
		if e != nil || len(b) > OwnedMaxRecordSize {
			return nil, ErrCapacity
		}
		rawInput, _ := json.Marshal(in)
		if len(rawInput) > MaxInputSize {
			return nil, ErrCapacity
		}
		nextIndex := append(make([]ownedIndexRow, 0, len(s.index)+1), s.index...)
		nextIndex = append(nextIndex, ownedIndexFor(r))
		sort.Slice(nextIndex, func(i, j int) bool { return nextIndex[i].BoutID < nextIndex[j].BoutID })
		if !ownedIndexBound(nextIndex) {
			return nil, ErrCapacity
		}
		s.mark("run-prepared")
		return func(v gymcard.OwnedBoutView, check func() error) error {
			if !s.mu.TryLock() {
				return ErrBusy
			}
			defer s.mu.Unlock()
			if e := s.stateLocked(); e != nil {
				return e
			}
			if e := s.checkView(v); e != nil {
				return e
			}
			if !ownedSameState(src, v.State) {
				return ErrSourceChanged
			}
			if !v.State.Usable {
				return ErrSourceConflict
			}
			if s.indexed(id) != nil {
				return ErrSourceChanged
			}
			if s.meta.BoutCount >= MaxSavedBouts {
				return ErrCapacity
			}
			if e := check(); e != nil {
				return e
			}
			if s.life.Err() != nil {
				return ErrClosed
			}
			if work.Err() != nil {
				return ErrCancelled
			}
			n, e := s.sampleLocked(max(v.ObservedAtMS, src.CardCreatedAtMS))
			if e != nil {
				return e
			}
			r.CreatedAtMS = n
			raw, e := json.Marshal(r)
			if e != nil || len(raw) > OwnedMaxRecordSize {
				return ErrCapacity
			}
			idx := ownedIndexFor(r)
			for i := range nextIndex {
				if nextIndex[i].BoutID == id {
					nextIndex[i] = idx
				}
			}
			batch := new(leveldb.Batch)
			batch.Put(key('r', id), raw)
			next := s.meta
			next.ClockMS = n
			next.BoutCount++
			s.mark("before-result-sync")
			if e := check(); e != nil {
				return e
			}
			if work.Err() != nil || s.life.Err() != nil {
				return ErrCancelled
			}
			mutated = true
			if e = s.commitLocked(batch, next); e != nil {
				return e
			}
			s.index = nextIndex
			s.mark("result-synced")
			state := v.State
			state.ObservedAtMS = n
			state.EscalationDue = n >= state.Event.IncidentDeadlineMS && (state.FinalizedAtMS == 0 || state.ConflictCount > 0 || state.ConflictOverflow)
			out = OwnedRunResult{Version: 1, Record: r, Status: OwnedStatus{Version: 1, StoreID: s.meta.StoreID, CardID: r.CardID, ObservedAtMS: n, Source: state, Saved: ownedSavedFor(gymcard.BoutRef{Bout: src.Bout, SourceDigest: r.SourceDigest}, &r, true)}}
			return nil
		}, nil
	})
	if e != nil || work.Err() != nil || s.life.Err() != nil {
		if mutated {
			return OwnedRunResult{}, ErrOutcomeUnknown
		}
		if e != nil {
			return OwnedRunResult{}, ownedMapped(e)
		}
		if s.life.Err() != nil {
			return OwnedRunResult{}, ErrClosed
		}
		return OwnedRunResult{}, ErrCancelled
	}
	return out, nil
}
