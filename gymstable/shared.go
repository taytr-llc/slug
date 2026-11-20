// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"context"
	"encoding/binary"
	"strings"
	"time"

	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/internal/careerstate"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/registry"
)

// SharedPinVersion2 explicitly selects a per-gym view of a shared issuer.
// Source sequences and all existing assignment/roster DTOs remain unchanged.
const SharedPinVersion2 uint32 = 2
const sharedMarkerName = "SLUG-SHARED-GYM-STABLE.json"
const maxSharedSkipSize = 1024

type sharedMetadata struct {
	Version         uint32 `json:"version"`
	AssignmentCount uint64 `json:"assignment_count"`
}

// sharedSkip records why an authenticated source position has no assignment.
// It is private audit evidence, never an inventory item or receipt authority.
type sharedSkip struct {
	Version            uint32 `json:"version"`
	Sequence           uint64 `json:"sequence"`
	GymID              string `json:"gym_id"`
	PublicKey          string `json:"public_key"`
	ProspectID         string `json:"prospect_id"`
	SourceRecordDigest string `json:"source_record_digest"`
}

// SharedStatus distinguishes local inventory size from global cursor progress.
// Status retains its original wire shape; its nested Pin2 selects shared mode.
type SharedStatus struct {
	Version         uint32 `json:"version"`
	Status          Status `json:"status"`
	AssignmentCount uint64 `json:"assignment_count"`
}

func stableMarker(p Pin, id string) (string, marker) {
	if p.Version == SharedPinVersion2 {
		return sharedMarkerName, marker{"slug-candidate-shared-stable", SharedPinVersion2, id, p}
	}
	return markerName, marker{"slug-candidate-stable", Version1, id, p}
}
func (s *Store) shared() bool { return s.meta.Pin.Version == SharedPinVersion2 }

// Shared consumers recheck the borrowed source after proof/preparation work.
// This never runs under an account lease or stable mutex. Isolated callers
// retain their established observation and cancellation ordering.
func (s *Store) sharedSourceFence(ctx context.Context) error {
	if !s.shared() {
		return nil
	}
	_, e := s.sourceInfoMode(ctx, true)
	return e
}

func CreateShared(path string, pin Pin, source *registry.ManifestStore) (*Store, error) {
	return createShared(path, pin, source, time.Now, nil)
}
func createShared(path string, pin Pin, source *registry.ManifestStore, clock func() time.Time, checkpoint func(string)) (*Store, error) {
	return createMode(path, pin, source, clock, checkpoint, SharedPinVersion2)
}
func OpenShared(path string, expected Pin, source *registry.ManifestStore) (*Store, error) {
	return openSharedCareerWithClock(path, expected, source, time.Now, nil, false)
}
func openSharedCareerWithClock(path string, expected Pin, source *registry.ManifestStore, clock func() time.Time, activation *careerstate.Activation, prepared bool) (*Store, error) {
	return openMode(path, expected, source, clock, activation, prepared, SharedPinVersion2)
}
func OpenSharedCareer(path string, expected Pin, source *registry.ManifestStore, activation careerstate.Activation) (*Store, error) {
	return openSharedCareerWithClock(path, expected, source, time.Now, &activation, false)
}
func ResumeSharedCareerBinding(path string, expected Pin, source *registry.ManifestStore, activation careerstate.Activation) (*Store, error) {
	return openSharedCareerWithClock(path, expected, source, time.Now, &activation, true)
}

func sharedSkipKey(n uint64) []byte {
	k := make([]byte, 9)
	k[0] = 's'
	binary.BigEndian.PutUint64(k[1:], n)
	return k
}
func validSharedSkip(v sharedSkip, p Pin, checkpoint uint64) bool {
	if p.Version != SharedPinVersion2 || v.Version != Version1 || v.Sequence == 0 || v.Sequence > checkpoint || checkpoint > MaxAssignments || !hashID(v.GymID) || !hashID(v.ProspectID) || !hashID(v.SourceRecordDigest) || len(v.PublicKey) != 66 || strings.ToLower(v.PublicKey) != v.PublicKey || v.GymID == p.Identity.GymID || v.PublicKey == p.Identity.GymPublicKey {
		return false
	}
	id, e := gymauth.GymID(prospect.Context{Version: prospect.Version1, Network: p.Source.Network, Genesis: p.Source.Genesis}, v.PublicKey)
	return e == nil && id == v.GymID
}

// This accepts only a record returned by the concrete verified source. Every
// foreign record is verified before it can become a durable skip witness.
func sharedRecordScope(r registry.ManifestRecord, p Pin) bool {
	if r.Version != registry.Version5 || r.Sequence == 0 || r.Sequence > MaxAssignments || r.Verified.Network != p.Source.Network || r.Verified.Genesis != p.Source.Genesis || r.Job.AuthorityID != p.Source.AuthorityID || r.Job.DescriptorID != p.Source.DescriptorID || r.StaticBinding.DescriptorID != p.Source.DescriptorID || r.Job.PublicKey != r.Authorization.PublicKey || r.Job.GymID != r.Verified.GymID {
		return false
	}
	id, e := gymauth.GymID(prospect.Context{Version: prospect.Version1, Network: p.Source.Network, Genesis: p.Source.Genesis}, r.Authorization.PublicKey)
	return e == nil && id == r.Verified.GymID
}

// SharedStatus refreshes metadata-only source health and retains its global
// highwater. It does not rehash history or confer new assignments.
func (s *Store) SharedStatus(ctx context.Context, session Session) (SharedStatus, error) {
	if e := normal(); e != nil {
		return SharedStatus{}, e
	}
	if s == nil {
		return SharedStatus{}, ErrInvalid
	}
	ctx, done, e := s.begin(ctx, session, gymaccount.View)
	if e != nil {
		return SharedStatus{}, e
	}
	defer done()
	if !s.shared() {
		return SharedStatus{}, ErrInvalid
	}
	if _, e = s.sourceInfoMode(ctx, true); e != nil {
		return SharedStatus{}, e
	}
	var out SharedStatus
	e = s.withLease(ctx, session, gymaccount.View, func(check func() error) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		if e := check(); e != nil {
			return e
		}
		if s.meta.assignmentCount > s.meta.Checkpoint {
			return ErrCorrupt
		}
		out = SharedStatus{Version1, s.statusLocked(), s.meta.assignmentCount}
		return boundedResult(out)
	})
	if e != nil {
		return SharedStatus{}, e
	}
	return out, nil
}

// OpenSharedCareerAudit opens only an established exact career-bound archive.
// It audits all existing rows without changing source highwaters or DB files.
func OpenSharedCareerAudit(path string, expected Pin, source *registry.ManifestStore, activation careerstate.Activation) (*Store, error) {
	return openModeAudit(path, expected, source, time.Now, &activation, false, SharedPinVersion2, true)
}
