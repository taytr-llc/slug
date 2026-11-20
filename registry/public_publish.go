// Copyright (c) 2026 The SLUG developers. ISC license.
package registry

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/chaincfg/v2"
)

type publicPublicationIntent struct {
	Version      uint32          `json:"version"`
	PinID        string          `json:"pin_id"`
	Sequence     uint32          `json:"sequence"`
	PreviousID   string          `json:"previous_id"`
	AttestedAtMS int64           `json:"attested_at_ms"`
	Kind         string          `json:"kind"`
	Payload      json.RawMessage `json:"payload"`
	AdmissionID  string          `json:"admission_id"`
}

func (v publicPublicationIntent) entry() PublicPublicationEntry {
	return PublicPublicationEntry{v.Version, v.PinID, v.Sequence, v.PreviousID, v.AttestedAtMS, v.Kind, append(json.RawMessage(nil), v.Payload...), ""}
}
func (s *ManifestStore) validatePending(v publicPublicationIntent) error {
	p := s.public
	if p == nil || v.Version != 1 || v.PinID != p.pinID || v.Sequence != p.meta.PublicationSequence+1 || v.Sequence > 16384 || v.PreviousID != p.meta.PublicationHead || v.AttestedAtMS < 0 || v.AttestedAtMS > MaxPublicTimestampMS || v.AttestedAtMS > p.meta.ClockHighWaterMS || validatePublicPayload(p.pin, v.entry()) != nil {
		return ErrCorrupt
	}
	if v.Kind == "admission" {
		var ref PublicAdmissionReference
		if json.Unmarshal(v.Payload, &ref) != nil || ref.AdmissionSequence != p.meta.SignedAdmissionSequence+1 || ref.AdmissionSequence > p.meta.AcceptedSequence || ref.AdmissionID != v.AdmissionID {
			return ErrCorrupt
		}
	} else if v.AdmissionID != "" {
		return ErrCorrupt
	}
	return nil
}
func (p *ManifestPublication) acquire(ctx context.Context) (context.Context, func(), error) {
	if ctx == nil {
		return nil, nil, ErrPublicOwnership
	}
	if e := ctx.Err(); e != nil {
		return nil, nil, e
	}
	if e := p.Alive(); e != nil {
		return nil, nil, e
	}
	select {
	case p.gate <- struct{}{}:
	default:
		return nil, nil, ErrBusy
	}
	owned, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(p.life, cancel)
	done := func() { stop(); cancel(); <-p.gate }
	if e := p.Alive(); e != nil {
		done()
		return nil, nil, e
	}
	return owned, done, nil
}
func (p *ManifestPublication) readPending() (*publicPublicationIntent, error) {
	s := p.store
	if !s.mu.TryLock() {
		return nil, ErrBusy
	}
	defer s.mu.Unlock()
	if e := s.state(false); e != nil {
		return nil, e
	}
	if s.public.meta.PendingPublicationID == "" {
		return nil, nil
	}
	raw, e := s.db.Get([]byte("ownership/pending"), nil)
	var v publicPublicationIntent
	if e != nil || publicDecode(raw, MaxPublicSmallSize, &v) != nil || s.validatePending(v) != nil {
		return nil, ErrCorrupt
	}
	return &v, nil
}
func (p *ManifestPublication) admission(ctx context.Context, n uint32) (PublicAdmissionStatement, error) {
	s := p.store
	if !s.mu.TryLock() {
		return PublicAdmissionStatement{}, ErrBusy
	}
	if e := s.state(false); e != nil {
		s.mu.Unlock()
		return PublicAdmissionStatement{}, e
	}
	if n == 0 || n != s.public.meta.SignedAdmissionSequence+1 || n > s.public.meta.AcceptedSequence {
		s.mu.Unlock()
		return PublicAdmissionStatement{}, ErrPublicOwnership
	}
	id, e := s.db.Get(sequenceKey(uint64(n)), nil)
	if e != nil {
		s.mu.Unlock()
		return PublicAdmissionStatement{}, ErrCorrupt
	}
	raw, e := s.db.Get(recordKey(string(id)), nil)
	if e != nil {
		s.mu.Unlock()
		return PublicAdmissionStatement{}, ErrCorrupt
	}
	r, e := s.indexedRecord(string(id), raw)
	previous := s.public.meta.AdmissionHead
	s.mu.Unlock()
	if e != nil {
		return PublicAdmissionStatement{}, e
	}
	if e = ctx.Err(); e != nil {
		return PublicAdmissionStatement{}, e
	}
	if e = verifyManifestRecord(r); e != nil {
		return PublicAdmissionStatement{}, e
	}
	if e = ctx.Err(); e != nil {
		return PublicAdmissionStatement{}, e
	}
	digest, e := PublicOwnershipDigest("SLUG fighter ownership admission record", r)
	if e != nil {
		return PublicAdmissionStatement{}, e
	}
	a := PublicAdmissionStatement{1, p.pinID, n, previous, digest, ownedManifestRecord(r), ""}
	a.Signature, e = p.signAdmission(ctx, a)
	if e != nil {
		return PublicAdmissionStatement{}, e
	}
	return a, p.codec.ValidatePublicAdmission(p.pin, a)
}
func (p *ManifestPublication) stage(ctx context.Context, kind string, payload json.RawMessage, admissionID string) (publicPublicationIntent, error) {
	var v publicPublicationIntent
	s := p.store
	if !s.mu.TryLock() {
		return v, ErrBusy
	}
	defer s.mu.Unlock()
	if e := s.state(true); e != nil {
		return v, e
	}
	if e := ctx.Err(); e != nil {
		return v, e
	}
	m := s.public.meta
	if m.PendingPublicationID != "" {
		return v, ErrBusy
	}
	if m.PublicationSequence >= 16384 {
		if m.PublicationSequence == 16384 && validatePublicPayload(p.pin, PublicPublicationEntry{Kind: kind, Payload: payload}) == nil {
			// Capacity cannot hide a different chain predecessor. This exact
			// identity check precedes the drain classification even at the cap.
			if kind == "chain" {
				var ref PublicChainReference
				if json.Unmarshal(payload, &ref) != nil || ref.PreviousChainEntryID != m.ChainEntryID {
					return v, ErrConflict
				}
			}
			return v, ErrPublicCapacity
		}
		return v, ErrPublicOwnership
	}
	now, e := s.clockMillis()
	if e != nil {
		return v, e
	}
	if now > MaxPublicTimestampMS {
		return v, ErrClock
	}
	v = publicPublicationIntent{1, p.pinID, m.PublicationSequence + 1, m.PublicationHead, now, kind, append(json.RawMessage(nil), payload...), admissionID}
	if validatePublicPayload(p.pin, v.entry()) != nil {
		return publicPublicationIntent{}, ErrPublicOwnership
	}
	if kind == "chain" {
		var ref PublicChainReference
		if json.Unmarshal(payload, &ref) != nil || ref.PreviousChainEntryID != m.ChainEntryID {
			return publicPublicationIntent{}, ErrConflict
		}
	}
	m.ClockHighWaterMS = now
	id, e := PublicOwnershipDigest("SLUG fighter ownership publication intent", v)
	if e != nil {
		return v, e
	}
	m.PendingPublicationID = id
	batch := new(leveldb.Batch)
	raw, _ := json.Marshal(v)
	batch.Put([]byte("ownership/pending"), raw)
	raw, _ = json.Marshal(m)
	batch.Put([]byte(publicMetaKey), raw)
	next := s.meta
	next.LastClockMS = now
	if e = s.commit(batch, next); e != nil {
		return publicPublicationIntent{}, e
	}
	return v, nil
}
func (p *ManifestPublication) complete(ctx context.Context, v publicPublicationIntent, a *PublicAdmissionStatement) (PublicPublicationEntry, error) {
	e := v.entry()
	var err error
	e.Signature, err = p.signPublication(ctx, e)
	if err != nil {
		return PublicPublicationEntry{}, err
	}
	return p.commitSigned(ctx, v, a, e, nil)
}

// commitSigned accepts only the exact staged bytes. Chain and cutoff signing
// belongs to the concrete ownership coordinator, never the registry replayer.
func (p *ManifestPublication) commitSigned(ctx context.Context, v publicPublicationIntent, a *PublicAdmissionStatement, e PublicPublicationEntry, cutoff *PublicCutoffRecord) (PublicPublicationEntry, error) {
	unsigned := e
	unsigned.Signature = ""
	wantEntry, _ := json.Marshal(v.entry())
	gotEntry, _ := json.Marshal(unsigned)
	if !bytes.Equal(wantEntry, gotEntry) {
		return PublicPublicationEntry{}, ErrConflict
	}
	id, err := p.codec.PublicPublicationID(p.pin, e)
	if err != nil {
		return PublicPublicationEntry{}, err
	}
	if err = ctx.Err(); err != nil {
		return PublicPublicationEntry{}, err
	}
	s := p.store
	if !s.mu.TryLock() {
		return PublicPublicationEntry{}, ErrBusy
	}
	defer s.mu.Unlock()
	if err = s.state(true); err != nil {
		return PublicPublicationEntry{}, err
	}
	if err = ctx.Err(); err != nil {
		return PublicPublicationEntry{}, err
	}
	if s.validatePending(v) != nil {
		return PublicPublicationEntry{}, ErrCorrupt
	}
	raw, err := s.db.Get([]byte("ownership/pending"), nil)
	want, _ := json.Marshal(v)
	if err != nil || !bytes.Equal(raw, want) {
		return PublicPublicationEntry{}, ErrConflict
	}
	m := s.public.meta
	batch := new(leveldb.Batch)
	if v.Kind == "admission" {
		if a == nil || a.AdmissionSequence != m.SignedAdmissionSequence+1 {
			return PublicPublicationEntry{}, ErrConflict
		}
		aid, er := p.codec.PublicAdmissionID(p.pin, *a)
		if er != nil || aid != v.AdmissionID || a.PreviousAdmissionID != m.AdmissionHead {
			return PublicPublicationEntry{}, ErrConflict
		}
		raw, _ = json.Marshal(a)
		batch.Put(publicCounterKey("ownership/admission/", a.AdmissionSequence), raw)
		m.SignedAdmissionSequence = a.AdmissionSequence
		m.AdmissionHead = aid
	}
	if v.Kind == "checkpoint" {
		var ref struct {
			Checkpoint chaincfg.SignedCheckpoint `json:"checkpoint"`
		}
		json.Unmarshal(v.Payload, &ref)
		raw, _ = json.Marshal(ref.Checkpoint)
		cid := publicHash("SLUG fighter ownership checkpoint", raw)
		k := publicIDKey("ownership/checkpoint/", cid)
		if _, er := s.db.Get(k, nil); !errors.Is(er, leveldb.ErrNotFound) {
			return PublicPublicationEntry{}, ErrConflict
		}
		var seq [4]byte
		binary.BigEndian.PutUint32(seq[:], v.Sequence)
		batch.Put(k, seq[:])
		m.CheckpointCount++
	}
	if v.Kind == "chain" {
		var ref PublicChainReference
		if json.Unmarshal(v.Payload, &ref) != nil || ref.PreviousChainEntryID != m.ChainEntryID {
			return PublicPublicationEntry{}, ErrConflict
		}
		m.ChainEntryID = id
	}
	if v.Kind == "cutoff" {
		if cutoff == nil || p.codec.ValidatePublicCutoff(p.pin, *cutoff) != nil || m.CutoffCount > 256 {
			return PublicPublicationEntry{}, ErrPublicOwnership
		}
		cid, er := p.codec.PublicCutoffID(p.pin, *cutoff)
		var ref PublicCutoffReference
		if er != nil || json.Unmarshal(v.Payload, &ref) != nil || ref.CutoffID != cid {
			return PublicPublicationEntry{}, ErrConflict
		}
		intent, er := s.db.Get(publicIDKey("ownership/cutoff-intent/", cutoff.EventID), nil)
		want, _ := json.Marshal(cutoff.Boundary.Intent)
		if er != nil || !bytes.Equal(intent, want) {
			return PublicPublicationEntry{}, ErrConflict
		}
		if _, er = s.db.Get(publicIDKey("ownership/cutoff/", cutoff.EventID), nil); !errors.Is(er, leveldb.ErrNotFound) {
			return PublicPublicationEntry{}, ErrConflict
		}
		if m.CutoffCount == 256 {
			return PublicPublicationEntry{}, ErrPublicCapacity
		}
		raw, _ := json.Marshal(cutoff)
		batch.Put(publicIDKey("ownership/cutoff/", cutoff.EventID), raw)
		m.CutoffCount++
	}
	raw, _ = json.Marshal(e)
	batch.Put(publicCounterKey("ownership/publication/", e.Sequence), raw)
	batch.Delete([]byte("ownership/pending"))
	m.PublicationSequence = e.Sequence
	m.PublicationHead = id
	m.PendingPublicationID = ""
	raw, _ = json.Marshal(m)
	batch.Put([]byte(publicMetaKey), raw)
	if err = p.checkOperatorAction(ctx); err != nil {
		return PublicPublicationEntry{}, err
	}
	if err = s.commit(batch, s.meta); err != nil {
		return PublicPublicationEntry{}, err
	}
	if err = p.checkOperatorAction(ctx); err != nil {
		return PublicPublicationEntry{}, err
	}
	return e, nil
}
func (p *ManifestPublication) resume(ctx context.Context, v publicPublicationIntent) (PublicPublicationEntry, error) {
	if v.Kind != "admission" && v.Kind != "checkpoint" {
		return PublicPublicationEntry{}, ErrBusy
	}
	var a *PublicAdmissionStatement
	if v.Kind == "admission" {
		var ref PublicAdmissionReference
		if json.Unmarshal(v.Payload, &ref) != nil {
			return PublicPublicationEntry{}, ErrCorrupt
		}
		value, e := p.admission(ctx, ref.AdmissionSequence)
		if e != nil {
			return PublicPublicationEntry{}, e
		}
		a = &value
	}
	return p.complete(ctx, v, a)
}
func (p *ManifestPublication) PublishNext(ctx context.Context) (PublicationStatus, error) {
	ctx, done, e := p.acquire(ctx)
	if e != nil {
		return PublicationStatus{}, e
	}
	defer done()
	ctx, release, e := p.beginOperatorAction(ctx)
	if e != nil {
		return PublicationStatus{}, e
	}
	defer release()
	pending, e := p.readPending()
	if e != nil {
		return PublicationStatus{}, e
	}
	if pending != nil {
		if _, e = p.resume(ctx, *pending); e != nil {
			return PublicationStatus{}, e
		}
		return p.Status(ctx)
	}
	status, e := p.Status(ctx)
	if e != nil || status.SignedAdmissionSequence == status.AcceptedSequence {
		return status, e
	}
	a, e := p.admission(ctx, status.SignedAdmissionSequence+1)
	if e != nil {
		return PublicationStatus{}, e
	}
	id, e := p.codec.PublicAdmissionID(p.pin, a)
	if e != nil {
		return PublicationStatus{}, e
	}
	payload, _ := json.Marshal(PublicAdmissionReference{a.AdmissionSequence, id})
	v, e := p.stage(ctx, "admission", payload, id)
	if e != nil {
		return PublicationStatus{}, e
	}
	if _, e = p.complete(ctx, v, &a); e != nil {
		return PublicationStatus{}, e
	}
	return p.Status(ctx)
}
func (p *ManifestPublication) PublishCheckpoint(ctx context.Context, c chaincfg.SignedCheckpoint) (PublicPublicationEntry, error) {
	ctx, done, e := p.acquire(ctx)
	if e != nil {
		return PublicPublicationEntry{}, e
	}
	defer done()
	ctx, release, e := p.beginOperatorAction(ctx)
	if e != nil {
		return PublicPublicationEntry{}, e
	}
	defer release()
	if validatePublicCheckpoint(p.pin, c) != nil {
		return PublicPublicationEntry{}, ErrPublicOwnership
	}
	raw, e := json.Marshal(c)
	if e != nil {
		return PublicPublicationEntry{}, e
	}
	cid := publicHash("SLUG fighter ownership checkpoint", raw)
	s := p.store
	if !s.mu.TryLock() {
		return PublicPublicationEntry{}, ErrBusy
	}
	if e = s.state(false); e != nil {
		s.mu.Unlock()
		return PublicPublicationEntry{}, e
	}
	seq, e := s.db.Get(publicIDKey("ownership/checkpoint/", cid), nil)
	if e == nil {
		if len(seq) != 4 {
			s.mu.Unlock()
			return PublicPublicationEntry{}, ErrCorrupt
		}
		raw, e = s.db.Get(publicCounterKey("ownership/publication/", binary.BigEndian.Uint32(seq)), nil)
		s.mu.Unlock()
		if e != nil {
			return PublicPublicationEntry{}, e
		}
		return p.codec.ParsePublicPublication(p.pin, raw)
	}
	if !errors.Is(e, leveldb.ErrNotFound) {
		s.mu.Unlock()
		return PublicPublicationEntry{}, e
	}
	count := s.public.meta.CheckpointCount
	s.mu.Unlock()
	if count == 1024 {
		return PublicPublicationEntry{}, ErrPublicCapacity
	}
	if count > 1024 {
		return PublicPublicationEntry{}, ErrPublicOwnership
	}
	payload, _ := json.Marshal(struct {
		Checkpoint chaincfg.SignedCheckpoint `json:"checkpoint"`
	}{c})
	pending, e := p.readPending()
	if e != nil {
		return PublicPublicationEntry{}, e
	}
	if pending != nil {
		if pending.Kind != "checkpoint" || !bytes.Equal(pending.Payload, payload) {
			return PublicPublicationEntry{}, ErrBusy
		}
		return p.resume(ctx, *pending)
	}
	v, e := p.stage(ctx, "checkpoint", payload, "")
	if e != nil {
		return PublicPublicationEntry{}, e
	}
	return p.complete(ctx, v, nil)
}
