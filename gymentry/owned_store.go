// Copyright (c) 2026 The SLUG developers. ISC license.
package gymentry

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/fighterownership"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/internal/ownedcodec"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const ownedMarkerName = "SLUG-OWNED-ENTRY-V1.json"

type ownedMarker struct {
	Format  string   `json:"format"`
	Version uint32   `json:"version"`
	StoreID string   `json:"store_id"`
	Pin     OwnedPin `json:"pin"`
}
type ownedMetadata struct {
	Version            uint32   `json:"version"`
	StoreID            string   `json:"store_id"`
	Pin                OwnedPin `json:"pin"`
	EventCount         uint32   `json:"event_count"`
	OperationCount     uint32   `json:"operation_count"`
	PendingOperationID string   `json:"pending_operation_id"`
	LastOperationID    string   `json:"last_operation_id"`
	ClockMS            int64    `json:"clock_ms"`
}

// The public intent grammar remains unchanged. This private preparation pins
// the exact source states selected before entry admission, including on reopen.
type ownedPreparation struct {
	Version     uint32                   `json:"version"`
	OperationID string                   `json:"operation_id"`
	Head        fighterownership.HeadRef `json:"head"`
	StateIDs    []string                 `json:"state_ids"`
}
type OwnedStore struct {
	gate                       chan struct{}
	life                       context.Context
	cancel                     context.CancelFunc
	closeOnce                  sync.Once
	closeErr                   error
	closed, poisoned, readOnly bool
	db                         *leveldb.DB
	source                     *gymstable.OwnedStore
	meta                       ownedMetadata
	now                        func() time.Time
	writeBatch                 func(*leveldb.Batch, *opt.WriteOptions) error
}
type OwnedCleanupError struct {
	Owner *OwnedStore
	cause error
}

func (e *OwnedCleanupError) Error() string { return "owned entry cleanup failed: " + e.cause.Error() }
func (e *OwnedCleanupError) Unwrap() error { return e.cause }
func ownedFailure(s *OwnedStore, e error) (*OwnedStore, error) {
	if ce := s.Close(); ce != nil {
		return nil, &OwnedCleanupError{s, errors.Join(e, ce)}
	}
	return nil, e
}
func newOwned(db *leveldb.DB, source *gymstable.OwnedStore, readOnly bool) *OwnedStore {
	ctx, cancel := context.WithCancel(context.Background())
	return &OwnedStore{gate: make(chan struct{}, 1), life: ctx, cancel: cancel, db: db, source: source, now: time.Now, writeBatch: db.Write, readOnly: readOnly}
}
func ownedSource(ctx context.Context, p OwnedPin, source *gymstable.OwnedStore) error {
	if _, e := ValidateOwnedPin(p); e != nil {
		return e
	}
	if source == nil {
		return ErrInvalid
	}
	info, e := source.SourceInfo(ctx)
	if e != nil {
		return e
	}
	if info.StoreID != p.StableStoreID || info.Pin != p.StablePin {
		return ErrConflict
	}
	return nil
}
func CreateOwned(ctx context.Context, path string, p OwnedPin, source *gymstable.OwnedStore) (*OwnedStore, error) {
	if e := normal(); e != nil {
		return nil, e
	}
	if ctx == nil {
		return nil, ErrInvalid
	}
	if e := ownedSource(ctx, p, source); e != nil {
		return nil, e
	}
	if !cleanPath(path) || !directory(filepath.Dir(path)) {
		return nil, ErrInvalid
	}
	id, _ := OwnedID(p)
	m := ownedMarker{"slug-owned-entry", 1, id, p}
	raw, _ := json.Marshal(m)
	if e := os.Mkdir(path, 0700); e != nil {
		return nil, e
	}
	f, e := os.OpenFile(filepath.Join(path, ownedMarkerName), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return nil, e
	}
	_, e = f.Write(append(raw, '\n'))
	if e = errors.Join(e, f.Sync(), f.Close()); e != nil {
		return nil, errors.Join(ErrOutcomeUnknown, e)
	}
	dbpath := filepath.Join(path, "db")
	if e = os.Mkdir(dbpath, 0700); e != nil {
		return nil, e
	}
	db, e := leveldb.OpenFile(dbpath, &opt.Options{Strict: opt.StrictAll, ErrorIfExist: true})
	if e != nil {
		return nil, e
	}
	s := newOwned(db, source, false)
	s.meta = ownedMetadata{Version: 1, StoreID: id, Pin: p, ClockMS: s.now().UnixMilli()}
	if !timestamp(s.meta.ClockMS) {
		return ownedFailure(s, ErrClock)
	}
	if e = s.persist(new(leveldb.Batch), s.meta); e != nil {
		return ownedFailure(s, e)
	}
	for _, dir := range []string{dbpath, path, filepath.Dir(path)} {
		if e = syncDir(dir); e != nil {
			return ownedFailure(s, errors.Join(ErrOutcomeUnknown, e))
		}
	}
	if e = ctx.Err(); e != nil {
		return ownedFailure(s, e)
	}
	return s, nil
}
func OpenOwned(ctx context.Context, path string, p OwnedPin, source *gymstable.OwnedStore) (*OwnedStore, error) {
	return openOwned(ctx, path, p, source, false)
}
func OpenOwnedAudit(ctx context.Context, path string, p OwnedPin, source *gymstable.OwnedStore) (*OwnedStore, error) {
	return openOwned(ctx, path, p, source, true)
}
func openOwned(ctx context.Context, path string, p OwnedPin, source *gymstable.OwnedStore, readOnly bool) (*OwnedStore, error) {
	if e := normal(); e != nil {
		return nil, e
	}
	if ctx == nil {
		return nil, ErrInvalid
	}
	if e := ownedSource(ctx, p, source); e != nil {
		return nil, e
	}
	if !cleanPath(path) || !directory(path) || !directory(filepath.Join(path, "db")) {
		return nil, ErrCorrupt
	}
	entries, e := os.ReadDir(path)
	if e != nil || len(entries) != 2 {
		return nil, ErrCorrupt
	}
	for _, entry := range entries {
		if entry.Name() != ownedMarkerName && entry.Name() != "db" {
			return nil, ErrCorrupt
		}
	}
	name := filepath.Join(path, ownedMarkerName)
	st, e := os.Lstat(name)
	if e != nil || !st.Mode().IsRegular() || st.Size() > OwnedMaxPinSize+1024 {
		return nil, ErrCorrupt
	}
	f, e := os.Open(name)
	if e != nil {
		return nil, e
	}
	actual, e := f.Stat()
	if e != nil || !os.SameFile(st, actual) {
		f.Close()
		return nil, ErrCorrupt
	}
	raw, e := io.ReadAll(io.LimitReader(f, OwnedMaxPinSize+1025))
	e = errors.Join(e, f.Close())
	var marker ownedMarker
	id, _ := OwnedID(p)
	if e != nil || len(raw) < 2 || raw[len(raw)-1] != '\n' || ownedcodec.Decode(raw[:len(raw)-1], OwnedMaxPinSize+1024, &marker) != nil || marker != (ownedMarker{"slug-owned-entry", 1, id, p}) {
		return nil, ErrCorrupt
	}
	db, e := leveldb.OpenFile(filepath.Join(path, "db"), &opt.Options{Strict: opt.StrictAll, ErrorIfMissing: true, ReadOnly: readOnly})
	if e != nil {
		return nil, e
	}
	s := newOwned(db, source, readOnly)
	if e = s.audit(ctx, marker); e != nil {
		return ownedFailure(s, e)
	}
	return s, nil
}
func (s *OwnedStore) enter(ctx context.Context) (context.Context, func(), error) {
	if s == nil || ctx == nil {
		return nil, nil, ErrInvalid
	}
	if e := normal(); e != nil {
		return nil, nil, e
	}
	work, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(s.life, cancel)
	release := func() { stop(); cancel() }
	select {
	case s.gate <- struct{}{}:
	case <-work.Done():
		release()
		return nil, nil, ErrCancelled
	default:
		release()
		return nil, nil, ErrBusy
	}
	done := func() { <-s.gate; release() }
	if s.closed || s.life.Err() != nil {
		done()
		return nil, nil, ErrClosed
	}
	if s.poisoned {
		done()
		return nil, nil, ErrPoisoned
	}
	return work, done, nil
}
func (s *OwnedStore) fence(ctx context.Context, session Session, cap gymaccount.Capability, fn func(func() error) error) error {
	if session.Lease == nil || session.Generation == 0 || session.Generation > MaxSafeInteger || !hashID(session.ViewID) {
		return ErrUnauthorized
	}
	var mu sync.Mutex
	active, called := true, false
	var inner error
	outer := session.Lease(ctx, cap, func(a gymaccount.Account, recheck func() error) error {
		mu.Lock()
		defer mu.Unlock()
		if !active || called {
			inner = ErrUnauthorized
			return inner
		}
		called = true
		check := func() error {
			if ctx.Err() != nil {
				return ErrCancelled
			}
			if s.life.Err() != nil {
				return ErrClosed
			}
			p := s.meta.Pin.StablePin.Stable
			if a.ID != p.AccountID || a.Generation != session.Generation {
				return ErrStaleView
			}
			if a.Enrollment == nil || *a.Enrollment != p.Identity || a.RecoveryRequired || recheck == nil {
				return ErrForbidden
			}
			return mapped(recheck())
		}
		if inner = check(); inner == nil && fn != nil {
			inner = fn(check)
		}
		return inner
	})
	mu.Lock()
	active = false
	seen := called
	mu.Unlock()
	if inner != nil {
		return mapped(inner)
	}
	if !seen && outer == nil {
		return ErrUnauthorized
	}
	if outer != nil {
		return mapped(outer)
	}
	if ctx.Err() != nil {
		return ErrCancelled
	}
	return nil
}
func (s *OwnedStore) begin(ctx context.Context, session Session, cap gymaccount.Capability) (context.Context, func(), error) {
	ctx, done, e := s.enter(ctx)
	if e != nil {
		return nil, nil, e
	}
	if e = s.fence(ctx, session, cap, nil); e != nil {
		done()
		return nil, nil, e
	}
	return ctx, done, nil
}
func (s *OwnedStore) Close() error {
	if s == nil {
		return nil
	}
	s.closeOnce.Do(func() {
		s.cancel()
		s.gate <- struct{}{}
		defer func() { <-s.gate }()
		s.closed = true
		s.closeErr = s.db.Close()
	})
	return s.closeErr
}
func (s *OwnedStore) persist(b *leveldb.Batch, m ownedMetadata) error {
	if s.readOnly {
		return ErrForbidden
	}
	if e := put(b, []byte("meta"), m, OwnedMaxMetadataSize); e != nil {
		return e
	}
	if e := s.writeBatch(b, &opt.WriteOptions{Sync: true}); e != nil {
		s.poisoned = true
		return errors.Join(ErrOutcomeUnknown, e)
	}
	s.meta = m
	return nil
}
func (s *OwnedStore) clock() (int64, error) {
	n := s.now().UnixMilli()
	if !timestamp(n) || n < s.meta.ClockMS {
		return 0, ErrClock
	}
	return n, nil
}
func (s *OwnedStore) row(k []byte, limit int) ([]byte, error) {
	it := s.db.NewIterator(nil, nil)
	defer it.Release()
	if !it.Seek(k) || !bytes.Equal(k, it.Key()) {
		return nil, ErrNotFound
	}
	if len(it.Value()) > limit || it.Error() != nil {
		return nil, ErrCorrupt
	}
	return append([]byte{}, it.Value()...), nil
}
func (s *OwnedStore) operation(id string) (OwnedOperation, error) {
	if !hashID(id) {
		return OwnedOperation{}, ErrInvalid
	}
	raw, e := s.row(key("o", id), OwnedMaxOperationSize)
	if e != nil {
		return OwnedOperation{}, e
	}
	o, e := ParseOwnedOperation(s.meta.Pin, raw)
	if e != nil || o.Intent.OperationID != id {
		return OwnedOperation{}, ErrCorrupt
	}
	return o, nil
}
func (s *OwnedStore) preparation(o OwnedOperation) (ownedPreparation, error) {
	raw, e := s.row(key("p", o.Intent.OperationID), 4096)
	var p ownedPreparation
	if e != nil || ownedcodec.Decode(raw, 4096, &p) != nil || p.Version != 1 || p.OperationID != o.Intent.OperationID || fighterownership.ValidateHead(s.meta.Pin.StablePin.Ownership, p.Head) != nil || p.StateIDs == nil || len(p.StateIDs) != len(o.Intent.Request.ProspectIDs) {
		return p, ErrCorrupt
	}
	for _, id := range p.StateIDs {
		if !hashID(id) {
			return p, ErrCorrupt
		}
	}
	return p, nil
}
func (s *OwnedStore) event(ev Event) (eventRow, bool, error) {
	raw, e := s.row(key("e", ev.EventID), MaxEventSize)
	if errors.Is(e, ErrNotFound) {
		eid, _ := OwnedEntryID(s.meta.StoreID, ev.EventID, s.meta.Pin.StablePin.Stable.Identity.GymID)
		return eventRow{Version: 1, Event: ev, EntryID: eid, Phase: "accepting"}, false, nil
	}
	var r eventRow
	if e != nil || ownedcodec.Decode(raw, MaxEventSize, &r) != nil || r.Event != ev {
		return r, false, ErrCorrupt
	}
	return r, true, nil
}
func (s *OwnedStore) randomID() (string, error) {
	for n := 0; n < 8; n++ {
		var b [32]byte
		if _, e := io.ReadFull(rand.Reader, b[:]); e != nil {
			return "", e
		}
		id := hex.EncodeToString(b[:])
		exists, e := s.db.Has(key("o", id), nil)
		if e != nil {
			return "", e
		}
		if !exists {
			return id, nil
		}
	}
	return "", ErrCapacity
}
