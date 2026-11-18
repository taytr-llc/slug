// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"context"
	"errors"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/internal/careerconsumer"
	"github.com/taytr-llc/slug/internal/careerstate"
)

type EntrySourceInfo struct {
	Version             uint32 `json:"version"`
	StoreID             string `json:"store_id"`
	Pin                 Pin    `json:"pin"`
	Checkpoint          uint64 `json:"checkpoint"`
	ObservedSourceCount uint64 `json:"observed_source_count"`
	RosterRevision      uint64 `json:"roster_revision"`
}
type EntrySelectionMember struct {
	Slot uint32 `json:"slot"`
	Item Item   `json:"item"`
}
type EntrySelection struct {
	Version uint32                 `json:"version"`
	Status  Status                 `json:"status"`
	AsOfMS  int64                  `json:"as_of_ms"`
	Members []EntrySelectionMember `json:"members"`
}

// EntrySourceInfo binds an actual borrowed store without an account assertion,
// proof, live registry query, or clock write. It is not eligibility evidence.
func (s *Store) EntrySourceInfo(ctx context.Context) (EntrySourceInfo, error) {
	if normal() != nil {
		return EntrySourceInfo{}, ErrProfile
	}
	if ctx == nil || s == nil {
		return EntrySourceInfo{}, ErrInvalid
	}
	work, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(s.life, cancel)
	defer func() { stop(); cancel() }()
	select {
	case <-work.Done():
		return EntrySourceInfo{}, ErrCancelled
	case s.gate <- struct{}{}:
	}
	defer func() { <-s.gate }()
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.stateLocked(); e != nil {
		return EntrySourceInfo{}, e
	}
	if work.Err() != nil {
		return EntrySourceInfo{}, ErrCancelled
	}
	return EntrySourceInfo{Version1, s.meta.StoreID, s.meta.Pin, s.meta.Checkpoint, s.meta.ObservedSourceCount, s.meta.RosterRevision}, nil
}
func entryIDs(ids []string) ([]string, error) {
	if len(ids) < 1 || len(ids) > 5 {
		return nil, ErrInvalid
	}
	for i, id := range ids {
		if !hashID(id) || (i > 0 && ids[i-1] >= id) {
			return nil, ErrInvalid
		}
	}
	return append([]string{}, ids...), nil
}
func (s *Store) entrySelection(ctx context.Context, session Session, cap gymaccount.Capability, expected *uint64, ids []string, fn func(EntrySelection, func() error) error) error {
	ids, e := entryIDs(ids)
	if e != nil || fn == nil {
		return ErrInvalid
	}
	ctx, done, e := s.begin(ctx, session, cap)
	if e != nil {
		return e
	}
	defer done()
	if expected != nil && (*expected > MaxOperations || s.meta.RosterRevision != *expected) {
		return ErrConflict
	}
	if _, e = s.sourceInfo(ctx); e != nil {
		return e
	}
	checkpoint, revision := s.meta.Checkpoint, s.meta.RosterRevision
	assignments := make([]Assignment, len(ids))
	items := make([]Item, len(ids))
	for i, id := range ids {
		assignments[i], e = s.assignment(id)
		if errors.Is(e, ErrNotFound) {
			return ErrSyncRequired
		}
		if e != nil {
			return e
		}
	}
	for i, id := range ids {
		if e = s.withLease(ctx, session, cap, func(check func() error) error { return check() }); e != nil {
			return e
		}
		items[i], e = s.readSource(ctx, assignments[i].Sequence, id)
		if e != nil {
			return e
		}
		if items[i].Assignment != assignments[i] {
			return ErrCorrupt
		}
	}
	return s.withLease(ctx, session, cap, func(check func() error) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.meta.Checkpoint != checkpoint || s.meta.RosterRevision != revision {
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
		if s.career.Required() {
			view, e := s.career.Snapshot(ctx, ids, now, careerstate.Current)
			if e != nil {
				return e
			}
			if e = careerconsumer.RequireZero(view, ids); e != nil {
				return e
			}
		}
		if e = projectItems(items, now); e != nil {
			return e
		}
		result := EntrySelection{Version: Version1, Status: s.statusLocked(), AsOfMS: now, Members: make([]EntrySelectionMember, 0, len(items))}
		result.Status.LastAsOfMS = now
		for _, item := range items {
			var slot uint32
			for i, id := range s.roster.Slots {
				if id == item.Assignment.ProspectID {
					slot = uint32(i + 1)
				}
			}
			// Expose only immutable public facts, never the private source receipt.
			public := Item{Assignment: item.Assignment, Manifest: item.Manifest, Condition: item.Condition}
			result.Members = append(result.Members, EntrySelectionMember{Slot: slot, Item: public})
		}
		if e = boundedResult(result); e != nil {
			return e
		}
		if e = check(); e != nil {
			return e
		}
		next := s.meta
		next.LastAsOfMS = now
		if e = s.commitLocked(new(leveldb.Batch), next); e != nil {
			return e
		}
		// All stable writes precede the foreign commit. Never write after fn.
		if e = check(); e != nil {
			return e
		}
		return fn(result, check)
	})
}
func (s *Store) ReadEntrySelection(ctx context.Context, session Session, ids []string) (EntrySelection, error) {
	var out EntrySelection
	e := s.entrySelection(ctx, session, gymaccount.View, nil, ids, func(v EntrySelection, check func() error) error {
		if e := check(); e != nil {
			return e
		}
		out = v
		return nil
	})
	if e != nil {
		return EntrySelection{}, e
	}
	return out, nil
}

// WithEntrySelection holds stable's gate, original account lease and mutex
// across one trusted synchronous callback. The callback must not reenter them.
func (s *Store) WithEntrySelection(ctx context.Context, session Session, expectedRosterRevision uint64, ids []string, fn func(EntrySelection, func() error) error) error {
	return s.entrySelection(ctx, session, gymaccount.EntryManage, &expectedRosterRevision, ids, fn)
}
