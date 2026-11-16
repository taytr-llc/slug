// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package registry

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/wire/v2"
)

const jobPrefix = "job:"

var (
	ErrJobNotFound = errors.New("mining job was not issued by this registry")
	ErrJobExpired  = errors.New("mining job has expired; new admission is rejected")
	ErrClock       = errors.New("trusted authority clock is invalid or moved backward")
)

// CreateBirth initializes a new schema-3 job authority and birth registry.
// It never migrates older evidence or activates a public admission service.
func CreateBirth(path string, network uint32, genesis string) (*Store, error) {
	if wire.TestPoW {
		return nil, fmt.Errorf("birth issuance requires production PoW; slugtestpow is unsupported")
	}
	return create(path, network, genesis, Version3)
}

func jobKey(id string) []byte { return []byte(jobPrefix + id) }

func (store *Store) decodeJob(raw []byte, id string) (prospectjob.Job, error) {
	var job prospectjob.Job
	if err := decodeCanonical(raw, prospectjob.MaxJobSize, &job); err != nil {
		return prospectjob.Job{}, err
	}
	if err := prospectjob.Validate(job); err != nil || job.ID != id ||
		job.Context.Network != store.meta.Network || job.Context.Genesis != store.meta.Genesis ||
		job.IssuedAtMS > store.meta.LastClockMS {
		return prospectjob.Job{}, fmt.Errorf("%w: inconsistent stored job", ErrCorrupt)
	}
	return job, nil
}

func (store *Store) loadJob(id string) (prospectjob.Job, error) {
	id, err := normalizeHash(id)
	if err != nil {
		return prospectjob.Job{}, err
	}
	raw, err := store.db.Get(jobKey(id), nil)
	if errors.Is(err, leveldb.ErrNotFound) {
		return prospectjob.Job{}, ErrJobNotFound
	}
	if err != nil {
		return prospectjob.Job{}, err
	}
	return store.decodeJob(raw, id)
}

// GetJob returns immutable public issuance evidence, not renewed eligibility.
// It does not consult a clock or node and does not prove an honest issuer.
func (store *Store) GetJob(id string) (prospectjob.Job, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if err := store.checkState(); err != nil {
		return prospectjob.Job{}, err
	}
	if store.meta.Version != Version3 {
		return prospectjob.Job{}, errors.New("jobs require a birth registry")
	}
	return store.loadJob(id)
}

func (store *Store) validateBirthRecord(record Record) error {
	if record.Version != Version3 {
		if record.Job != nil || record.Birth != nil {
			return fmt.Errorf("%w: legacy evidence contains birth fields", ErrCorrupt)
		}
		return nil
	}
	if record.Job == nil || record.Birth == nil || record.Authorization == nil {
		return fmt.Errorf("%w: birth receipt is missing issuance evidence", ErrCorrupt)
	}
	job, err := store.loadJob(record.Job.ID)
	if err != nil || job != *record.Job || record.Context != job.Context ||
		strings.ToLower(record.Proof.JobID) != job.ID || strings.ToLower(record.Proof.GymID) != job.GymID ||
		strings.ToLower(record.Authorization.PublicKey) != job.PublicKey {
		return fmt.Errorf("%w: receipt does not match issued job", ErrCorrupt)
	}
	birth := record.Birth
	if birth.RuleVersion != job.RuleVersion || birth.BornAtMS != job.IssuedAtMS ||
		birth.AcceptedAtMS < job.IssuedAtMS || birth.AcceptedAtMS >= job.ExpiresAtMS ||
		birth.AcceptedAtMS > store.meta.LastClockMS {
		return fmt.Errorf("%w: receipt birth interval is invalid", ErrCorrupt)
	}
	return nil
}

func (store *Store) timedWriteState() error {
	if err := store.checkState(); err != nil {
		return err
	}
	if store.readOnly {
		return ErrReadOnly
	}
	if store.meta.Version != Version3 {
		return errors.New("timed admission requires a birth registry")
	}
	if wire.TestPoW {
		return errors.New("birth admission requires production PoW; slugtestpow is unsupported")
	}
	return nil
}

func (store *Store) clockMillis() (int64, error) {
	now := time.Now
	if store.now != nil {
		now = store.now
	}
	value := now().UnixMilli()
	if value <= 0 || value < store.clockSeen || value < store.meta.LastClockMS {
		return 0, ErrClock
	}
	store.clockSeen = value
	return value, nil
}

func (store *Store) commitBatch(batch *leveldb.Batch, next metadata) error {
	raw, _ := json.Marshal(next)
	batch.Put([]byte(metadataKey), raw)
	if err := store.writeBatch(batch, &opt.WriteOptions{Sync: true}); err != nil {
		store.poisoned = err
		return fmt.Errorf("%w: %w", ErrPoisoned, err)
	}
	store.meta = next
	return nil
}

// persistClock preserves observations even when an expired or node-rejected
// new admission has no receipt. Reopening cannot forget a recorded later time.
// It does not make the wall clock trustworthy or detect a whole-store rollback.
func (store *Store) persistClock(value int64) error {
	if value <= store.meta.LastClockMS {
		return nil
	}
	next := store.meta
	next.LastClockMS = value
	return store.commitBatch(new(leveldb.Batch), next)
}

// IssueJob creates a fixed five-minute job with an unpredictable challenge.
// check is a mandatory trusted-node context guard and runs under the store lock.
// The caller is the trusted authority, not an untrusted context supplier. No
// timestamp, lifetime, challenge, or job ID can be provided by the caller.
// The new job is returned only after its issuance and clock are synchronized.
func (store *Store) IssueJob(context prospect.Context, publicKey string, check func() error) (prospectjob.Job, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if err := store.timedWriteState(); err != nil {
		return prospectjob.Job{}, err
	}
	if check == nil {
		return prospectjob.Job{}, errors.New("job issuance requires a final node check")
	}
	genesis, err := chainIdentity(context.Network, context.Genesis)
	if err != nil || context.Network != store.meta.Network || genesis != store.meta.Genesis {
		return prospectjob.Job{}, errors.New("job context does not match registry chain")
	}
	gym, err := gymauth.GymID(context, publicKey)
	if err != nil {
		return prospectjob.Job{}, err
	}
	if _, err := prospect.CommitmentScript(context, gym); err != nil {
		return prospectjob.Job{}, err
	}
	if store.meta.JobCount == ^uint64(0) {
		return prospectjob.Job{}, errors.New("job counter exhausted")
	}
	if err := check(); err != nil {
		return prospectjob.Job{}, fmt.Errorf("job issuance node check failed: %w", err)
	}
	random := store.random
	if random == nil {
		random = rand.Reader
	}
	var challenge [32]byte
	if _, err := io.ReadFull(random, challenge[:]); err != nil {
		return prospectjob.Job{}, errors.New("mining challenge generation failed")
	}
	issued, err := store.clockMillis()
	if err != nil {
		return prospectjob.Job{}, err
	}
	job, err := prospectjob.New(context, publicKey, hex.EncodeToString(challenge[:]), issued)
	if err != nil {
		return prospectjob.Job{}, err
	}
	if exists, err := store.db.Has(jobKey(job.ID), nil); err != nil || exists {
		return prospectjob.Job{}, errors.New("mining job ID is unavailable; issue a new job")
	}
	raw, err := json.Marshal(job)
	if err != nil || len(raw) > prospectjob.MaxJobSize {
		return prospectjob.Job{}, errors.New("job exceeds size bound")
	}
	next := store.meta
	next.JobCount++
	next.LastClockMS = issued
	batch := new(leveldb.Batch)
	batch.Put(jobKey(job.ID), raw)
	if err := store.commitBatch(batch, next); err != nil {
		return prospectjob.Job{}, err
	}
	return job, nil
}

// RecordJob admits a job-bound proof with an immutable D68 birth. Only jobs
// actually issued in this registry are eligible. New admissions require the
// mandatory node guard and a final time strictly before expiry. No birth or
// sequence is allocated on rejection; clock high-water observations may persist.
//
// An already accepted proof returns its original historical receipt without
// clock/expiry or node checks. This is recovery of evidence, not a new admission
// or a claim of current eligibility/canonicality. The signature and production
// work are still verified. The callback must not reenter this Store.
func (store *Store) RecordJob(proof prospect.Proof, authorization gymauth.Authorization, check func(prospect.Context, prospect.Proof) error) (Record, bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if err := store.timedWriteState(); err != nil {
		return Record{}, false, err
	}
	if check == nil {
		return Record{}, false, errors.New("birth admission requires a final node check")
	}
	if proof.Version != prospect.Version2 || len(proof.MerkleBranch) > 20 {
		return Record{}, false, errors.New("birth admission requires a bounded job-bound proof")
	}
	proof.MerkleBranch = append([]string{}, proof.MerkleBranch...)
	job, err := store.loadJob(proof.JobID)
	if err != nil {
		return Record{}, false, err
	}
	if strings.ToLower(authorization.PublicKey) != job.PublicKey {
		return Record{}, false, errors.New("authorization key does not match issued job")
	}
	if err := gymauth.VerifyAuthorization(job.Context, proof, authorization); err != nil {
		return Record{}, false, err
	}
	verified, err := prospect.Verify(job.Context, proof)
	if err != nil {
		return Record{}, false, err
	}
	id := verified.ProspectID
	raw, err := store.db.Get(recordKey(id), nil)
	if err == nil {
		original, err := store.verifiedRecord(id, raw)
		if err != nil {
			return Record{}, false, err
		}
		if original.Verified != verified || original.Job == nil || *original.Job != job {
			return Record{}, false, ErrConflict
		}
		return original, false, nil
	}
	if !errors.Is(err, leveldb.ErrNotFound) {
		return Record{}, false, err
	}
	observed, err := store.clockMillis()
	if err != nil {
		return Record{}, false, err
	}
	if err := store.persistClock(observed); err != nil {
		return Record{}, false, err
	}
	if observed < job.IssuedAtMS {
		return Record{}, false, ErrClock
	}
	if observed >= job.ExpiresAtMS {
		return Record{}, false, ErrJobExpired
	}
	if store.meta.LastSequence == ^uint64(0) {
		return Record{}, false, errors.New("registry receipt sequence is exhausted")
	}
	// The guard receives its own slice so it cannot change verified evidence
	// between authorization/work verification and the synchronized receipt.
	guardProof := proof
	guardProof.MerkleBranch = append([]string{}, proof.MerkleBranch...)
	if err := check(job.Context, guardProof); err != nil {
		return Record{}, false, fmt.Errorf("birth admission node check failed: %w", err)
	}
	accepted, err := store.clockMillis()
	if err != nil {
		return Record{}, false, err
	}
	if accepted >= job.ExpiresAtMS {
		if err := store.persistClock(accepted); err != nil {
			return Record{}, false, err
		}
		return Record{}, false, ErrJobExpired
	}
	record := Record{Version: Version3, Sequence: store.meta.LastSequence + 1,
		Context: job.Context, Proof: proof, Verified: verified, Authorization: &authorization, Job: &job,
		Birth: &Birth{RuleVersion: job.RuleVersion, BornAtMS: job.IssuedAtMS, AcceptedAtMS: accepted}}
	raw, err = json.Marshal(record)
	if err != nil || len(raw) > MaxRecordSize {
		return Record{}, false, errors.New("birth receipt exceeds size bound")
	}
	next := store.meta
	next.LastSequence = record.Sequence
	next.LastClockMS = accepted
	batch := new(leveldb.Batch)
	batch.Put(recordKey(id), raw)
	batch.Put(sequenceKey(record.Sequence), []byte(id))
	if err := store.commitBatch(batch, next); err != nil {
		return Record{}, false, err
	}
	return record, true, nil
}
