// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettle

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"encoding/hex"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/address/v2"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/poolround"
	"github.com/taytr-llc/slug/txscript/v2"
	"github.com/taytr-llc/slug/wire/v2"
)

const maxStateBytes = 64 << 20

// A later observation may expand every Available fraction from zero to its
// maximum canonical representation. Reserve that growth as well as statuses.
const statusReserve = 256*(4*maxFractionDigits+2048) + 4096

var stateKey = []byte("slug-pool-settlement-v1")

func nonnil(v any) bool {
	if v == nil {
		return false
	}
	r := reflect.ValueOf(v)
	switch r.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Map, reflect.Slice, reflect.Func, reflect.Chan:
		return !r.IsNil()
	}
	return true
}
func same(a, b any) bool {
	x, e := json.Marshal(a)
	y, f := json.Marshal(b)
	return e == nil && f == nil && bytes.Equal(x, y)
}
func clone[T any](v T) T {
	raw, _ := json.Marshal(v)
	var out T
	_ = json.Unmarshal(raw, &out)
	return out
}

func initial(c Config, source *poolround.Store, backend Backend) (state, error) {
	return initialMode(c, source, backend, false)
}

func initialMode(c Config, source *poolround.Store, backend Backend, auditOnly bool) (state, error) {
	if wire.TestPoW {
		return state{}, ErrProfile
	}
	if validateConfig(c) != nil || source == nil || (!auditOnly && !nonnil(backend)) {
		return state{}, ErrConfig
	}
	sourceConfig, err := source.WalkSettlement(context.Background(), nil)
	if err != nil {
		return state{}, ErrSource
	}
	addr, err := address.DecodeAddress(sourceConfig.PayoutAddress, &chaincfg.MainNetParams)
	if err != nil {
		return state{}, ErrConfig
	}
	script, err := txscript.PayToAddrScript(addr)
	if err != nil || hex.EncodeToString(script) != c.ChangeScript {
		return state{}, ErrConfig
	}
	return state{Config: clone(c), Source: sourceConfig, Credits: []Credit{}, Batches: []Batch{}}, nil
}
func newStore(st state, source *poolround.Store, backend Backend) *Store {
	return &Store{state: st, source: source, backend: backend, now: time.Now, gate: make(chan struct{}, 1), closeDone: make(chan struct{})}
}
func syncDir(path string) error {
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	return errors.Join(f.Sync(), f.Close())
}
func directory(path string) bool {
	i, e := os.Lstat(path)
	return e == nil && i.IsDir() && i.Mode()&os.ModeSymlink == 0
}

// Create makes a separate explicit settlement journal. Partial initialization
// is retained for inspection; never silently overwrite or repair it.
func Create(path string, c Config, source *poolround.Store, backend Backend) (*Store, error) {
	st, err := initial(c, source, backend)
	if err != nil {
		return nil, err
	}
	if os.Mkdir(path, 0700) != nil {
		return nil, ErrInvalid
	}
	db, err := leveldb.OpenFile(filepath.Join(path, "db"), &opt.Options{ErrorIfExist: true, Strict: opt.StrictAll})
	if err != nil {
		return nil, ErrCorrupt
	}
	s := newStore(st, source, backend)
	s.db, s.writeBatch = db, db.Write
	if err = s.commit(st, false); err != nil {
		_ = db.Close()
		return nil, err
	}
	if errors.Join(syncDir(filepath.Join(path, "db")), syncDir(path), syncDir(filepath.Dir(filepath.Clean(path)))) != nil {
		_ = db.Close()
		return nil, ErrPoisoned
	}
	return s, nil
}

func decode(raw []byte, st *state) error {
	if len(raw) == 0 || len(raw) > maxStateBytes {
		return ErrCorrupt
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(st) != nil {
		return ErrCorrupt
	}
	canonical, e := json.Marshal(st)
	if e != nil || !bytes.Equal(raw, canonical) {
		return ErrCorrupt
	}
	return nil
}

func Open(path string, c Config, source *poolround.Store, backend Backend) (*Store, error) {
	return openMode(path, c, source, backend, false)
}

func openMode(path string, c Config, source *poolround.Store, backend Backend, auditOnly bool) (*Store, error) {
	expected, err := initialMode(c, source, backend, auditOnly)
	if err != nil {
		return nil, err
	}
	if !directory(path) || !directory(filepath.Join(path, "db")) {
		return nil, ErrCorrupt
	}
	db, err := leveldb.OpenFile(filepath.Join(path, "db"), &opt.Options{ErrorIfMissing: true, ReadOnly: auditOnly, Strict: opt.StrictAll})
	if err != nil {
		return nil, ErrCorrupt
	}
	var st state
	raw, err := db.Get(stateKey, nil)
	if err != nil || decode(raw, &st) != nil || !same(st.Config, expected.Config) || st.Source != expected.Source {
		_ = db.Close()
		return nil, ErrCorrupt
	}
	s := newStore(st, source, backend)
	s.db, s.writeBatch, s.auditOnly = db, db.Write, auditOnly
	if err = s.verify(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) commit(st state, reserve bool) error {
	if s.auditOnly {
		return ErrInvalid
	}
	if s.poisoned {
		return ErrPoisoned
	}
	if st.LastClockMS < s.state.LastClockMS {
		return ErrClock
	}
	raw, err := json.Marshal(st)
	limit := maxStateBytes
	if reserve || len(st.Credits) > len(s.state.Credits) || len(st.Batches) > len(s.state.Batches) {
		limit -= statusReserve
	}
	if err != nil || len(raw) > limit {
		return ErrCapacity
	}
	batch := new(leveldb.Batch)
	batch.Put(stateKey, raw)
	if s.writeBatch(batch, &opt.WriteOptions{Sync: true}) != nil {
		s.poisoned = true
		return ErrPoisoned
	}
	s.state = clone(st)
	return nil
}

// sourceState authenticates credits through the concrete round journal. A
// supplied JSON allocation cannot create a balance. New confirmations are
// imported exactly once; old credits must still match their original source.
func (s *Store) sourceState(ctx context.Context, st *state, importNew bool) error {
	// Stage every effect until the concrete walk has authenticated the complete
	// prefix, every old credit and the original source. No partial page import
	// may survive a callback/read failure, cancellation or final identity check.
	existing := make(map[string]Credit, len(st.Credits))
	for _, credit := range st.Credits {
		if _, duplicate := existing[credit.RoundHash]; duplicate {
			return ErrSource
		}
		existing[credit.RoundHash] = credit
	}
	checked := make(map[string]bool, len(st.Credits))
	additional := make([]Credit, 0)
	sourceConfig, err := s.source.WalkSettlement(ctx, func(page []poolround.Record) error {
		for _, r := range page {
			if err := ctx.Err(); err != nil {
				return err
			}
			if r.Confirmation == nil {
				continue
			}
			credit, err := deriveCredit(r)
			if err != nil || credit.Coin.Script != st.Config.ChangeScript {
				return ErrSource
			}
			if old, exists := existing[credit.RoundHash]; exists {
				if checked[credit.RoundHash] || !same(credit, old) {
					return ErrSource
				}
				checked[credit.RoundHash] = true
			} else if importNew {
				// The verified concrete round stream already proves distinct
				// winning hashes and their original global sequence order.
				additional = append(additional, credit)
			}
		}
		return nil
	})
	if err != nil || sourceConfig != st.Source || len(checked) != len(existing) {
		return ErrSource
	}
	if importNew {
		if len(st.Credits)+len(additional) > int(st.Config.MaxRounds) {
			return ErrCapacity
		}
		gyms := make(map[string]bool)
		for _, d := range st.Config.Destinations {
			gyms[d.GymID] = true
		}
		for _, group := range [][]Credit{st.Credits, additional} {
			for _, credit := range group {
				for _, claim := range credit.Coin.Claims {
					if claim.GymID != "" {
						gyms[claim.GymID] = true
					}
				}
			}
		}
		if len(gyms) > 256 {
			return ErrCapacity
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		st.Credits = append(st.Credits, additional...)
		return nil
	}
	return ctx.Err()
}

func (s *Store) verify(ctx context.Context) error {
	if s.daily != nil {
		return s.verifyDaily(ctx)
	}
	it := s.db.NewIterator(nil, nil)
	count := 0
	for it.Next() {
		count++
		if !bytes.Equal(it.Key(), stateKey) {
			it.Release()
			return ErrCorrupt
		}
		var disk state
		if decode(it.Value(), &disk) != nil || !same(disk, s.state) {
			it.Release()
			return ErrCorrupt
		}
	}
	err := it.Error()
	it.Release()
	if err != nil || count != 1 {
		return ErrCorrupt
	}
	if validateConfig(s.state.Config) != nil || s.state.LastClockMS < 0 || len(s.state.Credits) > int(s.state.Config.MaxRounds) || len(s.state.Batches) > int(s.state.Config.MaxBatches) {
		return ErrCorrupt
	}
	if err = s.sourceState(ctx, &s.state, false); err != nil {
		return err
	}
	if _, err = liveCoins(s.state); err != nil {
		return err
	}
	if err = validateReport(s.state); err != nil {
		return err
	}
	return ctx.Err()
}

func (s *Store) begin(ctx context.Context) (func(), error) {
	if wire.TestPoW {
		return nil, ErrProfile
	}
	if ctx == nil {
		return nil, ErrInvalid
	}
	if s == nil || s.db == nil {
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
	if s == nil || s.db == nil && s.daily == nil {
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
	if s.daily != nil && s.daily.closeErr != nil {
		s.closeErr = s.daily.closeErr
	}
	if s.db != nil && s.db.Close() != nil {
		s.closeErr = ErrPoisoned
	}
	<-s.gate
	close(s.closeDone)
	return s.closeErr
}
