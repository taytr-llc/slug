// Copyright (c) 2026 The SLUG developers. ISC license.
package gymseed

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
	"github.com/taytr-llc/slug/gymevent"
)

const ownedMarkerName = "SLUG-OWNED-SEED-V1.json"

type ownedMarker struct {
	Format  string   `json:"format"`
	Version uint32   `json:"version"`
	StoreID string   `json:"store_id"`
	Pin     OwnedPin `json:"pin"`
}
type ownedMetadata struct {
	Version         uint32        `json:"version"`
	StoreID         string        `json:"store_id"`
	Pin             OwnedPin      `json:"pin"`
	CreatedAtMS     int64         `json:"created_at_ms"`
	ClockMS         int64         `json:"clock_ms"`
	EventCount      uint32        `json:"event_count"`
	FundingCursor   Cursor        `json:"funding_cursor"`
	FundingRestarts []ScanRestart `json:"funding_restarts"`
	FundingComplete bool          `json:"funding_complete"`
	LastFundingTip  *Tip          `json:"last_funding_tip"`
}
type ownedInitIntent struct {
	Version uint32   `json:"version"`
	StoreID string   `json:"store_id"`
	Pin     OwnedPin `json:"pin"`
}

// Store owns only its seed journal. Event, entry, session and publisher
// handles are borrowed per call and never retained. Order is seed -> event -> entry.
type OwnedStore struct {
	gate              chan struct{}
	mu                sync.Mutex
	life              context.Context
	cancel            context.CancelFunc
	closeOnce         sync.Once
	closed, poisoned  bool
	readOnly          bool
	closeErr          error
	db                *leveldb.DB
	meta              ownedMetadata
	clockSeen         int64
	sourceCreatedAtMS int64
	now               func() time.Time
	writeBatch        func(*leveldb.Batch, *opt.WriteOptions) error
	checkpoint        func(string)
}

func ownedNewStore(db *leveldb.DB, clock func() time.Time) *OwnedStore {
	life, cancel := context.WithCancel(context.Background())
	return &OwnedStore{gate: make(chan struct{}, 1), life: life, cancel: cancel, db: db, now: clock, writeBatch: db.Write}
}
func (s *OwnedStore) mark(v string) {
	if s.checkpoint != nil {
		s.checkpoint(v)
	}
}
func ownedBoundedInventory(path string, allowMissingDB bool) error {
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
		case ownedMarkerName:
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
func ownedReadMarker(path string, p OwnedPin) (ownedMarker, error) {
	var m ownedMarker
	if e := ownedBoundedInventory(path, true); e != nil {
		return m, e
	}
	name := filepath.Join(path, ownedMarkerName)
	before, e := os.Lstat(name)
	if e != nil || !before.Mode().IsRegular() || before.Size() < 2 || before.Size() > OwnedMaxPinSize {
		return m, ErrCorrupt
	}
	f, e := os.Open(name)
	if e != nil {
		return m, ErrCorrupt
	}
	after, e := f.Stat()
	if e != nil || !after.Mode().IsRegular() || after.Size() > OwnedMaxPinSize || !os.SameFile(before, after) {
		f.Close()
		return m, ErrCorrupt
	}
	raw, e := io.ReadAll(io.LimitReader(f, OwnedMaxPinSize+1))
	e = errors.Join(e, f.Close())
	if e != nil || len(raw) < 2 || len(raw) > OwnedMaxPinSize || raw[len(raw)-1] != '\n' || ownedDecode(raw[:len(raw)-1], OwnedMaxPinSize, &m, true) != nil {
		return m, ErrCorrupt
	}
	id, _ := OwnedID(p)
	if !equal(m, ownedMarker{"slug-owned-seed", 1, id, p}) {
		return m, ErrCorrupt
	}
	return m, nil
}
func CreateOwned(ctx context.Context, path string, pin OwnedPin, events *gymevent.OwnedStore) (*OwnedStore, error) {
	return ownedCreate(ctx, path, pin, events, time.Now, nil)
}
func ownedCreate(ctx context.Context, path string, pin OwnedPin, events *gymevent.OwnedStore, clock func() time.Time, checkpoint func(string)) (*OwnedStore, error) {
	if e := normal(); e != nil {
		return nil, e
	}
	if ctx == nil || clock == nil {
		return nil, ErrInvalid
	}
	p, e := ValidateOwnedPin(pin)
	if e != nil {
		return nil, e
	}
	source, e := ownedCheckSource(ctx, events, p)
	if e != nil {
		return nil, e
	}
	if !protectedPath(path) {
		return nil, ErrInvalid
	}
	if ctx.Err() != nil {
		return nil, ErrCancelled
	}
	id, _ := OwnedID(p)
	m := ownedMarker{"slug-owned-seed", 1, id, p}
	raw, _ := json.Marshal(m)
	if len(raw)+1 > OwnedMaxPinSize {
		return nil, ErrCapacity
	}
	if e = os.Mkdir(path, 0700); e != nil {
		if !errors.Is(e, os.ErrExist) || !emptyDirectory(path) {
			return nil, ErrUnavailable
		}
	}
	f, e := os.OpenFile(filepath.Join(path, ownedMarkerName), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
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
	s := ownedNewStore(db, clock)
	s.sourceCreatedAtMS = source.CreatedAtMS
	s.checkpoint = checkpoint
	if e = s.initialize(ctx, path, p); e != nil {
		return ownedFailed(s, e)
	}
	return s, nil
}
func (s *OwnedStore) initialize(ctx context.Context, path string, p OwnedPin) error {
	id, _ := OwnedID(p)
	v := ownedInitIntent{1, id, p}
	b := new(leveldb.Batch)
	if e := put(b, []byte("init"), v, OwnedMaxMetadataSize); e != nil {
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
	if !ownedTimestamp(now) || now < s.sourceCreatedAtMS {
		return ErrClock
	}
	if ctx.Err() != nil {
		return ErrCancelled
	}
	s.meta = ownedMetadata{Version: 1, StoreID: id, Pin: p, CreatedAtMS: now, ClockMS: now, FundingCursor: initialOwnedCursor(p, "", 0), FundingRestarts: []ScanRestart{}}
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
func OpenOwned(ctx context.Context, path string, expected OwnedPin, events *gymevent.OwnedStore) (*OwnedStore, error) {
	return ownedOpen(ctx, path, expected, events, time.Now, false)
}
func ResumeOwnedPrepared(ctx context.Context, path string, expected OwnedPin, events *gymevent.OwnedStore) (*OwnedStore, error) {
	return ownedOpen(ctx, path, expected, events, time.Now, true)
}

// OpenAudit opens an established journal with complete validation and a
// physically read-only database; it never creates or resumes partial state.
func OpenOwnedAudit(ctx context.Context, path string, expected OwnedPin, events *gymevent.OwnedStore) (*OwnedStore, error) {
	return ownedOpenAudit(ctx, path, expected, events, time.Now, false, true)
}
func ownedOpen(ctx context.Context, path string, expected OwnedPin, events *gymevent.OwnedStore, clock func() time.Time, resume bool) (*OwnedStore, error) {
	return ownedOpenAudit(ctx, path, expected, events, clock, resume, false)
}
func ownedOpenAudit(ctx context.Context, path string, expected OwnedPin, events *gymevent.OwnedStore, clock func() time.Time, resume, readOnly bool) (*OwnedStore, error) {
	if e := normal(); e != nil {
		return nil, e
	}
	if ctx == nil || clock == nil {
		return nil, ErrInvalid
	}
	p, e := ValidateOwnedPin(expected)
	if e != nil {
		return nil, e
	}
	source, e := ownedCheckSource(ctx, events, p)
	if e != nil {
		return nil, e
	}
	if resume {
		if _, statErr := os.Lstat(path); errors.Is(statErr, os.ErrNotExist) || emptyDirectory(path) {
			return ownedCreate(ctx, path, p, events, clock, nil)
		}
	}
	m, e := ownedReadMarker(path, p)
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
	db, e := leveldb.OpenFile(dbpath, &opt.Options{Strict: opt.StrictAll, ErrorIfMissing: !createDB, ErrorIfExist: createDB, ReadOnly: readOnly})
	if e != nil {
		return nil, ErrUnavailable
	}
	s := ownedNewStore(db, clock)
	s.readOnly = readOnly
	s.sourceCreatedAtMS = source.CreatedAtMS
	fail := func(e error) (*OwnedStore, error) { return ownedFailed(s, e) }
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
			if len(it.Key()) > 4 || len(it.Value()) > OwnedMaxMetadataSize {
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
				var in ownedInitIntent
				if ownedDecode(raw, OwnedMaxMetadataSize, &in, true) != nil || !equal(in, ownedInitIntent{1, m.StoreID, p}) {
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
	if resume && (s.meta.EventCount != 0 || s.meta.FundingCursor.Height != -1 || s.meta.FundingCursor.Generation != 0 || len(s.meta.FundingRestarts) != 0 || s.meta.LastFundingTip != nil) {
		return fail(ErrCorrupt)
	}
	if ctx.Err() != nil {
		return fail(ErrCancelled)
	}
	return s, nil
}
func (s *OwnedStore) stateLocked() error {
	if s.closed {
		return ErrClosed
	}
	if s.poisoned {
		return ErrPoisoned
	}
	return nil
}
func (s *OwnedStore) begin(ctx context.Context) (context.Context, func(), error) {
	if e := normal(); e != nil {
		return nil, nil, e
	}
	if s == nil || ctx == nil {
		return nil, nil, ErrInvalid
	}
	if s.life.Err() != nil {
		return nil, nil, ErrClosed
	}
	if ctx.Err() != nil {
		return nil, nil, ErrCancelled
	}
	work, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(s.life, cancel)
	finish := func() { stop(); cancel() }
	select {
	case <-work.Done():
		finish()
		if s.life.Err() != nil {
			return nil, nil, ErrClosed
		}
		return nil, nil, ErrCancelled
	case s.gate <- struct{}{}:
	default:
		finish()
		return nil, nil, ErrBusy
	}
	done := func() { <-s.gate; finish() }
	s.mu.Lock()
	e := s.stateLocked()
	s.mu.Unlock()
	if e == nil && s.life.Err() != nil {
		e = ErrClosed
	}
	if e == nil && ctx.Err() != nil {
		e = ErrCancelled
	}
	if e != nil {
		done()
		return nil, nil, e
	}
	return work, done, nil
}
func (s *OwnedStore) Close() error {
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
func (s *OwnedStore) Info(ctx context.Context) (OwnedInfo, error) {
	work, done, e := s.begin(ctx)
	if e != nil {
		return OwnedInfo{}, e
	}
	defer done()
	if work.Err() != nil {
		return OwnedInfo{}, ErrCancelled
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	m := s.meta
	return ValidateOwnedInfo(m.Pin, OwnedInfo{Version: 1, StoreID: m.StoreID, Pin: m.Pin, CreatedAtMS: m.CreatedAtMS, EventCount: m.EventCount, FundingHeight: m.FundingCursor.Height, FundingHash: m.FundingCursor.Hash, FundingComplete: m.FundingComplete, FundingGeneration: m.FundingCursor.Generation, FundingRestartCount: uint32(len(m.FundingRestarts))})
}
func (s *OwnedStore) commitLocked(b *leveldb.Batch, next ownedMetadata) error {
	if s.readOnly {
		return ErrForbidden
	}
	if e := put(b, []byte("meta"), next, OwnedMaxMetadataSize); e != nil {
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
func (s *OwnedStore) raw(k []byte, limit int) ([]byte, bool, error) {
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
func (s *OwnedStore) sampleLocked(lower int64) (int64, error) {
	n := s.now().UnixMilli()
	if !ownedTimestamp(n) || n < s.clockSeen || n < s.meta.ClockMS || n < lower {
		return 0, ErrClock
	}
	s.clockSeen = n
	return n, nil
}
func (s *OwnedStore) observeLocked(lower int64) (int64, error) {
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

func ownedCheckSource(ctx context.Context, events *gymevent.OwnedStore, p OwnedPin) (gymevent.OwnedInfo, error) {
	if ctx == nil || events == nil {
		return gymevent.OwnedInfo{}, ErrInvalid
	}
	v, e := events.Info(ctx)
	if e != nil {
		return v, ownedMapped(e)
	}
	if v.Version != 1 || v.StoreID != p.SourceStoreID || !equal(v.Pin, p.SourcePin) || !ownedTimestamp(v.CreatedAtMS) {
		return v, ErrForbidden
	}
	return v, nil
}
