// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"context"

	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/registry"
)

// CareerHistoryEvidence is an owned public copy, not an admission, assignment
// or history capability. The capture remains the caller's original selection.
type CareerHistoryEvidence struct {
	Admission registry.ManifestRecord
	Capture   CareerHistoryCapture
	Rows      []CareerHistoryRow
}

// ReadCareerHistoryEvidence performs one concrete source proof verification,
// outside account and stable locks. It cannot import claimant receipts or mint
// a newer history capture. Revocation/cancellation joins entered source work.
func (s *Store) ReadCareerHistoryEvidence(ctx context.Context, session Session, captured *CareerHistoryRead) (CareerHistoryEvidence, error) {
	if e := normal(); e != nil {
		return CareerHistoryEvidence{}, e
	}
	if s == nil || captured == nil || captured.owner != s || captured.source != s.source || captured.history == nil || len(captured.header) == 0 || captured.RetainedBytes() > MaxCareerHistoryRetainedBytes {
		return CareerHistoryEvidence{}, ErrInvalid
	}
	if captured.generation != session.Generation || captured.viewID != session.ViewID {
		return CareerHistoryEvidence{}, ErrStaleView
	}
	ctx, done, e := s.begin(ctx, session, gymaccount.View)
	if e != nil {
		return CareerHistoryEvidence{}, s.careerPreviewReadError(e)
	}
	defer done()
	c, e := ParseCareerHistoryCapture(captured.header)
	if e != nil || captured.history.EventCount() != c.EventCount {
		return CareerHistoryEvidence{}, ErrCorrupt
	}
	checkSource := func() error {
		if captured.source != s.source {
			return ErrConflict
		}
		count, e := s.sourceInfoMode(ctx, true)
		if e != nil {
			return e
		}
		if count < c.Status.ObservedSourceCount {
			return ErrConflict
		}
		return nil
	}
	checkView := func() error {
		return s.withLease(ctx, session, gymaccount.View, func(check func() error) error {
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
			activation, e := s.career.CheckHistory(ctx, "stable", st.StoreID)
			if e != nil {
				return e
			}
			if activation != c.Activation {
				return ErrConflict
			}
			return check()
		})
	}
	if e = checkSource(); e != nil {
		return CareerHistoryEvidence{}, s.careerPreviewReadError(e)
	}
	if e = checkView(); e != nil {
		return CareerHistoryEvidence{}, s.careerPreviewReadError(e)
	}
	// Immutable captured row decoding and allocation are outside account leases.
	rows := make([]CareerHistoryRow, 0, c.EventCount)
	for after := uint32(0); after < c.EventCount; after += 8 {
		if ctx.Err() != nil {
			return CareerHistoryEvidence{}, ErrCancelled
		}
		part, e := captured.history.Rows(after, 8)
		if e != nil {
			return CareerHistoryEvidence{}, s.careerPreviewReadError(e)
		}
		page := CareerHistoryPage{Version: Version1, Capture: c, After: after, Next: after + uint32(len(part)), Items: part}
		page.HasMore = page.Next < c.EventCount
		if _, e = ValidateCareerHistoryPageLimit(page, 8); e != nil {
			return CareerHistoryEvidence{}, ErrCorrupt
		}
		rows = append(rows, part...)
	}
	r, e := s.readVerifiedSource(ctx, c.Assignment.Sequence, c.Assignment.ProspectID, true)
	if e != nil {
		return CareerHistoryEvidence{}, s.careerPreviewReadError(e)
	}
	a, e := assignmentForRecord(r, c.Status.Pin)
	if e != nil || a != c.Assignment {
		return CareerHistoryEvidence{}, ErrCorrupt
	}
	if e = checkSource(); e != nil {
		return CareerHistoryEvidence{}, s.careerPreviewReadError(e)
	}
	if e = checkView(); e != nil {
		return CareerHistoryEvidence{}, s.careerPreviewReadError(e)
	}
	r.Proof.MerkleBranch = append([]string{}, r.Proof.MerkleBranch...)
	return CareerHistoryEvidence{Admission: r, Capture: c, Rows: rows}, nil
}
