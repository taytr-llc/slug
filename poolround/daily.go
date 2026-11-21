// Copyright (c) 2026 The SLUG developers. ISC license.
package poolround

import (
	"context"
	"errors"

	"github.com/taytr-llc/slug/poolshare"
)

const maxDailyAccepted uint64 = 16384

func validDailyLimits(l Limits) bool {
	return l.Version == Version2 && l.MaxRounds > 0 && l.MaxRounds <= maxDailyAccepted && l.MaxBytes >= statusReserve && l.MaxBytes <= 8<<30
}

func acceptedLimit(c poolshare.Config) uint64 {
	if c.Version == poolshare.Version2 {
		return maxDailyAccepted
	}
	return maxAccepted
}

// CreateDaily selects a fresh journal for the original explicitly selected V2
// source. Limits are lifetime bounds, never pages or a permission to reset work.
// Round encodings and the maximum 4096-share window remain unchanged.
func CreateDaily(ctx context.Context, path string, l Limits, source *poolshare.Pool, node Observer) (*Store, error) {
	return createProfile(ctx, path, l, source, node, true)
}

// OpenDaily authenticates the complete original V2 source and stored namespace.
// It neither observes the node nor adopts or repairs another journal version.
func OpenDaily(ctx context.Context, path string, l Limits, source *poolshare.Pool, node Observer) (*Store, error) {
	return openProfile(ctx, path, l, source, node, false, true)
}

// OpenDailyAudit is the physically read-only version of OpenDaily.
func OpenDailyAudit(ctx context.Context, path string, l Limits, source *poolshare.Pool) (*Store, error) {
	return openProfile(ctx, path, l, source, nil, true, true)
}

// DailyBinding authenticates the original selected source and lifetime bounds
// before another concrete owner attaches. Returned configuration is correlation,
// never a replacement source or permission to import caller-provided rounds.
func (s *Store) DailyBinding(ctx context.Context) (Limits, poolshare.Config, error) {
	end, err := s.begin(ctx)
	if err != nil {
		return Limits{}, poolshare.Config{}, err
	}
	defer end()
	if !validDailyLimits(s.limits) || s.sourceConfig.Version != poolshare.Version2 {
		return Limits{}, poolshare.Config{}, ErrConfig
	}
	if err := s.verifyStream(ctx); err != nil {
		return Limits{}, poolshare.Config{}, err
	}
	if err := ctx.Err(); err != nil {
		return Limits{}, poolshare.Config{}, err
	}
	return s.limits, s.sourceConfig, nil
}

// A failed close retains its only owner for explicit cleanup. Such a partial
// owner is poisoned and cannot disclose records or perform a new mutation.
func failedProfileOpen(s *Store, daily bool, cause error) (*Store, error) {
	if err := s.db.Close(); err != nil && daily {
		s.poisoned = true
		return s, errors.Join(cause, ErrPoisoned, err)
	}
	return nil, cause
}
