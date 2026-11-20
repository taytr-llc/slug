// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymresult"
	"github.com/taytr-llc/slug/gymseed"
	"path/filepath"
	"reflect"
)

type resultRootMarker struct {
	boutRootMarker
	ResultSchema  uint32 `json:"result_schema"`
	ResultPhase   string `json:"result_phase"`
	ResultStoreID string `json:"result_store_id"`
}

func (s *StableController) resultRoot() (resultRootMarker, error) {
	var m resultRootMarker
	path := filepath.Join(filepath.Dir(filepath.Dir(s.config.DataDir)), "SLUG-MANIFEST-ACCOUNTS.json")
	if s.careerEnabled {
		var cm careerRootMarker
		if readManifestJSON(path, 16384, &cm) == nil && cm.Version == 9 {
			if validateCareerRoot(cm) != nil {
				return m, ErrManifestInitialization
			}
			m = cm.resultRootMarker
			m.Version = 8
		} else if readManifestJSON(path, 16384, &m) != nil {
			return m, ErrManifestInitialization
		}
	} else if readManifestJSON(path, 16384, &m) != nil {
		return m, ErrManifestInitialization
	}
	if !s.resultEnabled || m.Version != 8 || m.ResultSchema != 1 {
		return m, ErrManifestInitialization
	}
	base := m.boutRootMarker
	base.Version = 7
	if validateBoutRoot(base) != nil {
		return m, ErrManifestInitialization
	}
	switch m.ResultPhase {
	case "waiting_source":
		if m.ResultStoreID != "" {
			return m, ErrManifestInitialization
		}
	case "prepared", "ready":
		if m.BoutPhase != "ready" {
			return m, ErrManifestInitialization
		}
		ev, e := eventPinFromRoot(base.cardRootMarker.seedRootMarker.eventRootMarker)
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
		bp, e := gymbout.CandidatePin(cp)
		if e != nil {
			return m, e
		}
		p, e := gymresult.CandidatePin(bp)
		if e != nil {
			return m, e
		}
		id, e := gymresult.ID(p)
		if e != nil || id != m.ResultStoreID {
			return m, ErrManifestInitialization
		}
	default:
		return m, ErrManifestInitialization
	}
	return m, nil
}
func NewResultManifestAccount(c Config, a StableAccountConfig) (*StableController, error) {
	s, e := NewBoutManifestAccount(c, a)
	if e == nil {
		s.resultEnabled = true
	}
	return s, e
}
func (s *StableController) AttachResults(ctx context.Context, x *gymresult.Store) error {
	m, e := s.resultRoot()
	if e != nil {
		return e
	}
	var pin gymresult.Pin
	var created, sourceCreated int64
	if x == nil {
		if m.ResultPhase != "waiting_source" {
			return ErrManifestInitialization
		}
	} else {
		if m.ResultPhase != "ready" || s.boutStore == nil {
			return ErrManifestInitialization
		}
		info, e := x.Info(ctx)
		if e != nil {
			return e
		}
		source, e := s.boutStore.Info(ctx)
		if e != nil {
			return e
		}
		pin, e = gymresult.CandidatePin(source.Pin)
		if e != nil {
			return e
		}
		if info.StoreID != m.ResultStoreID || !reflect.DeepEqual(info.Pin, pin) || info.SourceCreatedAtMS != source.CreatedAtMS || info.CreatedAtMS < source.CreatedAtMS {
			return ErrManifestInitialization
		}
		created, sourceCreated = info.CreatedAtMS, info.SourceCreatedAtMS
	}
	return s.shortView(ctx, s.view.ViewID, func() error {
		s.stableMu.Lock()
		defer s.stableMu.Unlock()
		if s.resultAttached || !s.boutAttached || s.stableClosed || !s.installed {
			return ErrManifestInitialization
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if !stableCacheFits(s.cache, s.receiptSlot, s.historyBytes+s.careerBytes+s.previewBytes+s.entryBytes+s.eventBytes+s.seedBytes+s.cardBytes+s.boutBytes+resultRetainedMetadataBytes) {
			return gymresult.ErrCapacity
		}
		s.resultStore = x
		s.resultPin = pin
		s.resultID = m.ResultStoreID
		s.resultCreated = created
		s.resultSourceCreated = sourceCreated
		s.resultAttached = true
		s.resultBytes = resultRetainedMetadataBytes
		return nil
	})
}
