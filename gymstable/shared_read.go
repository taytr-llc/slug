// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"bytes"
	"context"
	"encoding/binary"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
	"github.com/taytr-llc/slug/gymaccount"
)

// The caller holds the stable operation gate. Retain only bounded public IDs,
// never foreign proof/manifest bodies. Original registry authority is external.
func (s *Store) sharedSkippedIDs() (map[string]bool, error) {
	seen := make(map[string]bool)
	it := s.db.NewIterator(util.BytesPrefix([]byte{'s'}), nil)
	defer it.Release()
	for it.Next() {
		k := it.Key()
		if bytes.Equal(k, []byte("shared")) {
			continue
		}
		var skipped sharedSkip
		if len(k) != 9 || len(seen) >= MaxAssignments || decode(it.Value(), maxSharedSkipSize, 1, &skipped, true) != nil || !validSharedSkip(skipped, s.meta.Pin, s.meta.Checkpoint) || skipped.Sequence != binary.BigEndian.Uint64(k[1:]) || seen[skipped.ProspectID] {
			return nil, ErrCorrupt
		}
		seen[skipped.ProspectID] = true
	}
	if it.Error() != nil || s.meta.assignmentCount > s.meta.Checkpoint || uint64(len(seen)) != s.meta.Checkpoint-s.meta.assignmentCount {
		return nil, ErrCorrupt
	}
	return seen, nil
}

func (s *Store) syncShared(ctx context.Context, session Session) (SyncResult, error) {
	count, e := s.sourceInfoMode(ctx, true)
	if e != nil {
		return SyncResult{}, e
	}
	start := s.meta.Checkpoint
	end := min(start+MaxPageSize, count)
	seen, e := s.sharedSkippedIDs()
	if e != nil {
		return SyncResult{}, e
	}
	items := make([]Item, 0, MaxPageSize)
	skips := make([]sharedSkip, 0, MaxPageSize)
	var sourceTime int64
	for n := start + 1; n <= end; n++ {
		if e = s.withLease(ctx, session, gymaccount.View, nil); e != nil {
			return SyncResult{}, e
		}
		r, e := s.readVerifiedSource(ctx, n, "", true)
		if e != nil {
			return SyncResult{}, e
		}
		if !sharedRecordScope(r, s.meta.Pin) || seen[r.Verified.ProspectID] {
			return SyncResult{}, ErrCorrupt
		}
		exists, e := s.db.Has(idKey('i', r.Verified.ProspectID), nil)
		if e != nil || exists {
			return SyncResult{}, ErrCorrupt
		}
		seen[r.Verified.ProspectID] = true
		sourceTime = max(sourceTime, r.Birth.AcceptedAtMS, r.Birth.BornAtMS)
		if r.Verified.GymID == s.meta.Pin.Identity.GymID || r.Authorization.PublicKey == s.meta.Pin.Identity.GymPublicKey {
			item, e := itemForRecord(r, s.meta.Pin)
			if e != nil {
				return SyncResult{}, e
			}
			items = append(items, item)
		} else {
			digest, e := sourceRecordDigest(r)
			if e != nil {
				return SyncResult{}, e
			}
			v := sharedSkip{Version1, r.Sequence, r.Verified.GymID, r.Authorization.PublicKey, r.Verified.ProspectID, digest}
			if !validSharedSkip(v, s.meta.Pin, end) {
				return SyncResult{}, ErrCorrupt
			}
			skips = append(skips, v)
		}
	}
	if s.meta.assignmentCount+uint64(len(items)) > MaxAssignments {
		return SyncResult{}, ErrCapacity
	}
	// Detect closure/rollback learned during a real proof. Fresh growth is kept
	// as highwater, but this bounded call consumes only its original <=8 rows.
	if _, e = s.sourceInfoMode(ctx, true); e != nil {
		return SyncResult{}, e
	}
	var out SyncResult
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
		if now < sourceTime {
			return ErrClock
		}
		if e = projectItems(items, now); e != nil {
			return e
		}
		m := s.meta
		m.Checkpoint = end
		m.assignmentCount += uint64(len(items))
		m.LastAsOfMS = now
		batch := new(leveldb.Batch)
		for _, item := range items {
			a := item.Assignment
			if e = encodedPut(batch, seqKey(a.Sequence), a, MaxAssignmentSize); e != nil {
				return e
			}
			var n [8]byte
			binary.BigEndian.PutUint64(n[:], a.Sequence)
			batch.Put(idKey('i', a.ProspectID), n[:])
		}
		for _, skipped := range skips {
			if e = encodedPut(batch, sharedSkipKey(skipped.Sequence), skipped, maxSharedSkipSize); e != nil {
				return e
			}
		}
		status := s.statusLocked()
		status.Checkpoint, status.CaughtUp, status.LastAsOfMS = end, end == m.ObservedSourceCount, now
		out = SyncResult{Version1, status, now, items}
		if e = boundedResult(out); e != nil {
			return e
		}
		if e = check(); e != nil {
			return e
		}
		s.mark("before_sync")
		if e = s.commitLocked(batch, m); e != nil {
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
	return out, nil
}

func (s *Store) sharedAssignmentsAfter(after uint64, limit uint32) ([]Assignment, bool, error) {
	rows := make([]Assignment, 0, limit)
	it := s.db.NewIterator(util.BytesPrefix([]byte{'a'}), nil)
	defer it.Release()
	last := after
	more := false
	for ok := it.Seek(seqKey(after + 1)); ok; ok = it.Next() {
		k := it.Key()
		if len(k) != 9 {
			return nil, false, ErrCorrupt
		}
		n := binary.BigEndian.Uint64(k[1:])
		if n <= last || n > s.meta.Checkpoint {
			return nil, false, ErrCorrupt
		}
		a, e := s.assignmentSequence(n)
		if e != nil || !bytes.Equal(k, seqKey(a.Sequence)) {
			return nil, false, ErrCorrupt
		}
		if len(rows) == int(limit) {
			more = true
			break
		}
		rows = append(rows, a)
		last = n
	}
	if it.Error() != nil || uint64(len(rows)) > s.meta.assignmentCount || more && uint64(len(rows)) >= s.meta.assignmentCount {
		return nil, false, ErrCorrupt
	}
	return rows, more, nil
}

func (s *Store) sharedInventory(ctx context.Context, session Session, after uint64, limit uint32) (Page, error) {
	if limit < 1 || limit > MaxPageSize || after > s.meta.Checkpoint {
		return Page{}, ErrInvalid
	}
	if _, e := s.sourceInfoMode(ctx, true); e != nil {
		return Page{}, e
	}
	checkpoint, revision := s.meta.Checkpoint, s.meta.RosterRevision
	assignments, more, e := s.sharedAssignmentsAfter(after, limit)
	if e != nil {
		return Page{}, e
	}
	items := make([]Item, 0, len(assignments))
	for _, a := range assignments {
		if e = s.withLease(ctx, session, gymaccount.View, nil); e != nil {
			return Page{}, e
		}
		item, e := s.readSourceMode(ctx, a.Sequence, a.ProspectID, true)
		if e != nil {
			return Page{}, e
		}
		if item.Assignment != a {
			return Page{}, ErrCorrupt
		}
		items = append(items, item)
	}
	var out Page
	e = s.publishRead(ctx, session, items, checkpoint, revision, func(now int64, status Status) error {
		next := checkpoint
		if more {
			next = items[len(items)-1].Assignment.Sequence
		}
		out = Page{Version1, status, now, after, next, more, items}
		return boundedResult(out)
	})
	if e != nil {
		return Page{}, e
	}
	return out, nil
}

// SharedRecent selects the latest local assignments, never the last global
// source positions. It returns ordinary ascending source-cursor pagination.
func (s *Store) SharedRecent(ctx context.Context, session Session, limit uint32) (Page, error) {
	if e := normal(); e != nil {
		return Page{}, e
	}
	if s == nil {
		return Page{}, ErrInvalid
	}
	ctx, done, e := s.begin(ctx, session, gymaccount.View)
	if e != nil {
		return Page{}, e
	}
	defer done()
	if !s.shared() || limit < 1 || limit > MaxPageSize {
		return Page{}, ErrInvalid
	}
	after := uint64(0)
	it := s.db.NewIterator(util.BytesPrefix([]byte{'a'}), nil)
	n := uint64(0)
	last := s.meta.Checkpoint + 1
	for ok := it.Last(); ok && n <= uint64(limit); ok = it.Prev() {
		k := it.Key()
		if len(k) != 9 {
			it.Release()
			return Page{}, ErrCorrupt
		}
		seq := binary.BigEndian.Uint64(k[1:])
		if seq == 0 || seq >= last {
			it.Release()
			return Page{}, ErrCorrupt
		}
		if _, e := s.assignmentSequence(seq); e != nil {
			it.Release()
			return Page{}, e
		}
		last = seq
		n++
		if n > uint64(limit) {
			after = seq
			break
		}
	}
	ie := it.Error()
	it.Release()
	if ie != nil || n != min(s.meta.assignmentCount, uint64(limit)+1) {
		return Page{}, ErrCorrupt
	}
	return s.sharedInventory(ctx, session, after, limit)
}
