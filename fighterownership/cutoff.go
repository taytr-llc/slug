// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterownership

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/registry"
)

// cutoffFrames selects only the immutable unsigned prefix captured at the lock.
// A later signed frame is irrelevant; a hole inside the captured prefix is not.
func cutoffFrames(pin Pin, intent CutoffIntent, frames map[uint32]FinalityFrame, bodies map[uint32]FinalityFrameBody) ([]FinalityFrame, error) {
	return (ownershipCodec{}).cutoffFrames(pin, intent, frames, bodies)
}
func (codec ownershipCodec) cutoffFrames(pin Pin, intent CutoffIntent, frames map[uint32]FinalityFrame, bodies map[uint32]FinalityFrameBody) ([]FinalityFrame, error) {
	if codec.ValidatePublicCutoffIntent(pin, intent) != nil {
		return nil, ErrInvalid
	}
	values := make([]FinalityFrame, 0, intent.FinalitySequence)
	previous := zeroID
	for n := uint32(1); n <= intent.FinalitySequence; n++ {
		f, ok := frames[n]
		body, present := bodies[n]
		if !ok || !present {
			return nil, ErrUnavailable
		}
		if f.Sequence != n || f.FinalityFrameBody != body || body.PreviousBodyID != previous || codec.ValidateFinalityFrame(pin, f) != nil {
			return nil, ErrCorrupt
		}
		id, e := objectDigest("SLUG fighter ownership finality body", body)
		if e != nil {
			return nil, e
		}
		previous = id
		values = append(values, f)
	}
	if previous != intent.FinalityBodyHead {
		return nil, ErrCorrupt
	}
	return values, nil
}

// BeginCutoffReplay never substitutes a current head for the captured boundary.
// The node supplies bounded historical blocks; Step rechecks their normal work
// and full payloads before a signed historical result can be published.
func (s *Store) BeginCutoffReplay(ctx context.Context, source *PublicSource, node *OwnedChainSource, boundary SignedBoundary) (*Replay, error) {
	return s.beginReplay(ctx, source, node, &boundary)
}

func (r *Replay) finishCutoff(ctx context.Context, b *replayBranch, mp OwnershipMap, mapID, fp string) (Progress, error) {
	s := r.store
	boundary := *r.cutoff
	if _, e := s.codec.cutoffFrames(s.pin, boundary.Intent, s.frames, s.frameBodies); e != nil {
		return Progress{}, e
	}
	batch := new(leveldb.Batch)
	raw, _ := json.Marshal(mp)
	batch.Put(idKey("m/", mapID), raw)
	conflictID := ""
	var conflict *ConflictRecord
	for _, item := range mp.Items {
		v, ok := b.conflicted[item.ProspectID]
		if !ok {
			continue
		}
		old := r.officialFrontier[v.StateID]
		bid, _ := s.codec.PublicBoundaryID(s.pin, boundary)
		current := FrontierEvidence{"boundary", bid}
		c := ConflictRecord{1, s.pinID, "finalized_branch_conflict", v.ProspectID, v.StateID, &old, &current, sortedIDs(old.ID, bid)}
		if s.codec.validateConflict(s.pin, c) != nil {
			return Progress{}, ErrCorrupt
		}
		var e error
		conflictID, e = objectDigest("SLUG fighter ownership conflict", c)
		if e != nil {
			return Progress{}, e
		}
		raw, _ = json.Marshal(c)
		batch.Put(idKey("x/", conflictID), raw)
		conflict = &c
		break
	}
	h := HeadRef{Version: 1, PinID: s.pinID, AdmissionSequence: b.target.AdmissionSequence, AdmissionHead: b.target.AdmissionHead, PublicationSequence: b.target.PublicationSequence, PublicationHead: b.target.PublicationHead, ChainHeight: b.target.ChainHeight, ChainHash: b.target.ChainHash, FinalizedPrefixID: fp, ConflictID: conflictID}
	var e error
	h.ReplayDigest, e = replayDigest(h, mapID)
	if e != nil {
		return Progress{}, e
	}
	c := CutoffRecord{Version: 1, PinID: s.pinID, EventID: boundary.Intent.EventID, CutoffMS: boundary.Intent.CutoffMS, Boundary: boundary, AdmissionSequence: h.AdmissionSequence, AdmissionHead: h.AdmissionHead, Head: h, OwnershipDigest: mapID}
	if r.source.reader != nil {
		published, er := r.source.reader.Cutoff(ctx, c.EventID)
		if er != nil {
			return Progress{}, er
		}
		unsigned := published
		unsigned.Signature = ""
		if unsigned != c {
			return Progress{}, ErrConflict
		}
		c = published
	} else {
		c.Signature, e = s.signSelectedObject(ctx, "SLUG fighter ownership cutoff statement", c)
		if e != nil {
			return Progress{}, e
		}
	}
	cid, e := s.codec.PublicCutoffID(s.pin, c)
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
	defer s.mu.Unlock()
	if e = s.state(); e != nil {
		return Progress{}, e
	}
	if s.replay != r {
		return Progress{}, ErrStale
	}
	if old, ok := s.cutoffs[c.EventID]; ok && old != c {
		return Progress{}, ErrConflict
	}
	raw, _ = json.Marshal(c)
	batch.Put(idKey("k/", c.EventID), raw)
	meta := s.meta
	meta.WorkID = ""
	batch.Delete([]byte("work"))
	now := s.now().UnixMilli()
	if now < meta.ClockHighWaterMS || now < 0 || now > MaxTimestampMS {
		return Progress{}, ErrClock
	}
	meta.ClockHighWaterMS = now
	if e = ctx.Err(); e != nil {
		return Progress{}, e
	}
	if e = s.checkOperatorSigning(ctx); e != nil {
		return Progress{}, e
	}
	if e = s.commit(batch, meta); e != nil {
		return Progress{}, e
	}
	s.maps[mapID] = mp
	s.cutoffs[c.EventID] = c
	s.cutoffByID[cid] = c.EventID
	s.work = nil
	if conflict != nil {
		s.conflicts[conflictID] = *conflict
	}
	// Check the original operation before this successful completion cancels
	// its own replay lifetime. Preserve cleanup even when the check refuses.
	e = s.checkOperatorSigning(ctx)
	s.replay = nil
	r.closed.Store(true)
	r.cancel()
	r.releaseMemory()
	if e != nil {
		return Progress{}, e
	}
	return Progress{WorkID: r.id, Phase: "ready", NextAdmission: h.AdmissionSequence + 1, NextPublication: h.PublicationSequence + 1, NextHeight: h.ChainHeight + 1, Ready: true, Head: &h, Cutoff: &c}, nil
}

// PublishCutoff publishes the already replayed exact historical record through
// the registry's retained original boundary. It performs no new lock capture.
func (s *Store) PublishCutoff(ctx context.Context, source *PublicSource, eventID string) (*PublicSource, PublicationEntry, error) {
	if s == nil || source == nil || source.publisher == nil || source.pin != s.pin || !hexValue(eventID, 32, true) {
		return nil, PublicationEntry{}, ErrInvalid
	}
	ctx, done, e := s.mutation(ctx)
	if e != nil {
		return nil, PublicationEntry{}, e
	}
	defer done()
	ctx, release, e := s.beginOperatorSigning(ctx, source)
	if e != nil {
		return nil, PublicationEntry{}, e
	}
	defer release()
	if e = source.Check(ctx); e != nil {
		return nil, PublicationEntry{}, e
	}
	s.mu.Lock()
	c, ok := s.cutoffs[eventID]
	s.mu.Unlock()
	if !ok {
		return nil, PublicationEntry{}, ErrNotFound
	}
	// A completed publication is an exact replay, not another publication entry.
	old, e := source.publisher.Cutoff(ctx, eventID)
	if e == nil {
		if old != c {
			return nil, PublicationEntry{}, ErrConflict
		}
		// Find the one exact retained index through the original bounded prefix.
		id, _ := s.codec.PublicCutoffID(s.pin, c)
		for n := uint32(1); n <= source.status.PublicationSequence; n++ {
			entry, er := source.publisher.Publication(ctx, n)
			if er != nil {
				return nil, PublicationEntry{}, er
			}
			if entry.Kind != "cutoff" {
				continue
			}
			var ref registry.PublicCutoffReference
			if json.Unmarshal(entry.Payload, &ref) == nil && ref.CutoffID == id {
				return source, entry, nil
			}
		}
		return nil, PublicationEntry{}, ErrCorrupt
	}
	if !errors.Is(e, registry.ErrNotFound) {
		return nil, PublicationEntry{}, e
	}
	prepared, e := source.publisher.PrepareCutoffPublication(ctx, c)
	if e != nil {
		return nil, PublicationEntry{}, e
	}
	entry := prepared.Entry()
	entry.Signature, e = s.signSelectedObject(ctx, "SLUG fighter ownership publication statement", entry)
	if e != nil {
		return nil, PublicationEntry{}, e
	}
	if e = source.Check(ctx); e != nil {
		return nil, PublicationEntry{}, e
	}
	entry, e = s.commitSelectedPublication(ctx, prepared, entry)
	if e != nil {
		return nil, PublicationEntry{}, e
	}
	next, e := s.recaptureSource(ctx, source)
	return next, entry, e
}
