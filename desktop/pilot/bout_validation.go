// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"github.com/taytr-llc/slug/fighterbout"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/fightermanifest"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymstable"
	"math"
	"reflect"
)

func boutErrorCode(s string) bool {
	return stableOne(s, "invalid_request", "unauthorized", "forbidden", "stale_view", "busy", "outcome_unknown", "bout_unavailable", "bout_setup_incomplete", "bout_corrupt", "bout_capacity", "bout_clock", "bout_calendar", "bout_not_found", "bout_source_incomplete", "bout_source_conflict", "bout_source_changed", "bout_cancelled")
}
func boutRequestValid(r BoutRequest) bool {
	if r.ID == 0 || r.ID > gymstable.MaxSafeInteger {
		return false
	}
	b := r
	b.ID = 0
	b.Command = ""
	switch r.Command {
	case "bout_snapshot":
	case "bout_card_status":
		if !canonicalManifestID(r.ViewID) || !entryDateValid(r.Saturday) {
			return false
		}
		b.ViewID = ""
		b.Saturday = ""
	case "bout_status", "bout_record", "bout_run":
		if !canonicalManifestID(r.ViewID) || !entryDateValid(r.Saturday) || !canonicalManifestID(r.BoutID) {
			return false
		}
		b.ViewID = ""
		b.Saturday = ""
		b.BoutID = ""
	case "bout_job", "bout_cancel":
		if !canonicalManifestID(r.ViewID) || !canonicalManifestID(r.JobID) {
			return false
		}
		b.ViewID = ""
		b.JobID = ""
	default:
		return false
	}
	return b == (BoutRequest{})
}
func boutSnapshotValid(s BoutSnapshot) bool {
	if s.Version != 1 {
		return false
	}
	blank := s.Pin == nil && s.StoreID == "" && s.SourceCreatedAtMS == 0 && s.CreatedAtMS == 0 && s.ActiveJobID == "" && s.QueuedJobID == "" && s.LastJobID == ""
	if s.State == "unavailable" {
		return blank && s.View == nil && s.Error == "bout_unavailable"
	}
	if s.View == nil || !stableViewValid(*s.View) {
		return false
	}
	if s.State == "setup_incomplete" {
		return blank && s.Error == "bout_setup_incomplete"
	}
	if s.Pin == nil || !stableTime(s.SourceCreatedAtMS) || !stableTime(s.CreatedAtMS) || s.CreatedAtMS < s.SourceCreatedAtMS {
		return false
	}
	p, e := gymbout.ValidatePin(*s.Pin)
	if e != nil {
		return false
	}
	id, e := gymbout.ID(p)
	if e != nil || id != s.StoreID {
		return false
	}
	if _, ok := eventViewParticipant(*s.View, p.SourcePin.SourcePin.SourcePin); !ok {
		return false
	}
	switch s.State {
	case "ready", "busy":
		if s.Error != "" {
			return false
		}
	case "failed":
		if !stableOne(s.Error, "bout_unavailable", "bout_corrupt", "bout_clock", "bout_source_changed") {
			return false
		}
	default:
		return false
	}
	seen := map[string]bool{}
	for _, id := range []string{s.ActiveJobID, s.QueuedJobID, s.LastJobID} {
		if id != "" {
			if !canonicalManifestID(id) || seen[id] {
				return false
			}
			seen[id] = true
		}
	}
	return !(s.ActiveJobID != "" && s.QueuedJobID != "") && ((s.ActiveJobID == "" && s.QueuedJobID == "") || s.State == "busy")
}
func boutBookedShape(b gymcard.Bout) bool {
	return eventClass(b.Class) && canonicalManifestID(b.BoutID) && canonicalManifestID(b.CornerA) && canonicalManifestID(b.CornerB) && b.CornerA < b.CornerB && canonicalManifestID(b.Seed)
}
func boutClassIndex(c string) int {
	for i, x := range []string{"light", "welter", "middle", "light_heavy", "heavy"} {
		if c == x {
			return i
		}
	}
	return -1
}
func boutSourceHash(s gymcard.BoutSource) string {
	return seedHash("SLUG candidate booked bout source\x00v1\x00", s)
}
func boutRecordHash(r gymbout.Record) string {
	return seedHash("SLUG candidate bout record\x00v1\x00", r)
}
func boutRawHash(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func boutIDs(s gymcard.BoutSource) bool {
	raw := []byte("SLUG candidate card input\x00v1\x00")
	for _, id := range []string{s.CardID, s.SnapshotDigest, s.SeedEventKey} {
		b, _ := hex.DecodeString(id)
		raw = append(raw, b...)
	}
	raw = binary.LittleEndian.AppendUint32(raw, s.TargetHeight)
	for _, id := range []string{s.TargetHash, gymcard.RulesID()} {
		b, _ := hex.DecodeString(id)
		raw = append(raw, b...)
	}
	input := boutRawHash(raw)
	seed := cardHashParts("SLUG candidate card seed\x00v1\x00", input, s.TargetHash)
	raw = []byte("SLUG candidate card bout\x00v1\x00")
	b, _ := hex.DecodeString(input)
	raw = append(raw, b...)
	raw = binary.LittleEndian.AppendUint32(raw, uint32(boutClassIndex(s.Bout.Class)))
	for _, id := range []string{s.Bout.CornerA, s.Bout.CornerB} {
		b, _ = hex.DecodeString(id)
		raw = append(raw, b...)
	}
	id := boutRawHash(raw)
	return id == s.Bout.BoutID && cardHashParts("SLUG candidate card bout seed\x00v1\x00", seed, id) == s.Bout.Seed
}

// These are closed transport checks, not semantic source importers. No engine,
// manifest/condition/name derivation, proof, portrait or store call is reachable.
func boutSourceShape(s gymcard.BoutSource) bool {
	if s.Version != 1 || !eventShape(s.Event) || !canonicalManifestID(s.StoreID) || s.CardID != cardIDShape(s.StoreID, s.Event.EventID) || !boutBookedShape(s.Bout) || len(s.Corners) != 2 || !stableTime(s.CardCreatedAtMS) || !stableTime(s.FinalizedAtMS) || s.FinalizedAtMS < s.Event.CutoffMS || s.CardCreatedAtMS < s.FinalizedAtMS || s.TargetHeight < 1 || s.TargetHeight > math.MaxInt32 {
		return false
	}
	for _, x := range []string{s.CardRecordDigest, s.CardSourceDigest, s.SnapshotDigest, s.SeedEventKey, s.TargetHash} {
		if !canonicalManifestID(x) {
			return false
		}
	}
	for i, c := range s.Corners {
		m, a, o := c.Member, c.Member.Assignment, c.Owner
		if o.OwnershipBasis != "candidate_assignment" || !canonicalManifestID(o.AccountID) || !canonicalManifestID(o.AccountAuthorityID) || !canonicalManifestID(o.GymID) || o.GymID != a.GymID || !eventAssignmentShape(a) || m.Slot < 1 || m.Slot > 10 || m.SlotAssignedAtMS < a.AcceptedAtMS || m.SlotAssignedAtMS >= s.Event.CutoffMS || a.AcceptedAtMS > s.Event.CutoffMS || m.Eligibility != "eligible" || m.Class != s.Bout.Class || !eventConditionShape(m.ConditionAtCutoff, a, s.Event.CutoffMS) {
			return false
		}
		cnd := m.ConditionAtCutoff
		if !stableOne(cnd.Phase, "prime", "decline") || cnd.AgeWeeks > 90 || cnd.ElapsedDeclineWeeks > 38 || cnd.CareerRemainingMS <= 0 {
			return false
		}
		if (i == 0 && a.ProspectID != s.Bout.CornerA) || (i == 1 && a.ProspectID != s.Bout.CornerB) {
			return false
		}
	}
	a, b := s.Corners[0], s.Corners[1]
	ra, rb := a.Member.ConditionAtCutoff.RatingHundredths, b.Member.ConditionAtCutoff.RatingHundredths
	return a.Owner.GymID != b.Owner.GymID && a.Owner.AccountID != b.Owner.AccountID && 2*min(ra, rb) >= max(ra, rb) && boutIDs(s)
}
func boutStateShape(s gymcard.BoutState) bool {
	if s.Version != 1 || !canonicalManifestID(s.CardID) || !eventShape(s.Event) || !stableTime(s.ObservedAtMS) || s.ConflictCount > 8 || s.ConflictOverflow && s.ConflictCount != 8 || s.TargetDescendants > math.MaxInt32 || !stableOne(s.NodeState, "unobserved", "online", "offline", "unavailable") {
		return false
	}
	for _, t := range []int64{s.CardCreatedAtMS, s.FinalizedAtMS, s.IncidentAtMS, s.ChainObservedAtMS} {
		if t < 0 || t > s.ObservedAtMS {
			return false
		}
	}
	if s.EscalationDue != (s.ObservedAtMS >= s.Event.IncidentDeadlineMS && (s.FinalizedAtMS == 0 || s.ConflictCount > 0 || s.ConflictOverflow)) || s.IncidentAtMS != 0 && s.IncidentAtMS < s.Event.IncidentDeadlineMS {
		return false
	}
	if s.ChainObservedAtMS == 0 && (s.TargetDescendants != 0 || s.CurrentFinality) {
		return false
	}
	if s.CurrentFinality && (s.FinalizedAtMS == 0 || s.ChainObservedAtMS == 0 || s.TargetDescendants < 60 || s.ConflictCount != 0 || s.ConflictOverflow) {
		return false
	}
	if s.CardExists {
		if !canonicalManifestID(s.CardRecordDigest) || !canonicalManifestID(s.CardSourceDigest) || s.FinalizedAtMS <= 0 || s.CardCreatedAtMS < s.FinalizedAtMS {
			return false
		}
	} else if s.CardRecordDigest != "" || s.CardSourceDigest != "" || s.CardCreatedAtMS != 0 || s.Usable {
		return false
	}
	return !s.Usable || (s.CardExists && s.FinalizedAtMS > 0 && s.ConflictCount == 0 && !s.ConflictOverflow)
}
func boutRecordShape(r gymbout.Record) bool {
	s, in := r.Source, r.Input
	if r.Version != 1 || !canonicalManifestID(r.StoreID) || !boutSourceShape(s) || r.CardID != s.CardID || r.BoutID != s.Bout.BoutID || r.SourceDigest != boutSourceHash(s) || !stableTime(r.CreatedAtMS) || r.CreatedAtMS < s.CardCreatedAtMS || in.Version != 1 || in.EngineID != fighterbout.EngineID || in.Mode != "standard" || in.SeedKind != "explicit" || in.Seed != s.Bout.Seed || in.AsOfMS != s.Event.CutoffMS {
		return false
	}
	for i, f := range []fighterbout.FighterInput{in.A, in.B} {
		a := s.Corners[i].Member.Assignment
		if f.ProspectID != a.ProspectID || f.BornAtMS != a.BornAtMS || f.ProXP != 0 || f.TrainingSkill != 0 || !stableManifestShapeFor(f.Manifest, a.WorkHash, a.DescriptorID) || f.Manifest.Physical.Class != s.Bout.Class {
			return false
		}
		raw, _ := json.Marshal(f.Manifest)
		descriptor, err := fighterdescriptor.ResolveCandidate(a.DescriptorID)
		if err != nil {
			return false
		}
		var domain string
		switch descriptor.ManifestVersion {
		case fightermanifest.Version1:
			domain = "SLUG fighter manifest digest\x00v1\x00"
		case fightermanifest.Version2:
			domain = "SLUG fighter manifest digest\x00v2\x00"
		default:
			return false
		}
		if len(raw) > 8192 || previewHash(domain, raw, false) != a.ManifestDigest {
			return false
		}
	}
	raw, _ := json.Marshal(in)
	if len(raw) > 16384 || previewHash("SLUG fighter bout input\x00v1\x00", raw, false) != r.InputID || r.Result.InputID != r.InputID || !previewEngineShape(r.Result, in.A.ProspectID, in.B.ProspectID) {
		return false
	}
	d := r.Result.DrawCounts
	ticks := 0
	for _, round := range r.Result.Rounds {
		ticks += len(round.Ticks)
		for _, t := range round.Ticks {
			for i, st := range t.States {
				if st.StaminaQ > r.Result.Initial[i].MaxStaminaQ {
					return false
				}
			}
		}
		if round.Recovery != nil {
			for i, st := range round.Recovery.States {
				if st.StaminaQ > r.Result.Initial[i].MaxStaminaQ {
					return false
				}
			}
		}
	}
	if d.FormA > 256 || d.FormB > 256 || d.ChinA > 256 || d.ChinB > 256 || d.Combat < uint32(ticks*3) {
		return false
	}
	raw, _ = json.Marshal(r.Result)
	return len(raw) <= 196608 && previewHash("SLUG fighter bout result\x00v1\x00", raw, false) == r.ResultDigest
}
func boutSavedShape(s gymbout.SavedBout) bool {
	if !boutBookedShape(s.Bout) || !canonicalManifestID(s.SourceDigest) {
		return false
	}
	if !s.Exists {
		return s.RecordDigest == "" && s.CreatedAtMS == 0 && s.InputID == "" && s.ResultDigest == "" && s.Winner == "" && s.Method == "" && s.EndRound == 0 && s.EndTick == 0 && !s.Usable
	}
	if !canonicalManifestID(s.RecordDigest) || !canonicalManifestID(s.InputID) || !canonicalManifestID(s.ResultDigest) || !stableTime(s.CreatedAtMS) || s.EndRound < 1 || s.EndRound > 3 || s.EndTick < 1 || s.EndTick > 60 || !stableOne(s.Method, "decision", "decision_draw", "ko", "tko_referee", "tko_three_knockdowns", "tko_corner", "double_corner_stoppage") {
		return false
	}
	return (s.Winner == "" || s.Winner == s.Bout.CornerA || s.Winner == s.Bout.CornerB) && ((s.Winner == "") == stableOne(s.Method, "decision_draw", "double_corner_stoppage"))
}
func boutStatusShape(s gymbout.Status) bool {
	return s.Version == 1 && canonicalManifestID(s.StoreID) && s.CardID == s.Source.CardID && s.ObservedAtMS == s.Source.ObservedAtMS && boutStateShape(s.Source) && s.Source.CardExists && boutSavedShape(s.Saved) && (!s.Saved.Exists || (s.Saved.CreatedAtMS >= s.Source.CardCreatedAtMS && s.Saved.CreatedAtMS <= s.ObservedAtMS)) && (!s.Saved.Usable || s.Source.Usable)
}
func boutCardStatusShape(s gymbout.CardStatus) bool {
	if s.Version != 1 || !canonicalManifestID(s.StoreID) || s.CardID != s.Source.CardID || s.ObservedAtMS != s.Source.ObservedAtMS || !boutStateShape(s.Source) || s.Bouts == nil || len(s.Bouts) > 5 || !s.Source.CardExists && len(s.Bouts) != 0 {
		return false
	}
	ids := map[string]bool{}
	last := -1
	for _, b := range s.Bouts {
		if !boutSavedShape(b) || boutClassIndex(b.Bout.Class) <= last || b.Exists && (b.CreatedAtMS < s.Source.CardCreatedAtMS || b.CreatedAtMS > s.ObservedAtMS) || b.Usable && !s.Source.Usable {
			return false
		}
		last = boutClassIndex(b.Bout.Class)
		for _, id := range []string{b.Bout.BoutID, b.Bout.CornerA, b.Bout.CornerB} {
			if ids[id] {
				return false
			}
			ids[id] = true
		}
	}
	return true
}
func boutResultShape(r gymbout.Record, s gymbout.Status) bool {
	if !boutRecordShape(r) || !boutStatusShape(s) || !s.Saved.Exists || s.StoreID != r.StoreID || s.CardID != r.CardID || s.Source.Event != r.Source.Event || s.Source.CardRecordDigest != r.Source.CardRecordDigest || s.Source.CardSourceDigest != r.Source.CardSourceDigest || s.Source.CardCreatedAtMS != r.Source.CardCreatedAtMS || s.Source.FinalizedAtMS != r.Source.FinalizedAtMS {
		return false
	}
	b := s.Saved
	return b.Bout == r.Source.Bout && b.SourceDigest == r.SourceDigest && b.RecordDigest == boutRecordHash(r) && b.CreatedAtMS == r.CreatedAtMS && b.InputID == r.InputID && b.ResultDigest == r.ResultDigest && b.Winner == r.Result.Winner && b.Method == r.Result.Method && b.EndRound == r.Result.EndRound && b.EndTick == r.Result.EndTick && b.Usable == s.Source.Usable
}
func boutJobValid(j *BoutJob) bool {
	if j.Version != 1 || !canonicalManifestID(j.JobID) || j.Kind != "run" || !stableViewValid(j.View) || !canonicalManifestID(j.StoreID) || !canonicalManifestID(j.CardID) || !canonicalManifestID(j.BoutID) || !entryDateValid(j.Saturday) {
		return false
	}
	switch j.State {
	case "queued", "running":
		return j.Error == "" && j.Result == nil
	case "cancelled":
		return j.Error == "bout_cancelled" && j.Result == nil
	case "failed":
		return j.Error != "bout_setup_incomplete" && boutErrorCode(j.Error) && j.Result == nil
	case "completed":
		return j.Error == "" && j.Result != nil && j.Result.Version == 1 && boutResultShape(j.Result.Record, j.Result.Status) && j.Result.Record.StoreID == j.StoreID && j.Result.Record.CardID == j.CardID && j.Result.Record.BoutID == j.BoutID && j.Result.Record.Source.Event.Saturday == j.Saturday
	}
	return false
}
func boutResponseValid(r *BoutResponse) bool {
	if r.ID == 0 || r.ID > gymstable.MaxSafeInteger {
		return false
	}
	n := 0
	for _, x := range []bool{r.Snapshot != nil, r.CardStatus != nil, r.Status != nil, r.Record != nil, r.Job != nil} {
		if x {
			n++
		}
	}
	if !r.OK {
		return n == 0 && boutErrorCode(r.Error) && ((r.View == nil && r.StoreID == "") || (r.View != nil && stableViewValid(*r.View) && (r.StoreID == "" || canonicalManifestID(r.StoreID))))
	}
	if n != 1 || r.Error != "" {
		return false
	}
	if r.Snapshot != nil {
		return boutSnapshotValid(*r.Snapshot) && reflect.DeepEqual(r.View, r.Snapshot.View) && r.StoreID == r.Snapshot.StoreID
	}
	if r.View == nil || !stableViewValid(*r.View) || !canonicalManifestID(r.StoreID) {
		return false
	}
	switch {
	case r.CardStatus != nil:
		return boutCardStatusShape(*r.CardStatus) && r.CardStatus.StoreID == r.StoreID
	case r.Status != nil:
		return boutStatusShape(*r.Status) && r.Status.StoreID == r.StoreID
	case r.Record != nil:
		return r.Record.Version == 1 && boutResultShape(r.Record.Record, r.Record.Status) && r.Record.Record.StoreID == r.StoreID
	case r.Job != nil:
		return boutJobValid(r.Job) && r.Job.StoreID == r.StoreID && r.Job.View == *r.View
	}
	return false
}
func cloneBoutJob(v BoutJob) BoutJob {
	raw, _ := json.Marshal(v)
	var out BoutJob
	_ = json.Unmarshal(raw, &out)
	return out
}
func cloneBoutPin(v gymbout.Pin) gymbout.Pin {
	raw, _ := json.Marshal(v)
	var out gymbout.Pin
	_ = json.Unmarshal(raw, &out)
	return out
}
