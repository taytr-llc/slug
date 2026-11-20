// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterownership

import (
	"context"
	"encoding/json"
	"math"

	"github.com/syndtr/goleveldb/leveldb"
)

func (r *Replay) finish(ctx context.Context) (Progress, error) {
	if len(r.branches) == 0 || r.branch != len(r.branches) {
		return Progress{}, ErrUnavailable
	}
	b := r.branches[len(r.branches)-1]
	mp, e := r.branchMap(b)
	if e != nil {
		return Progress{}, e
	}
	mapID, e := OwnershipMapID(r.store.pin, mp)
	if e != nil {
		return Progress{}, e
	}
	fp, _ := FinalizedPrefixID(r.store.pin, mp)
	if r.cutoff != nil {
		return r.finishCutoff(ctx, b, mp, mapID, fp)
	}
	s := r.store
	batch := new(leveldb.Batch)
	raw, _ := json.Marshal(mp)
	batch.Put(idKey("m/", mapID), raw)
	needFrame := false
	for _, v := range b.effective {
		old, ok := r.official[v.ProspectID]
		if v.Generation > 0 && (!ok || old.StateID != v.StateID) {
			needFrame = true
		}
	}
	// A previously signed frame of this exact branch/map already supplies the
	// contradictory frontier. Replaying it must not grow another frame forever.
	contradictionFrameID := ""
	if len(b.conflicted) > 0 {
		for n := uint32(1); n <= r.frameLimit; n++ {
			f, ok := r.frames[n]
			if ok && f.ChainHash == b.target.ChainHash && f.ChainHeight == b.target.ChainHeight && f.OwnershipMapID == mapID && f.FinalizedPrefixID == fp {
				contradictionFrameID, _ = FinalityFrameID(s.pin, f)
			}
		}
		if contradictionFrameID == "" {
			needFrame = true
		}
	}
	// A remote reader cannot sign as its publication source. Current conditions
	// may prove new ownership, but complete reconstruction awaits source evidence.
	if needFrame && r.source.reader != nil {
		return Progress{}, ErrUnavailable
	}
	meta := s.meta
	newWitnesses := map[string]FinalityWitness{}
	var frame *FinalityFrame
	var conflict *ConflictRecord
	conflictID := ""
	if needFrame {
		if meta.FinalitySequence >= 4096 || meta.PublishedFinalitySequence != meta.FinalitySequence {
			return Progress{}, ErrUnavailable
		}
		previous := zeroID
		if meta.FinalitySequence > 0 {
			previous, e = objectDigest("SLUG fighter ownership finality body", s.frameBodies[meta.FinalitySequence])
			if e != nil {
				return Progress{}, e
			}
		}
		body := FinalityFrameBody{1, s.pinID, meta.FinalitySequence + 1, previous, b.target.AdmissionSequence, b.target.AdmissionHead, b.target.PublicationSequence, b.target.PublicationHead, b.target.ChainHeight, b.target.ChainHash, mapID, fp}
		f := FinalityFrame{FinalityFrameBody: body}
		f.Signature, e = signObject("SLUG fighter ownership finality statement", f)
		if e != nil {
			return Progress{}, e
		}
		fid, e := FinalityFrameID(s.pin, f)
		if e != nil {
			return Progress{}, e
		}
		frame = &f
		raw, _ := json.Marshal(body)
		batch.Put(countKey("fo/", body.Sequence), raw)
		raw, _ = json.Marshal(f)
		batch.Put(countKey("f/", body.Sequence), raw)
		meta.FinalitySequence = body.Sequence
		meta.PublishedFinalitySequence = body.Sequence
		meta.FinalityHead = fid
		for _, v := range b.effective {
			if v.Generation == 0 {
				continue
			}
			if old, ok := r.official[v.ProspectID]; ok && old.StateID == v.StateID {
				continue
			}
			var ev FinalStateEvidence
			json.Unmarshal(v.Evidence, &ev)
			w := FinalityWitness{1, s.pinID, v.StateID, FrontierEvidence{"finality_frame", fid}, ev.CheckpointPublicationID, ev.SixtiethDescendantHeight, ev.SixtiethDescendantHash}
			frontID, _ := objectDigest("SLUG fighter ownership frontier evidence", w.Frontier)
			key := append(idKey("v/", v.StateID), idKey("", frontID)...)
			raw, _ = json.Marshal(w)
			batch.Put(key, raw)
			newWitnesses[string(key)] = w
		}
		if len(b.conflicted) > 0 {
			contradictionFrameID = fid
		}
	}
	if len(b.conflicted) > 0 {
		// One exact signed contradictory frontier preserves the official owner.
		for _, item := range mp.Items {
			v, ok := b.conflicted[item.ProspectID]
			if !ok {
				continue
			}
			old := r.officialFrontier[v.StateID]
			current := FrontierEvidence{"finality_frame", contradictionFrameID}
			c := ConflictRecord{1, s.pinID, "finalized_branch_conflict", v.ProspectID, v.StateID, &old, &current, sortedIDs(old.ID, contradictionFrameID)}
			if validateConflict(s.pin, c) != nil {
				return Progress{}, ErrCorrupt
			}
			conflictID, e = objectDigest("SLUG fighter ownership conflict", c)
			if e != nil {
				return Progress{}, e
			}
			encoded, _ := json.Marshal(c)
			batch.Put(idKey("x/", conflictID), encoded)
			conflict = &c
			break
		}
	}

	h := HeadRef{Version: 1, PinID: s.pinID, AdmissionSequence: b.target.AdmissionSequence, AdmissionHead: b.target.AdmissionHead, PublicationSequence: b.target.PublicationSequence, PublicationHead: b.target.PublicationHead, ChainHeight: b.target.ChainHeight, ChainHash: b.target.ChainHash, FinalizedPrefixID: fp, ConflictID: conflictID}
	h.ReplayDigest, e = replayDigest(h, mapID)
	if e != nil {
		return Progress{}, e
	}
	if e = r.source.Check(ctx); e != nil {
		return Progress{}, e
	}
	if e = r.node.Check(ctx, r.observation); e != nil {
		return Progress{}, e
	}
	if e = ctx.Err(); e != nil {
		return Progress{}, e
	}
	s.mu.Lock()
	if e = s.state(); e != nil {
		s.mu.Unlock()
		return Progress{}, e
	}
	if s.replay != r || s.meta.Revision != meta.Revision {
		s.mu.Unlock()
		return Progress{}, ErrStale
	}
	now := s.now().UnixMilli()
	if now < 0 || now > MaxTimestampMS || now < meta.ClockHighWaterMS {
		s.mu.Unlock()
		return Progress{}, ErrClock
	}
	if meta.Revision == math.MaxUint32 {
		s.mu.Unlock()
		return Progress{}, ErrCapacity
	}
	meta.ClockHighWaterMS = now
	meta.CurrentHead = &h
	meta.CurrentMapID = mapID
	meta.WorkID = ""
	batch.Delete([]byte("work"))
	// A shorter selected branch leaves no trailing c/ index from its predecessor.
	for n := range s.canonical {
		if n > h.ChainHeight {
			batch.Delete(countKey("c/", n))
		}
	}
	if e = s.commit(batch, meta); e != nil {
		s.mu.Unlock()
		return Progress{}, e
	}
	s.maps[mapID] = mp
	s.head = h
	s.work = nil
	s.ready = false
	s.effective = b.effective
	s.commits = b.commits
	s.source = r.source
	s.node = r.node
	s.observation = r.observation
	for n := range s.canonical {
		if n > h.ChainHeight {
			delete(s.canonical, n)
		}
	}
	if frame != nil {
		s.frames[frame.Sequence] = *frame
		s.frameBodies[frame.Sequence] = frame.FinalityFrameBody
	}
	if conflict != nil {
		s.conflicts[conflictID] = *conflict
	}
	for key, v := range newWitnesses {
		s.finalities[key] = v
	}
	s.mu.Unlock()
	if e = r.source.Check(ctx); e == nil {
		e = r.node.Check(ctx, r.observation)
	}
	if e == nil {
		e = ctx.Err()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if e != nil {
		s.replay = nil
		r.closed.Store(true)
		r.cancel()
		return Progress{}, e
	}
	if se := s.state(); se != nil {
		return Progress{}, se
	}
	s.portable = map[string]bool{}
	for id := range r.states {
		s.portable[id] = true
	}
	s.ready = true
	s.replay = nil
	r.closed.Store(true)
	r.cancel()
	r.releaseMemory()
	return Progress{WorkID: r.id, Phase: "ready", NextAdmission: h.AdmissionSequence + 1, NextPublication: h.PublicationSequence + 1, NextHeight: h.ChainHeight + 1, Ready: true, Head: &h}, nil
}
func (s *Store) TransferStatus(ctx context.Context, id string) (TransferStatus, error) {
	if !hexValue(id, 32, true) {
		return TransferStatus{}, ErrInvalid
	}
	if e := s.readLock(ctx); e != nil {
		return TransferStatus{}, e
	}
	defer s.mu.Unlock()
	out := TransferStatus{TransferID: id, ConflictID: s.head.ConflictID}
	found := false
	for _, w := range s.witnesses {
		if w.TransferID == id {
			found = true
			out.State = "pending"
			break
		}
	}
	if !found {
		return TransferStatus{}, ErrNotFound
	}
	for _, v := range s.states {
		if v.Generation == 0 {
			continue
		}
		var ev FinalStateEvidence
		json.Unmarshal(v.Evidence, &ev)
		if ev.TransferID != id {
			continue
		}
		out.State = "finalized"
		owned := cloneState(v)
		out.EffectiveState = &owned
		out.PublicEvidencePending = !s.portable[v.StateID]
		return out, nil
	}
	return out, nil
}
