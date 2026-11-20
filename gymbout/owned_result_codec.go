// Copyright (c) 2026 The SLUG developers. ISC license.
package gymbout

import (
	"encoding/json"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/internal/ownedcodec"
)

func ownedValidateResultSource(p OwnedPin, v OwnedResultSource) error {
	id, _ := OwnedID(p)
	if v.Version != 1 || v.StoreID != id || !ownedTimestamp(v.StoreCreatedAtMS) || !ownedTimestamp(v.CreatedAtMS) || v.CreatedAtMS < v.StoreCreatedAtMS || v.CreatedAtMS < v.Source.CardCreatedAtMS || v.CreatedAtMS < v.Source.FinalizedAtMS || !hashID(v.RecordDigest) || !hashID(v.InputID) || !hashID(v.ResultDigest) || v.SourceDigest != ownedSourceDigest(v.Source) {
		return ErrInvalid
	}
	if _, e := gymcard.ValidateOwnedBoutSource(p.SourcePin, v.Source); e != nil {
		return ErrInvalid
	}
	return ownedValidateSavedBout(p, OwnedSavedBout{Bout: v.Source.Bout, SourceDigest: v.SourceDigest, Exists: true, RecordDigest: v.RecordDigest, CreatedAtMS: v.CreatedAtMS, InputID: v.InputID, ResultDigest: v.ResultDigest, Winner: v.Summary.Winner, Method: v.Summary.Method, EndRound: v.Summary.EndRound, EndTick: v.Summary.EndTick})
}
func ValidateOwnedResultSource(p OwnedPin, v OwnedResultSource) (OwnedResultSource, error) {
	if _, e := ValidateOwnedPin(p); e != nil {
		return OwnedResultSource{}, e
	}
	return ownedValue(v, OwnedMaxResultSourceSize, func(v OwnedResultSource) error { return ownedValidateResultSource(p, v) })
}
func MarshalOwnedResultSource(p OwnedPin, v OwnedResultSource) ([]byte, error) {
	v, e := ValidateOwnedResultSource(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseOwnedResultSource(p OwnedPin, b []byte) (OwnedResultSource, error) {
	var v OwnedResultSource
	if e := ownedDecode(b, OwnedMaxResultSourceSize, &v, false); e != nil {
		return v, e
	}
	return ValidateOwnedResultSource(p, v)
}
func ownedResultSourceDigest(v OwnedResultSource) string {
	id, _ := ownedcodec.Hash("bout result source", v)
	return id
}
func OwnedResultSourceDigest(p OwnedPin, v OwnedResultSource) (string, error) {
	v, e := ValidateOwnedResultSource(p, v)
	if e != nil {
		return "", e
	}
	return ownedResultSourceDigest(v), nil
}
