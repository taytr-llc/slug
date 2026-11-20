// Copyright (c) 2026 The SLUG developers. ISC license.
package gymevent

import (
	"encoding/hex"
	"encoding/json"
	"github.com/taytr-llc/slug/fightercondition"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/fightermanifest"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymstable"
	"strings"
)

func hashID(s string) bool {
	if len(s) != 64 || strings.ToLower(s) != s {
		return false
	}
	_, e := hex.DecodeString(s)
	return e == nil
}
func optionalID(s string) bool { return s == "" || hashID(s) }
func timestamp(n int64) bool   { return n > 0 && n <= MaxTimestampMS }
func fits(v any, n int) bool   { b, e := json.Marshal(v); return e == nil && len(b) <= n }
func validatePin(p Pin) error {
	if p.Version != 1 || p.CalendarID != gymentry.CalendarID() || p.RulesID != RulesID() || len(p.Participants) != 2 {
		return ErrInvalid
	}
	a, b := p.Participants[0], p.Participants[1]
	if _, e := gymentry.ValidatePin(a); e != nil {
		return ErrInvalid
	}
	if _, e := gymentry.ValidatePin(b); e != nil {
		return ErrInvalid
	}
	x, y := a.StablePin, b.StablePin
	f0, _ := gymaccount.FixtureIdentity("zero256")
	f1, _ := gymaccount.FixtureIdentity("incrementing256")
	if !((x.Identity == f0 && y.Identity == f1) || (x.Identity == f1 && y.Identity == f0)) || x.Identity.GymID >= y.Identity.GymID || x.AccountID == y.AccountID || a.StableStoreID == b.StableStoreID || x.AccountAuthorityID != y.AccountAuthorityID || a.NamespaceID != b.NamespaceID || a.CalendarID != b.CalendarID || a.RulesID != b.RulesID || x.Source.DescriptorID != y.Source.DescriptorID {
		return ErrInvalid
	}
	return nil
}
func participant(p Pin, id string) (gymentry.Pin, error) {
	for _, x := range p.Participants {
		if x.StablePin.Identity.GymID == id {
			return x, nil
		}
	}
	return gymentry.Pin{}, ErrInvalid
}
func validateInfo(p Pin, v Info) error {
	id, _ := ID(p)
	if v.Version != 1 || v.StoreID != id || !equal(p, v.Pin) || !timestamp(v.CreatedAtMS) || v.EventCount > MaxEvents {
		return ErrInvalid
	}
	return nil
}
func validateEvent(p Pin, v Event) error {
	want, e := EventForDate(p, v.Saturday)
	if e != nil || v != want {
		return ErrInvalid
	}
	return nil
}
func validateRoster(v gymstable.CutoffRoster, p gymentry.Pin, c int64) error {
	if v.Version != 1 || v.StoreID != p.StableStoreID || v.CutoffMS != c || v.CommittedCount > gymstable.MaxOperations || v.LastSequence > gymstable.MaxOperations || v.LastSequence < uint64(v.CommittedCount) || v.Revision > uint64(v.CommittedCount) || !hashID(v.HistoryDigest) || (v.CommittedCount == 0) != (v.LastSequence == 0) {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for i, id := range v.Slots {
		if !optionalID(id) {
			return ErrInvalid
		}
		if id == "" {
			if v.AssignedAtMS[i] != 0 {
				return ErrInvalid
			}
			continue
		}
		if v.Revision == 0 || seen[id] || !timestamp(v.AssignedAtMS[i]) || v.AssignedAtMS[i] >= c {
			return ErrInvalid
		}
		seen[id] = true
	}
	if v.CommittedCount == 0 && v.HistoryDigest != hash("SLUG stable cutoff history\x00v1\x00", rawID(v.StoreID), le64(uint64(c))) {
		return ErrInvalid
	}
	if !fits(v, MaxRosterSize) {
		return ErrInvalid
	}
	return nil
}
func validateEntry(v gymentry.CutoffEntry, p gymentry.Pin, ev Event) error {
	sid, _ := gymentry.ID(p)
	src, e := gymentry.EventForDate(p, ev.Saturday)
	eid, _ := gymentry.EntryID(sid, src.EventID, p.StablePin.Identity.GymID)
	if e != nil || v.Version != 1 || v.StoreID != sid || v.Event != src || v.EntryID != eid || v.CommittedCount > gymentry.MaxOperations || v.LastSequence > gymentry.MaxOperations || v.LastSequence < uint64(v.CommittedCount) || !hashID(v.HistoryDigest) || (v.CommittedCount == 0) != (v.LastSequence == 0) || (v.CommittedCount == 0) != (v.LatestReceipt == nil) {
		return ErrInvalid
	}
	if v.LatestReceipt == nil {
		if v.HistoryDigest != hash("SLUG entry cutoff history\x00v1\x00", rawID(sid), rawID(src.EventID), le64(uint64(src.CutoffMS))) {
			return ErrInvalid
		}
	} else {
		r, e := gymentry.ValidateReceipt(*v.LatestReceipt)
		if e != nil || r.StoreID != sid || r.Event != src || r.EntryID != eid || r.OperationSequence != v.LastSequence || r.Revision != uint64(v.CommittedCount) {
			return ErrInvalid
		}
		for _, m := range r.Members {
			if m.Assignment.GymID != p.StablePin.Identity.GymID || m.Assignment.DescriptorID != p.StablePin.Source.DescriptorID {
				return ErrInvalid
			}
		}
	}
	if !fits(v, MaxEntrySize) {
		return ErrInvalid
	}
	return nil
}
func validateParticipantSnapshot(p Pin, v ParticipantSnapshot) error {
	ep, e := participant(p, v.ParticipantID)
	if e != nil {
		return e
	}
	ev, e := EventForDate(p, v.Entry.Event.Saturday)
	if e != nil {
		return e
	}
	sid, _ := gymentry.ID(ep)
	owner := AssignmentOwner{"candidate_assignment", ep.StablePin.AccountAuthorityID, ep.StablePin.AccountID, ep.StablePin.Identity.GymID}
	if v.Version != 1 || v.EntryStoreID != sid || v.Owner != owner || v.Members == nil || len(v.Members) > 5 || validateRoster(v.Roster, ep, ev.CutoffMS) != nil || validateEntry(v.Entry, ep, ev) != nil {
		return ErrInvalid
	}
	var saved []gymentry.Member
	if v.Entry.LatestReceipt != nil {
		saved = v.Entry.LatestReceipt.Members
	}
	if len(v.Members) != len(saved) {
		return ErrInvalid
	}
	d, e := fighterdescriptor.ResolveCandidate(ep.StablePin.Source.DescriptorID)
	if e != nil {
		return ErrInvalid
	}
	for i, m := range v.Members {
		a := m.Assignment
		if a != saved[i].Assignment || m.Slot > 10 || !timestamp(a.BornAtMS) || !timestamp(a.AcceptedAtMS) || a.BornAtMS > a.AcceptedAtMS || a.AcceptedAtMS >= ev.CutoffMS || a.AcceptedAtMS > v.Entry.LatestReceipt.CommittedAtMS {
			return ErrInvalid
		}
		manifest, e := fightermanifest.GenerateText(d.ManifestVersion, a.WorkHash)
		if e != nil {
			return ErrInvalid
		}
		md, e := fightermanifest.Digest(manifest)
		if e != nil || md != a.ManifestDigest || manifest.DNA.DNAVersion != d.DNAVersion || manifest.Physical.Class != m.Class {
			return ErrInvalid
		}
		c, e := fightercondition.Project(1, manifest.DNA, a.BornAtMS, ev.CutoffMS, 0)
		if e != nil || c != m.ConditionAtCutoff {
			return ErrInvalid
		}
		var slot uint32
		var assigned int64
		for n, id := range v.Roster.Slots {
			if id == a.ProspectID {
				slot = uint32(n + 1)
				assigned = v.Roster.AssignedAtMS[n]
			}
		}
		if m.Slot != slot || m.SlotAssignedAtMS != assigned || (slot != 0 && a.AcceptedAtMS > assigned) {
			return ErrInvalid
		}
		eligibility := "eligible"
		if slot == 0 {
			eligibility = "not_rostered"
		}
		if c.Phase == "retired" {
			eligibility = "retired"
		}
		if m.Eligibility != eligibility || !fits(m, MaxMemberSize) {
			return ErrInvalid
		}
	}
	return nil
}
func validFence(f gymstable.CutoffFence, id string, c int64, last uint64) bool {
	return f.Version == 1 && f.StoreID == id && f.CutoffMS == c && timestamp(f.ObservedAtMS) && f.ObservedAtMS >= c && f.OperationCount <= 4096 && uint64(f.OperationCount) >= last
}
func validateEvidence(v CaptureEvidence, pin Pin, s ParticipantSnapshot) error {
	ep, e := participant(pin, s.ParticipantID)
	if e != nil {
		return e
	}
	c := s.Entry.Event.CutoffMS
	if v.Version != 1 || v.ParticipantID != s.ParticipantID || !validFence(v.StableFence, ep.StableStoreID, c, s.Roster.LastSequence) || !validFence(v.EntryFence, s.EntryStoreID, c, s.Entry.LastSequence) || v.EntryFence.ObservedAtMS < v.StableFence.ObservedAtMS || !timestamp(v.CapturedAtMS) || v.CapturedAtMS < v.EntryFence.ObservedAtMS || !fits(v, MaxEvidenceSize) {
		return ErrInvalid
	}
	return nil
}
func validateParticipantCapture(p Pin, v ParticipantCapture) error {
	if v.Version != 1 || validateParticipantSnapshot(p, v.Snapshot) != nil {
		return ErrInvalid
	}
	return validateEvidence(v.Evidence, p, v.Snapshot)
}
func validateSnapshotInput(p Pin, v SnapshotInput) error {
	if v.Version != 1 || validateEvent(p, v.Event) != nil || len(v.Participants) != 2 {
		return ErrInvalid
	}
	for i, x := range v.Participants {
		if x.ParticipantID != p.Participants[i].StablePin.Identity.GymID || x.Entry.Event.EventID != v.Event.SourceEventID || validateParticipantSnapshot(p, x) != nil {
			return ErrInvalid
		}
	}
	return nil
}
func validateSnapshotReceipt(p Pin, v SnapshotReceipt) error {
	if v.Version != 1 || validateSnapshotInput(p, v.Input) != nil || len(v.Captures) != 2 || !timestamp(v.CompletedAtMS) {
		return ErrInvalid
	}
	b, _ := json.Marshal(v.Input)
	want := hash("SLUG candidate cutoff input\x00v1\x00", lp(b))
	if v.SnapshotDigest != want {
		return ErrInvalid
	}
	for i, x := range v.Captures {
		if validateEvidence(x, p, v.Input.Participants[i]) != nil || x.CapturedAtMS > v.CompletedAtMS {
			return ErrInvalid
		}
	}
	return nil
}
func validateEventStatus(p Pin, v EventStatus) error {
	id, _ := ID(p)
	if v.Version != 1 || v.StoreID != id || validateEvent(p, v.Event) != nil || v.CapturedParticipants == nil || len(v.CapturedParticipants) > 2 || !optionalID(v.SnapshotDigest) || !timestamp(v.CoordinatorCreatedAtMS) || !timestamp(v.ObservedAtMS) || v.ObservedAtMS < v.CoordinatorCreatedAtMS || v.SeedAuthority != "unavailable" {
		return ErrInvalid
	}
	last := ""
	for _, x := range v.CapturedParticipants {
		if x <= last {
			return ErrInvalid
		}
		if _, e := participant(p, x); e != nil {
			return e
		}
		last = x
	}
	if !v.Exists {
		phase := "before_cutoff"
		if v.ObservedAtMS >= v.Event.CutoffMS {
			phase = "awaiting_capture"
		}
		if v.Phase != phase || len(v.CapturedParticipants) != 0 || v.CreatedAtMS != 0 || v.CompletedAtMS != 0 || v.SnapshotDigest != "" {
			return ErrInvalid
		}
		return nil
	}
	if !timestamp(v.CreatedAtMS) || v.CreatedAtMS < v.Event.CutoffMS || v.CreatedAtMS > v.ObservedAtMS || v.CoordinatorCreatedAtMS >= v.Event.CutoffMS {
		return ErrInvalid
	}
	switch v.Phase {
	case "awaiting_capture":
		if len(v.CapturedParticipants) != 0 {
			return ErrInvalid
		}
	case "capturing":
		if len(v.CapturedParticipants) != 1 {
			return ErrInvalid
		}
	case "snapshotted":
		if len(v.CapturedParticipants) != 2 || !hashID(v.SnapshotDigest) || v.CompletedAtMS < v.CreatedAtMS || v.CompletedAtMS > v.ObservedAtMS {
			return ErrInvalid
		}
		return nil
	default:
		return ErrInvalid
	}
	if v.CompletedAtMS != 0 || v.SnapshotDigest != "" {
		return ErrInvalid
	}
	return nil
}
func validateCaptureResult(p Pin, v CaptureResult) error {
	if v.Version != 1 || validateEventStatus(p, v.Status) != nil || validateParticipantCapture(p, v.Capture) != nil || !v.Status.Exists || v.Capture.Snapshot.Entry.Event.EventID != v.Status.Event.SourceEventID || v.Capture.Evidence.CapturedAtMS < v.Status.CreatedAtMS || v.Capture.Evidence.CapturedAtMS > v.Status.ObservedAtMS {
		return ErrInvalid
	}
	for _, id := range v.Status.CapturedParticipants {
		if id == v.Capture.Snapshot.ParticipantID {
			return nil
		}
	}
	return ErrInvalid
}
func validateHistoryPage(p Pin, v HistoryPage) error {
	id, _ := ID(p)
	if v.Version != 1 || v.StoreID != id || !optionalID(v.AfterEventID) || !optionalID(v.NextEventID) || !timestamp(v.ObservedAtMS) || v.Events == nil || len(v.Events) > 8 {
		return ErrInvalid
	}
	var last Event
	for i, x := range v.Events {
		if !x.Exists || x.ObservedAtMS != v.ObservedAtMS || validateEventStatus(p, x) != nil || x.Event.EventID == v.AfterEventID {
			return ErrInvalid
		}
		if i > 0 && (x.Event.CutoffMS < last.CutoffMS || (x.Event.CutoffMS == last.CutoffMS && x.Event.EventID <= last.EventID)) {
			return ErrInvalid
		}
		last = x.Event
	}
	if v.HasMore {
		if len(v.Events) == 0 || v.NextEventID != last.EventID {
			return ErrInvalid
		}
	} else if v.NextEventID != "" {
		return ErrInvalid
	}
	return nil
}
