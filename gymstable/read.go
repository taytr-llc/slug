// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"context"
	"encoding/binary"
	"encoding/json"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/fightercondition"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/fightermanifest"
	"github.com/taytr-llc/slug/fighterportrait"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/registry"
)

func sourceRecordDigest(r registry.ManifestRecord) (string, error) {
	raw, e := json.Marshal(r)
	if e != nil || len(raw) > registry.MaxRecordSize {
		return "", ErrCorrupt
	}
	return domainHash("SLUG stable source record\x00v1\x00", raw), nil
}

// assignmentForRecord accepts only a value obtained from this store's concrete
// verified source. It is private: parsed JSON is never an assignment authority.
func assignmentForRecord(r registry.ManifestRecord, p Pin) (Assignment, error) {
	if r.Version != registry.Version5 || r.Sequence == 0 || r.Sequence > MaxAssignments ||
		r.Verified.Network != p.Source.Network || r.Verified.Genesis != p.Source.Genesis ||
		r.Verified.GymID != p.Identity.GymID || r.Authorization.PublicKey != p.Identity.GymPublicKey ||
		r.Job.AuthorityID != p.Source.AuthorityID || r.Job.DescriptorID != p.Source.DescriptorID ||
		r.StaticBinding.DescriptorID != p.Source.DescriptorID {
		return Assignment{}, ErrCorrupt
	}
	digest, e := sourceRecordDigest(r)
	if e != nil {
		return Assignment{}, e
	}
	a := Assignment{Version: Version1, Sequence: r.Sequence, ProspectID: r.Verified.ProspectID, GymID: r.Verified.GymID,
		WorkHash: r.Verified.WorkHash, BornAtMS: r.Birth.BornAtMS, AcceptedAtMS: r.Birth.AcceptedAtMS,
		DescriptorID: r.StaticBinding.DescriptorID, ManifestDigest: r.StaticBinding.ManifestDigest,
		PortraitDigest: r.StaticBinding.PortraitDigest, SourceRecordDigest: digest}
	if !validAssignment(a, p) {
		return Assignment{}, ErrCorrupt
	}
	return a, nil
}

func itemForRecord(r registry.ManifestRecord, p Pin) (Item, error) {
	a, e := assignmentForRecord(r, p)
	if e != nil {
		return Item{}, e
	}
	d, e := fighterdescriptor.ResolveCandidate(p.Source.DescriptorID)
	if e != nil {
		return Item{}, ErrCorrupt
	}
	m, e := fightermanifest.GenerateText(d.ManifestVersion, r.Verified.WorkHash)
	if e != nil || m.DNA != r.Verified.DNA || m.DNA.DNAVersion != d.DNAVersion {
		return Item{}, ErrCorrupt
	}
	md, e := fightermanifest.Digest(m)
	if e != nil || md != a.ManifestDigest {
		return Item{}, ErrCorrupt
	}
	pd, e := fighterportrait.Digest(d.RendererVersion, m)
	if e != nil || pd != a.PortraitDigest {
		return Item{}, ErrCorrupt
	}
	r.Proof.MerkleBranch = append([]string{}, r.Proof.MerkleBranch...)
	return Item{Assignment: a, Manifest: m, record: r}, nil
}

func projectItems(items []Item, asOf int64) error {
	for i := range items {
		a := items[i].Assignment
		if a.AcceptedAtMS > asOf || a.BornAtMS > asOf {
			return ErrClock
		}
		c, e := fightercondition.Project(fightercondition.Version1, items[i].Manifest.DNA, a.BornAtMS, asOf, 0)
		if e != nil {
			return ErrCorrupt
		}
		items[i].Condition = c
		raw, e := json.Marshal(items[i])
		if e != nil || len(raw) > MaxItemSize {
			return ErrCapacity
		}
	}
	return nil
}
func boundedResult(v any) error {
	raw, e := json.Marshal(v)
	if e != nil || len(raw) > MaxResponseSize {
		return ErrCapacity
	}
	return nil
}

// Each call performs at most one source verification. Source methods have no
// cancellation API; cancellation joins that proof and prevents the next call.
func (s *Store) readSource(ctx context.Context, sequence uint64, id string) (Item, error) {
	return s.readSourceMode(ctx, sequence, id, false)
}

// History preserves a learned source failure even if cancellation arrives
// while the uncancellable proof is joined. Legacy callers keep their ordering.
func (s *Store) readSourceMode(ctx context.Context, sequence uint64, id string, fatalFirst bool) (Item, error) {
	if ctx.Err() != nil {
		return Item{}, ErrCancelled
	}
	if s.proofRead != nil {
		s.proofRead()
	}
	if ctx.Err() != nil {
		return Item{}, ErrCancelled
	}
	var r registry.ManifestRecord
	var e error
	if id == "" {
		var rows []registry.ManifestRecord
		rows, e = s.source.ListManifestAdmissions(sequence-1, 1)
		if e == nil {
			if len(rows) != 1 || rows[0].Sequence != sequence {
				e = ErrCorrupt
			} else {
				r = rows[0]
			}
		}
	} else {
		r, e = s.source.GetManifestAdmission(id)
	}
	// A failed/slow source read is still an authenticated clock observation.
	s.mu.Lock()
	_, clockErr := s.observeLocked()
	s.mu.Unlock()
	if clockErr != nil {
		return Item{}, clockErr
	}
	if fatalFirst && e != nil {
		return Item{}, ErrCorrupt
	}
	if ctx.Err() != nil {
		return Item{}, ErrCancelled
	}
	if e != nil {
		return Item{}, ErrCorrupt
	}
	item, e := itemForRecord(r, s.meta.Pin)
	if e != nil {
		return Item{}, e
	}
	if sequence != 0 && item.Assignment.Sequence != sequence {
		return Item{}, ErrCorrupt
	}
	if id != "" && item.Assignment.ProspectID != id {
		return Item{}, ErrCorrupt
	}
	return item, nil
}

func (s *Store) Sync(ctx context.Context, session Session) (SyncResult, error) {
	ctx, done, e := s.begin(ctx, session, gymaccount.View)
	if e != nil {
		return SyncResult{}, e
	}
	defer done()
	count, e := s.sourceInfo(ctx)
	if e != nil {
		return SyncResult{}, e
	}
	start := s.meta.Checkpoint
	items := make([]Item, 0, MaxPageSize)
	for n := start + 1; n <= min(start+MaxPageSize, count); n++ {
		item, e := s.readSource(ctx, n, "")
		if e != nil {
			return SyncResult{}, e
		}
		exists, e := s.db.Has(idKey('i', item.Assignment.ProspectID), nil)
		if e != nil || exists {
			return SyncResult{}, ErrCorrupt
		}
		for _, prev := range items {
			if prev.Assignment.ProspectID == item.Assignment.ProspectID {
				return SyncResult{}, ErrCorrupt
			}
		}
		items = append(items, item)
	}
	var result SyncResult
	committed := false
	e = s.withLease(ctx, session, gymaccount.View, func(check func() error) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.meta.Checkpoint != start {
			return ErrConflict
		}
		now, e := s.observeLocked()
		if e != nil {
			return e
		}
		if e = projectItems(items, now); e != nil {
			return e
		}
		m := s.meta
		m.Checkpoint = start + uint64(len(items))
		m.ObservedSourceCount = count
		m.LastAsOfMS = now
		b := new(leveldb.Batch)
		for _, item := range items {
			a := item.Assignment
			if e = encodedPut(b, seqKey(a.Sequence), a, MaxAssignmentSize); e != nil {
				return e
			}
			var n [8]byte
			binary.BigEndian.PutUint64(n[:], a.Sequence)
			b.Put(idKey('i', a.ProspectID), n[:])
		}
		status := s.statusLocked()
		status.Checkpoint = m.Checkpoint
		status.ObservedSourceCount = count
		status.CaughtUp = m.Checkpoint == count
		status.LastAsOfMS = now
		result = SyncResult{Version: Version1, Status: status, AsOfMS: now, Items: items}
		if e = boundedResult(result); e != nil {
			return e
		}
		if e = check(); e != nil {
			return e
		}
		s.mark("before_sync")
		if e = s.commitLocked(b, m); e != nil {
			return e
		}
		committed = true
		s.mark("after_sync")
		return nil
	})
	if e != nil || ctx.Err() != nil {
		if committed {
			return SyncResult{}, ErrOutcomeUnknown
		}
		if e != nil {
			return SyncResult{}, e
		}
		return SyncResult{}, ErrCancelled
	}
	return result, nil
}

func (s *Store) Inventory(ctx context.Context, session Session, after uint64, limit uint32) (Page, error) {
	ctx, done, e := s.begin(ctx, session, gymaccount.View)
	if e != nil {
		return Page{}, e
	}
	defer done()
	if limit < 1 || limit > MaxPageSize || after > s.meta.Checkpoint {
		return Page{}, ErrInvalid
	}
	if _, e = s.sourceInfo(ctx); e != nil {
		return Page{}, e
	}
	checkpoint, revision := s.meta.Checkpoint, s.meta.RosterRevision
	items := make([]Item, 0, limit)
	for n := after + 1; n <= min(after+uint64(limit), checkpoint); n++ {
		a, e := s.assignmentSequence(n)
		if e != nil {
			return Page{}, e
		}
		item, e := s.readSource(ctx, n, a.ProspectID)
		if e != nil {
			return Page{}, e
		}
		if item.Assignment != a {
			return Page{}, ErrCorrupt
		}
		items = append(items, item)
	}
	var result Page
	e = s.publishRead(ctx, session, items, checkpoint, revision, func(now int64, status Status) error {
		next := after + uint64(len(items))
		result = Page{Version: Version1, Status: status, AsOfMS: now, After: after, NextAfter: next, HasMore: next < checkpoint, Items: items}
		return boundedResult(result)
	})
	if e != nil {
		return Page{}, e
	}
	return result, nil
}
func (s *Store) Roster(ctx context.Context, session Session, firstSlot uint32, limit uint32) (RosterPage, error) {
	ctx, done, e := s.begin(ctx, session, gymaccount.View)
	if e != nil {
		return RosterPage{}, e
	}
	defer done()
	if firstSlot < 1 || firstSlot > RosterSlots || limit < 1 || limit > MaxPageSize {
		return RosterPage{}, ErrInvalid
	}
	if _, e = s.sourceInfo(ctx); e != nil {
		return RosterPage{}, e
	}
	checkpoint, revision := s.meta.Checkpoint, s.meta.RosterRevision
	end := min(firstSlot+limit-1, RosterSlots)
	items := make([]Item, 0, limit)
	slots := make([]uint32, 0, limit)
	for slot := firstSlot; slot <= end; slot++ {
		id := s.roster.Slots[slot-1]
		if id == "" {
			continue
		}
		a, e := s.assignment(id)
		if e != nil {
			return RosterPage{}, e
		}
		item, e := s.readSource(ctx, a.Sequence, id)
		if e != nil {
			return RosterPage{}, e
		}
		if a != item.Assignment {
			return RosterPage{}, ErrCorrupt
		}
		items = append(items, item)
		slots = append(slots, slot)
	}
	var result RosterPage
	e = s.publishRead(ctx, session, items, checkpoint, revision, func(now int64, status Status) error {
		result = RosterPage{Version: Version1, Status: status, AsOfMS: now, FirstSlot: firstSlot, Items: make([]RosterItem, 0, len(items))}
		if end < RosterSlots {
			result.NextSlot = end + 1
		}
		for i, item := range items {
			result.Items = append(result.Items, RosterItem{Slot: slots[i], Item: item})
		}
		return boundedResult(result)
	})
	if e != nil {
		return RosterPage{}, e
	}
	return result, nil
}
func (s *Store) publishRead(ctx context.Context, session Session, items []Item, checkpoint, revision uint64, build func(int64, Status) error) error {
	e := s.withLease(ctx, session, gymaccount.View, func(check func() error) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.meta.Checkpoint != checkpoint || s.meta.RosterRevision != revision {
			return ErrConflict
		}
		for _, item := range items {
			a, e := s.assignment(item.Assignment.ProspectID)
			if e != nil {
				return e
			}
			if a != item.Assignment {
				return ErrConflict
			}
		}
		now, e := s.observeLocked()
		if e != nil {
			return e
		}
		if e = projectItems(items, now); e != nil {
			return e
		}
		status := s.statusLocked()
		status.LastAsOfMS = now
		if e = build(now, status); e != nil {
			return e
		}
		if e = check(); e != nil {
			return e
		}
		m := s.meta
		m.LastAsOfMS = now
		return s.commitLocked(new(leveldb.Batch), m)
	})
	if e != nil {
		return e
	}
	if ctx.Err() != nil {
		return ErrCancelled
	}
	return nil
}
