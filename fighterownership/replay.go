// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterownership

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"sync"
	"sync/atomic"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/blockchain"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

var ownershipWork = make(chan struct{}, 1)

type Progress struct {
	WorkID          string        `json:"work_id"`
	Phase           string        `json:"phase"`
	NextAdmission   uint32        `json:"next_admission"`
	NextPublication uint32        `json:"next_publication"`
	NextHeight      uint32        `json:"next_height"`
	Ready           bool          `json:"ready"`
	Head            *HeadRef      `json:"head"`
	Cutoff          *CutoffRecord `json:"cutoff,omitempty"`
}
type Replay struct {
	id                 string
	store              *Store
	source             *PublicSource
	node               *OwnedChainSource
	observation        ChainObservation
	ctx                context.Context
	cancel             context.CancelFunc
	step               chan struct{}
	closed             atomic.Bool
	closeOnce          sync.Once
	work               replayWork
	verified           map[string]bool
	verifiedAdmissions map[uint32]bool
	branches           []*replayBranch
	branch             int
	headersBuilt       bool
	official           map[string]EffectiveState
	officialFrontier   map[string]FrontierEvidence
	states             map[string]EffectiveState
	newWitnesses       map[string]CarrierWitness
	cutoff             *SignedBoundary
	frameLimit         uint32
	frameNext          uint32
	frames             map[uint32]FinalityFrame
}
type replayBranch struct {
	target       ReplayTarget
	pinned       bool
	ancestryNext int
	frame        *FinalityFrame
	hashes       []string
	headerNext   uint32
	next         uint32
	effective    map[string]EffectiveState
	winners      map[string]string
	transfers    map[string]TransferStatus
	commits      map[string]TransactionWitness
	recognized   uint32
	checkpoints  map[string]PublicationEntry
	conflicted   map[string]EffectiveState
}

func (s *Store) BeginReplay(ctx context.Context, source *PublicSource, node *OwnedChainSource) (*Replay, error) {
	return s.beginReplay(ctx, source, node, nil)
}
func (s *Store) beginReplay(ctx context.Context, source *PublicSource, node *OwnedChainSource, boundary *SignedBoundary) (*Replay, error) {
	if s == nil || ctx == nil || source == nil || node == nil || source.pin != s.pin || node.pin != s.pin {
		return nil, ErrSource
	}
	if e := normal(); e != nil {
		return nil, e
	}
	select {
	case s.gate <- struct{}{}:
	default:
		return nil, ErrBusy
	}
	defer func() { <-s.gate }()
	if e := source.Check(ctx); e != nil {
		return nil, e
	}
	obs, e := node.Observe(ctx)
	if e != nil {
		return nil, e
	}
	if obs.Height >= s.pin.Limits.Headers {
		return nil, ErrCapacity
	}
	status := source.status

	target := ReplayTarget{status.SignedAdmissionSequence, status.AdmissionHead, status.PublicationSequence, status.PublicationHead, obs.Height, obs.Hash, nil}
	if boundary == nil {
		if status.AcceptedSequence != status.SignedAdmissionSequence || status.PublicEvidencePending {
			return nil, ErrUnavailable
		}
	} else {
		if registry.ValidatePublicBoundary(s.pin, *boundary) != nil {
			return nil, ErrInvalid
		}
		intent, e := source.cutoffIntent(ctx, boundary.Intent.EventID)
		if e != nil {
			return nil, e
		}
		if intent != boundary.Intent {
			return nil, ErrSource
		}
		if intent.AcceptedAdmissionSequence > status.SignedAdmissionSequence || intent.PublicationSequence > status.PublicationSequence {
			return nil, ErrUnavailable
		}
		admissionHead := zeroID
		if intent.AcceptedAdmissionSequence > 0 {
			a, er := source.admission(ctx, intent.AcceptedAdmissionSequence)
			if er != nil {
				return nil, er
			}
			admissionHead, er = registry.PublicAdmissionID(s.pin, a)
			if er != nil {
				return nil, er
			}
		}
		bid, er := registry.PublicBoundaryID(s.pin, *boundary)
		if er != nil {
			return nil, er
		}
		target = ReplayTarget{intent.AcceptedAdmissionSequence, admissionHead, intent.PublicationSequence, intent.PublicationHead, intent.First.Height, intent.First.Hash, &FrontierEvidence{"boundary", bid}}
		if target.ChainHeight >= s.pin.Limits.Headers {
			return nil, ErrCapacity
		}
	}
	if e = source.Check(ctx); e != nil {
		return nil, e
	}

	if !s.mu.TryLock() {
		return nil, ErrBusy
	}
	defer s.mu.Unlock()
	if e = s.state(); e != nil {
		return nil, e
	}
	if s.readOnly {
		return nil, ErrUnavailable
	}
	if s.replay != nil && !s.replay.closed.Load() {
		return nil, ErrBusy
	}

	frameLimit := s.meta.PublishedFinalitySequence
	if source.reader != nil {
		frameLimit = source.remoteStatus.PublishedFinalitySequence
		if boundary == nil && (frameLimit != source.remoteStatus.FinalitySequence || s.meta.PublishedFinalitySequence > frameLimit) {
			return nil, ErrUnavailable
		}
	}
	if boundary == nil {
		if s.meta.PublishedFinalitySequence != s.meta.FinalitySequence {
			return nil, ErrUnavailable
		}
	} else {
		if source.reader == nil {
			if _, e = cutoffFrames(s.pin, boundary.Intent, s.frames, s.frameBodies); e != nil {
				return nil, e
			}
		} else if boundary.Intent.FinalitySequence > frameLimit {
			return nil, ErrUnavailable
		}
		frameLimit = boundary.Intent.FinalitySequence
		if old, ok := s.boundaries[boundary.Intent.EventID]; ok && old != *boundary {
			return nil, ErrConflict
		}
	}

	var id [32]byte
	if _, e = rand.Read(id[:]); e != nil {
		return nil, e
	}
	work := replayWork{Version: 1, PinID: s.pinID, WorkID: hex.EncodeToString(id[:]), BaseRevision: s.meta.Revision, Target: target, Phase: "admissions", NextAdmission: 1, NextPublication: 1}
	work.ScanDigest, e = objectDigest("SLUG fighter ownership scan start", struct {
		PinID  string       `json:"pin_id"`
		Target ReplayTarget `json:"target"`
	}{s.pinID, target})
	if e != nil {
		return nil, e
	}
	// A reopened work record is not a durable verified flag. Recheck its evidence
	// in bounded steps before continuing; preserve its identity and exact target.
	if s.work != nil {
		old, _ := json.Marshal(s.work.Target)
		next, _ := json.Marshal(target)
		if !bytes.Equal(old, next) {
			return nil, ErrStale
		}
		work.WorkID = s.work.WorkID
		work.BaseRevision = s.work.BaseRevision
	}
	batch := new(leveldb.Batch)
	raw, _ := json.Marshal(work)
	batch.Put([]byte("work"), raw)
	if boundary != nil {
		encoded, _ := json.Marshal(boundary)
		batch.Put(idKey("b/", boundary.Intent.EventID), encoded)
	}
	meta := s.meta
	meta.WorkID = work.WorkID
	if e = s.commit(batch, meta); e != nil {
		return nil, e
	}
	s.work = &work
	if boundary == nil {
		s.ready = false
	}
	if boundary != nil {
		s.boundaries[boundary.Intent.EventID] = *boundary
	}
	life, cancel := context.WithCancel(s.life)
	r := &Replay{id: work.WorkID, store: s, source: source, node: node, observation: obs, ctx: life, cancel: cancel, step: make(chan struct{}, 1), work: work, verified: map[string]bool{}, verifiedAdmissions: map[uint32]bool{}, official: map[string]EffectiveState{}, officialFrontier: map[string]FrontierEvidence{}, states: map[string]EffectiveState{}, newWitnesses: map[string]CarrierWitness{}}
	r.frameLimit = frameLimit
	r.frames = map[uint32]FinalityFrame{}
	if source.reader != nil {
		r.frameNext = 1
	} else {
		for n := uint32(1); n <= frameLimit; n++ {
			r.frames[n] = s.frames[n]
		}
	}
	if boundary != nil {
		own := *boundary
		r.cutoff = &own
	}
	s.replay = r
	return r, nil
}
func (r *Replay) Close() error {
	if r == nil {
		return nil
	}
	r.closeOnce.Do(func() {
		r.closed.Store(true)
		r.cancel()
		r.step <- struct{}{}
		r.store.mu.Lock()
		if r.store.replay == r {
			r.store.replay = nil
		}
		r.store.mu.Unlock()
		r.releaseMemory()
		<-r.step
	})
	return nil
}
func (r *Replay) acquire(ctx context.Context) (context.Context, func(), error) {
	if ctx == nil || r == nil || r.store == nil {
		return nil, nil, ErrInvalid
	}
	if r.closed.Load() {
		return nil, nil, ErrClosed
	}
	if e := ctx.Err(); e != nil {
		return nil, nil, e
	}
	select {
	case r.step <- struct{}{}:
	default:
		return nil, nil, ErrBusy
	}
	select {
	case r.store.gate <- struct{}{}:
	default:
		<-r.step
		return nil, nil, ErrBusy
	}
	life, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(r.ctx, cancel)
	done := func() { stop(); cancel(); <-r.store.gate; <-r.step }
	if r.closed.Load() || r.ctx.Err() != nil {
		done()
		return nil, nil, ErrClosed
	}
	return life, done, nil
}
func (r *Replay) progress() Progress {
	p := Progress{WorkID: r.id, Phase: r.work.Phase, NextAdmission: r.work.NextAdmission, NextPublication: r.work.NextPublication, NextHeight: r.work.NextHeight}
	if r.branch < len(r.branches) {
		p.NextHeight = r.branches[r.branch].next
	}
	return p
}
func (r *Replay) Step(ctx context.Context) (Progress, error) {
	ctx, done, e := r.acquire(ctx)
	if e != nil {
		return Progress{}, e
	}
	defer done()
	if e = r.source.Check(ctx); e != nil {
		return Progress{}, e
	}
	if e = r.node.Check(ctx, r.observation); e != nil {
		return Progress{}, e
	}
	if r.source.reader != nil && r.frameNext <= r.frameLimit {
		e = r.stepFrame(ctx)
		return r.progress(), e
	}
	switch r.work.Phase {
	case "admissions":
		e = r.stepAdmission(ctx)
	case "publications":
		e = r.stepPublication(ctx)
	case "chain":
		e = r.stepChain(ctx)
	case "ready":
		return r.finish(ctx)
	default:
		e = ErrCorrupt
	}
	return r.progress(), e
}
func (r *Replay) save(ctx context.Context, batch *leveldb.Batch, work replayWork, apply func()) error {
	return r.saveMeta(ctx, batch, work, nil, apply)
}
func (r *Replay) saveMeta(ctx context.Context, batch *leveldb.Batch, work replayWork, expectedMeta *storeMetadata, apply func()) error {
	if e := ctx.Err(); e != nil {
		return e
	}
	if e := r.source.Check(ctx); e != nil {
		return e
	}
	if e := r.node.Check(ctx, r.observation); e != nil {
		return e
	}
	s := r.store
	if !s.mu.TryLock() {
		return ErrBusy
	}
	defer s.mu.Unlock()
	if e := s.state(); e != nil {
		return e
	}
	if s.replay != r {
		return ErrStale
	}
	raw, _ := json.Marshal(work)
	batch.Put([]byte("work"), raw)
	meta := s.meta
	if expectedMeta != nil {
		if expectedMeta.Revision != meta.Revision {
			return ErrStale
		}
		meta = *expectedMeta
	}
	meta.WorkID = work.WorkID
	if e := s.commit(batch, meta); e != nil {
		return e
	}
	r.work = work
	s.work = &work
	if apply != nil {
		apply()
	}
	return nil
}
func (r *Replay) stepAdmission(ctx context.Context) error {
	if r.work.NextAdmission > r.work.Target.AdmissionSequence {
		work := r.work
		work.Phase = "publications"
		return r.save(ctx, new(leveldb.Batch), work, nil)
	}
	n := r.work.NextAdmission
	a, e := r.source.admission(ctx, n)
	if e != nil {
		return e
	}
	if registry.ValidatePublicAdmission(r.store.pin, a) != nil {
		return ErrCorrupt
	}
	prev := zeroID
	if n > 1 {
		r.store.mu.Lock()
		old, ok := r.store.admissions[n-1]
		r.store.mu.Unlock()
		if !ok {
			return ErrUnavailable
		}
		prev, e = registry.PublicAdmissionID(r.store.pin, old)
		if e != nil {
			return e
		}
	}
	if a.PreviousAdmissionID != prev {
		return ErrCorrupt
	}
	select {
	case ownershipWork <- struct{}{}:
	default:
		return ErrBusy
	}
	verified, ve := prospect.Verify(a.Record.Context, a.Record.Proof)
	<-ownershipWork
	if ve != nil || verified != a.Record.Verified {
		return ErrCorrupt
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	initial, e := InitialState(r.store.pin, a)
	if e != nil {
		return e
	}
	batch := new(leveldb.Batch)
	raw, _ := json.Marshal(a)
	batch.Put(countKey("a/", n), raw)
	raw, _ = json.Marshal(initial)
	batch.Put(idKey("s/", initial.StateID), raw)
	work := r.work
	work.NextAdmission++
	if n == work.Target.AdmissionSequence {
		id, _ := registry.PublicAdmissionID(r.store.pin, a)
		if id != work.Target.AdmissionHead {
			return ErrSource
		}
	}
	return r.save(ctx, batch, work, func() {
		r.store.admissions[n] = a
		r.store.admissionByProspect[initial.ProspectID] = n
		r.store.states[initial.StateID] = initial
		r.states[initial.StateID] = initial
		r.verifiedAdmissions[n] = true
	})
}
func (r *Replay) stepPublication(ctx context.Context) error {
	if r.work.NextPublication > r.work.Target.PublicationSequence {
		work := r.work
		work.Phase = "chain"
		return r.save(ctx, new(leveldb.Batch), work, nil)
	}
	n := r.work.NextPublication
	p, e := r.source.publication(ctx, n)
	if e != nil {
		return e
	}
	if registry.ValidatePublicPublication(r.store.pin, p) != nil {
		return ErrCorrupt
	}
	previous := zeroID
	if n > 1 {
		r.store.mu.Lock()
		old, ok := r.store.publications[n-1]
		r.store.mu.Unlock()
		if !ok {
			return ErrUnavailable
		}
		previous, e = registry.PublicPublicationID(r.store.pin, old)
		if e != nil {
			return e
		}
	}
	if p.PreviousID != previous {
		return ErrCorrupt
	}
	if n > 1 && p.AttestedAtMS < r.store.publications[n-1].AttestedAtMS {
		return ErrCorrupt
	}
	if p.Kind == "admission" {
		var ref registry.PublicAdmissionReference
		json.Unmarshal(p.Payload, &ref)
		a, ok := r.store.admissions[ref.AdmissionSequence]
		id, er := registry.PublicAdmissionID(r.store.pin, a)
		if !ok || er != nil || id != ref.AdmissionID {
			return ErrUnavailable
		}
	}
	batch := new(leveldb.Batch)
	raw, _ := json.Marshal(p)
	batch.Put(countKey("p/", n), raw)
	work := r.work
	work.NextPublication++
	if n == work.Target.PublicationSequence {
		id, _ := registry.PublicPublicationID(r.store.pin, p)
		if id != work.Target.PublicationHead {
			return ErrSource
		}
	}
	return r.save(ctx, batch, work, func() { r.store.publications[n] = p })
}
func (r *Replay) buildBranches() error {
	s := r.store
	s.mu.Lock()
	defer s.mu.Unlock()
	for n := uint32(1); n <= r.frameLimit; n++ {
		frame, ok := r.frames[n]
		if !ok {
			return ErrUnavailable
		}
		if frame.ChainHeight >= s.pin.Limits.Headers {
			return ErrCapacity
		}
		if frame.AdmissionSequence > r.work.Target.AdmissionSequence || frame.PublicationSequence > r.work.Target.PublicationSequence {
			return ErrUnavailable
		}
		fid, e := FinalityFrameID(s.pin, frame)
		if e != nil {
			return e
		}
		target := ReplayTarget{frame.AdmissionSequence, frame.AdmissionHead, frame.PublicationSequence, frame.PublicationHead, frame.ChainHeight, frame.ChainHash, &FrontierEvidence{"finality_frame", fid}}
		r.branches = append(r.branches, &replayBranch{target: target, frame: &frame, pinned: true, ancestryNext: int(frame.ChainHeight)})
	}
	b := &replayBranch{target: r.work.Target, ancestryNext: -1}
	if r.cutoff != nil {
		b.pinned = true
		b.ancestryNext = int(b.target.ChainHeight)
	}
	r.branches = append(r.branches, b)
	r.headersBuilt = true
	return nil
}

// One backward ancestry step loads at most one bounded historical block. These
// raw rows confer no authority: verifyHeader rechecks normal work forward.
func (r *Replay) loadAncestry(ctx context.Context, b *replayBranch) error {
	height := uint32(b.ancestryNext)
	hash := b.hashes[height]
	r.store.mu.Lock()
	row, ok := r.store.headers[hash]
	r.store.mu.Unlock()
	if !ok {
		raw, e := r.node.Block(ctx, hash)
		if e != nil {
			return e
		}
		if len(raw) < 80 {
			return ErrCorrupt
		}
		row = HeaderRow{1, r.store.pinID, height, hash, hex.EncodeToString(raw[:80])}
	}
	if !validHeader(r.store.pinID, row) || row.Height != height {
		return ErrCorrupt
	}
	raw, _ := hex.DecodeString(row.Header)
	var h wire.BlockHeader
	if h.Deserialize(bytes.NewReader(raw)) != nil {
		return ErrCorrupt
	}
	if height == 0 && hash != r.store.pin.ScanOrigin.Hash {
		return ErrSource
	}
	batch := new(leveldb.Batch)
	encoded, _ := json.Marshal(row)
	batch.Put(idKey("h/", hash), encoded)
	return r.save(ctx, batch, r.work, func() {
		r.store.headers[hash] = row
		if height > 0 {
			b.hashes[height-1] = h.PrevBlock.String()
		}
		b.ancestryNext--
	})
}
func (r *Replay) verifyHeader(ctx context.Context, branch *replayBranch) error {
	height := branch.headerNext
	var row HeaderRow
	if !branch.pinned {
		v, e := r.node.HeaderAt(ctx, height)
		if e != nil {
			return e
		}
		row = HeaderRow{1, r.store.pinID, height, v.Hash, hex.EncodeToString(v.Raw)}
	} else {
		r.store.mu.Lock()
		row = r.store.headers[branch.hashes[height]]
		r.store.mu.Unlock()
	}
	if !validHeader(r.store.pinID, row) {
		return ErrCorrupt
	}
	raw, _ := hex.DecodeString(row.Header)
	var header wire.BlockHeader
	if header.Deserialize(bytes.NewReader(raw)) != nil {
		return ErrCorrupt
	}
	if height == 0 {
		if row.Hash != r.store.pin.ScanOrigin.Hash || row.Hash != chaincfg.MainNetParams.GenesisHash.String() {
			return ErrSource
		}
	} else if header.PrevBlock.String() != branch.hashes[height-1] {
		return ErrStale
	}
	if height == branch.target.ChainHeight && row.Hash != branch.target.ChainHash {
		return ErrStale
	}
	if !r.verified[row.Hash] {
		select {
		case ownershipWork <- struct{}{}:
		default:
			return ErrBusy
		}
		got, hashErr := prospect.TryHashHeader(ctx, header)
		<-ownershipWork
		if errors.Is(hashErr, prospect.ErrHeaderHashBusy) {
			return ErrBusy
		}
		if errors.Is(hashErr, prospect.ErrHeaderHashProfile) {
			return ErrProfile
		}
		if hashErr != nil {
			return hashErr
		}
		if got.String() != row.Hash {
			return ErrCorrupt
		}
		if height > 0 {
			target := blockchain.CompactToBig(header.Bits)
			if target.Sign() <= 0 || target.Cmp(chaincfg.MainNetParams.PowLimit) > 0 || blockchain.HashToBig(&got).Cmp(target) > 0 {
				return ErrCorrupt
			}
		}
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	batch := new(leveldb.Batch)
	encoded, _ := json.Marshal(row)
	batch.Put(idKey("h/", row.Hash), encoded)
	if !branch.pinned {
		if old, ok := r.store.canonical[height]; ok && old != row.Hash {
			for n := range r.store.canonical {
				if n > height {
					batch.Delete(countKey("c/", n))
				}
			}
		}
		b, _ := hex.DecodeString(row.Hash)
		batch.Put(countKey("c/", height), b)
	}
	return r.save(ctx, batch, r.work, func() {
		r.store.headers[row.Hash] = row
		if !branch.pinned {
			if old, ok := r.store.canonical[height]; ok && old != row.Hash {
				for n := range r.store.canonical {
					if n > height {
						delete(r.store.canonical, n)
					}
				}
			}
			r.store.canonical[height] = row.Hash
		}
		branch.hashes[height] = row.Hash
		branch.headerNext++
		r.verified[row.Hash] = true
	})
}
func (r *Replay) stepChain(ctx context.Context) error {
	if !r.headersBuilt {
		if e := r.buildBranches(); e != nil {
			return e
		}
	}
	if r.branch >= len(r.branches) {
		work := r.work
		work.Phase = "ready"
		work.NextHeight = work.Target.ChainHeight + 1
		return r.save(ctx, new(leveldb.Batch), work, nil)
	}
	b := r.branches[r.branch]
	if b.hashes == nil {
		b.hashes = make([]string, int(b.target.ChainHeight)+1)
		if b.pinned {
			b.hashes[b.target.ChainHeight] = b.target.ChainHash
		}
	}
	if b.pinned && b.ancestryNext >= 0 {
		return r.loadAncestry(ctx, b)
	}
	if b.headerNext <= b.target.ChainHeight {
		return r.verifyHeader(ctx, b)
	}
	if b.effective == nil {
		if e := r.startBranch(b); e != nil {
			return e
		}
	}
	if b.next <= b.target.ChainHeight {
		return r.scanBlock(ctx, b)
	}
	if e := r.completeBranch(ctx, b); e != nil {
		return e
	}
	if b.frame != nil {
		b.hashes = nil
		b.effective = nil
		b.winners = nil
		b.transfers = nil
		b.commits = nil
		b.checkpoints = nil
		b.conflicted = nil
	}
	r.branch++
	return nil
}

var _ = errors.Is
var _ = math.MaxInt32
