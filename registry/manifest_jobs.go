// Copyright (c) 2026 The SLUG developers. ISC license.
package registry

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
)

func (s *ManifestStore) clockMillis() (int64, error) {
	now := time.Now
	if s.now != nil {
		now = s.now
	}
	value := now().UnixMilli()
	if value <= 0 || value < s.clockSeen || value < s.meta.LastClockMS {
		return 0, ErrClock
	}
	s.clockSeen = value
	return value, nil
}

func (s *ManifestStore) persistClock(value int64) error {
	if value <= s.meta.LastClockMS {
		return nil
	}
	next := s.meta
	next.LastClockMS = value
	if err := s.commit(new(leveldb.Batch), next); err != nil {
		return err
	}
	s.mark("clock")
	return nil
}

func (s *ManifestStore) decodeJob(raw []byte, id string) (prospectjob.ManifestJob, error) {
	var job prospectjob.ManifestJob
	if err := decodeManifestJSON(raw, prospectjob.MaxManifestJobSize, 2, &job); err != nil {
		return prospectjob.ManifestJob{}, err
	}
	if err := prospectjob.ValidateManifest(job); err != nil {
		return prospectjob.ManifestJob{}, fmt.Errorf("%w: %w", ErrCorrupt, err)
	}
	if !manifestCanonicalHash(id) || job.ID != id || job.AuthorityID != s.meta.AuthorityID || job.DescriptorID != s.meta.DescriptorID || job.Context.Network != s.meta.Network || job.Context.Genesis != s.meta.Genesis || job.IssuedAtMS > s.meta.LastClockMS {
		return prospectjob.ManifestJob{}, fmt.Errorf("%w: manifest job pin or clock mismatch", ErrCorrupt)
	}
	return job, nil
}

func (s *ManifestStore) loadJob(id string) (prospectjob.ManifestJob, error) {
	id, err := normalizeHash(id)
	if err != nil {
		return prospectjob.ManifestJob{}, err
	}
	raw, err := s.db.Get(jobKey(id), nil)
	if errors.Is(err, leveldb.ErrNotFound) {
		return prospectjob.ManifestJob{}, ErrJobNotFound
	}
	if err != nil {
		return prospectjob.ManifestJob{}, err
	}
	return s.decodeJob(raw, id)
}

// GetManifestJob inspects an immutable issued job without a clock or work hash.
func (s *ManifestStore) GetManifestJob(id string) (prospectjob.ManifestJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.state(false); err != nil {
		return prospectjob.ManifestJob{}, err
	}
	return s.loadJob(id)
}

// IssueManifestJob is a trusted authority operation. Context/policy and gym
// access must come from its caller's trusted resolver and mandatory final guard.
// Issuance time, challenge, authority and descriptor are never caller-selected.
func (s *ManifestStore) IssueManifestJob(context prospect.Context, publicKey string, check func() error) (prospectjob.ManifestJob, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.issue != nil {
		return prospectjob.ManifestJob{}, ErrManifestAuthority
	}
	if err := s.state(true); err != nil {
		return prospectjob.ManifestJob{}, err
	}
	if check == nil {
		return prospectjob.ManifestJob{}, errors.New("manifest issuance requires a final context check")
	}
	genesis, err := chainIdentity(context.Network, context.Genesis)
	if err != nil || context.Network != s.meta.Network || genesis != s.meta.Genesis || context.DNAVersion != s.meta.Descriptor.DNAVersion {
		return prospectjob.ManifestJob{}, ErrManifestAuthority
	}
	if _, err := validateManifestPin(s.meta.pin()); err != nil {
		return prospectjob.ManifestJob{}, err
	}
	gym, err := gymauth.GymID(context, publicKey)
	if err != nil {
		return prospectjob.ManifestJob{}, err
	}
	if _, err := prospect.CommitmentScript(context, gym); err != nil {
		return prospectjob.ManifestJob{}, err
	}
	if s.meta.JobCount == math.MaxUint64 {
		return prospectjob.ManifestJob{}, errors.New("manifest job counter exhausted")
	}
	if err := check(); err != nil {
		return prospectjob.ManifestJob{}, fmt.Errorf("manifest issuance context check: %w", err)
	}
	random := s.random
	if random == nil {
		random = rand.Reader
	}
	var challenge [32]byte
	if _, err := io.ReadFull(random, challenge[:]); err != nil {
		return prospectjob.ManifestJob{}, errors.New("manifest challenge generation failed")
	}
	issued, err := s.clockMillis()
	if err != nil {
		return prospectjob.ManifestJob{}, err
	}
	if issued > math.MaxInt64-prospectjob.LifetimeMS {
		if err := s.persistClock(issued); err != nil {
			return prospectjob.ManifestJob{}, err
		}
		return prospectjob.ManifestJob{}, ErrClock
	}
	job, err := prospectjob.NewManifest(context, s.meta.AuthorityID, s.meta.DescriptorID, publicKey, hex.EncodeToString(challenge[:]), issued)
	if err != nil {
		return prospectjob.ManifestJob{}, err
	}
	exists, err := s.db.Has(jobKey(job.ID), nil)
	if err != nil {
		return prospectjob.ManifestJob{}, err
	}
	if exists {
		return prospectjob.ManifestJob{}, errors.New("manifest job ID collision")
	}
	raw, err := prospectjob.MarshalManifestJob(job)
	if err != nil {
		return prospectjob.ManifestJob{}, err
	}
	next := s.meta
	next.JobCount++
	next.LastClockMS = issued
	batch := new(leveldb.Batch)
	batch.Put(jobKey(job.ID), raw)
	s.mark("before-issue")
	if err := s.commit(batch, next); err != nil {
		return prospectjob.ManifestJob{}, err
	}
	s.mark("issued")
	return job, nil
}
