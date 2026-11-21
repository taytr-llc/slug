// Copyright (c) 2026 The SLUG developers. ISC license.
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
	"unicode/utf8"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/util"
	"github.com/taytr-llc/slug/prospectwork"
)

const (
	poolApplication  = "slug-pool-worker"
	poolMarkerName   = "SLUG-POOL-WORKER.json"
	poolMaxEntrySize = 40 << 20
	poolMaxScanSize  = prospectwork.MaxTemplateSize + (64 << 10)
	poolMaxPageBytes = poolMaxEntrySize + 2
	// Public auth (4KiB), manifest receipt (72KiB), pool receipt (8KiB),
	// optional-field prefixes and every mutable state/scheduling field fit.
	poolMutableAllowance        = 96 << 10
	poolInvalidReserve   uint64 = 1 << 40
)

type poolOutboxMetadata struct {
	Application  string     `json:"application"`
	Version      uint32     `json:"version"`
	Config       PoolConfig `json:"config"`
	LastSequence uint64     `json:"last_sequence"`
	Records      uint64     `json:"records"`
	Bytes        uint64     `json:"bytes"`
	Pending      uint64     `json:"pending"`
	Reserved     uint64     `json:"reserved"`
	IssuePending bool       `json:"-"`
}
type poolOutboxMarker struct {
	Application string     `json:"application"`
	Version     uint32     `json:"version"`
	Config      PoolConfig `json:"config"`
}

// PoolOutbox is a separate explicit journal, never a migration of a solo or
// manifest outbox. Bytes count logical payload; Reserved keeps all pending
// outcomes recoverable. LevelDB/index/compaction overhead is additional.
type PoolOutbox struct {
	mu                        sync.Mutex
	db                        *leveldb.DB
	meta                      poolOutboxMetadata
	config                    PoolConfig
	closed, poisoned, running bool
	closeErr                  error
	closeDB                   func() error
	checkpoint                func(string)
	writeBatch                func(*leveldb.Batch, *opt.WriteOptions) error
	cachedScan                *PoolScan
	cachedEntryBytes          uint64
	issueAttempt              *poolDailyIssueAttempt
}

func samePoolIdentity(a, b PoolConfig) bool {
	a.MaxBytes, b.MaxBytes = 0, 0
	a.MaxRecords, b.MaxRecords = 0, 0
	return a == b
}

// Bound container allocation before typed JSON decoding. Null is allowed only
// where the concrete type's canonical JSON retains it, preserving nil slices.
func poolDecodeCanonical(raw []byte, limit int, out any) error {
	if len(raw) == 0 || len(raw) > limit || raw[0] != '{' || !utf8.Valid(raw) {
		return ErrCorrupt
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var visit func(int, string) error
	visit = func(depth int, field string) error {
		if depth > 12 {
			return ErrCorrupt
		}
		token, e := d.Token()
		if e != nil {
			return ErrCorrupt
		}
		delim, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := make(map[string]bool)
			for d.More() {
				token, e := d.Token()
				key, ok := token.(string)
				if e != nil || !ok || seen[key] || len(seen) >= 64 {
					return ErrCorrupt
				}
				seen[key] = true
				if visit(depth+1, key) != nil {
					return ErrCorrupt
				}
			}
			token, e = d.Token()
			if e != nil || token != json.Delim('}') {
				return ErrCorrupt
			}
		case '[':
			maximum := 0
			switch field {
			case "merkle_branch":
				maximum = 20
			case "transactions", "depends":
				maximum = 16666
			case "mutable", "capabilities":
				maximum = 1024
			default:
				return ErrCorrupt
			}
			count := 0
			for d.More() {
				count++
				if count > maximum || visit(depth+1, "") != nil {
					return ErrCorrupt
				}
			}
			token, e = d.Token()
			if e != nil || token != json.Delim(']') {
				return ErrCorrupt
			}
		default:
			return ErrCorrupt
		}
		return nil
	}
	if visit(1, "") != nil {
		return ErrCorrupt
	}
	if _, e := d.Token(); e != io.EOF {
		return ErrCorrupt
	}
	d = json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(out) != nil {
		return ErrCorrupt
	}
	canonical, e := json.Marshal(out)
	if e != nil || !bytes.Equal(raw, canonical) {
		return ErrCorrupt
	}
	return nil
}
func clonePoolScan(s PoolScan) (PoolScan, error) {
	raw, e := json.Marshal(s)
	var result PoolScan
	if e != nil || poolDecodeCanonical(raw, poolMaxScanSize, &result) != nil {
		return result, ErrCorrupt
	}
	return result, nil
}
func clonePoolEntry(e PoolEntry) (PoolEntry, error) {
	raw, err := json.Marshal(e)
	var result PoolEntry
	if err != nil || poolDecodeCanonical(raw, poolMaxEntrySize, &result) != nil {
		return result, ErrCorrupt
	}
	return result, nil
}

func CreatePoolOutbox(path string, config PoolConfig) (*PoolOutbox, error) {
	return createPoolOutbox(path, config, false)
}
func createPoolOutbox(path string, config PoolConfig, daily bool) (*PoolOutbox, error) {
	if e := manifestProduction(); e != nil {
		return nil, e
	}
	c, e := validateSelectedPoolConfig(config, daily)
	if e != nil {
		return nil, e
	}
	if os.Mkdir(path, 0700) != nil {
		return nil, errors.New("could not create new pool outbox")
	}
	raw, _ := json.Marshal(poolOutboxMarker{poolApplication, c.Version, c})
	if len(raw) > maxMetadataSize {
		return nil, ErrCorrupt
	}
	f, e := os.OpenFile(filepath.Join(path, poolMarkerName), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return nil, e
	}
	_, writeErr := f.Write(raw)
	if e = errors.Join(writeErr, f.Sync(), f.Close()); e != nil {
		return nil, e
	}
	dbPath := filepath.Join(path, "db")
	if e = os.Mkdir(dbPath, 0700); e != nil {
		return nil, e
	}
	db, e := leveldb.OpenFile(dbPath, &opt.Options{ErrorIfExist: true, Strict: opt.StrictAll})
	if e != nil {
		return nil, errors.New("could not initialize pool outbox")
	}
	b := &PoolOutbox{db: db, config: c, meta: poolOutboxMetadata{Application: poolApplication, Version: c.Version, Config: c}, writeBatch: db.Write}
	if e = b.commit(new(leveldb.Batch), b.meta); e != nil {
		return poolOutboxConstructorFailure(b, e, daily)
	}
	if e = errors.Join(syncDir(dbPath), syncDir(path), syncDir(filepath.Dir(filepath.Clean(path)))); e != nil {
		return poolOutboxConstructorFailure(b, errors.New("could not synchronize pool outbox directory"), daily)
	}
	return b, nil
}
func readPoolMarker(path string, c PoolConfig) error {
	if regularDirectory(path) != nil || regularDirectory(filepath.Join(path, "db")) != nil {
		return ErrCorrupt
	}
	name := filepath.Join(path, poolMarkerName)
	i, e := os.Lstat(name)
	if e != nil || !i.Mode().IsRegular() || i.Size() > maxMetadataSize {
		return ErrCorrupt
	}
	f, e := os.Open(name)
	if e != nil {
		return ErrCorrupt
	}
	opened, e := f.Stat()
	if e != nil || !opened.Mode().IsRegular() || opened.Size() > maxMetadataSize || !os.SameFile(i, opened) {
		_ = f.Close()
		return ErrCorrupt
	}
	raw, re := io.ReadAll(io.LimitReader(f, maxMetadataSize+1))
	if errors.Join(re, f.Close()) != nil {
		return ErrCorrupt
	}
	var m poolOutboxMarker
	if poolDecodeCanonical(raw, maxMetadataSize, &m) != nil || m.Application != poolApplication || m.Version != c.Version || !poolReopenConfig(m.Config, c) {
		return ErrCorrupt
	}
	if valid, e := validateSelectedPoolConfig(m.Config, c.Version == PoolVersion2); e != nil || valid != m.Config {
		return ErrCorrupt
	}
	return nil
}
func OpenPoolOutbox(path string, config PoolConfig) (*PoolOutbox, error) {
	return openSelectedPoolOutbox(path, config, false, false)
}

// OpenPoolOutboxAudit requires the exact established configuration and audits
// all rows read-only. It cannot publish a capacity/configuration change.
func OpenPoolOutboxAudit(path string, config PoolConfig) (*PoolOutbox, error) {
	return openSelectedPoolOutbox(path, config, true, false)
}
func openPoolOutboxAudit(path string, config PoolConfig, readOnly bool) (*PoolOutbox, error) {
	return openSelectedPoolOutbox(path, config, readOnly, false)
}
func openSelectedPoolOutbox(path string, config PoolConfig, readOnly, daily bool) (*PoolOutbox, error) {
	if e := manifestProduction(); e != nil {
		return nil, e
	}
	c, e := validateSelectedPoolConfig(config, daily)
	if e != nil {
		return nil, e
	}
	if e = readPoolMarker(path, c); e != nil {
		return nil, e
	}
	db, e := leveldb.OpenFile(filepath.Join(path, "db"), &opt.Options{ErrorIfMissing: true, Strict: opt.StrictAll, ReadOnly: readOnly})
	if e != nil {
		return nil, errors.New("could not open existing pool outbox")
	}
	b := &PoolOutbox{db: db, config: c, writeBatch: db.Write}
	if e = b.audit(); e != nil {
		return poolOutboxConstructorFailure(b, e, daily)
	}
	if b.meta.Config != c {
		if readOnly {
			return poolOutboxConstructorFailure(b, ErrCorrupt, daily)
		}
		next := b.meta
		next.Config = c
		if e = b.commit(new(leveldb.Batch), next); e != nil {
			return poolOutboxConstructorFailure(b, e, daily)
		}
	}
	return b, nil
}
func (b *PoolOutbox) state() error {
	if e := manifestProduction(); e != nil {
		return e
	}
	if b == nil || b.db == nil || b.closed {
		return ErrClosed
	}
	if b.poisoned {
		return ErrPoisoned
	}
	return nil
}
func (b *PoolOutbox) fits(m poolOutboxMetadata) bool {
	return m.Bytes <= b.config.MaxBytes && m.Reserved <= b.config.MaxBytes-m.Bytes && m.Records <= b.config.MaxRecords
}
func (b *PoolOutbox) commit(batch *leveldb.Batch, m poolOutboxMetadata) error {
	if b.poisoned {
		return ErrPoisoned
	}
	raw, e := encodePoolMetadata(m)
	if e != nil || len(raw) > maxMetadataSize || !b.fits(m) {
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
func poolPendingKey(e PoolEntry) []byte {
	k := make([]byte, 17)
	k[0] = 'p'
	binary.BigEndian.PutUint64(k[1:9], uint64(e.NextAttemptMS))
	binary.BigEndian.PutUint64(k[9:], e.Sequence)
	return k
}
func poolBudget(e PoolEntry) uint64 {
	raw, err := json.Marshal(poolImmutable(e))
	if err != nil || len(raw) > poolMaxEntrySize-poolMutableAllowance {
		return poolInvalidReserve
	}
	return uint64(len(raw)) + poolMutableAllowance
}
func poolFutureReserve(e PoolEntry) uint64 {
	raw, err := json.Marshal(e)
	budget := poolBudget(e)
	if err != nil || budget == poolInvalidReserve || uint64(len(raw)) > budget {
		return poolInvalidReserve
	}
	if poolDone(e) {
		return 0
	}
	return budget - uint64(len(raw))
}

func (b *PoolOutbox) audit() error {
	raw, e := b.db.Get([]byte("meta"), nil)
	if e != nil || decodePoolMetadata(raw, b.config.Version, &b.meta) != nil || b.meta.Application != poolApplication || b.meta.Version != b.config.Version || !poolReopenConfig(b.meta.Config, b.config) || b.meta.Records != b.meta.LastSequence || b.meta.Pending > b.meta.Records || !b.fits(b.meta) {
		return ErrCorrupt
	}
	if c, e := validateSelectedPoolConfig(b.meta.Config, b.config.Version == PoolVersion2); e != nil || c != b.meta.Config {
		return ErrCorrupt
	}
	var count, size, indexes, pending, reserved uint64
	var scan *PoolScan
	var nonce uint64
	haveCursor := false
	it := b.db.NewIterator(nil, nil)
	defer it.Release()
	for it.Next() {
		k, v := it.Key(), it.Value()
		switch {
		case bytes.Equal(k, []byte("meta")):
		case bytes.Equal(k, []byte("scan")):
			var s PoolScan
			if poolDecodeCanonical(v, poolMaxScanSize, &s) != nil || s.NextNonce != 0 || validatePoolScan(s, b.config) != nil {
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
			var entry PoolEntry
			if poolDecodeCanonical(v, poolMaxEntrySize, &entry) != nil || entry.Sequence != binary.BigEndian.Uint64(k[1:]) || entry.Sequence > b.meta.LastSequence || validatePoolEntry(entry, b.config) != nil {
				return ErrCorrupt
			}
			index, e := b.db.Get(hashKey(entry.DuplicateKey), nil)
			if e != nil || !bytes.Equal(index, k) {
				return ErrCorrupt
			}
			r := poolFutureReserve(entry)
			if r == poolInvalidReserve {
				return ErrCorrupt
			}
			reserved += r
			if !poolDone(entry) {
				index, e := b.db.Get(poolPendingKey(entry), nil)
				if e != nil || !bytes.Equal(index, k) {
					return ErrCorrupt
				}
			}
			count++
			size += uint64(len(v))
			if count > b.meta.Records {
				return ErrCorrupt
			}
		case len(k) == 65 && k[0] == 'h':
			if !manifestEntryKey(v, b.meta.LastSequence) || !manifestHashShape(string(k[1:])) {
				return ErrCorrupt
			}
			r, e := b.db.Get(v, nil)
			var entry PoolEntry
			if e != nil || poolDecodeCanonical(r, poolMaxEntrySize, &entry) != nil || entry.DuplicateKey != string(k[1:]) || !bytes.Equal(v, seqKey(entry.Sequence)) {
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
			var entry PoolEntry
			if e != nil || poolDecodeCanonical(r, poolMaxEntrySize, &entry) != nil || poolDone(entry) || !bytes.Equal(v, seqKey(entry.Sequence)) || !bytes.Equal(k, poolPendingKey(entry)) {
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
	if it.Error() != nil || b.meta.IssuePending && (scan != nil || haveCursor) || (scan != nil) != haveCursor || count != b.meta.Records || indexes != count || size != b.meta.Bytes || pending != b.meta.Pending || reserved != b.meta.Reserved || !b.fits(b.meta) {
		return ErrCorrupt
	}
	if scan != nil {
		scan.NextNonce = nonce
		b.cachedScan = scan
		b.cachedEntryBytes = poolReservationEntryBytes(*scan, b.config)
		if b.cachedEntryBytes == poolInvalidReserve {
			return ErrCorrupt
		}
	}
	return nil
}

func (b *PoolOutbox) Close() error {
	if b == nil {
		return ErrClosed
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return b.closeErr
	}
	if b.running {
		return ErrInUse
	}
	b.closed = true
	if b.db != nil {
		if b.closeDB != nil {
			b.closeErr = b.closeDB()
		} else {
			b.closeErr = b.db.Close()
		}
	}
	return b.closeErr
}

// List returns owned rows bounded by count and complete encoded page bytes.
// Continue from the last returned sequence when the byte bound shortens a page.
func (b *PoolOutbox) List(after uint64, limit int) ([]PoolEntry, error) {
	if b == nil {
		return nil, ErrClosed
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.list(after, limit)
}
func (b *PoolOutbox) list(after uint64, limit int) ([]PoolEntry, error) {
	if e := b.state(); e != nil {
		return nil, e
	}
	if limit < 1 || limit > 100 {
		return nil, errors.New("pool page limit must be 1..100")
	}
	result := []PoolEntry{}
	if after >= b.meta.LastSequence {
		return result, nil
	}
	it := b.db.NewIterator(util.BytesPrefix([]byte{'e'}), nil)
	defer it.Release()
	expected := after + 1
	size := 2
	limited := false
	for ok := it.Seek(seqKey(expected)); ok && len(result) < limit; ok = it.Next() {
		raw := it.Value()
		if len(result) > 0 && size+1+len(raw) > poolMaxPageBytes {
			limited = true
			break
		}
		var entry PoolEntry
		if poolDecodeCanonical(raw, poolMaxEntrySize, &entry) != nil || entry.Sequence != expected || !bytes.Equal(it.Key(), seqKey(expected)) || validatePoolEntry(entry, b.config) != nil || poolFutureReserve(entry) == poolInvalidReserve {
			return nil, ErrCorrupt
		}
		result = append(result, entry)
		expected++
		size += len(raw)
		if len(result) > 1 {
			size++
		}
	}
	if it.Error() != nil || !limited && uint64(len(result)) != min(uint64(limit), b.meta.LastSequence-after) {
		return nil, ErrCorrupt
	}
	return result, nil
}
func (b *PoolOutbox) pendingCount() uint64 { b.mu.Lock(); defer b.mu.Unlock(); return b.meta.Pending }
func (b *PoolOutbox) nextDue(now int64) (PoolEntry, bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if e := b.state(); e != nil {
		return PoolEntry{}, false, e
	}
	if now < 0 {
		return PoolEntry{}, false, ErrCorrupt
	}
	it := b.db.NewIterator(util.BytesPrefix([]byte{'p'}), nil)
	defer it.Release()
	if !it.First() {
		if it.Error() != nil || b.meta.Pending != 0 {
			return PoolEntry{}, false, ErrCorrupt
		}
		return PoolEntry{}, false, nil
	}
	k, v := it.Key(), it.Value()
	if len(k) != 17 || binary.BigEndian.Uint64(k[1:9]) > math.MaxInt64 || !manifestEntryKey(v, b.meta.LastSequence) || !bytes.Equal(k[9:], v[1:]) {
		return PoolEntry{}, false, ErrCorrupt
	}
	if binary.BigEndian.Uint64(k[1:9]) > uint64(now) {
		return PoolEntry{}, false, nil
	}
	raw, e := b.db.Get(v, nil)
	var entry PoolEntry
	if e != nil || poolDecodeCanonical(raw, poolMaxEntrySize, &entry) != nil || validatePoolEntry(entry, b.config) != nil || poolFutureReserve(entry) == poolInvalidReserve || poolDone(entry) || !bytes.Equal(k, poolPendingKey(entry)) || !bytes.Equal(v, seqKey(entry.Sequence)) {
		return PoolEntry{}, false, ErrCorrupt
	}
	return entry, true, nil
}

func (b *PoolOutbox) scan() (PoolScan, bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if e := b.state(); e != nil {
		return PoolScan{}, false, e
	}
	if b.cachedScan == nil {
		return PoolScan{}, false, nil
	}
	s, e := clonePoolScan(*b.cachedScan)
	return s, e == nil, e
}
func (b *PoolOutbox) saveScan(s *PoolScan) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if e := b.state(); e != nil {
		return e
	}
	if b.meta.IssuePending {
		return errPoolIssueHeld
	}
	prepared, e := preparePoolScan(s, b.config)
	if e != nil {
		return e
	}
	return b.savePreparedScan(prepared, b.meta)
}

// Caller owns the original outbox mutex. Daily response validation and copying
// have already completed outside it; the remaining work is the exact sync commit.
func (b *PoolOutbox) savePreparedScan(p *poolPreparedScan, m poolOutboxMetadata) error {
	batch := new(leveldb.Batch)
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
	if p == nil {
		batch.Delete([]byte("scan"))
		batch.Delete([]byte("cursor"))
	} else {
		m.Bytes += uint64(len(p.raw)) + 8
		batch.Put([]byte("scan"), p.raw)
		batch.Put([]byte("cursor"), cursor(p.scan.NextNonce))
	}
	if !b.fits(m) {
		return ErrCapacity
	}
	if e = b.commit(batch, m); e != nil {
		return e
	}
	b.cachedScan, b.cachedEntryBytes = nil, 0
	if p != nil {
		b.cachedScan, b.cachedEntryBytes = &p.scan, p.reservation
	}
	return nil
}
func (b *PoolOutbox) sameScan(s PoolScan) bool {
	if b.cachedScan == nil || s.NextNonce > 1<<32 || s.NextNonce < b.cachedScan.NextNonce || s.NextNonce > b.cachedScan.NextNonce+1 {
		return false
	}
	a, c := s, *b.cachedScan
	a.NextNonce, c.NextNonce = 0, 0
	return reflect.DeepEqual(a, c)
}
func (b *PoolOutbox) advance(s PoolScan) error {
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
func poolReservationEntryBytes(s PoolScan, c PoolConfig) uint64 {
	w, e := preparePoolWork(c, s.Work)
	if e != nil {
		return poolInvalidReserve
	}
	candidate := withNonce(w.Candidate(), math.MaxUint32)
	check := prospectwork.ShareCheck{Nonce: math.MaxUint32, WorkHash: string(bytes.Repeat([]byte{'f'}, 64)), DuplicateKey: string(bytes.Repeat([]byte{'f'}, 64)), Pool: true, Prospect: true, Block: true, Candidate: &candidate}
	entry := poolEntryForCheck(s, check)
	entry.Sequence = math.MaxUint64
	entry.Fighter.Sequence = math.MaxUint64
	// False boolean encodings are one byte longer than true. All independent
	// qualification combinations fit this worst-case base plus four bytes.
	budget := poolBudget(entry)
	if budget == poolInvalidReserve {
		return budget
	}
	return budget + 4
}
func (b *PoolOutbox) reserve(s PoolScan) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if e := b.state(); e != nil {
		return e
	}
	if !b.sameScan(s) {
		return ErrCorrupt
	}
	if b.cachedScan.NextNonce >= 1<<32 || b.cachedEntryBytes == 0 || b.cachedEntryBytes == poolInvalidReserve || b.cachedEntryBytes > poolMaxEntrySize || b.meta.Records >= b.config.MaxRecords {
		return ErrCapacity
	}
	m := b.meta
	m.Bytes += b.cachedEntryBytes
	m.Records++
	if !b.fits(m) {
		return ErrCapacity
	}
	return nil
}

func (b *PoolOutbox) found(entry PoolEntry, s PoolScan) (PoolEntry, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if e := b.state(); e != nil {
		return PoolEntry{}, e
	}
	if !b.sameScan(s) || s.NextNonce == 0 || entry.Nonce != uint32(s.NextNonce-1) || !reflect.DeepEqual(entry.Work, s.Work) {
		return PoolEntry{}, ErrCorrupt
	}
	owned, e := clonePoolEntry(entry)
	if e != nil {
		return PoolEntry{}, e
	}
	entry = owned
	batch := new(leveldb.Batch)
	batch.Put([]byte("cursor"), cursor(s.NextNonce))
	if index, e := b.db.Get(hashKey(entry.DuplicateKey), nil); e == nil {
		if !manifestEntryKey(index, b.meta.LastSequence) {
			return PoolEntry{}, ErrCorrupt
		}
		raw, re := b.db.Get(index, nil)
		var previous PoolEntry
		if re != nil || poolDecodeCanonical(raw, poolMaxEntrySize, &previous) != nil || validatePoolEntry(previous, b.config) != nil {
			return PoolEntry{}, ErrCorrupt
		}
		entry.Sequence = previous.Sequence
		if entry.Fighter != nil {
			entry.Fighter.Sequence = entry.Sequence
		}
		if !reflect.DeepEqual(poolImmutable(previous), poolImmutable(entry)) {
			return PoolEntry{}, ErrCorrupt
		}
		if e = b.commit(batch, b.meta); e != nil {
			return PoolEntry{}, e
		}
		b.cachedScan.NextNonce = s.NextNonce
		return previous, nil
	} else if !errors.Is(e, leveldb.ErrNotFound) {
		return PoolEntry{}, ErrCorrupt
	}
	if s.NextNonce != b.cachedScan.NextNonce+1 {
		return PoolEntry{}, ErrCorrupt
	}
	if b.meta.Records >= b.config.MaxRecords || b.meta.LastSequence == math.MaxUint64 {
		return PoolEntry{}, ErrCapacity
	}
	entry.Sequence = b.meta.LastSequence + 1
	if entry.Fighter != nil {
		entry.Fighter.Sequence = entry.Sequence
	}
	if validatePoolEntry(entry, b.config) != nil {
		return PoolEntry{}, ErrCorrupt
	}
	raw, e := json.Marshal(entry)
	reserve := poolFutureReserve(entry)
	if e != nil || len(raw) > poolMaxEntrySize || reserve == poolInvalidReserve {
		return PoolEntry{}, ErrCapacity
	}
	m := b.meta
	m.Bytes += uint64(len(raw))
	m.Reserved += reserve
	m.Records++
	m.LastSequence++
	if !poolDone(entry) {
		m.Pending++
		batch.Put(poolPendingKey(entry), seqKey(entry.Sequence))
	}
	if !b.fits(m) {
		return PoolEntry{}, ErrCapacity
	}
	batch.Put(seqKey(entry.Sequence), raw)
	batch.Put(hashKey(entry.DuplicateKey), seqKey(entry.Sequence))
	if e = b.commit(batch, m); e != nil {
		return PoolEntry{}, e
	}
	b.cachedScan.NextNonce = s.NextNonce
	return entry, nil
}
func (b *PoolOutbox) update(entry PoolEntry) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if e := b.state(); e != nil {
		return e
	}
	if validatePoolEntry(entry, b.config) != nil {
		return ErrCorrupt
	}
	owned, e := clonePoolEntry(entry)
	if e != nil {
		return e
	}
	entry = owned
	raw, e := b.db.Get(seqKey(entry.Sequence), nil)
	var previous PoolEntry
	if e != nil || poolDecodeCanonical(raw, poolMaxEntrySize, &previous) != nil || validatePoolEntry(previous, b.config) != nil || !reflect.DeepEqual(poolImmutable(previous), poolImmutable(entry)) || !poolTransition(previous, entry) {
		return ErrCorrupt
	}
	encoded, e := json.Marshal(entry)
	oldReserve, newReserve := poolFutureReserve(previous), poolFutureReserve(entry)
	if e != nil || len(encoded) > poolMaxEntrySize || oldReserve == poolInvalidReserve || newReserve == poolInvalidReserve {
		return ErrCorrupt
	}
	m := b.meta
	if m.Bytes < uint64(len(raw)) || m.Reserved < oldReserve {
		return ErrCorrupt
	}
	m.Bytes = m.Bytes - uint64(len(raw)) + uint64(len(encoded))
	m.Reserved = m.Reserved - oldReserve + newReserve
	batch := new(leveldb.Batch)
	if !poolDone(previous) {
		if m.Pending == 0 {
			return ErrCorrupt
		}
		m.Pending--
		batch.Delete(poolPendingKey(previous))
	}
	if !poolDone(entry) {
		m.Pending++
		batch.Put(poolPendingKey(entry), seqKey(entry.Sequence))
	}
	if !b.fits(m) {
		return ErrCapacity
	}
	batch.Put(seqKey(entry.Sequence), encoded)
	return b.commit(batch, m)
}
