// Copyright (c) 2026 The SLUG developers. ISC license.
package registry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
)

// PreparedPublicPublication is an exact retained unsigned intent. It neither
// attests chain facts nor authorizes arbitrary signed publication bytes.
type PreparedPublicPublication struct {
	owner  *ManifestPublication
	intent publicPublicationIntent
	cutoff *PublicCutoffRecord
}

func (v *PreparedPublicPublication) Entry() PublicPublicationEntry {
	if v == nil {
		return PublicPublicationEntry{}
	}
	return v.intent.entry()
}
func (v *PreparedPublicPublication) Check(ctx context.Context) error {
	if v == nil || v.owner == nil {
		return ErrPublicOwnership
	}
	ctx, done, e := v.owner.acquire(ctx)
	if e != nil {
		return e
	}
	defer done()
	if e = ctx.Err(); e != nil {
		return e
	}
	got, e := v.owner.readPending()
	if e != nil {
		return e
	}
	if got == nil {
		return ErrConflict
	}
	a, _ := json.Marshal(got)
	b, _ := json.Marshal(v.intent)
	if !bytes.Equal(a, b) {
		return ErrConflict
	}
	return nil
}
func (v *PreparedPublicPublication) CommitSigned(ctx context.Context, entry PublicPublicationEntry) (PublicPublicationEntry, error) {
	if v == nil || v.owner == nil {
		return PublicPublicationEntry{}, ErrPublicOwnership
	}
	ctx, done, e := v.owner.acquire(ctx)
	if e != nil {
		return PublicPublicationEntry{}, e
	}
	defer done()
	return v.owner.commitSigned(ctx, v.intent, nil, entry, v.cutoff)
}
func (p *ManifestPublication) prepareExternal(ctx context.Context, kind string, payload json.RawMessage, cutoff *PublicCutoffRecord) (*PreparedPublicPublication, error) {
	ctx, done, e := p.acquire(ctx)
	if e != nil {
		return nil, e
	}
	defer done()
	pending, e := p.readPending()
	if e != nil {
		return nil, e
	}
	var intent publicPublicationIntent
	if pending != nil {
		if pending.Kind != kind || !bytes.Equal(pending.Payload, payload) {
			return nil, ErrBusy
		}
		intent = *pending
	} else {
		intent, e = p.stage(ctx, kind, payload, "")
		if e != nil {
			return nil, e
		}
	}
	return &PreparedPublicPublication{owner: p, intent: intent, cutoff: cutoff}, nil
}
func (p *ManifestPublication) PrepareChainPublication(ctx context.Context, reference PublicChainReference) (*PreparedPublicPublication, error) {
	raw, e := json.Marshal(reference)
	if e != nil {
		return nil, e
	}
	return p.prepareExternal(ctx, "chain", raw, nil)
}

// PendingChainPublication returns only this original owner's exact retained
// chain intent. A nil result means no pending chain, not an empty publication
// queue: another kind remains untouched for its original concrete publisher.
// Reading never stages a replacement or samples a new timestamp.
func (p *ManifestPublication) PendingChainPublication(ctx context.Context) (*PreparedPublicPublication, error) {
	ctx, done, e := p.acquire(ctx)
	if e != nil {
		return nil, e
	}
	defer done()
	pending, e := p.readPending()
	if e != nil {
		return nil, e
	}
	if pending == nil || pending.Kind != "chain" {
		return nil, nil
	}
	if e = ctx.Err(); e != nil {
		return nil, e
	}
	return &PreparedPublicPublication{owner: p, intent: *pending}, nil
}
func (p *ManifestPublication) PrepareCutoffPublication(ctx context.Context, cutoff PublicCutoffRecord) (*PreparedPublicPublication, error) {
	id, e := PublicCutoffID(p.pin, cutoff)
	if e != nil {
		return nil, e
	}
	// Cutoff has no mutable slices. Validation includes its exact signed boundary.
	raw, _ := json.Marshal(PublicCutoffReference{CutoffID: id})
	return p.prepareExternal(ctx, "cutoff", raw, &cutoff)
}

// PublicBoundaryLease freezes admission acceptance and publication sequencing
// for the concrete coordinator's bounded noon observation. No DB or mutex is
// held during RPC; fresh admissions fail busy until Close.
type PublicBoundaryLease struct {
	owner        *ManifestPublication
	status       PublicationStatus
	done         func()
	ctx          context.Context
	stopCancel   func() bool
	cancelDone   chan struct{}
	cancelJoined sync.Once
	mu           sync.Mutex
	closed       atomic.Bool
}

func (p *ManifestPublication) BeginBoundary(ctx context.Context) (*PublicBoundaryLease, error) {
	ctx, done, e := p.acquire(ctx)
	if e != nil {
		return nil, e
	}
	s := p.store
	if !s.mu.TryLock() {
		done()
		return nil, ErrBusy
	}
	defer s.mu.Unlock()
	if e = s.state(true); e != nil {
		done()
		return nil, e
	}
	if e = ctx.Err(); e != nil {
		done()
		return nil, e
	}
	if s.public.boundary || s.public.meta.PendingPublicationID != "" {
		done()
		return nil, ErrBusy
	}
	m := s.public.meta
	s.public.boundary = true
	status := PublicationStatus{1, p.pinID, m.AcceptedSequence, m.SignedAdmissionSequence, m.AdmissionHead, m.PublicationSequence, m.PublicationHead, m.AcceptedSequence != m.SignedAdmissionSequence}
	lease := &PublicBoundaryLease{owner: p, status: status, done: done, ctx: ctx, cancelDone: make(chan struct{})}
	lease.stopCancel = context.AfterFunc(ctx, func() { lease.closeInternal(); lease.cancelJoined.Do(func() { close(lease.cancelDone) }) })
	return lease, nil
}
func (l *PublicBoundaryLease) Status() PublicationStatus {
	if l == nil {
		return PublicationStatus{}
	}
	return l.status
}
func (l *PublicBoundaryLease) closeInternal() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed.Swap(true) {
		return
	}
	l.owner.store.mu.Lock()
	l.owner.store.public.boundary = false
	l.owner.store.mu.Unlock()
	l.done()
}
func (l *PublicBoundaryLease) Close() error {
	if l == nil {
		return nil
	}
	stopped := l.stopCancel()
	l.closeInternal()
	if stopped {
		l.cancelJoined.Do(func() { close(l.cancelDone) })
	}
	<-l.cancelDone
	return nil
}
func (l *PublicBoundaryLease) CommitIntent(ctx context.Context, v PublicCutoffIntent) error {
	if ctx == nil || l == nil {
		return ErrPublicOwnership
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed.Load() || l.owner.Alive() != nil {
		return ErrClosed
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	if e := l.ctx.Err(); e != nil {
		return e
	}
	if ValidatePublicCutoffIntent(l.owner.pin, v) != nil || v.AcceptedAdmissionSequence != l.status.AcceptedSequence || v.PublicationSequence != l.status.PublicationSequence || v.PublicationHead != l.status.PublicationHead {
		return ErrConflict
	}
	s := l.owner.store
	if !s.mu.TryLock() {
		return ErrBusy
	}
	defer s.mu.Unlock()
	if e := s.state(true); e != nil {
		return e
	}
	raw, _ := json.Marshal(v)
	key := publicIDKey("ownership/cutoff-intent/", v.EventID)
	old, e := s.db.Get(key, nil)
	if e == nil {
		if !bytes.Equal(old, raw) {
			return ErrConflict
		}
		return nil
	}
	if !errors.Is(e, leveldb.ErrNotFound) {
		return e
	}
	if s.public.meta.CutoffCount >= 256 {
		return ErrPublicOwnership
	}
	prior, e := s.previousPublicCutoff(v.CutoffMS)
	if e != nil {
		return e
	}
	if prior != v.PriorCutoffID {
		return ErrConflict
	}
	// At most one unpublished boundary is retained. A missed old lock is never
	// reconstructed from a new live observation.
	it := s.db.NewIterator(util.BytesPrefix([]byte("ownership/cutoff-intent/")), nil)
	pending := 0
	for it.Next() {
		k := it.Key()
		if bytes.HasPrefix(k, []byte("ownership/cutoff-intent/")) {
			_, er := s.db.Get(append([]byte("ownership/cutoff/"), k[len("ownership/cutoff-intent/"):]...), nil)
			if errors.Is(er, leveldb.ErrNotFound) {
				pending++
			} else if er != nil {
				it.Release()
				return er
			}
		}
	}
	e = it.Error()
	it.Release()
	if e != nil {
		return e
	}
	if pending != 0 {
		return ErrBusy
	}
	batch := new(leveldb.Batch)
	batch.Put(key, raw)
	if e := l.ctx.Err(); e != nil {
		return e
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	if e := s.commit(batch, s.meta); e != nil {
		return e
	}
	if e := l.ctx.Err(); e != nil {
		return e
	}
	return ctx.Err()
}
func (p *ManifestPublication) CutoffIntent(ctx context.Context, eventID string) (PublicCutoffIntent, error) {
	var v PublicCutoffIntent
	if ctx == nil || !publicHex(eventID, 32, true) {
		return v, ErrPublicOwnership
	}
	if e := ctx.Err(); e != nil {
		return v, e
	}
	if e := p.Alive(); e != nil {
		return v, e
	}
	s := p.store
	if !s.mu.TryLock() {
		return v, ErrBusy
	}
	defer s.mu.Unlock()
	if e := s.state(false); e != nil {
		return v, e
	}
	raw, e := s.db.Get(publicIDKey("ownership/cutoff-intent/", eventID), nil)
	if errors.Is(e, leveldb.ErrNotFound) {
		return v, ErrNotFound
	}
	if e != nil {
		return v, e
	}
	if publicDecode(raw, MaxPublicSmallSize, &v) != nil || ValidatePublicCutoffIntent(p.pin, v) != nil {
		return v, ErrCorrupt
	}
	return v, nil
}

// previousPublicCutoff is called only under the original store mutex. It scans
// at most the closed 256-row cutoff namespace, never unrelated receipt bytes.
func (s *ManifestStore) previousPublicCutoff(before int64) (string, error) {
	id := publicZero
	last := int64(-1)
	count := 0
	it := s.db.NewIterator(util.BytesPrefix([]byte("ownership/cutoff/")), nil)
	defer it.Release()
	for it.Next() {
		count++
		if count > 256 {
			return "", ErrCorrupt
		}
		var v PublicCutoffRecord
		if publicDecode(it.Value(), MaxPublicSmallSize, &v) != nil {
			return "", ErrCorrupt
		}
		if v.CutoffMS >= before {
			return "", ErrClock
		}
		if v.CutoffMS > last {
			var e error
			id, e = PublicCutoffID(s.public.pin, v)
			if e != nil {
				return "", e
			}
			last = v.CutoffMS
		}
	}
	return id, it.Error()
}
func (p *ManifestPublication) PreviousCutoffID(ctx context.Context, before int64) (string, error) {
	if ctx == nil || before < 0 || before > MaxPublicTimestampMS {
		return "", ErrPublicOwnership
	}
	if e := ctx.Err(); e != nil {
		return "", e
	}
	if e := p.Alive(); e != nil {
		return "", e
	}
	s := p.store
	if !s.mu.TryLock() {
		return "", ErrBusy
	}
	defer s.mu.Unlock()
	if e := s.state(false); e != nil {
		return "", e
	}
	return s.previousPublicCutoff(before)
}
func (p *ManifestPublication) ChainHeadID(ctx context.Context) (string, error) {
	if ctx == nil {
		return "", ErrPublicOwnership
	}
	if e := ctx.Err(); e != nil {
		return "", e
	}
	if e := p.Alive(); e != nil {
		return "", e
	}
	s := p.store
	if !s.mu.TryLock() {
		return "", ErrBusy
	}
	defer s.mu.Unlock()
	if e := s.state(false); e != nil {
		return "", e
	}
	return s.public.meta.ChainEntryID, nil
}
