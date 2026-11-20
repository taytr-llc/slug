// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterownership

import (
	"context"
	"encoding/json"

	"github.com/taytr-llc/slug/registry"
)

// ResumeCutoffPublication publishes only an exact retained reference to an
// already signed cutoff from this original store. It does not sign a new cutoff,
// sample a node/time, or grant current Head or historical consumer readiness.
// A fresh process still needs complete normal replay after this publication.
func (s *Store) ResumeCutoffPublication(ctx context.Context, source *PublicSource) (*PublicSource, bool, error) {
	if s == nil || source == nil || source.publisher == nil || source.pin != s.pin {
		return nil, false, ErrInvalid
	}
	ctx, done, e := s.mutation(ctx)
	if e != nil {
		return nil, false, e
	}
	defer done()
	if e = source.Check(ctx); e != nil {
		return nil, false, e
	}
	prepared, e := source.publisher.PendingCutoffPublication(ctx)
	if e != nil {
		return nil, false, e
	}
	if prepared == nil {
		return source, false, nil
	}
	entry := prepared.Entry()
	var reference registry.PublicCutoffReference
	if entry.Kind != "cutoff" || json.Unmarshal(entry.Payload, &reference) != nil {
		return nil, false, ErrCorrupt
	}
	// The audited index selects only the signed statement named by the original
	// pending reference. A parsed caller value or current map cannot replace it.
	s.mu.Lock()
	event, indexed := s.cutoffByID[reference.CutoffID]
	cutoff, present := s.cutoffs[event]
	boundary, hasBoundary := s.boundaries[event]
	s.mu.Unlock()
	if !indexed || !present || !hasBoundary {
		return nil, false, ErrUnavailable
	}
	id, e := registry.PublicCutoffID(s.pin, cutoff)
	if e != nil || id != reference.CutoffID || cutoff.EventID != event || cutoff.Boundary != boundary {
		return nil, false, ErrConflict
	}
	intent, e := source.publisher.CutoffIntent(ctx, event)
	if e != nil {
		return nil, false, e
	}
	if intent != cutoff.Boundary.Intent {
		return nil, false, ErrConflict
	}
	if e = source.Check(ctx); e != nil {
		return nil, false, e
	}
	if e = prepared.Check(ctx); e != nil {
		return nil, false, e
	}
	entry.Signature, e = signObject("SLUG fighter ownership publication statement", entry)
	if e != nil {
		return nil, false, e
	}
	if e = source.Check(ctx); e != nil {
		return nil, false, e
	}
	if _, e = prepared.CommitCutoffSigned(ctx, entry, cutoff); e != nil {
		return nil, false, e
	}
	next, e := NewPublicSource(ctx, s.pin, source.publisher)
	return next, true, e
}
