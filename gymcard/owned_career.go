// Copyright (c) 2026 The SLUG developers. ISC license.
package gymcard

import (
	"context"
	"sort"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/internal/careerconsumer"
	"github.com/taytr-llc/slug/internal/careerstate"
)

func OpenOwnedCareer(ctx context.Context, path string, expected OwnedPin, source *gymseed.OwnedStore, activation careerstate.Activation) (*OwnedStore, error) {
	return ownedOpenWithCareer(ctx, path, expected, source, time.Now, false, &activation, false)
}
func ResumeOwnedCareerBinding(ctx context.Context, path string, expected OwnedPin, source *gymseed.OwnedStore, activation careerstate.Activation) (*OwnedStore, error) {
	return ownedOpenWithCareer(ctx, path, expected, source, time.Now, false, &activation, true)
}
func (s *OwnedStore) installCareer(ctx context.Context, reader *careerstate.Reader, bind bool) error {
	if s == nil || reader == nil || ctx == nil {
		return ErrInvalid
	}
	work, done, e := s.begin(ctx)
	if e != nil {
		return e
	}
	defer done()
	s.mu.Lock()
	defer s.mu.Unlock()
	if e = s.stateLocked(); e != nil {
		return e
	}
	if work.Err() != nil {
		return ErrCancelled
	}
	if !bind {
		return s.career.Attach(reader, "card", s.meta.StoreID)
	}
	raw, install, e := s.career.PrepareBind(reader, "card", s.meta.StoreID)
	if e != nil {
		return e
	}
	if raw != nil {
		b := new(leveldb.Batch)
		b.Put([]byte(careerconsumer.GuardKey), raw)
		s.mark("before-career-bind")
		if e = s.writeBatch(b, &opt.WriteOptions{Sync: true}); e != nil {
			s.poisoned = true
			return ErrOutcomeUnknown
		}
		install()
		s.mark("career-bound")
		if work.Err() != nil {
			return ErrOutcomeUnknown
		}
	} else {
		install()
	}
	return nil
}
func (s *OwnedStore) BindCareer(ctx context.Context, reader *careerstate.Reader) error {
	return s.installCareer(ctx, reader, true)
}
func (s *OwnedStore) AttachCareer(ctx context.Context, reader *careerstate.Reader) error {
	return s.installCareer(ctx, reader, false)
}
func (s *OwnedStore) careerSelection(ctx context.Context, plan Plan, cutoff int64) (careerstate.Snapshot, []string, error) {
	ids := make([]string, 0, len(plan.Entrants))
	for _, entrant := range plan.Entrants {
		ids = append(ids, entrant.ProspectID)
	}
	sort.Strings(ids)
	selected := ids
	if len(ids) == 0 {
		ids = []string{"0000000000000000000000000000000000000000000000000000000000000000"}
	}
	view, e := s.career.Snapshot(ctx, ids, cutoff, careerstate.Cutoff)
	if e != nil {
		return view, nil, e
	}
	if e = careerconsumer.RequireZero(view, selected); e != nil {
		return view, nil, e
	}
	return view, ids, nil
}

// OpenCareerAudit validates an established activated card archive read-only.
func OpenOwnedCareerAudit(ctx context.Context, path string, p OwnedPin, source *gymseed.OwnedStore, activation careerstate.Activation) (*OwnedStore, error) {
	return ownedOpenWithCareerAudit(ctx, path, p, source, time.Now, false, &activation, false, true)
}
