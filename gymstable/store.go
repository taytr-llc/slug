// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/internal/careerconsumer"
	"github.com/taytr-llc/slug/internal/careerstate"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

const markerName = "SLUG-GYM-STABLE.json"

type marker struct {
	Application string `json:"application"`
	Version     uint32 `json:"version"`
	StoreID     string `json:"store_id"`
	Pin         Pin    `json:"pin"`
}
type metadata struct {
	StoreID             string `json:"store_id"`
	Pin                 Pin    `json:"pin"`
	Checkpoint          uint64 `json:"checkpoint"`
	ObservedSourceCount uint64 `json:"observed_source_count"`
	RosterRevision      uint64 `json:"roster_revision"`
	OperationCount      uint32 `json:"operation_count"`
	PendingOperationID  string `json:"pending_operation_id"`
	LastOperationID     string `json:"last_operation_id"`
	ClockMS             int64  `json:"clock_ms"`
	LastAsOfMS          int64  `json:"last_as_of_ms"`
}
type rosterRow struct {
	Version  uint32     `json:"version"`
	Revision uint64     `json:"revision"`
	Slots    [10]string `json:"slots"`
}

// Store borrows its exact source. Close drains real work but never closes source.
// No callback may reenter Store or invoke it from an already held account lease.
type Store struct {
	gate       chan struct{}
	mu         sync.Mutex
	life       context.Context
	cancel     context.CancelFunc
	closeOnce  sync.Once
	closed     bool
	poisoned   bool
	closeErr   error
	db         *leveldb.DB
	source     *registry.ManifestStore
	meta       metadata
	roster     rosterRow
	clockSeen  int64
	now        func() time.Time
	random     io.Reader
	writeBatch func(*leveldb.Batch, *opt.WriteOptions) error
	checkpoint func(string)
	proofRead  func()
	career     *careerconsumer.Guard
}

func normal() error {
	if wire.TestPoW {
		return ErrProfile
	}
	return nil
}
func directory(path string) bool {
	i, e := os.Lstat(path)
	return e == nil && i.IsDir() && i.Mode()&os.ModeSymlink == 0
}
func syncDir(path string) error {
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	return errors.Join(f.Sync(), f.Close())
}
func newStore(db *leveldb.DB, source *registry.ManifestStore) *Store {
	ctx, cancel := context.WithCancel(context.Background())
	career, _ := careerconsumer.New(nil, false)
	return &Store{db: db, source: source, gate: make(chan struct{}, 1), life: ctx, cancel: cancel, now: time.Now, random: rand.Reader, writeBatch: db.Write, career: career}
}
func sourceValid(source *registry.ManifestStore, pin Pin) (registry.ManifestAuthorityInfo, error) {
	if source == nil {
		return registry.ManifestAuthorityInfo{}, ErrInvalid
	}
	i, e := source.Info()
	if e != nil {
		return i, ErrClosed
	}
	if i.Pin != pin.Source || i.Records > MaxAssignments {
		return i, ErrCorrupt
	}
	return i, nil
}
func Create(path string, pin Pin, source *registry.ManifestStore) (*Store, error) {
	return create(path, pin, source, time.Now, nil)
}
func create(path string, pin Pin, source *registry.ManifestStore, clock func() time.Time, checkpoint func(string)) (*Store, error) {
	if e := normal(); e != nil {
		return nil, e
	}
	p, e := ValidatePin(pin)
	if e != nil {
		return nil, e
	}
	info, e := sourceValid(source, p)
	if e != nil {
		return nil, e
	}
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || !directory(filepath.Dir(path)) {
		return nil, ErrInvalid
	}
	id, _ := ID(p)
	now := clock().UnixMilli()
	if now <= 0 || now > MaxTimestampMS {
		return nil, ErrClock
	}
	m := marker{"slug-candidate-stable", Version1, id, p}
	raw, _ := json.Marshal(m)
	if len(raw)+1 > MaxMarkerSize {
		return nil, ErrInvalid
	}
	if e = os.Mkdir(path, 0700); e != nil {
		return nil, ErrClosed
	}
	f, e := os.OpenFile(filepath.Join(path, markerName), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return nil, ErrClosed
	}
	_, e = f.Write(append(raw, '\n'))
	if e = errors.Join(e, f.Sync(), f.Close()); e != nil {
		return nil, ErrOutcomeUnknown
	}
	dbpath := filepath.Join(path, "db")
	if e = os.Mkdir(dbpath, 0700); e != nil {
		return nil, ErrClosed
	}
	db, e := leveldb.OpenFile(dbpath, &opt.Options{Strict: opt.StrictAll, ErrorIfExist: true})
	if e != nil {
		return nil, ErrClosed
	}
	s := newStore(db, source)
	s.now = clock
	s.checkpoint = checkpoint
	s.meta = metadata{StoreID: id, Pin: p, ObservedSourceCount: info.Records, ClockMS: now}
	s.roster = rosterRow{Version: Version1}
	s.clockSeen = now
	b := new(leveldb.Batch)
	rr, _ := json.Marshal(s.roster)
	b.Put([]byte("roster"), rr)
	s.mark("before_initialize")
	if e = s.commitLocked(b, s.meta); e != nil {
		_ = s.Close()
		return nil, e
	}
	for _, p := range []string{dbpath, path, filepath.Dir(path)} {
		if e = syncDir(p); e != nil {
			_ = s.Close()
			return nil, ErrOutcomeUnknown
		}
	}
	s.mark("initialized")
	return s, nil
}
func Open(path string, expected Pin, source *registry.ManifestStore) (*Store, error) {
	return openWithClock(path, expected, source, time.Now)
}
func openWithClock(path string, expected Pin, source *registry.ManifestStore, clock func() time.Time) (*Store, error) {
	return openCareerWithClock(path, expected, source, clock, nil, false)
}
func openCareerWithClock(path string, expected Pin, source *registry.ManifestStore, clock func() time.Time, activation *careerstate.Activation, prepared bool) (*Store, error) {
	if e := normal(); e != nil {
		return nil, e
	}
	p, e := ValidatePin(expected)
	if e != nil {
		return nil, e
	}
	info, e := sourceValid(source, p)
	if e != nil {
		return nil, e
	}
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || !directory(path) || !directory(filepath.Join(path, "db")) {
		return nil, ErrCorrupt
	}
	file := filepath.Join(path, markerName)
	before, e := os.Lstat(file)
	if e != nil || !before.Mode().IsRegular() || before.Size() > MaxMarkerSize {
		return nil, ErrCorrupt
	}
	f, e := os.Open(file)
	if e != nil {
		return nil, ErrCorrupt
	}
	after, e := f.Stat()
	if e != nil || !os.SameFile(before, after) {
		f.Close()
		return nil, ErrCorrupt
	}
	raw, e := io.ReadAll(io.LimitReader(f, MaxMarkerSize+1))
	e = errors.Join(e, f.Close())
	if e != nil || len(raw) < 2 || len(raw) > MaxMarkerSize || raw[len(raw)-1] != '\n' {
		return nil, ErrCorrupt
	}
	var m marker
	if decode(raw[:len(raw)-1], MaxMarkerSize, 5, &m, true) != nil {
		return nil, ErrCorrupt
	}
	id, _ := ID(p)
	if m != (marker{"slug-candidate-stable", Version1, id, p}) {
		return nil, ErrCorrupt
	}
	db, e := leveldb.OpenFile(filepath.Join(path, "db"), &opt.Options{Strict: opt.StrictAll, ErrorIfMissing: true})
	if e != nil {
		return nil, ErrClosed
	}
	s := newStore(db, source)
	s.now = clock
	s.career, e = careerconsumer.New(activation, prepared)
	if e != nil {
		_ = s.Close()
		return nil, e
	}
	if e = s.audit(m, info.Records); e != nil {
		_ = s.Close()
		return nil, e
	}
	// Opening has already observed this authenticated source count. Retain that
	// highwater even before the first authorized page, without assigning rows.
	if info.Records > s.meta.ObservedSourceCount {
		next := s.meta
		next.ObservedSourceCount = info.Records
		if e = s.commitLocked(new(leveldb.Batch), next); e != nil {
			_ = s.Close()
			return nil, e
		}
	}
	return s, nil
}
func (s *Store) mark(stage string) {
	if s.checkpoint != nil {
		s.checkpoint(stage)
	}
}
func (s *Store) stateLocked() error {
	if s.closed {
		return ErrClosed
	}
	if s.poisoned {
		return ErrPoisoned
	}
	return nil
}
func (s *Store) commitLocked(b *leveldb.Batch, m metadata) error {
	raw, e := json.Marshal(m)
	if e != nil || len(raw) > MaxMetadataSize {
		return ErrCorrupt
	}
	b.Put([]byte("meta"), raw)
	if e = s.writeBatch(b, &opt.WriteOptions{Sync: true}); e != nil {
		s.poisoned = true
		return ErrOutcomeUnknown
	}
	s.meta = m
	s.clockSeen = max(s.clockSeen, m.ClockMS)
	return nil
}
func (s *Store) observeLocked() (int64, error) {
	n := s.now().UnixMilli()
	if n <= 0 || n > MaxTimestampMS || n < s.clockSeen || n < s.meta.ClockMS {
		return 0, ErrClock
	}
	s.clockSeen = n
	if n > s.meta.ClockMS {
		m := s.meta
		m.ClockMS = n
		if e := s.commitLocked(new(leveldb.Batch), m); e != nil {
			return 0, e
		}
	}
	return n, nil
}
func (s *Store) Close() error {
	s.closeOnce.Do(func() {
		s.cancel()
		s.gate <- struct{}{}
		defer func() { <-s.gate }()
		s.mu.Lock()
		defer s.mu.Unlock()
		s.closed = true
		s.closeErr = s.db.Close()
	})
	return s.closeErr
}
func (s *Store) begin(ctx context.Context, session Session, cap gymaccount.Capability) (context.Context, func(), error) {
	if e := normal(); e != nil {
		return nil, nil, e
	}
	if ctx == nil || session.Lease == nil || session.Generation == 0 || session.Generation > MaxSafeInteger || !hashID(session.ViewID) {
		return nil, nil, ErrUnauthorized
	}
	// Once Close starts, new calls see a closed dependency regardless of whether
	// its cancelled lifetime or the available gate wins the select below.
	// Operations already in progress still use the existing cancellation path.
	if s.life.Err() != nil {
		return nil, nil, ErrClosed
	}
	work, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(s.life, cancel)
	finish := func() { stop(); cancel() }
	select {
	case <-work.Done():
		finish()
		if ctx.Err() == nil && s.life.Err() != nil {
			return nil, nil, ErrClosed
		}
		return nil, nil, ErrCancelled
	case s.gate <- struct{}{}:
	}
	release := func() { <-s.gate; finish() }
	s.mu.Lock()
	e := s.stateLocked()
	if e == nil {
		e = s.career.Ready()
	}
	s.mu.Unlock()
	if e != nil {
		release()
		return nil, nil, e
	}
	if s.life.Err() != nil {
		release()
		return nil, nil, ErrClosed
	}
	if e = s.withLease(work, session, cap, nil); e != nil {
		release()
		return nil, nil, e
	}
	s.mu.Lock()
	_, e = s.observeLocked()
	s.mu.Unlock()
	if e != nil {
		release()
		return nil, nil, e
	}
	return work, release, nil
}
func mapped(e error) error {
	if e == nil {
		return nil
	}
	switch {
	case errors.Is(e, context.Canceled) || errors.Is(e, context.DeadlineExceeded):
		return ErrCancelled
	case errors.Is(e, gymaccount.ErrForbidden) || errors.Is(e, gymaccount.ErrHeld):
		return ErrForbidden
	case errors.Is(e, gymaccount.ErrUnauthorized) || errors.Is(e, gymaccount.ErrExpired):
		return ErrUnauthorized
	}
	return e
}
func (s *Store) withLease(ctx context.Context, session Session, cap gymaccount.Capability, fn func(func() error) error) error {
	var guard sync.Mutex
	active, called := true, false
	var callbackErr error
	err := session.Lease(ctx, cap, func(a gymaccount.Account, recheck func() error) error {
		guard.Lock()
		defer guard.Unlock()
		if !active || called {
			return ErrUnauthorized
		}
		called = true
		check := func() error {
			if ctx.Err() != nil {
				return ErrCancelled
			}
			if a.ID != s.meta.Pin.AccountID || a.Generation != session.Generation {
				return ErrStaleView
			}
			if a.Enrollment == nil || *a.Enrollment != s.meta.Pin.Identity || a.RecoveryRequired || recheck == nil {
				return ErrForbidden
			}
			return mapped(recheck())
		}
		if e := check(); e != nil {
			callbackErr = e
			return e
		}
		if fn != nil {
			callbackErr = fn(check)
			return callbackErr
		}
		return nil
	})
	guard.Lock()
	active = false
	didCall := called
	guard.Unlock()
	if !didCall && err == nil {
		return ErrUnauthorized
	}
	if callbackErr != nil {
		return mapped(callbackErr)
	}
	if ctx.Err() != nil {
		return ErrCancelled
	}
	return mapped(err)
}
func (s *Store) sourceInfo(ctx context.Context) (uint64, error) {
	return s.sourceInfoMode(ctx, false)
}

// The history family cannot downgrade learned source failures to cancellation.
// Existing consumers retain their original cancellation order.
func (s *Store) sourceInfoMode(ctx context.Context, fatalFirst bool) (uint64, error) {
	if ctx.Err() != nil {
		return 0, ErrCancelled
	}
	i, e := sourceValid(s.source, s.meta.Pin)
	if fatalFirst {
		if e != nil {
			return 0, e
		}
		if i.Records < s.meta.ObservedSourceCount || i.Records < s.meta.Checkpoint {
			return 0, ErrCorrupt
		}
	}
	if ctx.Err() != nil {
		return 0, ErrCancelled
	}
	if e != nil {
		return 0, e
	}
	if i.Records < s.meta.ObservedSourceCount || i.Records < s.meta.Checkpoint {
		return 0, ErrCorrupt
	}
	// Info has returned and released the registry gate. Never hold a source
	// lock while syncing local metadata or acquiring an account lease.
	if i.Records > s.meta.ObservedSourceCount {
		s.mu.Lock()
		next := s.meta
		next.ObservedSourceCount = i.Records
		e = s.commitLocked(new(leveldb.Batch), next)
		s.mu.Unlock()
		if e != nil {
			return 0, e
		}
	}
	return i.Records, nil
}
func (s *Store) statusLocked() Status {
	return Status{Version: Version1, StoreID: s.meta.StoreID, Pin: s.meta.Pin, Checkpoint: s.meta.Checkpoint, ObservedSourceCount: s.meta.ObservedSourceCount, CaughtUp: s.meta.Checkpoint == s.meta.ObservedSourceCount, RosterRevision: s.meta.RosterRevision, Slots: s.roster.Slots, OperationCount: s.meta.OperationCount, PendingOperationID: s.meta.PendingOperationID, LastOperationID: s.meta.LastOperationID, LastAsOfMS: s.meta.LastAsOfMS}
}
func (s *Store) Status(ctx context.Context, session Session) (Status, error) {
	ctx, done, e := s.begin(ctx, session, gymaccount.View)
	if e != nil {
		return Status{}, e
	}
	defer done()
	var result Status
	e = s.withLease(ctx, session, gymaccount.View, func(check func() error) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		if e := check(); e != nil {
			return e
		}
		result = s.statusLocked()
		return nil
	})
	if e != nil {
		return Status{}, e
	}
	return result, nil
}
func seqKey(n uint64) []byte {
	b := make([]byte, 9)
	b[0] = 'a'
	binary.BigEndian.PutUint64(b[1:], n)
	return b
}
func idKey(prefix byte, id string) []byte {
	b, _ := hex.DecodeString(id)
	return append([]byte{prefix}, b...)
}
func encodedPut(b *leveldb.Batch, key []byte, v any, limit int) error {
	raw, e := json.Marshal(v)
	if e != nil || len(raw) > limit {
		return ErrCapacity
	}
	b.Put(key, raw)
	return nil
}
func (s *Store) assignment(id string) (Assignment, error) {
	if !hashID(id) {
		return Assignment{}, ErrInvalid
	}
	v, e := s.db.Get(idKey('i', id), nil)
	if errors.Is(e, leveldb.ErrNotFound) {
		return Assignment{}, ErrNotFound
	}
	if e != nil || len(v) != 8 {
		return Assignment{}, ErrCorrupt
	}
	n := binary.BigEndian.Uint64(v)
	if n == 0 || n > s.meta.Checkpoint {
		return Assignment{}, ErrCorrupt
	}
	a, e := s.assignmentSequence(n)
	if e != nil || a.ProspectID != id {
		return Assignment{}, ErrCorrupt
	}
	return a, nil
}
func (s *Store) assignmentSequence(n uint64) (Assignment, error) {
	var a Assignment
	raw, e := s.db.Get(seqKey(n), nil)
	if e != nil || decode(raw, MaxAssignmentSize, 1, &a, true) != nil || !validAssignment(a, s.meta.Pin) || a.Sequence != n {
		return Assignment{}, ErrCorrupt
	}
	v, e := s.db.Get(idKey('i', a.ProspectID), nil)
	if e != nil || len(v) != 8 || binary.BigEndian.Uint64(v) != n {
		return Assignment{}, ErrCorrupt
	}
	return a, nil
}
func (s *Store) operation(id string) (Operation, error) {
	if !hashID(id) {
		return Operation{}, ErrInvalid
	}
	var o Operation
	raw, e := s.db.Get(idKey('o', id), nil)
	if errors.Is(e, leveldb.ErrNotFound) {
		return o, ErrNotFound
	}
	if e != nil || decode(raw, MaxOperationSize, 5, &o, true) != nil || o.Intent.OperationID != id || !validOperation(o, s.meta) {
		return Operation{}, ErrCorrupt
	}
	return o, nil
}
func equalEncoded(a, b any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return bytes.Equal(x, y)
}
