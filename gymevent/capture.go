// Copyright (c) 2026 The SLUG developers. ISC license.
package gymevent

import (
	"context"
	"encoding/json"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymstable"
)

func (s *Store) Capture(ctx context.Context, source *gymentry.Store, session gymstable.Session, saturday string) (CaptureResult, error) {
	work, done, e := s.begin(ctx)
	if e != nil {
		return CaptureResult{}, e
	}
	defer done()
	ev, e := EventForDate(s.meta.Pin, saturday)
	if e != nil {
		return CaptureResult{}, e
	}
	var out CaptureResult
	var participantID string
	reserved, replayed, committed := false, false, false
	// First authenticate and reserve under a short metadata lease. No historical
	// source gate is acquired while this account lease remains held.
	e = s.sourceView(work, source, session, func(info gymentry.SnapshotSourceInfo, check func() error) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		if e := check(); e != nil {
			return e
		}
		n, e := s.observeLocked(0)
		if e != nil {
			return e
		}
		// The coordinator's own not-due condition precedes cross-authority
		// clock agreement. A retained high-water rollback still fails above.
		if n < ev.CutoffMS {
			return ErrNotDue
		}
		if n < info.ObservedAtMS {
			return ErrClock
		}
		participantID = info.Pin.StablePin.Identity.GymID
		cap, ok, e := s.capture(ev, participantID)
		if e != nil {
			return e
		}
		if ok {
			status, e := s.status(ev, n)
			if e != nil {
				return e
			}
			out = CaptureResult{1, status, cap, true}
			replayed = true
			return nil
		}
		if s.meta.CreatedAtMS >= ev.CutoffMS {
			return ErrCreatedTooLate
		}
		if n < ev.CutoffMS {
			return ErrNotDue
		}
		_, exists, e := s.event(ev)
		if e != nil {
			return e
		}
		if !exists {
			if s.meta.EventCount >= MaxEvents {
				return ErrCapacity
			}
			r := eventRow{Version: 1, Event: ev, Phase: "awaiting_capture", CreatedAtMS: n}
			b := new(leveldb.Batch)
			if e = put(b, key('e', ev.EventID), r, MaxEventSize); e != nil {
				return e
			}
			m := s.meta
			m.EventCount++
			s.mark("before-reserve")
			if e = s.commitLocked(b, m); e != nil {
				return e
			}
			reserved = true
			s.mark("reserved")
		}
		return nil
	})
	if e != nil || work.Err() != nil {
		if reserved {
			return CaptureResult{}, ErrOutcomeUnknown
		}
		if e == nil {
			e = ErrCancelled
		}
		return CaptureResult{}, mapped(e)
	}
	if replayed {
		// No source proofs or new fences. Validation is outside the metadata lease.
		if validateCaptureResult(s.meta.Pin, out) != nil {
			return CaptureResult{}, ErrCorrupt
		}
		s.mark("replay-validated")
		e = s.sourceView(work, source, session, func(info gymentry.SnapshotSourceInfo, check func() error) error {
			s.mu.Lock()
			defer s.mu.Unlock()
			if e := check(); e != nil {
				return e
			}
			n, e := s.observeLocked(info.ObservedAtMS)
			if e != nil {
				return e
			}
			out.Status, e = s.status(ev, n)
			return e
		})
		if e != nil || work.Err() != nil {
			if e == nil {
				e = ErrCancelled
			}
			return CaptureResult{}, mapped(e)
		}
		return out, nil
	}
	p, e := participant(s.meta.Pin, participantID)
	if e != nil {
		return CaptureResult{}, ErrCorrupt
	}
	otherID := s.meta.Pin.Participants[0].StablePin.Identity.GymID
	if otherID == participantID {
		otherID = s.meta.Pin.Participants[1].StablePin.Identity.GymID
	}
	other, otherExists, e := s.capture(ev, otherID)
	if e != nil {
		return CaptureResult{}, e
	}
	if otherExists && validateParticipantCapture(s.meta.Pin, other) != nil {
		return CaptureResult{}, ErrCorrupt
	}
	e = source.WithCutoffSnapshot(work, session, saturday, func(selected gymentry.CutoffSnapshot) (func(gymentry.CutoffFences, func() error) error, error) {
		if selected.Version != 1 || selected.Entry.Event.EventID != ev.SourceEventID {
			return nil, ErrCorrupt
		}
		sid, _ := gymentry.ID(p)
		snapshot := ParticipantSnapshot{Version: 1, ParticipantID: participantID, EntryStoreID: sid,
			Owner: AssignmentOwner{"candidate_assignment", p.StablePin.AccountAuthorityID, p.StablePin.AccountID, participantID}, Roster: selected.Roster, Entry: selected.Entry, Members: []Member{}}
		for _, x := range selected.Members {
			eligibility := "eligible"
			if x.ConditionAtCutoff.Phase == "retired" {
				eligibility = "retired"
			} else if x.Slot == 0 {
				eligibility = "not_rostered"
			}
			snapshot.Members = append(snapshot.Members, Member{x.Slot, x.SlotAssignedAtMS, x.Assignment, x.Class, x.ConditionAtCutoff, eligibility})
		}
		// Own all nested values and finish canonical/condition work before final lease.
		snapshot, e = ValidateParticipantSnapshot(s.meta.Pin, snapshot)
		if e != nil {
			return nil, ErrCorrupt
		}
		in := SnapshotInput{Version: 1, Event: ev, Participants: []ParticipantSnapshot{}}
		var digest string
		if otherExists {
			for _, pin := range s.meta.Pin.Participants {
				if pin.StablePin.Identity.GymID == participantID {
					in.Participants = append(in.Participants, snapshot)
				} else {
					in.Participants = append(in.Participants, other.Snapshot)
				}
			}
			in, e = ValidateSnapshotInput(s.meta.Pin, in)
			if e != nil {
				return nil, ErrCorrupt
			}
			b, _ := json.Marshal(in)
			digest = hash("SLUG candidate cutoff input\x00v1\x00", lp(b))
		}
		largest := CaptureEvidence{1, participantID,
			gymstable.CutoffFence{Version: 1, StoreID: p.StableStoreID, CutoffMS: ev.CutoffMS, ObservedAtMS: MaxTimestampMS, OperationCount: gymstable.MaxOperations},
			gymstable.CutoffFence{Version: 1, StoreID: sid, CutoffMS: ev.CutoffMS, ObservedAtMS: MaxTimestampMS, OperationCount: gymentry.MaxOperations}, MaxTimestampMS}
		if !fits(ParticipantCapture{1, snapshot, largest}, MaxCaptureSize) {
			return nil, ErrCapacity
		}
		if otherExists && !fits(SnapshotReceipt{1, in, digest, []CaptureEvidence{largest, largest}, MaxTimestampMS}, MaxReceiptSize) {
			return nil, ErrCapacity
		}
		s.mark("capture-prepared")
		return func(fences gymentry.CutoffFences, check func() error) error {
			s.mark("source-fences-synced")
			s.mu.Lock()
			defer s.mu.Unlock()
			if check == nil {
				return ErrUnauthorized
			}
			if e := check(); e != nil {
				return e
			}
			if work.Err() != nil {
				return ErrCancelled
			}
			a, b := fences.StableFence, fences.EntryFence
			if fences.Version != 1 || a.Version != 1 || b.Version != 1 || a.StoreID != p.StableStoreID || b.StoreID != sid || a.CutoffMS != ev.CutoffMS || b.CutoffMS != ev.CutoffMS || a.ObservedAtMS < ev.CutoffMS || b.ObservedAtMS < a.ObservedAtMS || !timestamp(b.ObservedAtMS) || uint64(a.OperationCount) < snapshot.Roster.LastSequence || a.OperationCount > gymstable.MaxOperations || uint64(b.OperationCount) < snapshot.Entry.LastSequence || b.OperationCount > gymentry.MaxOperations {
				return ErrCorrupt
			}
			r, exists, e := s.event(ev)
			if e != nil {
				return e
			}
			if !exists || r.Phase == "snapshotted" {
				return ErrConflict
			}
			if _, exists, e := s.raw(key('c', ev.EventID, participantID), MaxCaptureSize); e != nil {
				return e
			} else if exists {
				return ErrConflict
			}
			n, e := s.sampleLocked(b.ObservedAtMS)
			if e != nil {
				return e
			}
			if n < r.CreatedAtMS {
				return ErrClock
			}
			cap := ParticipantCapture{1, snapshot, CaptureEvidence{1, participantID, a, b, n}}
			batch := new(leveldb.Batch)
			if e = put(batch, key('c', ev.EventID, participantID), cap, MaxCaptureSize); e != nil {
				return e
			}
			r.Phase = "capturing"
			if otherExists {
				captures := []CaptureEvidence{}
				for _, pin := range s.meta.Pin.Participants {
					if pin.StablePin.Identity.GymID == participantID {
						captures = append(captures, cap.Evidence)
					} else {
						captures = append(captures, other.Evidence)
					}
				}
				receipt := SnapshotReceipt{1, in, digest, captures, n}
				if e = put(batch, key('s', ev.EventID), receipt, MaxReceiptSize); e != nil {
					return e
				}
				r.Phase = "snapshotted"
				r.CompletedAtMS = n
				r.SnapshotDigest = digest
			}
			if e = put(batch, key('e', ev.EventID), r, MaxEventSize); e != nil {
				return e
			}
			m := s.meta
			m.ClockMS = n
			s.mark("before-capture-commit")
			if e = s.commitLocked(batch, m); e != nil {
				return e
			}
			committed = true
			s.mark("capture-committed")
			status, e := s.status(ev, n)
			if e != nil {
				return e
			}
			out = CaptureResult{1, status, cap, false}
			return nil
		}, nil
	})
	if e != nil || work.Err() != nil {
		if committed {
			return CaptureResult{}, ErrOutcomeUnknown
		}
		if e == nil {
			e = ErrCancelled
		}
		return CaptureResult{}, mapped(e)
	}
	if !committed {
		return CaptureResult{}, ErrCorrupt
	}
	return out, nil
}
