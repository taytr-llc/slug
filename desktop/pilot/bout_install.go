// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymseed"
	"path/filepath"
	"reflect"
)

type boutRootMarker struct {
	cardRootMarker
	BoutSchema  uint32 `json:"bout_schema"`
	BoutPhase   string `json:"bout_phase"`
	BoutStoreID string `json:"bout_store_id"`
}

func (s *StableController) boutRoot() (boutRootMarker, error) {
	var m boutRootMarker
	path := filepath.Join(filepath.Dir(filepath.Dir(s.config.DataDir)), "SLUG-MANIFEST-ACCOUNTS.json")
	if !s.boutEnabled || readManifestJSON(path, 16384, &m) != nil || m.Version != 7 || m.BoutSchema != 1 {
		return m, ErrManifestInitialization
	}
	base := m.cardRootMarker
	base.Version = 6
	if validateCardRoot(base) != nil {
		return m, ErrManifestInitialization
	}
	switch m.BoutPhase {
	case "waiting_source":
		if m.BoutStoreID != "" {
			return m, ErrManifestInitialization
		}
	case "prepared", "ready":
		if m.CardPhase != "ready" {
			return m, ErrManifestInitialization
		}
		ev, e := eventPinFromRoot(base.seedRootMarker.eventRootMarker)
		if e != nil {
			return m, e
		}
		sp, e := gymseed.CandidatePin(ev)
		if e != nil {
			return m, e
		}
		cp, e := gymcard.CandidatePin(sp)
		if e != nil {
			return m, e
		}
		p, e := gymbout.CandidatePin(cp)
		if e != nil {
			return m, e
		}
		id, e := gymbout.ID(p)
		if e != nil || id != m.BoutStoreID {
			return m, ErrManifestInitialization
		}
	default:
		return m, ErrManifestInitialization
	}
	return m, nil
}
func NewBoutManifestAccount(c Config, a StableAccountConfig) (*StableController, error) {
	s, e := NewCardManifestAccount(c, a)
	if e == nil {
		s.boutEnabled = true
	}
	return s, e
}
func (s *StableController) AttachBouts(ctx context.Context, x *gymbout.Store) error {
	m, e := s.boutRoot()
	if e != nil {
		return e
	}
	var pin gymbout.Pin
	var created, sourceCreated int64
	if x == nil {
		if m.BoutPhase != "waiting_source" {
			return ErrManifestInitialization
		}
	} else {
		if m.BoutPhase != "ready" || s.cardStore == nil {
			return ErrManifestInitialization
		}
		info, e := x.Info(ctx)
		if e != nil {
			return e
		}
		source, e := s.cardStore.Info(ctx)
		if e != nil {
			return e
		}
		pin, e = gymbout.CandidatePin(source.Pin)
		if e != nil {
			return e
		}
		if info.StoreID != m.BoutStoreID || !reflect.DeepEqual(info.Pin, pin) || info.SourceCreatedAtMS != source.CreatedAtMS || info.CreatedAtMS < source.CreatedAtMS {
			return ErrManifestInitialization
		}
		created, sourceCreated = info.CreatedAtMS, info.SourceCreatedAtMS
	}
	return s.shortView(ctx, s.view.ViewID, func() error {
		s.stableMu.Lock()
		defer s.stableMu.Unlock()
		if s.boutAttached || !s.cardAttached || s.stableClosed || !s.installed {
			return ErrManifestInitialization
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if !stableCacheFits(s.cache, s.receiptSlot, s.previewBytes+s.entryBytes+s.eventBytes+s.seedBytes+s.cardBytes+MaxBoutMetadataSize) {
			return gymbout.ErrCapacity
		}
		s.boutStore = x
		s.boutPin = pin
		s.boutID = m.BoutStoreID
		s.boutCreated = created
		s.boutSourceCreated = sourceCreated
		s.boutAttached = true
		s.boutBytes = MaxBoutMetadataSize
		return nil
	})
}
