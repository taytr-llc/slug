// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterownership

import (
	"context"
	"encoding/json"
	"github.com/syndtr/goleveldb/leveldb"
)

// stepFrame performs one bounded public row read. Frames remain untrusted
// memory until each complete historical branch has been normally replayed.
func (r *Replay) stepFrame(ctx context.Context) error {
	n := r.frameNext
	f, e := r.source.reader.FinalityFrame(ctx, n)
	if e != nil {
		return e
	}
	if f.Sequence != n || r.store.codec.ValidateFinalityFrame(r.store.pin, f) != nil {
		return ErrCorrupt
	}
	previous := zeroID
	if n > 1 {
		old, ok := r.frames[n-1]
		if !ok {
			return ErrUnavailable
		}
		previous, e = objectDigest("SLUG fighter ownership finality body", old.FinalityFrameBody)
		if e != nil {
			return e
		}
	}
	if f.PreviousBodyID != previous {
		return ErrCorrupt
	}
	if old, ok := r.store.frames[n]; ok && old != f {
		return ErrConflict
	}
	if f.AdmissionSequence > r.work.Target.AdmissionSequence || f.PublicationSequence > r.work.Target.PublicationSequence {
		return ErrUnavailable
	}
	if n == r.frameLimit {
		body, e := objectDigest("SLUG fighter ownership finality body", f.FinalityFrameBody)
		if e != nil {
			return e
		}
		if r.cutoff != nil {
			if body != r.cutoff.Intent.FinalityBodyHead {
				return ErrCorrupt
			}
		} else {
			signed, e := r.store.codec.FinalityFrameID(r.store.pin, f)
			if e != nil {
				return e
			}
			if body != r.source.remoteStatus.FinalityBodyHead || signed != r.source.remoteStatus.FinalityHead {
				return ErrSource
			}
		}
	}
	if e = r.source.Check(ctx); e != nil {
		return e
	}
	if e = r.node.Check(ctx, r.observation); e != nil {
		return e
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	r.frames[n] = f
	r.frameNext++
	return nil
}

// retainFrame composes only a fully replayed frame with its map and evidence.
// Importing a signed frame alone never mints an irreversible local head.
func (r *Replay) retainFrame(b *replayBranch, batch *leveldb.Batch) (*storeMetadata, map[string]FinalityWitness, error) {
	if b.frame == nil {
		return nil, nil, nil
	}
	s := r.store
	f := *b.frame
	if old, ok := s.frames[f.Sequence]; ok {
		if old != f {
			return nil, nil, ErrConflict
		}
		return nil, nil, nil
	}
	if f.Sequence != s.meta.PublishedFinalitySequence+1 || s.meta.PublishedFinalitySequence != s.meta.FinalitySequence {
		return nil, nil, ErrUnavailable
	}
	fid, e := s.codec.FinalityFrameID(s.pin, f)
	if e != nil {
		return nil, nil, e
	}
	raw, _ := json.Marshal(f.FinalityFrameBody)
	batch.Put(countKey("fo/", f.Sequence), raw)
	raw, _ = json.Marshal(f)
	batch.Put(countKey("f/", f.Sequence), raw)
	meta := s.meta
	meta.FinalitySequence = f.Sequence
	meta.PublishedFinalitySequence = f.Sequence
	meta.FinalityHead = fid
	witnesses := map[string]FinalityWitness{}
	for _, state := range b.effective {
		if state.Generation == 0 {
			continue
		}
		if old, ok := r.officialFrontier[state.StateID]; ok && old.ID != fid {
			continue
		}
		var ev FinalStateEvidence
		if json.Unmarshal(state.Evidence, &ev) != nil {
			return nil, nil, ErrCorrupt
		}
		w := FinalityWitness{1, s.pinID, state.StateID, FrontierEvidence{"finality_frame", fid}, ev.CheckpointPublicationID, ev.SixtiethDescendantHeight, ev.SixtiethDescendantHash}
		front, e := objectDigest("SLUG fighter ownership frontier evidence", w.Frontier)
		if e != nil {
			return nil, nil, e
		}
		key := append(idKey("v/", state.StateID), idKey("", front)...)
		raw, _ := json.Marshal(w)
		batch.Put(key, raw)
		witnesses[string(key)] = w
	}
	return &meta, witnesses, nil
}

func (r *Replay) releaseMemory() {
	r.branches = nil
	r.verified = nil
	r.verifiedAdmissions = nil
	r.official = nil
	r.officialFrontier = nil
	r.states = nil
	r.newWitnesses = nil
	r.frames = nil
}
