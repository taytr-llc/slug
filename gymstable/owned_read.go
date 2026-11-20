// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"context"
	"errors"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/fightercondition"
	"github.com/taytr-llc/slug/fighterownership"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/internal/careerstate"
)

// OwnedRead is one original-view capture of the concrete replay source. It is
// never reconstructed from a public ID or a caller supplied page.
type OwnedRead struct {
	owner      *OwnedStore
	source     *fighterownership.Store
	capture    *fighterownership.OwnedCapture
	generation uint64
	viewID     string
	status     OwnedStatus
	asOf       int64
}

func (r *OwnedRead) Head() fighterownership.HeadRef {
	if r == nil {
		return fighterownership.HeadRef{}
	}
	return r.status.Head
}

func (s *OwnedStore) Capture(ctx context.Context, session Session) (*OwnedRead, error) {
	ctx, done, e := s.begin(ctx, session, gymaccount.View)
	if e != nil {
		return nil, e
	}
	defer done()
	c, e := s.source.CaptureOwned(ctx, s.meta.Pin.Stable.Identity.GymPublicKey)
	if e != nil {
		return nil, e
	}
	lease, e := s.source.AcquireHead(ctx, c.Head())
	if e != nil {
		return nil, e
	}
	defer lease.Close()
	now, e := s.clock()
	if e != nil {
		return nil, e
	}
	m := s.meta
	m.ClockMS = now
	m.LastAsOfMS = now
	st := s.status(c.Head())
	st.LastAsOfMS = now
	r := &OwnedRead{s, s.source, c, session.Generation, session.ViewID, st, now}
	committed := false
	e = s.fence(ctx, session, gymaccount.View, func(check func() error) error {
		if e := lease.Check(); e != nil {
			return e
		}
		if e := check(); e != nil {
			return e
		}
		if !s.readOnly {
			if e := s.persist(new(leveldb.Batch), m); e != nil {
				return e
			}
			committed = true
		}
		return nil
	})
	if e != nil {
		if committed {
			return nil, errors.Join(ErrOutcomeUnknown, e)
		}
		return nil, e
	}
	s.read = r
	return r, nil
}
func (s *OwnedStore) checkRead(session Session, r *OwnedRead) error {
	if r == nil || r != s.read || r.owner != s || r.source != s.source || r.capture == nil {
		return ErrInvalid
	}
	if r.generation != session.Generation || r.viewID != session.ViewID {
		return ErrStaleView
	}
	if r.status.Pin != s.meta.Pin || r.status.StoreID != s.meta.StoreID || r.status.RosterRevision != s.meta.RosterRevision {
		return ErrConflict
	}
	return nil
}
func (s *OwnedStore) checkCareer(ctx context.Context) error {
	if _, e := s.career.CheckHistory(ctx, "stable", s.meta.StoreID); e != nil {
		return e
	}
	if s.legacy.Required() {
		id, e := ID(s.meta.Pin.Stable)
		if e != nil {
			return e
		}
		if _, e = s.legacy.CheckHistory(ctx, "stable", id); e != nil {
			return e
		}
	}
	return nil
}

// selectedHistory retains an actual reader's activation and rows. A second
// source must prove zero; it is never concatenated under the first activation.
func (s *OwnedStore) selectedHistory(ctx context.Context, id string, asOf int64) (*careerstate.History, bool, uint32, error) {
	h, e := s.career.CaptureHistory(ctx, id, asOf)
	if e != nil {
		return nil, false, 0, e
	}
	legacy := false
	if s.legacy.Required() {
		old, e := s.legacy.CaptureHistory(ctx, id, asOf)
		if e != nil {
			return nil, false, 0, e
		}
		if h.EventCount() > 0 && old.EventCount() > 0 {
			return nil, false, 0, ErrOwnedCareerConflict
		}
		if old.EventCount() > 0 {
			h = old
			legacy = true
		}
	}
	var wins uint32
	for after := uint32(0); after < h.EventCount(); after += 8 {
		rows, e := h.Rows(after, 8)
		if e != nil {
			return nil, false, 0, e
		}
		for _, row := range rows {
			corner := row.Event.CornerA
			if corner.ProspectID != id {
				corner = row.Event.CornerB
			}
			if row.Event.Winner == id && corner.GymID == s.meta.Pin.Stable.Identity.GymID {
				wins++
			}
		}
	}
	if e = s.checkCareer(ctx); e != nil {
		return nil, false, 0, e
	}
	return h, legacy, wins, nil
}
func (s *OwnedStore) projectOwned(ctx context.Context, raw fighterownership.OwnedItem, asOf int64) (OwnedItem, error) {
	if raw.State.OwnerPublicKey != s.meta.Pin.Stable.Identity.GymPublicKey || ownedState(s.meta.Pin, raw.Admission, raw.State) != nil {
		return OwnedItem{}, ErrConflict
	}
	birth, m, e := ownedAppearance(s.meta.Pin, raw.Admission)
	if e != nil {
		return OwnedItem{}, e
	}
	h, _, wins, e := s.selectedHistory(ctx, birth.ProspectID, asOf)
	if e != nil {
		return OwnedItem{}, e
	}
	snap := h.Snapshot()
	total, ok := snap.Totals(birth.ProspectID)
	if !ok {
		return OwnedItem{}, ErrCorrupt
	}
	condition, e := fightercondition.Project(1, m.DNA, birth.BornAtMS, asOf, int(total.XP))
	if e != nil {
		return OwnedItem{}, ErrCorrupt
	}
	return ValidateOwnedItem(s.meta.Pin, OwnedItem{birth, raw.Admission, raw.State, m, condition, OwnedCareer{snap.Activation(), snap.Digest(), total, wins}}, asOf)
}
func (s *OwnedStore) Page(ctx context.Context, session Session, r *OwnedRead, after, limit uint32) (OwnedPage, error) {
	if limit < 1 || limit > 8 {
		return OwnedPage{}, ErrInvalid
	}
	ctx, done, e := s.begin(ctx, session, gymaccount.View)
	if e != nil {
		return OwnedPage{}, e
	}
	defer done()
	if e = s.checkRead(session, r); e != nil {
		return OwnedPage{}, e
	}
	page, e := s.source.OwnedPage(ctx, r.capture, after, limit)
	if e != nil {
		return OwnedPage{}, e
	}
	out := OwnedPage{Version: 1, Status: r.status, Head: r.status.Head, AsOfMS: r.asOf, After: after, NextAfter: after, HasMore: !page.Complete, Items: make([]OwnedItem, 0, len(page.Items))}
	for _, v := range page.Items {
		i, e := s.projectOwned(ctx, v, r.asOf)
		if e != nil {
			return OwnedPage{}, e
		}
		out.Items = append(out.Items, i)
		out.NextAfter = uint32(i.Birth.Sequence)
	}
	lease, e := s.source.AcquireHead(ctx, r.status.Head)
	if e != nil {
		return OwnedPage{}, e
	}
	defer lease.Close()
	if e = s.checkCareer(ctx); e != nil {
		return OwnedPage{}, e
	}
	out, e = ValidateOwnedPage(out)
	if e != nil {
		return OwnedPage{}, e
	}
	e = s.fence(ctx, session, gymaccount.View, func(check func() error) error {
		if e := lease.Check(); e != nil {
			return e
		}
		return check()
	})
	if e != nil {
		return OwnedPage{}, e
	}
	return out, nil
}
