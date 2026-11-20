// Copyright (c) 2026 The SLUG developers. ISC license.
package gymbout

import (
	"context"
	"encoding/json"
)

func ValidateOwnedPin(v OwnedPin) (OwnedPin, error) {
	return ownedValue(v, OwnedMaxPinSize, ownedValidatePin)
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
	return ownedValue(v, OwnedMaxMetadataSize, func(v OwnedInfo) error { return ownedValidateInfo(p, v) })
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
	if e := ownedDecode(raw, OwnedMaxMetadataSize, &v, false); e != nil {
		return v, e
	}
	return ValidateOwnedInfo(p, v)
}
func ValidateOwnedSavedBout(p OwnedPin, v OwnedSavedBout) (OwnedSavedBout, error) {
	if _, e := ValidateOwnedPin(p); e != nil {
		return OwnedSavedBout{}, e
	}
	return ownedValue(v, MaxSavedBoutSize, func(v OwnedSavedBout) error { return ownedValidateSavedBout(p, v) })
}
func MarshalOwnedSavedBout(p OwnedPin, v OwnedSavedBout) ([]byte, error) {
	v, e := ValidateOwnedSavedBout(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseOwnedSavedBout(p OwnedPin, raw []byte) (OwnedSavedBout, error) {
	var v OwnedSavedBout
	if e := ownedDecode(raw, MaxSavedBoutSize, &v, false); e != nil {
		return v, e
	}
	return ValidateOwnedSavedBout(p, v)
}
func ValidateOwnedStatus(p OwnedPin, v OwnedStatus) (OwnedStatus, error) {
	if _, e := ValidateOwnedPin(p); e != nil {
		return OwnedStatus{}, e
	}
	return ownedValue(v, MaxStatusSize, func(v OwnedStatus) error { return ownedValidateStatus(p, v) })
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
func ValidateOwnedCardStatus(p OwnedPin, v OwnedCardStatus) (OwnedCardStatus, error) {
	if _, e := ValidateOwnedPin(p); e != nil {
		return OwnedCardStatus{}, e
	}
	return ownedValue(v, MaxCardStatusSize, func(v OwnedCardStatus) error { return ownedValidateCardStatus(p, v) })
}
func MarshalOwnedCardStatus(p OwnedPin, v OwnedCardStatus) ([]byte, error) {
	v, e := ValidateOwnedCardStatus(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseOwnedCardStatus(p OwnedPin, raw []byte) (OwnedCardStatus, error) {
	var v OwnedCardStatus
	if e := ownedDecode(raw, MaxCardStatusSize, &v, false); e != nil {
		return v, e
	}
	return ValidateOwnedCardStatus(p, v)
}
func ValidateOwnedRecord(ctx context.Context, p OwnedPin, v OwnedRecord) (OwnedRecord, error) {
	if ctx == nil {
		return OwnedRecord{}, ErrInvalid
	}
	if ctx.Err() != nil {
		return OwnedRecord{}, ErrCancelled
	}
	if _, e := ValidateOwnedPin(p); e != nil {
		return OwnedRecord{}, e
	}
	return ownedValue(v, OwnedMaxRecordSize, func(v OwnedRecord) error { return ownedValidateRecord(ctx, p, v) })
}
func MarshalOwnedRecord(ctx context.Context, p OwnedPin, v OwnedRecord) ([]byte, error) {
	v, e := ValidateOwnedRecord(ctx, p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseOwnedRecord(ctx context.Context, p OwnedPin, raw []byte) (OwnedRecord, error) {
	var v OwnedRecord
	if e := ownedDecode(raw, OwnedMaxRecordSize, &v, false); e != nil {
		return v, e
	}
	return ValidateOwnedRecord(ctx, p, v)
}
func ValidateOwnedRunResult(ctx context.Context, p OwnedPin, v OwnedRunResult) (OwnedRunResult, error) {
	if ctx == nil {
		return OwnedRunResult{}, ErrInvalid
	}
	if ctx.Err() != nil {
		return OwnedRunResult{}, ErrCancelled
	}
	if _, e := ValidateOwnedPin(p); e != nil {
		return OwnedRunResult{}, e
	}
	return ownedValue(v, OwnedMaxResponseSize, func(v OwnedRunResult) error { return ownedValidateRunResult(ctx, p, v) })
}
func MarshalOwnedRunResult(ctx context.Context, p OwnedPin, v OwnedRunResult) ([]byte, error) {
	v, e := ValidateOwnedRunResult(ctx, p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseOwnedRunResult(ctx context.Context, p OwnedPin, raw []byte) (OwnedRunResult, error) {
	var v OwnedRunResult
	if e := ownedDecode(raw, OwnedMaxResponseSize, &v, false); e != nil {
		return v, e
	}
	return ValidateOwnedRunResult(ctx, p, v)
}
func ValidateOwnedReadResult(ctx context.Context, p OwnedPin, v OwnedReadResult) (OwnedReadResult, error) {
	if ctx == nil {
		return OwnedReadResult{}, ErrInvalid
	}
	if ctx.Err() != nil {
		return OwnedReadResult{}, ErrCancelled
	}
	if _, e := ValidateOwnedPin(p); e != nil {
		return OwnedReadResult{}, e
	}
	return ownedValue(v, OwnedMaxResponseSize, func(v OwnedReadResult) error { return ownedValidateReadResult(ctx, p, v) })
}
func MarshalOwnedReadResult(ctx context.Context, p OwnedPin, v OwnedReadResult) ([]byte, error) {
	v, e := ValidateOwnedReadResult(ctx, p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseOwnedReadResult(ctx context.Context, p OwnedPin, raw []byte) (OwnedReadResult, error) {
	var v OwnedReadResult
	if e := ownedDecode(raw, OwnedMaxResponseSize, &v, false); e != nil {
		return v, e
	}
	return ValidateOwnedReadResult(ctx, p, v)
}
