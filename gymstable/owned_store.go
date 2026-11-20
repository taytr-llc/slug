// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/fighterownership"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/internal/careerconsumer"
	"github.com/taytr-llc/slug/internal/careerstate"
	"github.com/taytr-llc/slug/internal/ownedcodec"
)

const ownedMarkerName = "SLUG-OWNED-STABLE-V1.json"

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
	ClockMS            int64    `json:"clock_ms"`
	LastAsOfMS         int64    `json:"last_as_of_ms"`
	OperationCount     uint32   `json:"operation_count"`
	PendingOperationID string   `json:"pending_operation_id"`
	LastOperationID    string   `json:"last_operation_id"`
	RosterRevision     uint64   `json:"roster_revision"`
}

// OwnedStore owns its durable roster and borrows one exact public replay owner.
// It never converts that source to a legacy registry/assignment Store. One
// admitted operation owns the accounted 4MiB projection/serialization scratch;
// this is a bounded reservation, not a measured total Go heap claim.
type OwnedStore struct {
	gate                       chan struct{}
	life                       context.Context
	cancel                     context.CancelFunc
	closeOnce                  sync.Once
	closeErr                   error
	closed, poisoned, readOnly bool
	db                         *leveldb.DB
	source                     *fighterownership.Store
	meta                       ownedMetadata
	roster                     rosterRow
	career, legacy             *careerconsumer.Guard
	legacyExpected             bool
	now                        func() time.Time
	random                     io.Reader
	writeBatch                 func(*leveldb.Batch, *opt.WriteOptions) error
	read                       *OwnedRead
	history                    *OwnedCareerHistoryRead
}

// OwnedCleanupError retains the concrete failed-open owner. A caller must keep
// it until joined cleanup is acknowledged; it may not silently adopt the path.
type OwnedCleanupError struct {
	cause error
	Owner *OwnedStore
}

func (e *OwnedCleanupError) Error() string { return "owned stable cleanup failed: " + e.cause.Error() }
func (e *OwnedCleanupError) Unwrap() error { return e.cause }
func ownedFail(s *OwnedStore, e error) (*OwnedStore, error) {
	if ce := s.Close(); ce != nil {
		return nil, &OwnedCleanupError{errors.Join(e, ce), s}
	}
	return nil, e
}
func newOwnedStore(db *leveldb.DB, p OwnedPin, source *fighterownership.Store, bindings *OwnedCareerBindings, prepared, readOnly bool) (*OwnedStore, error) {
	var a *careerstate.Activation
	var l *careerstate.Activation
	if bindings != nil {
		a = &bindings.Current
		l = bindings.Legacy
	}
	g, e := careerconsumer.New(a, prepared)
	if e != nil {
		return nil, e
	}
	legacy, e := careerconsumer.New(l, prepared)
	if e != nil {
		return nil, e
	}
	life, cancel := context.WithCancel(context.Background())
	return &OwnedStore{gate: make(chan struct{}, 1), life: life, cancel: cancel, db: db, source: source, career: g, legacy: legacy, legacyExpected: l != nil, now: time.Now, random: rand.Reader, writeBatch: db.Write, readOnly: readOnly}, nil
}
func ownedSourceValid(p OwnedPin, source *fighterownership.Store) error {
	if _, e := ValidateOwnedPin(p); e != nil {
		return e
	}
	if source == nil || source.Pin() != p.Ownership || !hashID(source.Identity().StoreID) {
		return ErrInvalid
	}
	return nil
}
func CreateOwned(path string, p OwnedPin, source *fighterownership.Store) (*OwnedStore, error) {
	if e := normal(); e != nil {
		return nil, e
	}
	if e := ownedSourceValid(p, source); e != nil {
		return nil, e
	}
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || !directory(filepath.Dir(path)) {
		return nil, ErrInvalid
	}
	id, _ := OwnedID(p)
	m := ownedMarker{"slug-owned-stable", 1, id, p}
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
	s, e := newOwnedStore(db, p, source, nil, true, false)
	if e != nil {
		db.Close()
		return nil, e
	}
	now := s.now().UnixMilli()
	if !ownedTime(now) {
		return ownedFail(s, ErrClock)
	}
	s.meta = ownedMetadata{Version: 1, StoreID: id, Pin: p, ClockMS: now}
	s.roster = rosterRow{Version: 1}
	b := new(leveldb.Batch)
	if e = encodedPut(b, []byte("roster"), s.roster, 4096); e != nil {
		return ownedFail(s, e)
	}
	if e = s.persist(b, s.meta); e != nil {
		return ownedFail(s, e)
	}
	for _, x := range []string{dbpath, path, filepath.Dir(path)} {
		if e = syncDir(x); e != nil {
			return ownedFail(s, errors.Join(ErrOutcomeUnknown, e))
		}
	}
	return s, nil
}
func OpenOwned(path string, p OwnedPin, source *fighterownership.Store) (*OwnedStore, error) {
	return openOwned(path, p, source, nil, false, false)
}
func OpenOwnedAudit(path string, p OwnedPin, source *fighterownership.Store) (*OwnedStore, error) {
	return openOwned(path, p, source, nil, false, true)
}
func OpenOwnedCareer(path string, p OwnedPin, source *fighterownership.Store, a careerstate.Activation) (*OwnedStore, error) {
	return openOwned(path, p, source, &OwnedCareerBindings{Current: a}, false, false)
}
func OpenOwnedWithCareerBindings(path string, p OwnedPin, source *fighterownership.Store, b OwnedCareerBindings) (*OwnedStore, error) {
	return openOwned(path, p, source, &b, false, false)
}
func ResumeOwnedCareerBinding(path string, p OwnedPin, source *fighterownership.Store, b OwnedCareerBindings) (*OwnedStore, error) {
	return openOwned(path, p, source, &b, true, false)
}
func OpenOwnedCareerAudit(path string, p OwnedPin, source *fighterownership.Store, b OwnedCareerBindings) (*OwnedStore, error) {
	return openOwned(path, p, source, &b, false, true)
}
func openOwned(path string, p OwnedPin, source *fighterownership.Store, bindings *OwnedCareerBindings, prepared, readOnly bool) (*OwnedStore, error) {
	if e := normal(); e != nil {
		return nil, e
	}
	if e := ownedSourceValid(p, source); e != nil {
		return nil, e
	}
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || !directory(path) || !directory(filepath.Join(path, "db")) {
		return nil, ErrCorrupt
	}
	entries, e := os.ReadDir(path)
	if e != nil || len(entries) != 2 {
		return nil, ErrCorrupt
	}
	for _, x := range entries {
		if x.Name() != ownedMarkerName && x.Name() != "db" {
			return nil, ErrCorrupt
		}
	}
	file := filepath.Join(path, ownedMarkerName)
	st, e := os.Lstat(file)
	if e != nil || !st.Mode().IsRegular() || st.Size() > OwnedMaxPinSize+1024 {
		return nil, ErrCorrupt
	}
	f, e := os.Open(file)
	if e != nil {
		return nil, ErrCorrupt
	}
	cur, e := f.Stat()
	if e != nil || !os.SameFile(st, cur) {
		f.Close()
		return nil, ErrCorrupt
	}
	raw, e := io.ReadAll(io.LimitReader(f, OwnedMaxPinSize+1025))
	e = errors.Join(e, f.Close())
	if e != nil || len(raw) < 2 || raw[len(raw)-1] != '\n' {
		return nil, ErrCorrupt
	}
	var m ownedMarker
	id, _ := OwnedID(p)
	if ownedcodec.Decode(raw[:len(raw)-1], OwnedMaxPinSize+1024, &m) != nil || m != (ownedMarker{"slug-owned-stable", 1, id, p}) {
		return nil, ErrCorrupt
	}
	db, e := leveldb.OpenFile(filepath.Join(path, "db"), &opt.Options{Strict: opt.StrictAll, ErrorIfMissing: true, ReadOnly: readOnly})
	if e != nil {
		return nil, e
	}
	s, e := newOwnedStore(db, p, source, bindings, prepared, readOnly)
	if e != nil {
		db.Close()
		return nil, e
	}
	if e = s.audit(m); e != nil {
		return ownedFail(s, e)
	}
	return s, nil
}
func (s *OwnedStore) state() error {
	if s.closed || s.life.Err() != nil {
		return ErrClosed
	}
	if s.poisoned {
		return ErrPoisoned
	}
	return nil
}
func (s *OwnedStore) enter(ctx context.Context) (context.Context, func(), error) {
	if s == nil || ctx == nil {
		return nil, nil, ErrInvalid
	}
	if e := normal(); e != nil {
		return nil, nil, e
	}
	if s.life.Err() != nil {
		return nil, nil, ErrClosed
	}
	work, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(s.life, cancel)
	finish := func() { stop(); cancel() }
	select {
	case s.gate <- struct{}{}:
	case <-work.Done():
		finish()
		return nil, nil, ErrCancelled
	default:
		finish()
		return nil, nil, ErrBusy
	}
	release := func() { <-s.gate; finish() }
	if e := s.state(); e != nil {
		release()
		return nil, nil, e
	}
	return work, release, nil
}
func (s *OwnedStore) fence(ctx context.Context, session Session, cap gymaccount.Capability, fn func(func() error) error) error {
	if ctx == nil || session.Lease == nil || session.Generation == 0 || session.Generation > MaxSafeInteger || !hashID(session.ViewID) {
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
			if a.ID != s.meta.Pin.Stable.AccountID || a.Generation != session.Generation {
				return ErrStaleView
			}
			if a.Enrollment == nil || *a.Enrollment != s.meta.Pin.Stable.Identity || a.RecoveryRequired || recheck == nil {
				return ErrForbidden
			}
			return mapped(recheck())
		}
		if inner = check(); inner != nil {
			return inner
		}
		if fn != nil {
			inner = fn(check)
		}
		return inner
	})
	mu.Lock()
	active = false
	didCall := called
	mu.Unlock()
	if inner != nil {
		return mapped(inner)
	}
	if !didCall && outer == nil {
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
	work, done, e := s.enter(ctx)
	if e != nil {
		return nil, nil, e
	}
	if e = s.fence(work, session, cap, nil); e != nil {
		done()
		return nil, nil, e
	}
	if e = s.career.Ready(); e == nil {
		e = s.legacy.Ready()
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
		s.closed = true
		s.read = nil
		s.history = nil
		s.closeErr = s.db.Close()
	})
	return s.closeErr
}
func (s *OwnedStore) persist(b *leveldb.Batch, m ownedMetadata) error {
	if s.readOnly {
		return ErrForbidden
	}
	if e := encodedPut(b, []byte("meta"), m, OwnedMaxStatusSize); e != nil {
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
	if !ownedTime(n) || n < s.meta.ClockMS {
		return 0, ErrClock
	}
	return n, nil
}
func (s *OwnedStore) row(key []byte, limit int) ([]byte, error) {
	it := s.db.NewIterator(nil, nil)
	defer it.Release()
	if !it.Seek(key) || !bytes.Equal(it.Key(), key) {
		return nil, ErrNotFound
	}
	if len(it.Value()) > limit {
		return nil, ErrCorrupt
	}
	raw := append([]byte{}, it.Value()...)
	if it.Error() != nil {
		return nil, ErrCorrupt
	}
	return raw, nil
}
func (s *OwnedStore) operation(id string) (OwnedOperation, error) {
	if !hashID(id) {
		return OwnedOperation{}, ErrInvalid
	}
	raw, e := s.row(idKey('o', id), OwnedMaxOperationSize)
	if e != nil {
		return OwnedOperation{}, e
	}
	o, e := ParseOwnedOperation(s.meta.Pin, raw)
	if e != nil || o.Intent.OperationID != id {
		return OwnedOperation{}, ErrCorrupt
	}
	return o, nil
}
func (s *OwnedStore) audit(m ownedMarker) error {
	raw, e := s.row([]byte("meta"), OwnedMaxStatusSize)
	if e != nil || ownedcodec.Decode(raw, OwnedMaxStatusSize, &s.meta) != nil {
		return ErrCorrupt
	}
	x := s.meta
	if x.Version != 1 || x.StoreID != m.StoreID || x.Pin != m.Pin || !ownedTime(x.ClockMS) || x.LastAsOfMS < 0 || x.LastAsOfMS > x.ClockMS || x.OperationCount > 4096 || x.RosterRevision > uint64(x.OperationCount) {
		return ErrCorrupt
	}
	raw, e = s.row([]byte("roster"), 4096)
	if e != nil || ownedcodec.Decode(raw, 4096, &s.roster) != nil || s.roster.Version != 1 || s.roster.Revision != x.RosterRevision || !validSlots(s.roster.Slots) {
		return ErrCorrupt
	}
	var ids [MaxOperations]string
	var count uint32
	it := s.db.NewIterator(nil, nil)
	defer it.Release()
	for it.Next() {
		k, v := it.Key(), it.Value()
		if bytes.HasPrefix(k, []byte("legacy/")) {
			handled, e := s.legacy.Audit(k[len("legacy/"):], v)
			if !handled || e != nil {
				return ErrCorrupt
			}
			continue
		}
		if handled, e := s.career.Audit(k, v); handled {
			if e != nil {
				return ErrCorrupt
			}
			continue
		}
		switch {
		case bytes.Equal(k, []byte("meta")), bytes.Equal(k, []byte("roster")):
		case len(k) == 33 && k[0] == 'o':
			if count >= x.OperationCount || len(v) > OwnedMaxOperationSize {
				return ErrCorrupt
			}
			o, e := ParseOwnedOperation(x.Pin, v)
			if e != nil || o.Intent.OperationID != hex.EncodeToString(k[1:]) || o.Intent.Sequence > uint64(x.OperationCount) || ids[o.Intent.Sequence-1] != "" {
				return ErrCorrupt
			}
			ids[o.Intent.Sequence-1] = o.Intent.OperationID
			count++
		default:
			return ErrCorrupt
		}
	}
	if it.Error() != nil || count != x.OperationCount || s.career.Audited(x.ClockMS) != nil || s.legacy.Audited(x.ClockMS) != nil {
		return ErrCorrupt
	}
	var slots [10]string
	var revision uint64
	var stamp int64
	pending := ""
	for n := uint32(0); n < count; n++ {
		if ids[n] == "" {
			return ErrCorrupt
		}
		o, e := s.operation(ids[n])
		if e != nil || o.Intent.Sequence != uint64(n+1) || o.Intent.PreparedAtMS < stamp || o.Intent.Request.ExpectedRevision != revision {
			return ErrCorrupt
		}
		stamp = o.Intent.PreparedAtMS
		switch o.State {
		case "prepared":
			if n+1 != count {
				return ErrCorrupt
			}
			pending = o.Intent.OperationID
		case "abandoned":
			stamp = o.AbandonedAtMS
		case "committed":
			r := o.Receipt
			if r.PreviousProspectID != slots[r.Request.Slot-1] {
				return ErrCorrupt
			}
			slots[r.Request.Slot-1] = r.Request.ProspectID
			if slots != r.Slots {
				return ErrCorrupt
			}
			revision = r.Revision
			stamp = r.CommittedAtMS
		}
	}
	last := ""
	if count > 0 {
		last = ids[count-1]
	}
	if stamp > x.ClockMS || slots != s.roster.Slots || revision != x.RosterRevision || pending != x.PendingOperationID || last != x.LastOperationID {
		return ErrCorrupt
	}
	return nil
}
func (s *OwnedStore) status(h fighterownership.HeadRef) OwnedStatus {
	return OwnedStatus{1, s.meta.StoreID, s.meta.Pin, h, s.meta.RosterRevision, s.roster.Slots, s.meta.OperationCount, s.meta.PendingOperationID, s.meta.LastOperationID, s.meta.LastAsOfMS}
}
func (s *OwnedStore) Status(ctx context.Context, session Session) (OwnedStatus, error) {
	ctx, done, e := s.begin(ctx, session, gymaccount.View)
	if e != nil {
		return OwnedStatus{}, e
	}
	defer done()
	h, e := s.source.Head(ctx)
	if e != nil {
		return OwnedStatus{}, e
	}
	out := s.status(h)
	if e = s.fence(ctx, session, gymaccount.View, nil); e != nil {
		return OwnedStatus{}, e
	}
	return ValidateOwnedStatus(out)
}

// SourceInfo is metadata for concrete factory wiring, not an owner assertion.
func (s *OwnedStore) SourceInfo(ctx context.Context) (OwnedStatus, error) {
	ctx, done, e := s.enter(ctx)
	if e != nil {
		return OwnedStatus{}, e
	}
	defer done()
	h, e := s.source.Head(ctx)
	if e != nil {
		return OwnedStatus{}, e
	}
	return ValidateOwnedStatus(s.status(h))
}
func (s *OwnedStore) installCareer(ctx context.Context, r *careerstate.Reader, legacy, bind bool) error {
	ctx, done, e := s.enter(ctx)
	if e != nil {
		return e
	}
	defer done()
	if ctx.Err() != nil {
		return ErrCancelled
	}
	g := s.career
	id := s.meta.StoreID
	prefix := ""
	if legacy {
		g = s.legacy
		id, e = ID(s.meta.Pin.Stable)
		prefix = "legacy/"
		if e != nil {
			return e
		}
	}
	if !bind {
		return g.Attach(r, "stable", id)
	}
	if s.readOnly {
		return ErrForbidden
	}
	raw, install, e := g.PrepareBind(r, "stable", id)
	if e != nil {
		return e
	}
	if raw != nil {
		b := new(leveldb.Batch)
		b.Put([]byte(prefix+careerconsumer.GuardKey), raw)
		m := s.meta
		n, e := s.clock()
		if e != nil {
			return e
		}
		m.ClockMS = n
		if e = s.persist(b, m); e != nil {
			return e
		}
	}
	install()
	if legacy {
		s.legacyExpected = true
	}
	return nil
}
func (s *OwnedStore) BindCareer(ctx context.Context, r *careerstate.Reader) error {
	return s.installCareer(ctx, r, false, true)
}
func (s *OwnedStore) AttachCareer(ctx context.Context, r *careerstate.Reader) error {
	return s.installCareer(ctx, r, false, false)
}
func (s *OwnedStore) BindLegacyCareer(ctx context.Context, r *careerstate.Reader) error {
	return s.installCareer(ctx, r, true, true)
}
func (s *OwnedStore) AttachLegacyCareer(ctx context.Context, r *careerstate.Reader) error {
	return s.installCareer(ctx, r, true, false)
}
func (s *OwnedStore) newID() (string, error) {
	for n := 0; n < 8; n++ {
		var b [32]byte
		if _, e := io.ReadFull(s.random, b[:]); e != nil {
			return "", e
		}
		id := hex.EncodeToString(b[:])
		if strings.Trim(id, "0") == "" {
			continue
		}
		has, e := s.db.Has(idKey('o', id), nil)
		if e != nil {
			return "", ErrCorrupt
		}
		if !has {
			return id, nil
		}
	}
	return "", ErrCapacity
}
