// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package prospectworker

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"sync"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"
	"github.com/taytr-llc/slug/btcjson"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospectservice"
)

type manifestOutboxMetadata struct {
	Application  string         `json:"application"`
	Version      uint32         `json:"version"`
	Config       ManifestConfig `json:"config"`
	LastSequence uint64         `json:"last_sequence"`
	Records      uint64         `json:"records"`
	Bytes        uint64         `json:"bytes"`
	Pending      uint64         `json:"pending"`
	Reserved     uint64         `json:"reserved"`
}
type manifestOutboxMarker struct {
	Application string         `json:"application"`
	Version     uint32         `json:"version"`
	Config      ManifestConfig `json:"config"`
}

// ManifestOutbox is an explicitly initialized, exclusively locked public journal.
// Logical payload bytes plus Reserved protect every pending authorization and
// receipt; LevelDB/index/compaction overhead is additional. Per-attempt progress
// writes only an eight-byte cursor, never the immutable full block template.
type ManifestOutbox struct {
	mu                        sync.Mutex
	db                        *leveldb.DB
	meta                      manifestOutboxMetadata
	config                    ManifestConfig
	closed, poisoned, running bool
	closeErr                  error
	checkpoint                func(string)
	writeBatch                func(*leveldb.Batch, *opt.WriteOptions) error
	cachedScan                *ManifestScan
	cachedEntryBytes          uint64
}

func CreateManifestOutbox(path string, config ManifestConfig) (*ManifestOutbox, error) {
	if err := manifestProduction(); err != nil {
		return nil, err
	}
	c, e := ValidateManifestConfig(config)
	if e != nil {
		return nil, e
	}
	if e = os.Mkdir(path, 0700); e != nil {
		return nil, errors.New("could not create new worker outbox")
	}
	raw, _ := json.Marshal(manifestOutboxMarker{application, Version2, c})
	if len(raw) > maxMetadataSize {
		return nil, ErrCorrupt
	}
	f, e := os.OpenFile(filepath.Join(path, markerName), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return nil, e
	}
	_, we := f.Write(raw)
	if e = errors.Join(we, f.Sync(), f.Close()); e != nil {
		return nil, e
	}
	dbPath := filepath.Join(path, "db")
	if e = os.Mkdir(dbPath, 0700); e != nil {
		return nil, e
	}
	db, e := leveldb.OpenFile(dbPath, &opt.Options{ErrorIfExist: true, Strict: opt.StrictAll})
	if e != nil {
		return nil, errors.New("could not initialize worker outbox")
	}
	box := &ManifestOutbox{db: db, config: c, meta: manifestOutboxMetadata{Application: application, Version: Version2, Config: c}, writeBatch: db.Write}
	if e = box.commit(new(leveldb.Batch), box.meta); e != nil {
		_ = db.Close()
		return nil, e
	}
	if e = errors.Join(syncDir(dbPath), syncDir(path), syncDir(filepath.Dir(filepath.Clean(path)))); e != nil {
		_ = db.Close()
		return nil, errors.New("could not synchronize new outbox directory")
	}
	return box, nil
}
func readManifestOutboxMarker(path string, c ManifestConfig) error {
	if regularDirectory(path) != nil || regularDirectory(filepath.Join(path, "db")) != nil {
		return ErrCorrupt
	}
	name := filepath.Join(path, markerName)
	i, e := os.Lstat(name)
	if e != nil || !i.Mode().IsRegular() || i.Size() > maxMetadataSize {
		return ErrCorrupt
	}
	f, e := os.Open(name)
	if e != nil {
		return ErrCorrupt
	}
	opened, statErr := f.Stat()
	if statErr != nil || !opened.Mode().IsRegular() || opened.Size() > maxMetadataSize || !os.SameFile(i, opened) {
		_ = f.Close()
		return ErrCorrupt
	}
	raw, re := io.ReadAll(io.LimitReader(f, maxMetadataSize+1))
	if errors.Join(re, f.Close()) != nil {
		return ErrCorrupt
	}
	var v manifestOutboxMarker
	if manifestDecodeCanonical(raw, maxMetadataSize, &v) != nil || v.Application != application || v.Version != Version2 || !sameManifestIdentity(v.Config, c) || c.MaxRecords < v.Config.MaxRecords || c.MaxBytes < v.Config.MaxBytes {
		return ErrCorrupt
	}
	if canonical, err := ValidateManifestConfig(v.Config); err != nil || canonical != v.Config {
		return ErrCorrupt
	}
	return nil
}
func OpenManifestOutbox(path string, config ManifestConfig) (*ManifestOutbox, error) {
	if err := manifestProduction(); err != nil {
		return nil, err
	}
	c, e := ValidateManifestConfig(config)
	if e != nil {
		return nil, e
	}
	// Check identity before LevelDB can create a lock/log or missing directory.
	if e = readManifestOutboxMarker(path, c); e != nil {
		return nil, e
	}
	db, e := leveldb.OpenFile(filepath.Join(path, "db"), &opt.Options{ErrorIfMissing: true, Strict: opt.StrictAll})
	if e != nil {
		return nil, errors.New("could not open existing worker outbox")
	}
	box := &ManifestOutbox{db: db, config: c, writeBatch: db.Write}
	if e = box.audit(); e != nil {
		_ = db.Close()
		return nil, e
	}
	if box.meta.Config != c {
		next := box.meta
		next.Config = c
		if e = box.commit(new(leveldb.Batch), next); e != nil {
			_ = db.Close()
			return nil, e
		}
	}
	return box, nil
}
func manifestDecodeCanonical(raw []byte, limit int, v any) error {
	if manifestJSONShape(raw, limit) != nil {
		return ErrCorrupt
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(v) != nil {
		return ErrCorrupt
	}
	canonical, e := json.Marshal(v)
	if e != nil || !bytes.Equal(raw, canonical) {
		return ErrCorrupt
	}
	return nil
}
func manifestPendingKey(e ManifestEntry) []byte {
	k := make([]byte, 17)
	k[0] = 'p'
	binary.BigEndian.PutUint64(k[1:9], uint64(e.NextAttemptMS))
	binary.BigEndian.PutUint64(k[9:], e.Sequence)
	return k
}
func manifestDone(e ManifestEntry) bool {
	return (e.BlockState == "not-applicable" || e.BlockState == "canonical" || e.BlockState == "rejected") && (e.AdmissionState == "not-applicable" || e.AdmissionState == "accepted" || e.AdmissionState == "expired" || e.AdmissionState == "rejected")
}

// A fixed total mutable-state charge is split between actual row bytes and
// remaining reservation. Saving authorization or scheduling/state growth must
// consume reserved bytes, not charge the same bytes twice at a full budget.
func manifestFutureReserve(e ManifestEntry) uint64 {
	if manifestDone(e) {
		return 0
	}
	state := struct {
		Block     string `json:"block_state"`
		Admission string `json:"admission_state"`
		Error     string `json:"last_error,omitempty"`
		Next      int64  `json:"next_attempt_ms"`
		Retry     uint32 `json:"retry_count"`
	}{e.BlockState, e.AdmissionState, e.LastError, e.NextAttemptMS, e.RetryCount}
	raw, err := json.Marshal(state)
	// len-1 replaces the two object braces by the one leading field comma.
	if err != nil || len(raw) < 2 || len(raw)-1 > 512 {
		return 1 << 41
	}
	r := uint64(512 - (len(raw) - 1))
	if e.ProspectWin && e.AdmissionState != "accepted" && e.AdmissionState != "expired" && e.AdmissionState != "rejected" {
		const authPrefix = `,"authorization":`
		const receiptPrefix = `,"receipt":`
		r += uint64(gymauth.MaxAuthorizationSize + len(authPrefix) + prospectservice.MaxResponseSize + len(receiptPrefix))
		if e.Authorization != nil {
			auth, err := json.Marshal(e.Authorization)
			if err != nil || len(auth) > gymauth.MaxAuthorizationSize {
				return 1 << 41
			}
			r -= uint64(len(auth) + len(authPrefix))
		}
	}
	return r
}
func (b *ManifestOutbox) state() error {
	if err := manifestProduction(); err != nil {
		return err
	}
	if b.closed {
		return ErrClosed
	}
	if b.poisoned {
		return ErrPoisoned
	}
	return nil
}
func (b *ManifestOutbox) fits(m manifestOutboxMetadata) bool {
	return m.Bytes <= b.config.MaxBytes && m.Reserved <= b.config.MaxBytes-m.Bytes && m.Records <= b.config.MaxRecords
}
func (b *ManifestOutbox) commit(batch *leveldb.Batch, m manifestOutboxMetadata) error {
	raw, err := json.Marshal(m)
	if err != nil || len(raw) > maxMetadataSize || !b.fits(m) {
		return ErrCorrupt
	}
	batch.Put([]byte("meta"), raw)
	if b.writeBatch(batch, &opt.WriteOptions{Sync: true}) != nil {
		b.poisoned = true
		return ErrPoisoned
	}
	b.meta = m
	if b.checkpoint != nil {
		b.checkpoint("commit")
	}
	return nil
}
func sameManifestIdentity(a, b ManifestConfig) bool {
	a.MaxBytes, b.MaxBytes = 0, 0
	a.MaxRecords, b.MaxRecords = 0, 0
	return reflect.DeepEqual(a, b)
}
func (b *ManifestOutbox) audit() error {
	raw, e := b.db.Get([]byte("meta"), nil)
	if e != nil || manifestDecodeCanonical(raw, maxMetadataSize, &b.meta) != nil || b.meta.Application != application || b.meta.Version != Version2 || !sameManifestIdentity(b.meta.Config, b.config) || b.config.MaxRecords < b.meta.Config.MaxRecords || b.config.MaxBytes < b.meta.Config.MaxBytes || b.meta.Records != b.meta.LastSequence || !b.fits(b.meta) {
		return ErrCorrupt
	}
	if canonical, err := ValidateManifestConfig(b.meta.Config); err != nil || canonical != b.meta.Config {
		return ErrCorrupt
	}
	var count, size, indexes, pending, reserved uint64
	var scan *ManifestScan
	var nonce uint64
	haveCursor := false
	it := b.db.NewIterator(nil, nil)
	defer it.Release()
	for it.Next() {
		k, v := it.Key(), it.Value()
		switch {
		case bytes.Equal(k, []byte("meta")):
		case bytes.Equal(k, []byte("scan")):
			var s ManifestScan
			if manifestDecodeCanonical(v, maxScanSize, &s) != nil || s.NextNonce != 0 || validateManifestScan(s, b.config) != nil {
				return ErrCorrupt
			}
			scan = &s
			size += uint64(len(v))
		case bytes.Equal(k, []byte("cursor")):
			if len(v) != 8 {
				return ErrCorrupt
			}
			nonce = binary.BigEndian.Uint64(v)
			if nonce > 1<<32 {
				return ErrCorrupt
			}
			haveCursor = true
			size += 8
		case len(k) == 9 && k[0] == 'e':
			var entry ManifestEntry
			if manifestDecodeCanonical(v, maxEntrySize, &entry) != nil || entry.Sequence != binary.BigEndian.Uint64(k[1:]) || entry.Sequence > b.meta.LastSequence || validateManifestEntry(entry, b.config) != nil {
				return ErrCorrupt
			}
			index, e := b.db.Get(hashKey(entry.WorkHash), nil)
			if e != nil || !bytes.Equal(index, k) {
				return ErrCorrupt
			}
			if !manifestDone(entry) {
				index, e := b.db.Get(manifestPendingKey(entry), nil)
				if e != nil || !bytes.Equal(index, k) {
					return ErrCorrupt
				}
				reserved += manifestFutureReserve(entry)
			}
			count++
			if count > b.meta.Records {
				return ErrCorrupt
			}
			size += uint64(len(v))
		case len(k) == 65 && k[0] == 'h':
			if !manifestEntryKey(v, b.meta.LastSequence) || !manifestHashShape(string(k[1:])) {
				return ErrCorrupt
			}
			r, e := b.db.Get(v, nil)
			var entry ManifestEntry
			if e != nil || len(v) != 9 || v[0] != 'e' || manifestDecodeCanonical(r, maxEntrySize, &entry) != nil || entry.WorkHash != string(k[1:]) || !bytes.Equal(v, seqKey(entry.Sequence)) {
				return ErrCorrupt
			}
			indexes++
			if indexes > b.meta.Records {
				return ErrCorrupt
			}
		case len(k) == 17 && k[0] == 'p':
			if !manifestEntryKey(v, b.meta.LastSequence) || binary.BigEndian.Uint64(k[1:9]) > math.MaxInt64 || !bytes.Equal(k[9:], v[1:]) {
				return ErrCorrupt
			}
			r, e := b.db.Get(v, nil)
			var entry ManifestEntry
			if e != nil || manifestDecodeCanonical(r, maxEntrySize, &entry) != nil || manifestDone(entry) || !bytes.Equal(v, seqKey(entry.Sequence)) || !bytes.Equal(k, manifestPendingKey(entry)) {
				return ErrCorrupt
			}
			pending++
			if pending > b.meta.Records {
				return ErrCorrupt
			}
		default:
			return ErrCorrupt
		}
	}
	if it.Error() != nil || (scan != nil) != haveCursor || count != b.meta.Records || indexes != count || size != b.meta.Bytes || pending != b.meta.Pending || reserved != b.meta.Reserved || !b.fits(b.meta) {
		return ErrCorrupt
	}
	if scan != nil {
		scan.NextNonce = nonce
		b.cachedScan = scan
		b.cachedEntryBytes = manifestReservationEntryBytes(*scan)
	}
	return nil
}
func (b *ManifestOutbox) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return b.closeErr
	}
	if b.running {
		return ErrInUse
	}
	b.closed = true
	b.closeErr = b.db.Close()
	return b.closeErr
}

// List returns at most limit entries and at most 32 MiB of encoded payload.
// Continue from the last returned Sequence even when len(result)<limit.
func (b *ManifestOutbox) List(after uint64, limit int) ([]ManifestEntry, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.list(after, limit)
}
func (b *ManifestOutbox) list(after uint64, limit int) ([]ManifestEntry, error) {
	if e := b.state(); e != nil {
		return nil, e
	}
	if limit < 1 || limit > 100 {
		return nil, errors.New("page limit must be 1..100")
	}
	result := make([]ManifestEntry, 0)
	if after >= b.meta.LastSequence {
		return result, nil
	}
	it := b.db.NewIterator(util.BytesPrefix([]byte{'e'}), nil)
	defer it.Release()
	expected := after + 1
	size := 2
	byteLimited := false
	for ok := it.Seek(seqKey(expected)); ok && len(result) < limit; ok = it.Next() {
		raw := it.Value()
		if len(result) > 0 && size+1+len(raw) > maxPageBytes {
			byteLimited = true
			break
		}
		var entry ManifestEntry
		if manifestDecodeCanonical(raw, maxEntrySize, &entry) != nil || entry.Sequence != expected || !bytes.Equal(it.Key(), seqKey(expected)) || validateManifestEntry(entry, b.config) != nil {
			return nil, ErrCorrupt
		}
		result = append(result, entry)
		expected++
		size += len(raw)
		if len(result) > 1 {
			size++
		}
	}
	if it.Error() != nil || (!byteLimited && uint64(len(result)) != min(uint64(limit), b.meta.LastSequence-after)) {
		return nil, ErrCorrupt
	}
	return result, nil
}
func (b *ManifestOutbox) pendingCount() uint64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.meta.Pending
}
func (b *ManifestOutbox) nextDue(now int64) (ManifestEntry, bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if e := b.state(); e != nil {
		return ManifestEntry{}, false, e
	}
	if now < 0 {
		return ManifestEntry{}, false, ErrCorrupt
	}
	it := b.db.NewIterator(util.BytesPrefix([]byte{'p'}), nil)
	defer it.Release()
	if !it.First() {
		if it.Error() != nil || b.meta.Pending != 0 {
			return ManifestEntry{}, false, ErrCorrupt
		}
		return ManifestEntry{}, false, nil
	}
	k := it.Key()
	if len(k) != 17 || binary.BigEndian.Uint64(k[1:9]) > math.MaxInt64 || !manifestEntryKey(it.Value(), b.meta.LastSequence) || !bytes.Equal(k[9:], it.Value()[1:]) {
		return ManifestEntry{}, false, ErrCorrupt
	}
	if binary.BigEndian.Uint64(k[1:9]) > uint64(now) {
		return ManifestEntry{}, false, nil
	}
	raw, e := b.db.Get(it.Value(), nil)
	var entry ManifestEntry
	if e != nil || manifestDecodeCanonical(raw, maxEntrySize, &entry) != nil || validateManifestEntry(entry, b.config) != nil || manifestDone(entry) || !bytes.Equal(k, manifestPendingKey(entry)) || !bytes.Equal(it.Value(), seqKey(entry.Sequence)) {
		return ManifestEntry{}, false, ErrCorrupt
	}
	return entry, true, nil
}

// Copy mutable containers only; immutable transaction hex strings are shared.
func cloneManifestScan(s ManifestScan) ManifestScan {
	c := s
	c.Candidate.Proof.MerkleBranch = append([]string{}, s.Candidate.Proof.MerkleBranch...)
	c.Template.Block.Transactions = append([]btcjson.GetBlockTemplateResultTx{}, s.Template.Block.Transactions...)
	for i := range c.Template.Block.Transactions {
		c.Template.Block.Transactions[i].Depends = append([]int64{}, s.Template.Block.Transactions[i].Depends...)
	}
	c.Template.Block.Mutable = append(s.Template.Block.Mutable[:0:0], s.Template.Block.Mutable...)
	c.Template.Block.Capabilities = append(s.Template.Block.Capabilities[:0:0], s.Template.Block.Capabilities...)
	if v := s.Template.Block.CoinbaseValue; v != nil {
		x := *v
		c.Template.Block.CoinbaseValue = &x
	}
	if v := s.Template.Block.CoinbaseAux; v != nil {
		x := *v
		c.Template.Block.CoinbaseAux = &x
	}
	if v := s.Template.Block.SubmitOld; v != nil {
		x := *v
		c.Template.Block.SubmitOld = &x
	}
	return c
}
func (b *ManifestOutbox) scan() (ManifestScan, bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if e := b.state(); e != nil {
		return ManifestScan{}, false, e
	}
	if b.cachedScan == nil {
		return ManifestScan{}, false, nil
	}
	return cloneManifestScan(*b.cachedScan), true, nil
}
func (b *ManifestOutbox) saveScan(s *ManifestScan) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if e := b.state(); e != nil {
		return e
	}
	batch := new(leveldb.Batch)
	m := b.meta
	old, e := b.db.Get([]byte("scan"), nil)
	if e != nil && !errors.Is(e, leveldb.ErrNotFound) {
		return ErrCorrupt
	}
	if e == nil {
		if m.Bytes < uint64(len(old))+8 {
			return ErrCorrupt
		}
		m.Bytes -= uint64(len(old)) + 8
	}
	var cached *ManifestScan
	if s == nil {
		batch.Delete([]byte("scan"))
		batch.Delete([]byte("cursor"))
	} else {
		if !manifestTemplateBounds(s.Template) || !manifestCandidateBounds(s.Candidate) {
			return ErrCorrupt
		}
		ownedInput := cloneManifestScan(*s)
		s = &ownedInput
		if validateManifestScan(*s, b.config) != nil {
			return ErrCorrupt
		}
		owned := cloneManifestScan(*s)
		cached = &owned
		payload := owned
		payload.NextNonce = 0
		raw, _ := json.Marshal(payload)
		if len(raw) > maxScanSize {
			return ErrCapacity
		}
		m.Bytes += uint64(len(raw)) + 8
		batch.Put([]byte("scan"), raw)
		batch.Put([]byte("cursor"), cursor(s.NextNonce))
	}
	if !b.fits(m) {
		return ErrCapacity
	}
	if e := b.commit(batch, m); e != nil {
		return e
	}
	b.cachedScan = cached
	b.cachedEntryBytes = 0
	if cached != nil {
		b.cachedEntryBytes = manifestReservationEntryBytes(*cached)
	}
	return nil
}
func (b *ManifestOutbox) sameScan(s ManifestScan) bool {
	if b.cachedScan == nil || s.NextNonce > 1<<32 || s.NextNonce < b.cachedScan.NextNonce || s.NextNonce > b.cachedScan.NextNonce+1 {
		return false
	}
	a, c := s, *b.cachedScan
	a.NextNonce, c.NextNonce = 0, 0
	return reflect.DeepEqual(a, c)
}
func (b *ManifestOutbox) advance(s ManifestScan) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if e := b.state(); e != nil {
		return e
	}
	if !b.sameScan(s) {
		return ErrCorrupt
	}
	batch := new(leveldb.Batch)
	batch.Put([]byte("cursor"), cursor(s.NextNonce))
	if e := b.commit(batch, b.meta); e != nil {
		return e
	}
	b.cachedScan.NextNonce = s.NextNonce
	return nil
}
func manifestReservationEntryBytes(s ManifestScan) uint64 {
	entry := ManifestEntry{Version: Version2, Sequence: math.MaxUint64, WorkHash: string(bytes.Repeat([]byte{'f'}, 64)), Job: s.Job, Candidate: s.Candidate, BlockWin: true, ProspectWin: true, BlockState: "pending", AdmissionState: "needs-signature", NextAttemptMS: math.MaxInt64, RetryCount: 32}
	raw, _ := json.Marshal(entry)
	return uint64(len(raw))
}

func (b *ManifestOutbox) reserve(s ManifestScan) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if e := b.state(); e != nil {
		return e
	}
	if !b.sameScan(s) {
		return ErrCorrupt
	}
	if b.cachedEntryBytes == 0 || b.cachedEntryBytes > maxEntrySize || b.meta.Records >= b.config.MaxRecords {
		return ErrCapacity
	}
	m := b.meta
	m.Bytes += b.cachedEntryBytes
	m.Reserved += manifestFutureReserve(ManifestEntry{ProspectWin: true, BlockState: "pending", AdmissionState: "needs-signature"})
	m.Records++
	if !b.fits(m) {
		return ErrCapacity
	}
	return nil
}
func (b *ManifestOutbox) found(entry ManifestEntry, s ManifestScan) (ManifestEntry, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if e := b.state(); e != nil {
		return ManifestEntry{}, e
	}
	if !b.sameScan(s) || s.NextNonce == 0 || entry.Job != s.Job || !reflect.DeepEqual(entry.Candidate, withNonce(s.Candidate, uint32(s.NextNonce-1))) {
		return ManifestEntry{}, ErrCorrupt
	}
	batch := new(leveldb.Batch)
	batch.Put([]byte("cursor"), cursor(s.NextNonce))
	if index, e := b.db.Get(hashKey(entry.WorkHash), nil); e == nil {
		if !manifestEntryKey(index, b.meta.LastSequence) {
			return ManifestEntry{}, ErrCorrupt
		}
		raw, re := b.db.Get(index, nil)
		var previous ManifestEntry
		if re != nil || manifestDecodeCanonical(raw, maxEntrySize, &previous) != nil || validateManifestEntry(previous, b.config) != nil || previous.Job != entry.Job || !reflect.DeepEqual(previous.Candidate, entry.Candidate) || previous.BlockWin != entry.BlockWin || previous.ProspectWin != entry.ProspectWin {
			return ManifestEntry{}, ErrCorrupt
		}
		if e := b.commit(batch, b.meta); e != nil {
			return ManifestEntry{}, e
		}
		b.cachedScan.NextNonce = s.NextNonce
		return previous, nil
	} else if !errors.Is(e, leveldb.ErrNotFound) {
		return ManifestEntry{}, ErrCorrupt
	}
	if b.meta.LastSequence == math.MaxUint64 || b.meta.Records >= b.config.MaxRecords {
		return ManifestEntry{}, ErrCapacity
	}
	entry.Sequence = b.meta.LastSequence + 1
	if validateManifestEntry(entry, b.config) != nil {
		return ManifestEntry{}, ErrCorrupt
	}
	raw, _ := json.Marshal(entry)
	if len(raw) > maxEntrySize {
		return ManifestEntry{}, ErrCapacity
	}
	m := b.meta
	m.Bytes += uint64(len(raw))
	m.Records++
	m.LastSequence++
	m.Reserved += manifestFutureReserve(entry)
	if !manifestDone(entry) {
		m.Pending++
		batch.Put(manifestPendingKey(entry), seqKey(entry.Sequence))
	}
	if !b.fits(m) {
		return ManifestEntry{}, ErrCapacity
	}
	batch.Put(seqKey(entry.Sequence), raw)
	batch.Put(hashKey(entry.WorkHash), seqKey(entry.Sequence))
	if e := b.commit(batch, m); e != nil {
		return ManifestEntry{}, e
	}
	b.cachedScan.NextNonce = s.NextNonce
	return entry, nil
}
func manifestImmutable(e ManifestEntry) ManifestEntry {
	e.Authorization = nil
	e.Receipt = nil
	e.BlockState = ""
	e.AdmissionState = ""
	e.LastError = ""
	e.NextAttemptMS = 0
	e.RetryCount = 0
	return e
}
func (b *ManifestOutbox) update(entry ManifestEntry) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if e := b.state(); e != nil {
		return e
	}
	raw, e := b.db.Get(seqKey(entry.Sequence), nil)
	var previous ManifestEntry
	if e != nil || manifestDecodeCanonical(raw, maxEntrySize, &previous) != nil || validateManifestEntry(entry, b.config) != nil || !reflect.DeepEqual(manifestImmutable(previous), manifestImmutable(entry)) {
		return ErrCorrupt
	}
	if !manifestTransition(previous, entry) {
		return ErrCorrupt
	}
	if previous.Authorization != nil && !reflect.DeepEqual(previous.Authorization, entry.Authorization) || previous.Receipt != nil && !reflect.DeepEqual(previous.Receipt, entry.Receipt) {
		return ErrCorrupt
	}
	encoded, _ := json.Marshal(entry)
	if len(encoded) > maxEntrySize {
		return ErrCapacity
	}
	m := b.meta
	if m.Bytes < uint64(len(raw)) || m.Reserved < manifestFutureReserve(previous) {
		return ErrCorrupt
	}
	m.Bytes = m.Bytes - uint64(len(raw)) + uint64(len(encoded))
	m.Reserved = m.Reserved - manifestFutureReserve(previous) + manifestFutureReserve(entry)
	batch := new(leveldb.Batch)
	if !manifestDone(previous) {
		if m.Pending == 0 {
			return ErrCorrupt
		}
		m.Pending--
		batch.Delete(manifestPendingKey(previous))
	}
	if !manifestDone(entry) {
		m.Pending++
		batch.Put(manifestPendingKey(entry), seqKey(entry.Sequence))
	}
	if !b.fits(m) {
		return ErrCapacity
	}
	batch.Put(seqKey(entry.Sequence), encoded)
	return b.commit(batch, m)
}

func manifestTransition(a, b ManifestEntry) bool {
	block := a.BlockState == b.BlockState || (a.BlockState == "pending" && b.BlockState == "submitted") || (a.BlockState == "submitted" && b.BlockState == "canonical")
	admission := a.AdmissionState == b.AdmissionState || (a.AdmissionState == "needs-signature" && b.AdmissionState == "pending") || (a.AdmissionState == "pending" && (b.AdmissionState == "accepted" || b.AdmissionState == "expired" || b.AdmissionState == "rejected"))
	return block && admission && b.RetryCount >= a.RetryCount
}

// Validate raw reciprocal-index values before passing them to LevelDB lookups.
func manifestEntryKey(key []byte, last uint64) bool {
	return len(key) == 9 && key[0] == 'e' && binary.BigEndian.Uint64(key[1:]) > 0 && binary.BigEndian.Uint64(key[1:]) <= last
}
