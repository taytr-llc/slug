// Copyright (c) 2026 The SLUG developers. ISC license.
package gymseed

import (
	"encoding/json"
	"github.com/taytr-llc/slug/internal/ownedcodec"
)

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
	if ownedcodec.Decode(raw, OwnedMaxPinSize, &v) != nil {
		return v, ErrInvalid
	}
	return ValidateOwnedPin(v)
}
func ValidateOwnedInfo(p OwnedPin, v OwnedInfo) (OwnedInfo, error) {
	if validateOwnedPin(p) != nil {
		return OwnedInfo{}, ErrInvalid
	}
	return ownedValue(v, OwnedMaxPinSize, func(x OwnedInfo) error { return validateOwnedInfo(p, x) })
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
	if ownedcodec.Decode(raw, OwnedMaxPinSize, &v) != nil {
		return v, ErrInvalid
	}
	return ValidateOwnedInfo(p, v)
}
func ValidateOwnedDeclaration(p OwnedPin, v OwnedDeclaration) (OwnedDeclaration, error) {
	if validateOwnedPin(p) != nil {
		return OwnedDeclaration{}, ErrInvalid
	}
	return ownedValue(v, MaxDeclarationSize, func(x OwnedDeclaration) error { return validateOwnedDeclaration(p, x) })
}
func MarshalOwnedDeclaration(p OwnedPin, v OwnedDeclaration) ([]byte, error) {
	v, e := ValidateOwnedDeclaration(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseOwnedDeclaration(p OwnedPin, raw []byte) (OwnedDeclaration, error) {
	var v OwnedDeclaration
	if ownedcodec.Decode(raw, MaxDeclarationSize, &v) != nil {
		return v, ErrInvalid
	}
	return ValidateOwnedDeclaration(p, v)
}
func ValidateOwnedSignedDeclaration(p OwnedPin, v OwnedSignedDeclaration) (OwnedSignedDeclaration, error) {
	if validateOwnedPin(p) != nil {
		return OwnedSignedDeclaration{}, ErrInvalid
	}
	return ownedValue(v, MaxSignedDeclarationSize, func(x OwnedSignedDeclaration) error { return validateOwnedSignedDeclaration(p, x) })
}
func MarshalOwnedSignedDeclaration(p OwnedPin, v OwnedSignedDeclaration) ([]byte, error) {
	v, e := ValidateOwnedSignedDeclaration(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseOwnedSignedDeclaration(p OwnedPin, raw []byte) (OwnedSignedDeclaration, error) {
	var v OwnedSignedDeclaration
	if ownedcodec.Decode(raw, MaxSignedDeclarationSize, &v) != nil {
		return v, ErrInvalid
	}
	return ValidateOwnedSignedDeclaration(p, v)
}
func ValidateOwnedConflict(p OwnedPin, v OwnedConflict) (OwnedConflict, error) {
	if validateOwnedPin(p) != nil {
		return OwnedConflict{}, ErrInvalid
	}
	return ownedValue(v, MaxConflictSize, func(x OwnedConflict) error { return validateOwnedConflict(p, x) })
}
func MarshalOwnedConflict(p OwnedPin, v OwnedConflict) ([]byte, error) {
	v, e := ValidateOwnedConflict(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseOwnedConflict(p OwnedPin, raw []byte) (OwnedConflict, error) {
	var v OwnedConflict
	if ownedcodec.Decode(raw, MaxConflictSize, &v) != nil {
		return v, ErrInvalid
	}
	return ValidateOwnedConflict(p, v)
}
func ValidateOwnedRecord(p OwnedPin, v OwnedRecord) (OwnedRecord, error) {
	if validateOwnedPin(p) != nil {
		return OwnedRecord{}, ErrInvalid
	}
	return ownedValue(v, OwnedMaxRecordSize, func(x OwnedRecord) error { return validateOwnedRecord(p, x) })
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
	if ownedcodec.Decode(raw, OwnedMaxRecordSize, &v) != nil {
		return v, ErrInvalid
	}
	return ValidateOwnedRecord(p, v)
}
func ValidateOwnedStatus(p OwnedPin, v OwnedStatus) (OwnedStatus, error) {
	if validateOwnedPin(p) != nil {
		return OwnedStatus{}, ErrInvalid
	}
	return ownedValue(v, MaxStatusSize, func(x OwnedStatus) error { return validateOwnedStatus(p, x) })
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
	if ownedcodec.Decode(raw, MaxStatusSize, &v) != nil {
		return v, ErrInvalid
	}
	return ValidateOwnedStatus(p, v)
}
func ValidateOwnedHistoryPage(p OwnedPin, v OwnedHistoryPage) (OwnedHistoryPage, error) {
	if validateOwnedPin(p) != nil {
		return OwnedHistoryPage{}, ErrInvalid
	}
	return ownedValue(v, OwnedMaxResponseSize, func(x OwnedHistoryPage) error { return validateOwnedHistoryPage(p, x) })
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
	if ownedcodec.Decode(raw, OwnedMaxResponseSize, &v) != nil {
		return v, ErrInvalid
	}
	return ValidateOwnedHistoryPage(p, v)
}
