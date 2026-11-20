// Copyright (c) 2026 The SLUG developers. ISC license.
package gymseed

import "encoding/json"

func ValidateOwnedCardSource(p OwnedPin, v OwnedCardSource) (OwnedCardSource, error) {
	if _, e := ValidateOwnedPin(p); e != nil {
		return OwnedCardSource{}, e
	}
	return ownedValue(v, OwnedMaxCardSourceSize, func(v OwnedCardSource) error { return ownedValidateCardSource(p, v) })
}
func MarshalOwnedCardSource(p OwnedPin, v OwnedCardSource) ([]byte, error) {
	v, e := ValidateOwnedCardSource(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseOwnedCardSource(p OwnedPin, raw []byte) (OwnedCardSource, error) {
	var v OwnedCardSource
	if e := ownedDecode(raw, OwnedMaxCardSourceSize, &v, false); e != nil {
		return v, e
	}
	return ValidateOwnedCardSource(p, v)
}
func ValidateOwnedCardState(p OwnedPin, v OwnedCardState) (OwnedCardState, error) {
	if _, e := ValidateOwnedPin(p); e != nil {
		return OwnedCardState{}, e
	}
	return ownedValue(v, OwnedMaxCardStateSize, func(v OwnedCardState) error { return ownedValidateCardState(p, v) })
}
func MarshalOwnedCardState(p OwnedPin, v OwnedCardState) ([]byte, error) {
	v, e := ValidateOwnedCardState(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseOwnedCardState(p OwnedPin, raw []byte) (OwnedCardState, error) {
	var v OwnedCardState
	if e := ownedDecode(raw, OwnedMaxCardStateSize, &v, false); e != nil {
		return v, e
	}
	return ValidateOwnedCardState(p, v)
}
func ValidateOwnedCardView(p OwnedPin, v OwnedCardView) (OwnedCardView, error) {
	if _, e := ValidateOwnedPin(p); e != nil {
		return OwnedCardView{}, e
	}
	return ownedValue(v, OwnedMaxCardViewSize, func(v OwnedCardView) error { return ownedValidateCardView(p, v) })
}
func MarshalOwnedCardView(p OwnedPin, v OwnedCardView) ([]byte, error) {
	v, e := ValidateOwnedCardView(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseOwnedCardView(p OwnedPin, raw []byte) (OwnedCardView, error) {
	var v OwnedCardView
	if e := ownedDecode(raw, OwnedMaxCardViewSize, &v, false); e != nil {
		return v, e
	}
	return ValidateOwnedCardView(p, v)
}
