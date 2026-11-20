// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"context"

	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/internal/careerstate"
	"github.com/taytr-llc/slug/registry"
)

// CareerHistoryRead is a process-local capability, with no public importer.
// All mutable owner data is rechecked; row bytes are the immutable capture.
type CareerHistoryRead struct {
	owner      *Store
	source     *registry.ManifestStore
	generation uint64
	viewID     string
	header     []byte
	history    *careerstate.History
}

func (r *CareerHistoryRead) RetainedBytes() int {
	if r == nil {
		return 0
	}
	return len(r.header) + r.history.RetainedBytes() + 4096
}

func (s *Store) PrepareCareerHistory(ctx context.Context, session Session, prospectID string) (CareerHistoryCapture, *CareerHistoryRead, error) {
	if s == nil || !hashID(prospectID) {
		return CareerHistoryCapture{}, nil, ErrInvalid
	}
	ctx, done, e := s.begin(ctx, session, gymaccount.View)
	if e != nil {
		return CareerHistoryCapture{}, nil, s.careerPreviewReadError(e)
	}
	defer done()
	if !s.career.Required() {
		return CareerHistoryCapture{}, nil, careerstate.ErrInactive
	}
	if e = s.career.Ready(); e != nil {
		return CareerHistoryCapture{}, nil, s.careerPreviewReadError(e)
	}
	source := s.source
	count, e := s.sourceInfoMode(ctx, true)
	if e != nil {
		return CareerHistoryCapture{}, nil, s.careerPreviewReadError(e)
	}
	checkpoint, revision := s.meta.Checkpoint, s.meta.RosterRevision
	a, e := s.assignment(prospectID)
	if e != nil {
		return CareerHistoryCapture{}, nil, s.careerPreviewReadError(e)
	}
	if e = s.withLease(ctx, session, gymaccount.View, func(check func() error) error { return check() }); e != nil {
		return CareerHistoryCapture{}, nil, s.careerPreviewReadError(e)
	}
	item, e := s.readSourceMode(ctx, a.Sequence, prospectID, true)
	if e != nil {
		return CareerHistoryCapture{}, nil, s.careerPreviewReadError(e)
	}
	if item.Assignment != a {
		return CareerHistoryCapture{}, nil, ErrCorrupt
	}
	// A proof can be uncancellable and the source can change while it runs.
	// Recheck only metadata outside account/Stable locks before publication.
	if s.source != source {
		return CareerHistoryCapture{}, nil, ErrConflict
	}
	currentCount, e := s.sourceInfoMode(ctx, true)
	if e != nil {
		return CareerHistoryCapture{}, nil, s.careerPreviewReadError(e)
	}
	if currentCount != count {
		return CareerHistoryCapture{}, nil, ErrConflict
	}
	var capture CareerHistoryCapture
	var token *CareerHistoryRead
	e = s.publishRead(ctx, session, []Item{item}, checkpoint, revision, func(now int64, status Status) error {
		if status.ObservedSourceCount != count {
			return ErrConflict
		}
		h, e := s.career.CaptureHistory(ctx, prospectID, now)
		if e != nil {
			return e
		}
		snapshot := h.Snapshot()
		total, ok := snapshot.Totals(prospectID)
		if !ok {
			return ErrCorrupt
		}
		capture = CareerHistoryCapture{Version: Version1, Status: status, Activation: snapshot.Activation(), CareerDigest: snapshot.Digest(), AsOfMS: now, Assignment: a, PrefixCount: h.PrefixCount(), EventCount: h.EventCount(), Totals: total}
		capture.HistoryID = CareerHistoryID(capture)
		header, e := MarshalCareerHistoryCapture(capture)
		if e != nil {
			return ErrCorrupt
		}
		// Check original corner/account binding and all rows before the inherited
		// LastAsOf sync. Pages retain no decoded graph after this bounded walk.
		for after := uint32(0); after < capture.EventCount; after += 8 {
			rows, e := h.Rows(after, 8)
			if e != nil {
				return e
			}
			p := CareerHistoryPage{Version: Version1, Capture: capture, After: after, Next: after + uint32(len(rows)), Items: rows}
			p.HasMore = p.Next < capture.EventCount
			if _, e = ValidateCareerHistoryPageLimit(p, 8); e != nil {
				return ErrCorrupt
			}
		}
		token = &CareerHistoryRead{owner: s, source: s.source, generation: session.Generation, viewID: session.ViewID, header: header, history: h}
		if token.RetainedBytes() > MaxCareerHistoryRetainedBytes {
			return ErrCapacity
		}
		return nil
	})
	if e != nil {
		return CareerHistoryCapture{}, nil, s.careerPreviewReadError(e)
	}
	return capture, token, nil
}

func (s *Store) ReadCareerHistory(ctx context.Context, session Session, captured *CareerHistoryRead, after, limit uint32) (CareerHistoryPage, error) {
	if s == nil || captured == nil || captured.owner != s || captured.source != s.source || captured.history == nil || len(captured.header) == 0 || captured.RetainedBytes() > MaxCareerHistoryRetainedBytes || limit < 1 || limit > 8 || after > captured.history.EventCount() {
		return CareerHistoryPage{}, ErrInvalid
	}
	if captured.generation != session.Generation || captured.viewID != session.ViewID {
		return CareerHistoryPage{}, ErrStaleView
	}
	ctx, done, e := s.begin(ctx, session, gymaccount.View)
	if e != nil {
		return CareerHistoryPage{}, s.careerPreviewReadError(e)
	}
	defer done()
	c, e := ParseCareerHistoryCapture(captured.header)
	if e != nil {
		return CareerHistoryPage{}, ErrCorrupt
	}
	count, e := s.sourceInfoMode(ctx, true)
	if e != nil {
		return CareerHistoryPage{}, s.careerPreviewReadError(e)
	}
	if count < c.Status.ObservedSourceCount {
		return CareerHistoryPage{}, ErrConflict
	}
	var page CareerHistoryPage
	e = s.withLease(ctx, session, gymaccount.View, func(check func() error) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		if e := s.stateLocked(); e != nil {
			return e
		}
		st := c.Status
		if s.meta.StoreID != st.StoreID || s.meta.Pin != st.Pin || s.meta.Checkpoint != st.Checkpoint || s.meta.RosterRevision != st.RosterRevision || s.meta.ObservedSourceCount < st.ObservedSourceCount {
			return ErrConflict
		}
		a, e := s.assignment(c.Assignment.ProspectID)
		if e != nil || a != c.Assignment {
			return ErrConflict
		}
		live, e := s.career.CheckHistory(ctx, "stable", st.StoreID)
		if e != nil {
			return e
		}
		if live != c.Activation {
			return ErrConflict
		}
		rows, e := captured.history.Rows(after, limit)
		if e != nil {
			return e
		}
		page = CareerHistoryPage{Version: Version1, Capture: c, After: after, Next: after + uint32(len(rows)), Items: rows}
		page.HasMore = page.Next < c.EventCount
		if _, e = ValidateCareerHistoryPageLimit(page, limit); e != nil {
			return ErrCorrupt
		}
		return check()
	})
	if e != nil {
		return CareerHistoryPage{}, s.careerPreviewReadError(e)
	}
	return page, nil
}
