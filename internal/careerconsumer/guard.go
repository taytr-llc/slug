// Copyright (c) 2026 The SLUG developers. ISC license.

// Package careerconsumer holds bounded career bindings for trusted source
// stores. Callers serialize access and own synchronous storage and authorization.
package careerconsumer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"

	"github.com/taytr-llc/slug/internal/careerstate"
)

const MaxFences = 256
const MaxRowSize = 2048
const GuardKey = "career_guard"
const FencePrefix = "career_cutoff:"

var ErrIneligible = errors.New("career_candidate_ineligible")
var ErrCapacity = errors.New("career_consumer_capacity")

type IneligibleError struct{ ProspectIDs []string }

func (e *IneligibleError) Error() string { return ErrIneligible.Error() }
func (e *IneligibleError) Unwrap() error { return ErrIneligible }

type binding struct {
	Version     uint32                 `json:"version"`
	Activation  careerstate.Activation `json:"activation"`
	FenceCount  uint32                 `json:"fence_count"`
	FenceDigest string                 `json:"fence_digest"`
}
type fence struct {
	Version      uint32   `json:"version"`
	ActivationID string   `json:"activation_id"`
	CutoffMS     int64    `json:"cutoff_ms"`
	ProspectIDs  []string `json:"prospect_ids"`
	CareerDigest string   `json:"career_digest"`
}

// Guard is used only under its owning component's gate/mutex. A ready reopen
// requires the exact persisted binding; prepared recovery may encounter none.
type Guard struct {
	expected    *careerstate.Activation
	prepared    bool
	bound       bool
	reader      *careerstate.Reader
	fences      map[string]fence
	fenceCount  uint32
	fenceDigest string
}

func New(expected *careerstate.Activation, prepared bool) (*Guard, error) {
	g := &Guard{prepared: prepared, fences: make(map[string]fence)}
	if expected != nil {
		if careerstate.ValidateActivation(*expected) != nil {
			return nil, careerstate.ErrInvalid
		}
		a := *expected
		g.expected = &a
	}
	return g, nil
}

func canonical(raw []byte, out any) bool {
	if len(raw) == 0 || len(raw) > MaxRowSize || json.Unmarshal(raw, out) != nil {
		return false
	}
	b, e := json.Marshal(out)
	return e == nil && bytes.Equal(b, raw)
}
func validIDs(ids []string) bool {
	if len(ids) == 0 || len(ids) > 10 {
		return false
	}
	for i, id := range ids {
		if !careerstate.ID(id) || i > 0 && ids[i-1] >= id {
			return false
		}
	}
	return true
}
func fenceKey(cutoff int64, ids []string) string {
	var n [8]byte
	binary.LittleEndian.PutUint64(n[:], uint64(cutoff))
	b, _ := json.Marshal(ids)
	h := sha256.New()
	h.Write([]byte("SLUG candidate career consumer cutoff\x00v1\x00"))
	h.Write(n[:])
	h.Write(b)
	return FencePrefix + hex.EncodeToString(h.Sum(nil))
}

// Audit handles only the closed career key family. Unknown keys remain the
// owning store's responsibility. An old/legacy open refuses every career row.
func (g *Guard) Audit(key, raw []byte) (bool, error) {
	if string(key) != GuardKey && !bytes.HasPrefix(key, []byte(FencePrefix)) {
		return false, nil
	}
	if g == nil || g.expected == nil {
		return true, careerstate.ErrCorrupt
	}
	if string(key) == GuardKey {
		var b binding
		if g.bound || !canonical(raw, &b) || b.Version != 1 || b.Activation != *g.expected || b.FenceCount > MaxFences || !careerstate.ID(b.FenceDigest) {
			return true, careerstate.ErrCorrupt
		}
		g.bound = true
		g.fenceCount, g.fenceDigest = b.FenceCount, b.FenceDigest
		return true, nil
	}
	var f fence
	if len(g.fences) >= MaxFences || !canonical(raw, &f) || f.Version != 1 || f.ActivationID != g.expected.ActivationID || !validIDs(f.ProspectIDs) || f.CutoffMS < g.expected.ActivatedAtMS || f.CutoffMS > careerstate.MaxTimestampMS || !careerstate.ID(f.CareerDigest) || string(key) != fenceKey(f.CutoffMS, f.ProspectIDs) {
		return true, careerstate.ErrCorrupt
	}
	g.fences[string(key)] = f
	return true, nil
}
func (g *Guard) Audited(clockMS int64) error {
	if g == nil || !g.bound && (len(g.fences) != 0 || g.expected != nil && !g.prepared) {
		return careerstate.ErrCorrupt
	}
	for _, f := range g.fences {
		if f.CutoffMS > clockMS {
			return careerstate.ErrCorrupt
		}
	}
	if g.bound && (g.fenceCount != uint32(len(g.fences)) || g.fenceDigest != fenceCommitment(g.fences)) {
		return careerstate.ErrCorrupt
	}
	return nil
}

// The binding commits the complete bounded receipt set, so deletion of a
// receipt is detected even when a foreign journal already retained a snapshot.
func fenceCommitment(fences map[string]fence) string {
	keys := make([]string, 0, len(fences))
	for k := range fences {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	h := sha256.New()
	h.Write([]byte("SLUG candidate career consumer receipts\x00v1\x00"))
	var n [4]byte
	binary.LittleEndian.PutUint32(n[:], uint32(len(keys)))
	h.Write(n[:])
	for _, k := range keys {
		raw, _ := json.Marshal(fences[k])
		for _, b := range [][]byte{[]byte(k), raw} {
			binary.LittleEndian.PutUint32(n[:], uint32(len(b)))
			h.Write(n[:])
			h.Write(b)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}
func (g *Guard) Required() bool { return g != nil && (g.expected != nil || g.bound) }
func (g *Guard) Ready() error {
	if g == nil || g.Required() && (!g.bound || g.reader == nil) {
		return careerstate.ErrInactive
	}
	return nil
}

func (g *Guard) verify(reader *careerstate.Reader, kind, storeID string) (careerstate.Activation, error) {
	a, e := reader.Activation()
	if e != nil {
		return a, e
	}
	if g.expected != nil && a != *g.expected {
		return a, careerstate.ErrCorrupt
	}
	scope, e := reader.Scope()
	if e != nil {
		return a, e
	}
	allowed := kind == "stable" && (scope.StableStoreIDs[0] == storeID || scope.StableStoreIDs[1] == storeID) || kind == "card" && scope.CardStoreID == storeID
	if !allowed {
		return a, careerstate.ErrInvalid
	}
	return a, nil
}

// PrepareBind returns exact bytes and a post-Sync install closure. It does not
// change the Guard before the caller's storage write succeeds.
func (g *Guard) PrepareBind(reader *careerstate.Reader, kind, storeID string) ([]byte, func(), error) {
	if g == nil {
		return nil, nil, careerstate.ErrInactive
	}
	a, e := g.verify(reader, kind, storeID)
	if e != nil {
		return nil, nil, e
	}
	digest := fenceCommitment(g.fences)
	raw, e := json.Marshal(binding{1, a, uint32(len(g.fences)), digest})
	if e != nil || len(raw) > MaxRowSize {
		return nil, nil, careerstate.ErrCorrupt
	}
	install := func() {
		g.expected = &a
		g.bound = true
		g.reader = reader
		g.prepared = false
		g.fenceCount = uint32(len(g.fences))
		g.fenceDigest = digest
	}
	if g.bound {
		return nil, install, nil
	}
	return raw, install, nil
}
func (g *Guard) Attach(reader *careerstate.Reader, kind, storeID string) error {
	if g == nil || !g.bound {
		return careerstate.ErrInactive
	}
	_, install, e := g.PrepareBind(reader, kind, storeID)
	if e == nil {
		install()
	}
	return e
}

func (g *Guard) Snapshot(ctx context.Context, ids []string, asOf int64, mode careerstate.Mode) (careerstate.Snapshot, error) {
	if g == nil || !g.bound || g.reader == nil {
		return careerstate.Snapshot{}, careerstate.ErrInactive
	}
	s, e := g.reader.Snapshot(ctx, ids, asOf, mode)
	if e == nil && s.Activation() != *g.expected {
		return careerstate.Snapshot{}, careerstate.ErrCorrupt
	}
	return s, e
}
func RequireZero(snapshot careerstate.Snapshot, ids []string) error {
	if len(ids) != 0 && !validIDs(ids) {
		return careerstate.ErrInvalid
	}
	bad := make([]string, 0, len(ids))
	for _, id := range ids {
		t, ok := snapshot.Totals(id)
		if !ok {
			return careerstate.ErrCorrupt
		}
		if t.Wins != 0 || t.Losses != 0 || t.Draws != 0 {
			bad = append(bad, id)
		}
	}
	if len(bad) != 0 {
		sort.Strings(bad)
		return &IneligibleError{bad}
	}
	return nil
}

// PrepareFence binds the exact requested ID set as well as time. Different
// legitimate selections at one cutoff have separate bounded receipts.
func (g *Guard) PrepareFence(snapshot careerstate.Snapshot, ids []string) (string, []byte, []byte, func(), error) {
	if g == nil || !g.bound || snapshot.Activation() != *g.expected || !validIDs(ids) || !careerstate.ID(snapshot.Digest()) || snapshot.Mode() != careerstate.Cutoff {
		return "", nil, nil, nil, careerstate.ErrInvalid
	}
	want, _ := json.Marshal(ids)
	got, _ := json.Marshal(snapshot.ProspectIDs())
	if !bytes.Equal(want, got) {
		return "", nil, nil, nil, careerstate.ErrInvalid
	}
	f := fence{1, g.expected.ActivationID, snapshot.AsOfMS(), append([]string{}, ids...), snapshot.Digest()}
	k := fenceKey(f.CutoffMS, ids)
	raw, e := json.Marshal(f)
	if e != nil || len(raw) > MaxRowSize {
		return "", nil, nil, nil, careerstate.ErrCorrupt
	}
	if old, ok := g.fences[k]; ok {
		previous, _ := json.Marshal(old)
		if !bytes.Equal(previous, raw) {
			return "", nil, nil, nil, careerstate.ErrCorrupt
		}
		return k, nil, nil, func() {}, nil
	}
	if len(g.fences) >= MaxFences {
		return "", nil, nil, nil, ErrCapacity
	}
	next := make(map[string]fence, len(g.fences)+1)
	for key, row := range g.fences {
		next[key] = row
	}
	next[k] = f
	digest := fenceCommitment(next)
	guardRaw, e := json.Marshal(binding{1, *g.expected, uint32(len(next)), digest})
	if e != nil || len(guardRaw) > MaxRowSize {
		return "", nil, nil, nil, careerstate.ErrCorrupt
	}
	return k, raw, guardRaw, func() { g.fences = next; g.fenceCount = uint32(len(next)); g.fenceDigest = digest }, nil
}
