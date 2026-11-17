// Copyright (c) 2026 The SLUG developers. ISC license.
package gymevent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/wire/v2"
)

const markerName = "SLUG-CANDIDATE-EVENTS.json"

type marker struct {
	Application string `json:"application"`
	Version     uint32 `json:"version"`
	StoreID     string `json:"store_id"`
	Pin         Pin    `json:"pin"`
}
type metadata struct {
	Version     uint32 `json:"version"`
	StoreID     string `json:"store_id"`
	Pin         Pin    `json:"pin"`
	CreatedAtMS int64  `json:"created_at_ms"`
	ClockMS     int64  `json:"clock_ms"`
	EventCount  uint32 `json:"event_count"`
}
type initIntent struct {
	Version uint32 `json:"version"`
	StoreID string `json:"store_id"`
	Pin     Pin    `json:"pin"`
}
type eventRow struct {
	Version        uint32 `json:"version"`
	Event          Event  `json:"event"`
	Phase          string `json:"phase"`
	CreatedAtMS    int64  `json:"created_at_ms"`
	CompletedAtMS  int64  `json:"completed_at_ms"`
	SnapshotDigest string `json:"snapshot_digest"`
}

// Store owns its event journal only. Source handles are borrowed for each call;
// no source or session survives the call. Order is event -> entry -> stable.
type Store struct {
	gate             chan struct{}
	mu               sync.Mutex
	life             context.Context
	cancel           context.CancelFunc
	closeOnce        sync.Once
	closed, poisoned bool
	closeErr         error
	db               *leveldb.DB
	meta             metadata
	clockSeen        int64
	now              func() time.Time
	writeBatch       func(*leveldb.Batch, *opt.WriteOptions) error
	checkpoint       func(string)
}

func normal() error {
	if wire.TestPoW {
		return ErrProfile
	}
	return nil
}
func newStore(db *leveldb.DB, clock func() time.Time) *Store {
	life, cancel := context.WithCancel(context.Background())
	return &Store{gate: make(chan struct{}, 1), life: life, cancel: cancel, db: db, now: clock, writeBatch: db.Write}
}
func (s *Store) mark(v string) {
	if s.checkpoint != nil {
		s.checkpoint(v)
	}
}
func realDirectory(path string) bool {
	f, e := os.Lstat(path)
	return e == nil && f.IsDir() && f.Mode()&os.ModeSymlink == 0
}
func protectedPath(path string) bool {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return false
	}
	for p := filepath.Dir(path); ; p = filepath.Dir(p) {
		if !realDirectory(p) {
			return false
		}
		if p == filepath.Dir(p) {
			break
		}
	}
	return true
}
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
	if e != nil || !os.SameFile(before, after) {
		return false
	}
	names, e := f.ReadDir(1)
	return len(names) == 0 && e == io.EOF
}
func syncDir(path string) error {
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	return errors.Join(f.Sync(), f.Close())
}
func syncTree(path string) error {
	for _, p := range []string{filepath.Join(path, "db"), path, filepath.Dir(path)} {
		if e := syncDir(p); e != nil {
			return e
		}
	}
	return nil
}
func boundedInventory(path string, allowMissingDB bool) error {
	if !protectedPath(path) || !realDirectory(path) {
		return ErrCorrupt
	}
	f, e := os.Open(path)
	if e != nil {
		return ErrCorrupt
	}
	defer f.Close()
	entries, e := f.ReadDir(3)
	if e != nil && e != io.EOF {
		return ErrCorrupt
	}
	if len(entries) > 2 {
		return ErrCorrupt
	}
	markerSeen, dbSeen := false, false
	for _, x := range entries {
		switch x.Name() {
		case markerName:
			markerSeen = true
		case "db":
			dbSeen = true
			if !realDirectory(filepath.Join(path, "db")) {
				return ErrCorrupt
			}
		default:
			return ErrCorrupt
		}
	}
	if !markerSeen || (!dbSeen && !allowMissingDB) {
		return ErrCorrupt
	}
	return nil
}
func readMarker(path string, p Pin) (marker, error) {
	var m marker
	if e := boundedInventory(path, true); e != nil {
		return m, e
	}
	name := filepath.Join(path, markerName)
	before, e := os.Lstat(name)
	if e != nil || !before.Mode().IsRegular() || before.Size() < 2 || before.Size() > MaxMarkerSize {
		return m, ErrCorrupt
	}
	f, e := os.Open(name)
	if e != nil {
		return m, ErrCorrupt
	}
	after, e := f.Stat()
	if e != nil || !after.Mode().IsRegular() || after.Size() > MaxMarkerSize || !os.SameFile(before, after) {
		f.Close()
		return m, ErrCorrupt
	}
	raw, e := io.ReadAll(io.LimitReader(f, MaxMarkerSize+1))
	e = errors.Join(e, f.Close())
	if e != nil || len(raw) < 2 || len(raw) > MaxMarkerSize || raw[len(raw)-1] != '\n' || decode(raw[:len(raw)-1], MaxMarkerSize, &m, true) != nil {
		return m, ErrCorrupt
	}
	id, _ := ID(p)
	if !equal(m, marker{"slug-candidate-events", 1, id, p}) {
		return m, ErrCorrupt
	}
	return m, nil
}
func Create(ctx context.Context, path string, pin Pin) (*Store, error) {
	return create(ctx, path, pin, time.Now, nil)
}
func create(ctx context.Context, path string, pin Pin, clock func() time.Time, checkpoint func(string)) (*Store, error) {
	if e := normal(); e != nil {
		return nil, e
	}
	if ctx == nil || clock == nil {
		return nil, ErrInvalid
	}
	p, e := ValidatePin(pin)
	if e != nil {
		return nil, e
	}
	if !protectedPath(path) {
		return nil, ErrInvalid
	}
	if ctx.Err() != nil {
		return nil, ErrCancelled
	}
	id, _ := ID(p)
	m := marker{"slug-candidate-events", 1, id, p}
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
	e = errors.Join(e, f.Sync(), f.Close())
	if e != nil {
		return nil, ErrOutcomeUnknown
	}
	for _, d := range []string{path, filepath.Dir(path)} {
		if syncDir(d) != nil {
			return nil, ErrOutcomeUnknown
		}
	}
	if checkpoint != nil {
		checkpoint("marker-synced")
	}
	dbpath := filepath.Join(path, "db")
	if os.Mkdir(dbpath, 0700) != nil {
		return nil, ErrUnavailable
	}
	db, e := leveldb.OpenFile(dbpath, &opt.Options{Strict: opt.StrictAll, ErrorIfExist: true})
	if e != nil {
		return nil, ErrUnavailable
	}
	s := newStore(db, clock)
	s.checkpoint = checkpoint
	if e = s.initialize(ctx, path, p); e != nil {
		s.Close()
		return nil, e
	}
	return s, nil
}
func (s *Store) initialize(ctx context.Context, path string, p Pin) error {
	id, _ := ID(p)
	v := initIntent{1, id, p}
	b := new(leveldb.Batch)
	if e := put(b, []byte("init"), v, MaxMetadataSize); e != nil {
		return e
	}
	if e := s.writeBatch(b, &opt.WriteOptions{Sync: true}); e != nil {
		s.poisoned = true
		return ErrOutcomeUnknown
	}
	if syncTree(path) != nil {
		return ErrOutcomeUnknown
	}
	s.mark("init-synced")
	// Pin is durable before this sample; filesystem delay can never backdate it.
	now := s.now().UnixMilli()
	if !timestamp(now) {
		return ErrClock
	}
	if ctx.Err() != nil {
		return ErrCancelled
	}
	s.meta = metadata{Version: 1, StoreID: id, Pin: p, CreatedAtMS: now, ClockMS: now}
	s.clockSeen = now
	b = new(leveldb.Batch)
	b.Delete([]byte("init"))
	s.mark("before-initialize")
	if e := s.commitLocked(b, s.meta); e != nil {
		return e
	}
	s.mark("initialized")
	if syncTree(path) != nil || ctx.Err() != nil {
		return ErrOutcomeUnknown
	}
	return nil
}
func Open(ctx context.Context, path string, expected Pin) (*Store, error) {
	return open(ctx, path, expected, time.Now, false)
}
func ResumePrepared(ctx context.Context, path string, expected Pin) (*Store, error) {
	return open(ctx, path, expected, time.Now, true)
}
func open(ctx context.Context, path string, expected Pin, clock func() time.Time, resume bool) (*Store, error) {
	if e := normal(); e != nil {
		return nil, e
	}
	if ctx == nil || clock == nil {
		return nil, ErrInvalid
	}
	p, e := ValidatePin(expected)
	if e != nil {
		return nil, e
	}
	m, e := readMarker(path, p)
	if e != nil {
		return nil, e
	}
	if ctx.Err() != nil {
		return nil, ErrCancelled
	}
	dbpath := filepath.Join(path, "db")
	createDB := false
	if _, e = os.Lstat(dbpath); errors.Is(e, os.ErrNotExist) {
		if !resume {
			return nil, ErrCorrupt
		}
		if os.Mkdir(dbpath, 0700) != nil {
			return nil, ErrUnavailable
		}
		createDB = true
	} else if e != nil || !realDirectory(dbpath) {
		return nil, ErrCorrupt
	} else if emptyDirectory(dbpath) {
		if !resume {
			return nil, ErrCorrupt
		}
		createDB = true
	}
	db, e := leveldb.OpenFile(dbpath, &opt.Options{Strict: opt.StrictAll, ErrorIfMissing: !createDB, ErrorIfExist: createDB})
	if e != nil {
		return nil, ErrUnavailable
	}
	s := newStore(db, clock)
	fail := func(e error) (*Store, error) { s.Close(); return nil, e }
	if resume {
		it := db.NewIterator(nil, nil)
		count := 0
		kind := ""
		var raw []byte
		for it.Next() {
			count++
			if count > 1 {
				break
			}
			if len(it.Key()) > 4 || len(it.Value()) > MaxMetadataSize {
				count = 2
				break
			}
			kind = string(it.Key())
			raw = append([]byte(nil), it.Value()...)
		}
		e = it.Error()
		it.Release()
		if e != nil || count > 1 {
			return fail(ErrCorrupt)
		}
		if count == 0 || kind == "init" {
			if kind == "init" {
				var in initIntent
				if decode(raw, MaxMetadataSize, &in, true) != nil || !equal(in, initIntent{1, m.StoreID, p}) {
					return fail(ErrCorrupt)
				}
			}
			if e = s.initialize(ctx, path, p); e != nil {
				return fail(e)
			}
			return s, nil
		}
		if kind != "meta" {
			return fail(ErrCorrupt)
		}
	}
	if e = s.audit(ctx, m); e != nil {
		return fail(e)
	}
	if resume && s.meta.EventCount != 0 {
		return fail(ErrCorrupt)
	}
	if ctx.Err() != nil {
		return fail(ErrCancelled)
	}
	return s, nil
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
func (s *Store) begin(ctx context.Context) (context.Context, func(), error) {
	if e := normal(); e != nil {
		return nil, nil, e
	}
	if s == nil || ctx == nil {
		return nil, nil, ErrInvalid
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
	return work, done, nil
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
func (s *Store) Info(ctx context.Context) (Info, error) {
	work, done, e := s.begin(ctx)
	if e != nil {
		return Info{}, e
	}
	defer done()
	s.mu.Lock()
	defer s.mu.Unlock()
	if work.Err() != nil {
		return Info{}, ErrCancelled
	}
	v := Info{1, s.meta.StoreID, s.meta.Pin, s.meta.CreatedAtMS, s.meta.EventCount}
	return ValidateInfo(s.meta.Pin, v)
}
func (s *Store) commitLocked(b *leveldb.Batch, next metadata) error {
	if e := put(b, []byte("meta"), next, MaxMetadataSize); e != nil {
		return e
	}
	if e := s.writeBatch(b, &opt.WriteOptions{Sync: true}); e != nil {
		s.poisoned = true
		return ErrOutcomeUnknown
	}
	s.meta = next
	s.clockSeen = max(s.clockSeen, next.ClockMS)
	return nil
}
func put(b *leveldb.Batch, k []byte, v any, limit int) error {
	raw, e := json.Marshal(v)
	if e != nil || len(raw) > limit {
		return ErrCapacity
	}
	b.Put(k, raw)
	return nil
}
func key(prefix byte, ids ...string) []byte {
	k := []byte{prefix}
	for _, id := range ids {
		k = append(k, rawID(id)...)
	}
	return k
}
func (s *Store) raw(k []byte, limit int) ([]byte, bool, error) {
	it := s.db.NewIterator(nil, nil)
	defer it.Release()
	if !it.Seek(k) || !bytes.Equal(it.Key(), k) {
		if it.Error() != nil {
			return nil, false, ErrCorrupt
		}
		return nil, false, nil
	}
	if len(it.Value()) == 0 || len(it.Value()) > limit {
		return nil, false, ErrCorrupt
	}
	return append([]byte(nil), it.Value()...), true, nil
}
func (s *Store) sampleLocked(lower int64) (int64, error) {
	n := s.now().UnixMilli()
	if !timestamp(n) || n < s.clockSeen || n < s.meta.ClockMS || n < lower {
		return 0, ErrClock
	}
	s.clockSeen = n
	return n, nil
}
func (s *Store) observeLocked(lower int64) (int64, error) {
	n, e := s.sampleLocked(lower)
	if e != nil {
		return 0, e
	}
	if n != s.meta.ClockMS {
		m := s.meta
		m.ClockMS = n
		if e = s.commitLocked(new(leveldb.Batch), m); e != nil {
			return 0, e
		}
	}
	return n, nil
}
func mapped(e error) error {
	switch {
	case e == nil:
		return nil
	case errors.Is(e, context.Canceled), errors.Is(e, context.DeadlineExceeded), errors.Is(e, gymentry.ErrCancelled):
		return ErrCancelled
	case errors.Is(e, gymentry.ErrUnauthorized):
		return ErrUnauthorized
	case errors.Is(e, gymentry.ErrForbidden):
		return ErrForbidden
	case errors.Is(e, gymentry.ErrStaleView):
		return ErrStaleView
	case errors.Is(e, gymentry.ErrCorrupt):
		return ErrCorrupt
	case errors.Is(e, gymentry.ErrClock):
		return ErrClock
	case errors.Is(e, gymentry.ErrCalendar):
		return ErrCalendar
	case errors.Is(e, gymentry.ErrConflict), errors.Is(e, gymentry.ErrSyncRequired):
		return ErrConflict
	case errors.Is(e, gymentry.ErrOutcomeUnknown):
		return ErrOutcomeUnknown
	case errors.Is(e, gymentry.ErrUnavailable), errors.Is(e, gymentry.ErrClosed), errors.Is(e, gymentry.ErrPoisoned):
		return ErrUnavailable
	case errors.Is(e, gymentry.ErrInvalid):
		return ErrInvalid
	case errors.Is(e, gymentry.ErrCapacity):
		return ErrCapacity
	}
	return e
}
func (s *Store) sourceView(ctx context.Context, source *gymentry.Store, session gymstable.Session, fn func(gymentry.SnapshotSourceInfo, func() error) error) error {
	if source == nil {
		return ErrUnavailable
	}
	return mapped(source.WithSnapshotView(ctx, session, func(v gymentry.SnapshotSourceInfo, check func() error) error {
		p, e := participant(s.meta.Pin, v.Pin.StablePin.Identity.GymID)
		if e != nil || !equal(p, v.Pin) {
			return ErrForbidden
		}
		id, _ := gymentry.ID(p)
		if v.Version != 1 || v.StoreID != id || !timestamp(v.ObservedAtMS) || check == nil {
			return ErrCorrupt
		}
		return fn(v, check)
	}))
}
