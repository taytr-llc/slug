// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterownership

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/internal/operatorauthority"
)

const markerName = "SLUG-FIGHTER-OWNERSHIP-V1.json"

type storeMarker struct {
	Application  string `json:"application"`
	Version      uint32 `json:"version"`
	StoreID      string `json:"store_id"`
	Pin          Pin    `json:"pin"`
	PinID        string `json:"pin_id"`
	OperatorRole string `json:"operator_role,omitempty"`
}

func countKey(prefix string, n uint32) []byte {
	b := make([]byte, len(prefix)+4)
	copy(b, prefix)
	binary.BigEndian.PutUint32(b[len(prefix):], n)
	return b
}
func idKey(prefix, id string) []byte {
	b, _ := hex.DecodeString(id)
	return append([]byte(prefix), b...)
}
func fundingOrder(id string, index uint32) string {
	b, _ := hex.DecodeString(id)
	for i, j := 0, len(b)-1; i < j; i, j = i+1, j-1 {
		b[i], b[j] = b[j], b[i]
	}
	var v [4]byte
	binary.LittleEndian.PutUint32(v[:], index)
	return string(append(b, v[:]...))
}
func newStore(p Pin, id StoreIdentity, db *leveldb.DB, readOnly bool) *Store {
	return newSelectedStore(p, id, db, readOnly, ownershipCodec{}, "", nil)
}
func newSelectedStore(p Pin, id StoreIdentity, db *leveldb.DB, readOnly bool, codec ownershipCodec, role string, owner *operatorauthority.Owner) *Store {
	life, cancel := context.WithCancel(context.Background())
	pid, _ := codec.ID(p)
	return &Store{codec: codec, operatorRole: role, operator: owner, gate: make(chan struct{}, 1), projection: make(chan struct{}, 1), life: life, cancel: cancel, db: db, pin: p, pinID: pid, identity: id, readOnly: readOnly, generation: 1,
		effective: map[string]EffectiveState{}, states: map[string]EffectiveState{}, admissions: map[uint32]AdmissionStatement{}, admissionByProspect: map[string]uint32{}, maps: map[string]OwnershipMap{}, cutoffs: map[string]CutoffRecord{}, cutoffByID: map[string]string{}, portable: map[string]bool{}, keySizes: map[string]int{}, transferCounts: map[string]uint32{}, publications: map[uint32]PublicationEntry{}, headers: map[string]HeaderRow{}, canonical: map[uint32]string{}, witnesses: map[string]CarrierWitness{}, frames: map[uint32]FinalityFrame{}, frameBodies: map[uint32]FinalityFrameBody{}, finalities: map[string]FinalityWitness{}, boundaries: map[string]SignedBoundary{}, conflicts: map[string]ConflictRecord{}, now: time.Now, writeBatch: db.Write}
}
func markerFor(p Pin, id StoreIdentity) (storeMarker, error) {
	return selectedMarkerFor(p, id, ownershipCodec{}, "")
}
func selectedMarkerFor(p Pin, id StoreIdentity, codec ownershipCodec, role string) (storeMarker, error) {
	pid, e := codec.ID(p)
	if e != nil || !hexValue(id.StoreID, 32, true) {
		return storeMarker{}, ErrInvalid
	}
	if codec.operator && role != OperatorIssuer && role != OperatorReplica || !codec.operator && role != "" {
		return storeMarker{}, ErrSource
	}
	return storeMarker{"slug-fighter-ownership", codec.wireVersion(), id.StoreID, p, pid, role}, nil
}
func syncDirectory(path string) error {
	d, e := os.Open(path)
	if e != nil {
		return e
	}
	return errors.Join(d.Sync(), d.Close())
}
func Create(dir string, p Pin, id StoreIdentity) (*Store, error) {
	return createSelectedStore(dir, p, id, ownershipCodec{}, "", nil)
}
func createSelectedStore(dir string, p Pin, id StoreIdentity, codec ownershipCodec, role string, owner *operatorauthority.Owner) (*Store, error) {
	if e := normal(); e != nil {
		return nil, e
	}
	marker, e := selectedMarkerFor(p, id, codec, role)
	if e != nil {
		return nil, e
	}
	st, e := os.Lstat(dir)
	if errors.Is(e, os.ErrNotExist) {
		if e = os.Mkdir(dir, 0700); e != nil {
			return nil, e
		}
		if e = syncDirectory(filepath.Dir(dir)); e != nil {
			return nil, e
		}
	} else if e != nil {
		return nil, e
	} else if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return nil, ErrInvalid
	}
	entries, e := os.ReadDir(dir)
	if e != nil || len(entries) != 0 {
		return nil, ErrConflict
	}
	raw, _ := json.Marshal(marker)
	raw = append(raw, '\n')
	if len(raw) > 8192 {
		return nil, ErrCapacity
	}
	f, e := os.OpenFile(filepath.Join(dir, codec.markerName()), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return nil, e
	}
	n, we := f.Write(raw)
	if we == nil && n != len(raw) {
		we = ErrCorrupt
	}
	e = errors.Join(we, f.Sync(), f.Close())
	if e != nil {
		return nil, e
	}
	if e = syncDirectory(dir); e != nil {
		return nil, e
	}
	db, e := leveldb.OpenFile(filepath.Join(dir, "db"), &opt.Options{ErrorIfExist: true, Strict: opt.StrictAll})
	if e != nil {
		return nil, e
	}
	s := newSelectedStore(p, id, db, false, codec, role, owner)
	s.meta = storeMetadata{OperatorRole: role, Version: 1, PinID: s.pinID, StoreID: id.StoreID, FinalityHead: zeroID}
	for n := 0; n < 8; n++ {
		raw, _ = json.Marshal(s.meta)
		size := uint32(4 + len(raw))
		if size == s.meta.LogicalBytes {
			break
		}
		s.meta.LogicalBytes = size
	}
	raw, _ = json.Marshal(s.meta)
	batch := new(leveldb.Batch)
	batch.Put([]byte("meta"), raw)
	if e = s.writeBatch(batch, &opt.WriteOptions{Sync: true}); e != nil {
		return nil, errors.Join(e, s.Close())
	}
	s.keySizes["meta"] = len(raw)
	if e = syncDirectory(dir); e != nil {
		return nil, errors.Join(e, s.Close())
	}
	return s, nil
}
func Open(dir string, p Pin, id StoreIdentity) (*Store, error) { return openStore(dir, p, id, false) }
func OpenAudit(dir string, p Pin, id StoreIdentity) (*Store, error) {
	return openStore(dir, p, id, true)
}
func openStore(dir string, p Pin, id StoreIdentity, ro bool) (*Store, error) {
	return openSelectedStore(dir, p, id, ro, ownershipCodec{}, "", nil)
}
func openSelectedStore(dir string, p Pin, id StoreIdentity, ro bool, codec ownershipCodec, role string, owner *operatorauthority.Owner) (*Store, error) {
	if e := normal(); e != nil {
		return nil, e
	}
	want, e := selectedMarkerFor(p, id, codec, role)
	if e != nil {
		return nil, e
	}
	st, e := os.Lstat(dir)
	if e != nil {
		return nil, e
	}
	if !st.IsDir() || st.Mode()&os.ModeSymlink != 0 {
		return nil, ErrCorrupt
	}
	entries, e := os.ReadDir(dir)
	if e != nil || len(entries) != 2 {
		return nil, ErrCorrupt
	}
	for _, x := range entries {
		if x.Name() != codec.markerName() && x.Name() != "db" {
			return nil, ErrCorrupt
		}
		if x.Type()&os.ModeSymlink != 0 {
			return nil, ErrCorrupt
		}
	}
	path := filepath.Join(dir, codec.markerName())
	before, e := os.Lstat(path)
	if e != nil || !before.Mode().IsRegular() || before.Size() < 2 || before.Size() > 8192 {
		return nil, ErrCorrupt
	}
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	opened, e := f.Stat()
	if e != nil || !os.SameFile(before, opened) {
		f.Close()
		return nil, ErrCorrupt
	}
	raw := make([]byte, before.Size())
	n, e := io.ReadFull(f, raw)
	ce := f.Close()
	if e != nil || ce != nil || n != len(raw) || raw[len(raw)-1] != '\n' {
		return nil, ErrCorrupt
	}
	var got storeMarker
	if decode(raw[:len(raw)-1], 8191, &got) != nil || got != want {
		return nil, ErrSource
	}
	ds, e := os.Lstat(filepath.Join(dir, "db"))
	if e != nil || !ds.IsDir() || ds.Mode()&os.ModeSymlink != 0 {
		return nil, ErrCorrupt
	}
	db, e := leveldb.OpenFile(filepath.Join(dir, "db"), &opt.Options{ErrorIfMissing: true, ReadOnly: ro, Strict: opt.StrictAll})
	if e != nil {
		return nil, e
	}
	s := newSelectedStore(p, id, db, ro, codec, role, owner)
	if e = s.audit(); e != nil {
		return nil, errors.Join(e, s.Close())
	}
	return s, nil
}

type rowChange struct {
	value  []byte
	remove bool
}
type changes map[string]rowChange

func (c changes) Put(k, v []byte) { c[string(k)] = rowChange{append([]byte(nil), v...), false} }
func (c changes) Delete(k []byte) { c[string(k)] = rowChange{remove: true} }
func mutableKey(k string) bool    { return k == "meta" || k == "work" || strings.HasPrefix(k, "c/") }
func countChange(c *storeCounts, key string, d int) {
	var p *uint32
	switch {
	case strings.HasPrefix(key, "a/"):
		p = &c.Admissions
	case strings.HasPrefix(key, "p/"):
		p = &c.Publications
	case strings.HasPrefix(key, "h/"):
		p = &c.Headers
	case strings.HasPrefix(key, "w/"):
		p = &c.Witnesses
	case strings.HasPrefix(key, "s/"):
		p = &c.States
	case strings.HasPrefix(key, "m/"):
		p = &c.Maps
	case strings.HasPrefix(key, "k/"):
		p = &c.Cutoffs
	case strings.HasPrefix(key, "b/"):
		p = &c.Boundaries
	case strings.HasPrefix(key, "x/"):
		p = &c.Conflicts
	}
	if p != nil {
		if d > 0 {
			*p++
		} else {
			*p--
		}
	}
}
func countsBound(c storeCounts) bool {
	return c.Admissions <= 1000 && c.Publications <= 16384 && c.Headers <= 65536 && c.Carriers <= 4096 && c.Witnesses <= 8192 && c.States <= MaxStates && c.Maps <= MaxMaps && c.Cutoffs <= 256 && c.Boundaries <= 257 && c.Boundaries <= c.Cutoffs+1 && c.Conflicts <= 8192
}

// commit is called under the store mutex by its joined mutation owner. All
// expensive verification and signing precede it; failed sync poisons the owner.
func (s *Store) commit(batch *leveldb.Batch, next storeMetadata) error {
	if e := s.state(); e != nil {
		return e
	}
	if s.readOnly {
		return ErrUnavailable
	}
	if s.meta.Revision == math.MaxUint32 {
		return ErrCapacity
	}
	ch := changes{}
	if e := batch.Replay(ch); e != nil {
		return e
	}
	delete(ch, "meta")
	next.Revision = s.meta.Revision + 1
	next.Counts = s.meta.Counts
	total := int64(s.meta.LogicalBytes) - int64(4+s.keySizes["meta"])
	transferDelta := map[string]int{}
	for k, v := range ch {
		old, exists := s.keySizes[k]
		if !mutableKey(k) && exists {
			raw, e := s.db.Get([]byte(k), nil)
			if e != nil {
				return e
			}
			if v.remove || !bytes.Equal(raw, v.value) {
				return ErrConflict
			}
		}
		if exists {
			total -= int64(len(k) + old)
		}
		if !v.remove {
			total += int64(len(k) + len(v.value))
		}
		delta := 0
		if exists && v.remove {
			delta = -1
		}
		if !exists && !v.remove {
			delta = 1
		}
		if delta != 0 {
			countChange(&next.Counts, k, delta)
			if strings.HasPrefix(k, "t/") && len(k) == 66 {
				transferDelta[k[2:34]] += delta
			}
		}
	}
	for id, d := range transferDelta {
		old := int(s.transferCounts[id])
		if old == 0 && old+d > 0 {
			next.Counts.Carriers++
		}
		if old > 0 && old+d == 0 {
			next.Counts.Carriers--
		}
		if old+d < 0 {
			return ErrCorrupt
		}
	}
	if !countsBound(next.Counts) || next.FinalitySequence > 4096 || next.PublishedFinalitySequence > next.FinalitySequence {
		return ErrCapacity
	}
	for n := 0; n < 10; n++ {
		raw, _ := json.Marshal(next)
		size := total + int64(4+len(raw))
		if size < 0 || size > MaxStoreBytes {
			return ErrCapacity
		}
		if uint32(size) == next.LogicalBytes {
			break
		}
		next.LogicalBytes = uint32(size)
	}
	raw, _ := json.Marshal(next)
	if total+int64(4+len(raw)) != int64(next.LogicalBytes) || len(raw) > 8192 {
		return ErrCorrupt
	}
	batch.Put([]byte("meta"), raw)
	if e := s.writeBatch(batch, &opt.WriteOptions{Sync: true}); e != nil {
		s.poisoned = true
		return errors.Join(ErrOutcomeUnknown, e)
	}
	for k, v := range ch {
		if v.remove {
			delete(s.keySizes, k)
		} else {
			s.keySizes[k] = len(v.value)
		}
	}
	for id, d := range transferDelta {
		s.transferCounts[id] = uint32(int(s.transferCounts[id]) + d)
	}
	s.keySizes["meta"] = len(raw)
	s.meta = next
	return nil
}
