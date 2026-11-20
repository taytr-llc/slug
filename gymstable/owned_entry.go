// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"context"
	"errors"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/internal/careerconsumer"
)

type OwnedEntrySelectionMember struct {
	Slot uint32    `json:"slot"`
	Item OwnedItem `json:"item"`
}
type OwnedEntrySelection struct {
	Version uint32                      `json:"version"`
	Status  OwnedStatus                 `json:"status"`
	AsOfMS  int64                       `json:"as_of_ms"`
	Members []OwnedEntrySelectionMember `json:"members"`
}

// WithEntrySelection prepares source facts, careers and the consumer's exact
// bytes outside the account lease. Only its returned bounded commit enters the
// original EntryManage fence while this stable's writer lane remains retained.
func (s *OwnedStore) WithEntrySelection(ctx context.Context, session Session, revision uint64, ids []string, prepare func(OwnedEntrySelection) (func(func() error) error, error)) error {
	return s.ownedEntrySelection(ctx, session, gymaccount.EntryManage, &revision, ids, prepare)
}
func (s *OwnedStore) ReadEntrySelection(ctx context.Context, session Session, ids []string) (OwnedEntrySelection, error) {
	var out OwnedEntrySelection
	e := s.ownedEntrySelection(ctx, session, gymaccount.View, nil, ids, func(v OwnedEntrySelection) (func(func() error) error, error) {
		return func(check func() error) error {
			if e := check(); e != nil {
				return e
			}
			out = v
			return nil
		}, nil
	})
	if e != nil {
		return OwnedEntrySelection{}, e
	}
	return out, nil
}
func (s *OwnedStore) ownedEntrySelection(ctx context.Context, session Session, cap gymaccount.Capability, revision *uint64, ids []string, prepare func(OwnedEntrySelection) (func(func() error) error, error)) error {
	ids, e := entryIDs(ids)
	if e != nil || prepare == nil {
		return ErrInvalid
	}
	ctx, done, e := s.begin(ctx, session, cap)
	if e != nil {
		return e
	}
	defer done()
	if revision != nil && *revision != s.meta.RosterRevision {
		return ErrConflict
	}
	c, e := s.source.CaptureOwned(ctx, s.meta.Pin.Stable.Identity.GymPublicKey)
	if e != nil {
		return e
	}
	now, e := s.clock()
	if e != nil {
		return e
	}
	out := OwnedEntrySelection{1, s.status(c.Head()), now, make([]OwnedEntrySelectionMember, 0, len(ids))}
	out.Status.LastAsOfMS = now
	for _, id := range ids {
		v, e := s.source.OwnedItem(ctx, c, id)
		if e != nil {
			return e
		}
		item, e := s.projectOwned(ctx, v, now)
		if e != nil {
			return e
		}
		if item.Career.Totals.Wins+item.Career.Totals.Losses+item.Career.Totals.Draws != 0 {
			return &careerconsumer.IneligibleError{ProspectIDs: []string{id}}
		}
		var slot uint32
		for n, x := range s.roster.Slots {
			if x == id {
				slot = uint32(n + 1)
			}
		}
		out.Members = append(out.Members, OwnedEntrySelectionMember{slot, item})
	}
	commit, e := prepare(out)
	if e != nil {
		return e
	}
	if commit == nil {
		return ErrInvalid
	}
	head, e := s.source.AcquireHead(ctx, c.Head())
	if e != nil {
		return e
	}
	defer head.Close()
	if e = s.checkCareer(ctx); e != nil {
		return e
	}
	m := s.meta
	m.ClockMS = now
	m.LastAsOfMS = now
	b := new(leveldb.Batch)
	if e = encodedPut(b, []byte("meta"), m, OwnedMaxStatusSize); e != nil {
		return e
	}
	committed := false
	e = s.fence(ctx, session, cap, func(check func() error) error {
		if e := head.Check(); e != nil {
			return e
		}
		if e := check(); e != nil {
			return e
		}
		if !s.readOnly {
			if e := s.persist(b, m); e != nil {
				return e
			}
			committed = true
		}
		if e := check(); e != nil {
			return e
		}
		return commit(check)
	})
	if e != nil && committed {
		return errors.Join(ErrOutcomeUnknown, e)
	}
	return e
}
