// Copyright (c) 2026 The SLUG developers. ISC license.
package registry

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/wire/v2"
)

func validateManifestPin(pin ManifestAuthorityPin) (fighterdescriptor.Descriptor, error) {
	genesis, err := chainIdentity(pin.Network, pin.Genesis)
	if err != nil || pin.Version != Version5 || genesis != pin.Genesis || !manifestCanonicalHash(pin.AuthorityID) {
		return fighterdescriptor.Descriptor{}, ErrManifestAuthority
	}
	d, err := fighterdescriptor.ResolveCandidate(pin.DescriptorID)
	if err != nil {
		return fighterdescriptor.Descriptor{}, fmt.Errorf("%w: %w", ErrManifestAuthority, err)
	}
	return d, nil
}

func manifestProduction() error {
	if wire.TestPoW {
		return errors.New("manifest authority requires production PoW; slugtestpow is unsupported")
	}
	return nil
}

// CreateManifestAuthority initializes a fresh candidate authority. The caller
// must retain Info().Pin in trusted configuration. Existing paths are refused.
func CreateManifestAuthority(path string, network uint32, genesis, descriptorID string) (*ManifestStore, error) {
	return createManifestAuthority(path, network, genesis, descriptorID, rand.Reader, nil)
}

func createManifestAuthority(path string, network uint32, genesis, descriptorID string, entropy io.Reader, checkpoint func(string)) (*ManifestStore, error) {
	if err := manifestProduction(); err != nil {
		return nil, err
	}
	canonical, err := chainIdentity(network, genesis)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrManifestAuthority, err)
	}
	d, err := fighterdescriptor.ResolveCandidate(descriptorID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrManifestAuthority, err)
	}
	var authority [32]byte
	if entropy == nil {
		return nil, errors.New("authority entropy unavailable")
	}
	if _, err := io.ReadFull(entropy, authority[:]); err != nil {
		return nil, errors.New("authority namespace generation failed")
	}
	m := manifestMarker{application, Version5, network, canonical, hex.EncodeToString(authority[:]), descriptorID, d}
	raw, err := json.Marshal(m)
	if err != nil || len(raw)+1 > MaxManifestMarkerSize {
		return nil, ErrManifestAuthority
	}
	if err := os.Mkdir(path, 0700); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filepath.Join(path, markerName), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, err
	}
	_, writeErr := file.Write(append(raw, '\n'))
	var syncErr error
	if writeErr == nil {
		syncErr = file.Sync()
	}
	if err := errors.Join(writeErr, syncErr, file.Close()); err != nil {
		return nil, err
	}
	if checkpoint != nil {
		checkpoint("marker")
	}
	dbPath := filepath.Join(path, databaseName)
	if err := os.Mkdir(dbPath, 0700); err != nil {
		return nil, err
	}
	db, err := leveldb.OpenFile(dbPath, &opt.Options{Strict: opt.StrictAll, ErrorIfExist: true})
	if err != nil {
		return nil, err
	}
	s := &ManifestStore{db: db, writeBatch: db.Write, checkpoint: checkpoint, meta: manifestMetadata{Version: Version5, Network: network, Genesis: canonical, AuthorityID: m.AuthorityID, DescriptorID: descriptorID, Descriptor: d}}
	if err := s.commit(new(leveldb.Batch), s.meta); err != nil {
		return nil, errors.Join(err, db.Close())
	}
	for _, directory := range []string{dbPath, path, filepath.Dir(filepath.Clean(path))} {
		if err := syncDirectory(directory); err != nil {
			return nil, errors.Join(err, db.Close())
		}
	}
	s.mark("initialized")
	return s, nil
}

func readManifestMarker(path string) (manifestMarker, error) {
	var m manifestMarker
	if err := regularDirectory(path); err != nil {
		return m, err
	}
	markerPath := filepath.Join(path, markerName)
	info, err := os.Lstat(markerPath)
	if err != nil {
		return m, err
	}
	if !info.Mode().IsRegular() || info.Size() > MaxManifestMarkerSize {
		return m, fmt.Errorf("%w: invalid manifest marker", ErrCorrupt)
	}
	f, err := os.Open(markerPath)
	if err != nil {
		return m, err
	}
	opened, err := f.Stat()
	if err != nil || !opened.Mode().IsRegular() || opened.Size() > MaxManifestMarkerSize || !os.SameFile(info, opened) {
		return m, errors.Join(fmt.Errorf("%w: manifest marker changed while opening", ErrCorrupt), err, f.Close())
	}
	raw, readErr := io.ReadAll(io.LimitReader(f, MaxManifestMarkerSize+1))
	if err := errors.Join(readErr, f.Close()); err != nil {
		return m, err
	}
	if len(raw) == 0 || len(raw) > MaxManifestMarkerSize || raw[len(raw)-1] != '\n' {
		return m, fmt.Errorf("%w: invalid manifest marker size or terminator", ErrCorrupt)
	}
	if err := decodeManifestJSON(raw[:len(raw)-1], MaxManifestMarkerSize-1, 2, &m); err != nil {
		return manifestMarker{}, err
	}
	d, err := validateManifestPin(m.pin())
	if err != nil {
		return manifestMarker{}, fmt.Errorf("%w: %w", ErrCorrupt, err)
	}
	if m.Application != application || m.Descriptor != d {
		return manifestMarker{}, fmt.Errorf("%w: manifest marker descriptor mismatch", ErrCorrupt)
	}
	return m, nil
}

// InspectManifestAuthority returns unsigned marker discovery information only.
// It never opens a database, authenticates an issuer or silently adopts a pin.
func InspectManifestAuthority(path string) (ManifestAuthorityPin, error) {
	m, err := readManifestMarker(path)
	if err != nil {
		return ManifestAuthorityPin{}, err
	}
	return m.pin(), nil
}

// OpenManifestAuthority requires a pre-established expected pin and an existing
// complete store. Its structural audit performs no Argon work or clock query.
func OpenManifestAuthority(path string, expected ManifestAuthorityPin, readOnly bool) (*ManifestStore, error) {
	if err := manifestProduction(); err != nil {
		return nil, err
	}
	if _, err := validateManifestPin(expected); err != nil {
		return nil, err
	}
	m, err := readManifestMarker(path)
	if err != nil {
		return nil, err
	}
	if m.pin() != expected {
		return nil, ErrManifestAuthority
	}
	dbPath := filepath.Join(path, databaseName)
	if err := regularDirectory(dbPath); err != nil {
		return nil, err
	}
	db, err := leveldb.OpenFile(dbPath, &opt.Options{Strict: opt.StrictAll, ErrorIfMissing: true, ReadOnly: readOnly})
	if err != nil {
		return nil, err
	}
	s := &ManifestStore{db: db, readOnly: readOnly, writeBatch: db.Write}
	if err := s.audit(m); err != nil {
		return nil, errors.Join(err, db.Close())
	}
	return s, nil
}

func (s *ManifestStore) state(write bool) error {
	if s.closed || s.db == nil {
		return ErrClosed
	}
	if s.poisoned != nil {
		return fmt.Errorf("%w: %v", ErrPoisoned, s.poisoned)
	}
	if err := manifestProduction(); err != nil {
		return err
	}
	if write && s.readOnly {
		return ErrReadOnly
	}
	return nil
}

func (s *ManifestStore) commit(batch *leveldb.Batch, next manifestMetadata) error {
	raw, err := json.Marshal(next)
	if err != nil || len(raw) > MaxManifestMetadataSize {
		return fmt.Errorf("%w: manifest metadata encoding", ErrCorrupt)
	}
	batch.Put([]byte(metadataKey), raw)
	if err := s.writeBatch(batch, &opt.WriteOptions{Sync: true}); err != nil {
		s.poisoned = err
		return fmt.Errorf("%w: %w", ErrPoisoned, err)
	}
	s.meta = next
	return nil
}

// Info returns copied public metadata, not verification of stored proof work.
func (s *ManifestStore) Info() (ManifestAuthorityInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.state(false); err != nil {
		return ManifestAuthorityInfo{}, err
	}
	return ManifestAuthorityInfo{s.meta.pin(), s.meta.Descriptor, s.meta.LastSequence, s.meta.JobCount, s.readOnly}, nil
}

// Close drains the current operation, closes once, and retains the close error.
func (s *ManifestStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return s.closeErr
	}
	s.closed = true
	if s.db != nil {
		s.closeErr = s.db.Close()
	}
	return s.closeErr
}

func (s *ManifestStore) audit(m manifestMarker) error {
	raw, err := s.db.Get([]byte(metadataKey), nil)
	if err != nil {
		return fmt.Errorf("%w: missing manifest metadata: %w", ErrCorrupt, err)
	}
	if err := decodeManifestJSON(raw, MaxManifestMetadataSize, 2, &s.meta); err != nil {
		return err
	}
	if s.meta.pin() != m.pin() || s.meta.Descriptor != m.Descriptor || s.meta.LastClockMS < 0 {
		return fmt.Errorf("%w: manifest metadata disagrees with marker", ErrCorrupt)
	}
	s.clockSeen = s.meta.LastClockMS
	var records, sequences, jobs uint64
	it := s.db.NewIterator(nil, nil)
	defer it.Release()
	for it.Next() {
		key := it.Key()
		switch {
		case bytes.Equal(key, []byte(metadataKey)):
		case bytes.HasPrefix(key, []byte(jobPrefix)):
			if len(key) != len(jobPrefix)+64 {
				return ErrCorrupt
			}
			if _, err := s.decodeJob(it.Value(), string(key[len(jobPrefix):])); err != nil {
				return err
			}
			if jobs == ^uint64(0) {
				return ErrCorrupt
			}
			jobs++
		case bytes.HasPrefix(key, []byte(recordPrefix)):
			if len(key) != len(recordPrefix)+64 {
				return ErrCorrupt
			}
			r, err := s.decodeRecord(it.Value(), string(key[len(recordPrefix):]))
			if err != nil {
				return err
			}
			id, err := s.db.Get(sequenceKey(r.Sequence), nil)
			if err != nil || len(id) != 64 || string(id) != r.Verified.ProspectID {
				return fmt.Errorf("%w: manifest receipt index mismatch", ErrCorrupt)
			}
			if records == ^uint64(0) {
				return ErrCorrupt
			}
			records++
		case bytes.HasPrefix(key, []byte(sequencePrefix)):
			if len(key) != len(sequencePrefix)+8 || len(it.Value()) != 64 || !manifestCanonicalHash(string(it.Value())) {
				return ErrCorrupt
			}
			raw, err := s.db.Get(recordKey(string(it.Value())), nil)
			if err != nil {
				return fmt.Errorf("%w: manifest index has no receipt", ErrCorrupt)
			}
			r, err := s.decodeRecord(raw, string(it.Value()))
			if err != nil {
				return err
			}
			if !bytes.Equal(key, sequenceKey(r.Sequence)) {
				return fmt.Errorf("%w: manifest index points to another sequence", ErrCorrupt)
			}
			if sequences == ^uint64(0) {
				return ErrCorrupt
			}
			sequences++
		default:
			return fmt.Errorf("%w: unknown manifest authority key", ErrCorrupt)
		}
	}
	if err := it.Error(); err != nil {
		return err
	}
	if records != s.meta.LastSequence || sequences != s.meta.LastSequence || jobs != s.meta.JobCount {
		return fmt.Errorf("%w: manifest authority count or sequence gap", ErrCorrupt)
	}
	return nil
}
