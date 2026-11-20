// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterownership

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"math"
	"strings"

	"github.com/taytr-llc/slug/registry"
)

func (s *Store) audit() error {
	raw, e := s.db.Get([]byte("meta"), nil)
	if e != nil || decode(raw, 8192, &s.meta) != nil {
		return ErrCorrupt
	}
	m := s.meta
	if m.OperatorRole != s.operatorRole {
		return ErrCorrupt
	}
	if m.Version != 1 || m.PinID != s.pinID || m.StoreID != s.identity.StoreID || m.ClockHighWaterMS < 0 || m.ClockHighWaterMS > MaxTimestampMS || !countsBound(m.Counts) || m.FinalitySequence > 4096 || m.PublishedFinalitySequence > m.FinalitySequence || !validPrefix(m.PublishedFinalitySequence, m.FinalityHead) || (m.WorkID != "" && !hexValue(m.WorkID, 32, true)) {
		return ErrCorrupt
	}
	s.publications = map[uint32]PublicationEntry{}
	s.headers = map[string]HeaderRow{}
	s.canonical = map[uint32]string{}
	s.witnesses = map[string]CarrierWitness{}
	s.frames = map[uint32]FinalityFrame{}
	s.frameBodies = map[uint32]FinalityFrameBody{}
	s.finalities = map[string]FinalityWitness{}
	s.boundaries = map[string]SignedBoundary{}
	s.conflicts = map[string]ConflictRecord{}
	var counts storeCounts
	var total uint64
	indices := map[string]bool{}
	it := s.db.NewIterator(nil, nil)
	for it.Next() {
		k, v := it.Key(), it.Value()
		key := string(k)
		total += uint64(len(k) + len(v))
		if total > MaxStoreBytes {
			it.Release()
			return ErrCapacity
		}
		s.keySizes[key] = len(v)
		countChange(&counts, key, 1)
		suffix := func(prefix string, n int) (string, bool) {
			return hex.EncodeToString(k[len(prefix):]), len(k) == len(prefix)+n
		}
		fail := false
		switch {
		case key == "meta":
		case key == "work":
			var x replayWork
			fail = decode(v, 16384, &x) != nil || validateWork(s.pinID, x) != nil || x.WorkID != m.WorkID
			if !fail {
				s.work = &x
			}
		case strings.HasPrefix(key, "a/") && len(k) == 6:
			n := binary.BigEndian.Uint32(k[2:])
			x, er := s.codec.ParsePublicAdmission(s.pin, v)
			fail = er != nil || n != x.AdmissionSequence || n == 0 || s.admissionByProspect[x.Record.Verified.ProspectID] != 0
			if !fail {
				s.admissions[n] = x
				s.admissionByProspect[x.Record.Verified.ProspectID] = n
			}
		case strings.HasPrefix(key, "p/") && len(k) == 6:
			n := binary.BigEndian.Uint32(k[2:])
			x, er := s.codec.ParsePublicPublication(s.pin, v)
			fail = er != nil || n != x.Sequence || n == 0
			if !fail {
				s.publications[n] = x
			}
		case strings.HasPrefix(key, "h/") && len(k) == 34:
			id, _ := suffix("h/", 32)
			var x HeaderRow
			fail = decode(v, 1024, &x) != nil || !validHeader(s.pinID, x) || x.Hash != id
			if !fail {
				s.headers[id] = x
			}
		case strings.HasPrefix(key, "c/") && len(k) == 6:
			n := binary.BigEndian.Uint32(k[2:])
			fail = n > math.MaxInt32 || len(v) != 32
			if !fail {
				s.canonical[n] = hex.EncodeToString(v)
			}
		case strings.HasPrefix(key, "w/") && len(k) == 34:
			id, _ := suffix("w/", 32)
			var x CarrierWitness
			fail = decode(v, 65536, &x) != nil || s.codec.validateWitness(s.pin, x) != nil
			got, er := objectDigest("SLUG fighter ownership witness", x)
			fail = fail || er != nil || got != id
			if !fail {
				s.witnesses[id] = x
			}
		case strings.HasPrefix(key, "t/") && len(k) == 66:
			fail = len(v) != 0
			if !fail {
				indices[key] = true
				s.transferCounts[key[2:34]]++
			}
		case strings.HasPrefix(key, "s/") && len(k) == 34:
			id, _ := suffix("s/", 32)
			x, er := s.codec.ParseEffectiveState(s.pin, v)
			fail = er != nil || x.StateID != id
			if !fail {
				s.states[id] = x
			}
		case strings.HasPrefix(key, "v/") && len(k) == 66:
			state := hex.EncodeToString(k[2:34])
			front := hex.EncodeToString(k[34:])
			var x FinalityWitness
			fail = decode(v, 4096, &x) != nil || s.codec.validateFinalityWitness(s.pin, x) != nil || x.StateID != state
			got, er := objectDigest("SLUG fighter ownership frontier evidence", x.Frontier)
			fail = fail || er != nil || got != front
			if !fail {
				s.finalities[key] = x
			}
		case strings.HasPrefix(key, "m/") && len(k) == 34:
			id, _ := suffix("m/", 32)
			x, er := s.codec.ParseOwnershipMap(s.pin, v)
			got, ge := s.codec.OwnershipMapID(s.pin, x)
			fail = er != nil || ge != nil || got != id
			if !fail {
				s.maps[id] = x
			}
		case strings.HasPrefix(key, "k/") && len(k) == 34:
			event, _ := suffix("k/", 32)
			x, er := s.codec.ParsePublicCutoff(s.pin, v)
			id, ie := s.codec.PublicCutoffID(s.pin, x)
			fail = er != nil || ie != nil || x.EventID != event
			if !fail {
				s.cutoffs[event] = x
				s.cutoffByID[id] = event
			}
		case strings.HasPrefix(key, "x/") && len(k) == 34:
			id, _ := suffix("x/", 32)
			var x ConflictRecord
			fail = decode(v, 8192, &x) != nil || s.codec.validateConflict(s.pin, x) != nil
			got, er := objectDigest("SLUG fighter ownership conflict", x)
			fail = fail || er != nil || got != id
			if !fail {
				s.conflicts[id] = x
			}
		case strings.HasPrefix(key, "f/") && len(k) == 6:
			n := binary.BigEndian.Uint32(k[2:])
			var x FinalityFrame
			fail = decode(v, 8192, &x) != nil || s.codec.ValidateFinalityFrame(s.pin, x) != nil || n != x.Sequence
			if !fail {
				s.frames[n] = x
			}
		case strings.HasPrefix(key, "fo/") && len(k) == 7:
			n := binary.BigEndian.Uint32(k[3:])
			var x FinalityFrameBody
			fail = decode(v, 8192, &x) != nil || s.codec.validateFrameBody(s.pin, x) != nil || n != x.Sequence
			if !fail {
				s.frameBodies[n] = x
			}
		case strings.HasPrefix(key, "b/") && len(k) == 34:
			event, _ := suffix("b/", 32)
			x, er := s.codec.ParsePublicBoundary(s.pin, v)
			fail = er != nil || x.Intent.EventID != event
			if !fail {
				s.boundaries[event] = x
			}
		default:
			fail = true
		}
		if fail {
			it.Release()
			return ErrCorrupt
		}
	}
	e = it.Error()
	it.Release()
	if e != nil {
		return e
	}
	counts.Carriers = uint32(len(s.transferCounts))
	if counts != m.Counts || total != uint64(m.LogicalBytes) || uint32(len(s.frames)) != m.PublishedFinalitySequence || uint32(len(s.frameBodies)) != m.FinalitySequence || (s.work == nil) != (m.WorkID == "") {
		return ErrCorrupt
	}
	ah := zeroID
	for n := uint32(1); n <= counts.Admissions; n++ {
		a, ok := s.admissions[n]
		if !ok || a.PreviousAdmissionID != ah {
			return ErrCorrupt
		}
		ah, e = s.codec.PublicAdmissionID(s.pin, a)
		if e != nil {
			return ErrCorrupt
		}
	}
	prefix := make([]PublicationEntry, 0, counts.Publications)
	for n := uint32(1); n <= counts.Publications; n++ {
		prefix = append(prefix, s.publications[n])
	}
	if s.codec.verifyPublicationPrefix(s.pin, prefix) != nil {
		return ErrCorrupt
	}
	frames := make([]FinalityFrame, 0, m.PublishedFinalitySequence)
	for n := uint32(1); n <= m.PublishedFinalitySequence; n++ {
		frames = append(frames, s.frames[n])
	}
	if s.codec.verifyFramePrefix(s.pin, frames) != nil {
		return ErrCorrupt
	}
	ph := zeroID
	chain := zeroID
	seenAdmissions := map[uint32]bool{}
	for n := uint32(1); n <= counts.Publications; n++ {
		p, ok := s.publications[n]
		if !ok || p.PreviousID != ph {
			return ErrCorrupt
		}
		ph, e = s.codec.PublicPublicationID(s.pin, p)
		if e != nil {
			return ErrCorrupt
		}
		switch p.Kind {
		case "admission":
			var ref registry.PublicAdmissionReference
			if json.Unmarshal(p.Payload, &ref) != nil || seenAdmissions[ref.AdmissionSequence] {
				return ErrCorrupt
			}
			a, ok := s.admissions[ref.AdmissionSequence]
			id, er := s.codec.PublicAdmissionID(s.pin, a)
			if !ok || er != nil || id != ref.AdmissionID {
				return ErrCorrupt
			}
			seenAdmissions[ref.AdmissionSequence] = true
		case "chain":
			var ref registry.PublicChainReference
			if json.Unmarshal(p.Payload, &ref) != nil || ref.PreviousChainEntryID != chain {
				return ErrCorrupt
			}
			chain = ph
		}
	}
	for n, h := range s.canonical {
		row, ok := s.headers[h]
		if !ok || row.Height != n {
			return ErrCorrupt
		}
		if n > 0 {
			prev, ok := s.canonical[n-1]
			if !ok {
				return ErrCorrupt
			}
			header, _ := hex.DecodeString(row.Header)
			reverse := append([]byte(nil), header[4:36]...)
			for i, j := 0, 31; i < j; i, j = i+1, j-1 {
				reverse[i], reverse[j] = reverse[j], reverse[i]
			}
			if hex.EncodeToString(reverse) != prev {
				return ErrCorrupt
			}
		} else if h != s.pin.ScanOrigin.Hash {
			return ErrCorrupt
		}
	}
	for id, w := range s.witnesses {
		k := string(append(idKey("t/", w.TransferID), idKey("", id)...))
		if !indices[k] {
			return ErrCorrupt
		}
		delete(indices, k)
		for _, tx := range []TransactionWitness{w.Commit, w.Spend} {
			h, ok := s.headers[tx.BlockHash]
			if !ok || h.Height != tx.Height {
				return ErrCorrupt
			}
			if checkTransactionWitness(context.Background(), tx, h) != nil {
				return ErrCorrupt
			}
		}
	}
	if len(indices) != 0 {
		return ErrCorrupt
	}
	for _, state := range s.states {
		a, ok := s.admissions[s.admissionByProspect[state.ProspectID]]
		aid, er := s.codec.PublicAdmissionID(s.pin, a)
		if !ok || er != nil || aid != state.BirthAdmissionID {
			return ErrCorrupt
		}
		if state.Generation == 0 {
			expected, er := s.codec.InitialState(s.pin, a)
			if er != nil || expected.StateID != state.StateID {
				return ErrCorrupt
			}
		} else {
			var ev FinalStateEvidence
			json.Unmarshal(state.Evidence, &ev)
			prior, ok := s.states[ev.PriorStateID]
			if !ok || prior.ProspectID != state.ProspectID || prior.Generation+1 != state.Generation {
				return ErrCorrupt
			}
			found := false
			for _, w := range s.witnesses {
				if w.TransferID == ev.TransferID && w.PriorStateID == ev.PriorStateID && w.Kind == "reveal" {
					found = true
					break
				}
			}
			if !found {
				return ErrCorrupt
			}
		}
	}
	for _, mp := range s.maps {
		for _, item := range mp.Items {
			state, ok := s.states[item.StateID]
			a, admitted := s.admissions[item.AdmissionSequence]
			if !ok || !admitted || state.ProspectID != item.ProspectID || a.Record.Verified.ProspectID != item.ProspectID {
				return ErrCorrupt
			}
		}
	}
	previous := zeroID
	frameHead := zeroID
	for n := uint32(1); n <= m.FinalitySequence; n++ {
		body, ok := s.frameBodies[n]
		if !ok || body.PreviousBodyID != previous {
			return ErrCorrupt
		}
		previous, e = objectDigest("SLUG fighter ownership finality body", body)
		if e != nil {
			return ErrCorrupt
		}
		mp, ok := s.maps[body.OwnershipMapID]
		fp, er := s.codec.FinalizedPrefixID(s.pin, mp)
		if !ok || er != nil || fp != body.FinalizedPrefixID || len(mp.Items) != int(body.AdmissionSequence) {
			return ErrCorrupt
		}
		if n <= m.PublishedFinalitySequence {
			frame, ok := s.frames[n]
			if !ok || frame.FinalityFrameBody != body {
				return ErrCorrupt
			}
			frameHead, e = s.codec.FinalityFrameID(s.pin, frame)
			if e != nil {
				return ErrCorrupt
			}
		}
	}
	if frameHead != m.FinalityHead {
		return ErrCorrupt
	}
	for _, v := range s.finalities {
		state, ok := s.states[v.StateID]
		if !ok || state.Generation == 0 {
			return ErrCorrupt
		}
		var ev FinalStateEvidence
		json.Unmarshal(state.Evidence, &ev)
		if v.CheckpointPublicationID != ev.CheckpointPublicationID || v.SixtiethDescendantHeight != ev.SixtiethDescendantHeight || v.SixtiethDescendantHash != ev.SixtiethDescendantHash {
			return ErrCorrupt
		}
		if !s.frontierExists(v.Frontier) {
			return ErrCorrupt
		}
	}
	for event, c := range s.cutoffs {
		b, ok := s.boundaries[event]
		if !ok || b != c.Boundary {
			return ErrCorrupt
		}
		mp, ok := s.maps[c.OwnershipDigest]
		if !ok || uint32(len(mp.Items)) != c.AdmissionSequence {
			return ErrCorrupt
		}
		if e = s.validateHeadMap(c.Head, mp); e != nil {
			return e
		}
	}
	if m.CurrentHead == nil {
		if m.CurrentMapID != "" {
			return ErrCorrupt
		}
	} else {
		mp, ok := s.maps[m.CurrentMapID]
		if !ok || s.validateHeadMap(*m.CurrentHead, mp) != nil {
			return ErrCorrupt
		}
		s.head = *m.CurrentHead
	}
	return nil
}
func (s *Store) frontierExists(f FrontierEvidence) bool {
	switch f.Kind {
	case "finality_frame":
		for _, v := range s.frames {
			id, _ := s.codec.FinalityFrameID(s.pin, v)
			if id == f.ID {
				return true
			}
		}
	case "boundary":
		for _, v := range s.boundaries {
			id, _ := s.codec.PublicBoundaryID(s.pin, v)
			if id == f.ID {
				return true
			}
		}
	case "publication":
		for _, v := range s.publications {
			if v.Kind == "chain" {
				id, _ := s.codec.PublicPublicationID(s.pin, v)
				if id == f.ID {
					return true
				}
			}
		}
	}
	return false
}
func (s *Store) validateHeadMap(h HeadRef, m OwnershipMap) error {
	if s.codec.ValidateHead(s.pin, h) != nil || len(m.Items) != int(h.AdmissionSequence) {
		return ErrCorrupt
	}
	mid, e := s.codec.OwnershipMapID(s.pin, m)
	if e != nil {
		return e
	}
	fp, e := s.codec.FinalizedPrefixID(s.pin, m)
	if e != nil || fp != h.FinalizedPrefixID {
		return ErrCorrupt
	}
	d, e := replayDigest(h, mid)
	if e != nil || d != h.ReplayDigest {
		return ErrCorrupt
	}
	if h.AdmissionSequence > 0 {
		a, ok := s.admissions[h.AdmissionSequence]
		id, e := s.codec.PublicAdmissionID(s.pin, a)
		if !ok || e != nil || id != h.AdmissionHead {
			return ErrCorrupt
		}
	}
	if h.PublicationSequence > 0 {
		p, ok := s.publications[h.PublicationSequence]
		id, e := s.codec.PublicPublicationID(s.pin, p)
		if !ok || e != nil || id != h.PublicationHead {
			return ErrCorrupt
		}
	}
	if h.ConflictID != "" {
		if _, ok := s.conflicts[h.ConflictID]; !ok {
			return ErrCorrupt
		}
	}
	return nil
}
func replayDigest(h HeadRef, mapID string) (string, error) {
	return objectDigest("SLUG fighter ownership replay", struct {
		PinID               string `json:"pin_id"`
		AdmissionSequence   uint32 `json:"admission_sequence"`
		AdmissionHead       string `json:"admission_head"`
		PublicationSequence uint32 `json:"publication_sequence"`
		PublicationHead     string `json:"publication_head"`
		ChainHeight         uint32 `json:"chain_height"`
		ChainHash           string `json:"chain_hash"`
		OwnershipMapID      string `json:"ownership_map_id"`
		FinalizedPrefixID   string `json:"finalized_prefix_id"`
		ConflictID          string `json:"conflict_id"`
	}{h.PinID, h.AdmissionSequence, h.AdmissionHead, h.PublicationSequence, h.PublicationHead, h.ChainHeight, h.ChainHash, mapID, h.FinalizedPrefixID, h.ConflictID})
}
