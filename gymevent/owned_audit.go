// Copyright (c) 2026 The SLUG developers. ISC license.
package gymevent

import (
	"context"
	"sort"

	"github.com/taytr-llc/slug/fighterownership"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/internal/ownedcodec"
)

// The saved compact snapshot is reattached to the exact historical public
// source, never to the latest ownership head. Shape checks alone cannot grant
// authority to a reconstructed or coherently edited local snapshot.
func (s *OwnedStore) auditCutoffParticipant(ctx context.Context, cutoff fighterownership.CutoffRecord, cutoffID string, pin gymentry.OwnedPin, snapshot OwnedParticipantSnapshot) error {
	ids := make([]string, 0, len(snapshot.Members)+len(snapshot.Excluded))
	for _, member := range snapshot.Members {
		ids = append(ids, member.Birth.ProspectID)
	}
	for _, excluded := range snapshot.Excluded {
		ids = append(ids, excluded.ProspectID)
	}
	sort.Strings(ids)
	resolved, e := s.source.ResolveCutoff(ctx, cutoffID, ids)
	if e != nil {
		return e
	}
	if !ownedcodec.Equal(resolved.Cutoff, cutoff) || len(resolved.Items) != len(ids) {
		return ErrCorrupt
	}
	for n, item := range resolved.Items {
		if item.State.ProspectID != ids[n] || item.Admission.Record.Verified.ProspectID != ids[n] || item.Admission.Record.Birth.AcceptedAtMS >= cutoff.CutoffMS {
			return ErrCorrupt
		}
		found := false
		for _, member := range snapshot.Members {
			if member.Birth.ProspectID != ids[n] {
				continue
			}
			if item.State.OwnerPublicKey != pin.StablePin.Stable.Identity.GymPublicKey || !ownedcodec.Equal(item.State, member.Ownership) || gymstable.ValidateOwnedBirth(pin.StablePin, item.Admission, member.Birth) != nil {
				return ErrCorrupt
			}
			found = true
		}
		for _, excluded := range snapshot.Excluded {
			if excluded.ProspectID != ids[n] {
				continue
			}
			if item.State.OwnerPublicKey == pin.StablePin.Stable.Identity.GymPublicKey || excluded.StateID != item.State.StateID || excluded.OwnerPublicKey != item.State.OwnerPublicKey {
				return ErrCorrupt
			}
			found = true
		}
		if !found {
			return ErrCorrupt
		}
	}
	return nil
}
