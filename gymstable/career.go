// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/fightercareer"
	"github.com/taytr-llc/slug/fightercondition"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/internal/careerconsumer"
	"github.com/taytr-llc/slug/internal/careerstate"
	"github.com/taytr-llc/slug/registry"
)

const MaxCareerPageSize = 196608

var ErrCareerCandidateIneligible = careerconsumer.ErrIneligible

type CareerIneligibleError = careerconsumer.IneligibleError
type CareerItem struct {
	ProspectID string                    `json:"prospect_id"`
	Totals     fightercareer.Totals      `json:"totals"`
	Condition  fightercondition.Snapshot `json:"condition"`
}
type CareerPage struct {
	Version      uint32                 `json:"version"`
	Activation   careerstate.Activation `json:"activation"`
	CareerDigest string                 `json:"career_digest"`
	AsOfMS       int64                  `json:"as_of_ms"`
	Source       Page                   `json:"source"`
	Items        []CareerItem           `json:"items"`
}
type CareerRosterPage struct {
	Version      uint32                 `json:"version"`
	Activation   careerstate.Activation `json:"activation"`
	CareerDigest string                 `json:"career_digest"`
	AsOfMS       int64                  `json:"as_of_ms"`
	Source       RosterPage             `json:"source"`
	Items        []CareerItem           `json:"items"`
}

func OpenCareer(path string, expected Pin, source *registry.ManifestStore, activation careerstate.Activation) (*Store, error) {
	return openCareerWithClock(path, expected, source, time.Now, &activation, false)
}
func ResumeCareerBinding(path string, expected Pin, source *registry.ManifestStore, activation careerstate.Activation) (*Store, error) {
	return openCareerWithClock(path, expected, source, time.Now, &activation, true)
}

// installCareer is trusted parent work, serialized with all owned operations.
// It is never exposed as an account or browser capability.
func (s *Store) installCareer(ctx context.Context, reader *careerstate.Reader, bind bool) error {
	if s == nil || ctx == nil || reader == nil {
		return ErrInvalid
	}
	work, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(s.life, cancel)
	defer func() { stop(); cancel() }()
	select {
	case <-work.Done():
		return ErrCancelled
	case s.gate <- struct{}{}:
	}
	defer func() { <-s.gate }()
	s.mu.Lock()
	defer s.mu.Unlock()
	if e := s.stateLocked(); e != nil {
		return e
	}
	if work.Err() != nil {
		return ErrCancelled
	}
	if !bind {
		return s.career.Attach(reader, "stable", s.meta.StoreID)
	}
	raw, install, e := s.career.PrepareBind(reader, "stable", s.meta.StoreID)
	if e != nil {
		return e
	}
	if raw != nil {
		b := new(leveldb.Batch)
		b.Put([]byte(careerconsumer.GuardKey), raw)
		s.mark("before-career-bind")
		if e = s.writeBatch(b, &opt.WriteOptions{Sync: true}); e != nil {
			s.poisoned = true
			return ErrOutcomeUnknown
		}
		install()
		s.mark("career-bound")
		if work.Err() != nil {
			return ErrOutcomeUnknown
		}
	} else {
		install()
	}
	return nil
}
func (s *Store) BindCareer(ctx context.Context, reader *careerstate.Reader) error {
	return s.installCareer(ctx, reader, true)
}
func (s *Store) AttachCareer(ctx context.Context, reader *careerstate.Reader) error {
	return s.installCareer(ctx, reader, false)
}

// careerProjection rejoins the original authorization after the legacy page
// read and binds the overlay to exactly that page's assignments and as-of time.
func careerPageIDs(items []Item) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		ids = append(ids, item.Assignment.ProspectID)
	}
	sort.Strings(ids)
	// Empty pages still bind to a complete live prefix. This fixed zero ID is
	// used only for the digest request; no fighter or item is fabricated.
	if len(ids) == 0 {
		ids = []string{"0000000000000000000000000000000000000000000000000000000000000000"}
	}
	return ids
}

func (s *Store) careerProjection(ctx context.Context, session Session, status Status, asOf int64, items []Item) (careerstate.Snapshot, []CareerItem, error) {
	ctx, done, e := s.begin(ctx, session, gymaccount.View)
	if e != nil {
		return careerstate.Snapshot{}, nil, e
	}
	defer done()
	ids := careerPageIDs(items)
	view, e := s.career.Snapshot(ctx, ids, asOf, careerstate.Current)
	if e != nil {
		return careerstate.Snapshot{}, nil, e
	}
	out := make([]CareerItem, 0, len(items))
	for _, item := range items {
		totals, ok := view.Totals(item.Assignment.ProspectID)
		if !ok {
			return careerstate.Snapshot{}, nil, ErrCorrupt
		}
		condition, e := fightercondition.Project(1, item.Manifest.DNA, item.Assignment.BornAtMS, asOf, int(totals.XP))
		if e != nil {
			return careerstate.Snapshot{}, nil, ErrCorrupt
		}
		out = append(out, CareerItem{item.Assignment.ProspectID, totals, condition})
	}
	e = s.recheckCareerPage(ctx, session, status, asOf, items, view.Activation())
	return view, out, e
}
func (s *Store) CareerInventory(ctx context.Context, session Session, after uint64, limit uint32) (CareerPage, error) {
	p, e := s.Inventory(ctx, session, after, limit)
	if e != nil {
		return CareerPage{}, e
	}
	return s.careerInventoryPage(ctx, session, p)
}

// CareerSharedRecent overlays the exact sparse recent page with the same
// authoritative career snapshot used by ordinary career inventory reads.
func (s *Store) CareerSharedRecent(ctx context.Context, session Session, limit uint32) (CareerPage, error) {
	p, e := s.SharedRecent(ctx, session, limit)
	if e != nil {
		return CareerPage{}, e
	}
	return s.careerInventoryPage(ctx, session, p)
}

func (s *Store) careerInventoryPage(ctx context.Context, session Session, p Page) (CareerPage, error) {
	v, items, e := s.careerProjection(ctx, session, p.Status, p.AsOfMS, p.Items)
	if e != nil {
		return CareerPage{}, e
	}
	out := CareerPage{1, v.Activation(), v.Digest(), p.AsOfMS, p, items}
	raw, e := json.Marshal(out)
	if e != nil || len(raw) > MaxCareerPageSize {
		return CareerPage{}, ErrCapacity
	}
	return out, nil
}
func (s *Store) CareerRoster(ctx context.Context, session Session, firstSlot, limit uint32) (CareerRosterPage, error) {
	p, e := s.Roster(ctx, session, firstSlot, limit)
	if e != nil {
		return CareerRosterPage{}, e
	}
	base := make([]Item, 0, len(p.Items))
	for _, item := range p.Items {
		base = append(base, item.Item)
	}
	v, items, e := s.careerProjection(ctx, session, p.Status, p.AsOfMS, base)
	if e != nil {
		return CareerRosterPage{}, e
	}
	out := CareerRosterPage{1, v.Activation(), v.Digest(), p.AsOfMS, p, items}
	raw, e := json.Marshal(out)
	if e != nil || len(raw) > MaxCareerPageSize {
		return CareerRosterPage{}, ErrCapacity
	}
	return out, nil
}
