// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"context"
	"encoding/hex"
	"errors"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
	"github.com/taytr-llc/slug/fightercondition"
	"github.com/taytr-llc/slug/fighterownership"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/internal/careerconsumer"
	"github.com/taytr-llc/slug/internal/careerstate"
	"github.com/taytr-llc/slug/internal/ownedcodec"
)

type OwnedCutoffMember struct {
	Slot              uint32                          `json:"slot"`
	SlotAssignedAtMS  int64                           `json:"slot_assigned_at_ms"`
	Birth             Assignment                      `json:"birth"`
	Ownership         fighterownership.EffectiveState `json:"ownership"`
	Class             string                          `json:"class"`
	ConditionAtCutoff fightercondition.Snapshot       `json:"condition_at_cutoff"`
	Eligibility       string                          `json:"eligibility"`
}
type OwnedExcludedMember struct {
	ProspectID     string `json:"prospect_id"`
	StateID        string `json:"state_id"`
	OwnerPublicKey string `json:"owner_public_key"`
	Reason         string `json:"reason"`
}
type OwnedCutoffSelection struct {
	Version  uint32                        `json:"version"`
	Cutoff   fighterownership.CutoffRecord `json:"cutoff"`
	Roster   CutoffRoster                  `json:"roster"`
	Members  []OwnedCutoffMember           `json:"members"`
	Excluded []OwnedExcludedMember         `json:"excluded"`
}

func (s *OwnedStore) ownedCutoffRoster(ctx context.Context, cutoff int64) (CutoffRoster, error) {
	out := CutoffRoster{Version: 1, StoreID: s.meta.StoreID, CutoffMS: cutoff}
	out.HistoryDigest, _ = ownedcodec.Hash("stable cutoff history", struct {
		Version  uint32 `json:"version"`
		StoreID  string `json:"store_id"`
		EventID  string `json:"event_id"`
		CutoffMS int64  `json:"cutoff_ms"`
	}{1, s.meta.StoreID, "", cutoff})
	var ids [MaxOperations]string
	it := s.db.NewIterator(util.BytesPrefix([]byte{'o'}), nil)
	var count uint32
	for it.Next() {
		if ctx.Err() != nil {
			it.Release()
			return out, ErrCancelled
		}
		key, raw := it.Key(), it.Value()
		if len(key) != 33 || count >= s.meta.OperationCount || len(raw) > OwnedMaxOperationSize {
			it.Release()
			return out, ErrCorrupt
		}
		o, e := ParseOwnedOperation(s.meta.Pin, raw)
		if e != nil || o.Intent.OperationID != hex.EncodeToString(key[1:]) || o.Intent.Sequence > uint64(s.meta.OperationCount) || ids[o.Intent.Sequence-1] != "" {
			it.Release()
			return out, ErrCorrupt
		}
		ids[o.Intent.Sequence-1] = o.Intent.OperationID
		count++
	}
	e := it.Error()
	it.Release()
	if e != nil || count != s.meta.OperationCount {
		return out, ErrCorrupt
	}
	for n := uint32(0); n < count; n++ {
		if ctx.Err() != nil {
			return out, ErrCancelled
		}
		if ids[n] == "" {
			return out, ErrCorrupt
		}
		o, e := s.operation(ids[n])
		if e != nil {
			return out, e
		}
		if o.State != "committed" || o.Receipt.CommittedAtMS >= cutoff {
			continue
		}
		r := o.Receipt
		out.Slots = r.Slots
		out.Revision = r.Revision
		out.CommittedCount++
		out.LastSequence = o.Intent.Sequence
		if r.Changed {
			out.AssignedAtMS[r.Request.Slot-1] = 0
			if r.Request.ProspectID != "" {
				out.AssignedAtMS[r.Request.Slot-1] = r.CommittedAtMS
			}
		}
		out.HistoryDigest, e = ownedcodec.Hash("stable cutoff history", struct {
			Previous  string         `json:"previous_digest"`
			Operation OwnedOperation `json:"operation"`
		}{out.HistoryDigest, o})
		if e != nil {
			return out, e
		}
	}
	return out, nil
}
func (s *OwnedStore) WithCutoffSelection(ctx context.Context, session Session, cutoffID string, ids []string, prepare func(OwnedCutoffSelection) (func(CutoffFence, func() error) error, error)) error {
	if !hashID(cutoffID) || len(ids) > 5 || prepare == nil {
		return ErrInvalid
	}
	for n, id := range ids {
		if !hashID(id) || n > 0 && ids[n-1] >= id {
			return ErrInvalid
		}
	}
	ids = append([]string{}, ids...)
	ctx, done, e := s.begin(ctx, session, gymaccount.View)
	if e != nil {
		return e
	}
	defer done()
	resolved, e := s.source.ResolveCutoff(ctx, cutoffID, ids)
	if e != nil {
		return e
	}
	cutoff := resolved.Cutoff.CutoffMS
	now, e := s.clock()
	if e != nil {
		return e
	}
	if now < cutoff {
		return ErrClock
	}
	roster, e := s.ownedCutoffRoster(ctx, cutoff)
	if e != nil {
		return e
	}
	selection := OwnedCutoffSelection{1, resolved.Cutoff, roster, make([]OwnedCutoffMember, 0, len(ids)), make([]OwnedExcludedMember, 0, len(ids))}
	if len(resolved.Items) != len(ids) {
		return ErrCorrupt
	}
	ownedIDs := make([]string, 0, len(ids))
	for n, v := range resolved.Items {
		if v.State.ProspectID != ids[n] || ownedState(s.meta.Pin, v.Admission, v.State) != nil {
			return ErrCorrupt
		}
		if v.State.OwnerPublicKey != s.meta.Pin.Stable.Identity.GymPublicKey {
			selection.Excluded = append(selection.Excluded, OwnedExcludedMember{ids[n], v.State.StateID, v.State.OwnerPublicKey, "not_owned_at_cutoff"})
			continue
		}
		birth, manifest, e := ownedAppearance(s.meta.Pin, v.Admission)
		if e != nil || birth.AcceptedAtMS >= cutoff {
			return ErrCorrupt
		}
		condition, e := fightercondition.Project(1, manifest.DNA, birth.BornAtMS, cutoff, 0)
		if e != nil {
			return ErrCorrupt
		}
		m := OwnedCutoffMember{Birth: birth, Ownership: v.State, Class: manifest.Physical.Class, ConditionAtCutoff: condition, Eligibility: "not_rostered"}
		for i, id := range roster.Slots {
			if id == ids[n] {
				m.Slot = uint32(i + 1)
				m.SlotAssignedAtMS = roster.AssignedAtMS[i]
				m.Eligibility = "eligible"
			}
		}
		if m.Slot != 0 && m.SlotAssignedAtMS < birth.AcceptedAtMS {
			return ErrCorrupt
		}
		if condition.Phase == "retired" {
			m.Eligibility = "retired"
		}
		selection.Members = append(selection.Members, m)
		ownedIDs = append(ownedIDs, ids[n])
	}
	// A retained result prefix is fenced at the exact cutoff before disclosure.
	// Outgoing exclusions are not engine entrants and do not trigger C1.
	careerIDs := ownedIDs
	if len(careerIDs) == 0 {
		careerIDs = []string{"0000000000000000000000000000000000000000000000000000000000000000"}
	}
	b := new(leveldb.Batch)
	installs := []func(){}
	guards := []struct {
		g      *careerconsumer.Guard
		prefix string
	}{{s.career, ""}}
	if s.legacy.Required() {
		guards = append(guards, struct {
			g      *careerconsumer.Guard
			prefix string
		}{s.legacy, "legacy/"})
	}
	for _, binding := range guards {
		snapshot, e := binding.g.Snapshot(ctx, careerIDs, cutoff, careerstate.Cutoff)
		if e != nil {
			return e
		}
		if e = careerconsumer.RequireZero(snapshot, ownedIDs); e != nil {
			return e
		}
		key, raw, guard, install, e := binding.g.PrepareFence(snapshot, careerIDs)
		if e != nil {
			return e
		}
		if raw != nil {
			b.Put([]byte(binding.prefix+key), raw)
			b.Put([]byte(binding.prefix+careerconsumer.GuardKey), guard)
		}
		installs = append(installs, install)
	}
	commit, e := prepare(selection)
	if e != nil {
		return e
	}
	if commit == nil {
		return ErrInvalid
	}
	current, e := s.source.Head(ctx)
	if e != nil {
		return e
	}
	lease, e := s.source.AcquireHead(ctx, current)
	if e != nil {
		return e
	}
	defer lease.Close()
	if e = s.checkCareer(ctx); e != nil {
		return e
	}
	m := s.meta
	m.ClockMS = now
	if e = encodedPut(b, []byte("meta"), m, OwnedMaxStatusSize); e != nil {
		return e
	}
	committed := false
	e = s.fence(ctx, session, gymaccount.View, func(check func() error) error {
		if e := lease.Check(); e != nil {
			return e
		}
		if e := check(); e != nil {
			return e
		}
		if e := s.persist(b, m); e != nil {
			return e
		}
		committed = true
		for _, install := range installs {
			install()
		}
		if e := check(); e != nil {
			return e
		}
		return commit(CutoffFence{1, s.meta.StoreID, cutoff, now, s.meta.OperationCount}, check)
	})
	if e != nil && committed {
		return errors.Join(ErrOutcomeUnknown, e)
	}
	return e
}
