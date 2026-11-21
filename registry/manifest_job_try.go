// Copyright (c) 2026 The SLUG developers. ISC license.
package registry

import (
	"context"
	"github.com/taytr-llc/slug/prospectjob"
)

// TryGetManifestJob reads the original immutable job without waiting behind
// another registry operation. It adds no clock, issuance or admission authority.
// ErrBusy makes no change; callers retry only outside any account lease.
func (s *ManifestStore) TryGetManifestJob(ctx context.Context, id string) (prospectjob.ManifestJob, error) {
	if ctx == nil {
		return prospectjob.ManifestJob{}, ErrManifestAuthority
	}
	if err := ctx.Err(); err != nil {
		return prospectjob.ManifestJob{}, err
	}
	if err := manifestProduction(); err != nil {
		return prospectjob.ManifestJob{}, err
	}
	if s == nil {
		return prospectjob.ManifestJob{}, ErrClosed
	}
	if !s.mu.TryLock() {
		return prospectjob.ManifestJob{}, ErrBusy
	}
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return prospectjob.ManifestJob{}, err
	}
	if err := s.state(false); err != nil {
		return prospectjob.ManifestJob{}, err
	}
	job, err := s.loadJob(id)
	if err != nil {
		return prospectjob.ManifestJob{}, err
	}
	if err := ctx.Err(); err != nil {
		return prospectjob.ManifestJob{}, err
	}
	return job, nil
}
