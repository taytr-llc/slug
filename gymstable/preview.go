// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/gymaccount"
)

var ErrClassMismatch = errors.New("preview_class_mismatch")

// PreviewPair is a common-time projection of two consumed assignments. It is
// neither a fight record nor a caller-importable assignment authority.
type PreviewPair struct {
	Version  uint32  `json:"version"`
	Status   Status  `json:"status"`
	AsOfMS   int64   `json:"as_of_ms"`
	Fighters [2]Item `json:"fighters"`
}

// ReadPreviewPair verifies at most two retained proofs, serially. Only existing
// clock/count observation metadata can change; it never catches up assignments.
func (s *Store) ReadPreviewPair(ctx context.Context, session Session, ids [2]string) (PreviewPair, error) {
	if !hashID(ids[0]) || !hashID(ids[1]) || ids[0] == ids[1] {
		return PreviewPair{}, ErrInvalid
	}
	if ids[1] < ids[0] {
		ids[0], ids[1] = ids[1], ids[0]
	}
	ctx, done, e := s.begin(ctx, session, gymaccount.View)
	if e != nil {
		return PreviewPair{}, e
	}
	defer done()
	if _, e = s.sourceInfo(ctx); e != nil {
		return PreviewPair{}, e
	}
	checkpoint, revision := s.meta.Checkpoint, s.meta.RosterRevision
	var assignments [2]Assignment
	for n, id := range ids {
		assignments[n], e = s.assignment(id)
		if e != nil {
			return PreviewPair{}, e
		}
	}
	items := make([]Item, 2)
	for n, id := range ids {
		if e = s.withLease(ctx, session, gymaccount.View, func(check func() error) error { return check() }); e != nil {
			return PreviewPair{}, e
		}
		items[n], e = s.readSource(ctx, assignments[n].Sequence, id)
		if e != nil {
			return PreviewPair{}, e
		}
		if items[n].Assignment != assignments[n] {
			return PreviewPair{}, ErrCorrupt
		}
	}
	if items[0].Manifest.Physical.Class != items[1].Manifest.Physical.Class {
		return PreviewPair{}, ErrClassMismatch
	}
	var result PreviewPair
	e = s.publishRead(ctx, session, items, checkpoint, revision, func(now int64, status Status) error {
		result = PreviewPair{Version: Version1, Status: status, AsOfMS: now, Fighters: [2]Item{items[0], items[1]}}
		raw, err := json.Marshal(result)
		if err != nil || len(raw) > 24576 {
			return ErrCapacity
		}
		return nil
	})
	if e != nil {
		return PreviewPair{}, e
	}
	return result, nil
}
