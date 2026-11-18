// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/registry"
	"path/filepath"
	"reflect"
	"sort"
)

type eventRootMarker struct {
	entryRootMarker
	EventSchema  uint32 `json:"event_schema"`
	EventPhase   string `json:"event_phase"`
	EventStoreID string `json:"event_store_id"`
}

func eventPinFromRoot(m eventRootMarker) (gymevent.Pin, error) {
	p := gymevent.Pin{Version: 1, CalendarID: gymentry.CalendarID(), RulesID: gymevent.RulesID(), Participants: []gymentry.Pin{}}
	for _, x := range m.EntryInstalls {
		if x.Phase == "ready" {
			p.Participants = append(p.Participants, x.Pin)
		}
	}
	sort.Slice(p.Participants, func(i, j int) bool {
		return p.Participants[i].StablePin.Identity.GymID < p.Participants[j].StablePin.Identity.GymID
	})
	return gymevent.ValidatePin(p)
}
func (s *StableController) eventRoot() (eventRootMarker, error) {
	if s.seedEnabled {
		m, e := s.seedRoot()
		base := m.eventRootMarker
		base.Version = 4
		return base, e
	}
	var m eventRootMarker
	path := filepath.Join(filepath.Dir(filepath.Dir(s.config.DataDir)), "SLUG-MANIFEST-ACCOUNTS.json")
	if !s.eventEnabled || readManifestJSON(path, 16384, &m) != nil || m.Version != 4 || m.EventSchema != 1 {
		return m, ErrManifestInitialization
	}
	return m, validateEventRoot(m)
}
func validateEventRoot(m eventRootMarker) error {
	if m.Version != 4 || m.EventSchema != 1 {
		return ErrManifestInitialization
	}
	base := m.entryRootMarker
	base.Version = 3
	if validateEntryRoot(base) != nil {
		return ErrManifestInitialization
	}
	p, e := eventPinFromRoot(m)
	switch m.EventPhase {
	case "waiting_participants":
		if e == nil || m.EventStoreID != "" {
			return ErrManifestInitialization
		}
		ready := 0
		for _, x := range m.EntryInstalls {
			if x.Phase == "ready" {
				ready++
			}
		}
		if ready >= 2 {
			return ErrManifestInitialization
		}
	case "prepared", "ready":
		if e != nil {
			return e
		}
		id, _ := gymevent.ID(p)
		if id != m.EventStoreID {
			return ErrManifestInitialization
		}
	default:
		return ErrManifestInitialization
	}
	return nil
}
func NewEventManifestAccount(c Config, a StableAccountConfig) (*StableController, error) {
	return newEventManifestAccount(c, a, nil)
}
func newEventManifestAccount(c Config, a StableAccountConfig, f *eventStoreFactories) (*StableController, error) {
	s, e := NewEntryManifestAccount(c, a)
	if e != nil {
		return nil, e
	}
	s.eventEnabled = true
	s.eventFactories = f
	return s, nil
}
func (s *StableController) AttachEvents(ctx context.Context, x *gymevent.Store) error {
	m, e := s.eventRoot()
	if e != nil {
		return e
	}
	id, e := gymaccount.FixtureIdentity(s.account.Fixture)
	if e != nil {
		return e
	}
	var pin gymevent.Pin
	var created int64
	if x == nil {
		if m.EventPhase != "waiting_participants" {
			return ErrManifestInitialization
		}
	} else {
		if m.EventPhase != "ready" {
			return ErrManifestInitialization
		}
		pin, e = eventPinFromRoot(m)
		if e != nil {
			return e
		}
		info, e := x.Info(ctx)
		if e != nil {
			return e
		}
		if info.StoreID != m.EventStoreID || !reflect.DeepEqual(info.Pin, pin) {
			return ErrManifestInitialization
		}
		created = info.CreatedAtMS
	}
	return s.shortView(ctx, s.view.ViewID, func() error {
		s.stableMu.Lock()
		defer s.stableMu.Unlock()
		if s.eventAttached || s.stableClosed || !s.installed {
			return ErrManifestInitialization
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if !stableCacheFits(s.cache, s.receiptSlot, s.previewBytes+s.entryBytes+eventMetadataBytes+s.seedBytes+s.cardBytes) {
			return gymevent.ErrCapacity
		}
		s.eventStore = x
		s.eventPin = pin
		s.eventCreated = created
		s.eventID = m.EventStoreID
		s.eventParticipant = id.GymID
		s.eventAttached = true
		s.eventBytes = eventMetadataBytes
		return nil
	})
}

// Immutable, private dependencies for the explicitly tagged fixture constructor.
// Nil uses the existing production constructors. No clock setter or public Config
// bypass is present, and the same factories govern every later handle reopen.
type eventStoreFactories struct {
	createStable func(string, gymstable.Pin, *registry.ManifestStore) (*gymstable.Store, error)
	openStable   func(string, gymstable.Pin, *registry.ManifestStore) (*gymstable.Store, error)
	createEntry  func(context.Context, string, gymentry.Pin, *gymstable.Store) (*gymentry.Store, error)
	openEntry    func(context.Context, string, gymentry.Pin, *gymstable.Store) (*gymentry.Store, error)
}

func (s *StableController) createStableStore(p string, pin gymstable.Pin, source *registry.ManifestStore) (*gymstable.Store, error) {
	if s.eventFactories != nil {
		return s.eventFactories.createStable(p, pin, source)
	}
	return gymstable.Create(p, pin, source)
}
func (s *StableController) openStableStore(p string, pin gymstable.Pin, source *registry.ManifestStore) (*gymstable.Store, error) {
	if s.eventFactories != nil {
		return s.eventFactories.openStable(p, pin, source)
	}
	return gymstable.Open(p, pin, source)
}
func (s *StableController) createEntryStore(ctx context.Context, p string, pin gymentry.Pin, source *gymstable.Store) (*gymentry.Store, error) {
	if s.eventFactories != nil {
		return s.eventFactories.createEntry(ctx, p, pin, source)
	}
	return gymentry.Create(ctx, p, pin, source)
}
func (s *StableController) openEntryStore(ctx context.Context, p string, pin gymentry.Pin, source *gymstable.Store) (*gymentry.Store, error) {
	if s.eventFactories != nil {
		return s.eventFactories.openEntry(ctx, p, pin, source)
	}
	return gymentry.Open(ctx, p, pin, source)
}
