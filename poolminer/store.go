// Copyright (c) 2026 The SLUG developers. ISC license.
package poolminer

import (
	"bytes"
	"context"
	"errors"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/wire/v2"
	"path/filepath"
	"reflect"
	"time"
)

func Create(ctx context.Context, path string, expected Config, b Binding) (*Store, error) {
	return open(ctx, path, expected, b, true)
}
func Open(ctx context.Context, path string, expected Config, b Binding) (*Store, error) {
	return open(ctx, path, expected, b, false)
}
func open(ctx context.Context, path string, expected Config, b Binding, create bool) (*Store, error) {
	return openVersion(ctx, path, expected, b, create, false)
}
func openVersion(ctx context.Context, path string, expected Config, b Binding, create, daily bool) (s *Store, err error) {
	if wire.TestPoW {
		return nil, ErrProfile
	}
	if ctx == nil {
		return nil, ErrInvalid
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	idFor := ConfigID
	if daily {
		idFor = DailyConfigID
	}
	id, e := idFor(expected, b)
	if e != nil {
		return nil, e
	}
	s = &Store{daily: daily, gate: make(chan struct{}, 1), closeGate: make(chan struct{}, 1), configID: id, now: time.Now}
	defer func() {
		if err != nil {
			original := err
			if ce := s.Close(context.Background()); ce != nil {
				err = errors.Join(original, &CleanupError{owner: s, cause: ce})
			}
			s = nil
		}
	}()
	s.files, err = openCredentialFiles(path, create)
	if err != nil {
		return s, err
	}
	marker, e := encodeMarkerVersion(expected, s.version())
	if e != nil {
		return s, e
	}
	if create {
		if err = s.files.initialize(marker); err != nil {
			return s, err
		}
	} else {
		got, e := decodeMarkerVersion(s.files.markerRaw, s.version())
		if e != nil || !reflect.DeepEqual(got, expected) {
			return s, ErrInvalid
		}
	}
	if !bytes.Equal(marker, s.files.markerRaw) {
		return s, ErrInvalid
	}
	if err = s.files.auditDBBeforeOpen(); err != nil {
		return s, err
	}
	s.db, err = leveldb.OpenFile(filepath.Join(path, "db"), &opt.Options{ErrorIfExist: create, ErrorIfMissing: !create, Strict: opt.StrictAll})
	if err != nil {
		return s, ErrStorage
	}
	s.write = normalWrite(s.db)
	if create {
		now := s.now().UTC().UnixMilli()
		if !validTime(now) {
			return s, ErrClock
		}
		s.state = State{Version: s.version(), Config: cloneConfig(expected), Revision: 1, ClockMS: now, Credentials: []Credential{}, History: []TokenUse{}}
		raw, e := encodeStateVersion(s.state, s.version())
		if e != nil {
			return s, e
		}
		if s.write(raw) != nil || s.files.sync() != nil {
			s.poisoned.Store(true)
			return s, errors.Join(ErrOutcomeUnknown, ErrStorage)
		}
	} else {
		raw, e := s.db.Get(stateKey, nil)
		if e != nil {
			return s, ErrStorage
		}
		st, e := decodeStateVersion(raw, s.version())
		if e != nil || !reflect.DeepEqual(st.Config, expected) {
			return s, ErrInvalid
		}
		s.state = st
	}
	if err = s.audit(); err != nil {
		return s, err
	}
	if err = ctx.Err(); err != nil {
		return s, err
	}
	return s, nil
}
func (s *Store) alive() error {
	if s == nil || s.gate == nil || s.closing.Load() || s.closed.Load() {
		return ErrClosed
	}
	if s.poisoned.Load() {
		return ErrPoisoned
	}
	return nil
}
func (s *Store) begin(ctx context.Context) (func(), error) {
	if wire.TestPoW {
		return nil, ErrProfile
	}
	if ctx == nil {
		return nil, ErrInvalid
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	if e := s.alive(); e != nil {
		return nil, e
	}
	select {
	case s.gate <- struct{}{}:
	default:
		return nil, ErrBusy
	}
	end := func() { <-s.gate }
	if e := s.alive(); e != nil {
		end()
		return nil, e
	}
	if e := ctx.Err(); e != nil {
		end()
		return nil, e
	}
	if e := s.audit(); e != nil {
		end()
		return nil, e
	}
	return end, nil
}

// The operation gate owns state/db; checkMu additionally joins concurrent uses
// of the callback's temporary recheck before that gate can be released.
func (s *Store) audit() error {
	if s.db == nil || s.files == nil {
		return ErrClosed
	}
	if s.poisoned.Load() {
		return ErrPoisoned
	}
	if s.files.check() != nil {
		s.poisoned.Store(true)
		return ErrPoisoned
	}
	it := s.db.NewIterator(nil, nil)
	count := 0
	var raw []byte
	for it.Next() {
		count++
		if count > 1 || !bytes.Equal(it.Key(), stateKey) || len(it.Value()) > maxStateBytes {
			it.Release()
			s.poisoned.Store(true)
			return ErrPoisoned
		}
		raw = append([]byte{}, it.Value()...)
	}
	e := it.Error()
	it.Release()
	want, we := encodeStateVersion(s.state, s.version())
	if e != nil || we != nil || count != 1 || !bytes.Equal(raw, want) {
		s.poisoned.Store(true)
		return ErrPoisoned
	}
	if _, e = decodeStateVersion(raw, s.version()); e != nil {
		s.poisoned.Store(true)
		return ErrPoisoned
	}
	return nil
}
func (s *Store) commit(ctx context.Context, next State) error {
	if e := s.alive(); e != nil {
		return e
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	if s.state.Revision == ^uint32(0) {
		return ErrCapacity
	}
	if next.Revision != s.state.Revision+1 || next.ClockMS < s.state.ClockMS || !reflect.DeepEqual(next.Config, s.state.Config) {
		return ErrInvalid
	}
	raw, e := encodeStateVersion(next, s.version())
	if e != nil {
		return e
	}
	if e = s.audit(); e != nil {
		return e
	}
	if e = s.write(raw); e != nil {
		s.poisoned.Store(true)
		return errors.Join(ErrOutcomeUnknown, ErrStorage)
	}
	if e = s.files.sync(); e != nil {
		s.poisoned.Store(true)
		return errors.Join(ErrOutcomeUnknown, ErrStorage)
	}
	s.state = next
	if s.checkpoint != nil {
		s.checkpoint("committed")
	}
	if ctx.Err() != nil || s.alive() != nil {
		s.poisoned.Store(true)
		return errors.Join(ErrOutcomeUnknown, ctx.Err(), s.alive())
	}
	return nil
}
func (s *Store) clock(ctx context.Context) (int64, error) {
	now := s.now().UTC().UnixMilli()
	if !validTime(now) || now < s.state.ClockMS {
		return 0, ErrClock
	}
	if now > s.state.ClockMS {
		n := cloneState(s.state)
		n.Revision++
		n.ClockMS = now
		if e := s.commit(ctx, n); e != nil {
			return 0, e
		}
	}
	return now, nil
}

// Snapshot reports only the original audited enrollment and hash/status state.
// It is neither a session grant nor proof that any credential is still active.
func (s *Store) Snapshot(ctx context.Context) (State, error) {
	end, e := s.begin(ctx)
	if e != nil {
		return State{}, e
	}
	defer end()
	if _, e = s.clock(ctx); e != nil {
		return State{}, e
	}
	out := cloneState(s.state)
	if e = ctx.Err(); e != nil {
		return State{}, e
	}
	if e = s.alive(); e != nil {
		return State{}, e
	}
	return out, nil
}
func (s *Store) Close(ctx context.Context) error {
	if s == nil {
		return nil
	}
	if ctx == nil {
		return ErrInvalid
	}
	s.closing.Store(true)
	if s.closeGate == nil {
		return nil
	}
	select {
	case s.closeGate <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-s.closeGate }()
	if s.closeErr != nil {
		return s.closeErr
	}
	if s.closed.Load() {
		return nil
	}
	select {
	case s.gate <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-s.gate }()
	if s.db != nil && !s.dbClosed {
		if e := s.db.Close(); e != nil {
			s.closeErr = errors.Join(ErrStorage, e)
			return s.closeErr
		}
		s.dbClosed = true
	}
	if s.files != nil {
		if e := s.files.close(); e != nil {
			s.closeErr = errors.Join(ErrStorage, e)
			return s.closeErr
		}
	}
	s.db = nil
	s.closed.Store(true)
	return nil
}
