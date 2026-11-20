// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"context"
	"errors"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/fighterownership"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/internal/careerstate"
)

type OwnedCareerHistoryRead struct {
	owner      *OwnedStore
	source     *fighterownership.Store
	generation uint64
	viewID     string
	header     []byte
	history    *careerstate.History
	legacy     bool
}

func (r *OwnedCareerHistoryRead) RetainedBytes() int {
	if r == nil {
		return 0
	}
	return len(r.header) + r.history.RetainedBytes() + 4096
}
func (s *OwnedStore) PrepareCareerHistory(ctx context.Context, session Session, id string) (OwnedCareerHistoryCapture, *OwnedCareerHistoryRead, error) {
	if !hashID(id) {
		return OwnedCareerHistoryCapture{}, nil, ErrInvalid
	}
	ctx, done, e := s.begin(ctx, session, gymaccount.View)
	if e != nil {
		return OwnedCareerHistoryCapture{}, nil, e
	}
	defer done()
	c, e := s.source.CaptureOwned(ctx, s.meta.Pin.Stable.Identity.GymPublicKey)
	if e != nil {
		return OwnedCareerHistoryCapture{}, nil, e
	}
	v, e := s.source.OwnedItem(ctx, c, id)
	if e != nil {
		return OwnedCareerHistoryCapture{}, nil, e
	}
	birth, e := ownedBirth(s.meta.Pin, v.Admission)
	if e != nil || ownedState(s.meta.Pin, v.Admission, v.State) != nil {
		return OwnedCareerHistoryCapture{}, nil, ErrCorrupt
	}
	now, e := s.clock()
	if e != nil {
		return OwnedCareerHistoryCapture{}, nil, e
	}
	h, legacy, wins, e := s.selectedHistory(ctx, id, now)
	if e != nil {
		return OwnedCareerHistoryCapture{}, nil, e
	}
	snap := h.Snapshot()
	total, ok := snap.Totals(id)
	if !ok {
		return OwnedCareerHistoryCapture{}, nil, ErrCorrupt
	}
	status := s.status(c.Head())
	status.LastAsOfMS = now
	capture := OwnedCareerHistoryCapture{Version: 1, Status: status, Activation: snap.Activation(), CareerDigest: snap.Digest(), AsOfMS: now, Birth: birth, Ownership: v.State, PrefixCount: h.PrefixCount(), EventCount: h.EventCount(), Totals: total, CurrentGymWins: wins}
	capture.HistoryID = OwnedCareerHistoryID(capture)
	header, e := MarshalOwnedCareerHistoryCapture(capture)
	if e != nil {
		return OwnedCareerHistoryCapture{}, nil, e
	}
	r := &OwnedCareerHistoryRead{s, s.source, session.Generation, session.ViewID, header, h, legacy}
	if r.RetainedBytes() > OwnedMaxHistoryRetainedBytes {
		return OwnedCareerHistoryCapture{}, nil, ErrCapacity
	}
	for after := uint32(0); after < h.EventCount(); after += 8 {
		if _, e = ownedHistoryPage(capture, h, after, 8); e != nil {
			return OwnedCareerHistoryCapture{}, nil, e
		}
	}
	lease, e := s.source.AcquireHead(ctx, c.Head())
	if e != nil {
		return OwnedCareerHistoryCapture{}, nil, e
	}
	defer lease.Close()
	if e = s.checkCareer(ctx); e != nil {
		return OwnedCareerHistoryCapture{}, nil, e
	}
	m := s.meta
	m.ClockMS = now
	m.LastAsOfMS = now
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
			return OwnedCareerHistoryCapture{}, nil, errors.Join(ErrOutcomeUnknown, e)
		}
		return OwnedCareerHistoryCapture{}, nil, e
	}
	s.history = r
	return capture, r, nil
}
func ownedHistoryPage(c OwnedCareerHistoryCapture, h *careerstate.History, after, limit uint32) (OwnedCareerHistoryPage, error) {
	rows, e := h.Rows(after, limit)
	if e != nil {
		return OwnedCareerHistoryPage{}, e
	}
	p := OwnedCareerHistoryPage{1, c, after, after + uint32(len(rows)), false, rows}
	p.HasMore = p.Next < c.EventCount
	return ValidateOwnedCareerHistoryPage(p)
}
func (s *OwnedStore) ReadCareerHistory(ctx context.Context, session Session, r *OwnedCareerHistoryRead, after, limit uint32) (OwnedCareerHistoryPage, error) {
	if r == nil || r.owner != s || r.source != s.source || r.history == nil || r.RetainedBytes() > OwnedMaxHistoryRetainedBytes || limit < 1 || limit > 8 {
		return OwnedCareerHistoryPage{}, ErrInvalid
	}
	ctx, done, e := s.begin(ctx, session, gymaccount.View)
	if e != nil {
		return OwnedCareerHistoryPage{}, e
	}
	defer done()
	if r != s.history {
		return OwnedCareerHistoryPage{}, ErrConflict
	}
	if r.generation != session.Generation || r.viewID != session.ViewID {
		return OwnedCareerHistoryPage{}, ErrStaleView
	}
	c, e := ParseOwnedCareerHistoryCapture(r.header)
	if e != nil {
		return OwnedCareerHistoryPage{}, ErrCorrupt
	}
	if c.Status.Pin != s.meta.Pin || c.Status.RosterRevision != s.meta.RosterRevision {
		return OwnedCareerHistoryPage{}, ErrConflict
	}
	lease, e := s.source.AcquireHead(ctx, c.Status.Head)
	if e != nil {
		return OwnedCareerHistoryPage{}, e
	}
	defer lease.Close()
	if e = s.checkCareer(ctx); e != nil {
		return OwnedCareerHistoryPage{}, e
	}
	p, e := ownedHistoryPage(c, r.history, after, limit)
	if e != nil {
		return OwnedCareerHistoryPage{}, e
	}
	e = s.fence(ctx, session, gymaccount.View, func(check func() error) error {
		if e := lease.Check(); e != nil {
			return e
		}
		return check()
	})
	if e != nil {
		return OwnedCareerHistoryPage{}, e
	}
	return p, nil
}
