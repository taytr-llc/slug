// Copyright (c) 2026 The SLUG developers. ISC license.
package gymbout

import (
	"context"
	"encoding/json"
)

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
		return v, e
	}
	return ValidatePin(v)
}
func ValidateInfo(p Pin, v Info) (Info, error) {
	if _, e := ValidatePin(p); e != nil {
		return Info{}, e
	}
	return owned(v, MaxInfoSize, func(v Info) error { return validateInfo(p, v) })
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
	if e := decode(raw, MaxInfoSize, &v, false); e != nil {
		return v, e
	}
	return ValidateInfo(p, v)
}
func ValidateSavedBout(p Pin, v SavedBout) (SavedBout, error) {
	if _, e := ValidatePin(p); e != nil {
		return SavedBout{}, e
	}
	return owned(v, MaxSavedBoutSize, func(v SavedBout) error { return validateSavedBout(p, v) })
}
func MarshalSavedBout(p Pin, v SavedBout) ([]byte, error) {
	v, e := ValidateSavedBout(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseSavedBout(p Pin, raw []byte) (SavedBout, error) {
	var v SavedBout
	if e := decode(raw, MaxSavedBoutSize, &v, false); e != nil {
		return v, e
	}
	return ValidateSavedBout(p, v)
}
func ValidateStatus(p Pin, v Status) (Status, error) {
	if _, e := ValidatePin(p); e != nil {
		return Status{}, e
	}
	return owned(v, MaxStatusSize, func(v Status) error { return validateStatus(p, v) })
}
func MarshalStatus(p Pin, v Status) ([]byte, error) {
	v, e := ValidateStatus(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseStatus(p Pin, raw []byte) (Status, error) {
	var v Status
	if e := decode(raw, MaxStatusSize, &v, false); e != nil {
		return v, e
	}
	return ValidateStatus(p, v)
}
func ValidateCardStatus(p Pin, v CardStatus) (CardStatus, error) {
	if _, e := ValidatePin(p); e != nil {
		return CardStatus{}, e
	}
	return owned(v, MaxCardStatusSize, func(v CardStatus) error { return validateCardStatus(p, v) })
}
func MarshalCardStatus(p Pin, v CardStatus) ([]byte, error) {
	v, e := ValidateCardStatus(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseCardStatus(p Pin, raw []byte) (CardStatus, error) {
	var v CardStatus
	if e := decode(raw, MaxCardStatusSize, &v, false); e != nil {
		return v, e
	}
	return ValidateCardStatus(p, v)
}
func ValidateRecord(ctx context.Context, p Pin, v Record) (Record, error) {
	if ctx == nil {
		return Record{}, ErrInvalid
	}
	if ctx.Err() != nil {
		return Record{}, ErrCancelled
	}
	if _, e := ValidatePin(p); e != nil {
		return Record{}, e
	}
	return owned(v, MaxRecordSize, func(v Record) error { return validateRecord(ctx, p, v) })
}
func MarshalRecord(ctx context.Context, p Pin, v Record) ([]byte, error) {
	v, e := ValidateRecord(ctx, p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseRecord(ctx context.Context, p Pin, raw []byte) (Record, error) {
	var v Record
	if e := decode(raw, MaxRecordSize, &v, false); e != nil {
		return v, e
	}
	return ValidateRecord(ctx, p, v)
}
func ValidateRunResult(ctx context.Context, p Pin, v RunResult) (RunResult, error) {
	if ctx == nil {
		return RunResult{}, ErrInvalid
	}
	if ctx.Err() != nil {
		return RunResult{}, ErrCancelled
	}
	if _, e := ValidatePin(p); e != nil {
		return RunResult{}, e
	}
	return owned(v, MaxRunResultSize, func(v RunResult) error { return validateRunResult(ctx, p, v) })
}
func MarshalRunResult(ctx context.Context, p Pin, v RunResult) ([]byte, error) {
	v, e := ValidateRunResult(ctx, p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseRunResult(ctx context.Context, p Pin, raw []byte) (RunResult, error) {
	var v RunResult
	if e := decode(raw, MaxRunResultSize, &v, false); e != nil {
		return v, e
	}
	return ValidateRunResult(ctx, p, v)
}
func ValidateReadResult(ctx context.Context, p Pin, v ReadResult) (ReadResult, error) {
	if ctx == nil {
		return ReadResult{}, ErrInvalid
	}
	if ctx.Err() != nil {
		return ReadResult{}, ErrCancelled
	}
	if _, e := ValidatePin(p); e != nil {
		return ReadResult{}, e
	}
	return owned(v, MaxReadResultSize, func(v ReadResult) error { return validateReadResult(ctx, p, v) })
}
func MarshalReadResult(ctx context.Context, p Pin, v ReadResult) ([]byte, error) {
	v, e := ValidateReadResult(ctx, p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseReadResult(ctx context.Context, p Pin, raw []byte) (ReadResult, error) {
	var v ReadResult
	if e := decode(raw, MaxReadResultSize, &v, false); e != nil {
		return v, e
	}
	return ValidateReadResult(ctx, p, v)
}
