// Copyright (c) 2026 The SLUG developers. ISC license.
package gymevent

import "encoding/json"

func ValidatePin(v Pin) (Pin, error) { return owned(v, MaxPinSize, validatePin) }
func MarshalPin(v Pin) ([]byte, error) {
	v, e := ValidatePin(v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParsePin(raw []byte) (Pin, error) {
	var v Pin
	if e := decode(raw, MaxPinSize, &v, false); e != nil {
		return Pin{}, e
	}
	return ValidatePin(v)
}
func ValidateInfo(p Pin, v Info) (Info, error) {
	if _, e := ValidatePin(p); e != nil {
		return Info{}, e
	}
	f := func(v Info) error { return validateInfo(p, v) }
	return owned(v, MaxMetadataSize, f)
}
func MarshalInfo(p Pin, v Info) ([]byte, error) {
	v, e := ValidateInfo(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseInfo(p Pin, raw []byte) (Info, error) {
	var v Info
	if e := decode(raw, MaxMetadataSize, &v, false); e != nil {
		return Info{}, e
	}
	return ValidateInfo(p, v)
}
func ValidateEvent(p Pin, v Event) (Event, error) {
	if _, e := ValidatePin(p); e != nil {
		return Event{}, e
	}
	f := func(v Event) error { return validateEvent(p, v) }
	return owned(v, MaxEventSize, f)
}
func MarshalEvent(p Pin, v Event) ([]byte, error) {
	v, e := ValidateEvent(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseEvent(p Pin, raw []byte) (Event, error) {
	var v Event
	if e := decode(raw, MaxEventSize, &v, false); e != nil {
		return Event{}, e
	}
	return ValidateEvent(p, v)
}
func ValidateParticipantSnapshot(p Pin, v ParticipantSnapshot) (ParticipantSnapshot, error) {
	if _, e := ValidatePin(p); e != nil {
		return ParticipantSnapshot{}, e
	}
	f := func(v ParticipantSnapshot) error { return validateParticipantSnapshot(p, v) }
	return owned(v, MaxParticipantSize, f)
}
func MarshalParticipantSnapshot(p Pin, v ParticipantSnapshot) ([]byte, error) {
	v, e := ValidateParticipantSnapshot(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseParticipantSnapshot(p Pin, raw []byte) (ParticipantSnapshot, error) {
	var v ParticipantSnapshot
	if e := decode(raw, MaxParticipantSize, &v, false); e != nil {
		return ParticipantSnapshot{}, e
	}
	return ValidateParticipantSnapshot(p, v)
}
func ValidateParticipantCapture(p Pin, v ParticipantCapture) (ParticipantCapture, error) {
	if _, e := ValidatePin(p); e != nil {
		return ParticipantCapture{}, e
	}
	f := func(v ParticipantCapture) error { return validateParticipantCapture(p, v) }
	return owned(v, MaxCaptureSize, f)
}
func MarshalParticipantCapture(p Pin, v ParticipantCapture) ([]byte, error) {
	v, e := ValidateParticipantCapture(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseParticipantCapture(p Pin, raw []byte) (ParticipantCapture, error) {
	var v ParticipantCapture
	if e := decode(raw, MaxCaptureSize, &v, false); e != nil {
		return ParticipantCapture{}, e
	}
	return ValidateParticipantCapture(p, v)
}
func ValidateSnapshotInput(p Pin, v SnapshotInput) (SnapshotInput, error) {
	if _, e := ValidatePin(p); e != nil {
		return SnapshotInput{}, e
	}
	f := func(v SnapshotInput) error { return validateSnapshotInput(p, v) }
	return owned(v, MaxInputSize, f)
}
func MarshalSnapshotInput(p Pin, v SnapshotInput) ([]byte, error) {
	v, e := ValidateSnapshotInput(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseSnapshotInput(p Pin, raw []byte) (SnapshotInput, error) {
	var v SnapshotInput
	if e := decode(raw, MaxInputSize, &v, false); e != nil {
		return SnapshotInput{}, e
	}
	return ValidateSnapshotInput(p, v)
}
func ValidateSnapshotReceipt(p Pin, v SnapshotReceipt) (SnapshotReceipt, error) {
	if _, e := ValidatePin(p); e != nil {
		return SnapshotReceipt{}, e
	}
	f := func(v SnapshotReceipt) error { return validateSnapshotReceipt(p, v) }
	return owned(v, MaxReceiptSize, f)
}
func MarshalSnapshotReceipt(p Pin, v SnapshotReceipt) ([]byte, error) {
	v, e := ValidateSnapshotReceipt(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseSnapshotReceipt(p Pin, raw []byte) (SnapshotReceipt, error) {
	var v SnapshotReceipt
	if e := decode(raw, MaxReceiptSize, &v, false); e != nil {
		return SnapshotReceipt{}, e
	}
	return ValidateSnapshotReceipt(p, v)
}
func ValidateEventStatus(p Pin, v EventStatus) (EventStatus, error) {
	if _, e := ValidatePin(p); e != nil {
		return EventStatus{}, e
	}
	f := func(v EventStatus) error { return validateEventStatus(p, v) }
	return owned(v, MaxEventSize, f)
}
func MarshalEventStatus(p Pin, v EventStatus) ([]byte, error) {
	v, e := ValidateEventStatus(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseEventStatus(p Pin, raw []byte) (EventStatus, error) {
	var v EventStatus
	if e := decode(raw, MaxEventSize, &v, false); e != nil {
		return EventStatus{}, e
	}
	return ValidateEventStatus(p, v)
}
func ValidateCaptureResult(p Pin, v CaptureResult) (CaptureResult, error) {
	if _, e := ValidatePin(p); e != nil {
		return CaptureResult{}, e
	}
	f := func(v CaptureResult) error { return validateCaptureResult(p, v) }
	return owned(v, MaxResponseSize, f)
}
func MarshalCaptureResult(p Pin, v CaptureResult) ([]byte, error) {
	v, e := ValidateCaptureResult(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseCaptureResult(p Pin, raw []byte) (CaptureResult, error) {
	var v CaptureResult
	if e := decode(raw, MaxResponseSize, &v, false); e != nil {
		return CaptureResult{}, e
	}
	return ValidateCaptureResult(p, v)
}
func ValidateHistoryPage(p Pin, v HistoryPage) (HistoryPage, error) {
	if _, e := ValidatePin(p); e != nil {
		return HistoryPage{}, e
	}
	f := func(v HistoryPage) error { return validateHistoryPage(p, v) }
	return owned(v, MaxResponseSize, f)
}
func MarshalHistoryPage(p Pin, v HistoryPage) ([]byte, error) {
	v, e := ValidateHistoryPage(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseHistoryPage(p Pin, raw []byte) (HistoryPage, error) {
	var v HistoryPage
	if e := decode(raw, MaxResponseSize, &v, false); e != nil {
		return HistoryPage{}, e
	}
	return ValidateHistoryPage(p, v)
}
