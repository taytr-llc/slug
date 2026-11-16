// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package registry

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/wire/v2"
)

const (
	// Version1 stores conditional proof evidence without key authorization.
	Version1 uint32 = 1

	// Version2 requires gym-key authorization in every immutable receipt.
	Version2 uint32 = 2

	// Version3 records D68 jobs and immutable issuance-based fighter births.
	Version3 uint32 = 3

	// MaxRecordSize bounds one stored JSON receipt in bytes.
	MaxRecordSize = 65536

	// MaxPageSize bounds the number of receipts returned by List.
	MaxPageSize = 100

	application    = "slug-prospect-registry"
	markerName     = "SLUG-REGISTRY.json"
	databaseName   = "db"
	maxMarkerSize  = 1024
	metadataKey    = "meta"
	recordPrefix   = "record:"
	sequencePrefix = "sequence:"
)

var (
	ErrClosed   = errors.New("registry is closed")
	ErrReadOnly = errors.New("registry is read-only")
	ErrPoisoned = errors.New("registry write outcome is uncertain; close and reopen")
	ErrNotFound = errors.New("registry record not found")
	ErrCorrupt  = errors.New("registry structure or evidence is corrupt")
	ErrConflict = errors.New("prospect ID has conflicting verified evidence")
)

// Record is the first successfully verified evidence for a prospect ID.
// Sequence orders local receipts, not discovery, birth, or chain events.
type Record struct {
	Version       uint32                 `json:"version"`
	Sequence      uint64                 `json:"sequence"`
	Context       prospect.Context       `json:"context"`
	Proof         prospect.Proof         `json:"proof"`
	Verified      prospect.Verified      `json:"verified"`
	Authorization *gymauth.Authorization `json:"authorization,omitempty"`
	Job           *prospectjob.Job       `json:"job,omitempty"`
	Birth         *Birth                 `json:"birth,omitempty"`
}

// Birth is the immutable D68 league clock, not an exact discovery timestamp.
// The trusted authority observed qualifying work within the recorded job's
// issuance/expiry interval. Replays retain these original values forever.
type Birth struct {
	RuleVersion  uint32 `json:"rule_version"`
	BornAtMS     int64  `json:"born_at_ms"`
	AcceptedAtMS int64  `json:"accepted_at_ms"`
}

type marker struct {
	Application string `json:"application"`
	Version     uint32 `json:"version"`
	Network     uint32 `json:"network"`
	Genesis     string `json:"genesis"`
}

type metadata struct {
	Version      uint32 `json:"version"`
	Network      uint32 `json:"network"`
	Genesis      string `json:"genesis"`
	LastSequence uint64 `json:"last_sequence"`
	JobCount     uint64 `json:"job_count,omitempty"`
	LastClockMS  int64  `json:"last_clock_ms,omitempty"`
}

// Store owns an exclusively opened database handle and serializes all API
// operations. Callers must bound admission and queues around slow verification.
// Close the store before copying its complete directory for backup.
// The zero value is not usable; obtain a handle with Create or Open.
type Store struct {
	mu         sync.Mutex
	db         *leveldb.DB
	meta       metadata
	readOnly   bool
	closed     bool
	poisoned   error
	writeBatch func(*leveldb.Batch, *opt.WriteOptions) error
	// Clock and randomness are internal test seams, never caller-supplied time
	// or nonce inputs. Durable high water survives normal close/reopen.
	now       func() time.Time
	random    io.Reader
	clockSeen int64
}

func normalizeHash(value string) (string, error) {
	if len(value) != 64 {
		return "", fmt.Errorf("hash or ID must contain exactly 64 hexadecimal characters")
	}
	var decoded [32]byte
	if _, err := hex.Decode(decoded[:], []byte(value)); err != nil {
		return "", fmt.Errorf("invalid hash or ID: %w", err)
	}
	return hex.EncodeToString(decoded[:]), nil
}

func chainIdentity(network uint32, genesis string) (string, error) {
	if network == 0 {
		return "", fmt.Errorf("network must be nonzero")
	}
	canonical, err := normalizeHash(genesis)
	if err != nil {
		return "", err
	}
	if canonical == strings.Repeat("0", 64) {
		return "", fmt.Errorf("genesis must be nonzero")
	}
	return canonical, nil
}

func syncDirectory(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	syncErr := file.Sync()
	closeErr := file.Close()
	return errors.Join(syncErr, closeErr)
}

func writeMarker(path string, value marker) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(raw)
	var syncErr error
	if writeErr == nil {
		syncErr = file.Sync()
	}
	return errors.Join(writeErr, syncErr, file.Close())
}

func regularDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("registry path is not a regular directory: %s", path)
	}
	return nil
}

func readMarker(path string) (marker, error) {
	var value marker
	if err := regularDirectory(path); err != nil {
		return value, err
	}
	markerPath := filepath.Join(path, markerName)
	info, err := os.Lstat(markerPath)
	if err != nil {
		return value, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxMarkerSize {
		return value, fmt.Errorf("%w: invalid application marker", ErrCorrupt)
	}
	file, err := os.Open(markerPath)
	if err != nil {
		return value, err
	}
	raw, readErr := io.ReadAll(io.LimitReader(file, maxMarkerSize+1))
	err = errors.Join(readErr, file.Close())
	if err != nil {
		return value, err
	}
	if len(raw) > maxMarkerSize || json.Unmarshal(raw, &value) != nil {
		return value, fmt.Errorf("%w: malformed application marker", ErrCorrupt)
	}
	canonical, _ := json.Marshal(value)
	canonical = append(canonical, '\n')
	genesis, err := chainIdentity(value.Network, value.Genesis)
	if err != nil || value.Application != application || !supportedVersion(value.Version) ||
		value.Genesis != genesis || !bytes.Equal(raw, canonical) {
		return value, fmt.Errorf("%w: unrecognized application marker or schema", ErrCorrupt)
	}
	return value, nil
}

// Create initializes a new dedicated directory. It refuses every existing
// path and retains an incomplete directory after failure for inspection;
// callers must never erase evidence merely to retry initialization.
func Create(path string, network uint32, genesis string) (*Store, error) {
	return create(path, network, genesis, Version1)
}

// CreateAuthenticated initializes a separate schema-2 registry. Every admission
// requires a proof-bound gym-key authorization. It does not migrate old evidence
// or establish an account, wallet balance, ownership transfer, or birth time.
func CreateAuthenticated(path string, network uint32, genesis string) (*Store, error) {
	return create(path, network, genesis, Version2)
}

func supportedVersion(version uint32) bool {
	return version == Version1 || version == Version2 || version == Version3
}

func create(path string, network uint32, genesis string, version uint32) (*Store, error) {
	canonicalGenesis, err := chainIdentity(network, genesis)
	if err != nil {
		return nil, err
	}
	if err := os.Mkdir(path, 0700); err != nil {
		return nil, err
	}
	value := marker{application, version, network, canonicalGenesis}
	if err := writeMarker(filepath.Join(path, markerName), value); err != nil {
		return nil, fmt.Errorf("registry initialization incomplete; directory retained: %w", err)
	}
	dbPath := filepath.Join(path, databaseName)
	if err := os.Mkdir(dbPath, 0700); err != nil {
		return nil, err
	}
	db, err := leveldb.OpenFile(dbPath, &opt.Options{Strict: opt.StrictAll, ErrorIfExist: true})
	if err != nil {
		return nil, err
	}
	meta := metadata{Version: version, Network: network, Genesis: canonicalGenesis}
	raw, _ := json.Marshal(meta)
	batch := new(leveldb.Batch)
	batch.Put([]byte(metadataKey), raw)
	if err := db.Write(batch, &opt.WriteOptions{Sync: true}); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("registry initialization write outcome uncertain; directory retained: %w", err)
	}
	for _, directory := range []string{dbPath, path, filepath.Dir(filepath.Clean(path))} {
		if err := syncDirectory(directory); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("registry initialization synchronization failed; directory retained: %w", err)
		}
	}
	return &Store{db: db, meta: meta, writeBatch: db.Write}, nil
}

// Open requires an existing recognized registry. It checks schema, receipts,
// sequence indexes, and gap-free counts without doing Argon verification.
// Records are reverified when read. No repair, migration, or recreation occurs.
func Open(path string, readOnly bool) (*Store, error) {
	value, err := readMarker(path)
	if err != nil {
		return nil, err
	}
	dbPath := filepath.Join(path, databaseName)
	if err := regularDirectory(dbPath); err != nil {
		return nil, err
	}
	db, err := leveldb.OpenFile(dbPath, &opt.Options{
		Strict: opt.StrictAll, ErrorIfMissing: true, ReadOnly: readOnly,
	})
	if err != nil {
		return nil, err
	}
	store := &Store{db: db, readOnly: readOnly, writeBatch: db.Write}
	if err := store.checkStructure(value); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func decodeCanonical(raw []byte, limit int, value interface{}) error {
	if len(raw) == 0 || len(raw) > limit {
		return fmt.Errorf("%w: stored JSON exceeds its size bound or is empty", ErrCorrupt)
	}
	if err := json.Unmarshal(raw, value); err != nil {
		return fmt.Errorf("%w: malformed stored JSON: %v", ErrCorrupt, err)
	}
	canonical, err := json.Marshal(value)
	if err != nil || !bytes.Equal(raw, canonical) {
		return fmt.Errorf("%w: stored JSON is noncanonical or has missing/unknown/duplicate fields", ErrCorrupt)
	}
	return nil
}

func sequenceKey(sequence uint64) []byte {
	key := make([]byte, len(sequencePrefix)+8)
	copy(key, sequencePrefix)
	binary.BigEndian.PutUint64(key[len(sequencePrefix):], sequence)
	return key
}

func recordKey(id string) []byte { return []byte(recordPrefix + id) }

func (store *Store) decodeRecord(raw []byte, id string) (Record, error) {
	var record Record
	if err := decodeCanonical(raw, MaxRecordSize, &record); err != nil {
		return Record{}, err
	}
	if record.Version != store.meta.Version || record.Sequence == 0 || record.Sequence > store.meta.LastSequence {
		return Record{}, fmt.Errorf("%w: invalid receipt version or sequence", ErrCorrupt)
	}
	if (record.Version == Version1 && record.Authorization != nil) ||
		(record.Version >= Version2 && record.Authorization == nil) {
		return Record{}, fmt.Errorf("%w: receipt authorization does not match schema", ErrCorrupt)
	}
	if err := store.validateBirthRecord(record); err != nil {
		return Record{}, err
	}
	if record.Authorization != nil {
		if err := gymauth.VerifyAuthorization(record.Context, record.Proof, *record.Authorization); err != nil {
			return Record{}, fmt.Errorf("%w: invalid stored gym authorization: %v", ErrCorrupt, err)
		}
	}
	genesis, err := chainIdentity(record.Context.Network, record.Context.Genesis)
	if err != nil || record.Context.Network != store.meta.Network || genesis != store.meta.Genesis {
		return Record{}, fmt.Errorf("%w: receipt context has a different chain", ErrCorrupt)
	}
	if _, err := prospect.CommitmentScript(record.Context, record.Proof.GymID); err != nil {
		return Record{}, fmt.Errorf("%w: malformed receipt context: %v", ErrCorrupt, err)
	}
	// Open checks shapes, linkage, and cheap key authorization. Argon proof
	// verification is deferred to reads so opening does not rehash all history.
	proof := record.Proof
	expectedVersion := prospect.Version1
	if record.Version == Version3 {
		expectedVersion = prospect.Version2
	}
	if proof.Version != expectedVersion || (proof.Version == prospect.Version1 && proof.JobID != "") || len(proof.Header) != 160 ||
		len(proof.Coinbase) == 0 || len(proof.Coinbase) > 32768 || len(proof.Coinbase)%2 != 0 ||
		proof.MerkleBranch == nil || len(proof.MerkleBranch) > 20 {
		return Record{}, fmt.Errorf("%w: invalid stored proof shape", ErrCorrupt)
	}
	if _, err := hex.DecodeString(proof.Header); err != nil {
		return Record{}, fmt.Errorf("%w: invalid stored header encoding", ErrCorrupt)
	}
	if _, err := hex.DecodeString(proof.Coinbase); err != nil {
		return Record{}, fmt.Errorf("%w: invalid stored coinbase encoding", ErrCorrupt)
	}
	for _, sibling := range proof.MerkleBranch {
		if len(sibling) != 64 {
			return Record{}, fmt.Errorf("%w: invalid stored sibling length", ErrCorrupt)
		}
		if _, err := hex.DecodeString(sibling); err != nil {
			return Record{}, fmt.Errorf("%w: invalid stored sibling encoding", ErrCorrupt)
		}
	}
	verified := record.Verified
	if verified.Version != expectedVersion || verified.Verification != "supplied-context" ||
		verified.ProspectID != id || verified.Network != store.meta.Network ||
		verified.Genesis != store.meta.Genesis || verified.ParentHeight != record.Context.ParentHeight ||
		verified.ParentHash != strings.ToLower(record.Context.ParentHash) ||
		verified.GymID != strings.ToLower(proof.GymID) ||
		verified.DNA.DNAVersion != record.Context.DNAVersion || verified.DNA.Seed != verified.WorkHash {
		return Record{}, fmt.Errorf("%w: inconsistent stored verified record", ErrCorrupt)
	}
	for _, hash := range []string{id, verified.PolicyID, verified.WorkHash, verified.ParentHash, verified.GymID} {
		canonical, err := normalizeHash(hash)
		if err != nil || canonical != hash {
			return Record{}, fmt.Errorf("%w: invalid stored identifier", ErrCorrupt)
		}
	}
	return record, nil
}

func (store *Store) checkStructure(value marker) error {
	raw, err := store.db.Get([]byte(metadataKey), nil)
	if err != nil {
		return fmt.Errorf("%w: missing or unreadable metadata: %w", ErrCorrupt, err)
	}
	if err := decodeCanonical(raw, maxMarkerSize, &store.meta); err != nil {
		return err
	}
	if store.meta.Version != value.Version || store.meta.Network != value.Network || store.meta.Genesis != value.Genesis {
		return fmt.Errorf("%w: metadata does not match application marker", ErrCorrupt)
	}
	if store.meta.LastClockMS < 0 || (store.meta.Version != Version3 && (store.meta.JobCount != 0 || store.meta.LastClockMS != 0)) {
		return fmt.Errorf("%w: clock/job metadata does not match schema", ErrCorrupt)
	}
	store.clockSeen = store.meta.LastClockMS
	var records, sequences, jobs uint64
	iterator := store.db.NewIterator(nil, nil)
	defer iterator.Release()
	for iterator.Next() {
		key := iterator.Key()
		switch {
		case bytes.Equal(key, []byte(metadataKey)):
		case bytes.HasPrefix(key, []byte(jobPrefix)):
			if store.meta.Version != Version3 {
				return fmt.Errorf("%w: jobs require birth registry schema", ErrCorrupt)
			}
			if _, err := store.decodeJob(iterator.Value(), string(key[len(jobPrefix):])); err != nil {
				return err
			}
			jobs++
		case bytes.HasPrefix(key, []byte(recordPrefix)):
			id := string(key[len(recordPrefix):])
			record, err := store.decodeRecord(iterator.Value(), id)
			if err != nil {
				return err
			}
			indexedID, err := store.db.Get(sequenceKey(record.Sequence), nil)
			if err != nil || string(indexedID) != id {
				return fmt.Errorf("%w: missing or mismatched sequence index", ErrCorrupt)
			}
			records++
		case bytes.HasPrefix(key, []byte(sequencePrefix)):
			if len(key) != len(sequencePrefix)+8 {
				return fmt.Errorf("%w: malformed sequence index key", ErrCorrupt)
			}
			sequence := binary.BigEndian.Uint64(key[len(sequencePrefix):])
			id := string(iterator.Value())
			canonical, err := normalizeHash(id)
			if err != nil || id != canonical || sequence == 0 || sequence > store.meta.LastSequence {
				return fmt.Errorf("%w: malformed sequence index value", ErrCorrupt)
			}
			raw, err := store.db.Get(recordKey(id), nil)
			if err != nil {
				return fmt.Errorf("%w: index references missing record: %w", ErrCorrupt, err)
			}
			record, err := store.decodeRecord(raw, id)
			if err != nil {
				return err
			}
			if record.Sequence != sequence {
				return fmt.Errorf("%w: sequence index references another receipt", ErrCorrupt)
			}
			sequences++
		default:
			return fmt.Errorf("%w: unknown database key", ErrCorrupt)
		}
	}
	if err := iterator.Error(); err != nil {
		return fmt.Errorf("%w: scan failed: %w", ErrCorrupt, err)
	}
	// Unique keys, in-range sequences, matching links, and these counts prove
	// a consecutive sequence without looping up to untrusted LastSequence.
	if records != store.meta.LastSequence || sequences != store.meta.LastSequence || jobs != store.meta.JobCount {
		return fmt.Errorf("%w: receipt counts or sequence continuity mismatch", ErrCorrupt)
	}
	return nil
}

func (store *Store) checkState() error {
	if store.closed {
		return ErrClosed
	}
	if store.poisoned != nil {
		return fmt.Errorf("%w: %v", ErrPoisoned, store.poisoned)
	}
	return nil
}

func (store *Store) verifiedRecord(id string, raw []byte) (Record, error) {
	record, err := store.decodeRecord(raw, id)
	if err != nil {
		return Record{}, err
	}
	indexedID, err := store.db.Get(sequenceKey(record.Sequence), nil)
	if err != nil || string(indexedID) != id {
		return Record{}, fmt.Errorf("%w: receipt index mismatch", ErrCorrupt)
	}
	if wire.TestPoW {
		return Record{}, fmt.Errorf("registry evidence reads require production PoW; slugtestpow is unsupported")
	}
	verified, err := prospect.Verify(record.Context, record.Proof)
	if err != nil {
		return Record{}, fmt.Errorf("%w: stored proof failed verification: %v", ErrCorrupt, err)
	}
	if verified != record.Verified {
		return Record{}, fmt.Errorf("%w: stored verified result does not match its proof", ErrCorrupt)
	}
	return record, nil
}

// Record verifies a raw submission and atomically assigns its first receipt.
// A duplicate returns the original record and false without writing anything.
// A write error poisons the handle because its durable outcome may be unknown.
func (store *Store) Record(context prospect.Context, proof prospect.Proof) (Record, bool, error) {
	return store.record(context, proof, nil, nil)
}

// RecordChecked verifies the evidence, then runs check immediately before a
// new write or duplicate acknowledgment. A failed check writes nothing and
// returns no receipt. The check must not call this Store: it runs under the
// admission mutex. External state can still change after check returns; this
// is an observation boundary, not a transaction with an external chain node.
// Evidence keeps its supplied-context meaning and the callback is not stored.
func (store *Store) RecordChecked(context prospect.Context, proof prospect.Proof, check func() error) (Record, bool, error) {
	if check == nil {
		return Record{}, false, fmt.Errorf("registry pre-commit check is required")
	}
	return store.record(context, proof, nil, check)
}

// RecordAuthorized verifies key authorization before production work and stores
// both atomically in a schema-2 registry. A valid replay preserves the original
// signature and receipt. Relaying this specific authorized admission is allowed.
func (store *Store) RecordAuthorized(context prospect.Context, proof prospect.Proof, authorization gymauth.Authorization) (Record, bool, error) {
	return store.record(context, proof, &authorization, nil)
}

// RecordAuthorizedChecked also runs check immediately before writing or
// acknowledging a duplicate. The same guard rules as RecordChecked apply.
func (store *Store) RecordAuthorizedChecked(context prospect.Context, proof prospect.Proof, authorization gymauth.Authorization, check func() error) (Record, bool, error) {
	if check == nil {
		return Record{}, false, fmt.Errorf("registry pre-commit check is required")
	}
	return store.record(context, proof, &authorization, check)
}

func (store *Store) record(context prospect.Context, proof prospect.Proof, authorization *gymauth.Authorization, check func() error) (Record, bool, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if err := store.checkState(); err != nil {
		return Record{}, false, err
	}
	if store.readOnly {
		return Record{}, false, ErrReadOnly
	}
	if store.meta.Version == Version3 || proof.Version != prospect.Version1 || proof.JobID != "" {
		return Record{}, false, fmt.Errorf("job-bound proofs require the birth admission path")
	}
	if (store.meta.Version == Version1 && authorization != nil) ||
		(store.meta.Version == Version2 && authorization == nil) {
		return Record{}, false, fmt.Errorf("admission authorization does not match registry schema")
	}
	genesis, err := chainIdentity(context.Network, context.Genesis)
	if err != nil || context.Network != store.meta.Network || genesis != store.meta.Genesis {
		return Record{}, false, fmt.Errorf("context does not match registry chain")
	}
	if len(proof.MerkleBranch) > 20 {
		return Record{}, false, fmt.Errorf("proof has too many Merkle siblings")
	}
	// Snapshot the only mutable input backing array and normalize nil to the
	// required empty JSON array. Other input fields are immutable Go values.
	proof.MerkleBranch = append([]string{}, proof.MerkleBranch...)
	if authorization != nil {
		if err := gymauth.VerifyAuthorization(context, proof, *authorization); err != nil {
			return Record{}, false, err
		}
	}
	verified, err := prospect.Verify(context, proof)
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
		if original.Verified != verified {
			return Record{}, false, ErrConflict
		}
		if check != nil {
			if err := check(); err != nil {
				return Record{}, false, fmt.Errorf("registry pre-commit check failed: %w", err)
			}
		}
		return original, false, nil
	}
	if !errors.Is(err, leveldb.ErrNotFound) {
		return Record{}, false, err
	}
	if store.meta.LastSequence == ^uint64(0) {
		return Record{}, false, fmt.Errorf("registry receipt sequence is exhausted")
	}
	record := Record{Version: store.meta.Version, Sequence: store.meta.LastSequence + 1,
		Context: context, Proof: proof, Verified: verified, Authorization: authorization}
	raw, err = json.Marshal(record)
	if err != nil || len(raw) > MaxRecordSize {
		return Record{}, false, fmt.Errorf("registry record exceeds %d bytes or cannot be encoded", MaxRecordSize)
	}
	next := store.meta
	next.LastSequence = record.Sequence
	metaBytes, _ := json.Marshal(next)
	batch := new(leveldb.Batch)
	batch.Put(recordKey(id), raw)
	batch.Put(sequenceKey(record.Sequence), []byte(id))
	batch.Put([]byte(metadataKey), metaBytes)
	if check != nil {
		if err := check(); err != nil {
			return Record{}, false, fmt.Errorf("registry pre-commit check failed: %w", err)
		}
	}
	if err := store.writeBatch(batch, &opt.WriteOptions{Sync: true}); err != nil {
		store.poisoned = err
		return Record{}, false, fmt.Errorf("%w: %w", ErrPoisoned, err)
	}
	store.meta = next
	return record, true, nil
}

// Get re-verifies one immutable receipt before returning it. Its ID accepts
// either hex case; the stored result and prospect ID remain unchanged.
func (store *Store) Get(id string) (Record, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if err := store.checkState(); err != nil {
		return Record{}, err
	}
	id, err := normalizeHash(id)
	if err != nil {
		return Record{}, err
	}
	raw, err := store.db.Get(recordKey(id), nil)
	if errors.Is(err, leveldb.ErrNotFound) {
		return Record{}, ErrNotFound
	}
	if err != nil {
		return Record{}, err
	}
	return store.verifiedRecord(id, raw)
}

// List returns at most limit verified receipts with sequence greater than
// after, in sequence order. Limit must be between one and MaxPageSize.
func (store *Store) List(after uint64, limit int) ([]Record, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if err := store.checkState(); err != nil {
		return nil, err
	}
	if limit < 1 || limit > MaxPageSize {
		return nil, fmt.Errorf("list limit must be in [1,%d]", MaxPageSize)
	}
	records := make([]Record, 0, limit)
	if after >= store.meta.LastSequence {
		return records, nil
	}
	iterator := store.db.NewIterator(util.BytesPrefix([]byte(sequencePrefix)), nil)
	defer iterator.Release()
	expected := after + 1
	for ok := iterator.Seek(sequenceKey(after + 1)); ok && len(records) < limit; ok = iterator.Next() {
		id := string(iterator.Value())
		raw, err := store.db.Get(recordKey(id), nil)
		if err != nil {
			return nil, fmt.Errorf("%w: indexed record unavailable: %w", ErrCorrupt, err)
		}
		record, err := store.verifiedRecord(id, raw)
		if err != nil {
			return nil, err
		}
		if record.Sequence != expected || !bytes.Equal(iterator.Key(), sequenceKey(record.Sequence)) {
			return nil, fmt.Errorf("%w: listing index mismatch", ErrCorrupt)
		}
		records = append(records, record)
		expected++
	}
	if err := iterator.Error(); err != nil {
		return nil, err
	}
	if uint64(len(records)) != min(uint64(limit), store.meta.LastSequence-after) {
		return nil, fmt.Errorf("%w: incomplete listing sequence", ErrCorrupt)
	}
	return records, nil
}

// Export streams a consistent sequence-ordered NDJSON view, re-verifying each
// receipt. An error may follow a valid output prefix; only nil means complete.
// Callers that need atomic publication must stage and rename their own output.
func (store *Store) Export(writer io.Writer) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if err := store.checkState(); err != nil {
		return err
	}
	if writer == nil {
		return fmt.Errorf("export writer is nil")
	}
	iterator := store.db.NewIterator(util.BytesPrefix([]byte(sequencePrefix)), nil)
	defer iterator.Release()
	var emitted uint64
	for iterator.Next() {
		id := string(iterator.Value())
		raw, err := store.db.Get(recordKey(id), nil)
		if err != nil {
			return fmt.Errorf("%w: indexed record unavailable: %w", ErrCorrupt, err)
		}
		record, err := store.verifiedRecord(id, raw)
		if err != nil {
			return err
		}
		if record.Sequence != emitted+1 || !bytes.Equal(iterator.Key(), sequenceKey(record.Sequence)) {
			return fmt.Errorf("%w: export sequence is not consecutive", ErrCorrupt)
		}
		line := append(raw, '\n')
		written, err := writer.Write(line)
		if err != nil {
			return err
		}
		if written != len(line) {
			return io.ErrShortWrite
		}
		emitted++
	}
	if err := iterator.Error(); err != nil {
		return err
	}
	if emitted != store.meta.LastSequence {
		return fmt.Errorf("%w: incomplete export sequence", ErrCorrupt)
	}
	return nil
}

// Close releases the database lock. It is safe to call repeatedly, including
// after an uncertain write. Other operations on a closed handle fail.
func (store *Store) Close() error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.closed {
		return nil
	}
	store.closed = true
	return store.db.Close()
}
