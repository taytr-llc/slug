// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterownership

import (
	"context"
	"encoding/json"
)

// Empty reports the exact pristine, metadata-only store created by Create and
// accepted by Open's full audit. A missing ready head alone is never evidence
// of emptiness. This setup read neither replays nor repairs any state.
func (s *Store) Empty(ctx context.Context) (bool, error) {
	if ctx == nil || s == nil {
		return false, ErrInvalid
	}
	if e := ctx.Err(); e != nil {
		return false, e
	}
	select {
	case s.gate <- struct{}{}:
		defer func() { <-s.gate }()
	default:
		return false, ErrBusy
	}
	if !s.mu.TryLock() {
		return false, ErrBusy
	}
	defer s.mu.Unlock()
	if e := s.state(); e != nil {
		return false, e
	}
	want := storeMetadata{OperatorRole: s.operatorRole, Version: 1, PinID: s.pinID, StoreID: s.identity.StoreID,
		LogicalBytes: s.meta.LogicalBytes, FinalityHead: zeroID}
	if s.meta != want || s.ready || s.head != (HeadRef{}) || s.work != nil || s.replay != nil ||
		len(s.keySizes) != 1 || len(s.admissions)+len(s.publications)+len(s.headers)+len(s.canonical)+
		len(s.witnesses)+len(s.states)+len(s.maps)+len(s.cutoffs)+len(s.boundaries)+len(s.conflicts)+
		len(s.frames)+len(s.frameBodies)+len(s.finalities)+len(s.commits)+len(s.effective)+
		len(s.transfers)+len(s.portable)+len(s.admissionByProspect)+len(s.cutoffByID)+len(s.transferCounts) != 0 {
		return false, nil
	}
	raw, e := json.Marshal(s.meta)
	if e != nil || s.keySizes["meta"] != len(raw) || s.meta.LogicalBytes != uint32(4+len(raw)) {
		return false, ErrCorrupt
	}
	return true, ctx.Err()
}
