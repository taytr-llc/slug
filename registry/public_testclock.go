//go:build desktopintegration && slugtestclock && !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package registry

import (
	"context"
	"time"
)

// UseAuthorityClockForTest offsets the retained fixture authority, including
// its shared job/admission clock. The harness must stop all jobs/admissions
// before installation and forbid further issuance/admission in that phase.
// It does not forge a source prefix, publication, signature or boundary.
func (p *ManifestPublication) UseAuthorityClockForTest(ctx context.Context, expected PublicManifestPin, clock func() time.Time) error {
	if p == nil || clock == nil || p.pin != expected {
		return ErrPublicOwnership
	}
	ctx, done, e := p.acquire(ctx)
	if e != nil {
		return e
	}
	defer done()
	s := p.store
	if !s.mu.TryLock() {
		return ErrBusy
	}
	defer s.mu.Unlock()
	if e = s.state(true); e != nil {
		return e
	}
	if s.public == nil || s.public.pin != expected {
		return ErrPublicOwnership
	}
	m := s.public.meta
	if s.public.boundary || m.PendingPublicationID != "" || m.AcceptedSequence != m.SignedAdmissionSequence {
		return ErrBusy
	}
	n := clock().UnixMilli()
	if n <= 0 || n > MaxPublicTimestampMS || n < s.clockSeen || n < s.meta.LastClockMS || n < m.ClockHighWaterMS {
		return ErrClock
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	s.now = clock
	return nil
}
