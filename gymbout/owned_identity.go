// Copyright (c) 2026 The SLUG developers. ISC license.
package gymbout

import (
	"context"
	"encoding/json"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/internal/ownedcodec"
)

func CandidateOwnedPin(source gymcard.OwnedPin) (OwnedPin, error) {
	source, e := gymcard.ValidateOwnedPin(source)
	if e != nil {
		return OwnedPin{}, ErrInvalid
	}
	id, e := gymcard.OwnedID(source)
	if e != nil {
		return OwnedPin{}, ErrInvalid
	}
	return OwnedPin{1, id, source, RulesID()}, nil
}
func OwnedID(p OwnedPin) (string, error) {
	if ownedValidatePin(p) != nil {
		return "", ErrInvalid
	}
	return ownedcodec.Hash("bout store", p)
}
func ownedRecordDigest(v OwnedRecord) string { id, _ := ownedcodec.Hash("bout record", v); return id }
func OwnedRecordDigest(ctx context.Context, p OwnedPin, v OwnedRecord) (string, error) {
	if _, e := ValidateOwnedRecord(ctx, p, v); e != nil {
		return "", e
	}
	return ownedRecordDigest(v), nil
}
func ownedSourceDigest(v gymcard.OwnedBoutSource) string {
	id, _ := ownedcodec.Hash("card bout source", v)
	return id
}
func ownedValue[T any](v T, limit int, validate func(T) error) (T, error) {
	var z T
	if e := validate(v); e != nil {
		return z, e
	}
	b, e := json.Marshal(v)
	if e != nil || len(b) > limit {
		return z, ErrInvalid
	}
	if json.Unmarshal(b, &z) != nil {
		return z, ErrInvalid
	}
	return z, nil
}
func ownedDecode(raw []byte, limit int, out any, _ bool) error {
	if ownedcodec.Decode(raw, limit, out) != nil {
		return ErrInvalid
	}
	return nil
}
func ownedTimestamp(v int64) bool { return v > 0 && v <= 253402300799999 }
