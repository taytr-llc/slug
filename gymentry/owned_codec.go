// Copyright (c) 2026 The SLUG developers. ISC license.
package gymentry

import (
	"encoding/json"
	"github.com/taytr-llc/slug/fighterownership"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/internal/ownedcodec"
)

func ValidateOwnedPin(p OwnedPin) (OwnedPin, error) {
	if p.Version != 1 || p.CalendarID != CalendarID() || p.RulesID != RulesID() {
		return OwnedPin{}, ErrInvalid
	}
	sp, e := gymstable.ValidateOwnedPin(p.StablePin)
	if e != nil {
		return OwnedPin{}, ErrInvalid
	}
	sid, _ := gymstable.OwnedID(sp)
	ns, _ := NamespaceID(sp.Stable.Identity.Network, sp.Stable.Identity.Genesis, sp.Stable.AccountAuthorityID)
	if p.StableStoreID != sid || p.NamespaceID != ns || !ownedcodec.Fits(p, OwnedMaxPinSize) {
		return OwnedPin{}, ErrInvalid
	}
	return p, nil
}
func OwnedID(p OwnedPin) (string, error) {
	if _, e := ValidateOwnedPin(p); e != nil {
		return "", e
	}
	return ownedcodec.Hash("entry store", p)
}
func OwnedEntryID(store, event, gym string) (string, error) {
	if !hashID(store) || !hashID(event) || !hashID(gym) {
		return "", ErrInvalid
	}
	return ownedcodec.Hash("entry", struct {
		Version uint32 `json:"version"`
		StoreID string `json:"store_id"`
		EventID string `json:"event_id"`
		GymID   string `json:"gym_id"`
	}{1, store, event, gym})
}
func OwnedEventForDate(p OwnedPin, date string) (Event, error) {
	if _, e := ValidateOwnedPin(p); e != nil {
		return Event{}, e
	}
	return eventForNamespace(p.NamespaceID, date)
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
func ValidateOwnedReceipt(p OwnedPin, r OwnedReceipt) (OwnedReceipt, error) {
	sid, e := OwnedID(p)
	if e != nil {
		return OwnedReceipt{}, e
	}
	ev, e := OwnedEventForDate(p, r.Event.Saturday)
	eid, _ := OwnedEntryID(sid, ev.EventID, p.StablePin.Stable.Identity.GymID)
	if e != nil || r.Version != 1 || r.StoreID != sid || r.EntryID != eid || r.Event != ev || !hashID(r.OperationID) || !hashID(r.ViewID) || r.OperationSequence < 1 || r.OperationSequence > MaxOperations || r.Generation < 1 || r.Generation > MaxSafeInteger || r.Revision != r.PreviousRevision+1 || r.Revision > r.OperationSequence || r.RosterRevision > gymstable.MaxOperations || fighterownership.ValidateHead(p.StablePin.Ownership, r.OwnershipHead) != nil || !timestamp(r.CommittedAtMS) || r.CommittedAtMS >= ev.CutoffMS || r.Members == nil || len(r.Members) > 5 {
		return OwnedReceipt{}, ErrInvalid
	}
	if r.Action == "withdraw" {
		if len(r.Members) != 0 || r.RosterRevision != 0 {
			return OwnedReceipt{}, ErrInvalid
		}
	} else if r.Action != "replace" || len(r.Members) == 0 {
		return OwnedReceipt{}, ErrInvalid
	}
	classes, slots := map[string]bool{}, map[uint32]bool{}
	for n, m := range r.Members {
		if m.Slot < 1 || m.Slot > 10 || slots[m.Slot] || classes[m.Class] || n > 0 && r.Members[n-1].Birth.ProspectID >= m.Birth.ProspectID || m.Birth.DescriptorID != p.StablePin.Stable.Source.DescriptorID || m.Birth.Sequence > uint64(r.OwnershipHead.AdmissionSequence) || fighterownership.ValidateEffectiveState(p.StablePin.Ownership, m.Ownership) != nil || m.Ownership.ProspectID != m.Birth.ProspectID || m.Ownership.OwnerPublicKey != p.StablePin.Stable.Identity.GymPublicKey {
			return OwnedReceipt{}, ErrInvalid
		}
		manifest, condition, e := memberCondition(m.Birth, r.CommittedAtMS)
		if e != nil || manifest.Physical.Class != m.Class || condition != m.ConditionAtSave || condition.Phase == "retired" || !ownedcodec.Fits(m, OwnedMaxMemberSize) {
			return OwnedReceipt{}, ErrInvalid
		}
		classes[m.Class] = true
		slots[m.Slot] = true
	}
	if !ownedcodec.Fits(r, OwnedMaxReceiptSize) {
		return OwnedReceipt{}, ErrInvalid
	}
	raw, _ := json.Marshal(r)
	var out OwnedReceipt
	if json.Unmarshal(raw, &out) != nil {
		return OwnedReceipt{}, ErrInvalid
	}
	return out, nil
}
func MarshalOwnedReceipt(p OwnedPin, r OwnedReceipt) ([]byte, error) {
	r, e := ValidateOwnedReceipt(p, r)
	if e != nil {
		return nil, e
	}
	return json.Marshal(r)
}
func ParseOwnedReceipt(p OwnedPin, raw []byte) (OwnedReceipt, error) {
	var r OwnedReceipt
	if ownedcodec.Decode(raw, OwnedMaxReceiptSize, &r) != nil {
		return r, ErrInvalid
	}
	return ValidateOwnedReceipt(p, r)
}
func OwnedReceiptDigest(p OwnedPin, r OwnedReceipt) (string, error) {
	if _, e := ValidateOwnedReceipt(p, r); e != nil {
		return "", e
	}
	return ownedcodec.Hash("entry receipt", r)
}
func ValidateOwnedOperation(p OwnedPin, o OwnedOperation) (OwnedOperation, error) {
	if _, e := ValidateOwnedPin(p); e != nil {
		return OwnedOperation{}, e
	}
	i := o.Intent
	if o.Version != 1 || validateIntent(Intent(i)) != nil {
		return OwnedOperation{}, ErrInvalid
	}
	ev, e := OwnedEventForDate(p, i.Request.Saturday)
	if e != nil || ev.EventID != i.Request.EventID {
		return OwnedOperation{}, ErrInvalid
	}
	switch o.State {
	case "prepared":
		if o.Receipt != nil || o.AbandonedAtMS != 0 {
			return OwnedOperation{}, ErrInvalid
		}
	case "abandoned":
		if o.Receipt != nil || !timestamp(o.AbandonedAtMS) || o.AbandonedAtMS < i.PreparedAtMS {
			return OwnedOperation{}, ErrInvalid
		}
	case "committed":
		if o.Receipt == nil || o.AbandonedAtMS != 0 {
			return OwnedOperation{}, ErrInvalid
		}
		r, e := ValidateOwnedReceipt(p, *o.Receipt)
		if e != nil || r.OperationID != i.OperationID || r.OperationSequence != i.Sequence || r.Generation != i.Generation || r.ViewID != i.ViewID || r.Event != ev || r.Action != i.Request.Action || r.PreviousRevision != i.Request.ExpectedRevision || r.RosterRevision != i.Request.ExpectedRosterRevision || r.CommittedAtMS < i.PreparedAtMS || len(r.Members) != len(i.Request.ProspectIDs) {
			return OwnedOperation{}, ErrInvalid
		}
		for n, m := range r.Members {
			if m.Birth.ProspectID != i.Request.ProspectIDs[n] {
				return OwnedOperation{}, ErrInvalid
			}
		}
		o.Receipt = &r
	default:
		return OwnedOperation{}, ErrInvalid
	}
	if !ownedcodec.Fits(o, OwnedMaxOperationSize) {
		return OwnedOperation{}, ErrInvalid
	}
	o.Intent.Request.ProspectIDs = append([]string{}, i.Request.ProspectIDs...)
	return o, nil
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
func MarshalOwnedIntent(i OwnedIntent) ([]byte, error) {
	if validateIntent(Intent(i)) != nil {
		return nil, ErrInvalid
	}
	return json.Marshal(i)
}
func ParseOwnedIntent(raw []byte) (OwnedIntent, error) {
	var i OwnedIntent
	if ownedcodec.Decode(raw, 8192, &i) != nil || validateIntent(Intent(i)) != nil {
		return i, ErrInvalid
	}
	return i, nil
}
