// Copyright (c) 2026 The SLUG developers. ISC license.
package registry

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
)

func (s *ManifestStore) decodeRecord(raw []byte, id string) (ManifestRecord, error) {
	var r ManifestRecord
	if err := decodeManifestJSON(raw, MaxRecordSize, MaxManifestRecordDepth, &r); err != nil {
		return ManifestRecord{}, err
	}
	bad := fmt.Errorf("%w: inconsistent manifest receipt", ErrCorrupt)
	if r.Version != Version5 || r.Sequence == 0 || r.Sequence > s.meta.LastSequence || !manifestCanonicalHash(id) {
		return ManifestRecord{}, bad
	}
	job, err := s.loadJob(r.Job.ID)
	if err != nil {
		return ManifestRecord{}, fmt.Errorf("%w: %w", ErrCorrupt, err)
	}
	if r.Job != job || r.Context != job.Context || r.Proof.JobID != job.ID || r.Proof.GymID != job.GymID || r.Authorization.PublicKey != job.PublicKey {
		return ManifestRecord{}, bad
	}
	p, a, err := manifestEvidence(r.Proof, r.Authorization)
	if err != nil || r.Proof.MerkleBranch == nil || !manifestSameProof(p, r.Proof) || a != r.Authorization {
		return ManifestRecord{}, bad
	}
	if err := gymauth.VerifyAuthorization(job.Context, r.Proof, r.Authorization); err != nil {
		return ManifestRecord{}, fmt.Errorf("%w: stored manifest authorization: %w", ErrCorrupt, err)
	}
	b := r.Birth
	if b.RuleVersion != job.RuleVersion || b.BornAtMS != job.IssuedAtMS || b.AcceptedAtMS < job.IssuedAtMS || b.AcceptedAtMS >= job.ExpiresAtMS || b.AcceptedAtMS > s.meta.LastClockMS {
		return ManifestRecord{}, bad
	}
	v := r.Verified
	if v.Version != prospect.Version2 || v.Verification != "supplied-context" || v.ProspectID != id || v.Network != s.meta.Network || v.Genesis != s.meta.Genesis || v.ParentHash != job.Context.ParentHash || v.ParentHeight != job.Context.ParentHeight || v.GymID != job.GymID || v.DNA.DNAVersion != job.Context.DNAVersion || v.DNA.Seed != v.WorkHash {
		return ManifestRecord{}, bad
	}
	for _, hash := range []string{v.ProspectID, v.PolicyID, v.WorkHash, v.ParentHash, v.GymID} {
		if !manifestCanonicalHash(hash) {
			return ManifestRecord{}, bad
		}
	}
	if err := ValidateEligibility(job.Context, v, r.Eligibility); err != nil {
		return ManifestRecord{}, fmt.Errorf("%w: %w", ErrCorrupt, err)
	}
	binding, err := s.binding(v)
	if err != nil {
		return ManifestRecord{}, fmt.Errorf("%w: %w", ErrCorrupt, err)
	}
	if binding != r.StaticBinding {
		return ManifestRecord{}, bad
	}
	return r, nil
}

func (s *ManifestStore) indexedRecord(id string, raw []byte) (ManifestRecord, error) {
	r, err := s.decodeRecord(raw, id)
	if err != nil {
		return ManifestRecord{}, err
	}
	index, err := s.db.Get(sequenceKey(r.Sequence), nil)
	if err != nil || len(index) != 64 || string(index) != id {
		return ManifestRecord{}, fmt.Errorf("%w: manifest sequence mismatch", ErrCorrupt)
	}
	return r, nil
}

func verifyManifestRecord(r ManifestRecord) error {
	v, err := prospect.Verify(r.Context, r.Proof)
	if err != nil {
		return fmt.Errorf("%w: manifest work verification: %w", ErrCorrupt, err)
	}
	if v != r.Verified {
		return fmt.Errorf("%w: stored verified result differs from manifest proof", ErrCorrupt)
	}
	return nil
}

func (s *ManifestStore) verifiedRecord(id string, raw []byte) (ManifestRecord, error) {
	r, err := s.indexedRecord(id, raw)
	if err != nil {
		return ManifestRecord{}, err
	}
	if err := verifyManifestRecord(r); err != nil {
		return ManifestRecord{}, err
	}
	// The decoded binding was regenerated from the same exact DNA/WorkHash
	// which is now proven to equal the production verifier's complete result.
	return r, nil
}

// manifestReceiptFits reserves every dynamic post-guard field's longest
// encoding. The local size-only value is never persisted, returned or trusted.
func manifestReceiptFits(r ManifestRecord) bool {
	r.Birth.AcceptedAtMS = math.MaxInt64
	r.Eligibility = Eligibility{EligibilityRuleVersion2, "canonical-winning-block", strings.Repeat("f", 64), math.MaxUint32}
	raw, err := json.Marshal(r)
	return err == nil && len(raw) <= MaxRecordSize
}

// RecordManifestJobChecked verifies work and atomically binds its immutable
// candidate appearance. The mandatory guard is used only for a new receipt.
// Historical recovery still needs authorization by any outer account service;
// this local store implements no login or session policy.
func (s *ManifestStore) RecordManifestJobChecked(proof prospect.Proof, auth gymauth.Authorization, check func(prospect.Context, prospect.Proof, prospect.Verified) (Eligibility, error)) (ManifestRecord, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.state(true); err != nil {
		return ManifestRecord{}, false, err
	}
	if check == nil {
		return ManifestRecord{}, false, errors.New("manifest admission requires a final eligibility guard")
	}
	proof, auth, err := manifestEvidence(proof, auth)
	if err != nil {
		return ManifestRecord{}, false, err
	}
	job, err := s.loadJob(proof.JobID)
	if err != nil {
		return ManifestRecord{}, false, err
	}
	if proof.GymID != job.GymID || auth.PublicKey != job.PublicKey {
		return ManifestRecord{}, false, errors.New("manifest proof key or gym differs from issued job")
	}
	if err := gymauth.VerifyAuthorization(job.Context, proof, auth); err != nil {
		return ManifestRecord{}, false, err
	}
	verified, err := prospect.Verify(job.Context, proof)
	if err != nil {
		return ManifestRecord{}, false, err
	}
	id := verified.ProspectID
	raw, err := s.db.Get(recordKey(id), nil)
	if err == nil {
		original, err := s.indexedRecord(id, raw)
		if err != nil {
			return ManifestRecord{}, false, err
		}
		// Reuse only the result of this call's exact normalized evidence. No
		// cross-call cache, caller-provided hash or header-only shortcut exists.
		if !manifestSameProof(original.Proof, proof) || original.Context != job.Context || original.Job != job || original.Verified != verified {
			if err := verifyManifestRecord(original); err != nil {
				return ManifestRecord{}, false, err
			}
		}
		if original.Verified != verified || original.Job != job {
			return ManifestRecord{}, false, ErrConflict
		}
		return original, false, nil
	}
	if !errors.Is(err, leveldb.ErrNotFound) {
		return ManifestRecord{}, false, err
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
	if s.meta.LastSequence == math.MaxUint64 {
		return ManifestRecord{}, false, errors.New("manifest receipt sequence exhausted")
	}
	binding, err := s.binding(verified)
	if err != nil {
		return ManifestRecord{}, false, err
	}
	s.mark("bound")
	r := ManifestRecord{Version: Version5, Sequence: s.meta.LastSequence + 1, Context: job.Context, Proof: proof, Verified: verified, Authorization: auth, Job: job, Birth: Birth{RuleVersion: job.RuleVersion, BornAtMS: job.IssuedAtMS}, StaticBinding: binding}
	if !manifestReceiptFits(r) {
		return ManifestRecord{}, false, errors.New("manifest receipt exceeds size bound")
	}
	guardProof := proof
	guardProof.MerkleBranch = append([]string{}, proof.MerkleBranch...)
	observation, err := check(job.Context, guardProof, verified)
	if err != nil {
		return ManifestRecord{}, false, fmt.Errorf("manifest admission eligibility check: %w", err)
	}
	if err := ValidateEligibility(job.Context, verified, observation); err != nil {
		return ManifestRecord{}, false, err
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
	r.Eligibility = observation
	raw, err = json.Marshal(r)
	if err != nil || len(raw) > MaxRecordSize {
		return ManifestRecord{}, false, errors.New("manifest receipt encoding exceeds size bound")
	}
	next := s.meta
	next.LastSequence = r.Sequence
	next.LastClockMS = accepted
	batch := new(leveldb.Batch)
	batch.Put(recordKey(id), raw)
	batch.Put(sequenceKey(r.Sequence), []byte(id))
	s.mark("before-admission")
	if err := s.commit(batch, next); err != nil {
		return ManifestRecord{}, false, err
	}
	s.mark("admitted")
	return r, true, nil
}

// GetManifestAdmission fully verifies one historical receipt through this
// pinned authority. It observes neither current node eligibility nor a clock.
func (s *ManifestStore) GetManifestAdmission(id string) (ManifestRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.state(false); err != nil {
		return ManifestRecord{}, err
	}
	id, err := normalizeHash(id)
	if err != nil {
		return ManifestRecord{}, err
	}
	raw, err := s.db.Get(recordKey(id), nil)
	if errors.Is(err, leveldb.ErrNotFound) {
		return ManifestRecord{}, ErrNotFound
	}
	if err != nil {
		return ManifestRecord{}, err
	}
	return s.verifiedRecord(id, raw)
}

// ListManifestAdmissions verifies at most eight consecutive historical
// receipts serially. A corrupt page returns no partial trusted output.
func (s *ManifestStore) ListManifestAdmissions(after uint64, limit int) ([]ManifestRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.state(false); err != nil {
		return nil, err
	}
	if limit < 1 || limit > MaxManifestPageSize {
		return nil, errors.New("manifest page limit must be in [1,8]")
	}
	result := make([]ManifestRecord, 0, limit)
	if after >= s.meta.LastSequence {
		return result, nil
	}
	it := s.db.NewIterator(util.BytesPrefix([]byte(sequencePrefix)), nil)
	defer it.Release()
	expected := after + 1
	encodedBytes := 2
	for ok := it.Seek(sequenceKey(expected)); ok && len(result) < limit; ok = it.Next() {
		if len(it.Key()) != len(sequencePrefix)+8 || len(it.Value()) != 64 {
			return nil, ErrCorrupt
		}
		id := string(it.Value())
		if !manifestCanonicalHash(id) {
			return nil, ErrCorrupt
		}
		raw, err := s.db.Get(recordKey(id), nil)
		if err != nil {
			return nil, fmt.Errorf("%w: missing manifest page receipt", ErrCorrupt)
		}
		r, err := s.verifiedRecord(id, raw)
		if err != nil {
			return nil, err
		}
		if r.Sequence != expected || !bytes.Equal(it.Key(), sequenceKey(expected)) {
			return nil, fmt.Errorf("%w: manifest page sequence mismatch", ErrCorrupt)
		}
		encodedBytes += len(raw)
		if len(result) > 0 {
			encodedBytes++
		}
		if encodedBytes > MaxManifestPageBytes {
			return nil, ErrCorrupt
		}
		result = append(result, r)
		expected++
	}
	if err := it.Error(); err != nil {
		return nil, err
	}
	if uint64(len(result)) != min(uint64(limit), s.meta.LastSequence-after) {
		return nil, fmt.Errorf("%w: incomplete manifest page", ErrCorrupt)
	}
	return result, nil
}
