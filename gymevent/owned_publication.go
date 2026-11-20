// Copyright (c) 2026 The SLUG developers. ISC license.
package gymevent

import (
	"bytes"
	"context"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymstable"
)

// OwnedSeedSourceInfo is an observation from the concrete owned event store,
// not an IPC claim or a legacy event handle with rewritten identity fields.
type OwnedSeedSourceInfo struct {
	Version              uint32
	StoreID              string
	Pin                  OwnedPin
	ObservedAtMS         int64
	ParticipantAccountID string
	ParticipantGymID     string
}

func (s *OwnedStore) publicationView(ctx context.Context, entry *gymentry.OwnedStore, session gymstable.Session, cap gymaccount.Capability, result bool, fn func(OwnedSeedSourceInfo, func() error) error) error {
	if entry == nil || fn == nil {
		return ErrInvalid
	}
	// Validate and copy the configured source before entering an account fence.
	info, e := entry.SnapshotSourceInfo(ctx)
	if e != nil {
		return e
	}
	if _, e = OwnedParticipantID(s.meta.Pin, info.Pin); e != nil {
		return ErrForbidden
	}
	id, _ := gymentry.OwnedID(info.Pin)
	if info.StoreID != id {
		return ErrCorrupt
	}
	pin, e := ValidateOwnedPin(s.meta.Pin)
	if e != nil {
		return ErrCorrupt
	}
	observe := entry.WithSeedView
	if result {
		observe = entry.WithResultView
	}
	return observe(ctx, session, cap, func(v gymentry.OwnedSnapshotSourceInfo, check func() error) error {
		if v.Version != 1 || v.StoreID != info.StoreID || v.Pin != info.Pin || check == nil {
			return ErrCorrupt
		}
		if e := check(); e != nil {
			return e
		}
		if s.life.Err() != nil {
			return ErrClosed
		}
		now, e := s.clock(v.ObservedAtMS)
		if e != nil {
			return e
		}
		if !s.readOnly && now != s.meta.ClockMS {
			meta := s.meta
			meta.ClockMS = now
			if e = s.persist(new(leveldb.Batch), meta); e != nil {
				return e
			}
		}
		p := info.Pin.StablePin.Stable
		return fn(OwnedSeedSourceInfo{1, s.meta.StoreID, pin, now, p.AccountID, p.Identity.GymID}, check)
	})
}

func (s *OwnedStore) WithSeedView(ctx context.Context, entry *gymentry.OwnedStore, session gymstable.Session, cap gymaccount.Capability, fn func(OwnedSeedSourceInfo, func() error) error) error {
	if fn == nil || (cap != gymaccount.View && cap != gymaccount.SeedPublish) {
		return ErrInvalid
	}
	ctx, done, e := s.begin(ctx)
	if e != nil {
		return e
	}
	defer done()
	return s.publicationView(ctx, entry, session, cap, false, fn)
}

func (s *OwnedStore) WithResultView(ctx context.Context, entry *gymentry.OwnedStore, session gymstable.Session, cap gymaccount.Capability, fn func(OwnedSeedSourceInfo, func() error) error) error {
	if fn == nil || (cap != gymaccount.View && cap != gymaccount.ResultPublish) {
		return ErrInvalid
	}
	ctx, done, e := s.begin(ctx)
	if e != nil {
		return e
	}
	defer done()
	return s.publicationView(ctx, entry, session, cap, true, fn)
}

func (s *OwnedStore) WithSeedSnapshot(ctx context.Context, entry *gymentry.OwnedStore, session gymstable.Session, saturday string, prepare func(OwnedSnapshotReceipt) (func(OwnedSeedSourceInfo, func() error) error, error)) error {
	if prepare == nil {
		return ErrInvalid
	}
	ctx, done, e := s.begin(ctx)
	if e != nil {
		return e
	}
	defer done()
	ev, e := OwnedEventForDate(s.meta.Pin, saturday)
	if e != nil {
		return e
	}
	if e = s.publicationView(ctx, entry, session, gymaccount.View, false, func(_ OwnedSeedSourceInfo, check func() error) error { return check() }); e != nil {
		return e
	}
	receipt, found, e := s.receipt(ev)
	if e != nil {
		return e
	}
	if !found {
		return ErrIncomplete
	}
	canonical, e := MarshalOwnedSnapshotReceipt(s.meta.Pin, receipt)
	if e != nil {
		return ErrCorrupt
	}
	commit, e := prepare(receipt)
	if e != nil {
		return e
	}
	if commit == nil {
		return ErrInvalid
	}
	return s.publicationView(ctx, entry, session, gymaccount.View, false, func(info OwnedSeedSourceInfo, check func() error) error {
		raw, found, e := s.raw(key('s', ev.EventID), OwnedMaxReceiptSize)
		if e != nil {
			return e
		}
		if !found || !bytes.Equal(raw, canonical) {
			return ErrConflict
		}
		if e = check(); e != nil {
			return e
		}
		return commit(info, check)
	})
}
