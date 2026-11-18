// Copyright (c) 2026 The SLUG developers. ISC license.
package gymbout

import (
	"encoding/json"
	"github.com/taytr-llc/slug/gymcard"
)

func validateResultSource(p Pin, v ResultSource) error {
	id, _ := ID(p)
	if v.Version != 1 || v.StoreID != id || !timestamp(v.StoreCreatedAtMS) || !timestamp(v.CreatedAtMS) || v.CreatedAtMS < v.StoreCreatedAtMS || v.CreatedAtMS < v.Source.CardCreatedAtMS || v.CreatedAtMS < v.Source.FinalizedAtMS || !hashID(v.RecordDigest) || !hashID(v.InputID) || !hashID(v.ResultDigest) || v.SourceDigest != sourceDigest(v.Source) {
		return ErrInvalid
	}
	if _, e := gymcard.ValidateBoutSource(p.SourcePin, v.Source); e != nil {
		return ErrInvalid
	}
	return validateSavedBout(p, SavedBout{Bout: v.Source.Bout, SourceDigest: v.SourceDigest, Exists: true, RecordDigest: v.RecordDigest, CreatedAtMS: v.CreatedAtMS, InputID: v.InputID, ResultDigest: v.ResultDigest, Winner: v.Summary.Winner, Method: v.Summary.Method, EndRound: v.Summary.EndRound, EndTick: v.Summary.EndTick})
}
func ValidateResultSource(p Pin, v ResultSource) (ResultSource, error) {
	if _, e := ValidatePin(p); e != nil {
		return ResultSource{}, e
	}
	return owned(v, MaxResultSourceSize, func(v ResultSource) error { return validateResultSource(p, v) })
}
func MarshalResultSource(p Pin, v ResultSource) ([]byte, error) {
	v, e := ValidateResultSource(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseResultSource(p Pin, b []byte) (ResultSource, error) {
	var v ResultSource
	if e := decode(b, MaxResultSourceSize, &v, false); e != nil {
		return v, e
	}
	return ValidateResultSource(p, v)
}
func resultSourceDigest(v ResultSource) string {
	b, _ := json.Marshal(v)
	return hash("SLUG candidate result source\x00v1\x00", lp(b))
}
func ResultSourceDigest(p Pin, v ResultSource) (string, error) {
	v, e := ValidateResultSource(p, v)
	if e != nil {
		return "", e
	}
	return resultSourceDigest(v), nil
}
