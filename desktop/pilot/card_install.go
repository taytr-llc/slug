// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymseed"
	"path/filepath"
	"reflect"
)

type cardRootMarker struct {
	seedRootMarker
	CardSchema  uint32 `json:"card_schema"`
	CardPhase   string `json:"card_phase"`
	CardStoreID string `json:"card_store_id"`
}

func (s *StableController) cardRoot() (cardRootMarker, error) {
	if s.boutEnabled {
		m, e := s.boutRoot()
		b := m.cardRootMarker
		b.Version = 6
		return b, e
	}
	var m cardRootMarker
	path := filepath.Join(filepath.Dir(filepath.Dir(s.config.DataDir)), "SLUG-MANIFEST-ACCOUNTS.json")
	if !s.cardEnabled || readManifestJSON(path, 16384, &m) != nil || m.Version != 6 || m.CardSchema != 1 {
		return m, ErrManifestInitialization
	}
	if e := validateCardRoot(m); e != nil {
		return m, e
	}
	return m, nil
}
func NewCardManifestAccount(c Config, a StableAccountConfig) (*StableController, error) {
	s, e := NewSeedManifestAccount(c, a)
	if e == nil {
		s.cardEnabled = true
	}
	return s, e
}
func (s *StableController) AttachCards(ctx context.Context, x *gymcard.Store) error {
	m, e := s.cardRoot()
	if e != nil {
		return e
	}
	var pin gymcard.Pin
	var created, sourceCreated int64
	if x == nil {
		if m.CardPhase != "waiting_source" {
			return ErrManifestInitialization
		}
	} else {
		if m.CardPhase != "ready" || s.seedStore == nil {
			return ErrManifestInitialization
		}
		info, e := x.Info(ctx)
		if e != nil {
			return e
		}
		source, e := s.seedStore.Info(ctx)
		if e != nil {
			return e
		}
		pin, e = gymcard.CandidatePin(source.Pin)
		if e != nil {
			return e
		}
		if info.StoreID != m.CardStoreID || !reflect.DeepEqual(info.Pin, pin) || info.SourceCreatedAtMS != source.CreatedAtMS || info.CreatedAtMS < source.CreatedAtMS {
			return ErrManifestInitialization
		}
		created, sourceCreated = info.CreatedAtMS, info.SourceCreatedAtMS
	}
	return s.shortView(ctx, s.view.ViewID, func() error {
		s.stableMu.Lock()
		defer s.stableMu.Unlock()
		if s.cardAttached || !s.seedAttached || s.stableClosed || !s.installed {
			return ErrManifestInitialization
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if !stableCacheFits(s.cache, s.receiptSlot, s.careerBytes+s.previewBytes+s.entryBytes+s.eventBytes+s.seedBytes+s.boutBytes+MaxCardMetadataSize+s.resultBytes) {
			return gymcard.ErrCapacity
		}
		s.cardStore = x
		s.cardPin = pin
		s.cardID = m.CardStoreID
		s.cardCreated = created
		s.cardSourceCreated = sourceCreated
		s.cardAttached = true
		s.cardBytes = MaxCardMetadataSize
		return nil
	})
}

func validateCardRoot(m cardRootMarker) error {
	if m.Version != 6 || m.CardSchema != 1 {
		return ErrManifestInitialization
	}
	base := m.seedRootMarker
	base.Version = 5
	if validateSeedRoot(base) != nil {
		return ErrManifestInitialization
	}
	switch m.CardPhase {
	case "waiting_source":
		if m.CardStoreID != "" {
			return ErrManifestInitialization
		}
	case "prepared", "ready":
		if m.SeedPhase != "ready" {
			return ErrManifestInitialization
		}
		ev, e := eventPinFromRoot(base.eventRootMarker)
		if e != nil {
			return e
		}
		sp, e := gymseed.CandidatePin(ev)
		if e != nil {
			return e
		}
		p, e := gymcard.CandidatePin(sp)
		if e != nil {
			return e
		}
		id, e := gymcard.ID(p)
		if e != nil || id != m.CardStoreID {
			return ErrManifestInitialization
		}
	default:
		return ErrManifestInitialization
	}
	return nil
}
