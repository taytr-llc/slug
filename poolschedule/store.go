// Copyright (c) 2026 The SLUG developers. ISC license.
package poolschedule

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/wire/v2"
)

var claims = struct {
	sync.Mutex
	owners map[*poolsettle.Store]*Scheduler
}{owners: make(map[*poolsettle.Store]*Scheduler)}

// SettlementBinding authenticates configuration through this exact writable
// Store. The digest is not a unique DB identity or a replacement-store permit.
func SettlementBinding(ctx context.Context, store *poolsettle.Store) (string, error) {
	if wire.TestPoW {
		return "", ErrProfile
	}
	if ctx == nil {
		return "", ErrInvalid
	}
	if store == nil {
		return "", ErrInvalid
	}
	work, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	c, source, e := store.SchedulerBinding(work)
	if e != nil {
		return "", e
	}
	body, e := json.Marshal(struct {
		Config poolsettle.Config `json:"config"`
		Source poolshare.Config  `json:"source"`
	}{c, source})
	if e != nil || uint64(len(body)) > uint64(^uint32(0)) {
		return "", ErrInvalid
	}
	h := sha256.New()
	h.Write([]byte("SLUG pool settlement binding\x00v1\x00"))
	var n [4]byte
	binary.LittleEndian.PutUint32(n[:], uint32(len(body)))
	h.Write(n[:])
	h.Write(body)
	if e = work.Err(); e != nil {
		return "", e
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func Create(ctx context.Context, path string, expected Config, store *poolsettle.Store) (*Scheduler, error) {
	return open(ctx, path, expected, store, true)
}
func Open(ctx context.Context, path string, expected Config, store *poolsettle.Store) (*Scheduler, error) {
	return open(ctx, path, expected, store, false)
}
func open(ctx context.Context, path string, expected Config, store *poolsettle.Store, create bool) (s *Scheduler, err error) {
	if wire.TestPoW {
		return nil, ErrProfile
	}
	if ctx == nil || !validConfig(expected) || store == nil {
		return nil, ErrInvalid
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	binding, e := SettlementBinding(ctx, store)
	if e != nil {
		return nil, e
	}
	if binding != expected.SettlementBindingID {
		return nil, ErrInvalid
	}
	s = &Scheduler{store: store}
	claims.Lock()
	if claims.owners[store] != nil {
		claims.Unlock()
		return nil, ErrBusy
	}
	claims.owners[store] = s
	s.claimed = true
	claims.Unlock()
	defer func() {
		if err == nil {
			return
		}
		original := err
		if closeErr := s.Close(context.Background()); closeErr != nil {
			err = errors.Join(original, &CleanupError{owner: s, cause: closeErr})
		}
		s = nil
	}()
	s.files, err = openScheduleFiles(path, create)
	if err != nil {
		return s, err
	}
	raw, e := encodeMarker(expected)
	if e != nil {
		return s, e
	}
	if create {
		if err = s.files.initialize(raw); err != nil {
			return s, err
		}
	} else {
		got, e := decodeMarker(s.files.markerRaw)
		if e != nil || got != expected {
			return s, ErrInvalid
		}
	}
	if !bytes.Equal(s.files.markerRaw, raw) {
		return s, ErrInvalid
	}
	if err = s.files.auditDBBeforeOpen(); err != nil {
		return s, err
	}
	s.db, err = leveldb.OpenFile(filepath.Join(path, "db"), &opt.Options{ErrorIfExist: create, ErrorIfMissing: !create, Strict: opt.StrictAll})
	if err != nil {
		return s, ErrStorage
	}
	if create {
		now := time.Now().UTC().UnixMilli()
		if checkedTime(now, 0) != nil {
			return s, ErrClock
		}
		s.state = State{Version: Version1, Config: expected, Revision: 1, LastClockMS: now}
		raw, e := encodeState(s.state)
		if e != nil {
			return s, e
		}
		if e = s.db.Put(stateKey, raw, &opt.WriteOptions{Sync: true}); e != nil {
			s.poisoned = true
			return s, ErrPoisoned
		}
		if e = s.files.sync(); e != nil {
			s.poisoned = true
			return s, ErrPoisoned
		}
	} else {
		raw, e := s.db.Get(stateKey, nil)
		if e != nil {
			return s, ErrStorage
		}
		st, e := decodeState(raw)
		if e != nil || st.Config != expected {
			return s, ErrInvalid
		}
		s.state = st
	}
	if err = s.auditLocked(); err != nil {
		return s, err
	}
	if err = ctx.Err(); err != nil {
		return s, err
	}
	return s, nil
}

// auditLocked verifies the one-row DB and original descriptors. Filesystem
// replacement is detected and poisons this owner; the private-directory model
// deliberately retains LevelDB's ordinary pathname-based storage.
func (s *Scheduler) auditLocked() error {
	if s == nil || s.closed || s.db == nil {
		return ErrClosed
	}
	if s.poisoned {
		return ErrPoisoned
	}
	if s.files.check() != nil {
		s.poisoned = true
		return ErrPoisoned
	}
	it := s.db.NewIterator(nil, nil)
	count := 0
	var stored []byte
	for it.Next() {
		count++
		if count > 1 || len(it.Value()) > maxStateBytes || !bytes.Equal(it.Key(), stateKey) {
			it.Release()
			s.poisoned = true
			return ErrPoisoned
		}
		stored = append([]byte(nil), it.Value()...)
	}
	e := it.Error()
	it.Release()
	want, we := encodeState(s.state)
	if e != nil || we != nil || count != 1 || !bytes.Equal(stored, want) {
		s.poisoned = true
		return ErrPoisoned
	}
	if _, e = decodeState(stored); e != nil {
		s.poisoned = true
		return ErrPoisoned
	}
	return nil
}
func (s *Scheduler) writeLocked(next State) error {
	if s.poisoned {
		return ErrPoisoned
	}
	if s.closed || s.closing {
		return ErrClosed
	}
	if next.Revision != s.state.Revision+1 || next.Revision == 0 || next.Config != s.state.Config || next.LastClockMS < s.state.LastClockMS || next.DailyCursor < s.state.DailyCursor {
		return ErrInvalid
	}
	raw, e := encodeState(next)
	if e != nil {
		return e
	}
	if e = s.auditLocked(); e != nil {
		return e
	}
	if e = s.db.Put(stateKey, raw, &opt.WriteOptions{Sync: true}); e != nil {
		s.poisoned = true
		return ErrPoisoned
	}
	if e = s.files.check(); e != nil {
		s.poisoned = true
		return ErrPoisoned
	}
	s.state = cloneState(next)
	return nil
}
func (s *Scheduler) Status(ctx context.Context) (Status, error) {
	if wire.TestPoW {
		return Status{}, ErrProfile
	}
	if ctx == nil {
		return Status{}, ErrInvalid
	}
	if e := ctx.Err(); e != nil {
		return Status{}, e
	}
	if s == nil {
		return Status{}, ErrClosed
	}
	if !s.mu.TryLock() {
		return Status{}, ErrBusy
	}
	defer s.mu.Unlock()
	if s.closing || s.closed {
		return Status{}, ErrClosed
	}
	e := s.auditLocked()
	if e != nil {
		return Status{State: cloneState(s.state), Running: s.running, Poisoned: s.poisoned}, e
	}
	if e = ctx.Err(); e != nil {
		return Status{}, e
	}
	return Status{State: cloneState(s.state), Running: s.running, Poisoned: s.poisoned}, nil
}
func (s *Scheduler) Close(ctx context.Context) error {
	if ctx == nil {
		return ErrInvalid
	}
	if s == nil {
		return nil
	}
	s.mu.Lock()
	s.closing = true
	if s.runCancel != nil {
		s.runCancel()
	}
	done := s.runDone
	s.mu.Unlock()
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			return errors.Join(ErrClosed, ctx.Err())
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closeErr != nil {
		return s.closeErr
	}
	if s.closed {
		return nil
	}
	if s.db != nil && !s.dbClosed {
		s.dbClosed = true
		if e := s.db.Close(); e != nil {
			s.closeErr = errors.Join(ErrStorage, e)
			return s.closeErr
		}
	}
	if e := s.files.close(); e != nil {
		s.closeErr = errors.Join(ErrStorage, e)
		return s.closeErr
	}
	if s.claimed {
		claims.Lock()
		if claims.owners[s.store] == s {
			delete(claims.owners, s.store)
		}
		claims.Unlock()
		s.claimed = false
	}
	s.closed = true
	return nil
}
