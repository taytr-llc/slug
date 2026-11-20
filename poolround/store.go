// Copyright (c) 2026 The SLUG developers. ISC license.
package poolround

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/wire/v2"
)

const (
	application     = "slug-pool-round-journal"
	markerName      = "SLUG-POOL-ROUNDS.json"
	maxMetadataSize = 16384
	maxEntrySize    = 64 << 10
	maxRoundSize    = 2 << 20
	statusReserve   = 4096
	maxRecordSize   = maxRoundSize + statusReserve
)

type marker struct {
	Application string           `json:"application"`
	Version     uint32           `json:"version"`
	Limits      Limits           `json:"limits"`
	Source      poolshare.Config `json:"source"`
}

func nonnil(value any) bool {
	if value == nil {
		return false
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Map, reflect.Slice, reflect.Func, reflect.Chan:
		return !v.IsNil()
	}
	return true
}

func validLimits(l Limits) bool {
	return l.Version == Version1 && l.MaxRounds > 0 && l.MaxRounds <= maxAccepted && l.MaxBytes >= statusReserve && l.MaxBytes <= 1<<30
}

func sourceError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	if errors.Is(err, poolshare.ErrProfile) {
		return ErrProfile
	}
	if errors.Is(err, poolshare.ErrCorrupt) || errors.Is(err, poolshare.ErrInvalid) {
		return ErrSourceChanged
	}
	return ErrSourceUnavailable
}

func configuration(l Limits, source *poolshare.Pool, node Observer) (poolshare.Config, error) {
	return configurationMode(l, source, node, false)
}

func configurationMode(l Limits, source *poolshare.Pool, node Observer, auditOnly bool) (poolshare.Config, error) {
	if wire.TestPoW {
		return poolshare.Config{}, ErrProfile
	}
	if !validLimits(l) || source == nil || (!auditOnly && !nonnil(node)) {
		return poolshare.Config{}, ErrConfig
	}
	page, err := source.AccountingPage(context.Background(), 0, 0, 1)
	if err != nil {
		return poolshare.Config{}, sourceError(err)
	}
	if page.Config.Version != poolshare.Version1 || page.Config.MaxSubmissions == 0 || page.Config.MaxSubmissions > maxAccepted || !canonicalHash(page.Config.PoolTarget, true) {
		return poolshare.Config{}, ErrConfig
	}
	return page.Config, nil
}

func decode(raw []byte, limit int, value any) error {
	if len(raw) == 0 || len(raw) > limit {
		return ErrCorrupt
	}
	depth, quoted, escape := 0, false, false
	for _, b := range raw {
		if quoted {
			if escape {
				escape = false
			} else if b == '\\' {
				escape = true
			} else if b == '"' {
				quoted = false
			}
			continue
		}
		switch b {
		case '"':
			quoted = true
		case '{', '[':
			depth++
		case '}', ']':
			depth--
		}
		if depth < 0 || depth > 12 {
			return ErrCorrupt
		}
	}
	if depth != 0 || quoted {
		return ErrCorrupt
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(value) != nil {
		return ErrCorrupt
	}
	canonical, err := json.Marshal(value)
	if err != nil || !bytes.Equal(raw, canonical) {
		return ErrCorrupt
	}
	return nil
}

func syncDir(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	return errors.Join(f.Sync(), f.Close())
}

func directory(path string) bool {
	i, err := os.Lstat(path)
	return err == nil && i.IsDir() && i.Mode()&os.ModeSymlink == 0
}

func newStore(l Limits, c poolshare.Config, source *poolshare.Pool, node Observer) *Store {
	return &Store{limits: l, sourceConfig: c, source: source, node: node, gate: make(chan struct{}, 1), closeDone: make(chan struct{})}
}

// Create initializes a separate new journal without importing or changing the
// source. Partial creation is retained for inspection, never silently repaired.
func Create(path string, l Limits, source *poolshare.Pool, node Observer) (*Store, error) {
	c, err := configuration(l, source, node)
	if err != nil {
		return nil, err
	}
	if os.Mkdir(path, 0700) != nil {
		return nil, ErrInvalid
	}
	raw, _ := json.Marshal(marker{application, Version1, l, c})
	f, err := os.OpenFile(filepath.Join(path, markerName), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, ErrCorrupt
	}
	_, writeErr := f.Write(raw)
	if errors.Join(writeErr, f.Sync(), f.Close()) != nil {
		return nil, ErrPoisoned
	}
	dbPath := filepath.Join(path, "db")
	if os.Mkdir(dbPath, 0700) != nil {
		return nil, ErrCorrupt
	}
	db, err := leveldb.OpenFile(dbPath, &opt.Options{ErrorIfExist: true, Strict: opt.StrictAll})
	if err != nil {
		return nil, ErrCorrupt
	}
	s := newStore(l, c, source, node)
	s.db, s.writeBatch = db, db.Write
	s.meta = metadata{Application: application, Version: Version1, Limits: l, Source: c, Prefix: initialPrefix(c)}
	if err := s.commit(new(leveldb.Batch), s.meta); err != nil {
		_ = db.Close()
		return nil, err
	}
	if errors.Join(syncDir(dbPath), syncDir(path), syncDir(filepath.Dir(filepath.Clean(path)))) != nil {
		_ = db.Close()
		return nil, ErrPoisoned
	}
	return s, nil
}

// Open requires the exact limits/source identity and checks all stored windows
// against the original accepted prefix. It performs no new node observations.
func Open(path string, l Limits, source *poolshare.Pool, node Observer) (*Store, error) {
	return openMode(path, l, source, node, false)
}

func openMode(path string, l Limits, source *poolshare.Pool, node Observer, auditOnly bool) (*Store, error) {
	c, err := configurationMode(l, source, node, auditOnly)
	if err != nil {
		return nil, err
	}
	if !directory(path) || !directory(filepath.Join(path, "db")) {
		return nil, ErrCorrupt
	}
	name := filepath.Join(path, markerName)
	i, err := os.Lstat(name)
	if err != nil || !i.Mode().IsRegular() || i.Size() > maxMetadataSize {
		return nil, ErrCorrupt
	}
	f, err := os.Open(name)
	if err != nil {
		return nil, ErrCorrupt
	}
	raw, readErr := io.ReadAll(io.LimitReader(f, maxMetadataSize+1))
	if errors.Join(readErr, f.Close()) != nil {
		return nil, ErrCorrupt
	}
	var m marker
	if decode(raw, maxMetadataSize, &m) != nil || m.Application != application || m.Version != Version1 || m.Limits != l || m.Source != c {
		return nil, ErrSourceChanged
	}
	db, err := leveldb.OpenFile(filepath.Join(path, "db"), &opt.Options{ErrorIfMissing: true, ReadOnly: auditOnly, Strict: opt.StrictAll})
	if err != nil {
		return nil, ErrCorrupt
	}
	s := newStore(l, c, source, node)
	s.db, s.writeBatch, s.auditOnly = db, db.Write, auditOnly
	raw, err = db.Get([]byte("meta"), nil)
	if err != nil || decode(raw, maxMetadataSize, &s.meta) != nil || !s.validMetadata(s.meta) {
		_ = db.Close()
		return nil, ErrCorrupt
	}
	if _, err := s.verifyStored(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func roundKey(hash string) []byte { return []byte("r" + hash) }
func indexKey(sequence uint64) []byte {
	k := make([]byte, 9)
	k[0] = 'i'
	binary.BigEndian.PutUint64(k[1:], sequence)
	return k
}

func (s *Store) validMetadata(m metadata) bool {
	return m.Application == application && m.Version == Version1 && m.Limits == s.limits && m.Source == s.sourceConfig && m.Through <= s.sourceConfig.MaxSubmissions && m.Rounds <= m.Through && m.Rounds <= s.limits.MaxRounds && m.Bytes <= s.limits.MaxBytes && canonicalHash(m.Prefix, false)
}

func (s *Store) commit(batch *leveldb.Batch, m metadata) error {
	if s.auditOnly {
		return ErrInvalid
	}
	if s.poisoned {
		return ErrPoisoned
	}
	if !s.validMetadata(m) || m.Through < s.meta.Through || m.Rounds < s.meta.Rounds || m.Bytes < s.meta.Bytes {
		return ErrCorrupt
	}
	raw, err := json.Marshal(m)
	if err != nil || len(raw) > maxMetadataSize {
		return ErrCorrupt
	}
	batch.Put([]byte("meta"), raw)
	if s.writeBatch(batch, &opt.WriteOptions{Sync: true}) != nil {
		s.poisoned = true
		return ErrPoisoned
	}
	s.meta = m
	return nil
}

func roundBudget(r Round) (uint64, error) {
	raw, err := json.Marshal(r)
	if err != nil || len(raw) > maxRoundSize {
		return 0, ErrCapacity
	}
	return uint64(len(raw)) + statusReserve, nil
}

func recordBytes(r Record) ([]byte, error) {
	budget, err := roundBudget(r.Round)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(r)
	if err != nil || uint64(len(raw)) > budget || len(raw) > maxRecordSize || !validRecord(r) {
		return nil, ErrCorrupt
	}
	return raw, nil
}

// snapshot reads one fixed accepted tail, never source delivery state. A nil
// limit selects the current tail; zero explicitly requests an empty old prefix.
func (s *Store) snapshot(ctx context.Context, limit *uint64) ([]poolshare.AccountingEntry, string, error) {
	requested := uint64(0)
	if limit != nil {
		requested = *limit
	}
	page, err := s.source.AccountingPage(ctx, 0, requested, poolshare.MaxAccountingPage)
	if err != nil {
		return nil, "", sourceError(err)
	}
	if page.Config != s.sourceConfig || page.Through > s.sourceConfig.MaxSubmissions {
		return nil, "", ErrSourceChanged
	}
	through := page.Through
	if limit != nil {
		through = *limit
		if requested != 0 && page.Through != requested {
			return nil, "", ErrSourceChanged
		}
	}
	prefix := initialPrefix(s.sourceConfig)
	entries := make([]poolshare.AccountingEntry, 0, through)
	if through == 0 {
		return entries, prefix, nil
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, "", err
		}
		if page.Config != s.sourceConfig || page.Through != through || len(page.Entries) == 0 || len(page.Entries) > poolshare.MaxAccountingPage {
			return nil, "", ErrSourceChanged
		}
		for _, entry := range page.Entries {
			if uint64(len(entries)) >= through || entry.Share.AcceptedSequence != uint64(len(entries))+1 {
				return nil, "", ErrSourceChanged
			}
			prefix, err = extendPrefix(prefix, entry)
			if err != nil {
				return nil, "", err
			}
			entries = append(entries, entry)
		}
		if uint64(len(entries)) == through {
			break
		}
		page, err = s.source.AccountingPage(ctx, uint64(len(entries)), through, poolshare.MaxAccountingPage)
		if err != nil {
			return nil, "", sourceError(err)
		}
	}
	return entries, prefix, nil
}

func sameRound(a, b Round) bool {
	x, e := json.Marshal(a)
	if e != nil {
		return false
	}
	y, e := json.Marshal(b)
	return e == nil && bytes.Equal(x, y)
}

// stored validates all indexes and immutable rows against independently
// reconstructed source windows. No incoming JSON can authorize a round.
func (s *Store) stored(expected []Round) ([]Record, error) {
	if uint64(len(expected)) != s.meta.Rounds {
		return nil, ErrSourceChanged
	}
	records := make([]Record, len(expected))
	byHash := make(map[string]int, len(expected))
	for i, r := range expected {
		byHash[r.BlockHash] = i
	}
	var rows, indexes, size uint64
	seenMetadata := false
	it := s.db.NewIterator(nil, nil)
	defer it.Release()
	for it.Next() {
		k, v := it.Key(), it.Value()
		if string(k) == "meta" {
			var m metadata
			if decode(v, maxMetadataSize, &m) != nil || m != s.meta {
				return nil, ErrCorrupt
			}
			seenMetadata = true
			continue
		}
		if len(k) == 65 && k[0] == 'r' {
			index, exists := byHash[string(k[1:])]
			if !exists {
				return nil, ErrCorrupt
			}
			var record Record
			if decode(v, maxRecordSize, &record) != nil || !sameRound(record.Round, expected[index]) || !validRecord(record) {
				return nil, ErrCorrupt
			}
			budget, err := roundBudget(record.Round)
			if err != nil || uint64(len(v)) > budget {
				return nil, ErrCorrupt
			}
			records[index] = record
			rows++
			size += budget
		} else if len(k) == 9 && k[0] == 'i' {
			n := binary.BigEndian.Uint64(k[1:])
			if n == 0 || n > uint64(len(expected)) || string(v) != expected[n-1].BlockHash {
				return nil, ErrCorrupt
			}
			indexes++
		} else {
			return nil, ErrCorrupt
		}
		if rows > s.meta.Rounds || indexes > s.meta.Rounds || size > s.meta.Bytes {
			return nil, ErrCorrupt
		}
	}
	if it.Error() != nil || !seenMetadata || rows != s.meta.Rounds || indexes != s.meta.Rounds || size != s.meta.Bytes {
		return nil, ErrCorrupt
	}
	return records, nil
}

func (s *Store) verifyStored(ctx context.Context) ([]Record, error) {
	entries, prefix, err := s.snapshot(ctx, &s.meta.Through)
	if err != nil {
		return nil, err
	}
	if prefix != s.meta.Prefix {
		return nil, ErrSourceChanged
	}
	rounds, err := derive(s.sourceConfig, entries)
	if err != nil {
		return nil, err
	}
	return s.stored(rounds)
}

func (s *Store) begin(ctx context.Context) (func(), error) {
	if wire.TestPoW {
		return nil, ErrProfile
	}
	if ctx == nil {
		return nil, ErrInvalid
	}
	if s == nil || s.db == nil || s.gate == nil {
		return nil, ErrClosed
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.lifecycle.Lock()
	if s.closing {
		s.lifecycle.Unlock()
		return nil, ErrClosed
	}
	select {
	case s.gate <- struct{}{}:
	default:
		s.lifecycle.Unlock()
		return nil, ErrBusy
	}
	s.lifecycle.Unlock()
	end := func() { <-s.gate }
	if s.poisoned {
		end()
		return nil, ErrPoisoned
	}
	return end, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil || s.gate == nil || s.closeDone == nil {
		return ErrClosed
	}
	s.lifecycle.Lock()
	if s.closing {
		done := s.closeDone
		s.lifecycle.Unlock()
		<-done
		return s.closeErr
	}
	s.closing = true
	s.lifecycle.Unlock()
	s.gate <- struct{}{}
	if s.db.Close() != nil {
		s.closeErr = ErrPoisoned
	}
	<-s.gate
	close(s.closeDone)
	return s.closeErr
}
