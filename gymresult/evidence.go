// Copyright (c) 2026 The SLUG developers. ISC license.
package gymresult

import (
	"context"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/gymstable"
)

func (s *Store) ObserveCheckpoint(ctx context.Context, bouts *gymbout.Store, cards *gymcard.Store, seeds *gymseed.Store, events *gymevent.Store, entries *gymentry.Store, session gymstable.Session, saturday, boutID string, cp chaincfg.SignedCheckpoint) (ActionResult, error) {
	c, done, e := s.call(sourceCall{ctx: ctx, bouts: bouts, cards: cards, seeds: seeds, events: events, entries: entries, session: session, date: saturday, id: boutID})
	if e != nil {
		return ActionResult{}, e
	}
	defer done()
	if !validCheckpoint(s.meta.Pin, cp) {
		return ActionResult{}, ErrInvalid
	}
	cp.Signature = append([]byte(nil), cp.Signature...)
	r, st, e := s.read(c, gymaccount.View, true)
	if e != nil {
		return ActionResult{}, e
	}
	if r.Inclusion == nil || cp.Height != int32(r.Inclusion.Anchor.Height) {
		return ActionResult{}, ErrInvalid
	}
	if r.Checkpoint != nil && cp.Hash == r.Checkpoint.Hash {
		return ActionResult{1, r, st}, nil
	}
	oldConflicts := len(r.Conflicts)
	n := max(s.now().UnixMilli(), s.meta.ClockMS)
	if cp.Hash.String() != r.Inclusion.Anchor.Hash {
		addConflict(&r, Conflict{Kind: "checkpoint_equivocation", ObservedAtMS: n, ExpectedHeight: r.Inclusion.Anchor.Height, ExpectedHash: r.Inclusion.Anchor.Hash, ObservedHash: cp.Hash.String(), Checkpoint: &cp})
	} else {
		r.Checkpoint = &cp
	}
	r.Observation.ObservedAtMS = n
	return s.save(c, gymaccount.View, r, s.meta, nil, func(r *Record, _ *metadata, n int64) {
		r.Observation.ObservedAtMS = n
		for i := oldConflicts; i < len(r.Conflicts); i++ {
			r.Conflicts[i].ObservedAtMS = n
		}
	}, false)
}
func (s *Store) ObserveEquivocation(ctx context.Context, bouts *gymbout.Store, cards *gymcard.Store, seeds *gymseed.Store, events *gymevent.Store, entries *gymentry.Store, session gymstable.Session, saturday, boutID string, statement SignedStatement) (ActionResult, error) {
	c, done, e := s.call(sourceCall{ctx: ctx, bouts: bouts, cards: cards, seeds: seeds, events: events, entries: entries, session: session, date: saturday, id: boutID})
	if e != nil {
		return ActionResult{}, e
	}
	defer done()
	statement, e = owned(statement, MaxSignedStatementSize, func(v SignedStatement) error { return validateEvidenceStatement(s.meta.Pin, v) })
	if e != nil {
		return ActionResult{}, e
	}
	r, st, e := s.read(c, gymaccount.View, true)
	if e != nil {
		return ActionResult{}, e
	}
	if statement.Statement.ResultKey != r.ResultKey || statement.Statement.Source.Source.CardID != r.Statement.Statement.Source.Source.CardID || statement.Statement.Source.Source.Bout.BoutID != r.Statement.Statement.Source.Source.Bout.BoutID {
		return ActionResult{}, ErrInvalid
	}
	if equal(statement.Statement, r.Statement.Statement) {
		return ActionResult{1, r, st}, nil
	}
	n := max(s.now().UnixMilli(), s.meta.ClockMS)
	if r.Equivocation != nil {
		if equal(r.Equivocation.Statement, statement.Statement) {
			return ActionResult{1, r, st}, nil
		}
		r.ConflictOverflow = true
	} else {
		r.Equivocation = &statement
		a, _ := StatementID(r.Statement.Statement)
		b, _ := StatementID(statement.Statement)
		addConflict(&r, Conflict{Kind: "statement_equivocation", ObservedAtMS: n, ExpectedHash: a, ObservedHash: b})
	}
	r.Observation.CurrentFinality = false
	r.Observation.ObservedAtMS = n
	return s.save(c, gymaccount.View, r, s.meta, nil, func(r *Record, _ *metadata, n int64) { r.Observation.ObservedAtMS = n }, false)
}
