// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"context"
	"encoding/json"

	"github.com/taytr-llc/slug/fightercareer"
	"github.com/taytr-llc/slug/fightercondition"
	"github.com/taytr-llc/slug/fightermanifest"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/internal/careerstate"
	"github.com/taytr-llc/slug/registry"
)

const MaxCareerPreviewPairSize = 24576

// CareerPreviewFighter is a captured assignment and its already effective XP.
// It is deliberately separate from the legacy zero-XP Item.
type CareerPreviewFighter struct {
	Assignment Assignment                `json:"assignment"`
	Manifest   fightermanifest.Manifest  `json:"manifest"`
	Totals     fightercareer.Totals      `json:"totals"`
	Condition  fightercondition.Snapshot `json:"condition"`
}

type CareerPreviewPair struct {
	Version      uint32                  `json:"version"`
	Status       Status                  `json:"status"`
	Activation   careerstate.Activation  `json:"activation"`
	CareerDigest string                  `json:"career_digest"`
	AsOfMS       int64                   `json:"as_of_ms"`
	Fighters     [2]CareerPreviewFighter `json:"fighters"`
}

// CareerPreviewRead is an immutable, process-local capture, not an importable
// public verdict. Its concrete source and original owner/epoch remain required.
type CareerPreviewRead struct {
	owner      *Store
	source     *registry.ManifestStore
	generation uint64
	viewID     string
	raw        []byte
}

func (r *CareerPreviewRead) RetainedBytes() int {
	if r == nil {
		return 0
	}
	return len(r.raw) + careerReadOverhead
}

func careerPreviewBound(v any, limit int) bool {
	raw, e := json.Marshal(v)
	return e == nil && len(raw) <= limit
}

// PrepareCareerPreviewPair serially verifies two consumed proofs, then captures
// one Current prefix for both IDs under the original account publication lease.
// Only inherited clock/count/as-of observations are written; practice awards
// nothing and never establishes a noon fence.
func (s *Store) PrepareCareerPreviewPair(ctx context.Context, session Session, ids [2]string) (CareerPreviewPair, *CareerPreviewRead, error) {
	if s == nil || !hashID(ids[0]) || !hashID(ids[1]) || ids[0] == ids[1] {
		return CareerPreviewPair{}, nil, ErrInvalid
	}
	if ids[1] < ids[0] {
		ids[0], ids[1] = ids[1], ids[0]
	}
	ctx, done, e := s.begin(ctx, session, gymaccount.View)
	if e != nil {
		return CareerPreviewPair{}, nil, s.careerPreviewReadError(e)
	}
	defer done()
	// Ready alone permits legacy stores. This family requires an installed
	// Career binding even when all selected fighters happen to have zero XP.
	if !s.career.Required() {
		return CareerPreviewPair{}, nil, careerstate.ErrInactive
	}
	if e = s.career.Ready(); e != nil {
		return CareerPreviewPair{}, nil, s.careerPreviewReadError(e)
	}
	count, e := s.sourceInfo(ctx)
	if e != nil {
		return CareerPreviewPair{}, nil, s.careerPreviewReadError(e)
	}
	checkpoint, revision := s.meta.Checkpoint, s.meta.RosterRevision
	var assignments [2]Assignment
	for n, id := range ids {
		assignments[n], e = s.assignment(id)
		if e != nil {
			return CareerPreviewPair{}, nil, s.careerPreviewReadError(e)
		}
	}
	items := make([]Item, 2)
	for n, id := range ids {
		if e = s.withLease(ctx, session, gymaccount.View, func(check func() error) error { return check() }); e != nil {
			return CareerPreviewPair{}, nil, s.careerPreviewReadError(e)
		}
		items[n], e = s.readSource(ctx, assignments[n].Sequence, id)
		if e != nil {
			return CareerPreviewPair{}, nil, s.careerPreviewReadError(e)
		}
		if items[n].Assignment != assignments[n] {
			return CareerPreviewPair{}, nil, ErrCorrupt
		}
	}
	if items[0].Manifest.Physical.Class != items[1].Manifest.Physical.Class {
		return CareerPreviewPair{}, nil, ErrClassMismatch
	}
	var pair CareerPreviewPair
	var captured *CareerPreviewRead
	e = s.publishRead(ctx, session, items, checkpoint, revision, func(now int64, status Status) error {
		if status.ObservedSourceCount != count {
			return ErrConflict
		}
		view, e := s.career.Snapshot(ctx, ids[:], now, careerstate.Current)
		if e != nil {
			return e
		}
		activation := view.Activation()
		if now < activation.ActivatedAtMS {
			return ErrClock
		}
		pair = CareerPreviewPair{Version: Version1, Status: status, Activation: activation, CareerDigest: view.Digest(), AsOfMS: now}
		for n, item := range items {
			totals, ok := view.Totals(item.Assignment.ProspectID)
			if !ok {
				return ErrCorrupt
			}
			condition, e := fightercondition.Project(1, item.Manifest.DNA, item.Assignment.BornAtMS, now, int(totals.XP))
			if e != nil {
				return ErrCorrupt
			}
			pair.Fighters[n] = CareerPreviewFighter{item.Assignment, item.Manifest, totals, condition}
			if !careerPreviewBound(totals, 128) || !careerPreviewBound(pair.Fighters[n], 8448) {
				return ErrCapacity
			}
		}
		if !careerPreviewBound(status, 4096) || !careerPreviewBound(activation, 512) {
			return ErrCapacity
		}
		raw, e := json.Marshal(pair)
		if e != nil || len(raw) > MaxCareerPreviewPairSize {
			return ErrCapacity
		}
		captured = &CareerPreviewRead{owner: s, source: s.source, generation: session.Generation, viewID: session.ViewID, raw: raw}
		return nil
	})
	// The enclosing lease and observation sync can still fail after building
	// the value. Neither the capture nor the token escapes on any such failure.
	if e != nil {
		return CareerPreviewPair{}, nil, s.careerPreviewReadError(e)
	}
	return pair, captured, nil
}

// ReadCareerPreviewPair reauthorizes a captured pair without repeating proof
// work or replacing its as-of time/prefix. New unrelated admissions are allowed;
// old Career page handles keep their separate count-equality rule.
func (s *Store) ReadCareerPreviewPair(ctx context.Context, session Session, captured *CareerPreviewRead) (CareerPreviewPair, error) {
	if s == nil || captured == nil || captured.owner != s || captured.source != s.source || len(captured.raw) == 0 || len(captured.raw) > MaxCareerPreviewPairSize {
		return CareerPreviewPair{}, ErrInvalid
	}
	if captured.generation != session.Generation || captured.viewID != session.ViewID {
		return CareerPreviewPair{}, ErrStaleView
	}
	ctx, done, e := s.begin(ctx, session, gymaccount.View)
	if e != nil {
		return CareerPreviewPair{}, s.careerPreviewReadError(e)
	}
	defer done()
	var pair CareerPreviewPair
	if e = json.Unmarshal(captured.raw, &pair); e != nil {
		return CareerPreviewPair{}, ErrCorrupt
	}
	count, e := s.sourceInfo(ctx)
	if e != nil {
		return CareerPreviewPair{}, s.careerPreviewReadError(e)
	}
	if count < pair.Status.ObservedSourceCount {
		return CareerPreviewPair{}, ErrConflict
	}
	e = s.withLease(ctx, session, gymaccount.View, func(check func() error) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		if e := s.stateLocked(); e != nil {
			return e
		}
		status := pair.Status
		if s.meta.StoreID != status.StoreID || s.meta.Pin != status.Pin || s.meta.Checkpoint != status.Checkpoint || s.meta.RosterRevision != status.RosterRevision || s.meta.ObservedSourceCount < status.ObservedSourceCount {
			return ErrConflict
		}
		ids := make([]string, 2)
		for n, f := range pair.Fighters {
			ids[n] = f.Assignment.ProspectID
			a, e := s.assignment(ids[n])
			if e != nil || a != f.Assignment {
				return ErrConflict
			}
		}
		// This is only a liveness/activation check. A same-millisecond award
		// can extend Current; the captured prefix and conditions stay frozen.
		live, e := s.career.Snapshot(ctx, ids, pair.AsOfMS, careerstate.Current)
		if e != nil {
			return e
		}
		if live.Activation() != pair.Activation {
			return ErrConflict
		}
		return check()
	})
	if e != nil {
		return CareerPreviewPair{}, s.careerPreviewReadError(e)
	}
	return pair, nil
}

// Only known cancellation identities from this read path can become owner
// closure. Unknown writes, poisoned state and foreign opaque errors keep their
// original precedence; a successful completed operation is never remapped.
func (s *Store) careerPreviewReadError(e error) error {
	if s != nil && s.life.Err() != nil && (e == ErrCancelled || e == careerstate.ErrCancelled) {
		return ErrClosed
	}
	return e
}
