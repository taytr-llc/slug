// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"reflect"

	"github.com/taytr-llc/slug/fightercondition"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymstable"
)

func eventErrorCode(s string) bool {
	switch s {
	case "invalid_request", "unauthorized", "forbidden", "stale_view", "busy", "event_unavailable", "event_corrupt", "event_capacity", "event_clock", "event_calendar", "event_not_due", "event_created_too_late", "event_incomplete", "event_conflict", "event_not_found", "event_cancelled", "outcome_unknown", "event_setup_incomplete":
		return true
	}
	return false
}
func eventRequestValid(r EventRequest) bool {
	if r.ID == 0 || r.ID > gymstable.MaxSafeInteger {
		return false
	}
	blank := r
	blank.ID = 0
	blank.Command = ""
	switch r.Command {
	case "event_snapshot":
	case "event_status", "event_record", "event_capture":
		if !canonicalManifestID(r.ViewID) || !entryDateValid(r.Saturday) {
			return false
		}
		blank.ViewID = ""
		blank.Saturday = ""
	case "event_history":
		if !canonicalManifestID(r.ViewID) || r.AfterEventID != "" && !canonicalManifestID(r.AfterEventID) || r.Limit < 1 || r.Limit > 8 {
			return false
		}
		blank.ViewID = ""
		blank.AfterEventID = ""
		blank.Limit = 0
	case "event_job", "event_cancel":
		if !canonicalManifestID(r.ViewID) || !canonicalManifestID(r.JobID) {
			return false
		}
		blank.ViewID = ""
		blank.JobID = ""
	default:
		return false
	}
	return blank == (EventRequest{})
}
func eventViewParticipant(v ManifestViewPin, p gymevent.Pin) (string, bool) {
	for _, x := range p.Participants {
		if x.StablePin.AccountID == v.AccountID && x.StablePin.AccountAuthorityID == v.AccountAuthorityID && x.StablePin.Source == v.AuthorityPin {
			return x.StablePin.Identity.GymID, true
		}
	}
	return "", false
}
func eventSnapshotValid(s EventSnapshot) bool {
	if s.Version != 1 {
		return false
	}
	if s.State == "unavailable" {
		return s.Error == "event_unavailable" && s.View == nil && s.Pin == nil && s.StoreID == "" && s.ParticipantID == "" && s.CoordinatorCreatedAtMS == 0 && s.ActiveJobID == "" && s.QueuedJobID == "" && s.LastJobID == ""
	}
	if s.View == nil || !stableViewValid(*s.View) || !canonicalManifestID(s.ParticipantID) {
		return false
	}
	if s.State == "setup_incomplete" {
		return s.Error == "event_setup_incomplete" && s.Pin == nil && s.StoreID == "" && s.CoordinatorCreatedAtMS == 0 && s.ActiveJobID == "" && s.QueuedJobID == "" && s.LastJobID == ""
	}
	if s.Pin == nil || !stableTime(s.CoordinatorCreatedAtMS) {
		return false
	}
	p, e := gymevent.ValidatePin(*s.Pin)
	if e != nil {
		return false
	}
	id, e := gymevent.ID(p)
	if e != nil || id != s.StoreID {
		return false
	}
	who, ok := eventViewParticipant(*s.View, p)
	if !ok || who != s.ParticipantID {
		return false
	}
	switch s.State {
	case "ready", "busy":
		if s.Error != "" {
			return false
		}
	case "failed":
		if !eventErrorCode(s.Error) || s.Error == "outcome_unknown" {
			return false
		}
	default:
		return false
	}
	for _, id := range []string{s.ActiveJobID, s.QueuedJobID, s.LastJobID} {
		if id != "" && !canonicalManifestID(id) {
			return false
		}
	}
	return s.ActiveJobID == "" || s.ActiveJobID != s.QueuedJobID
}

// All following checks are bounded transport checks, never source work or
// condition/manifest regeneration. Full semantic validation belongs before
// helper publication and to the caller's established Pin, outside any lease.
func eventShape(e gymevent.Event) bool {
	if e.Version != 1 || !entryDateValid(e.Saturday) || !stableTime(e.CutoffMS) || !stableTime(e.IncidentDeadlineMS) || e.IncidentDeadlineMS <= e.CutoffMS {
		return false
	}
	for _, id := range []string{e.EventID, e.SourceEventID, e.StoreID, e.NamespaceID, e.CalendarID, e.RulesID} {
		if !canonicalManifestID(id) {
			return false
		}
	}
	source := gymentry.Event{Version: 1, EventID: e.SourceEventID, NamespaceID: e.NamespaceID, CalendarID: e.CalendarID, RulesID: gymentry.RulesID(), Saturday: e.Saturday, CutoffMS: e.CutoffMS}
	if _, x := gymentry.ValidateEvent(source); x != nil {
		return false
	}
	deadline, x := gymentry.IncidentDeadline(source)
	if x != nil || deadline != e.IncidentDeadlineMS || e.RulesID != gymevent.RulesID() {
		return false
	}
	raw := []byte("SLUG candidate cutoff event\x00v1\x00")
	for _, id := range []string{e.StoreID, e.SourceEventID} {
		b, _ := hex.DecodeString(id)
		raw = append(raw, b...)
	}
	raw = binary.LittleEndian.AppendUint64(raw, uint64(e.CutoffMS))
	raw = binary.LittleEndian.AppendUint64(raw, uint64(e.IncidentDeadlineMS))
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:]) == e.EventID
}
func eventStatusShape(s gymevent.EventStatus) bool {
	if s.Version != 1 || s.StoreID != s.Event.StoreID || !eventShape(s.Event) || !stableTime(s.CoordinatorCreatedAtMS) || !stableTime(s.ObservedAtMS) || s.ObservedAtMS < s.CoordinatorCreatedAtMS || s.SeedAuthority != "unavailable" || s.CapturedParticipants == nil || len(s.CapturedParticipants) > 2 {
		return false
	}
	for i, id := range s.CapturedParticipants {
		if !canonicalManifestID(id) || i > 0 && s.CapturedParticipants[i-1] >= id {
			return false
		}
	}
	if !s.Exists {
		return s.CreatedAtMS == 0 && s.CompletedAtMS == 0 && s.SnapshotDigest == "" && len(s.CapturedParticipants) == 0 && ((s.ObservedAtMS < s.Event.CutoffMS && s.Phase == "before_cutoff") || (s.ObservedAtMS >= s.Event.CutoffMS && s.Phase == "awaiting_capture"))
	}
	if !stableTime(s.CreatedAtMS) || s.CreatedAtMS < s.Event.CutoffMS || s.CreatedAtMS > s.ObservedAtMS || s.CoordinatorCreatedAtMS >= s.Event.CutoffMS {
		return false
	}
	switch s.Phase {
	case "awaiting_capture":
		return len(s.CapturedParticipants) == 0 && s.CompletedAtMS == 0 && s.SnapshotDigest == ""
	case "capturing":
		return len(s.CapturedParticipants) == 1 && s.CompletedAtMS == 0 && s.SnapshotDigest == ""
	case "snapshotted":
		return len(s.CapturedParticipants) == 2 && canonicalManifestID(s.SnapshotDigest) && s.CompletedAtMS >= s.CreatedAtMS && s.CompletedAtMS <= s.ObservedAtMS
	}
	return false
}
func eventConditionShape(c fightercondition.Snapshot, a gymstable.Assignment, at int64) bool {
	if c.Version != 1 || c.ProjectorID != fightercondition.ProjectorID || c.DNAVersion != 1 || c.Seed != a.WorkHash || c.BornAtMS != a.BornAtMS || c.AsOfMS != at || !stableTime(at) || at < a.BornAtMS || c.AgeWeeks != (at-a.BornAtMS)/604800000 || c.ProXP != 0 || c.AgedAttributesTenths != c.LeagueAttributesTenths {
		return false
	}
	if c.ElapsedDeclineWeeks < 0 || c.ElapsedDeclineWeeks > 39 || c.PrimeRemainingWeeks < 0 || c.PrimeRemainingWeeks > 52 || c.PrimeRemainingMS < 0 || c.PrimeRemainingMS > 52*604800000 || c.CareerRemainingMS < 0 || c.CareerRemainingMS > 91*604800000 {
		return false
	}
	switch c.Phase {
	case "prime":
		if c.ElapsedDeclineWeeks != 0 || c.PrimeRemainingWeeks == 0 || c.PrimeRemainingMS == 0 {
			return false
		}
	case "decline":
		if c.PrimeRemainingWeeks != 0 || c.PrimeRemainingMS != 0 || c.CareerRemainingMS == 0 {
			return false
		}
	case "retired":
		if c.PrimeRemainingWeeks != 0 || c.PrimeRemainingMS != 0 || c.CareerRemainingMS != 0 {
			return false
		}
	default:
		return false
	}
	v := c.LeagueAttributesTenths
	for _, x := range []int{v.Power, v.Speed, v.Chin, v.Stamina, v.Skill} {
		if x < 1 || x > 1000 {
			return false
		}
	}
	return c.RatingHundredths == 10*v.Power+25*v.Speed+2*v.Chin+4*v.Stamina+8*v.Skill
}
func eventAssignmentShape(a gymstable.Assignment) bool {
	if a.Version != 1 || a.Sequence < 1 || a.Sequence > 1000 || !stableTime(a.BornAtMS) || !stableTime(a.AcceptedAtMS) || a.BornAtMS > a.AcceptedAtMS || a.DescriptorID != manifestDescriptorID {
		return false
	}
	for _, x := range []string{a.ProspectID, a.GymID, a.WorkHash, a.DescriptorID, a.ManifestDigest, a.PortraitDigest, a.SourceRecordDigest} {
		if !canonicalManifestID(x) {
			return false
		}
	}
	return true
}
func eventClass(c string) bool {
	switch c {
	case "light", "welter", "middle", "light_heavy", "heavy":
		return true
	}
	return false
}
func eventParticipantShape(p gymevent.ParticipantSnapshot, e gymevent.Event) bool {
	r := p.Roster
	en := p.Entry
	o := p.Owner
	if p.Version != 1 || !canonicalManifestID(p.ParticipantID) || !canonicalManifestID(p.EntryStoreID) || o.OwnershipBasis != "candidate_assignment" || o.GymID != p.ParticipantID || !canonicalManifestID(o.AccountAuthorityID) || !canonicalManifestID(o.AccountID) || r.Version != 1 || !canonicalManifestID(r.StoreID) || r.CutoffMS != e.CutoffMS || r.CommittedCount > 4096 || r.LastSequence > 4096 || r.Revision > uint64(r.CommittedCount) || !canonicalManifestID(r.HistoryDigest) || en.Version != 1 || en.StoreID != p.EntryStoreID || en.Event.EventID != e.SourceEventID || en.Event.CutoffMS != e.CutoffMS || en.Event.Saturday != e.Saturday || en.CommittedCount > 4096 || en.LastSequence > 4096 || !canonicalManifestID(en.HistoryDigest) || p.Members == nil || len(p.Members) > 5 {
		return false
	}
	if _, x := gymentry.ValidateEvent(en.Event); x != nil {
		return false
	}
	want, x := gymentry.EntryID(en.StoreID, e.SourceEventID, p.ParticipantID)
	if x != nil || en.EntryID != want {
		return false
	}
	if (r.CommittedCount == 0) != (r.LastSequence == 0) || (en.CommittedCount == 0) != (en.LastSequence == 0) || r.LastSequence < uint64(r.CommittedCount) || en.LastSequence < uint64(en.CommittedCount) {
		return false
	}
	seen := map[string]bool{}
	for i, id := range r.Slots {
		if id == "" {
			if r.AssignedAtMS[i] != 0 {
				return false
			}
		} else {
			if !canonicalManifestID(id) || seen[id] || !stableTime(r.AssignedAtMS[i]) || r.AssignedAtMS[i] >= e.CutoffMS {
				return false
			}
			seen[id] = true
		}
	}
	if en.LatestReceipt == nil {
		return en.CommittedCount == 0 && len(p.Members) == 0
	}
	q := en.LatestReceipt
	if q.Version != 1 || q.StoreID != en.StoreID || q.EntryID != en.EntryID || q.Event != en.Event || q.OperationSequence != en.LastSequence || q.Revision != uint64(en.CommittedCount) || q.PreviousRevision+1 != q.Revision || !canonicalManifestID(q.OperationID) || !canonicalManifestID(q.ViewID) || q.Generation == 0 || q.Generation > gymstable.MaxSafeInteger || !stableTime(q.CommittedAtMS) || q.CommittedAtMS >= e.CutoffMS || q.Members == nil || len(q.Members) != len(p.Members) || q.StableCheckpoint > q.ObservedSourceCount || q.ObservedSourceCount > 1000 || q.RosterRevision > 4096 {
		return false
	}
	if q.Action == "withdraw" {
		return len(p.Members) == 0 && q.RosterRevision == 0
	}
	if q.Action != "replace" || len(p.Members) == 0 {
		return false
	}
	classes := map[string]bool{}
	oldSlots := map[uint32]bool{}
	for i, m := range p.Members {
		old := q.Members[i]
		a := m.Assignment
		if !eventAssignmentShape(a) || a.GymID != p.ParticipantID || a.AcceptedAtMS > q.CommittedAtMS || a.Sequence > q.StableCheckpoint || !eventClass(m.Class) || classes[m.Class] || m.Assignment != old.Assignment || m.Class != old.Class || old.Slot < 1 || old.Slot > 10 || oldSlots[old.Slot] || old.ConditionAtSave.Phase == "retired" || !eventConditionShape(old.ConditionAtSave, a, q.CommittedAtMS) || !eventConditionShape(m.ConditionAtCutoff, a, e.CutoffMS) || i > 0 && p.Members[i-1].Assignment.ProspectID >= a.ProspectID {
			return false
		}
		slot := uint32(0)
		for k, id := range r.Slots {
			if id == a.ProspectID {
				slot = uint32(k + 1)
			}
		}
		if m.Slot != slot {
			return false
		}
		if slot == 0 {
			if m.SlotAssignedAtMS != 0 {
				return false
			}
		} else {
			if m.SlotAssignedAtMS != r.AssignedAtMS[slot-1] || a.AcceptedAtMS > m.SlotAssignedAtMS {
				return false
			}
		}
		eligible := "eligible"
		if slot == 0 {
			eligible = "not_rostered"
		}
		if m.ConditionAtCutoff.Phase == "retired" {
			eligible = "retired"
		}
		if m.Eligibility != eligible {
			return false
		}
		classes[m.Class] = true
		oldSlots[old.Slot] = true
	}
	return true
}
func eventEvidenceShape(c gymevent.CaptureEvidence, p gymevent.ParticipantSnapshot, e gymevent.Event) bool {
	if c.Version != 1 || c.ParticipantID != p.ParticipantID || !stableTime(c.CapturedAtMS) {
		return false
	}
	for i, f := range []gymstable.CutoffFence{c.StableFence, c.EntryFence} {
		want := p.Roster.StoreID
		count := p.Roster.LastSequence
		if i == 1 {
			want = p.EntryStoreID
			count = p.Entry.LastSequence
		}
		if f.Version != 1 || f.StoreID != want || f.CutoffMS != e.CutoffMS || f.OperationCount > 4096 || uint64(f.OperationCount) < count || !stableTime(f.ObservedAtMS) || f.ObservedAtMS < e.CutoffMS || f.ObservedAtMS > c.CapturedAtMS {
			return false
		}
	}
	return c.EntryFence.ObservedAtMS >= c.StableFence.ObservedAtMS
}
func eventCaptureShape(r gymevent.CaptureResult) bool {
	p := r.Capture.Snapshot
	e := r.Status.Event
	if r.Version != 1 || !eventStatusShape(r.Status) || r.Capture.Version != 1 || !eventParticipantShape(p, e) || !eventEvidenceShape(r.Capture.Evidence, p, e) || r.Capture.Evidence.CapturedAtMS < r.Status.CreatedAtMS || r.Capture.Evidence.CapturedAtMS > r.Status.ObservedAtMS {
		return false
	}
	for _, id := range r.Status.CapturedParticipants {
		if id == p.ParticipantID {
			return true
		}
	}
	return false
}
func eventRecordShape(r gymevent.SnapshotReceipt) bool {
	if r.Version != 1 || r.Input.Version != 1 || !eventShape(r.Input.Event) || len(r.Input.Participants) != 2 || len(r.Captures) != 2 || !stableTime(r.CompletedAtMS) {
		return false
	}
	for i, p := range r.Input.Participants {
		if !eventParticipantShape(p, r.Input.Event) || !eventEvidenceShape(r.Captures[i], p, r.Input.Event) || r.Captures[i].CapturedAtMS > r.CompletedAtMS || i > 0 && r.Input.Participants[i-1].ParticipantID >= p.ParticipantID {
			return false
		}
	}
	raw, _ := json.Marshal(r.Input)
	b := []byte("SLUG candidate cutoff input\x00v1\x00")
	b = binary.LittleEndian.AppendUint32(b, uint32(len(raw)))
	b = append(b, raw...)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]) == r.SnapshotDigest
}
func eventHistoryShape(h gymevent.HistoryPage) bool {
	if h.Version != 1 || !canonicalManifestID(h.StoreID) || !stableTime(h.ObservedAtMS) || h.Events == nil || len(h.Events) > 8 || h.AfterEventID != "" && !canonicalManifestID(h.AfterEventID) {
		return false
	}
	for i, s := range h.Events {
		if !eventStatusShape(s) || !s.Exists || s.StoreID != h.StoreID || s.ObservedAtMS != h.ObservedAtMS || s.Event.EventID == h.AfterEventID || i > 0 && h.Events[i-1].Event.CutoffMS >= s.Event.CutoffMS {
			return false
		}
	}
	if h.HasMore {
		return len(h.Events) > 0 && h.NextEventID == h.Events[len(h.Events)-1].Event.EventID
	}
	return h.NextEventID == ""
}
func eventJobValid(j *EventJob) bool {
	if j.Version != 1 || j.Kind != "capture" || !canonicalManifestID(j.JobID) || !stableViewValid(j.View) || !canonicalManifestID(j.StoreID) || !canonicalManifestID(j.EventID) || !entryDateValid(j.Saturday) || !canonicalManifestID(j.ParticipantID) {
		return false
	}
	switch j.State {
	case "queued", "running":
		return j.Error == "" && j.Result == nil
	case "failed":
		return eventErrorCode(j.Error) && j.Result == nil
	case "cancelled":
		return j.Error == "event_cancelled" && j.Result == nil
	case "completed":
		if j.Error != "" || j.Result == nil || !eventCaptureShape(*j.Result) {
			return false
		}
	default:
		return false
	}
	r := j.Result
	p := r.Capture.Snapshot
	return r.Status.StoreID == j.StoreID && r.Status.Event.EventID == j.EventID && r.Status.Event.Saturday == j.Saturday && p.ParticipantID == j.ParticipantID && p.Owner.AccountID == j.View.AccountID && p.Owner.AccountAuthorityID == j.View.AccountAuthorityID
}
func eventResponseValid(r *EventResponse) bool {
	if r.ID == 0 || r.ID > gymstable.MaxSafeInteger {
		return false
	}
	n := 0
	for _, b := range []bool{r.Snapshot != nil, r.Status != nil, r.Record != nil, r.History != nil, r.Job != nil} {
		if b {
			n++
		}
	}
	if !r.OK {
		return n == 0 && r.View == nil && r.StoreID == "" && eventErrorCode(r.Error)
	}
	if n != 1 || r.Error != "" {
		return false
	}
	if r.Snapshot != nil {
		return eventSnapshotValid(*r.Snapshot) && reflect.DeepEqual(r.View, r.Snapshot.View) && r.StoreID == r.Snapshot.StoreID
	}
	if r.View == nil || !stableViewValid(*r.View) || !canonicalManifestID(r.StoreID) {
		return false
	}
	switch {
	case r.Status != nil:
		return eventStatusShape(*r.Status) && r.Status.StoreID == r.StoreID
	case r.Record != nil:
		return eventRecordShape(*r.Record) && r.Record.Input.Event.StoreID == r.StoreID
	case r.History != nil:
		return eventHistoryShape(*r.History) && r.History.StoreID == r.StoreID
	case r.Job != nil:
		return eventJobValid(r.Job) && r.Job.StoreID == r.StoreID && r.Job.View == *r.View
	}
	return false
}
