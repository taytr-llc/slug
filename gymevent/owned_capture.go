// Copyright (c) 2026 The SLUG developers. ISC license.
package gymevent

import (
	"context"
	"errors"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/internal/ownedcodec"
	"github.com/taytr-llc/slug/registry"
)

func (s *OwnedStore) Capture(ctx context.Context, entry *gymentry.OwnedStore, session gymstable.Session, saturday string) (OwnedCaptureResult, error) {
	ctx, done, e := s.begin(ctx)
	if e != nil {
		return OwnedCaptureResult{}, e
	}
	defer done()
	ev, e := OwnedEventForDate(s.meta.Pin, saturday)
	if e != nil {
		return OwnedCaptureResult{}, e
	}
	if entry == nil {
		return OwnedCaptureResult{}, ErrInvalid
	}
	info, e := entry.SnapshotSourceInfo(ctx)
	if e != nil {
		return OwnedCaptureResult{}, e
	}
	participantID, e := OwnedParticipantID(s.meta.Pin, info.Pin)
	if e != nil {
		return OwnedCaptureResult{}, e
	}
	cutoff, e := s.source.Cutoff(ctx, ev.EventID)
	if e != nil {
		return OwnedCaptureResult{}, e
	}
	cutoffID, e := registry.PublicCutoffID(s.meta.Pin.Ownership, cutoff)
	if e != nil || cutoff.CutoffMS != ev.CutoffMS {
		return OwnedCaptureResult{}, ErrCorrupt
	}
	existing, found, e := s.capture(ev, participantID)
	if e != nil {
		return OwnedCaptureResult{}, e
	}
	if found {
		if ownedEvidence(s.meta.Pin, existing.Evidence, existing.Snapshot, cutoffID) != nil {
			return OwnedCaptureResult{}, ErrCorrupt
		}
		var observed int64
		e = s.sourceView(ctx, entry, session, func(info gymentry.OwnedSnapshotSourceInfo, check func() error) error {
			var e error
			observed, e = s.clock(info.ObservedAtMS)
			if e != nil {
				return e
			}
			return check()
		})
		if e != nil {
			return OwnedCaptureResult{}, e
		}
		status, e := s.status(ev, observed)
		if e != nil {
			return OwnedCaptureResult{}, e
		}
		return OwnedCaptureResult{1, status, existing, true}, nil
	}
	if s.readOnly {
		return OwnedCaptureResult{}, ErrForbidden
	}
	reserved := false
	e = s.sourceView(ctx, entry, session, func(info gymentry.OwnedSnapshotSourceInfo, check func() error) error {
		n, e := s.clock(info.ObservedAtMS)
		if e != nil {
			return e
		}
		if n < ev.CutoffMS {
			return ErrNotDue
		}
		if s.meta.CreatedAtMS >= ev.CutoffMS {
			return ErrCreatedTooLate
		}
		_, exists, e := s.event(ev)
		if e != nil {
			return e
		}
		m := s.meta
		m.ClockMS = n
		b := new(leveldb.Batch)
		if !exists {
			if m.EventCount >= 256 {
				return ErrCapacity
			}
			m.EventCount++
			row := eventRow{Version: 1, Event: ev, Phase: "awaiting_capture", CreatedAtMS: n}
			if e = put(b, key('e', ev.EventID), row, MaxEventSize); e != nil {
				return e
			}
		}
		if e = check(); e != nil {
			return e
		}
		if e = s.persist(b, m); e != nil {
			return e
		}
		reserved = true
		return nil
	})
	if e != nil {
		if reserved {
			return OwnedCaptureResult{}, errors.Join(ErrOutcomeUnknown, e)
		}
		return OwnedCaptureResult{}, e
	}
	otherID := ""
	for _, p := range s.meta.Pin.Participants {
		id, _ := OwnedParticipantID(s.meta.Pin, p)
		if id != participantID {
			otherID = id
		}
	}
	other, hasOther, e := s.capture(ev, otherID)
	if e != nil {
		return OwnedCaptureResult{}, e
	}
	if hasOther && ownedEvidence(s.meta.Pin, other.Evidence, other.Snapshot, cutoffID) != nil {
		return OwnedCaptureResult{}, ErrCorrupt
	}
	var result OwnedParticipantCapture
	committed := false
	e = entry.WithCutoffSnapshot(ctx, session, saturday, cutoffID, func(v gymentry.OwnedCutoffSnapshot) (func(gymentry.CutoffFences, func() error) error, error) {
		if v.Version != 1 || v.Entry.Event.EventID != ev.SourceEventID || !ownedcodec.Equal(v.Cutoff, cutoff) {
			return nil, ErrCorrupt
		}
		p := info.Pin.StablePin.Stable
		snapshot := OwnedParticipantSnapshot{1, participantID, info.StoreID, AssignmentOwner{"finalized_public_chain", p.AccountAuthorityID, p.AccountID, p.Identity.GymID}, v.Roster, v.Entry, v.Members, v.Excluded}
		snapshot, e = ValidateOwnedParticipantSnapshot(s.meta.Pin, snapshot)
		if e != nil {
			return nil, e
		}
		input := OwnedSnapshotInput{Version: 1, Event: ev, Cutoff: cutoff, Participants: []OwnedParticipantSnapshot{}}
		digest := ""
		if hasOther {
			for _, p := range s.meta.Pin.Participants {
				id, _ := OwnedParticipantID(s.meta.Pin, p)
				if id == participantID {
					input.Participants = append(input.Participants, snapshot)
				} else {
					input.Participants = append(input.Participants, other.Snapshot)
				}
			}
			input, e = ValidateOwnedSnapshotInput(s.meta.Pin, input)
			if e != nil {
				return nil, e
			}
			digest, e = OwnedSnapshotDigest(s.meta.Pin, input)
			if e != nil {
				return nil, e
			}
		}
		return func(f gymentry.CutoffFences, check func() error) error {
			if check == nil {
				return ErrUnauthorized
			}
			if e := check(); e != nil {
				return e
			}
			n, e := s.clock(f.EntryFence.ObservedAtMS)
			if e != nil {
				return e
			}
			evidence := OwnedCaptureEvidence{1, participantID, f.StableFence, f.EntryFence, cutoffID, n}
			if f.Version != 1 || ownedEvidence(s.meta.Pin, evidence, snapshot, cutoffID) != nil {
				return ErrCorrupt
			}
			row, exists, e := s.event(ev)
			if e != nil || !exists || row.Phase == "snapshotted" {
				return ErrConflict
			}
			result = OwnedParticipantCapture{1, snapshot, evidence}
			b := new(leveldb.Batch)
			if e = put(b, key('c', ev.EventID, participantID), result, OwnedMaxParticipantSize+4096); e != nil {
				return e
			}
			row.Phase = "capturing"
			if hasOther {
				captures := make([]OwnedCaptureEvidence, 0, 2)
				for _, p := range input.Participants {
					if p.ParticipantID == participantID {
						captures = append(captures, evidence)
					} else {
						captures = append(captures, other.Evidence)
					}
				}
				receipt := OwnedSnapshotReceipt{1, input, digest, captures, n}
				if e = put(b, key('s', ev.EventID), receipt, OwnedMaxReceiptSize); e != nil {
					return e
				}
				row.Phase = "snapshotted"
				row.CompletedAtMS = n
				row.SnapshotDigest = digest
			}
			if e = put(b, key('e', ev.EventID), row, MaxEventSize); e != nil {
				return e
			}
			m := s.meta
			m.ClockMS = n
			if e = check(); e != nil {
				return e
			}
			if e = s.persist(b, m); e != nil {
				return e
			}
			committed = true
			return nil
		}, nil
	})
	if e != nil {
		if committed {
			return OwnedCaptureResult{}, errors.Join(ErrOutcomeUnknown, e)
		}
		return OwnedCaptureResult{}, e
	}
	if !committed {
		return OwnedCaptureResult{}, ErrCorrupt
	}
	status, e := s.status(ev, s.meta.ClockMS)
	if e != nil {
		return OwnedCaptureResult{}, errors.Join(ErrOutcomeUnknown, e)
	}
	return OwnedCaptureResult{1, status, result, false}, nil
}
