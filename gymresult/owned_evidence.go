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

func (s *OwnedStore) ObserveCheckpoint(ctx context.Context, bouts *gymbout.OwnedStore, cards *gymcard.OwnedStore, seeds *gymseed.OwnedStore, events *gymevent.OwnedStore, entries *gymentry.OwnedStore, session gymstable.Session, saturday, boutID string, cp chaincfg.SignedCheckpoint) (OwnedActionResult, error) {
	c, done, e := s.call(ownedSourceCall{ctx: ctx, bouts: bouts, cards: cards, seeds: seeds, events: events, entries: entries, session: session, date: saturday, id: boutID})
	if e != nil {
		return OwnedActionResult{}, e
	}
	defer done()
	if !ownedValidCheckpoint(s.meta.Pin, cp) {
		return OwnedActionResult{}, ErrInvalid
	}
	cp.Signature = append([]byte(nil), cp.Signature...)
	r, st, e := s.read(c, gymaccount.View, true)
	if e != nil {
		return OwnedActionResult{}, e
	}
	if r.Inclusion == nil || cp.Height != int32(r.Inclusion.Anchor.Height) {
		return OwnedActionResult{}, ErrInvalid
	}
	if r.Checkpoint != nil && cp.Hash == r.Checkpoint.Hash {
		return OwnedActionResult{1, r, st}, nil
	}
	oldConflicts := len(r.Conflicts)
	n := max(s.now().UnixMilli(), s.meta.ClockMS)
	if cp.Hash.String() != r.Inclusion.Anchor.Hash {
		ownedAddConflict(&r, OwnedConflict{Kind: "checkpoint_equivocation", ObservedAtMS: n, ExpectedHeight: r.Inclusion.Anchor.Height, ExpectedHash: r.Inclusion.Anchor.Hash, ObservedHash: cp.Hash.String(), Checkpoint: &cp})
	} else {
		r.Checkpoint = &cp
	}
	r.Observation.ObservedAtMS = n
	return s.save(c, gymaccount.View, r, s.meta, nil, func(r *OwnedRecord, _ *ownedMetadata, n int64) {
		r.Observation.ObservedAtMS = n
		for i := oldConflicts; i < len(r.Conflicts); i++ {
			r.Conflicts[i].ObservedAtMS = n
		}
	}, false)
}
func (s *OwnedStore) ObserveEquivocation(ctx context.Context, bouts *gymbout.OwnedStore, cards *gymcard.OwnedStore, seeds *gymseed.OwnedStore, events *gymevent.OwnedStore, entries *gymentry.OwnedStore, session gymstable.Session, saturday, boutID string, statement OwnedSignedStatement) (OwnedActionResult, error) {
	c, done, e := s.call(ownedSourceCall{ctx: ctx, bouts: bouts, cards: cards, seeds: seeds, events: events, entries: entries, session: session, date: saturday, id: boutID})
	if e != nil {
		return OwnedActionResult{}, e
	}
	defer done()
	statement, e = ownedValue(statement, OwnedMaxSignedStatementSize, func(v OwnedSignedStatement) error { return ownedValidateEvidenceStatement(s.meta.Pin, v) })
	if e != nil {
		return OwnedActionResult{}, e
	}
	r, st, e := s.read(c, gymaccount.View, true)
	if e != nil {
		return OwnedActionResult{}, e
	}
	if statement.Statement.ResultKey != r.ResultKey || statement.Statement.Source.Source.CardID != r.Statement.Statement.Source.Source.CardID || statement.Statement.Source.Source.Bout.BoutID != r.Statement.Statement.Source.Source.Bout.BoutID {
		return OwnedActionResult{}, ErrInvalid
	}
	if equal(statement.Statement, r.Statement.Statement) {
		return OwnedActionResult{1, r, st}, nil
	}
	n := max(s.now().UnixMilli(), s.meta.ClockMS)
	if r.Equivocation != nil {
		if equal(r.Equivocation.Statement, statement.Statement) {
			return OwnedActionResult{1, r, st}, nil
		}
		r.ConflictOverflow = true
	} else {
		r.Equivocation = &statement
		a, _ := OwnedStatementID(r.Statement.Statement)
		b, _ := OwnedStatementID(statement.Statement)
		ownedAddConflict(&r, OwnedConflict{Kind: "statement_equivocation", ObservedAtMS: n, ExpectedHash: a, ObservedHash: b})
	}
	r.Observation.CurrentFinality = false
	r.Observation.ObservedAtMS = n
	return s.save(c, gymaccount.View, r, s.meta, nil, func(r *OwnedRecord, _ *ownedMetadata, n int64) { r.Observation.ObservedAtMS = n }, false)
}
