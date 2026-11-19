// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"encoding/json"
	"github.com/taytr-llc/slug/fightercondition"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/fightermanifest"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymstable"
)

// These private transport checks keep polling separate from authority and disk
// validation. They check bounded public semantics on the trusted helper pipe
// without SVG, proof, or bout work.
func entryItemShape(i gymstable.Item, at int64, descriptorID string) bool {
	a := i.Assignment
	status := gymstable.Status{Checkpoint: gymstable.MaxAssignments, Pin: gymstable.Pin{ProjectorID: i.Condition.ProjectorID}}
	status.Pin.Identity.GymID = a.GymID
	status.Pin.Source.DescriptorID = descriptorID
	if a.Sequence == 0 || a.Sequence > gymstable.MaxAssignments || !canonicalManifestID(a.GymID) || i.Condition.ProjectorID != fightercondition.ProjectorID || !stableItemValid(i, status, at) {
		return false
	}
	digest, e := fightermanifest.Digest(i.Manifest)
	return e == nil && digest == a.ManifestDigest
}
func entryReceiptShape(r gymentry.Receipt, descriptorID string) bool {
	descriptor, err := fighterdescriptor.ResolveCandidate(descriptorID)
	if err != nil {
		return false
	}
	if r.Version != 1 || !canonicalManifestID(r.StoreID) || !canonicalManifestID(r.EntryID) || !canonicalManifestID(r.OperationID) || !canonicalManifestID(r.ViewID) || r.OperationSequence < 1 || r.OperationSequence > gymentry.MaxOperations || r.Generation == 0 || r.Generation > gymentry.MaxSafeInteger || r.PreviousRevision >= gymentry.MaxOperations || r.Revision != r.PreviousRevision+1 || r.Revision > r.OperationSequence || r.RosterRevision > gymstable.MaxOperations || r.StableCheckpoint > r.ObservedSourceCount || r.ObservedSourceCount > gymstable.MaxAssignments || !stableTime(r.CommittedAtMS) || r.Members == nil || len(r.Members) > 5 {
		return false
	}
	if _, e := gymentry.ValidateEvent(r.Event); e != nil || r.CommittedAtMS >= r.Event.CutoffMS {
		return false
	}
	if r.Action == "withdraw" {
		return len(r.Members) == 0 && r.RosterRevision == 0
	}
	if r.Action != "replace" || len(r.Members) == 0 {
		return false
	}
	slots := map[uint32]bool{}
	classes := map[string]bool{}
	for n, m := range r.Members {
		if m.Slot < 1 || m.Slot > 10 || slots[m.Slot] || classes[m.Class] || m.Assignment.Sequence > r.StableCheckpoint || m.Assignment.DescriptorID != descriptorID || n > 0 && r.Members[n-1].Assignment.ProspectID >= m.Assignment.ProspectID {
			return false
		}
		manifest, e := fightermanifest.GenerateText(descriptor.ManifestVersion, m.Assignment.WorkHash)
		if e != nil || manifest.Physical.Class != m.Class || m.ConditionAtSave.Phase == "retired" || !entryItemShape(gymstable.Item{Assignment: m.Assignment, Manifest: manifest, Condition: m.ConditionAtSave}, r.CommittedAtMS, descriptorID) {
			return false
		}
		if n > 0 && (m.Assignment.GymID != r.Members[0].Assignment.GymID || m.Assignment.DescriptorID != r.Members[0].Assignment.DescriptorID) {
			return false
		}
		id, e := gymentry.EntryID(r.StoreID, r.Event.EventID, m.Assignment.GymID)
		if e != nil || id != r.EntryID {
			return false
		}
		raw, _ := json.Marshal(m)
		if len(raw) > gymentry.MaxMemberSize {
			return false
		}
		slots[m.Slot] = true
		classes[m.Class] = true
	}
	return true
}
func entryOperationShape(v gymentry.OperationResult, descriptorID string) bool {
	if v.Version != 1 || !canonicalManifestID(v.StoreID) {
		return false
	}
	if v.Operation == nil {
		return true
	}
	o := v.Operation
	if o.Version != 1 {
		return false
	}
	if _, e := gymentry.ValidateIntent(o.Intent); e != nil {
		return false
	}
	switch o.State {
	case "prepared":
		return o.Receipt == nil && o.AbandonedAtMS == 0
	case "abandoned":
		return o.Receipt == nil && stableTime(o.AbandonedAtMS) && o.AbandonedAtMS >= o.Intent.PreparedAtMS
	case "committed":
		if o.Receipt == nil || o.AbandonedAtMS != 0 || !entryReceiptShape(*o.Receipt, descriptorID) {
			return false
		}
		r, i := o.Receipt, o.Intent
		if r.StoreID != v.StoreID || r.OperationID != i.OperationID || r.OperationSequence != i.Sequence || r.Generation != i.Generation || r.ViewID != i.ViewID || r.Event.EventID != i.Request.EventID || r.Event.Saturday != i.Request.Saturday || r.Action != i.Request.Action || r.PreviousRevision != i.Request.ExpectedRevision || r.RosterRevision != i.Request.ExpectedRosterRevision || r.CommittedAtMS < i.PreparedAtMS || len(r.Members) != len(i.Request.ProspectIDs) {
			return false
		}
		for n, m := range r.Members {
			if m.Assignment.ProspectID != i.Request.ProspectIDs[n] {
				return false
			}
		}
		return true
	}
	return false
}
func entryCheckShape(v gymentry.CheckResult, descriptorID string) bool {
	if v.Version != 1 || v.AsOfMS != v.Status.ObservedAtMS || v.RosterRevision != v.Status.CurrentRosterRevision || v.Members == nil || len(v.Members) != len(v.Status.SavedIDs) || len(v.Members) > 5 {
		return false
	}
	if _, e := gymentry.ValidateEventStatus(v.Status); e != nil {
		return false
	}
	slots := map[uint32]bool{}
	for n, m := range v.Members {
		if m.Slot > 10 || m.Slot != 0 && slots[m.Slot] || m.Item.Assignment.ProspectID != v.Status.SavedIDs[n] || !entryItemShape(m.Item, v.AsOfMS, descriptorID) {
			return false
		}
		want := "eligible"
		if m.Slot == 0 {
			want = "not_rostered"
		}
		if m.Item.Condition.Phase == "retired" {
			want = "retired"
		}
		if m.Eligibility != want {
			return false
		}
		raw, _ := json.Marshal(m)
		if len(raw) > gymentry.MaxCheckMemberSize {
			return false
		}
		if m.Slot != 0 {
			slots[m.Slot] = true
		}
	}
	return true
}
