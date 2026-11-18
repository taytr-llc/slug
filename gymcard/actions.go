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

func (s *Store) authorize(ctx context.Context, seeds *gymseed.Store, events *gymevent.Store, entries *gymentry.Store, session gymstable.Session) error {
	return mapped(seeds.WithCardView(ctx, events, entries, session, []string{}, func(v gymseed.CardView, check func() error) error {
		if e := s.checkView(v); e != nil {
			return e
		}
		return check()
	}))
}
func callValid(ctx context.Context, seeds *gymseed.Store, events *gymevent.Store, entries *gymentry.Store) bool {
	return ctx != nil && seeds != nil && events != nil && entries != nil
}
func (s *Store) Build(ctx context.Context, seeds *gymseed.Store, events *gymevent.Store, entries *gymentry.Store, session gymstable.Session, date string) (BuildResult, error) {
	if !callValid(ctx, seeds, events, entries) {
		return BuildResult{}, ErrInvalid
	}
	work, done, e := s.begin(ctx)
	if e != nil {
		return BuildResult{}, e
	}
	defer done()
	ev, e := gymevent.EventForDate(s.meta.Pin.SourcePin.SourcePin, date)
	if e != nil {
		return BuildResult{}, ErrCalendar
	}
	id, _ := CardID(s.meta.Pin, ev)
	if e = s.authorize(work, seeds, events, entries, session); e != nil {
		return BuildResult{}, e
	}
	r, ok, e := s.record(id)
	if e != nil {
		return BuildResult{}, e
	}
	if ok {
		// Validation belongs before the final disclosure lease, and replay preserves
		// the exact retained Plan and creation time even after source conflict.
		statuses, e := s.observe(work, seeds, events, entries, session, []gymevent.Event{ev}, []*Record{&r})
		if e != nil {
			return BuildResult{}, e
		}
		return BuildResult{1, r, statuses[0], true}, nil
	}
	if s.meta.EventCount >= MaxEvents {
		return BuildResult{}, ErrCapacity
	}
	if e = s.career.Ready(); e != nil {
		return BuildResult{}, e
	}
	var out BuildResult
	mutated := false
	e = seeds.WithCardSource(work, events, entries, session, date, func(source gymseed.CardSource) (func(gymseed.CardView, func() error) error, error) {
		plan, e := DerivePlan(s.meta.Pin, source)
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
		digest, e := gymseed.CardSourceDigest(s.meta.Pin.SourcePin, source)
		if e != nil {
			return nil, ErrCorrupt
		}
		r := Record{1, s.meta.StoreID, id, source, digest, plan, max(s.meta.CreatedAtMS, source.Finality.ObservedAtMS)}
		r, e = ValidateRecord(s.meta.Pin, r)
		if e != nil {
			return nil, ErrCorrupt
		}
		probe := r
		probe.CreatedAtMS = MaxTimestampMS
		raw, e := json.Marshal(probe)
		if e != nil || len(raw) > MaxRecordSize {
			return nil, ErrCapacity
		}
		nextIndex := make([]indexRow, len(s.index)+1)
		copy(nextIndex, s.index)
		nextIndex[len(s.index)] = indexRow{id: id, cutoff: ev.CutoffMS}
		sortIndex(nextIndex)
		s.mark("build-prepared")
		return func(v gymseed.CardView, check func() error) error {
			s.mu.Lock()
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
			if !usable(v.States[0]) {
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
			if e != nil || len(raw) > MaxRecordSize {
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
			out = BuildResult{Version: 1, Record: r, Status: statusFor(s.meta.Pin, ev, &r, v.States[0], n), Replayed: false}
			return nil
		}, nil
	})
	if e != nil || work.Err() != nil {
		if mutated {
			return BuildResult{}, ErrOutcomeUnknown
		}
		if e != nil {
			return BuildResult{}, mapped(e)
		}
		return BuildResult{}, ErrCancelled
	}
	return out, nil
}
func statusFor(p Pin, ev gymevent.Event, r *Record, source gymseed.CardState, n int64) Status {
	// These inputs are already semantically validated outside the final lease.
	// The fixed byte recipes below perform no source/name/condition regeneration.
	raw, _ := json.Marshal(p)
	store := hash("SLUG candidate card store\x00v1\x00", lp(raw))
	id := hash("SLUG candidate card event\x00v1\x00", rawID(store), rawID(ev.EventID))
	v := Status{Version: 1, StoreID: store, CardID: id, Event: ev, ObservedAtMS: n, Source: source}
	if r != nil {
		v.Exists = true
		v.RecordDigest = recordDigest(*r)
		v.CreatedAtMS = r.CreatedAtMS
		v.Counts = r.Plan.Counts
		v.BoutCount = r.Plan.BoutCount
		v.Usable = usable(source) && source.SourceDigest == r.SourceDigest
	}
	return v
}
func (s *Store) observe(ctx context.Context, seeds *gymseed.Store, events *gymevent.Store, entries *gymentry.Store, session gymstable.Session, evs []gymevent.Event, records []*Record) ([]Status, error) {
	dates := make([]string, len(evs))
	for i, x := range evs {
		dates[i] = x.Saturday
	}
	out := []Status{}
	mutated := false
	e := seeds.WithCardView(ctx, events, entries, session, dates, s.observeCommit(ctx, evs, records, &out, &mutated))
	if e = observationError(ctx, e, mutated); e != nil {
		return nil, e
	}
	return out, nil
}

func (s *Store) observeCommit(ctx context.Context, evs []gymevent.Event, records []*Record, out *[]Status, mutated *bool) func(gymseed.CardView, func() error) error {
	return func(v gymseed.CardView, check func() error) error {
		s.mu.Lock()
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
			*out = append(*out, statusFor(s.meta.Pin, ev, records[i], v.States[i], n))
		}
		s.mark("read-observed")
		return nil
	}
}

func observationError(ctx context.Context, e error, mutated bool) error {
	if e != nil || ctx.Err() != nil {
		if mutated {
			return ErrOutcomeUnknown
		}
		if e != nil {
			return mapped(e)
		}
		return ErrCancelled
	}
	return nil
}
