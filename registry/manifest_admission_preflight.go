// Copyright (c) 2026 The SLUG developers. ISC license.
package registry

import (
	"errors"
	"fmt"
	"math"

	"github.com/syndtr/goleveldb/leveldb"
)

// PreflightManifestAdmission observes the original issuer's expiry before an
// explicitly selected service applies its intake hold. It grants no reservation
// or later admission authority: CommitManifestAdmission still performs every
// original check, including its final expiry sample.
//
// Like the staged commits, this method never waits for the registry gate. check
// must recheck an already-held original account lease, without external work or
// reacquiring a lease. Only the exact ErrJobExpired returned by this method's
// clock branch is authoritative expiry; errors supplied by check are wrapped.
func (s *ManifestStore) PreflightManifestAdmission(prepared *PreparedManifestAdmission, check func() error) error {
	if s == nil {
		return ErrClosed
	}
	// Reuse the original historical validation rather than maintaining a second
	// copy of its record/index/session rules. This branch cannot create a record
	// or sample a clock. Selected services normally bypass preflight for history.
	if prepared != nil && prepared.state != nil && prepared.state.historical {
		_, _, err := s.CommitManifestAdmission(prepared, Eligibility{}, check)
		return err
	}
	if !s.mu.TryLock() {
		return ErrBusy
	}
	defer s.mu.Unlock()
	if err := s.state(true); err != nil {
		return err
	}
	if prepared == nil || prepared.state == nil || prepared.state.store != s {
		return ErrManifestAuthority
	}
	p := prepared.state
	if p.pin != s.meta.pin() || p.descriptor != s.meta.Descriptor {
		return ErrManifestAuthority
	}
	if check == nil {
		return errors.New("manifest admission preflight requires an original-session recheck")
	}
	job, err := s.loadJob(p.record.Job.ID)
	if err != nil {
		return err
	}
	if job != p.record.Job {
		return ErrConflict
	}
	_, err = s.db.Get(recordKey(p.record.Verified.ProspectID), nil)
	if err == nil {
		// A concurrent insert requires a fresh preparation and the original
		// historical path; an old nonhistorical token cannot decide expiry.
		return ErrConflict
	}
	if !errors.Is(err, leveldb.ErrNotFound) {
		return err
	}
	if s.meta.LastSequence == math.MaxUint64 {
		return errors.New("manifest receipt sequence exhausted")
	}
	if err := check(); err != nil {
		return fmt.Errorf("manifest admission preflight session check: %w", err)
	}
	observed, err := s.clockMillis()
	if err != nil {
		return err
	}
	// A clock-only write retains the original logical-byte/reservation and
	// poison semantics. A full birth count is not itself a clock-write failure.
	// Never turn a refused or uncertain sync into an expired response.
	if err := s.persistClock(observed); err != nil {
		return err
	}
	if observed < job.IssuedAtMS {
		return ErrClock
	}
	if observed >= job.ExpiresAtMS {
		return ErrJobExpired
	}
	return nil
}
