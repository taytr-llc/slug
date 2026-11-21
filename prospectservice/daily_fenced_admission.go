// Copyright (c) 2026 The SLUG developers. ISC license.
package prospectservice

import (
	"errors"

	"github.com/taytr-llc/slug/internal/mininglease"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/registry"
)

// NewPausedDailyFencedManifestAdmissionServer explicitly selects authoritative
// expiry preflight for retained DAILY work. It borrows the same original issuer,
// node and session owners as the old constructor and exposes no issuance route.
// Intake still starts paused; DrainOperatorIntake remains permanent. The DAILY
// operator independently decides when its original-job admission can be enabled.
func NewPausedDailyFencedManifestAdmissionServer(config ManifestServerConfig, policy prospectnode.Policy, node ManifestNode, store *registry.ManifestStore, sessions SessionAuthorizer) (*ManifestServer, error) {
	s, err := NewPausedFencedManifestAdmissionServer(config, policy, node, store, sessions)
	if err != nil {
		return nil, err
	}
	s.dailyAdmission = true
	return s, nil
}

func (server *ManifestServer) preflightDailyAdmission(lease *mininglease.Lease, prepared *registry.PreparedManifestAdmission) *responseError {
	var original error
	err := lease.Do(func(check func() error) error {
		original = server.store.PreflightManifestAdmission(prepared, check)
		// Do must observe the outer authorizer's result independently of the
		// helper result. Its legacy operation-error precedence can otherwise
		// discard a mixed outer failure behind an expired callback error.
		return nil
	})
	// Preserve the source of the terminal decision. A session/wrapper error
	// that merely contains ErrJobExpired cannot authorize clearing retained work.
	if err != nil {
		return server.dailyPreflightFailure(errors.Join(err, original))
	}
	// Reauthorize the original session after the short operation lease, even
	// when only the clock changed. No terminal disclosure or node work follows
	// a lost/replaced/canceled original session.
	if err := lease.Check(); err != nil {
		return server.dailyPreflightFailure(errors.Join(err, original))
	}
	if original == registry.ErrJobExpired {
		return apiError("expired", 409)
	}
	if original != nil {
		return server.dailyPreflightFailure(original)
	}
	return nil
}

func (server *ManifestServer) dailyPreflightFailure(err error) *responseError {
	if errors.Is(err, registry.ErrPoisoned) {
		server.poisoned.Store(true)
		return &responseError{"outcome_unknown", 503, true}
	}
	if errors.Is(err, mininglease.ErrOutcomeUnknown) {
		return &responseError{"outcome_unknown", 503, true}
	}
	if errors.Is(err, mininglease.ErrUnauthorized) {
		return apiError("unauthorized", 401)
	}
	if errors.Is(err, registry.ErrJobExpired) {
		// Only preflightDailyAdmission's exact original result can emit expired.
		return apiError("unavailable", 503)
	}
	return fencedAdmissionFailure(server, err)
}
