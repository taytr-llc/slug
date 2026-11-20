// Copyright (c) 2026 The SLUG developers. ISC license.
package gymcard

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/internal/careerconsumer"
	"github.com/taytr-llc/slug/internal/careerstate"
)

func (s *OwnedStore) authorize(ctx context.Context, seeds *gymseed.OwnedStore, events *gymevent.OwnedStore, entries *gymentry.OwnedStore, session gymstable.Session) error {
	return ownedMapped(seeds.WithCardView(ctx, events, entries, session, []string{}, func(v gymseed.OwnedCardView, check func() error) error {
		if e := s.checkView(v); e != nil {
			return e
		}
		return check()
	}))
}
func ownedCallValid(ctx context.Context, seeds *gymseed.OwnedStore, events *gymevent.OwnedStore, entries *gymentry.OwnedStore) bool {
	return ctx != nil && seeds != nil && events != nil && entries != nil
}
func (s *OwnedStore) Build(ctx context.Context, seeds *gymseed.OwnedStore, events *gymevent.OwnedStore, entries *gymentry.OwnedStore, session gymstable.Session, date string) (OwnedBuildResult, error) {
	if !ownedCallValid(ctx, seeds, events, entries) {
		return OwnedBuildResult{}, ErrInvalid
	}
	work, done, e := s.begin(ctx)
	if e != nil {
		return OwnedBuildResult{}, e
	}
	defer done()
	ev, e := gymevent.OwnedEventForDate(s.meta.Pin.SourcePin.SourcePin, date)
	if e != nil {
		return OwnedBuildResult{}, ErrCalendar
	}
	id, _ := OwnedCardID(s.meta.Pin, ev)
	if e = s.authorize(work, seeds, events, entries, session); e != nil {
		return OwnedBuildResult{}, e
	}
	r, ok, e := s.record(id)
	if e != nil {
		return OwnedBuildResult{}, e
	}
	if ok {
		// Validation belongs before the final disclosure lease, and replay preserves
		// the exact retained Plan and creation time even after source conflict.
		statuses, e := s.observe(work, seeds, events, entries, session, []gymevent.Event{ev}, []*OwnedRecord{&r})
		if e != nil {
			return OwnedBuildResult{}, e
		}
		return OwnedBuildResult{1, r, statuses[0], true}, nil
	}
	if s.meta.EventCount >= MaxEvents {
		return OwnedBuildResult{}, ErrCapacity
	}
	if e = s.career.Ready(); e != nil {
		return OwnedBuildResult{}, e
	}
	var out OwnedBuildResult
	mutated := false
	e = seeds.WithCardSource(work, events, entries, session, date, func(source gymseed.OwnedCardSource) (func(gymseed.OwnedCardView, func() error) error, error) {
		plan, e := DeriveOwnedPlan(s.meta.Pin, source)
		if e != nil {
			return nil, ErrCorrupt
		}
		var careerView careerstate.Snapshot
		var careerIDs []string
		if s.career.Required() {
			careerView, careerIDs, e = s.careerSelection(work, plan, ev.CutoffMS)
			if e != nil {
				return nil, e
			}
		}
		digest, e := gymseed.OwnedCardSourceDigest(s.meta.Pin.SourcePin, source)
		if e != nil {
			return nil, ErrCorrupt
		}
		r := OwnedRecord{1, s.meta.StoreID, id, source, digest, plan, max(s.meta.CreatedAtMS, source.Finality.ObservedAtMS)}
		r, e = ValidateOwnedRecord(s.meta.Pin, r)
		if e != nil {
			return nil, ErrCorrupt
		}
		probe := r
		probe.CreatedAtMS = MaxTimestampMS
		raw, e := json.Marshal(probe)
		if e != nil || len(raw) > OwnedMaxRecordSize {
			return nil, ErrCapacity
		}
		nextIndex := make([]indexRow, len(s.index)+1)
		copy(nextIndex, s.index)
		nextIndex[len(s.index)] = indexRow{id: id, cutoff: ev.CutoffMS}
		sortIndex(nextIndex)
		s.mark("build-prepared")
		return func(v gymseed.OwnedCardView, check func() error) error {
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
			if len(v.States) != 1 || v.States[0].SourceDigest != digest {
				return ErrSourceChanged
			}
			if !ownedUsable(v.States[0]) {
				return ErrSourceConflict
			}
			if e := check(); e != nil {
				return e
			}
			if work.Err() != nil {
				return ErrCancelled
			}
			if s.meta.EventCount >= MaxEvents {
				return ErrCapacity
			}
			n, e := s.sampleLocked(max(v.ObservedAtMS, r.CreatedAtMS))
			if e != nil {
				return e
			}
			r.CreatedAtMS = n
			b := new(leveldb.Batch)
			installCareerFence := func() {}
			if s.career.Required() {
				current, e := s.career.Snapshot(work, careerIDs, ev.CutoffMS, careerstate.Cutoff)
				if e != nil {
					return e
				}
				if current.Digest() != careerView.Digest() {
					return ErrSourceChanged
				}
				key, raw, guardRaw, install, e := s.career.PrepareFence(careerView, careerIDs)
				if e != nil {
					if e == careerconsumer.ErrCapacity {
						return ErrCapacity
					}
					return e
				}
				if raw != nil {
					b.Put([]byte(key), raw)
					b.Put([]byte(careerconsumer.GuardKey), guardRaw)
				}
				installCareerFence = install
			}
			raw, e := json.Marshal(r)
			if e != nil || len(raw) > OwnedMaxRecordSize {
				return ErrCapacity
			}
			b.Put(key('r', id), raw)
			for i := range nextIndex {
				if nextIndex[i].id == id {
					nextIndex[i].digest = sha256.Sum256(raw)
					break
				}
			}
			next := s.meta
			next.ClockMS = n
			next.EventCount++
			s.mark("before-card-sync")
			if e = check(); e != nil {
				return e
			}
			if work.Err() != nil {
				return ErrCancelled
			}
			mutated = true
			if e = s.commitLocked(b, next); e != nil {
				return e
			}
			s.index = nextIndex
			installCareerFence()
			s.mark("card-synced")
			out = OwnedBuildResult{Version: 1, Record: r, Status: ownedStatusFor(s.meta.Pin, ev, &r, v.States[0], n), Replayed: false}
			return nil
		}, nil
	})
	if e != nil || work.Err() != nil {
		if mutated {
			return OwnedBuildResult{}, ErrOutcomeUnknown
		}
		if e != nil {
			return OwnedBuildResult{}, ownedMapped(e)
		}
		return OwnedBuildResult{}, ErrCancelled
	}
	return out, nil
}
func ownedStatusFor(p OwnedPin, ev gymevent.Event, r *OwnedRecord, source gymseed.OwnedCardState, n int64) OwnedStatus {
	// These inputs are already semantically validated outside the final lease.
	// The fixed byte recipes below perform no source/name/condition regeneration.
	store, _ := OwnedID(p)
	id := ownedCardID(p, ev)
	v := OwnedStatus{Version: 1, StoreID: store, CardID: id, Event: ev, ObservedAtMS: n, Source: source}
	if r != nil {
		v.Exists = true
		v.RecordDigest = ownedRecordDigest(*r)
		v.CreatedAtMS = r.CreatedAtMS
		v.Counts = r.Plan.Counts
		v.BoutCount = r.Plan.BoutCount
		v.Usable = ownedUsable(source) && source.SourceDigest == r.SourceDigest
	}
	return v
}
func (s *OwnedStore) observe(ctx context.Context, seeds *gymseed.OwnedStore, events *gymevent.OwnedStore, entries *gymentry.OwnedStore, session gymstable.Session, evs []gymevent.Event, records []*OwnedRecord) ([]OwnedStatus, error) {
	dates := make([]string, len(evs))
	for i, x := range evs {
		dates[i] = x.Saturday
	}
	out := []OwnedStatus{}
	mutated := false
	e := seeds.WithCardView(ctx, events, entries, session, dates, s.observeCommit(ctx, evs, records, &out, &mutated))
	if e = observationError(ctx, e, mutated); e != nil {
		return nil, e
	}
	return out, nil
}

func (s *OwnedStore) observeCommit(ctx context.Context, evs []gymevent.Event, records []*OwnedRecord, out *[]OwnedStatus, mutated *bool) func(gymseed.OwnedCardView, func() error) error {
	return func(v gymseed.OwnedCardView, check func() error) error {
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
		if len(v.States) != len(evs) {
			return ErrCorrupt
		}
		for i, r := range records {
			if r != nil && v.States[i].SourceDigest != r.SourceDigest {
				return ErrSourceChanged
			}
		}
		if e := check(); e != nil {
			return e
		}
		if ctx.Err() != nil {
			return ErrCancelled
		}
		before := s.meta.ClockMS
		n, e := s.observeLocked(v.ObservedAtMS)
		*mutated = *mutated || s.meta.ClockMS != before || e == ErrOutcomeUnknown
		if e != nil {
			return e
		}
		for i, ev := range evs {
			*out = append(*out, ownedStatusFor(s.meta.Pin, ev, records[i], v.States[i], n))
		}
		s.mark("read-observed")
		return nil
	}
}
