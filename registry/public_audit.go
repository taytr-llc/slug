// Copyright (c) 2026 The SLUG developers. ISC license.
package registry

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/taytr-llc/slug/chaincfg/v2"
)

func (s *ManifestStore) auditPublic() error { return s.auditPublicContext(context.Background()) }

func (s *ManifestStore) auditPublicContext(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p := s.public
	if p == nil {
		return ErrPublicOwnership
	}
	raw, e := s.db.Get([]byte(publicMetaKey), nil)
	if e != nil || publicDecode(raw, MaxPublicSmallSize, &p.meta) != nil {
		return ErrCorrupt
	}
	m := p.meta
	if m.Version != 1 || m.PinID != p.pinID || uint64(m.AcceptedSequence) != s.meta.LastSequence || m.AcceptedSequence > 1000 || m.SignedAdmissionSequence > m.AcceptedSequence || m.PublicationSequence > 16384 || !publicPrefix(m.SignedAdmissionSequence, m.AdmissionHead) || !publicPrefix(m.PublicationSequence, m.PublicationHead) || !publicHex(m.ChainEntryID, 32, false) || m.CheckpointCount > 1024 || m.CutoffCount > 256 || m.ClockHighWaterMS < s.meta.LastClockMS || m.ClockHighWaterMS > MaxPublicTimestampMS || m.PendingPublicationID != "" && !publicHex(m.PendingPublicationID, 32, true) {
		return ErrCorrupt
	}
	outboxes := map[uint32]publicOutbox{}
	admissions := map[uint32]PublicAdmissionStatement{}
	publications := map[uint32]PublicPublicationEntry{}
	checkpointIndices := map[string]uint32{}
	intents := map[string]PublicCutoffIntent{}
	cutoffs := map[string]PublicCutoffRecord{}
	var pending *publicPublicationIntent
	guardSeen := false
	var size uint64
	it := s.db.NewIterator(nil, nil)
	defer it.Release()
	for it.Next() {
		if err := ctx.Err(); err != nil {
			return err
		}
		key, value := it.Key(), it.Value()
		size += uint64(len(key) + len(value))
		if size > uint64(p.pin.Limits.LogicalBytes) {
			return ErrCorrupt
		}
		if !bytes.HasPrefix(key, []byte("ownership/")) {
			continue
		}
		if !s.publicKeyAllowed(key) {
			return ErrCorrupt
		}
		switch {
		case bytes.Equal(key, []byte("ownership/guard")):
			var g publicGuard
			if publicDecode(value, MaxPublicSmallSize, &g) != nil || g != p.codec.guard(p.pin) {
				return ErrCorrupt
			}
			guardSeen = true
		case bytes.Equal(key, []byte(publicMetaKey)):
		case bytes.Equal(key, []byte("ownership/pending")):
			var v publicPublicationIntent
			if publicDecode(value, MaxPublicSmallSize, &v) != nil {
				return ErrCorrupt
			}
			pending = &v
		case bytes.HasPrefix(key, []byte("ownership/outbox/")):
			n := binary.BigEndian.Uint32(key[len("ownership/outbox/"):])
			var v publicOutbox
			if n == 0 || n > m.AcceptedSequence || publicDecode(value, 256, &v) != nil || v.Version != 1 || v.AdmissionSequence != n || !publicHex(v.ProspectID, 32, true) {
				return ErrCorrupt
			}
			r, e := s.db.Get(recordKey(v.ProspectID), nil)
			if e != nil {
				return ErrCorrupt
			}
			record, e := s.indexedRecord(v.ProspectID, r)
			if e != nil || record.Sequence != uint64(n) {
				return ErrCorrupt
			}
			outboxes[n] = v
		case bytes.HasPrefix(key, []byte("ownership/admission/")):
			n := binary.BigEndian.Uint32(key[len("ownership/admission/"):])
			v, e := p.codec.ParsePublicAdmission(p.pin, value)
			if e != nil || n == 0 || n > m.SignedAdmissionSequence || v.AdmissionSequence != n {
				return ErrCorrupt
			}
			raw, e := s.db.Get(recordKey(v.Record.Verified.ProspectID), nil)
			want, _ := json.Marshal(v.Record)
			if e != nil || !bytes.Equal(raw, want) {
				return ErrCorrupt
			}
			admissions[n] = v
		case bytes.HasPrefix(key, []byte("ownership/publication/")):
			n := binary.BigEndian.Uint32(key[len("ownership/publication/"):])
			v, e := p.codec.ParsePublicPublication(p.pin, value)
			if e != nil || n == 0 || n > m.PublicationSequence || v.Sequence != n {
				return ErrCorrupt
			}
			publications[n] = v
		case bytes.HasPrefix(key, []byte("ownership/checkpoint/")):
			if len(value) != 4 {
				return ErrCorrupt
			}
			checkpointIndices[fmt.Sprintf("%x", key[len("ownership/checkpoint/"):])] = binary.BigEndian.Uint32(value)
		case bytes.HasPrefix(key, []byte("ownership/cutoff-intent/")):
			var v PublicCutoffIntent
			if publicDecode(value, MaxPublicSmallSize, &v) != nil || p.codec.ValidatePublicCutoffIntent(p.pin, v) != nil || !bytes.Equal(publicIDKey("ownership/cutoff-intent/", v.EventID), key) {
				return ErrCorrupt
			}
			intents[v.EventID] = v
		case bytes.HasPrefix(key, []byte("ownership/cutoff/")):
			var v PublicCutoffRecord
			if publicDecode(value, MaxPublicSmallSize, &v) != nil || p.codec.ValidatePublicCutoff(p.pin, v) != nil || !bytes.Equal(publicIDKey("ownership/cutoff/", v.EventID), key) {
				return ErrCorrupt
			}
			cutoffs[v.EventID] = v
		default:
			return ErrCorrupt
		}
	}
	if it.Error() != nil || !guardSeen || size != uint64(m.LogicalBytes) || len(outboxes) != int(m.AcceptedSequence) || len(admissions) != int(m.SignedAdmissionSequence) || len(publications) != int(m.PublicationSequence) || len(checkpointIndices) != int(m.CheckpointCount) || len(cutoffs) != int(m.CutoffCount) || len(intents) > len(cutoffs)+1 {
		return ErrCorrupt
	}
	previous := publicZero
	for n := uint32(1); n <= m.SignedAdmissionSequence; n++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		a, ok := admissions[n]
		if !ok || a.PreviousAdmissionID != previous || outboxes[n].ProspectID != a.Record.Verified.ProspectID {
			return ErrCorrupt
		}
		previous, e = p.codec.PublicAdmissionID(p.pin, a)
		if e != nil {
			return ErrCorrupt
		}
	}
	if previous != m.AdmissionHead {
		return ErrCorrupt
	}
	previous = publicZero
	chainPrevious := publicZero
	var clock int64
	seenAdmissions := map[uint32]bool{}
	seenCheckpoints := map[string]bool{}
	seenCutoffs := map[string]bool{}
	for n := uint32(1); n <= m.PublicationSequence; n++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		v, ok := publications[n]
		if !ok || v.PreviousID != previous || v.AttestedAtMS < clock || v.AttestedAtMS > m.ClockHighWaterMS {
			return ErrCorrupt
		}
		id, e := p.codec.PublicPublicationID(p.pin, v)
		if e != nil {
			return ErrCorrupt
		}
		previous = id
		clock = v.AttestedAtMS
		switch v.Kind {
		case "admission":
			var ref PublicAdmissionReference
			json.Unmarshal(v.Payload, &ref)
			a, ok := admissions[ref.AdmissionSequence]
			aid, _ := p.codec.PublicAdmissionID(p.pin, a)
			if !ok || aid != ref.AdmissionID || seenAdmissions[ref.AdmissionSequence] {
				return ErrCorrupt
			}
			seenAdmissions[ref.AdmissionSequence] = true
		case "checkpoint":
			var ref struct {
				Checkpoint chaincfg.SignedCheckpoint `json:"checkpoint"`
			}
			json.Unmarshal(v.Payload, &ref)
			raw, _ := json.Marshal(ref.Checkpoint)
			cid := publicHash("SLUG fighter ownership checkpoint", raw)
			if checkpointIndices[cid] != n || seenCheckpoints[cid] {
				return ErrCorrupt
			}
			seenCheckpoints[cid] = true
		case "chain":
			var ref PublicChainReference
			json.Unmarshal(v.Payload, &ref)
			if ref.PreviousChainEntryID != chainPrevious {
				return ErrCorrupt
			}
			chainPrevious = id
		case "cutoff":
			var ref PublicCutoffReference
			json.Unmarshal(v.Payload, &ref)
			if seenCutoffs[ref.CutoffID] {
				return ErrCorrupt
			}
			found := false
			for _, c := range cutoffs {
				cid, _ := PublicOwnershipDigest("SLUG fighter ownership cutoff id", c)
				if cid == ref.CutoffID {
					found = true
					break
				}
			}
			if !found {
				return ErrCorrupt
			}
			seenCutoffs[ref.CutoffID] = true
		}
	}
	if previous != m.PublicationHead || chainPrevious != m.ChainEntryID || len(seenAdmissions) != len(admissions) || len(seenCheckpoints) != len(checkpointIndices) {
		return ErrCorrupt
	}
	for event, c := range cutoffs {
		intent, ok := intents[event]
		if !ok || c.Boundary.Intent != intent {
			return ErrCorrupt
		}
	}
	if (pending == nil) != (m.PendingPublicationID == "") {
		return ErrCorrupt
	}
	if pending != nil {
		if e = s.validatePending(*pending); e != nil {
			return e
		}
		id, _ := PublicOwnershipDigest("SLUG fighter ownership publication intent", *pending)
		if id != m.PendingPublicationID {
			return ErrCorrupt
		}
	}
	return nil
}
