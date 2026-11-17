// Copyright (c) 2026 The SLUG developers. ISC license.
package gymentry

import (
	"encoding/hex"
	"encoding/json"
	"github.com/taytr-llc/slug/fightercondition"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/fightermanifest"
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
func ids(v []string) bool {
	if v == nil || len(v) > MaxMembers {
		return false
	}
	for i, s := range v {
		if !hashID(s) || (i > 0 && v[i-1] >= s) {
			return false
		}
	}
	return true
}
func validatePin(p Pin) error {
	if p.Version != Version1 || p.CalendarID != CalendarID() || p.RulesID != RulesID() {
		return ErrInvalid
	}
	sp, e := gymstable.ValidatePin(p.StablePin)
	if e != nil {
		return ErrInvalid
	}
	sid, _ := gymstable.ID(sp)
	ns, _ := NamespaceID(sp.Identity.Network, sp.Identity.Genesis, sp.AccountAuthorityID)
	if p.StableStoreID != sid || p.NamespaceID != ns {
		return ErrInvalid
	}
	return nil
}
func ID(p Pin) (string, error) {
	b, e := MarshalPin(p)
	if e != nil {
		return "", e
	}
	return hash("SLUG candidate entry store\x00v1\x00", b), nil
}
func ReceiptDigest(r Receipt) (string, error) {
	b, e := MarshalReceipt(r)
	if e != nil {
		return "", e
	}
	return hash("SLUG candidate entry receipt\x00v1\x00", b), nil
}
func validateEvent(v Event) error {
	want, e := eventForNamespace(v.NamespaceID, v.Saturday)
	if e != nil {
		return e
	}
	if v != want {
		return ErrInvalid
	}
	return nil
}
func validateRequest(v Request) error {
	if v.Version != Version1 || !hashID(v.EventID) || v.ExpectedRevision > MaxOperations || v.ExpectedRosterRevision > gymstable.MaxOperations || !ids(v.ProspectIDs) {
		return ErrInvalid
	}
	if _, e := cutoff(v.Saturday); e != nil {
		return e
	}
	switch v.Action {
	case "replace":
		if len(v.ProspectIDs) < 1 {
			return ErrInvalid
		}
	case "withdraw":
		if len(v.ProspectIDs) != 0 || v.ExpectedRosterRevision != 0 {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}
func validateIntent(v Intent) error {
	if v.Version != Version1 || v.Sequence == 0 || v.Sequence > MaxOperations || !hashID(v.OperationID) || v.Generation == 0 || v.Generation > MaxSafeInteger || !hashID(v.ViewID) || !timestamp(v.PreparedAtMS) {
		return ErrInvalid
	}
	if e := validateRequest(v.Request); e != nil {
		return e
	}
	c, _ := cutoff(v.Request.Saturday)
	if v.PreparedAtMS >= c || v.Request.ExpectedRevision >= v.Sequence {
		return ErrInvalid
	}
	return nil
}
func manifestFor(a gymstable.Assignment) (fightermanifest.Manifest, error) {
	if a.Version != gymstable.Version1 || a.Sequence < 1 || a.Sequence > gymstable.MaxAssignments || !timestamp(a.BornAtMS) || a.AcceptedAtMS < a.BornAtMS || !timestamp(a.AcceptedAtMS) || a.AcceptedAtMS-a.BornAtMS >= 300000 {
		return fightermanifest.Manifest{}, ErrInvalid
	}
	for _, s := range []string{a.ProspectID, a.GymID, a.WorkHash, a.DescriptorID, a.ManifestDigest, a.PortraitDigest, a.SourceRecordDigest} {
		if !hashID(s) {
			return fightermanifest.Manifest{}, ErrInvalid
		}
	}
	d, e := fighterdescriptor.ResolveCandidate(a.DescriptorID)
	if e != nil {
		return fightermanifest.Manifest{}, ErrInvalid
	}
	m, e := fightermanifest.GenerateText(d.ManifestVersion, a.WorkHash)
	if e != nil {
		return m, ErrInvalid
	}
	md, e := fightermanifest.Digest(m)
	if e != nil || md != a.ManifestDigest {
		return m, ErrInvalid
	}
	// Journal codecs and Open recompute bounded DNA/manifest/condition facts,
	// not portrait bytes or proof authority. The portrait digest is checked for
	// canonical shape above. Apply and Check obtain its authenticated binding
	// from the concrete source through gymstable's verified selection path.
	return m, nil
}
func memberCondition(a gymstable.Assignment, at int64) (fightermanifest.Manifest, fightercondition.Snapshot, error) {
	m, e := manifestFor(a)
	if e != nil {
		return m, fightercondition.Snapshot{}, e
	}
	if at < a.AcceptedAtMS || !timestamp(at) {
		return m, fightercondition.Snapshot{}, ErrInvalid
	}
	c, e := fightercondition.Project(fightercondition.Version1, m.DNA, a.BornAtMS, at, 0)
	return m, c, e
}
func validateReceipt(r Receipt) error {
	if r.Version != Version1 || !hashID(r.StoreID) || !hashID(r.EntryID) || !hashID(r.OperationID) || !hashID(r.ViewID) || r.OperationSequence < 1 || r.OperationSequence > MaxOperations || r.Generation == 0 || r.Generation > MaxSafeInteger || r.Revision != r.PreviousRevision+1 || r.Revision > r.OperationSequence || r.RosterRevision > gymstable.MaxOperations || r.StableCheckpoint > r.ObservedSourceCount || r.ObservedSourceCount > gymstable.MaxAssignments || !timestamp(r.CommittedAtMS) || r.Members == nil || len(r.Members) > MaxMembers {
		return ErrInvalid
	}
	if e := validateEvent(r.Event); e != nil {
		return e
	}
	if r.CommittedAtMS >= r.Event.CutoffMS {
		return ErrInvalid
	}
	if r.Action == "withdraw" {
		if len(r.Members) != 0 || r.RosterRevision != 0 {
			return ErrInvalid
		}
		return nil
	}
	if r.Action != "replace" || len(r.Members) == 0 {
		return ErrInvalid
	}
	classes, slots := map[string]bool{}, map[uint32]bool{}
	for i, m := range r.Members {
		if m.Slot < 1 || m.Slot > 10 || slots[m.Slot] || classes[m.Class] || (i > 0 && r.Members[i-1].Assignment.ProspectID >= m.Assignment.ProspectID) || m.Assignment.Sequence > r.StableCheckpoint {
			return ErrInvalid
		}
		manifest, c, e := memberCondition(m.Assignment, r.CommittedAtMS)
		if e != nil || manifest.Physical.Class != m.Class || c != m.ConditionAtSave || c.Phase == "retired" {
			return ErrInvalid
		}
		if i > 0 && (m.Assignment.GymID != r.Members[0].Assignment.GymID || m.Assignment.DescriptorID != r.Members[0].Assignment.DescriptorID) {
			return ErrInvalid
		}
		eid, _ := EntryID(r.StoreID, r.Event.EventID, m.Assignment.GymID)
		if eid != r.EntryID {
			return ErrInvalid
		}
		b, _ := json.Marshal(m)
		if len(b) > MaxMemberSize {
			return ErrInvalid
		}
		slots[m.Slot] = true
		classes[m.Class] = true
	}
	return nil
}
func validateOperation(v Operation) error {
	if v.Version != Version1 {
		return ErrInvalid
	}
	if e := validateIntent(v.Intent); e != nil {
		return e
	}
	switch v.State {
	case "prepared":
		if v.Receipt != nil || v.AbandonedAtMS != 0 {
			return ErrInvalid
		}
	case "abandoned":
		if v.Receipt != nil || !timestamp(v.AbandonedAtMS) || v.AbandonedAtMS < v.Intent.PreparedAtMS {
			return ErrInvalid
		}
	case "committed":
		if v.Receipt == nil || v.AbandonedAtMS != 0 {
			return ErrInvalid
		}
		r := *v.Receipt
		i := v.Intent
		if e := validateReceipt(r); e != nil {
			return e
		}
		if r.OperationID != i.OperationID || r.OperationSequence != i.Sequence || r.Generation != i.Generation || r.ViewID != i.ViewID || r.Event.EventID != i.Request.EventID || r.Event.Saturday != i.Request.Saturday || r.Action != i.Request.Action || r.PreviousRevision != i.Request.ExpectedRevision || r.RosterRevision != i.Request.ExpectedRosterRevision || r.CommittedAtMS < i.PreparedAtMS || len(r.Members) != len(i.Request.ProspectIDs) {
			return ErrInvalid
		}
		for n, m := range r.Members {
			if m.Assignment.ProspectID != i.Request.ProspectIDs[n] {
				return ErrInvalid
			}
		}
	default:
		return ErrInvalid
	}
	return nil
}
func validateMutationResult(v MutationResult) error {
	if v.Version != Version1 {
		return ErrInvalid
	}
	return validateReceipt(v.Receipt)
}
func validateOperationResult(v OperationResult) error {
	if v.Version != Version1 || !hashID(v.StoreID) {
		return ErrInvalid
	}
	if v.Operation != nil {
		if e := validateOperation(*v.Operation); e != nil {
			return e
		}
		if v.Operation.Receipt != nil && v.Operation.Receipt.StoreID != v.StoreID {
			return ErrInvalid
		}
	}
	return nil
}
func validateEventStatus(v EventStatus) error {
	if v.Version != Version1 || !hashID(v.StoreID) || !hashID(v.EntryID) || !optionalID(v.LatestOperationID) || !optionalID(v.LatestReceiptDigest) || !optionalID(v.PendingOperationID) || !ids(v.SavedIDs) || v.Revision > MaxOperations || v.SavedRosterRevision > gymstable.MaxOperations || v.CurrentRosterRevision > gymstable.MaxOperations || v.Eligibility != "not_checked" || !timestamp(v.ObservedAtMS) || v.SavedAtMS < 0 || v.SavedAtMS > v.ObservedAtMS || v.ClosedObservedAtMS < 0 || v.ClosedObservedAtMS > v.ObservedAtMS {
		return ErrInvalid
	}
	if e := validateEvent(v.Event); e != nil {
		return e
	}
	phase := "accepting"
	if v.ObservedAtMS >= v.Event.CutoffMS {
		phase = "closed_pending_lock"
	}
	if v.Phase != phase {
		return ErrInvalid
	}
	if !v.Exists && (v.Revision != 0 || v.PendingOperationID != "" || v.ClosedObservedAtMS != 0) {
		return ErrInvalid
	}
	if v.Revision == 0 {
		if v.LatestOperationID != "" || v.LatestReceiptDigest != "" || len(v.SavedIDs) != 0 || v.SavedAtMS != 0 || v.SavedRosterRevision != 0 {
			return ErrInvalid
		}
	} else if v.LatestOperationID == "" || v.LatestReceiptDigest == "" || v.SavedAtMS <= 0 || v.SavedAtMS >= v.Event.CutoffMS {
		return ErrInvalid
	}
	if v.RosterChanged != (len(v.SavedIDs) > 0 && v.SavedRosterRevision != v.CurrentRosterRevision) {
		return ErrInvalid
	}
	if len(v.SavedIDs) == 0 && v.SavedRosterRevision != 0 {
		return ErrInvalid
	}
	if phase == "accepting" && v.ClosedObservedAtMS != 0 {
		return ErrInvalid
	}
	if v.Exists && phase == "closed_pending_lock" && v.ClosedObservedAtMS < v.Event.CutoffMS {
		return ErrInvalid
	}
	return nil
}
func validateCheckResult(v CheckResult) error {
	if v.Version != Version1 || v.AsOfMS != v.Status.ObservedAtMS || v.RosterRevision != v.Status.CurrentRosterRevision || v.Members == nil || len(v.Members) != len(v.Status.SavedIDs) {
		return ErrInvalid
	}
	if e := validateEventStatus(v.Status); e != nil {
		return e
	}
	slots := map[uint32]bool{}
	for i, m := range v.Members {
		if m.Slot > 10 || (m.Slot != 0 && slots[m.Slot]) || m.Item.Assignment.ProspectID != v.Status.SavedIDs[i] {
			return ErrInvalid
		}
		manifest, c, e := memberCondition(m.Item.Assignment, v.AsOfMS)
		if e != nil || !equal(manifest, m.Item.Manifest) || c != m.Item.Condition {
			return ErrInvalid
		}
		want := "eligible"
		if m.Slot == 0 {
			want = "not_rostered"
		}
		if c.Phase == "retired" {
			want = "retired"
		}
		if m.Eligibility != want {
			return ErrInvalid
		}
		if m.Slot != 0 {
			slots[m.Slot] = true
		}
		b, _ := json.Marshal(m)
		if len(b) > MaxCheckMemberSize {
			return ErrInvalid
		}
	}
	return nil
}
func validateHistoryPage(v HistoryPage) error {
	if v.Version != Version1 || !hashID(v.StoreID) || !optionalID(v.AfterEventID) || !optionalID(v.NextEventID) || !timestamp(v.ObservedAtMS) || v.Events == nil || len(v.Events) > MaxPageSize || v.HasMore != (v.NextEventID != "") {
		return ErrInvalid
	}
	for i, e := range v.Events {
		if validateEventStatus(e) != nil || e.StoreID != v.StoreID || e.ObservedAtMS != v.ObservedAtMS || !e.Exists || e.Event.EventID == v.AfterEventID {
			return ErrInvalid
		}
		if i > 0 {
			p := v.Events[i-1].Event
			if p.CutoffMS > e.Event.CutoffMS || (p.CutoffMS == e.Event.CutoffMS && p.EventID >= e.Event.EventID) {
				return ErrInvalid
			}
		}
	}
	if v.HasMore && (len(v.Events) == 0 || v.NextEventID != v.Events[len(v.Events)-1].Event.EventID) {
		return ErrInvalid
	}
	return nil
}
