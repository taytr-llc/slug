// Copyright (c) 2026 The SLUG developers. ISC license.
package gymevent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/fighterownership"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/internal/ownedcodec"
	"github.com/taytr-llc/slug/registry"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const ownedMarkerName = "SLUG-OWNED-EVENT-V1.json"

type ownedMarker struct {
	Format  string   `json:"format"`
	Version uint32   `json:"version"`
	StoreID string   `json:"store_id"`
	Pin     OwnedPin `json:"pin"`
}
type ownedMetadata struct {
	Version     uint32   `json:"version"`
	StoreID     string   `json:"store_id"`
	Pin         OwnedPin `json:"pin"`
	CreatedAtMS int64    `json:"created_at_ms"`
	ClockMS     int64    `json:"clock_ms"`
	EventCount  uint32   `json:"event_count"`
}
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
	now                        func() time.Time
	writeBatch                 func(*leveldb.Batch, *opt.WriteOptions) error
}
type OwnedCleanupError struct {
	Owner *OwnedStore
	cause error
}

func (e *OwnedCleanupError) Error() string { return "owned event cleanup failed: " + e.cause.Error() }
func (e *OwnedCleanupError) Unwrap() error { return e.cause }
func ownedFailure(s *OwnedStore, e error) (*OwnedStore, error) {
	if ce := s.Close(); ce != nil {
		return nil, &OwnedCleanupError{s, errors.Join(e, ce)}
	}
	return nil, e
}
func newOwned(db *leveldb.DB, source *fighterownership.Store, readOnly bool) *OwnedStore {
	ctx, cancel := context.WithCancel(context.Background())
	return &OwnedStore{gate: make(chan struct{}, 1), life: ctx, cancel: cancel, db: db, source: source, readOnly: readOnly, now: time.Now, writeBatch: db.Write}
}
func ownedSource(p OwnedPin, source *fighterownership.Store) error {
	if _, e := ValidateOwnedPin(p); e != nil {
		return e
	}
	if source == nil || source.Pin() != p.Ownership || !hashID(source.Identity().StoreID) {
		return ErrInvalid
	}
	return nil
}
func CreateOwned(ctx context.Context, path string, p OwnedPin, source *fighterownership.Store) (*OwnedStore, error) {
	if e := normal(); e != nil {
		return nil, e
	}
	if ctx == nil {
		return nil, ErrInvalid
	}
	if e := ownedSource(p, source); e != nil {
		return nil, e
	}
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || !realDirectory(filepath.Dir(path)) {
		return nil, ErrInvalid
	}
	id, _ := OwnedID(p)
	marker := ownedMarker{"slug-owned-event", 1, id, p}
	raw, _ := json.Marshal(marker)
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
	now := s.now().UnixMilli()
	s.meta = ownedMetadata{1, id, p, now, now, 0}
	if !timestamp(now) {
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
func OpenOwned(ctx context.Context, path string, p OwnedPin, source *fighterownership.Store) (*OwnedStore, error) {
	return openOwned(ctx, path, p, source, false)
}
func OpenOwnedAudit(ctx context.Context, path string, p OwnedPin, source *fighterownership.Store) (*OwnedStore, error) {
	return openOwned(ctx, path, p, source, true)
}
func openOwned(ctx context.Context, path string, p OwnedPin, source *fighterownership.Store, readOnly bool) (*OwnedStore, error) {
	if e := normal(); e != nil {
		return nil, e
	}
	if ctx == nil {
		return nil, ErrInvalid
	}
	if e := ownedSource(p, source); e != nil {
		return nil, e
	}
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || !realDirectory(path) || !realDirectory(filepath.Join(path, "db")) {
		return nil, ErrCorrupt
	}
	entries, e := os.ReadDir(path)
	if e != nil || len(entries) != 2 {
		return nil, ErrCorrupt
	}
	for _, v := range entries {
		if v.Name() != ownedMarkerName && v.Name() != "db" {
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
	if e != nil || len(raw) < 2 || raw[len(raw)-1] != '\n' || ownedcodec.Decode(raw[:len(raw)-1], OwnedMaxPinSize+1024, &marker) != nil || !ownedcodec.Equal(marker, ownedMarker{"slug-owned-event", 1, id, p}) {
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
func (s *OwnedStore) begin(ctx context.Context) (context.Context, func(), error) {
	if s == nil || ctx == nil {
		return nil, nil, ErrInvalid
	}
	if e := normal(); e != nil {
		return nil, nil, e
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
	done := func() { <-s.gate; finish() }
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
func (s *OwnedStore) clock(lower int64) (int64, error) {
	n := s.now().UnixMilli()
	if !timestamp(n) || n < s.meta.ClockMS || n < lower {
		return 0, ErrClock
	}
	return n, nil
}
func (s *OwnedStore) raw(k []byte, limit int) ([]byte, bool, error) {
	it := s.db.NewIterator(nil, nil)
	defer it.Release()
	if !it.Seek(k) || !bytes.Equal(k, it.Key()) {
		return nil, false, nil
	}
	if len(it.Value()) > limit || it.Error() != nil {
		return nil, false, ErrCorrupt
	}
	return append([]byte{}, it.Value()...), true, nil
}
func (s *OwnedStore) event(ev Event) (eventRow, bool, error) {
	raw, ok, e := s.raw(key('e', ev.EventID), MaxEventSize)
	if e != nil || !ok {
		return eventRow{}, ok, e
	}
	var r eventRow
	if ownedcodec.Decode(raw, MaxEventSize, &r) != nil || r.Event != ev {
		return r, false, ErrCorrupt
	}
	return r, true, nil
}
func (s *OwnedStore) capture(ev Event, participant string) (OwnedParticipantCapture, bool, error) {
	raw, ok, e := s.raw(key('c', ev.EventID, participant), OwnedMaxParticipantSize+4096)
	if e != nil || !ok {
		return OwnedParticipantCapture{}, ok, e
	}
	var c OwnedParticipantCapture
	if ownedcodec.Decode(raw, OwnedMaxParticipantSize+4096, &c) != nil || c.Version != 1 || c.Snapshot.ParticipantID != participant {
		return c, false, ErrCorrupt
	}
	snapshot, e := ValidateOwnedParticipantSnapshot(s.meta.Pin, c.Snapshot)
	if e != nil {
		return c, false, ErrCorrupt
	}
	c.Snapshot = snapshot
	return c, true, nil
}
func (s *OwnedStore) receipt(ev Event) (OwnedSnapshotReceipt, bool, error) {
	raw, ok, e := s.raw(key('s', ev.EventID), OwnedMaxReceiptSize)
	if e != nil || !ok {
		return OwnedSnapshotReceipt{}, ok, e
	}
	v, e := ParseOwnedSnapshotReceipt(s.meta.Pin, raw)
	if e != nil || v.Input.Event != ev {
		return v, false, ErrCorrupt
	}
	return v, true, nil
}
func (s *OwnedStore) Info(ctx context.Context) (OwnedInfo, error) {
	ctx, done, e := s.begin(ctx)
	if e != nil {
		return OwnedInfo{}, e
	}
	defer done()
	p, e := ValidateOwnedPin(s.meta.Pin)
	if e != nil {
		return OwnedInfo{}, ErrCorrupt
	}
	return OwnedInfo{1, s.meta.StoreID, p, s.meta.CreatedAtMS, s.meta.EventCount}, ctx.Err()
}
func (s *OwnedStore) sourceView(ctx context.Context, source *gymentry.OwnedStore, session gymstable.Session, fn func(gymentry.OwnedSnapshotSourceInfo, func() error) error) error {
	if source == nil || fn == nil {
		return ErrInvalid
	}
	info, e := source.SnapshotSourceInfo(ctx)
	if e != nil {
		return e
	}
	found := false
	for _, p := range s.meta.Pin.Participants {
		if p == info.Pin {
			sid, _ := gymentry.OwnedID(p)
			found = sid == info.StoreID
		}
	}
	if !found {
		return ErrInvalid
	}
	return source.WithSnapshotView(ctx, session, func(current gymentry.OwnedSnapshotSourceInfo, check func() error) error {
		if current.StoreID != info.StoreID || current.Pin != info.Pin || s.life.Err() != nil {
			return ErrConflict
		}
		if e := check(); e != nil {
			return e
		}
		return fn(current, check)
	})
}
func (s *OwnedStore) audit(ctx context.Context, marker ownedMarker) error {
	raw, ok, e := s.raw([]byte("meta"), OwnedMaxMetadataSize)
	if e != nil || !ok || ownedcodec.Decode(raw, OwnedMaxMetadataSize, &s.meta) != nil {
		return ErrCorrupt
	}
	m := s.meta
	if m.Version != 1 || m.StoreID != marker.StoreID || !ownedcodec.Equal(m.Pin, marker.Pin) || !timestamp(m.CreatedAtMS) || m.ClockMS < m.CreatedAtMS || !timestamp(m.ClockMS) || m.EventCount > 256 {
		return ErrCorrupt
	}
	var events []eventRow
	captures, receipts := 0, 0
	it := s.db.NewIterator(nil, nil)
	for it.Next() {
		if ctx.Err() != nil {
			it.Release()
			return ErrCancelled
		}
		k, v := it.Key(), it.Value()
		switch {
		case string(k) == "meta":
		case len(k) == 33 && k[0] == 'e':
			if len(events) >= 256 || len(v) > MaxEventSize {
				it.Release()
				return ErrCorrupt
			}
			var r eventRow
			if ownedcodec.Decode(v, MaxEventSize, &r) != nil {
				it.Release()
				return ErrCorrupt
			}
			ev, e := OwnedEventForDate(m.Pin, r.Event.Saturday)
			if e != nil || r.Version != 1 || r.Event != ev || !bytes.Equal(k, key('e', ev.EventID)) || !timestamp(r.CreatedAtMS) || m.CreatedAtMS >= ev.CutoffMS || r.CreatedAtMS < ev.CutoffMS || r.CreatedAtMS < m.CreatedAtMS || r.CreatedAtMS > m.ClockMS || r.CompletedAtMS > m.ClockMS {
				it.Release()
				return ErrCorrupt
			}
			events = append(events, r)
		case len(k) == 65 && k[0] == 'c':
			captures++
			if captures > 512 || len(v) > OwnedMaxParticipantSize+4096 {
				it.Release()
				return ErrCorrupt
			}
		case len(k) == 33 && k[0] == 's':
			receipts++
			if receipts > 256 || len(v) > OwnedMaxReceiptSize {
				it.Release()
				return ErrCorrupt
			}
		default:
			it.Release()
			return ErrCorrupt
		}
	}
	e = it.Error()
	it.Release()
	if e != nil || len(events) != int(m.EventCount) {
		return ErrCorrupt
	}
	actualCaps, actualReceipts := 0, 0
	for _, r := range events {
		cutoff, e := s.source.Cutoff(ctx, r.Event.EventID)
		if e != nil {
			return e
		}
		cid, e := registry.PublicCutoffID(m.Pin.Ownership, cutoff)
		if e != nil || cutoff.CutoffMS != r.Event.CutoffMS {
			return ErrCorrupt
		}
		count := 0
		for _, p := range m.Pin.Participants {
			pid, _ := OwnedParticipantID(m.Pin, p)
			c, ok, e := s.capture(r.Event, pid)
			if e != nil {
				return e
			}
			if !ok {
				continue
			}
			count++
			actualCaps++
			if ownedEvidence(m.Pin, c.Evidence, c.Snapshot, cid) != nil || c.Evidence.CapturedAtMS < r.CreatedAtMS || c.Evidence.CapturedAtMS > m.ClockMS {
				return ErrCorrupt
			}
			if e = s.auditCutoffParticipant(ctx, cutoff, cid, p, c.Snapshot); e != nil {
				return e
			}
		}
		receipt, ok, e := s.receipt(r.Event)
		if e != nil {
			return e
		}
		if count == 2 {
			if !ok || r.Phase != "snapshotted" || r.CompletedAtMS != receipt.CompletedAtMS || r.SnapshotDigest != receipt.SnapshotDigest || !ownedcodec.Equal(receipt.Input.Cutoff, cutoff) {
				return ErrCorrupt
			}
			actualReceipts++
			for n, p := range m.Pin.Participants {
				pid, _ := OwnedParticipantID(m.Pin, p)
				c, _, _ := s.capture(r.Event, pid)
				if !ownedcodec.Equal(c.Snapshot, receipt.Input.Participants[n]) || c.Evidence != receipt.Captures[n] {
					return ErrCorrupt
				}
			}
		} else if ok || r.Phase != "awaiting_capture" && r.Phase != "capturing" || r.CompletedAtMS != 0 || r.SnapshotDigest != "" || r.Phase == "awaiting_capture" && count != 0 || r.Phase == "capturing" && count != 1 {
			return ErrCorrupt
		}
	}
	if captures != actualCaps || receipts != actualReceipts {
		return ErrCorrupt
	}
	return nil
}
