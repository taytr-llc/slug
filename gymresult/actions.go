// Copyright (c) 2026 The SLUG developers. ISC license.
package gymresult

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"errors"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/gymstable"
)

type sourceCall struct {
	ctx           context.Context
	bouts         *gymbout.Store
	cards         *gymcard.Store
	seeds         *gymseed.Store
	events        *gymevent.Store
	entries       *gymentry.Store
	session       gymstable.Session
	date, id, key string
}

func (s *Store) call(c sourceCall) (sourceCall, func(), error) {
	if c.ctx == nil || c.bouts == nil || c.cards == nil || c.seeds == nil || c.events == nil || c.entries == nil || !hashID(c.id) {
		return c, nil, ErrInvalid
	}
	work, done, e := s.begin(c.ctx)
	if e != nil {
		return c, nil, e
	}
	c.ctx = work
	ev, e := gymevent.EventForDate(s.meta.Pin.SourcePin.SourcePin.SourcePin.SourcePin, c.date)
	if e != nil {
		done()
		return c, nil, ErrCalendar
	}
	cid, e := gymcard.CardID(s.meta.Pin.SourcePin.SourcePin, ev)
	if e != nil {
		done()
		return c, nil, ErrInvalid
	}
	c.key = resultKey(s.meta.Pin, cid, c.id)
	return c, done, nil
}
func (s *Store) sourceView(c sourceCall, cap gymaccount.Capability, fn func(gymbout.ResultView, func() error) error) error {
	return mapped(c.bouts.WithResultView(c.ctx, c.cards, c.seeds, c.events, c.entries, c.session, cap, c.date, c.id, func(v gymbout.ResultView, check func() error) error {
		if e := s.checkView(v); e != nil {
			return e
		}
		if cap == gymaccount.ResultPublish && (v.ParticipantAccountID != s.meta.Pin.PublisherAccountID || v.ParticipantGymID != s.meta.Pin.PublisherGymID) {
			return ErrForbidden
		}
		return fn(v, check)
	}))
}
func (s *Store) guard(c sourceCall, check func() error) error {
	if e := check(); e != nil {
		return e
	}
	if s.life.Err() != nil {
		return ErrClosed
	}
	if c.ctx.Err() != nil {
		return ErrCancelled
	}
	return s.stateLocked()
}
func (s *Store) finish(c sourceCall, e error, mutated bool) error {
	// The full source body's final guard is the success linearization point.
	// A Close that starts after it must not revoke an already completed result.
	if e == nil {
		s.mark("guarded-body-complete")
		return nil
	}
	if mutated {
		return ErrOutcomeUnknown
	}
	e = mapped(e)
	if s.life.Err() != nil && (errors.Is(e, ErrCancelled) || errors.Is(e, context.Canceled) || errors.Is(e, context.DeadlineExceeded)) {
		return ErrClosed
	}
	return e
}
func (s *Store) fundingState(ctx context.Context) (string, error) {
	if s.meta.LastFundingTip == nil {
		return "unknown", nil
	}
	if !s.meta.FundingComplete {
		return "scanning", nil
	}
	coins, e := s.funding(ctx)
	if e != nil {
		return "", e
	}
	for _, coin := range coins {
		_, reserved, e := s.raw(coinKey('u', coin.Outpoint), 256)
		if e != nil {
			return "", e
		}
		if !reserved && mature(coin, *s.meta.LastFundingTip) && coin.ValueSatoshis >= 1294 {
			return "available", nil
		}
	}
	return "waiting_for_funds", nil
}
func mature(c FundingCoin, t Tip) bool {
	return t.Height >= c.BlockHeight && (!c.Coinbase || uint64(t.Height)+1-uint64(c.BlockHeight) >= 100) && uint64(t.Height)+1 >= 62
}
func (s *Store) read(c sourceCall, cap gymaccount.Capability, required bool) (Record, Status, error) {
	var r Record
	var out Status
	mutated := false
	var funding string
	e := s.sourceView(c, cap, func(v gymbout.ResultView, check func() error) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		if e := s.guard(c, check); e != nil {
			return e
		}
		row, ok, e := s.record(c.key)
		if e != nil {
			return e
		}
		funding, e = s.fundingState(c.ctx)
		if e != nil {
			return e
		}
		if ok && (v.Source == nil || !equal(row.Statement.Statement.Source, *v.Source)) {
			return ErrSourceChanged
		}
		before := s.meta.ClockMS
		n, e := s.observeLocked(v.ObservedAtMS)
		mutated = mutated || s.meta.ClockMS != before || e == ErrOutcomeUnknown
		if e != nil {
			return e
		}
		ss := v.Status
		ss.ObservedAtMS = n
		ss.Source.ObservedAtMS = n
		ss.Source.EscalationDue = n >= ss.Source.Event.IncidentDeadlineMS && (ss.Source.FinalizedAtMS == 0 || ss.Source.ConflictCount > 0 || ss.Source.ConflictOverflow)
		var rp *Record
		if ok {
			r = row
			rp = &r
		}
		out = statusFor(s.meta.Pin, rp, ss, funding)
		s.mark("read-observed")
		return s.guard(c, check)
	})
	if e = s.finish(c, e, mutated); e != nil {
		return Record{}, Status{}, e
	}
	if required && !out.Exists {
		return Record{}, Status{}, ErrNotPrepared
	}
	return r, out, nil
}

// save validates owned state before entering the original account lease. The
// final callback only checks, stamps bounded times, encodes and synchronizes.
func (s *Store) save(c sourceCall, cap gymaccount.Capability, r Record, next metadata, b *leveldb.Batch, stamp func(*Record, *metadata, int64), healthy bool) (ActionResult, error) {
	if validateRecord(s.meta.Pin, r) != nil || !fits(r, MaxRecordSize) {
		return ActionResult{}, ErrInvalid
	}
	if b == nil {
		b = new(leveldb.Batch)
	}
	committed := false
	var out ActionResult
	funding, e := s.projectFunding(c.ctx, next, b)
	if e != nil {
		return out, e
	}
	e = s.sourceView(c, cap, func(v gymbout.ResultView, check func() error) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		if e := s.guard(c, check); e != nil {
			return e
		}
		if v.Source == nil || !equal(*v.Source, r.Statement.Statement.Source) {
			return ErrSourceChanged
		}
		if healthy && !v.Status.Saved.Usable {
			return ErrSourceConflict
		}
		n, e := s.sampleLocked(max(v.ObservedAtMS, recordMaxTime(r)))
		if e != nil {
			return e
		}
		if stamp != nil {
			stamp(&r, &next, n)
		}
		next.ClockMS = n
		careerIndex, e := s.proposedCareerIndex(r)
		if e != nil {
			return e
		}
		if e = put(b, key('r', r.ResultKey), r, MaxRecordSize); e != nil {
			return e
		}
		s.mark("before-record-sync")
		if e = s.commitLocked(b, next); e != nil {
			return e
		}
		s.installIndex(r)
		s.careerIndex = careerIndex
		committed = true
		s.mark("record-synced")
		ss := v.Status
		ss.ObservedAtMS = n
		ss.Source.ObservedAtMS = n
		ss.Source.EscalationDue = n >= ss.Source.Event.IncidentDeadlineMS && (ss.Source.FinalizedAtMS == 0 || ss.Source.ConflictCount > 0 || ss.Source.ConflictOverflow)
		out = ActionResult{1, r, statusFor(s.meta.Pin, &r, ss, funding)}
		return s.guard(c, check)
	})
	if e = s.finish(c, e, committed); e != nil {
		return ActionResult{}, e
	}
	return out, nil
}
func (s *Store) Status(ctx context.Context, bouts *gymbout.Store, cards *gymcard.Store, seeds *gymseed.Store, events *gymevent.Store, entries *gymentry.Store, session gymstable.Session, saturday, boutID string) (Status, error) {
	c, done, e := s.call(sourceCall{ctx: ctx, bouts: bouts, cards: cards, seeds: seeds, events: events, entries: entries, session: session, date: saturday, id: boutID})
	if e != nil {
		return Status{}, e
	}
	defer done()
	r, st, e := s.read(c, gymaccount.View, false)
	_ = r
	return st, e
}
func (s *Store) Read(ctx context.Context, bouts *gymbout.Store, cards *gymcard.Store, seeds *gymseed.Store, events *gymevent.Store, entries *gymentry.Store, session gymstable.Session, saturday, boutID string) (ReadResult, error) {
	c, done, e := s.call(sourceCall{ctx: ctx, bouts: bouts, cards: cards, seeds: seeds, events: events, entries: entries, session: session, date: saturday, id: boutID})
	if e != nil {
		return ReadResult{}, e
	}
	defer done()
	r, st, e := s.read(c, gymaccount.View, false)
	if e != nil {
		return ReadResult{}, e
	}
	if !st.Exists {
		return ReadResult{}, ErrNotFound
	}
	return ReadResult{1, r, st}, nil
}
func (s *Store) Prepare(ctx context.Context, bouts *gymbout.Store, cards *gymcard.Store, seeds *gymseed.Store, events *gymevent.Store, entries *gymentry.Store, session gymstable.Session, saturday, boutID string) (ActionResult, error) {
	c, done, e := s.call(sourceCall{ctx: ctx, bouts: bouts, cards: cards, seeds: seeds, events: events, entries: entries, session: session, date: saturday, id: boutID})
	if e != nil {
		return ActionResult{}, e
	}
	defer done()
	return s.prepare(c)
}
func (s *Store) prepare(c sourceCall) (ActionResult, error) {
	var out ActionResult
	committed := false
	e := c.bouts.WithResultSource(c.ctx, c.cards, c.seeds, c.events, c.entries, c.session, c.date, c.id, func(src gymbout.ResultSource) (func(gymbout.ResultView, func() error) error, error) {
		if src.StoreID != s.meta.Pin.SourceStoreID {
			return nil, ErrForbidden
		}
		if src.StoreCreatedAtMS != s.meta.SourceCreatedAtMS {
			return nil, ErrSourceChanged
		}
		r, exists, e := s.record(c.key)
		if e != nil {
			return nil, e
		}
		if exists {
			if !equal(r.Statement.Statement.Source, src) {
				return nil, ErrSourceChanged
			}
		} else {
			if s.meta.ResultCount >= MaxResults {
				return nil, ErrCapacity
			}
			statement := expectedStatement(s.meta.Pin, src)
			signed := SignedStatement{1, statement, hex.EncodeToString(ed25519.Sign(fixtureKey(false), statementMessage(statement)))}
			r = Record{Version: 1, StoreID: s.meta.StoreID, ResultKey: c.key, Statement: signed, PreparedAtMS: max(s.now().UnixMilli(), src.CreatedAtMS, s.meta.CreatedAtMS), Cursor: initialCursor(s.meta.Pin, c.key, 0), ScanRestarts: []ScanRestart{}, PriorInclusions: []PriorInclusion{}, Conflicts: []Conflict{}, Observation: Observation{NodeState: "unknown"}}
			if validateRecord(s.meta.Pin, r) != nil {
				return nil, ErrInvalid
			}
		}
		funding, e := s.fundingState(c.ctx)
		if e != nil {
			return nil, e
		}
		return func(v gymbout.ResultView, check func() error) error {
			if e := s.checkView(v); e != nil {
				return e
			}
			if v.ParticipantAccountID != s.meta.Pin.PublisherAccountID || v.ParticipantGymID != s.meta.Pin.PublisherGymID {
				return ErrForbidden
			}
			s.mu.Lock()
			defer s.mu.Unlock()
			if e := s.guard(c, check); e != nil {
				return e
			}
			n, e := s.sampleLocked(max(v.ObservedAtMS, r.PreparedAtMS))
			if e != nil {
				return e
			}
			next := s.meta
			next.ClockMS = n
			b := new(leveldb.Batch)
			if !exists {
				r.PreparedAtMS = n
				next.ResultCount++
				if e = put(b, key('r', r.ResultKey), r, MaxRecordSize); e != nil {
					return e
				}
			}
			if !exists || next.ClockMS != s.meta.ClockMS {
				s.mark("before-prepare")
				if e = s.commitLocked(b, next); e != nil {
					return e
				}
				s.installIndex(r)
				committed = true
				s.mark("prepared")
			}
			ss := v.Status
			ss.ObservedAtMS = n
			ss.Source.ObservedAtMS = n
			out = ActionResult{1, r, statusFor(s.meta.Pin, &r, ss, funding)}
			return s.guard(c, check)
		}, nil
	})
	if e = s.finish(c, e, committed); e != nil {
		return ActionResult{}, e
	}
	return out, nil
}
