// Copyright (c) 2026 The SLUG developers. ISC license.
package gymcard

import "encoding/json"

func ValidateOwnedPin(v OwnedPin) (OwnedPin, error) {
	return ownedValue(v, OwnedMaxPinSize, validateOwnedPin)
}
func MarshalOwnedPin(v OwnedPin) ([]byte, error) {
	v, e := ValidateOwnedPin(v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseOwnedPin(raw []byte) (OwnedPin, error) {
	var v OwnedPin
	if e := ownedDecode(raw, OwnedMaxPinSize, &v, false); e != nil {
		return v, e
	}
	return ValidateOwnedPin(v)
}
func ValidateOwnedInfo(p OwnedPin, v OwnedInfo) (OwnedInfo, error) {
	if _, e := ValidateOwnedPin(p); e != nil {
		return OwnedInfo{}, e
	}
	return ownedValue(v, OwnedMaxPinSize, func(v OwnedInfo) error { return validateOwnedInfo(p, v) })
}
func MarshalOwnedInfo(p OwnedPin, v OwnedInfo) ([]byte, error) {
	v, e := ValidateOwnedInfo(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseOwnedInfo(p OwnedPin, raw []byte) (OwnedInfo, error) {
	var v OwnedInfo
	if e := ownedDecode(raw, OwnedMaxPinSize, &v, false); e != nil {
		return v, e
	}
	return ValidateOwnedInfo(p, v)
}
func ValidateOwnedGymCounts(p OwnedPin, v GymCounts) (GymCounts, error) {
	if _, e := ValidateOwnedPin(p); e != nil {
		return GymCounts{}, e
	}
	return ownedValue(v, 1024, func(v GymCounts) error { return validateOwnedGymCounts(p, v) })
}
func MarshalOwnedGymCounts(p OwnedPin, v GymCounts) ([]byte, error) {
	v, e := ValidateOwnedGymCounts(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseOwnedGymCounts(p OwnedPin, raw []byte) (GymCounts, error) {
	var v GymCounts
	if e := ownedDecode(raw, 1024, &v, false); e != nil {
		return v, e
	}
	return ValidateOwnedGymCounts(p, v)
}
func ValidateOwnedEntrant(p OwnedPin, v Entrant) (Entrant, error) {
	if _, e := ValidateOwnedPin(p); e != nil {
		return Entrant{}, e
	}
	return ownedValue(v, 2048, func(v Entrant) error { return validateOwnedEntrant(p, v) })
}
func MarshalOwnedEntrant(p OwnedPin, v Entrant) ([]byte, error) {
	v, e := ValidateOwnedEntrant(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseOwnedEntrant(p OwnedPin, raw []byte) (Entrant, error) {
	var v Entrant
	if e := ownedDecode(raw, 2048, &v, false); e != nil {
		return v, e
	}
	return ValidateOwnedEntrant(p, v)
}
func ValidateOwnedPlan(p OwnedPin, v Plan) (Plan, error) {
	if _, e := ValidateOwnedPin(p); e != nil {
		return Plan{}, e
	}
	return ownedValue(v, MaxPlanSize, func(v Plan) error { return validateOwnedPlan(p, v) })
}
func MarshalOwnedPlan(p OwnedPin, v Plan) ([]byte, error) {
	v, e := ValidateOwnedPlan(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseOwnedPlan(p OwnedPin, raw []byte) (Plan, error) {
	var v Plan
	if e := ownedDecode(raw, MaxPlanSize, &v, false); e != nil {
		return v, e
	}
	return ValidateOwnedPlan(p, v)
}
func ValidateOwnedRecord(p OwnedPin, v OwnedRecord) (OwnedRecord, error) {
	if _, e := ValidateOwnedPin(p); e != nil {
		return OwnedRecord{}, e
	}
	return ownedValue(v, OwnedMaxRecordSize, func(v OwnedRecord) error { return validateOwnedRecord(p, v) })
}
func MarshalOwnedRecord(p OwnedPin, v OwnedRecord) ([]byte, error) {
	v, e := ValidateOwnedRecord(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseOwnedRecord(p OwnedPin, raw []byte) (OwnedRecord, error) {
	var v OwnedRecord
	if e := ownedDecode(raw, OwnedMaxRecordSize, &v, false); e != nil {
		return v, e
	}
	return ValidateOwnedRecord(p, v)
}
func ValidateOwnedStatus(p OwnedPin, v OwnedStatus) (OwnedStatus, error) {
	if _, e := ValidateOwnedPin(p); e != nil {
		return OwnedStatus{}, e
	}
	return ownedValue(v, MaxStatusSize, func(v OwnedStatus) error { return validateOwnedStatus(p, v) })
}
func MarshalOwnedStatus(p OwnedPin, v OwnedStatus) ([]byte, error) {
	v, e := ValidateOwnedStatus(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseOwnedStatus(p OwnedPin, raw []byte) (OwnedStatus, error) {
	var v OwnedStatus
	if e := ownedDecode(raw, MaxStatusSize, &v, false); e != nil {
		return v, e
	}
	return ValidateOwnedStatus(p, v)
}
func ValidateOwnedBuildResult(p OwnedPin, v OwnedBuildResult) (OwnedBuildResult, error) {
	if _, e := ValidateOwnedPin(p); e != nil {
		return OwnedBuildResult{}, e
	}
	return ownedValue(v, OwnedMaxResponseSize, func(v OwnedBuildResult) error { return validateOwnedBuildResult(p, v) })
}
func MarshalOwnedBuildResult(p OwnedPin, v OwnedBuildResult) ([]byte, error) {
	v, e := ValidateOwnedBuildResult(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseOwnedBuildResult(p OwnedPin, raw []byte) (OwnedBuildResult, error) {
	var v OwnedBuildResult
	if e := ownedDecode(raw, OwnedMaxResponseSize, &v, false); e != nil {
		return v, e
	}
	return ValidateOwnedBuildResult(p, v)
}
func ValidateOwnedReadResult(p OwnedPin, v OwnedReadResult) (OwnedReadResult, error) {
	if _, e := ValidateOwnedPin(p); e != nil {
		return OwnedReadResult{}, e
	}
	return ownedValue(v, OwnedMaxResponseSize, func(v OwnedReadResult) error { return validateOwnedReadResult(p, v) })
}
func MarshalOwnedReadResult(p OwnedPin, v OwnedReadResult) ([]byte, error) {
	v, e := ValidateOwnedReadResult(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseOwnedReadResult(p OwnedPin, raw []byte) (OwnedReadResult, error) {
	var v OwnedReadResult
	if e := ownedDecode(raw, OwnedMaxResponseSize, &v, false); e != nil {
		return v, e
	}
	return ValidateOwnedReadResult(p, v)
}
func ValidateOwnedHistoryPage(p OwnedPin, v OwnedHistoryPage) (OwnedHistoryPage, error) {
	if _, e := ValidateOwnedPin(p); e != nil {
		return OwnedHistoryPage{}, e
	}
	return ownedValue(v, MaxHistoryPageSize, func(v OwnedHistoryPage) error { return validateOwnedHistoryPage(p, v) })
}
func MarshalOwnedHistoryPage(p OwnedPin, v OwnedHistoryPage) ([]byte, error) {
	v, e := ValidateOwnedHistoryPage(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseOwnedHistoryPage(p OwnedPin, raw []byte) (OwnedHistoryPage, error) {
	var v OwnedHistoryPage
	if e := ownedDecode(raw, MaxHistoryPageSize, &v, false); e != nil {
		return v, e
	}
	return ValidateOwnedHistoryPage(p, v)
}

// ValidateOwnedBout validates the unchanged physical booking fields under an
// explicitly validated owned card pin. It grants no live source authority.
func ValidateOwnedBout(p OwnedPin, v Bout) (Bout, error) {
	if validateOwnedPin(p) != nil {
		return Bout{}, ErrInvalid
	}
	return ownedValue(v, 1024, func(v Bout) error { return validateBout(Pin{}, v) })
}
