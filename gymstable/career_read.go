// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"context"
	"encoding/json"

	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/internal/careerstate"
)

const careerReadOverhead = 512

// CareerRead is a process-local, immutable result of verified asynchronous page
// work. Its private owner and original session prevent importing a public page
// or reusing one after reopening the store or changing authentication epochs.
type CareerRead struct {
	owner      *Store
	roster     bool
	generation uint64
	viewID     string
	raw        []byte
}

// RetainedBytes charges the public composite plus a conservative fixed allowance.
// Callers must charge their separately retained terminal job as well.
func (r *CareerRead) RetainedBytes() int {
	if r == nil {
		return 0
	}
	return len(r.raw) + careerReadOverhead
}

func (s *Store) sealCareerRead(session Session, roster bool, page any) (*CareerRead, error) {
	raw, e := json.Marshal(page)
	if e != nil || len(raw) > MaxCareerPageSize {
		return nil, ErrCapacity
	}
	return &CareerRead{owner: s, roster: roster, generation: session.Generation, viewID: session.ViewID, raw: raw}, nil
}

// PrepareCareerInventory performs full proof verification and projection in the
// caller's bounded asynchronous work. The transient page retains private receipt
// data for the existing portrait publisher; the opaque handle contains only JSON.
func (s *Store) PrepareCareerInventory(ctx context.Context, session Session, after uint64, limit uint32) (CareerPage, *CareerRead, error) {
	p, e := s.CareerInventory(ctx, session, after, limit)
	if e != nil {
		return CareerPage{}, nil, e
	}
	r, e := s.sealCareerRead(session, false, p)
	if e != nil {
		return CareerPage{}, nil, e
	}
	return p, r, nil
}

// PrepareCareerSharedRecent seals the same original-session handle for the
// explicit shared source's latest local page. ReadCareerInventory redeems it.
func (s *Store) PrepareCareerSharedRecent(ctx context.Context, session Session, limit uint32) (CareerPage, *CareerRead, error) {
	p, e := s.CareerSharedRecent(ctx, session, limit)
	if e != nil {
		return CareerPage{}, nil, e
	}
	r, e := s.sealCareerRead(session, false, p)
	if e != nil {
		return CareerPage{}, nil, e
	}
	return p, r, nil
}

// PrepareCareerRoster is the roster equivalent of PrepareCareerInventory.
func (s *Store) PrepareCareerRoster(ctx context.Context, session Session, first, limit uint32) (CareerRosterPage, *CareerRead, error) {
	p, e := s.CareerRoster(ctx, session, first, limit)
	if e != nil {
		return CareerRosterPage{}, nil, e
	}
	r, e := s.sealCareerRead(session, true, p)
	if e != nil {
		return CareerRosterPage{}, nil, e
	}
	return p, r, nil
}

func (s *Store) checkCareerRead(session Session, r *CareerRead, roster bool) error {
	if s == nil || r == nil || r.owner != s || r.roster != roster || len(r.raw) == 0 || len(r.raw) > MaxCareerPageSize {
		return ErrInvalid
	}
	if r.generation != session.Generation || r.viewID != session.ViewID {
		return ErrStaleView
	}
	return nil
}

// ReadCareerInventory rechecks the original authority and source revisions, then
// returns a detached public copy of the captured page. It does not redo proof
// work, advance the page's observation time or replace its career prefix.
func (s *Store) ReadCareerInventory(ctx context.Context, session Session, r *CareerRead) (CareerPage, error) {
	if e := s.checkCareerRead(session, r, false); e != nil {
		return CareerPage{}, e
	}
	ctx, done, e := s.begin(ctx, session, gymaccount.View)
	if e != nil {
		return CareerPage{}, e
	}
	defer done()
	var p CareerPage
	if e = json.Unmarshal(r.raw, &p); e != nil {
		return CareerPage{}, ErrCorrupt
	}
	if e = s.recheckCareerPage(ctx, session, p.Source.Status, p.AsOfMS, p.Source.Items, p.Activation); e != nil {
		return CareerPage{}, e
	}
	return p, nil
}

// ReadCareerRoster is the roster equivalent of ReadCareerInventory.
func (s *Store) ReadCareerRoster(ctx context.Context, session Session, r *CareerRead) (CareerRosterPage, error) {
	if e := s.checkCareerRead(session, r, true); e != nil {
		return CareerRosterPage{}, e
	}
	ctx, done, e := s.begin(ctx, session, gymaccount.View)
	if e != nil {
		return CareerRosterPage{}, e
	}
	defer done()
	var p CareerRosterPage
	if e = json.Unmarshal(r.raw, &p); e != nil {
		return CareerRosterPage{}, ErrCorrupt
	}
	items := make([]Item, len(p.Source.Items))
	for i, x := range p.Source.Items {
		items[i] = x.Item
	}
	if e = s.recheckCareerPage(ctx, session, p.Source.Status, p.AsOfMS, items, p.Activation); e != nil {
		return CareerRosterPage{}, e
	}
	return p, nil
}

// The caller owns the Stable operation gate. Only the original account lease,
// small immutable assignments and the borrowed reader's liveness are consulted.
func (s *Store) recheckCareerPage(ctx context.Context, session Session, status Status, asOf int64, items []Item, activation careerstate.Activation) error {
	// Metadata-only dependency validation precedes the account/store locks.
	// A closed source or a newly observed admission count invalidates the token.
	count, e := s.sourceInfo(ctx)
	if e != nil {
		return e
	}
	if count != status.ObservedSourceCount {
		return ErrConflict
	}
	ids := careerPageIDs(items)
	return s.withLease(ctx, session, gymaccount.View, func(check func() error) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		if e := s.stateLocked(); e != nil {
			return e
		}
		if s.meta.StoreID != status.StoreID || s.meta.Checkpoint != status.Checkpoint || s.meta.RosterRevision != status.RosterRevision || s.meta.ObservedSourceCount != status.ObservedSourceCount {
			return ErrConflict
		}
		for _, item := range items {
			a, e := s.assignment(item.Assignment.ProspectID)
			if e != nil || a != item.Assignment {
				return ErrConflict
			}
		}
		// Current prefixes can change after capture, including in the same
		// millisecond. Verify liveness/activation without replacing that prefix.
		live, e := s.career.Snapshot(ctx, ids, asOf, careerstate.Current)
		if e != nil {
			return e
		}
		if live.Activation() != activation {
			return ErrConflict
		}
		return check()
	})
}
