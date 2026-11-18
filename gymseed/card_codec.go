// Copyright (c) 2026 The SLUG developers. ISC license.
package gymseed

import "encoding/json"

func ValidateCardSource(p Pin, v CardSource) (CardSource, error) {
	if _, e := ValidatePin(p); e != nil {
		return CardSource{}, e
	}
	return cardOwned(v, MaxCardSourceSize, func(v CardSource) error { return validateCardSource(p, v) })
}
func MarshalCardSource(p Pin, v CardSource) ([]byte, error) {
	v, e := ValidateCardSource(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseCardSource(p Pin, raw []byte) (CardSource, error) {
	var v CardSource
	if e := cardDecode(raw, MaxCardSourceSize, &v, false); e != nil {
		return v, e
	}
	return ValidateCardSource(p, v)
}
func ValidateCardState(p Pin, v CardState) (CardState, error) {
	if _, e := ValidatePin(p); e != nil {
		return CardState{}, e
	}
	return cardOwned(v, MaxCardStateSize, func(v CardState) error { return validateCardState(p, v) })
}
func MarshalCardState(p Pin, v CardState) ([]byte, error) {
	v, e := ValidateCardState(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseCardState(p Pin, raw []byte) (CardState, error) {
	var v CardState
	if e := cardDecode(raw, MaxCardStateSize, &v, false); e != nil {
		return v, e
	}
	return ValidateCardState(p, v)
}
func ValidateCardView(p Pin, v CardView) (CardView, error) {
	if _, e := ValidatePin(p); e != nil {
		return CardView{}, e
	}
	return cardOwned(v, MaxCardViewSize, func(v CardView) error { return validateCardView(p, v) })
}
func MarshalCardView(p Pin, v CardView) ([]byte, error) {
	v, e := ValidateCardView(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseCardView(p Pin, raw []byte) (CardView, error) {
	var v CardView
	if e := cardDecode(raw, MaxCardViewSize, &v, false); e != nil {
		return v, e
	}
	return ValidateCardView(p, v)
}
