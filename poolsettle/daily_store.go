// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettle

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/address/v2"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/poolround"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/txscript/v2"
	"github.com/taytr-llc/slug/wire/v2"
)

func dailyInitial(ctx context.Context, c Config, source *poolround.Store, backend Backend, audit bool) (state, dailyMarker, error) {
	if wire.TestPoW {
		return state{}, dailyMarker{}, ErrProfile
	}
	if ctx == nil || validateDailyConfig(c) != nil || source == nil || !audit && !nonnil(backend) {
		return state{}, dailyMarker{}, ErrConfig
	}
	if err := ctx.Err(); err != nil {
		return state{}, dailyMarker{}, err
	}
	limits, src, err := source.DailyBinding(ctx)
	if err != nil {
		return state{}, dailyMarker{}, err
	}
	if uint64(c.MaxRounds) > limits.MaxRounds || uint64(c.MaxRounds) > src.MaxSubmissions {
		return state{}, dailyMarker{}, ErrConfig
	}
	configID, err := DailyConfigBindingID(c)
	if err != nil {
		return state{}, dailyMarker{}, err
	}
	sourceID, err := poolshare.DailyConfigID(src)
	if err != nil {
		return state{}, dailyMarker{}, ErrConfig
	}
	addr, err := address.DecodeAddress(src.PayoutAddress, &chaincfg.MainNetParams)
	if err != nil {
		return state{}, dailyMarker{}, ErrConfig
	}
	script, err := txscript.PayToAddrScript(addr)
	if err != nil || hex.EncodeToString(script) != c.ChangeScript {
		return state{}, dailyMarker{}, ErrConfig
	}
	own := c
	own.Destinations = append([]Destination(nil), c.Destinations...)
	m := dailyMarker{Application: dailyApplication, Version: Version2, Config: own, Source: src, ConfigID: configID, SourceID: sourceID}
	return state{Config: own, Source: src}, m, nil
}

// CreateDaily selects a new indexed Config2 journal. It never adopts, repairs
// or overwrites any legacy or partial root. The original source/backend stay
// attached to this same owner throughout its lifetime.
func CreateDaily(ctx context.Context, path string, c Config, source *poolround.Store, backend Backend) (*Store, error) {
	st, m, err := dailyInitial(ctx, c, source, backend, false)
	if err != nil {
		return nil, err
	}
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || !directory(filepath.Dir(path)) {
		return nil, ErrInvalid
	}
	if err = os.Mkdir(path, 0700); err != nil {
		return nil, ErrInvalid
	}
	s := newStore(st, source, backend)
	s.daily = &dailyState{path: path, marker: m, head: dailyHeader{Application: dailyApplication, Version: Version2, ConfigID: m.ConfigID, SourceID: m.SourceID, Revision: 1, LogicalBytes: dailyHeadLimit + dailyReportLimit}}
	raw, err := dailyEncode(m, dailyHeadLimit)
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(path, dailyMarkerName), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, ErrCorrupt
	}
	_, writeErr := f.Write(raw)
	syncErr := f.Sync()
	closeErr := s.closeDailyFile(f)
	if errors.Join(writeErr, syncErr, closeErr) != nil {
		return dailyOpenFailed(s, ErrPoisoned)
	}
	db, err := leveldb.OpenFile(filepath.Join(path, "db"), &opt.Options{ErrorIfExist: true, Strict: opt.StrictAll})
	if err != nil {
		return nil, ErrCorrupt
	}
	s.db, s.writeBatch = db, db.Write
	head, _ := dailyEncode(s.daily.head, dailyHeadLimit)
	report, _ := dailyEncode(Report{}, dailyReportLimit)
	b := new(leveldb.Batch)
	b.Put([]byte("head"), head)
	b.Put([]byte("report"), report)
	if db.Write(b, &opt.WriteOptions{Sync: true}) != nil {
		return dailyOpenFailed(s, ErrPoisoned)
	}
	if err = errors.Join(s.syncDailyDir(filepath.Join(path, "db")), s.syncDailyDir(path), s.syncDailyDir(filepath.Dir(path))); err != nil {
		return dailyOpenFailed(s, ErrPoisoned)
	}
	if err = ctx.Err(); err != nil {
		return dailyOpenFailed(s, err)
	}
	return s, nil
}
func OpenDaily(ctx context.Context, path string, c Config, source *poolround.Store, backend Backend) (*Store, error) {
	return openDaily(ctx, path, c, source, backend, false)
}
func OpenDailyAudit(ctx context.Context, path string, c Config, source *poolround.Store) (*Store, error) {
	return openDaily(ctx, path, c, source, nil, true)
}
func openDaily(ctx context.Context, path string, c Config, source *poolround.Store, backend Backend, audit bool) (*Store, error) {
	st, m, err := dailyInitial(ctx, c, source, backend, audit)
	if err != nil {
		return nil, err
	}
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || !directory(path) || !directory(filepath.Join(path, "db")) {
		return nil, ErrCorrupt
	}
	s := newStore(st, source, backend)
	s.daily = &dailyState{path: path, marker: m}
	if err = s.checkDailyMarker(); err != nil {
		return dailyOpenFailed(s, err)
	}
	db, err := leveldb.OpenFile(filepath.Join(path, "db"), &opt.Options{ErrorIfMissing: true, ReadOnly: audit, Strict: opt.StrictAll})
	if err != nil {
		return nil, ErrCorrupt
	}
	s.db, s.writeBatch, s.auditOnly = db, db.Write, audit
	raw, err := db.Get([]byte("head"), nil)
	if err != nil || dailyDecode(raw, dailyHeadLimit, &s.daily.head) != nil {
		return dailyOpenFailed(s, ErrCorrupt)
	}
	raw, err = db.Get([]byte("report"), nil)
	if err != nil || dailyDecode(raw, dailyReportLimit, &s.daily.report) != nil {
		return dailyOpenFailed(s, ErrCorrupt)
	}
	s.state.Report, s.state.LastClockMS = s.daily.report, s.daily.head.LastClockMS
	if err = s.verifyDaily(ctx); err != nil {
		return dailyOpenFailed(s, err)
	}
	return s, nil
}
func dailyOpenFailed(s *Store, cause error) (*Store, error) {
	if err := s.Close(); err != nil {
		s.poisoned = true
		return s, errors.Join(cause, err)
	}
	return nil, cause
}
func (s *Store) checkDailyMarker() error {
	path, want := s.daily.path, s.daily.marker
	p := filepath.Join(path, dailyMarkerName)
	info, err := os.Lstat(p)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() <= 0 || info.Size() > dailyHeadLimit {
		return ErrCorrupt
	}
	f, err := os.Open(p)
	if err != nil {
		return ErrCorrupt
	}
	raw, readErr := io.ReadAll(io.LimitReader(f, dailyHeadLimit+1))
	opened, statErr := f.Stat()
	closeErr := s.closeDailyFile(f)
	if errors.Join(readErr, statErr, closeErr) != nil || !os.SameFile(info, opened) {
		return ErrCorrupt
	}
	expected, err := dailyEncode(want, dailyHeadLimit)
	if err != nil || !bytes.Equal(raw, expected) {
		return ErrCorrupt
	}
	return nil
}

func (s *Store) closeDailyFile(f *os.File) error {
	err := f.Close()
	if err != nil {
		s.daily.failedFiles = append(s.daily.failedFiles, f)
		s.daily.closeErr = ErrPoisoned
		s.poisoned = true
	}
	return err
}
func (s *Store) syncDailyDir(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	return errors.Join(f.Sync(), s.closeDailyFile(f))
}
