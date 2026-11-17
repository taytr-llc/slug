// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"github.com/taytr-llc/slug/gymseed"
	"path/filepath"
	"reflect"
)

type seedRootMarker struct {
	eventRootMarker
	SeedSchema  uint32 `json:"seed_schema"`
	SeedPhase   string `json:"seed_phase"`
	SeedStoreID string `json:"seed_store_id"`
}

func (s *StableController) seedRoot() (seedRootMarker, error) {
	var m seedRootMarker
	path := filepath.Join(filepath.Dir(filepath.Dir(s.config.DataDir)), "SLUG-MANIFEST-ACCOUNTS.json")
	if !s.seedEnabled || readManifestJSON(path, 16384, &m) != nil || m.Version != 5 || m.SeedSchema != 1 {
		return m, ErrManifestInitialization
	}
	base := m.eventRootMarker
	base.Version = 4
	if validateEventRoot(base) != nil {
		return m, ErrManifestInitialization
	}
	switch m.SeedPhase {
	case "waiting_source":
		if m.SeedStoreID != "" {
			return m, ErrManifestInitialization
		}
	case "prepared", "ready":
		if m.EventPhase != "ready" {
			return m, ErrManifestInitialization
		}
		ev, e := eventPinFromRoot(base)
		if e != nil {
			return m, e
		}
		p, e := gymseed.CandidatePin(ev)
		if e != nil {
			return m, e
		}
		id, e := gymseed.ID(p)
		if e != nil || id != m.SeedStoreID {
			return m, ErrManifestInitialization
		}
	default:
		return m, ErrManifestInitialization
	}
	return m, nil
}
func NewSeedManifestAccount(c Config, a StableAccountConfig) (*StableController, error) {
	s, e := NewEventManifestAccount(c, a)
	if e == nil {
		s.seedEnabled = true
	}
	return s, e
}
func (s *StableController) AttachSeeds(ctx context.Context, x *gymseed.Store) error {
	m, e := s.seedRoot()
	if e != nil {
		return e
	}
	var pin gymseed.Pin
	var created int64
	if x == nil {
		if m.SeedPhase != "waiting_source" {
			return ErrManifestInitialization
		}
	} else {
		if m.SeedPhase != "ready" {
			return ErrManifestInitialization
		}
		ev, e := eventPinFromRoot(m.eventRootMarker)
		if e != nil {
			return e
		}
		pin, e = gymseed.CandidatePin(ev)
		if e != nil {
			return e
		}
		info, e := x.Info(ctx)
		if e != nil {
			return e
		}
		if info.StoreID != m.SeedStoreID || !reflect.DeepEqual(info.Pin, pin) {
			return ErrManifestInitialization
		}
		created = info.CreatedAtMS
	}
	return s.shortView(ctx, s.view.ViewID, func() error {
		s.stableMu.Lock()
		defer s.stableMu.Unlock()
		if s.seedAttached || !s.eventAttached || s.stableClosed || !s.installed {
			return ErrManifestInitialization
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if !stableCacheFits(s.cache, s.receiptSlot, s.previewBytes+s.entryBytes+s.eventBytes+MaxSeedMetadataSize) {
			return gymseed.ErrCapacity
		}
		s.seedStore = x
		s.seedPin = pin
		s.seedCreated = created
		s.seedID = m.SeedStoreID
		s.seedAttached = true
		s.seedBytes = MaxSeedMetadataSize
		return nil
	})
}
