// Copyright (c) 2026 The SLUG developers. ISC license.
package registry

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/fighterdescriptor"
)

const publicMarkerName = "SLUG-MANIFEST-OWNERSHIP-V1.json"
const publicMetaKey = "ownership/meta"

type publicGuard struct {
	Application   string            `json:"application"`
	Version       uint32            `json:"version"`
	Pin           PublicManifestPin `json:"pin"`
	AuthorityID   string            `json:"authority_id"`
	PublicationID string            `json:"publication_id"`
}
type publicOutbox struct {
	Version           uint32 `json:"version"`
	AdmissionSequence uint32 `json:"admission_sequence"`
	ProspectID        string `json:"prospect_id"`
}

func publicCounterKey(prefix string, n uint32) []byte {
	raw := make([]byte, len(prefix)+4)
	copy(raw, prefix)
	binary.BigEndian.PutUint32(raw[len(prefix):], n)
	return raw
}
func publicIDKey(prefix, id string) []byte {
	raw, _ := hex.DecodeString(id)
	return append([]byte(prefix), raw...)
}
func newPublicState(p PublicManifestPin) *publicManifestState {
	id, _ := PublicManifestPinID(p)
	return &publicManifestState{pin: p, pinID: id, meta: publicManifestMetadata{Version: 1, PinID: id, AdmissionHead: publicZero, PublicationHead: publicZero, ChainEntryID: publicZero}}
}
func publicGuardFor(p PublicManifestPin) publicGuard {
	return publicGuard{"slug-manifest-ownership", 1, p, p.Source.AuthorityID, p.PublicationID}
}
func writePublicFile(path string, value any) error {
	raw, e := json.Marshal(value)
	if e != nil || len(raw)+1 > MaxPublicSmallSize {
		return ErrPublicOwnership
	}
	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	_, we := f.Write(append(raw, '\n'))
	var se error
	if we == nil {
		se = f.Sync()
	}
	return errors.Join(we, se, f.Close())
}
func publicInventory(path string, partial bool) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return ErrPublicOwnership
	}
	for p := path; ; p = filepath.Dir(p) {
		if regularDirectory(p) != nil {
			return ErrCorrupt
		}
		if p == filepath.Dir(p) {
			break
		}
	}
	entries, e := os.ReadDir(path)
	if e != nil || len(entries) > 3 {
		return ErrCorrupt
	}
	seen := map[string]bool{}
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 {
			return ErrCorrupt
		}
		switch entry.Name() {
		case markerName, publicMarkerName:
			if !entry.Type().IsRegular() {
				return ErrCorrupt
			}
		case databaseName:
			if !entry.IsDir() {
				return ErrCorrupt
			}
		default:
			return ErrCorrupt
		}
		seen[entry.Name()] = true
	}
	if !seen[markerName] || !seen[publicMarkerName] || !partial && !seen[databaseName] {
		return ErrCorrupt
	}
	return nil
}
func readPublicGuard(path string, expected PublicManifestPin) (publicGuard, error) {
	var g publicGuard
	if e := publicInventory(path, false); e != nil {
		return g, e
	}
	path = filepath.Join(path, publicMarkerName)
	before, e := os.Lstat(path)
	if e != nil || !before.Mode().IsRegular() || before.Size() > MaxPublicSmallSize {
		return g, ErrCorrupt
	}
	f, e := os.Open(path)
	if e != nil {
		return g, e
	}
	opened, e := f.Stat()
	if e != nil || !os.SameFile(before, opened) || !opened.Mode().IsRegular() {
		return g, errors.Join(ErrCorrupt, f.Close())
	}
	raw, re := io.ReadAll(io.LimitReader(f, MaxPublicSmallSize+1))
	if e = errors.Join(re, f.Close()); e != nil {
		return g, e
	}
	if len(raw) < 2 || len(raw) > MaxPublicSmallSize || raw[len(raw)-1] != '\n' || publicDecode(raw[:len(raw)-1], MaxPublicSmallSize, &g) != nil || g != publicGuardFor(expected) {
		return g, ErrCorrupt
	}
	return g, nil
}

// CreatePublicManifestAuthority selects the guarded mode only on a fresh path.
// The caller must already retain the exact random AuthorityID/PublicationID.
func CreatePublicManifestAuthority(path string, d fighterdescriptor.Descriptor, p PublicManifestPin) (*ManifestStore, error) {
	if e := manifestProduction(); e != nil {
		return nil, e
	}
	if ValidatePublicManifestPin(p) != nil {
		return nil, ErrPublicOwnership
	}
	want, e := validateManifestPin(p.Source)
	if e != nil || want != d {
		return nil, ErrPublicOwnership
	}
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return nil, ErrPublicOwnership
	}
	for parent := filepath.Dir(path); ; parent = filepath.Dir(parent) {
		if regularDirectory(parent) != nil {
			return nil, ErrCorrupt
		}
		if parent == filepath.Dir(parent) {
			break
		}
	}
	if e = os.Mkdir(path, 0700); e != nil {
		return nil, e
	}
	m := manifestMarker{application, Version5, p.Source.Network, p.Source.Genesis, p.Source.AuthorityID, p.Source.DescriptorID, d}
	if e = writePublicFile(filepath.Join(path, publicMarkerName), publicGuardFor(p)); e != nil {
		return nil, e
	}
	if e = writePublicFile(filepath.Join(path, markerName), m); e != nil {
		return nil, e
	}
	if e = syncDirectory(path); e != nil {
		return nil, e
	}
	dbPath := filepath.Join(path, databaseName)
	if e = os.Mkdir(dbPath, 0700); e != nil {
		return nil, e
	}
	db, e := leveldb.OpenFile(dbPath, &opt.Options{Strict: opt.StrictAll, ErrorIfExist: true})
	if e != nil {
		return nil, e
	}
	s := &ManifestStore{db: db, writeBatch: db.Write, public: newPublicState(p), meta: manifestMetadata{Version: Version5, Network: p.Source.Network, Genesis: p.Source.Genesis, AuthorityID: p.Source.AuthorityID, DescriptorID: p.Source.DescriptorID, Descriptor: d}}
	batch := new(leveldb.Batch)
	guard, _ := json.Marshal(publicGuardFor(p))
	batch.Put([]byte("ownership/guard"), guard)
	if e = s.commit(batch, s.meta); e != nil {
		return nil, errors.Join(e, db.Close())
	}
	for _, dir := range []string{dbPath, path, filepath.Dir(path)} {
		if e = syncDirectory(dir); e != nil {
			return nil, errors.Join(e, db.Close())
		}
	}
	return s, nil
}
func OpenPublicManifestAuthority(path string, p PublicManifestPin) (*ManifestStore, error) {
	return openPublicManifestAuthority(path, p, false)
}
func OpenPublicManifestAuthorityAudit(path string, p PublicManifestPin) (*ManifestStore, error) {
	return openPublicManifestAuthority(path, p, true)
}
func openPublicManifestAuthority(path string, p PublicManifestPin, readOnly bool) (*ManifestStore, error) {
	if e := manifestProduction(); e != nil {
		return nil, e
	}
	if e := ValidatePublicManifestPin(p); e != nil {
		return nil, e
	}
	if _, e := readPublicGuard(path, p); e != nil {
		return nil, e
	}
	m, e := readManifestMarker(path)
	if e != nil || m.pin() != p.Source {
		return nil, ErrPublicOwnership
	}
	db, e := leveldb.OpenFile(filepath.Join(path, databaseName), &opt.Options{Strict: opt.StrictAll, ErrorIfMissing: true, ReadOnly: readOnly})
	if e != nil {
		return nil, e
	}
	s := &ManifestStore{db: db, writeBatch: db.Write, readOnly: readOnly, public: newPublicState(p)}
	if e = s.audit(m); e == nil {
		e = s.auditPublic()
	}
	if e != nil {
		return nil, errors.Join(e, db.Close())
	}
	return s, nil
}
func (s *ManifestStore) PublicPublication(expected PublicManifestPin) (*ManifestPublication, error) {
	if s == nil {
		return nil, ErrClosed
	}
	if !s.mu.TryLock() {
		return nil, ErrBusy
	}
	defer s.mu.Unlock()
	if e := s.state(false); e != nil {
		return nil, e
	}
	if s.public == nil || s.public.pin != expected {
		return nil, ErrPublicOwnership
	}
	if old := s.public.publisher; old != nil && !old.closed.Load() {
		return old, nil
	}
	life, cancel := context.WithCancel(context.Background())
	p := &ManifestPublication{store: s, pin: expected, pinID: s.public.pinID, life: life, cancel: cancel, gate: make(chan struct{}, 1)}
	s.public.publisher = p
	return p, nil
}
func (s *ManifestStore) appendPublicAdmission(batch *leveldb.Batch, r ManifestRecord) error {
	if s.public == nil {
		return nil
	}
	if s.public.boundary {
		return ErrBusy
	}
	if r.Sequence == 0 || r.Sequence > uint64(s.public.pin.Limits.Admissions) || r.Sequence != uint64(s.public.meta.AcceptedSequence)+1 {
		return ErrPublicOwnership
	}
	v := publicOutbox{1, uint32(r.Sequence), r.Verified.ProspectID}
	raw, _ := json.Marshal(v)
	batch.Put(publicCounterKey("ownership/outbox/", v.AdmissionSequence), raw)
	return nil
}

type publicBatchValue struct {
	value  []byte
	remove bool
}
type publicBatchChanges map[string]publicBatchValue

func (b publicBatchChanges) Put(k, v []byte) { b[string(k)] = publicBatchValue{v, false} }
func (b publicBatchChanges) Delete(k []byte) { b[string(k)] = publicBatchValue{nil, true} }

// stagePublicBatch counts lengths only for existing record bytes; no full proof
// hash/signature/projection is added to an original account-fenced commit.
func (s *ManifestStore) stagePublicBatch(batch *leveldb.Batch, next manifestMetadata) (publicManifestMetadata, error) {
	m := s.public.meta
	changes := publicBatchChanges{}
	if e := batch.Replay(changes); e != nil {
		return m, e
	}
	if replacement, ok := changes[publicMetaKey]; ok {
		if replacement.remove || publicDecode(replacement.value, MaxPublicSmallSize, &m) != nil {
			return m, ErrPublicOwnership
		}
		if m.PinID != s.public.pinID {
			return m, ErrPublicOwnership
		}
	}
	m.AcceptedSequence = uint32(next.LastSequence)
	if next.LastSequence > uint64(s.public.pin.Limits.Admissions) {
		return m, ErrPublicOwnership
	}
	if m.ClockHighWaterMS < next.LastClockMS {
		m.ClockHighWaterMS = next.LastClockMS
	}
	if m.ClockHighWaterMS > MaxPublicTimestampMS {
		return m, ErrClock
	}
	total := int64(s.public.meta.LogicalBytes)
	for k, v := range changes {
		if k == publicMetaKey {
			continue
		}
		old, e := s.db.Get([]byte(k), nil)
		if e == nil {
			total -= int64(len(k) + len(old))
		} else if !errors.Is(e, leveldb.ErrNotFound) {
			return m, e
		}
		if !v.remove {
			total += int64(len(k) + len(v.value))
		}
	}
	old, e := s.db.Get([]byte(publicMetaKey), nil)
	if e == nil {
		total -= int64(len(publicMetaKey) + len(old))
	} else if !errors.Is(e, leveldb.ErrNotFound) {
		return m, e
	}
	for i := 0; i < 12; i++ {
		raw, e := json.Marshal(m)
		if e != nil || len(raw) > MaxPublicSmallSize {
			return m, ErrPublicOwnership
		}
		want := total + int64(len(publicMetaKey)+len(raw))
		if want < 0 || want > int64(s.public.pin.Limits.LogicalBytes) {
			return m, ErrPublicOwnership
		}
		if m.LogicalBytes == uint32(want) {
			reserve := int64(m.AcceptedSequence-m.SignedAdmissionSequence) * int64(MaxPublicAdmissionSize+MaxPublicSmallSize)
			if want+reserve > int64(s.public.pin.Limits.LogicalBytes) {
				return m, ErrPublicOwnership
			}
			batch.Put([]byte(publicMetaKey), raw)
			return m, nil
		}
		m.LogicalBytes = uint32(want)
	}
	return m, ErrPublicOwnership
}
func (s *ManifestStore) publicKeyAllowed(key []byte) bool {
	if s.public == nil {
		return false
	}
	if bytes.Equal(key, []byte("ownership/guard")) || bytes.Equal(key, []byte(publicMetaKey)) || bytes.Equal(key, []byte("ownership/pending")) {
		return true
	}
	for _, p := range []string{"ownership/outbox/", "ownership/admission/", "ownership/publication/"} {
		if bytes.HasPrefix(key, []byte(p)) {
			return len(key) == len(p)+4
		}
	}
	for _, p := range []string{"ownership/checkpoint/", "ownership/cutoff-intent/", "ownership/cutoff/"} {
		if bytes.HasPrefix(key, []byte(p)) {
			return len(key) == len(p)+32
		}
	}
	return false
}
