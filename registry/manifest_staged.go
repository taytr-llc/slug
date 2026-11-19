// Copyright (c) 2026 The SLUG developers. ISC license.
package registry

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
)

// ErrBusy means a staged commit did not acquire the registry gate and made no
// change. Release any outer account lease before retrying or preparing again.
var ErrBusy = errors.New("manifest registry busy")

// PreparedManifestIssue is an opaque, store-bound preparation. Copying this
// handle does not permit a second issuance. It is neither a job nor issuance
// authority; only CommitManifestIssue can assign and persist its birth time.
type PreparedManifestIssue struct{ state *preparedManifestIssue }

type preparedManifestIssue struct {
	store      *ManifestStore
	pin        ManifestAuthorityPin
	descriptor fighterdescriptor.Descriptor
	context    prospect.Context
	publicKey  string
	challenge  string
	committed  bool // accessed only under store.mu, shared by copied handles
}

// PreparedManifestAdmission retains a complete local verification result.
// It is bound to this open store, not transferable across Close/Open, and no
// public constructor accepts a caller-supplied Verified or stored receipt.
type PreparedManifestAdmission struct{ state *preparedManifestAdmission }

type preparedManifestAdmission struct {
	store       *ManifestStore
	pin         ManifestAuthorityPin
	descriptor  fighterdescriptor.Descriptor
	record      ManifestRecord
	historical  bool
	originalRaw []byte
}

func (p *PreparedManifestAdmission) Context() prospect.Context {
	if p == nil || p.state == nil {
		return prospect.Context{}
	}
	return p.state.record.Context
}

// Proof returns owned Merkle siblings, including an owned nonnil empty branch.
func (p *PreparedManifestAdmission) Proof() prospect.Proof {
	if p == nil || p.state == nil {
		return prospect.Proof{}
	}
	return ownedManifestProof(p.state.record.Proof)
}

func (p *PreparedManifestAdmission) Verified() prospect.Verified {
	if p == nil || p.state == nil {
		return prospect.Verified{}
	}
	return p.state.record.Verified
}

func (p *PreparedManifestAdmission) Historical() bool {
	return p != nil && p.state != nil && p.state.historical
}

func ownedManifestProof(p prospect.Proof) prospect.Proof {
	p.MerkleBranch = append([]string{}, p.MerkleBranch...)
	return p
}

func ownedManifestRecord(r ManifestRecord) ManifestRecord {
	r.Proof = ownedManifestProof(r.Proof)
	return r
}

// PrepareManifestIssue performs validation and entropy acquisition without an
// account lease. It neither samples the clock nor writes registry state. The
// caller must resolve and recheck trusted context before the final commit.
func (s *ManifestStore) PrepareManifestIssue(context prospect.Context, publicKey string) (*PreparedManifestIssue, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.state(true); err != nil {
		return nil, err
	}
	genesis, err := chainIdentity(context.Network, context.Genesis)
	if err != nil || context.Network != s.meta.Network || genesis != s.meta.Genesis || context.DNAVersion != s.meta.Descriptor.DNAVersion {
		return nil, ErrManifestAuthority
	}
	if _, err := validateManifestPin(s.meta.pin()); err != nil {
		return nil, err
	}
	if s.meta.JobCount == math.MaxUint64 {
		return nil, errors.New("manifest job counter exhausted")
	}
	// This temporary time is only for complete input normalization. It is
	// discarded; no prepared object is an issued or serializable public job.
	validated, err := prospectjob.NewManifest(context, s.meta.AuthorityID, s.meta.DescriptorID, publicKey, stringsZeroChallenge, 1)
	if err != nil {
		return nil, err
	}
	random := s.random
	if random == nil {
		random = rand.Reader
	}
	var challenge [32]byte
	if _, err := io.ReadFull(random, challenge[:]); err != nil {
		return nil, errors.New("manifest challenge generation failed")
	}
	return &PreparedManifestIssue{state: &preparedManifestIssue{
		store: s, pin: s.meta.pin(), descriptor: s.meta.Descriptor,
		context: validated.Context, publicKey: validated.PublicKey, challenge: hex.EncodeToString(challenge[:]),
	}}, nil
}

const stringsZeroChallenge = "0000000000000000000000000000000000000000000000000000000000000000"

// CommitManifestIssue never waits for the registry gate or obtains entropy.
// The caller holds its original account lease. check must only recheck that
// already-held lease; it must not acquire a lease, reenter the registry, or do
// external I/O. ErrBusy is safe to retry after releasing the outer lease.
func (s *ManifestStore) CommitManifestIssue(prepared *PreparedManifestIssue, check func() error) (prospectjob.ManifestJob, error) {
	if !s.mu.TryLock() {
		return prospectjob.ManifestJob{}, ErrBusy
	}
	defer s.mu.Unlock()
	if err := s.state(true); err != nil {
		return prospectjob.ManifestJob{}, err
	}
	if prepared == nil || prepared.state == nil || prepared.state.store != s {
		return prospectjob.ManifestJob{}, ErrManifestAuthority
	}
	p := prepared.state
	if p.pin != s.meta.pin() || p.descriptor != s.meta.Descriptor {
		return prospectjob.ManifestJob{}, ErrManifestAuthority
	}
	if check == nil {
		return prospectjob.ManifestJob{}, errors.New("manifest issuance requires an original-session recheck")
	}
	if p.committed {
		return prospectjob.ManifestJob{}, ErrConflict
	}
	if s.meta.JobCount == math.MaxUint64 {
		return prospectjob.ManifestJob{}, errors.New("manifest job counter exhausted")
	}
	if err := check(); err != nil {
		return prospectjob.ManifestJob{}, fmt.Errorf("manifest issuance session check: %w", err)
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
	job, err := prospectjob.NewManifest(p.context, p.pin.AuthorityID, p.pin.DescriptorID, p.publicKey, p.challenge, issued)
	if err != nil {
		return prospectjob.ManifestJob{}, err
	}
	exists, err := s.db.Has(jobKey(job.ID), nil)
	if err != nil {
		return prospectjob.ManifestJob{}, err
	}
	if exists {
		return prospectjob.ManifestJob{}, ErrConflict
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
	if err := check(); err != nil {
		return prospectjob.ManifestJob{}, fmt.Errorf("manifest issuance session check: %w", err)
	}
	if err := s.commit(batch, next); err != nil {
		return prospectjob.ManifestJob{}, err
	}
	p.committed = true
	s.mark("issued")
	return job, nil
}

// PrepareManifestAdmission performs all proof hashing, signature checking and
// appearance binding outside an account lease. It does not observe a clock or
// persist anything. Historical recovery verifies original stored evidence and
// does not require a new node eligibility decision.
func (s *ManifestStore) PrepareManifestAdmission(proof prospect.Proof, auth gymauth.Authorization) (*PreparedManifestAdmission, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.state(true); err != nil {
		return nil, err
	}
	proof, auth, err := manifestEvidence(proof, auth)
	if err != nil {
		return nil, err
	}
	job, err := s.loadJob(proof.JobID)
	if err != nil {
		return nil, err
	}
	if proof.GymID != job.GymID || auth.PublicKey != job.PublicKey {
		return nil, errors.New("manifest proof key or gym differs from issued job")
	}
	if err := gymauth.VerifyAuthorization(job.Context, proof, auth); err != nil {
		return nil, err
	}
	verified, err := prospect.Verify(job.Context, proof)
	if err != nil {
		return nil, err
	}
	p := &preparedManifestAdmission{store: s, pin: s.meta.pin(), descriptor: s.meta.Descriptor}
	raw, err := s.db.Get(recordKey(verified.ProspectID), nil)
	if err == nil {
		original, err := s.indexedRecord(verified.ProspectID, raw)
		if err != nil {
			return nil, err
		}
		if !manifestSameProof(original.Proof, proof) || original.Context != job.Context || original.Job != job || original.Verified != verified {
			if err := verifyManifestRecord(original); err != nil {
				return nil, err
			}
		}
		if original.Verified != verified || original.Job != job {
			return nil, ErrConflict
		}
		p.record, p.historical, p.originalRaw = ownedManifestRecord(original), true, bytes.Clone(raw)
		return &PreparedManifestAdmission{state: p}, nil
	}
	if !errors.Is(err, leveldb.ErrNotFound) {
		return nil, err
	}
	if s.meta.LastSequence == math.MaxUint64 {
		return nil, errors.New("manifest receipt sequence exhausted")
	}
	binding, err := s.binding(verified)
	if err != nil {
		return nil, err
	}
	p.record = ManifestRecord{Version: Version5, Sequence: math.MaxUint64, Context: job.Context, Proof: proof, Verified: verified, Authorization: auth, Job: job, Birth: Birth{RuleVersion: job.RuleVersion, BornAtMS: job.IssuedAtMS}, StaticBinding: binding}
	if !manifestReceiptFits(p.record) {
		return nil, errors.New("manifest receipt exceeds size bound")
	}
	s.mark("bound")
	return &PreparedManifestAdmission{state: p}, nil
}

// CommitManifestAdmission performs bounded local checks and synchronized DB
// writes only. It never hashes work, renders a binding, calls a node, or waits
// for the registry gate. check has the same already-held-lease contract as
// CommitManifestIssue. A changed duplicate returns ErrConflict for a fresh
// preparation outside the account lease, never an under-lease verification.
func (s *ManifestStore) CommitManifestAdmission(prepared *PreparedManifestAdmission, observation Eligibility, check func() error) (ManifestRecord, bool, error) {
	if !s.mu.TryLock() {
		return ManifestRecord{}, false, ErrBusy
	}
	defer s.mu.Unlock()
	if err := s.state(true); err != nil {
		return ManifestRecord{}, false, err
	}
	if prepared == nil || prepared.state == nil || prepared.state.store != s {
		return ManifestRecord{}, false, ErrManifestAuthority
	}
	p := prepared.state
	if p.pin != s.meta.pin() || p.descriptor != s.meta.Descriptor {
		return ManifestRecord{}, false, ErrManifestAuthority
	}
	if check == nil {
		return ManifestRecord{}, false, errors.New("manifest admission requires an original-session recheck")
	}
	job, err := s.loadJob(p.record.Job.ID)
	if err != nil {
		return ManifestRecord{}, false, err
	}
	if job != p.record.Job {
		return ManifestRecord{}, false, ErrConflict
	}
	id := p.record.Verified.ProspectID
	raw, err := s.db.Get(recordKey(id), nil)
	if p.historical {
		if err != nil {
			if errors.Is(err, leveldb.ErrNotFound) {
				return ManifestRecord{}, false, ErrConflict
			}
			return ManifestRecord{}, false, err
		}
		if !bytes.Equal(raw, p.originalRaw) || p.record.Sequence > s.meta.LastSequence {
			return ManifestRecord{}, false, ErrConflict
		}
		index, err := s.db.Get(sequenceKey(p.record.Sequence), nil)
		if err != nil || string(index) != id {
			return ManifestRecord{}, false, ErrConflict
		}
		if err := check(); err != nil {
			return ManifestRecord{}, false, fmt.Errorf("manifest recovery session check: %w", err)
		}
		return ownedManifestRecord(p.record), false, nil
	}
	if err == nil {
		return ManifestRecord{}, false, ErrConflict
	}
	if !errors.Is(err, leveldb.ErrNotFound) {
		return ManifestRecord{}, false, err
	}
	if s.meta.LastSequence == math.MaxUint64 {
		return ManifestRecord{}, false, errors.New("manifest receipt sequence exhausted")
	}
	if err := check(); err != nil {
		return ManifestRecord{}, false, fmt.Errorf("manifest admission session check: %w", err)
	}
	observed, err := s.clockMillis()
	if err != nil {
		return ManifestRecord{}, false, err
	}
	if err := s.persistClock(observed); err != nil {
		return ManifestRecord{}, false, err
	}
	if observed < job.IssuedAtMS {
		return ManifestRecord{}, false, ErrClock
	}
	if observed >= job.ExpiresAtMS {
		return ManifestRecord{}, false, ErrJobExpired
	}
	if err := ValidateEligibility(job.Context, p.record.Verified, observation); err != nil {
		return ManifestRecord{}, false, err
	}
	r := ownedManifestRecord(p.record)
	r.Sequence = s.meta.LastSequence + 1
	r.Eligibility = observation
	s.mark("before-admission")
	if err := check(); err != nil {
		return ManifestRecord{}, false, fmt.Errorf("manifest admission session check: %w", err)
	}
	accepted, err := s.clockMillis()
	if err != nil {
		return ManifestRecord{}, false, err
	}
	if accepted < job.IssuedAtMS || accepted >= job.ExpiresAtMS {
		if err := s.persistClock(accepted); err != nil {
			return ManifestRecord{}, false, err
		}
		if accepted < job.IssuedAtMS {
			return ManifestRecord{}, false, ErrClock
		}
		return ManifestRecord{}, false, ErrJobExpired
	}
	r.Birth.AcceptedAtMS = accepted
	raw, err = json.Marshal(r)
	if err != nil || len(raw) > MaxRecordSize {
		return ManifestRecord{}, false, errors.New("manifest receipt encoding exceeds size bound")
	}
	next := s.meta
	next.LastSequence, next.LastClockMS = r.Sequence, accepted
	batch := new(leveldb.Batch)
	batch.Put(recordKey(id), raw)
	batch.Put(sequenceKey(r.Sequence), []byte(id))
	if err := s.commit(batch, next); err != nil {
		return ManifestRecord{}, false, err
	}
	s.mark("admitted")
	return r, true, nil
}
