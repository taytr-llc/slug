// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"time"

	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/internal/careerstate"
	"github.com/taytr-llc/slug/registry"
)

const careerMetadataTimeout = 500 * time.Millisecond

// Includes the opaque public composite and its bounded private metadata.
const MaxCareerCacheSize = gymstable.MaxCareerPageSize + 8192

// ConfigureCareer is parent-only wiring before the child is published.
func (s *StableController) ConfigureCareer(r *careerstate.Reader, open func(string, gymstable.Pin, *registry.ManifestStore, careerstate.Activation) (*gymstable.Store, error)) error {
	if s == nil || open == nil {
		return careerstate.ErrInvalid
	}
	s.careerEnabled = true
	s.careerReader = r
	s.careerOpenStable = open
	return nil
}

// BindCareer attaches the already-open installing child's concrete Stable.
func (s *StableController) BindCareer(ctx context.Context, r *careerstate.Reader, open func(string, gymstable.Pin, *registry.ManifestStore, careerstate.Activation) (*gymstable.Store, error)) error {
	select {
	case s.laneGate <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-s.laneGate }()
	if s.stableStore == nil {
		return careerstate.ErrInactive
	}
	if e := s.stableStore.BindCareer(ctx, r); e != nil {
		return e
	}
	return s.ConfigureCareer(r, open)
}
func (s *StableController) careerRead(ctx context.Context, v, jobID string, roster bool, after uint64, first, limit uint32, fn func(context.Context, *gymstable.Store, *gymstable.CareerRead) error) error {
	if ctx == nil || !canonicalManifestID(jobID) {
		return careerstate.ErrInvalid
	}
	ctx, cancel := context.WithTimeout(ctx, careerMetadataTimeout)
	defer cancel()
	if e := s.viewWithBudget(ctx, v, careerMetadataTimeout, func() error { return nil }); e != nil {
		return e
	}
	select {
	case s.laneGate <- struct{}{}:
	default:
		return gymstable.ErrBusy
	}
	defer func() { <-s.laneGate }()
	s.stableMu.Lock()
	valid := s.careerEnabled && s.installed && !s.stableClosed && !s.stopping
	store := s.stableStore
	r := s.careerReader
	handle := s.careerHandle
	selected := s.careerJobID == jobID && s.careerView == s.viewPin() && s.careerRoster == roster && s.careerAfter == after && s.careerFirst == first && s.careerLimit == limit && s.last != nil && s.last.JobID == jobID && s.last.State == "done"
	s.stableMu.Unlock()
	if !valid || store == nil || r == nil {
		return careerstate.ErrInactive
	}
	if !selected || handle == nil {
		return gymstable.ErrNotFound
	}
	if e := fn(ctx, store, handle); e != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) && !errors.Is(e, gymstable.ErrOutcomeUnknown) {
			return gymstable.ErrBusy
		}
		return e
	}
	if e := s.viewWithBudget(ctx, v, careerMetadataTimeout, func() error {
		s.stableMu.Lock()
		defer s.stableMu.Unlock()
		if s.stableClosed || s.stopping || s.careerJobID != jobID || s.careerHandle != handle || s.careerView != s.viewPin() {
			return gymstable.ErrStaleView
		}
		return nil
	}); e != nil {
		return gymstable.ErrOutcomeUnknown
	}
	return nil
}
func (s *StableController) CareerInventory(ctx context.Context, v, jobID string, after uint64, limit uint32) (gymstable.CareerPage, error) {
	var out gymstable.CareerPage
	e := s.careerRead(ctx, v, jobID, false, after, 0, limit, func(ctx context.Context, x *gymstable.Store, handle *gymstable.CareerRead) error {
		var e error
		out, e = x.ReadCareerInventory(ctx, s.session(), handle)
		return e
	})
	if e != nil {
		return gymstable.CareerPage{}, e
	}
	return out, nil
}
func (s *StableController) CareerRoster(ctx context.Context, v, jobID string, first, limit uint32) (gymstable.CareerRosterPage, error) {
	var out gymstable.CareerRosterPage
	e := s.careerRead(ctx, v, jobID, true, 0, first, limit, func(ctx context.Context, x *gymstable.Store, handle *gymstable.CareerRead) error {
		var e error
		out, e = x.ReadCareerRoster(ctx, s.session(), handle)
		return e
	})
	if e != nil {
		return gymstable.CareerRosterPage{}, e
	}
	return out, nil
}
func (s *StableController) careerFence(ctx context.Context, date string) error {
	if !s.careerEnabled {
		return nil
	}
	if s.careerReader == nil || s.resultStore == nil || s.entryStore == nil {
		return careerstate.ErrInactive
	}
	return s.resultStore.FenceCareer(ctx, s.entryStore, s.session(), date)
}

func (s *StableController) CareerView(v string) (ManifestViewPin, error) {
	s.stableMu.Lock()
	defer s.stableMu.Unlock()
	if !s.careerEnabled || s.stableClosed || s.view.ViewID != v {
		return ManifestViewPin{}, gymstable.ErrStaleView
	}
	return s.viewPin(), nil
}

// The Stable mutex serializes selection; the Manifest mutex owns cache charging.
func (s *StableController) clearCareerReadLocked() {
	s.careerHandle = nil
	s.careerJobID = ""
	s.careerView = ManifestViewPin{}
	s.mu.Lock()
	s.careerBytes = 0
	s.mu.Unlock()
}
func (s *StableController) reserveCareerReadLocked() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	old := s.careerBytes
	if !stableCacheFits(s.cache, s.receiptSlot, s.historyBytes+MaxCareerCacheSize+s.previewBytes+s.entryBytes+s.eventBytes+s.seedBytes+s.cardBytes+s.boutBytes+s.resultBytes) {
		return old, gymstable.ErrCapacity
	}
	s.careerBytes = MaxCareerCacheSize
	return old, nil
}
