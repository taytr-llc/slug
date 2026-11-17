// Copyright (c) 2026 The SLUG developers. ISC license.
package gymentry

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/wire/v2"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const markerName = "SLUG-CANDIDATE-ENTRIES.json"

type marker struct {
	Application string `json:"application"`
	Version     uint32 `json:"version"`
	StoreID     string `json:"store_id"`
	Pin         Pin    `json:"pin"`
}
type metadata struct {
	Version                uint32 `json:"version"`
	StoreID                string `json:"store_id"`
	Pin                    Pin    `json:"pin"`
	EventCount             uint32 `json:"event_count"`
	OperationCount         uint32 `json:"operation_count"`
	PendingOperationID     string `json:"pending_operation_id"`
	LastOperationID        string `json:"last_operation_id"`
	ObservedCheckpoint     uint64 `json:"observed_checkpoint"`
	ObservedSourceCount    uint64 `json:"observed_source_count"`
	ObservedRosterRevision uint64 `json:"observed_roster_revision"`
	ClockMS                int64  `json:"clock_ms"`
}
type eventRow struct {
	Version            uint32 `json:"version"`
	Event              Event  `json:"event"`
	EntryID            string `json:"entry_id"`
	CreatedAtMS        int64  `json:"created_at_ms"`
	Revision           uint64 `json:"revision"`
	LatestOperationID  string `json:"latest_operation_id"`
	Phase              string `json:"phase"`
	ClosedObservedAtMS int64  `json:"closed_observed_at_ms"`
}

// Store owns only its entry database. The source must outlive it; callers must
// close this handle before closing/rebinding the borrowed concrete stable.
// No callback may reenter account, stable or entry while its lease is held.
type Store struct {
	gate             chan struct{}
	mu               sync.Mutex
	life             context.Context
	cancel           context.CancelFunc
	closeOnce        sync.Once
	closed, poisoned bool
	closeErr         error
	db               *leveldb.DB
	source           *gymstable.Store
	meta             metadata
	clockSeen        int64
	now              func() time.Time
	random           io.Reader
	writeBatch       func(*leveldb.Batch, *opt.WriteOptions) error
	checkpoint       func(string)
}

func normal() error {
	if wire.TestPoW {
		return ErrProfile
	}
	return nil
}
func directory(path string) bool {
	f, e := os.Lstat(path)
	return e == nil && f.IsDir() && f.Mode()&os.ModeSymlink == 0
}

// Only the intended real empty directory is resumable. Read one entry so a
// hostile nonempty directory cannot force an unbounded inventory allocation.
func emptyDirectory(path string) bool {
	before, e := os.Lstat(path)
	if e != nil || !before.IsDir() || before.Mode()&os.ModeSymlink != 0 {
		return false
	}
	f, e := os.Open(path)
	if e != nil {
		return false
	}
	defer f.Close()
	after, e := f.Stat()
	if e != nil || !after.IsDir() || !os.SameFile(before, after) {
		return false
	}
	entries, e := f.ReadDir(1)
	return len(entries) == 0 && e == io.EOF
}

func syncDir(path string) error {
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	return errors.Join(f.Sync(), f.Close())
}
func cleanPath(path string) bool {
	return filepath.IsAbs(path) && filepath.Clean(path) == path && directory(filepath.Dir(path))
}
func newStore(db *leveldb.DB, source *gymstable.Store) *Store {
	ctx, cancel := context.WithCancel(context.Background())
	return &Store{gate: make(chan struct{}, 1), life: ctx, cancel: cancel, db: db, source: source, now: time.Now, random: rand.Reader, writeBatch: db.Write}
}
func sourceInfo(ctx context.Context, source *gymstable.Store, p Pin) (gymstable.EntrySourceInfo, error) {
	if ctx == nil || source == nil {
		return gymstable.EntrySourceInfo{}, ErrInvalid
	}
	v, e := source.EntrySourceInfo(ctx)
	if e != nil {
		return v, mapped(e)
	}
	if v.Version != gymstable.Version1 || v.StoreID != p.StableStoreID || v.Pin != p.StablePin || v.Checkpoint > v.ObservedSourceCount || v.ObservedSourceCount > gymstable.MaxAssignments || v.RosterRevision > gymstable.MaxOperations {
		return v, ErrCorrupt
	}
	return v, nil
}
func Create(ctx context.Context, path string, pin Pin, source *gymstable.Store) (*Store, error) {
	return create(ctx, path, pin, source, time.Now, nil)
}
func create(ctx context.Context, path string, pin Pin, source *gymstable.Store, clock func() time.Time, checkpoint func(string)) (*Store, error) {
	if e := normal(); e != nil {
		return nil, e
	}
	p, e := ValidatePin(pin)
	if e != nil {
		return nil, e
	}
	info, e := sourceInfo(ctx, source, p)
	if e != nil {
		return nil, e
	}
	if !cleanPath(path) {
		return nil, ErrInvalid
	}
	now := clock().UnixMilli()
	if !timestamp(now) {
		return nil, ErrClock
	}
	if ctx.Err() != nil {
		return nil, ErrCancelled
	}
	id, _ := ID(p)
	m := marker{"slug-candidate-entries", Version1, id, p}
	raw, _ := json.Marshal(m)
	if len(raw)+1 > MaxMarkerSize {
		return nil, ErrCapacity
	}
	if e = os.Mkdir(path, 0700); e != nil {
		if !errors.Is(e, os.ErrExist) || !emptyDirectory(path) {
			return nil, ErrUnavailable
		}
	}
	f, e := os.OpenFile(filepath.Join(path, markerName), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return nil, ErrUnavailable
	}
	_, e = f.Write(append(raw, '\n'))
	if e = errors.Join(e, f.Sync(), f.Close()); e != nil {
		return nil, ErrOutcomeUnknown
	}
	dbpath := filepath.Join(path, "db")
	if e = os.Mkdir(dbpath, 0700); e != nil {
		return nil, ErrUnavailable
	}
	db, e := leveldb.OpenFile(dbpath, &opt.Options{Strict: opt.StrictAll, ErrorIfExist: true})
	if e != nil {
		return nil, ErrUnavailable
	}
	s := newStore(db, source)
	s.now = clock
	s.checkpoint = checkpoint
	s.meta = metadata{Version: Version1, StoreID: id, Pin: p, ObservedCheckpoint: info.Checkpoint, ObservedSourceCount: info.ObservedSourceCount, ObservedRosterRevision: info.RosterRevision, ClockMS: now}
	s.clockSeen = now
	s.mark("before_initialize")
	if e = s.commitLocked(new(leveldb.Batch), s.meta); e != nil {
		s.Close()
		return nil, e
	}
	for _, p := range []string{dbpath, path, filepath.Dir(path)} {
		if syncDir(p) != nil {
			s.Close()
			return nil, ErrOutcomeUnknown
		}
	}
	s.mark("initialized")
	if ctx.Err() != nil {
		s.Close()
		return nil, ErrOutcomeUnknown
	}
	return s, nil
}
func Open(ctx context.Context, path string, expected Pin, source *gymstable.Store) (*Store, error) {
	if e := normal(); e != nil {
		return nil, e
	}
	p, e := ValidatePin(expected)
	if e != nil {
		return nil, e
	}
	info, e := sourceInfo(ctx, source, p)
	if e != nil {
		return nil, e
	}
	if !cleanPath(path) || !directory(path) || !directory(filepath.Join(path, "db")) {
		return nil, ErrCorrupt
	}
	name := filepath.Join(path, markerName)
	before, e := os.Lstat(name)
	if e != nil || !before.Mode().IsRegular() || before.Size() < 2 || before.Size() > MaxMarkerSize {
		return nil, ErrCorrupt
	}
	f, e := os.Open(name)
	if e != nil {
		return nil, ErrCorrupt
	}
	after, e := f.Stat()
	if e != nil || !after.Mode().IsRegular() || after.Size() > MaxMarkerSize || !os.SameFile(before, after) {
		f.Close()
		return nil, ErrCorrupt
	}
	raw, e := io.ReadAll(io.LimitReader(f, MaxMarkerSize+1))
	e = errors.Join(e, f.Close())
	if e != nil || len(raw) < 2 || len(raw) > MaxMarkerSize || raw[len(raw)-1] != '\n' {
		return nil, ErrCorrupt
	}
	var m marker
	if decode(raw[:len(raw)-1], MaxMarkerSize, &m, true) != nil {
		return nil, ErrCorrupt
	}
	id, _ := ID(p)
	if m != (marker{"slug-candidate-entries", Version1, id, p}) {
		return nil, ErrCorrupt
	}
	if ctx.Err() != nil {
		return nil, ErrCancelled
	}
	db, e := leveldb.OpenFile(filepath.Join(path, "db"), &opt.Options{Strict: opt.StrictAll, ErrorIfMissing: true})
	if e != nil {
		return nil, ErrUnavailable
	}
	s := newStore(db, source)
	if e = s.audit(ctx, m, info); e != nil {
		s.Close()
		return nil, e
	}
	if ctx.Err() != nil {
		s.Close()
		return nil, ErrCancelled
	}
	next := s.meta
	if e = mergeSource(&next, info); e != nil {
		s.Close()
		return nil, e
	}
	if next != s.meta {
		if e = s.commitLocked(new(leveldb.Batch), next); e != nil {
			s.Close()
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
		return ErrUnavailable
	}
	if s.poisoned {
		return ErrPoisoned
	}
	return nil
}
func (s *Store) Close() error {
	if s == nil {
		return nil
	}
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
func (s *Store) commitLocked(b *leveldb.Batch, next metadata) error {
	raw, e := json.Marshal(next)
	if e != nil || len(raw) > MaxMetadataSize {
		return ErrCorrupt
	}
	b.Put([]byte("meta"), raw)
	if e = s.writeBatch(b, &opt.WriteOptions{Sync: true}); e != nil {
		s.poisoned = true
		return ErrOutcomeUnknown
	}
	s.meta = next
	s.clockSeen = max(s.clockSeen, next.ClockMS)
	return nil
}
func put(b *leveldb.Batch, key []byte, v any, limit int) error {
	raw, e := json.Marshal(v)
	if e != nil || len(raw) > limit {
		return ErrCapacity
	}
	b.Put(key, raw)
	return nil
}
func key(prefix, id string) []byte { return []byte(prefix + ":" + id) }
func seqKey(n uint64) []byte {
	b := make([]byte, 10)
	copy(b, "s:")
	binary.BigEndian.PutUint64(b[2:], n)
	return b
}
func mergeSource(m *metadata, v gymstable.EntrySourceInfo) error {
	if v.StoreID != m.Pin.StableStoreID || v.Pin != m.Pin.StablePin || v.Checkpoint < m.ObservedCheckpoint || v.ObservedSourceCount < m.ObservedSourceCount || v.RosterRevision < m.ObservedRosterRevision {
		return ErrCorrupt
	}
	m.ObservedCheckpoint = v.Checkpoint
	m.ObservedSourceCount = v.ObservedSourceCount
	m.ObservedRosterRevision = v.RosterRevision
	return nil
}
func (s *Store) sampleLocked(lower int64) (int64, error) {
	n := s.now().UnixMilli()
	if !timestamp(n) || n < s.clockSeen || n < s.meta.ClockMS || n < lower {
		return 0, ErrClock
	}
	s.clockSeen = n
	return n, nil
}

// Observation-only changes survive definite rejection. An absent event is never
// allocated merely because its cutoff has passed.
func (s *Store) observeLocked(event *Event, lower int64, info *gymstable.EntrySourceInfo) (int64, error) {
	n, e := s.sampleLocked(lower)
	if e != nil {
		return 0, e
	}
	return s.observeAtLocked(n, event, info)
}
func (s *Store) observeAtLocked(n int64, event *Event, info *gymstable.EntrySourceInfo) (int64, error) {
	if !timestamp(n) || n < s.meta.ClockMS {
		return 0, ErrClock
	}
	m := s.meta
	m.ClockMS = n
	if info != nil {
		if e := mergeSource(&m, *info); e != nil {
			return 0, e
		}
	}
	b := new(leveldb.Batch)
	if event != nil {
		r, exists, e := s.event(*event)
		if e != nil {
			return 0, e
		}
		if exists && n >= r.Event.CutoffMS && r.Phase == "accepting" {
			r.Phase = "closed_pending_lock"
			r.ClosedObservedAtMS = n
			if e = put(b, key("e", r.Event.EventID), r, MaxEventSize); e != nil {
				return 0, e
			}
		}
	}
	if m != s.meta || b.Len() > 0 {
		if e := s.commitLocked(b, m); e != nil {
			return 0, e
		}
	}
	return n, nil
}
func (s *Store) event(ev Event) (eventRow, bool, error) {
	raw, e := s.db.Get(key("e", ev.EventID), nil)
	if errors.Is(e, leveldb.ErrNotFound) {
		id, _ := EntryID(s.meta.StoreID, ev.EventID, s.meta.Pin.StablePin.Identity.GymID)
		return eventRow{Version: Version1, Event: ev, EntryID: id, Phase: "accepting"}, false, nil
	}
	var r eventRow
	if e != nil || decode(raw, MaxEventSize, &r, true) != nil || validateEventRow(r, s.meta) != nil || r.Event != ev {
		return r, false, ErrCorrupt
	}
	return r, true, nil
}
func (s *Store) operation(id string) (Operation, error) {
	if !hashID(id) {
		return Operation{}, ErrInvalid
	}
	raw, e := s.db.Get(key("o", id), nil)
	if errors.Is(e, leveldb.ErrNotFound) {
		return Operation{}, ErrNotFound
	}
	var v Operation
	if e != nil || decode(raw, MaxOperationSize, &v, true) != nil || validateOperation(v) != nil || v.Intent.OperationID != id || validateBoundOperation(v, s.meta) != nil {
		return Operation{}, ErrCorrupt
	}
	return v, nil
}
func mapped(e error) error {
	if e == nil {
		return nil
	}
	switch {
	case errors.Is(e, context.Canceled), errors.Is(e, context.DeadlineExceeded), errors.Is(e, gymstable.ErrCancelled):
		return ErrCancelled
	case errors.Is(e, gymaccount.ErrUnauthorized), errors.Is(e, gymaccount.ErrExpired), errors.Is(e, gymstable.ErrUnauthorized):
		return ErrUnauthorized
	case errors.Is(e, gymaccount.ErrForbidden), errors.Is(e, gymaccount.ErrHeld), errors.Is(e, gymstable.ErrForbidden):
		return ErrForbidden
	case errors.Is(e, gymstable.ErrStaleView):
		return ErrStaleView
	case errors.Is(e, gymstable.ErrCorrupt):
		return ErrCorrupt
	case errors.Is(e, gymstable.ErrClock), errors.Is(e, gymaccount.ErrClock):
		return ErrClock
	case errors.Is(e, gymstable.ErrSyncRequired), errors.Is(e, gymstable.ErrNotFound):
		return ErrSyncRequired
	case errors.Is(e, gymstable.ErrConflict):
		return ErrConflict
	case errors.Is(e, gymstable.ErrClosed), errors.Is(e, gymstable.ErrPoisoned):
		return ErrUnavailable
	case errors.Is(e, gymstable.ErrOutcomeUnknown):
		return ErrOutcomeUnknown
	case errors.Is(e, gymstable.ErrCapacity):
		return ErrCapacity
	case errors.Is(e, gymstable.ErrInvalid):
		return ErrInvalid
	}
	return e
}
func (s *Store) withLease(ctx context.Context, session gymstable.Session, cap gymaccount.Capability, fn func(func() error) error) error {
	var guard sync.Mutex
	active, called := true, false
	var callbackErr error
	e := session.Lease(ctx, cap, func(a gymaccount.Account, recheck func() error) error {
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
			if a.ID != s.meta.Pin.StablePin.AccountID || a.Generation != session.Generation {
				return ErrStaleView
			}
			if a.Enrollment == nil || *a.Enrollment != s.meta.Pin.StablePin.Identity || a.RecoveryRequired || recheck == nil {
				return ErrForbidden
			}
			return mapped(recheck())
		}
		if callbackErr = check(); callbackErr != nil {
			return callbackErr
		}
		if fn != nil {
			callbackErr = fn(check)
		}
		return callbackErr
	})
	guard.Lock()
	active = false
	did := called
	guard.Unlock()
	if !did && e == nil {
		return ErrUnauthorized
	}
	if callbackErr != nil {
		return mapped(callbackErr)
	}
	if ctx.Err() != nil {
		return ErrCancelled
	}
	return mapped(e)
}
func (s *Store) begin(ctx context.Context, session gymstable.Session, cap gymaccount.Capability) (context.Context, func(), error) {
	if e := normal(); e != nil {
		return nil, nil, e
	}
	if s == nil || ctx == nil || session.Lease == nil || session.Generation == 0 || session.Generation > MaxSafeInteger || !hashID(session.ViewID) {
		return nil, nil, ErrUnauthorized
	}
	work, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(s.life, cancel)
	finish := func() { stop(); cancel() }
	select {
	case <-work.Done():
		finish()
		return nil, nil, ErrCancelled
	case s.gate <- struct{}{}:
	}
	done := func() { <-s.gate; finish() }
	s.mu.Lock()
	e := s.stateLocked()
	s.mu.Unlock()
	if e != nil {
		done()
		return nil, nil, e
	}
	info, e := sourceInfo(work, s.source, s.meta.Pin)
	if e != nil {
		done()
		return nil, nil, e
	}
	e = s.withLease(work, session, cap, func(check func() error) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		if e := check(); e != nil {
			return e
		}
		_, e := s.observeLocked(nil, 0, &info)
		return e
	})
	if e != nil {
		done()
		return nil, nil, e
	}
	return work, done, nil
}
func (s *Store) randomID() (string, error) {
	for i := 0; i < 8; i++ {
		var b [32]byte
		if _, e := io.ReadFull(s.random, b[:]); e != nil {
			return "", ErrUnavailable
		}
		id := hex.EncodeToString(b[:])
		exists, e := s.db.Has(key("o", id), nil)
		if e != nil {
			return "", ErrCorrupt
		}
		if !exists {
			return id, nil
		}
	}
	return "", ErrCapacity
}
