// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"encoding/json"
	"github.com/taytr-llc/slug/fightercondition"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/fightermanifest"
	"github.com/taytr-llc/slug/fighterownership"
	"github.com/taytr-llc/slug/fighterportrait"
	"github.com/taytr-llc/slug/internal/careerstate"
	"github.com/taytr-llc/slug/internal/ownedcodec"
	"github.com/taytr-llc/slug/registry"
)

func ownedTime(n int64) bool { return n > 0 && n <= fighterownership.MaxTimestampMS }
func ValidateOwnedPin(p OwnedPin) (OwnedPin, error) {
	if p.Version != 1 || p.Stable.Version != SharedPinVersion2 {
		return OwnedPin{}, ErrInvalid
	}
	if _, e := ValidatePin(p.Stable); e != nil {
		return OwnedPin{}, e
	}
	if _, e := fighterownership.ValidatePin(p.Ownership); e != nil || p.Ownership.Source != p.Stable.Source || !ownedcodec.Fits(p, OwnedMaxPinSize) {
		return OwnedPin{}, ErrInvalid
	}
	return p, nil
}
func OwnedID(p OwnedPin) (string, error) {
	if _, e := ValidateOwnedPin(p); e != nil {
		return "", e
	}
	return ownedcodec.Hash("stable store", p)
}
func MarshalOwnedPin(p OwnedPin) ([]byte, error) {
	if _, e := ValidateOwnedPin(p); e != nil {
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
func ValidateOwnedStatus(s OwnedStatus) (OwnedStatus, error) {
	id, e := OwnedID(s.Pin)
	if e != nil || s.Version != 1 || s.StoreID != id || fighterownership.ValidateHead(s.Pin.Ownership, s.Head) != nil || s.OperationCount > MaxOperations || s.RosterRevision > uint64(s.OperationCount) || !validSlots(s.Slots) || s.LastAsOfMS < 0 || s.LastAsOfMS > fighterownership.MaxTimestampMS || s.PendingOperationID != "" && !hashID(s.PendingOperationID) || s.LastOperationID != "" && !hashID(s.LastOperationID) || (s.OperationCount == 0) != (s.LastOperationID == "") || s.PendingOperationID != "" && s.PendingOperationID != s.LastOperationID || !ownedcodec.Fits(s, OwnedMaxStatusSize) {
		return OwnedStatus{}, ErrInvalid
	}
	return s, nil
}
func MarshalOwnedStatus(s OwnedStatus) ([]byte, error) {
	if _, e := ValidateOwnedStatus(s); e != nil {
		return nil, e
	}
	return json.Marshal(s)
}
func ParseOwnedStatus(raw []byte) (OwnedStatus, error) {
	var s OwnedStatus
	if ownedcodec.Decode(raw, OwnedMaxStatusSize, &s) != nil {
		return s, ErrInvalid
	}
	return ValidateOwnedStatus(s)
}
func ValidateOwnedSlotRequest(p OwnedPin, r OwnedSlotRequest) (OwnedSlotRequest, error) {
	if _, e := ValidateOwnedPin(p); e != nil {
		return OwnedSlotRequest{}, e
	}
	if r.ExpectedRevision > MaxOperations || r.Slot < 1 || r.Slot > 10 || (r.ProspectID == "") != (r.ExpectedStateID == "") || r.ProspectID != "" && (!hashID(r.ProspectID) || !hashID(r.ExpectedStateID)) || fighterownership.ValidateHead(p.Ownership, r.Head) != nil || !ownedcodec.Fits(r, 4096) {
		return OwnedSlotRequest{}, ErrInvalid
	}
	return r, nil
}
func MarshalOwnedSlotRequest(p OwnedPin, r OwnedSlotRequest) ([]byte, error) {
	if _, e := ValidateOwnedSlotRequest(p, r); e != nil {
		return nil, e
	}
	return json.Marshal(r)
}
func ParseOwnedSlotRequest(p OwnedPin, raw []byte) (OwnedSlotRequest, error) {
	var r OwnedSlotRequest
	if ownedcodec.Decode(raw, 4096, &r) != nil {
		return r, ErrInvalid
	}
	return ValidateOwnedSlotRequest(p, r)
}
func ValidateOwnedOperation(p OwnedPin, o OwnedOperation) (OwnedOperation, error) {
	i := o.Intent
	if o.Version != 1 || i.Version != 1 || i.Sequence < 1 || i.Sequence > MaxOperations || !hashID(i.OperationID) || i.Generation == 0 || i.Generation > MaxSafeInteger || !hashID(i.ViewID) || !ownedTime(i.PreparedAtMS) {
		return OwnedOperation{}, ErrInvalid
	}
	if _, e := ValidateOwnedSlotRequest(p, i.Request); e != nil {
		return OwnedOperation{}, e
	}
	switch o.State {
	case "prepared":
		if o.Receipt != nil || o.AbandonedAtMS != 0 {
			return OwnedOperation{}, ErrInvalid
		}
	case "abandoned":
		if o.Receipt != nil || o.AbandonedAtMS < i.PreparedAtMS || !ownedTime(o.AbandonedAtMS) {
			return OwnedOperation{}, ErrInvalid
		}
	case "committed":
		r := o.Receipt
		if r == nil || r.Version != 1 || r.OperationID != i.OperationID || r.Generation != i.Generation || r.ViewID != i.ViewID || r.Request != i.Request || r.PreviousProspectID != "" && !hashID(r.PreviousProspectID) || r.Changed != (r.PreviousProspectID != i.Request.ProspectID) || r.PreviousRevision != i.Request.ExpectedRevision || r.Revision != r.PreviousRevision+uint64(boolInt(r.Changed)) || !validSlots(r.Slots) || r.Slots[i.Request.Slot-1] != i.Request.ProspectID || r.CommittedAtMS < i.PreparedAtMS || !ownedTime(r.CommittedAtMS) || o.AbandonedAtMS != 0 {
			return OwnedOperation{}, ErrInvalid
		}
	default:
		return OwnedOperation{}, ErrInvalid
	}
	if !ownedcodec.Fits(o, OwnedMaxOperationSize) {
		return OwnedOperation{}, ErrInvalid
	}
	b, _ := json.Marshal(o)
	var out OwnedOperation
	if json.Unmarshal(b, &out) != nil {
		return OwnedOperation{}, ErrInvalid
	}
	return out, nil
}
func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
func MarshalOwnedOperation(p OwnedPin, o OwnedOperation) ([]byte, error) {
	o, e := ValidateOwnedOperation(p, o)
	if e != nil {
		return nil, e
	}
	return json.Marshal(o)
}
func ParseOwnedOperation(p OwnedPin, raw []byte) (OwnedOperation, error) {
	var o OwnedOperation
	if ownedcodec.Decode(raw, OwnedMaxOperationSize, &o) != nil {
		return o, ErrInvalid
	}
	return ValidateOwnedOperation(p, o)
}

// ownedBirth preserves the actual authenticated birth gym. It never substitutes
// the current owner's account or identity into an old assignment validator.
func ownedBirth(p OwnedPin, a fighterownership.AdmissionStatement) (Assignment, error) {
	if registry.ValidatePublicAdmission(p.Ownership, a) != nil {
		return Assignment{}, ErrInvalid
	}
	r := a.Record
	d, e := sourceRecordDigest(r)
	if e != nil {
		return Assignment{}, e
	}
	return Assignment{Version: 1, Sequence: r.Sequence, ProspectID: r.Verified.ProspectID, GymID: r.Verified.GymID, WorkHash: r.Verified.WorkHash, BornAtMS: r.Birth.BornAtMS, AcceptedAtMS: r.Birth.AcceptedAtMS, DescriptorID: r.StaticBinding.DescriptorID, ManifestDigest: r.StaticBinding.ManifestDigest, PortraitDigest: r.StaticBinding.PortraitDigest, SourceRecordDigest: d}, nil
}

// ValidateOwnedBirth checks the unchanged birth attribution against the supplied
// signed admission. It is a pure consistency check, not a source/read capability.
func ValidateOwnedBirth(p OwnedPin, a fighterownership.AdmissionStatement, birth Assignment) error {
	if _, e := ValidateOwnedPin(p); e != nil {
		return ErrInvalid
	}
	want, e := ownedBirth(p, a)
	if e != nil || birth != want {
		return ErrInvalid
	}
	return nil
}
func ownedState(p OwnedPin, a fighterownership.AdmissionStatement, s fighterownership.EffectiveState) error {
	id, e := registry.PublicAdmissionID(p.Ownership, a)
	if e != nil || fighterownership.ValidateEffectiveState(p.Ownership, s) != nil || s.ProspectID != a.Record.Verified.ProspectID || s.BirthAdmissionID != id {
		return ErrInvalid
	}
	if s.Generation == 0 && s.OwnerPublicKey != a.Record.Authorization.PublicKey {
		return ErrInvalid
	}
	return nil
}
func ownedAppearance(p OwnedPin, a fighterownership.AdmissionStatement) (Assignment, fightermanifest.Manifest, error) {
	b, e := ownedBirth(p, a)
	if e != nil {
		return b, fightermanifest.Manifest{}, e
	}
	d, e := fighterdescriptor.ResolveCandidate(p.Stable.Source.DescriptorID)
	if e != nil {
		return b, fightermanifest.Manifest{}, ErrInvalid
	}
	m, e := fightermanifest.GenerateText(d.ManifestVersion, b.WorkHash)
	if e != nil || m.DNA != a.Record.Verified.DNA {
		return b, m, ErrInvalid
	}
	md, e := fightermanifest.Digest(m)
	if e != nil || md != b.ManifestDigest {
		return b, m, ErrInvalid
	}
	pd, e := fighterportrait.Digest(d.RendererVersion, m)
	if e != nil || pd != b.PortraitDigest {
		return b, m, ErrInvalid
	}
	return b, m, nil
}
func ValidateOwnedItem(p OwnedPin, i OwnedItem, asOf int64) (OwnedItem, error) {
	if _, e := ValidateOwnedPin(p); e != nil {
		return OwnedItem{}, e
	}
	if !ownedTime(asOf) || ownedState(p, i.Admission, i.Ownership) != nil || i.Ownership.OwnerPublicKey != p.Stable.Identity.GymPublicKey {
		return OwnedItem{}, ErrInvalid
	}
	b, m, e := ownedAppearance(p, i.Admission)
	if e != nil || b != i.Birth || m != i.Manifest || b.AcceptedAtMS > asOf || careerstate.ValidateActivation(i.Career.Activation) != nil || !hashID(i.Career.CareerDigest) || !historyTotalsValid(i.Career.Totals, i.Career.Totals.Wins+i.Career.Totals.Losses+i.Career.Totals.Draws) || i.Career.CurrentGymWins > i.Career.Totals.Wins {
		return OwnedItem{}, ErrInvalid
	}
	c, e := fightercondition.Project(1, m.DNA, b.BornAtMS, asOf, int(i.Career.Totals.XP))
	if e != nil || c != i.Condition || !ownedcodec.Fits(i, OwnedMaxItemSize) {
		return OwnedItem{}, ErrInvalid
	}
	raw, _ := json.Marshal(i)
	var out OwnedItem
	if json.Unmarshal(raw, &out) != nil {
		return OwnedItem{}, ErrInvalid
	}
	return out, nil
}
func MarshalOwnedItem(p OwnedPin, i OwnedItem, asOf int64) ([]byte, error) {
	i, e := ValidateOwnedItem(p, i, asOf)
	if e != nil {
		return nil, e
	}
	return json.Marshal(i)
}
func ParseOwnedItem(p OwnedPin, raw []byte, asOf int64) (OwnedItem, error) {
	var i OwnedItem
	if ownedcodec.Decode(raw, OwnedMaxItemSize, &i) != nil {
		return i, ErrInvalid
	}
	return ValidateOwnedItem(p, i, asOf)
}
func ValidateOwnedPage(p OwnedPage) (OwnedPage, error) {
	if _, e := ValidateOwnedStatus(p.Status); e != nil {
		return OwnedPage{}, e
	}
	if p.Version != 1 || p.Head != p.Status.Head || !ownedTime(p.AsOfMS) || p.After > 1000 || p.NextAfter > 1000 || p.Items == nil || len(p.Items) > 8 {
		return OwnedPage{}, ErrInvalid
	}
	last := p.After
	out := p
	out.Items = make([]OwnedItem, 0, len(p.Items))
	for _, i := range p.Items {
		v, e := ValidateOwnedItem(p.Status.Pin, i, p.AsOfMS)
		if e != nil || v.Birth.Sequence <= uint64(last) || v.Birth.Sequence > 1000 {
			return OwnedPage{}, ErrInvalid
		}
		last = uint32(v.Birth.Sequence)
		out.Items = append(out.Items, v)
	}
	if last != p.NextAfter || len(p.Items) == 0 && p.HasMore || !ownedcodec.Fits(out, OwnedMaxPageSize) {
		return OwnedPage{}, ErrInvalid
	}
	return out, nil
}
func MarshalOwnedPage(p OwnedPage) ([]byte, error) {
	p, e := ValidateOwnedPage(p)
	if e != nil {
		return nil, e
	}
	return json.Marshal(p)
}
func ParseOwnedPage(raw []byte) (OwnedPage, error) {
	var p OwnedPage
	if ownedcodec.Decode(raw, OwnedMaxPageSize, &p) != nil {
		return p, ErrInvalid
	}
	return ValidateOwnedPage(p)
}
