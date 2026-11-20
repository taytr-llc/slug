// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
	"github.com/taytr-llc/slug/fightercondition"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/internal/careerconsumer"
	"github.com/taytr-llc/slug/internal/careerstate"
)

func cutoffHash(domain string, parts ...[]byte) string {
	h := sha256.New()
	h.Write([]byte(domain))
	for _, p := range parts {
		h.Write(p)
	}
	return hex.EncodeToString(h.Sum(nil))
}
func cutoffRaw(s string) []byte { b, _ := hex.DecodeString(s); return b }
func cutoffLE(n uint64) []byte  { b := make([]byte, 8); binary.LittleEndian.PutUint64(b, n); return b }
func cutoffLP(b []byte) []byte {
	r := make([]byte, 4, len(b)+4)
	binary.LittleEndian.PutUint32(r, uint32(len(b)))
	return append(r, b...)
}

// Operation values are bounded before decoding/copying, unlike an unchecked Get.
func (s *Store) cutoffOperation(id string) (Operation, error) {
	it := s.db.NewIterator(nil, nil)
	defer it.Release()
	key := idKey('o', id)
	if !it.Seek(key) || !bytes.Equal(it.Key(), key) || len(it.Value()) > MaxOperationSize {
		return Operation{}, ErrCorrupt
	}
	var o Operation
	if decode(it.Value(), MaxOperationSize, 5, &o, true) != nil || !validOperation(o, s.meta) || o.Intent.OperationID != id || it.Error() != nil {
		return Operation{}, ErrCorrupt
	}
	return o, nil
}
func (s *Store) cutoffRoster(ctx context.Context, c int64) (CutoffRoster, error) {
	out := CutoffRoster{Version: 1, StoreID: s.meta.StoreID, CutoffMS: c}
	out.HistoryDigest = cutoffHash("SLUG stable cutoff history\x00v1\x00", cutoffRaw(out.StoreID), cutoffLE(uint64(c)))
	if s.meta.OperationCount > MaxOperations {
		return out, ErrCorrupt
	}
	// Only bounded sequence→ID metadata is retained, never the operation bodies.
	var ids [MaxOperations]string
	it := s.db.NewIterator(util.BytesPrefix([]byte{'o'}), nil)
	count := uint32(0)
	for it.Next() {
		if ctx.Err() != nil {
			it.Release()
			return out, ErrCancelled
		}
		k, v := it.Key(), it.Value()
		if len(k) != 33 || count >= s.meta.OperationCount || len(v) > MaxOperationSize {
			it.Release()
			return out, ErrCorrupt
		}
		var o Operation
		if decode(v, MaxOperationSize, 5, &o, true) != nil || !validOperation(o, s.meta) || o.Intent.OperationID != hex.EncodeToString(k[1:]) || ids[o.Intent.Sequence-1] != "" {
			it.Release()
			return out, ErrCorrupt
		}
		ids[o.Intent.Sequence-1] = o.Intent.OperationID
		count++
	}
	ie := it.Error()
	it.Release()
	if ie != nil || count != s.meta.OperationCount {
		return out, ErrCorrupt
	}
	var slots [10]string
	var rev uint64
	var last int64
	pending := ""
	for n := uint32(0); n < count; n++ {
		if ctx.Err() != nil {
			return out, ErrCancelled
		}
		if ids[n] == "" {
			return out, ErrCorrupt
		}
		o, e := s.cutoffOperation(ids[n])
		if e != nil {
			return out, e
		}
		i := o.Intent
		if i.Sequence != uint64(n+1) || i.PreparedAtMS < last || i.Request.ExpectedRevision != rev {
			return out, ErrCorrupt
		}
		last = i.PreparedAtMS
		if id := i.Request.ProspectID; id != "" {
			if _, e := s.assignment(id); e != nil {
				return out, ErrCorrupt
			}
		}
		switch o.State {
		case "prepared":
			if n+1 != count {
				return out, ErrCorrupt
			}
			pending = i.OperationID
		case "abandoned":
			last = o.AbandonedAtMS
		case "committed":
			r := o.Receipt
			idx := r.Request.Slot - 1
			if r.PreviousRevision != rev || r.PreviousProspectID != slots[idx] {
				return out, ErrCorrupt
			}
			changed := slots[idx] != r.Request.ProspectID
			slots[idx] = r.Request.ProspectID
			if changed {
				rev++
			}
			if r.Changed != changed || r.Revision != rev || r.Slots != slots {
				return out, ErrCorrupt
			}
			last = r.CommittedAtMS
			if r.CommittedAtMS < c {
				if r.Request.ProspectID != "" {
					a, e := s.assignment(r.Request.ProspectID)
					if e != nil || a.AcceptedAtMS > r.CommittedAtMS || a.BornAtMS > r.CommittedAtMS {
						return out, ErrCorrupt
					}
				}
				out.Slots = slots
				out.Revision = rev
				out.CommittedCount++
				out.LastSequence = i.Sequence
				if changed {
					out.AssignedAtMS[idx] = 0
					if r.Request.ProspectID != "" {
						out.AssignedAtMS[idx] = r.CommittedAtMS
					}
				}
				raw, _ := json.Marshal(o)
				out.HistoryDigest = cutoffHash("SLUG stable cutoff row\x00v1\x00", cutoffRaw(out.HistoryDigest), cutoffLE(i.Sequence), cutoffLP(raw))
			}
		}
		if n+1 == count && i.OperationID != s.meta.LastOperationID {
			return out, ErrCorrupt
		}
	}
	if pending != s.meta.PendingOperationID || rev != s.meta.RosterRevision || slots != s.roster.Slots {
		return out, ErrCorrupt
	}
	return out, nil
}

// WithCutoffSelection drains roster writers and verifies selected historical
// source facts. Preparation is outside account leases; its final callback runs
// once after the durable clock fence and must not reenter any held authority.
func (s *Store) WithCutoffSelection(ctx context.Context, session Session, cutoffMS int64, ids []string, prepare func(CutoffSelection) (func(CutoffFence, func() error) error, error)) error {
	if s == nil || prepare == nil || cutoffMS <= 0 || cutoffMS > MaxTimestampMS || len(ids) > 5 {
		return ErrInvalid
	}
	for i, id := range ids {
		if !hashID(id) || (i > 0 && ids[i-1] >= id) {
			return ErrInvalid
		}
	}
	ids = append([]string{}, ids...)
	ctx, done, e := s.begin(ctx, session, gymaccount.View)
	if e != nil {
		return e
	}
	defer done()
	if s.meta.ClockMS < cutoffMS {
		return ErrClock
	}
	careerIDs := ids
	if len(careerIDs) == 0 {
		careerIDs = []string{"0000000000000000000000000000000000000000000000000000000000000000"}
	}
	var careerView careerstate.Snapshot
	if s.career.Required() {
		careerView, e = s.career.Snapshot(ctx, careerIDs, cutoffMS, careerstate.Cutoff)
		if e != nil {
			return e
		}
		if e = careerconsumer.RequireZero(careerView, ids); e != nil {
			return e
		}
	}
	if _, e = s.sourceInfo(ctx); e != nil {
		return e
	}
	roster, e := s.cutoffRoster(ctx, cutoffMS)
	if e != nil {
		return e
	}
	checkpoint, operationCount, revision := s.meta.Checkpoint, s.meta.OperationCount, s.meta.RosterRevision
	assignments := make([]Assignment, len(ids))
	selection := CutoffSelection{Version: 1, Roster: roster, Members: make([]CutoffMember, 0, len(ids))}
	for i, id := range ids {
		if e = s.withLease(ctx, session, gymaccount.View, nil); e != nil {
			return e
		}
		a, e := s.assignment(id)
		if e != nil {
			return e
		}
		assignments[i] = a
		item, e := s.readSource(ctx, a.Sequence, id)
		if e != nil {
			return e
		}
		if item.Assignment != a || a.AcceptedAtMS >= cutoffMS || a.BornAtMS >= cutoffMS {
			return ErrCorrupt
		}
		condition, e := fightercondition.Project(1, item.Manifest.DNA, a.BornAtMS, cutoffMS, 0)
		if e != nil {
			return ErrCorrupt
		}
		member := CutoffMember{Assignment: a, Class: item.Manifest.Physical.Class, ConditionAtCutoff: condition}
		for n, x := range roster.Slots {
			if x == id {
				member.Slot = uint32(n + 1)
				member.SlotAssignedAtMS = roster.AssignedAtMS[n]
			}
		}
		if member.Slot != 0 && a.AcceptedAtMS > member.SlotAssignedAtMS {
			return ErrCorrupt
		}
		selection.Members = append(selection.Members, member)
	}
	if ctx.Err() != nil {
		return ErrCancelled
	}
	commit, e := prepare(selection)
	if e != nil {
		return e
	}
	if commit == nil {
		return ErrInvalid
	}
	if ctx.Err() != nil {
		return ErrCancelled
	}
	if e = s.sharedSourceFence(ctx); e != nil {
		return e
	}
	return s.withLease(ctx, session, gymaccount.View, func(check func() error) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.meta.Checkpoint != checkpoint || s.meta.OperationCount != operationCount || s.meta.RosterRevision != revision {
			return ErrConflict
		}
		for _, a := range assignments {
			current, e := s.assignment(a.ProspectID)
			if e != nil {
				return e
			}
			if a != current {
				return ErrConflict
			}
		}
		now, e := s.observeLocked()
		if e != nil {
			return e
		}
		if now < cutoffMS {
			return ErrClock
		}
		if e = check(); e != nil {
			return e
		}
		s.mark("cutoff-fenced")
		if s.career.Required() {
			current, e := s.career.Snapshot(ctx, careerIDs, cutoffMS, careerstate.Cutoff)
			if e != nil {
				return e
			}
			if current.Digest() != careerView.Digest() {
				return ErrConflict
			}
			key, raw, guardRaw, install, e := s.career.PrepareFence(careerView, careerIDs)
			if e != nil {
				if e == careerconsumer.ErrCapacity {
					return ErrCapacity
				}
				return e
			}
			if raw != nil {
				b := new(leveldb.Batch)
				b.Put([]byte(key), raw)
				b.Put([]byte(careerconsumer.GuardKey), guardRaw)
				s.mark("before-career-cutoff")
				if e = s.commitLocked(b, s.meta); e != nil {
					return e
				}
				install()
				s.mark("career-cutoff-synced")
				if ctx.Err() != nil {
					return ErrOutcomeUnknown
				}
			}
		}
		return commit(CutoffFence{1, s.meta.StoreID, cutoffMS, now, s.meta.OperationCount}, check)
	})
}
