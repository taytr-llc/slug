// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterownership

import (
	"context"
	"math"

	"github.com/syndtr/goleveldb/leveldb"
)

// WorkID is the original immutable work identity, including across an exact
// close/reopen resume. It is correlation data, not a verified ownership claim.
func (r *Replay) WorkID() string {
	if r == nil {
		return ""
	}
	return r.id
}

// InspectReplay exposes only retained public progress. It never turns partial
// verification or an old cached map into a ready head.
func (s *Store) InspectReplay(ctx context.Context) (Progress, error) {
	if ctx == nil || s == nil {
		return Progress{}, ErrInvalid
	}
	if e := ctx.Err(); e != nil {
		return Progress{}, e
	}
	if !s.mu.TryLock() {
		return Progress{}, ErrBusy
	}
	defer s.mu.Unlock()
	if e := s.state(); e != nil {
		return Progress{}, e
	}
	if s.work == nil {
		return Progress{}, ErrNotFound
	}
	w := *s.work
	return Progress{WorkID: w.WorkID, Phase: w.Phase, NextAdmission: w.NextAdmission, NextPublication: w.NextPublication, NextHeight: w.NextHeight}, nil
}

// AbandonReplay requires the exact named work after its active Replay has been
// closed/joined. Only resumable work metadata is removed; all immutable evidence
// remains. A new BeginReplay must verify the new source/node target completely.
func (s *Store) AbandonReplay(ctx context.Context, workID string) error {
	if !hexValue(workID, 32, true) {
		return ErrInvalid
	}
	ctx, done, e := s.mutation(ctx)
	if e != nil {
		return e
	}
	defer done()
	release, e := s.project(ctx)
	if e != nil {
		return e
	}
	defer release()
	if !s.mu.TryLock() {
		return ErrBusy
	}
	defer s.mu.Unlock()
	if e = s.state(); e != nil {
		return e
	}
	if s.work == nil {
		return ErrNotFound
	}
	if s.work.WorkID != workID || s.meta.WorkID != workID {
		return ErrConflict
	}
	if s.generation == math.MaxUint64 {
		return ErrCapacity
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	batch := new(leveldb.Batch)
	batch.Delete([]byte("work"))
	meta := s.meta
	meta.WorkID = ""
	if e = s.commit(batch, meta); e != nil {
		return e
	}
	s.work = nil
	s.ready = false
	s.generation++
	return ctx.Err()
}
