// Copyright (c) 2026 The SLUG developers. ISC license.
package gymcard

import (
	"encoding/json"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/internal/ownedcodec"
)

func CandidateOwnedPin(source gymseed.OwnedPin) (OwnedPin, error) {
	source, e := gymseed.ValidateOwnedPin(source)
	if e != nil {
		return OwnedPin{}, ErrInvalid
	}
	id, e := gymseed.OwnedID(source)
	if e != nil {
		return OwnedPin{}, ErrInvalid
	}
	return OwnedPin{1, id, source, RulesID()}, nil
}
func OwnedID(p OwnedPin) (string, error) {
	if validateOwnedPin(p) != nil {
		return "", ErrInvalid
	}
	return ownedcodec.Hash("card store", p)
}
func ownedCardID(p OwnedPin, ev gymevent.Event) string {
	id, _ := OwnedID(p)
	v, _ := ownedcodec.Hash("card", struct {
		Version uint32 `json:"version"`
		StoreID string `json:"store_id"`
		EventID string `json:"event_id"`
	}{1, id, ev.EventID})
	return v
}
func OwnedCardID(p OwnedPin, ev gymevent.Event) (string, error) {
	if validateOwnedPin(p) != nil {
		return "", ErrInvalid
	}
	if _, e := gymevent.ValidateOwnedEvent(p.SourcePin.SourcePin, ev); e != nil {
		return "", ErrInvalid
	}
	return ownedCardID(p, ev), nil
}
func ownedInputID(card, snapshot, event string, height uint32, target, rules string) string {
	v, _ := ownedcodec.Hash("card input", struct {
		Version        uint32 `json:"version"`
		CardID         string `json:"card_id"`
		SnapshotDigest string `json:"snapshot_digest"`
		SeedEventKey   string `json:"seed_event_key"`
		TargetHeight   uint32 `json:"target_height"`
		TargetHash     string `json:"target_hash"`
		RulesID        string `json:"rules_id"`
	}{1, card, snapshot, event, height, target, rules})
	return v
}
func ownedRecordDigest(v OwnedRecord) string { id, _ := ownedcodec.Hash("card record", v); return id }
func OwnedRecordDigest(p OwnedPin, v OwnedRecord) (string, error) {
	if _, e := ValidateOwnedRecord(p, v); e != nil {
		return "", e
	}
	return ownedRecordDigest(v), nil
}
func ownedValue[T any](v T, limit int, validate func(T) error) (T, error) {
	var z T
	if validate(v) != nil {
		return z, ErrInvalid
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
