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
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospectservice"
)

var (
	ErrClosed   = errors.New("worker outbox is closed")
	ErrPoisoned = errors.New("worker outbox write outcome uncertain; close and reopen")
	ErrCorrupt  = errors.New("worker outbox is corrupt")
	ErrCapacity = errors.New("worker outbox capacity reached")
	ErrInUse    = errors.New("worker outbox is in use")
)

const (
	maxEntrySize    = 12 << 20
	maxScanSize     = 32 << 20
	maxPageBytes    = 32 << 20
	maxMetadataSize = 128 << 10
	markerName      = "SLUG-WORKER.json"
	application     = "slug-prospect-worker"
)

type metadata struct {
	Application  string `json:"application"`
	Version      uint32 `json:"version"`
	Config       Config `json:"config"`
	LastSequence uint64 `json:"last_sequence"`
	Records      uint64 `json:"records"`
	Bytes        uint64 `json:"bytes"`
	Pending      uint64 `json:"pending"`
	Reserved     uint64 `json:"reserved"`
}
type marker struct {
	Application string `json:"application"`
	Version     uint32 `json:"version"`
	Config      Config `json:"config"`
}

// Outbox is an explicitly initialized, exclusively locked public journal.
// Logical payload bytes plus Reserved protect every pending authorization and
// receipt; LevelDB/index/compaction overhead is additional. Per-attempt progress
// writes only an eight-byte cursor, never the immutable full block template.
type Outbox struct {
	mu                        sync.Mutex
	db                        *leveldb.DB
	meta                      metadata
	config                    Config
	closed, poisoned, running bool
	writeBatch                func(*leveldb.Batch, *opt.WriteOptions) error
	cachedScan                *Scan
	cachedEntryBytes          uint64
}

func syncDir(path string) error {
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	return errors.Join(f.Sync(), f.Close())
}
func regularDirectory(path string) error {
	i, e := os.Lstat(path)
	if e != nil || !i.IsDir() || i.Mode()&os.ModeSymlink != 0 {
		return ErrCorrupt
	}
	return nil
}
func CreateOutbox(path string, config Config) (*Outbox, error) {
	c, e := validateConfig(config)
	if e != nil {
		return nil, e
	}
	if e = os.Mkdir(path, 0700); e != nil {
		return nil, errors.New("could not create new worker outbox")
	}
	raw, _ := json.Marshal(marker{application, Version1, c})
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
	box := &Outbox{db: db, config: c, meta: metadata{Application: application, Version: Version1, Config: c}, writeBatch: db.Write}
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
func readMarker(path string, c Config) error {
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
	raw, re := io.ReadAll(io.LimitReader(f, maxMetadataSize+1))
	if errors.Join(re, f.Close()) != nil {
		return ErrCorrupt
	}
	var v marker
	if decodeCanonical(raw, maxMetadataSize, &v) != nil || v.Application != application || v.Version != Version1 || !sameIdentity(v.Config, c) || c.MaxRecords < v.Config.MaxRecords || c.MaxBytes < v.Config.MaxBytes {
		return ErrCorrupt
	}
	if canonical, err := validateConfig(v.Config); err != nil || canonical != v.Config {
		return ErrCorrupt
	}
	return nil
}
func OpenOutbox(path string, config Config) (*Outbox, error) {
	c, e := validateConfig(config)
	if e != nil {
		return nil, e
	}
	// Check identity before LevelDB can create a lock/log or missing directory.
	if e = readMarker(path, c); e != nil {
		return nil, e
	}
	db, e := leveldb.OpenFile(filepath.Join(path, "db"), &opt.Options{ErrorIfMissing: true, Strict: opt.StrictAll})
	if e != nil {
		return nil, errors.New("could not open existing worker outbox")
	}
	box := &Outbox{db: db, config: c, writeBatch: db.Write}
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
func decodeCanonical(raw []byte, limit int, v any) error {
	if len(raw) == 0 || len(raw) > limit {
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
func seqKey(n uint64) []byte {
	k := make([]byte, 9)
	k[0] = 'e'
	binary.BigEndian.PutUint64(k[1:], n)
	return k
}
func hashKey(h string) []byte { return []byte("h" + h) }
func pendingKey(e Entry) []byte {
	k := make([]byte, 17)
	k[0] = 'p'
	binary.BigEndian.PutUint64(k[1:9], uint64(e.NextAttemptMS))
	binary.BigEndian.PutUint64(k[9:], e.Sequence)
	return k
}
func cursor(n uint64) []byte { b := make([]byte, 8); binary.BigEndian.PutUint64(b, n); return b }
func done(e Entry) bool {
	return (e.BlockState == "not-applicable" || e.BlockState == "canonical" || e.BlockState == "rejected") && (e.AdmissionState == "not-applicable" || e.AdmissionState == "accepted" || e.AdmissionState == "expired" || e.AdmissionState == "rejected")
}
func futureReserve(e Entry) uint64 {
	if done(e) {
		return 0
	}
	r := uint64(512)
	if e.ProspectWin && e.AdmissionState != "accepted" && e.AdmissionState != "expired" && e.AdmissionState != "rejected" {
		r += gymauth.MaxAuthorizationSize + prospectservice.MaxResponseSize
	}
	return r
}
func (b *Outbox) state() error {
	if b.closed {
		return ErrClosed
	}
	if b.poisoned {
		return ErrPoisoned
	}
	return nil
}
func (b *Outbox) fits(m metadata) bool {
	return m.Bytes <= b.config.MaxBytes && m.Reserved <= b.config.MaxBytes-m.Bytes && m.Records <= b.config.MaxRecords
}
func (b *Outbox) commit(batch *leveldb.Batch, m metadata) error {
	raw, _ := json.Marshal(m)
	batch.Put([]byte("meta"), raw)
	if b.writeBatch(batch, &opt.WriteOptions{Sync: true}) != nil {
		b.poisoned = true
		return ErrPoisoned
	}
	b.meta = m
	return nil
}
func sameIdentity(a, b Config) bool {
	a.MaxBytes, b.MaxBytes = 0, 0
	a.MaxRecords, b.MaxRecords = 0, 0
	return reflect.DeepEqual(a, b)
}
func (b *Outbox) audit() error {
	raw, e := b.db.Get([]byte("meta"), nil)
	if e != nil || decodeCanonical(raw, maxMetadataSize, &b.meta) != nil || b.meta.Application != application || b.meta.Version != Version1 || !sameIdentity(b.meta.Config, b.config) || b.config.MaxRecords < b.meta.Config.MaxRecords || b.config.MaxBytes < b.meta.Config.MaxBytes || b.meta.Records != b.meta.LastSequence {
		return ErrCorrupt
	}
	if canonical, err := validateConfig(b.meta.Config); err != nil || canonical != b.meta.Config {
		return ErrCorrupt
	}
	var count, size, indexes, pending, reserved uint64
	var scan *Scan
	var nonce uint64
	haveCursor := false
	it := b.db.NewIterator(nil, nil)
	defer it.Release()
	for it.Next() {
		k, v := it.Key(), it.Value()
		switch {
		case bytes.Equal(k, []byte("meta")):
		case bytes.Equal(k, []byte("scan")):
			var s Scan
			if decodeCanonical(v, maxScanSize, &s) != nil || s.NextNonce != 0 || validateScan(s, b.config) != nil {
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
			var entry Entry
			if decodeCanonical(v, maxEntrySize, &entry) != nil || entry.Sequence != binary.BigEndian.Uint64(k[1:]) || entry.Sequence > b.meta.LastSequence || validateEntry(entry, b.config) != nil {
				return ErrCorrupt
			}
			index, e := b.db.Get(hashKey(entry.WorkHash), nil)
			if e != nil || !bytes.Equal(index, k) {
				return ErrCorrupt
			}
			if !done(entry) {
				index, e := b.db.Get(pendingKey(entry), nil)
				if e != nil || !bytes.Equal(index, k) {
					return ErrCorrupt
				}
				reserved += futureReserve(entry)
			}
			count++
			size += uint64(len(v))
		case len(k) == 65 && k[0] == 'h':
			r, e := b.db.Get(v, nil)
			var entry Entry
			if e != nil || len(v) != 9 || v[0] != 'e' || decodeCanonical(r, maxEntrySize, &entry) != nil || entry.WorkHash != string(k[1:]) || !bytes.Equal(v, seqKey(entry.Sequence)) {
				return ErrCorrupt
			}
			indexes++
		case len(k) == 17 && k[0] == 'p':
			r, e := b.db.Get(v, nil)
			var entry Entry
			if e != nil || decodeCanonical(r, maxEntrySize, &entry) != nil || done(entry) || !bytes.Equal(v, seqKey(entry.Sequence)) || !bytes.Equal(k, pendingKey(entry)) {
				return ErrCorrupt
			}
			pending++
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
		b.cachedEntryBytes = reservationEntryBytes(*scan)
	}
	return nil
}
func (b *Outbox) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil
	}
	if b.running {
		return ErrInUse
	}
	b.closed = true
	return b.db.Close()
}

// List returns at most limit entries and at most32MiB of encoded payload.
// Continue from the last returned Sequence even when len(result)<limit.
func (b *Outbox) List(after uint64, limit int) ([]Entry, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.list(after, limit)
}
func (b *Outbox) list(after uint64, limit int) ([]Entry, error) {
	if e := b.state(); e != nil {
		return nil, e
	}
	if limit < 1 || limit > 100 {
		return nil, errors.New("page limit must be1..100")
	}
	result := make([]Entry, 0)
	if after >= b.meta.LastSequence {
		return result, nil
	}
	it := b.db.NewIterator(util.BytesPrefix([]byte{'e'}), nil)
	defer it.Release()
	expected := after + 1
	size := 0
	byteLimited := false
	for ok := it.Seek(seqKey(expected)); ok && len(result) < limit; ok = it.Next() {
		raw := it.Value()
		if len(result) > 0 && size+len(raw) > maxPageBytes {
			byteLimited = true
			break
		}
		var entry Entry
		if decodeCanonical(raw, maxEntrySize, &entry) != nil || entry.Sequence != expected || !bytes.Equal(it.Key(), seqKey(expected)) || validateEntry(entry, b.config) != nil {
			return nil, ErrCorrupt
		}
		result = append(result, entry)
		expected++
		size += len(raw)
	}
	if it.Error() != nil || (!byteLimited && uint64(len(result)) != min(uint64(limit), b.meta.LastSequence-after)) {
		return nil, ErrCorrupt
	}
	return result, nil
}
func (b *Outbox) pendingCount() uint64 { b.mu.Lock(); defer b.mu.Unlock(); return b.meta.Pending }
func (b *Outbox) nextDue(now int64) (Entry, bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if e := b.state(); e != nil {
		return Entry{}, false, e
	}
	if now < 0 {
		return Entry{}, false, ErrCorrupt
	}
	it := b.db.NewIterator(util.BytesPrefix([]byte{'p'}), nil)
	defer it.Release()
	if !it.First() {
		if it.Error() != nil || b.meta.Pending != 0 {
			return Entry{}, false, ErrCorrupt
		}
		return Entry{}, false, nil
	}
	k := it.Key()
	if len(k) != 17 {
		return Entry{}, false, ErrCorrupt
	}
	if binary.BigEndian.Uint64(k[1:9]) > uint64(now) {
		return Entry{}, false, nil
	}
	raw, e := b.db.Get(it.Value(), nil)
	var entry Entry
	if e != nil || decodeCanonical(raw, maxEntrySize, &entry) != nil || validateEntry(entry, b.config) != nil || done(entry) || !bytes.Equal(k, pendingKey(entry)) || !bytes.Equal(it.Value(), seqKey(entry.Sequence)) {
		return Entry{}, false, ErrCorrupt
	}
	return entry, true, nil
}

// Copy mutable containers only; immutable transaction hex strings are shared.
func cloneScan(s Scan) Scan {
	c := s
	c.Candidate.Proof.MerkleBranch = append(s.Candidate.Proof.MerkleBranch[:0:0], s.Candidate.Proof.MerkleBranch...)
	c.Template.Block.Transactions = append(s.Template.Block.Transactions[:0:0], s.Template.Block.Transactions...)
	for i := range c.Template.Block.Transactions {
		c.Template.Block.Transactions[i].Depends = append(s.Template.Block.Transactions[i].Depends[:0:0], s.Template.Block.Transactions[i].Depends...)
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
func (b *Outbox) scan() (Scan, bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if e := b.state(); e != nil {
		return Scan{}, false, e
	}
	if b.cachedScan == nil {
		return Scan{}, false, nil
	}
	return cloneScan(*b.cachedScan), true, nil
}
func (b *Outbox) saveScan(s *Scan) error {
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
	var cached *Scan
	if s == nil {
		batch.Delete([]byte("scan"))
		batch.Delete([]byte("cursor"))
	} else {
		if validateScan(*s, b.config) != nil {
			return ErrCorrupt
		}
		owned := cloneScan(*s)
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
		b.cachedEntryBytes = reservationEntryBytes(*cached)
	}
	return nil
}
func (b *Outbox) sameScan(s Scan) bool {
	if b.cachedScan == nil || s.NextNonce > 1<<32 || s.NextNonce < b.cachedScan.NextNonce || s.NextNonce > b.cachedScan.NextNonce+1 {
		return false
	}
	a, c := s, *b.cachedScan
	a.NextNonce, c.NextNonce = 0, 0
	return reflect.DeepEqual(a, c)
}
func (b *Outbox) advance(s Scan) error {
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
func reservationEntryBytes(s Scan) uint64 {
	entry := Entry{Version: Version1, Sequence: math.MaxUint64, WorkHash: string(bytes.Repeat([]byte{'f'}, 64)), Job: s.Job, Candidate: s.Candidate, BlockWin: true, ProspectWin: true, BlockState: "pending", AdmissionState: "needs-signature", NextAttemptMS: math.MaxInt64, RetryCount: math.MaxUint32}
	raw, _ := json.Marshal(entry)
	return uint64(len(raw))
}

func (b *Outbox) reserve(s Scan) error {
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
	m.Reserved += futureReserve(Entry{ProspectWin: true, BlockState: "pending", AdmissionState: "needs-signature"})
	m.Records++
	if !b.fits(m) {
		return ErrCapacity
	}
	return nil
}
func (b *Outbox) found(entry Entry, s Scan) (Entry, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if e := b.state(); e != nil {
		return Entry{}, e
	}
	if !b.sameScan(s) || s.NextNonce == 0 || entry.Job != s.Job || !reflect.DeepEqual(entry.Candidate, withNonce(s.Candidate, uint32(s.NextNonce-1))) {
		return Entry{}, ErrCorrupt
	}
	batch := new(leveldb.Batch)
	batch.Put([]byte("cursor"), cursor(s.NextNonce))
	if index, e := b.db.Get(hashKey(entry.WorkHash), nil); e == nil {
		raw, re := b.db.Get(index, nil)
		var previous Entry
		if re != nil || decodeCanonical(raw, maxEntrySize, &previous) != nil || validateEntry(previous, b.config) != nil || previous.Job != entry.Job || !reflect.DeepEqual(previous.Candidate, entry.Candidate) || previous.BlockWin != entry.BlockWin || previous.ProspectWin != entry.ProspectWin {
			return Entry{}, ErrCorrupt
		}
		if e := b.commit(batch, b.meta); e != nil {
			return Entry{}, e
		}
		b.cachedScan.NextNonce = s.NextNonce
		return previous, nil
	} else if !errors.Is(e, leveldb.ErrNotFound) {
		return Entry{}, ErrCorrupt
	}
	if b.meta.LastSequence == math.MaxUint64 || b.meta.Records >= b.config.MaxRecords {
		return Entry{}, ErrCapacity
	}
	entry.Sequence = b.meta.LastSequence + 1
	if validateEntry(entry, b.config) != nil {
		return Entry{}, ErrCorrupt
	}
	raw, _ := json.Marshal(entry)
	if len(raw) > maxEntrySize {
		return Entry{}, ErrCapacity
	}
	m := b.meta
	m.Bytes += uint64(len(raw))
	m.Records++
	m.LastSequence++
	m.Reserved += futureReserve(entry)
	if !done(entry) {
		m.Pending++
		batch.Put(pendingKey(entry), seqKey(entry.Sequence))
	}
	if !b.fits(m) {
		return Entry{}, ErrCapacity
	}
	batch.Put(seqKey(entry.Sequence), raw)
	batch.Put(hashKey(entry.WorkHash), seqKey(entry.Sequence))
	if e := b.commit(batch, m); e != nil {
		return Entry{}, e
	}
	b.cachedScan.NextNonce = s.NextNonce
	return entry, nil
}
func immutable(e Entry) Entry {
	e.Authorization = nil
	e.Receipt = nil
	e.BlockState = ""
	e.AdmissionState = ""
	e.LastError = ""
	e.NextAttemptMS = 0
	e.RetryCount = 0
	return e
}
func (b *Outbox) update(entry Entry) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if e := b.state(); e != nil {
		return e
	}
	raw, e := b.db.Get(seqKey(entry.Sequence), nil)
	var previous Entry
	if e != nil || decodeCanonical(raw, maxEntrySize, &previous) != nil || validateEntry(entry, b.config) != nil || !reflect.DeepEqual(immutable(previous), immutable(entry)) {
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
	if m.Bytes < uint64(len(raw)) || m.Reserved < futureReserve(previous) {
		return ErrCorrupt
	}
	m.Bytes = m.Bytes - uint64(len(raw)) + uint64(len(encoded))
	m.Reserved = m.Reserved - futureReserve(previous) + futureReserve(entry)
	batch := new(leveldb.Batch)
	if !done(previous) {
		if m.Pending == 0 {
			return ErrCorrupt
		}
		m.Pending--
		batch.Delete(pendingKey(previous))
	}
	if !done(entry) {
		m.Pending++
		batch.Put(pendingKey(entry), seqKey(entry.Sequence))
	}
	if !b.fits(m) {
		return ErrCapacity
	}
	batch.Put(seqKey(entry.Sequence), encoded)
	return b.commit(batch, m)
}
