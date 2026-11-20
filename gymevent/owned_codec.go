// Copyright (c) 2026 The SLUG developers. ISC license.
package gymevent

import (
	"encoding/json"
	"github.com/taytr-llc/slug/fightercondition"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/fightermanifest"
	"github.com/taytr-llc/slug/fighterownership"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/internal/ownedcodec"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/registry"
)

func OwnedRulesID() string {
	v := struct {
		Version          uint32 `json:"version"`
		Boundary         string `json:"boundary"`
		ConditionTime    string `json:"condition_time"`
		OwnershipBasis   string `json:"ownership_basis"`
		ConditionVersion uint32 `json:"condition_version"`
		ProjectorID      string `json:"projector_id"`
		XP               uint32 `json:"xp"`
		Training         uint32 `json:"training"`
		Participants     uint32 `json:"participants"`
		MembersPerGym    uint32 `json:"members_per_gym"`
		ExclusionOrder   string `json:"exclusion_order"`
		LaterChanges     string `json:"later_changes"`
		SeedAuthority    string `json:"seed_authority"`
	}{1, "committed_before_cutoff", "cutoff", "finalized_public_chain", 1, fightercondition.ProjectorID, 0, 0, 2, 5, "retired_then_not_rostered", "immutable", "unavailable"}
	id, _ := ownedcodec.Hash("event rules", v)
	return id
}
func ValidateOwnedPin(p OwnedPin) (OwnedPin, error) {
	if p.Version != 1 || p.CalendarID != gymentry.CalendarID() || p.RulesID != OwnedRulesID() || len(p.Participants) != 2 {
		return OwnedPin{}, ErrInvalid
	}
	if _, e := fighterownership.ValidatePin(p.Ownership); e != nil {
		return OwnedPin{}, ErrInvalid
	}
	for _, ep := range p.Participants {
		if _, e := gymentry.ValidateOwnedPin(ep); e != nil || ep.StablePin.Ownership != p.Ownership {
			return OwnedPin{}, ErrInvalid
		}
	}
	a, b := p.Participants[0], p.Participants[1]
	x, y := a.StablePin.Stable, b.StablePin.Stable
	if x.Identity.GymID >= y.Identity.GymID || x.AccountID == y.AccountID || a.StableStoreID == b.StableStoreID || x.AccountAuthorityID != y.AccountAuthorityID || a.NamespaceID != b.NamespaceID || a.CalendarID != b.CalendarID || a.RulesID != b.RulesID || !ownedcodec.Fits(p, OwnedMaxPinSize) {
		return OwnedPin{}, ErrInvalid
	}
	p.Participants = append([]gymentry.OwnedPin{}, p.Participants...)
	return p, nil
}
func OwnedID(p OwnedPin) (string, error) {
	if _, e := ValidateOwnedPin(p); e != nil {
		return "", e
	}
	return ownedcodec.Hash("event store", p)
}
func OwnedEventForDate(p OwnedPin, date string) (Event, error) {
	if _, e := ValidateOwnedPin(p); e != nil {
		return Event{}, e
	}
	source, e := gymentry.OwnedEventForDate(p.Participants[0], date)
	if e != nil {
		return Event{}, e
	}
	deadline, e := gymentry.IncidentDeadline(source)
	if e != nil {
		return Event{}, e
	}
	sid, _ := OwnedID(p)
	id, _ := ownedcodec.Hash("event", struct {
		Version       uint32 `json:"version"`
		StoreID       string `json:"store_id"`
		SourceEventID string `json:"source_event_id"`
	}{1, sid, source.EventID})
	return Event{1, id, source.EventID, sid, source.NamespaceID, p.CalendarID, p.RulesID, date, source.CutoffMS, deadline}, nil
}
func ValidateOwnedEvent(p OwnedPin, event Event) (Event, error) {
	want, e := OwnedEventForDate(p, event.Saturday)
	if e != nil || event != want {
		return Event{}, ErrInvalid
	}
	return want, nil
}
func OwnedParticipantID(p OwnedPin, ep gymentry.OwnedPin) (string, error) {
	sid, e := OwnedID(p)
	if e != nil {
		return "", e
	}
	found := false
	for _, x := range p.Participants {
		if x == ep {
			found = true
		}
	}
	if !found {
		return "", ErrInvalid
	}
	eid, _ := gymentry.OwnedID(ep)
	return ownedcodec.Hash("event participant", struct {
		Version      uint32 `json:"version"`
		EventStoreID string `json:"event_store_id"`
		EntryStoreID string `json:"entry_store_id"`
		GymID        string `json:"gym_id"`
	}{1, sid, eid, ep.StablePin.Stable.Identity.GymID})
}
func ownedParticipant(p OwnedPin, id string) (gymentry.OwnedPin, error) {
	for _, ep := range p.Participants {
		want, _ := OwnedParticipantID(p, ep)
		if id == want {
			return ep, nil
		}
	}
	return gymentry.OwnedPin{}, ErrInvalid
}
func MarshalOwnedPin(p OwnedPin) ([]byte, error) {
	p, e := ValidateOwnedPin(p)
	if e != nil {
		return nil, e
	}
	return json.Marshal(p)
}
func ParseOwnedPin(raw []byte) (OwnedPin, error) {
	var p OwnedPin
	if ownedcodec.Decode(raw, OwnedMaxPinSize, &p) != nil {
		return p, ErrInvalid
	}
	return ValidateOwnedPin(p)
}
func ownedRoster(v gymstable.CutoffRoster, ep gymentry.OwnedPin, cutoff int64) error {
	if v.Version != 1 || v.StoreID != ep.StableStoreID || v.CutoffMS != cutoff || v.CommittedCount > 4096 || v.LastSequence > 4096 || v.LastSequence < uint64(v.CommittedCount) || v.Revision > uint64(v.CommittedCount) || !hashID(v.HistoryDigest) || (v.CommittedCount == 0) != (v.LastSequence == 0) {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for n, id := range v.Slots {
		if !optionalID(id) {
			return ErrInvalid
		}
		if id == "" {
			if v.AssignedAtMS[n] != 0 {
				return ErrInvalid
			}
			continue
		}
		if v.Revision == 0 || seen[id] || !timestamp(v.AssignedAtMS[n]) || v.AssignedAtMS[n] >= cutoff {
			return ErrInvalid
		}
		seen[id] = true
	}
	if v.CommittedCount == 0 {
		want, _ := ownedcodec.Hash("stable cutoff history", struct {
			Version  uint32 `json:"version"`
			StoreID  string `json:"store_id"`
			EventID  string `json:"event_id"`
			CutoffMS int64  `json:"cutoff_ms"`
		}{1, v.StoreID, "", cutoff})
		if v.HistoryDigest != want {
			return ErrInvalid
		}
	}
	return nil
}
func ownedEntry(v gymentry.OwnedCutoffEntry, ep gymentry.OwnedPin, ev Event) error {
	sid, _ := gymentry.OwnedID(ep)
	source, e := gymentry.OwnedEventForDate(ep, ev.Saturday)
	eid, _ := gymentry.OwnedEntryID(sid, source.EventID, ep.StablePin.Stable.Identity.GymID)
	if e != nil || v.Version != 1 || v.StoreID != sid || v.Event != source || v.EntryID != eid || v.CommittedCount > 4096 || v.LastSequence > 4096 || v.LastSequence < uint64(v.CommittedCount) || !hashID(v.HistoryDigest) || (v.CommittedCount == 0) != (v.LastSequence == 0) || (v.CommittedCount == 0) != (v.LatestReceipt == nil) {
		return ErrInvalid
	}
	if v.LatestReceipt == nil {
		want, _ := ownedcodec.Hash("entry cutoff history", struct {
			Version  uint32 `json:"version"`
			StoreID  string `json:"store_id"`
			EventID  string `json:"event_id"`
			CutoffMS int64  `json:"cutoff_ms"`
		}{1, sid, source.EventID, source.CutoffMS})
		if want != v.HistoryDigest {
			return ErrInvalid
		}
	} else {
		r, e := gymentry.ValidateOwnedReceipt(ep, *v.LatestReceipt)
		if e != nil || r.Event != source || r.OperationSequence != v.LastSequence || r.Revision != uint64(v.CommittedCount) {
			return ErrInvalid
		}
	}
	if !ownedcodec.Fits(v, 52*1024) {
		return ErrInvalid
	}
	return nil
}
func ValidateOwnedParticipantSnapshot(p OwnedPin, v OwnedParticipantSnapshot) (OwnedParticipantSnapshot, error) {
	ep, e := ownedParticipant(p, v.ParticipantID)
	if e != nil {
		return v, e
	}
	ev, e := OwnedEventForDate(p, v.Entry.Event.Saturday)
	sid, _ := gymentry.OwnedID(ep)
	sp := ep.StablePin.Stable
	owner := AssignmentOwner{"finalized_public_chain", sp.AccountAuthorityID, sp.AccountID, sp.Identity.GymID}
	if e != nil || v.Version != 1 || v.EntryStoreID != sid || v.Owner != owner || v.Members == nil || v.Excluded == nil || len(v.Members)+len(v.Excluded) > 5 || ownedRoster(v.Roster, ep, ev.CutoffMS) != nil || ownedEntry(v.Entry, ep, ev) != nil {
		return v, ErrInvalid
	}
	saved := map[string]gymentry.OwnedMember{}
	if v.Entry.LatestReceipt != nil {
		for _, m := range v.Entry.LatestReceipt.Members {
			saved[m.Birth.ProspectID] = m
		}
	}
	if len(v.Members)+len(v.Excluded) != len(saved) {
		return v, ErrInvalid
	}
	seen := map[string]bool{}
	d, e := fighterdescriptor.ResolveCandidate(sp.Source.DescriptorID)
	if e != nil {
		return v, ErrInvalid
	}
	for n, m := range v.Members {
		a := m.Birth
		old, ok := saved[a.ProspectID]
		if !ok || a != old.Birth || seen[a.ProspectID] || n > 0 && v.Members[n-1].Birth.ProspectID >= a.ProspectID || fighterownership.ValidateEffectiveState(p.Ownership, m.Ownership) != nil || m.Ownership.ProspectID != a.ProspectID || m.Ownership.OwnerPublicKey != sp.Identity.GymPublicKey || a.AcceptedAtMS >= ev.CutoffMS {
			return v, ErrInvalid
		}
		seen[a.ProspectID] = true
		manifest, e := fightermanifest.GenerateText(d.ManifestVersion, a.WorkHash)
		if e != nil {
			return v, ErrInvalid
		}
		md, e := fightermanifest.Digest(manifest)
		if e != nil || md != a.ManifestDigest || manifest.Physical.Class != m.Class {
			return v, ErrInvalid
		}
		condition, e := fightercondition.Project(1, manifest.DNA, a.BornAtMS, ev.CutoffMS, 0)
		if e != nil || condition != m.ConditionAtCutoff {
			return v, ErrInvalid
		}
		var slot uint32
		var assigned int64
		for i, id := range v.Roster.Slots {
			if id == a.ProspectID {
				slot = uint32(i + 1)
				assigned = v.Roster.AssignedAtMS[i]
			}
		}
		if m.Slot != slot || m.SlotAssignedAtMS != assigned || slot > 0 && assigned < a.AcceptedAtMS {
			return v, ErrInvalid
		}
		eligibility := "eligible"
		if slot == 0 {
			eligibility = "not_rostered"
		}
		if condition.Phase == "retired" {
			eligibility = "retired"
		}
		if m.Eligibility != eligibility || !ownedcodec.Fits(m, 6*1024) {
			return v, ErrInvalid
		}
	}
	for n, x := range v.Excluded {
		_, ok := saved[x.ProspectID]
		if !ok || seen[x.ProspectID] || !hashID(x.StateID) || n > 0 && v.Excluded[n-1].ProspectID >= x.ProspectID || x.Reason != "not_owned_at_cutoff" {
			return v, ErrInvalid
		}
		gym, e := gymauth.GymID(prospect.Context{Version: prospect.Version1, Network: p.Ownership.Source.Network, Genesis: p.Ownership.Source.Genesis}, x.OwnerPublicKey)
		if e != nil || gym == sp.Identity.GymID {
			return v, ErrInvalid
		}
		seen[x.ProspectID] = true
	}
	if !ownedcodec.Fits(v, OwnedMaxParticipantSize) {
		return v, ErrInvalid
	}
	raw, _ := json.Marshal(v)
	var out OwnedParticipantSnapshot
	if json.Unmarshal(raw, &out) != nil {
		return v, ErrInvalid
	}
	return out, nil
}
func ValidateOwnedSnapshotInput(p OwnedPin, v OwnedSnapshotInput) (OwnedSnapshotInput, error) {
	p, e := ValidateOwnedPin(p)
	if e != nil {
		return v, e
	}
	ev, e := OwnedEventForDate(p, v.Event.Saturday)
	if e != nil || v.Version != 1 || v.Event != ev || len(v.Participants) != 2 || registry.ValidatePublicCutoff(p.Ownership, v.Cutoff) != nil || v.Cutoff.EventID != ev.EventID || v.Cutoff.CutoffMS != ev.CutoffMS {
		return v, ErrInvalid
	}
	v.Participants = append([]OwnedParticipantSnapshot{}, v.Participants...)
	seen := map[string]bool{}
	for n, x := range v.Participants {
		id, _ := OwnedParticipantID(p, p.Participants[n])
		if x.ParticipantID != id || x.Entry.Event.EventID != ev.SourceEventID {
			return v, ErrInvalid
		}
		x, e = ValidateOwnedParticipantSnapshot(p, x)
		if e != nil {
			return v, e
		}
		for _, m := range x.Members {
			if seen[m.Birth.ProspectID] {
				return v, ErrInvalid
			}
			seen[m.Birth.ProspectID] = true
		}
		v.Participants[n] = x
	}
	if !ownedcodec.Fits(v, OwnedMaxInputSize) {
		return v, ErrInvalid
	}
	return v, nil
}
func OwnedSnapshotDigest(p OwnedPin, v OwnedSnapshotInput) (string, error) {
	v, e := ValidateOwnedSnapshotInput(p, v)
	if e != nil {
		return "", e
	}
	return ownedcodec.Hash("event snapshot", v)
}
func MarshalOwnedSnapshotInput(p OwnedPin, v OwnedSnapshotInput) ([]byte, error) {
	v, e := ValidateOwnedSnapshotInput(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseOwnedSnapshotInput(p OwnedPin, raw []byte) (OwnedSnapshotInput, error) {
	var v OwnedSnapshotInput
	if ownedcodec.Decode(raw, OwnedMaxInputSize, &v) != nil {
		return v, ErrInvalid
	}
	return ValidateOwnedSnapshotInput(p, v)
}
func ownedEvidence(p OwnedPin, v OwnedCaptureEvidence, s OwnedParticipantSnapshot, cutoffID string) error {
	ep, e := ownedParticipant(p, s.ParticipantID)
	c := s.Entry.Event.CutoffMS
	if e != nil || v.Version != 1 || v.ParticipantID != s.ParticipantID || v.CutoffID != cutoffID || !validFence(v.StableFence, ep.StableStoreID, c, s.Roster.LastSequence) || !validFence(v.EntryFence, s.EntryStoreID, c, s.Entry.LastSequence) || v.EntryFence.ObservedAtMS < v.StableFence.ObservedAtMS || v.CapturedAtMS < v.EntryFence.ObservedAtMS || !timestamp(v.CapturedAtMS) {
		return ErrInvalid
	}
	return nil
}
func ValidateOwnedSnapshotReceipt(p OwnedPin, v OwnedSnapshotReceipt) (OwnedSnapshotReceipt, error) {
	input, e := ValidateOwnedSnapshotInput(p, v.Input)
	if e != nil {
		return v, e
	}
	digest, _ := OwnedSnapshotDigest(p, input)
	cutoffID, _ := registry.PublicCutoffID(p.Ownership, input.Cutoff)
	if v.Version != 1 || v.SnapshotDigest != digest || len(v.Captures) != 2 || !timestamp(v.CompletedAtMS) {
		return v, ErrInvalid
	}
	for n, c := range v.Captures {
		if ownedEvidence(p, c, input.Participants[n], cutoffID) != nil || c.CapturedAtMS > v.CompletedAtMS {
			return v, ErrInvalid
		}
	}
	if !ownedcodec.Fits(v, OwnedMaxReceiptSize) {
		return v, ErrInvalid
	}
	v.Input = input
	v.Captures = append([]OwnedCaptureEvidence{}, v.Captures...)
	return v, nil
}
func MarshalOwnedSnapshotReceipt(p OwnedPin, v OwnedSnapshotReceipt) ([]byte, error) {
	v, e := ValidateOwnedSnapshotReceipt(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseOwnedSnapshotReceipt(p OwnedPin, raw []byte) (OwnedSnapshotReceipt, error) {
	var v OwnedSnapshotReceipt
	if ownedcodec.Decode(raw, OwnedMaxReceiptSize, &v) != nil {
		return v, ErrInvalid
	}
	return ValidateOwnedSnapshotReceipt(p, v)
}
func OwnedReceiptDigest(p OwnedPin, v OwnedSnapshotReceipt) (string, error) {
	v, e := ValidateOwnedSnapshotReceipt(p, v)
	if e != nil {
		return "", e
	}
	return ownedcodec.Hash("event receipt", v)
}
