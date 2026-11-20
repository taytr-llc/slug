package gymcard

import "encoding/json"

func ValidateOwnedBoutCorner(p OwnedPin, v OwnedBoutCorner) (OwnedBoutCorner, error) {
	if _, e := ValidateOwnedPin(p); e != nil {
		return OwnedBoutCorner{}, e
	}
	return ownedValue(v, 8192, func(v OwnedBoutCorner) error { return ownedValidateBoutCorner(p, v) })
}
func MarshalOwnedBoutCorner(p OwnedPin, v OwnedBoutCorner) ([]byte, error) {
	v, e := ValidateOwnedBoutCorner(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseOwnedBoutCorner(p OwnedPin, raw []byte) (OwnedBoutCorner, error) {
	var v OwnedBoutCorner
	if e := ownedDecode(raw, 8192, &v, false); e != nil {
		return v, e
	}
	return ValidateOwnedBoutCorner(p, v)
}
func ValidateOwnedBoutSource(p OwnedPin, v OwnedBoutSource) (OwnedBoutSource, error) {
	if _, e := ValidateOwnedPin(p); e != nil {
		return OwnedBoutSource{}, e
	}
	return ownedValue(v, OwnedMaxBoutSourceSize, func(v OwnedBoutSource) error { return ownedValidateBoutSource(p, v) })
}
func MarshalOwnedBoutSource(p OwnedPin, v OwnedBoutSource) ([]byte, error) {
	v, e := ValidateOwnedBoutSource(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseOwnedBoutSource(p OwnedPin, raw []byte) (OwnedBoutSource, error) {
	var v OwnedBoutSource
	if e := ownedDecode(raw, OwnedMaxBoutSourceSize, &v, false); e != nil {
		return v, e
	}
	return ValidateOwnedBoutSource(p, v)
}
func ValidateOwnedBoutState(p OwnedPin, v OwnedBoutState) (OwnedBoutState, error) {
	if _, e := ValidateOwnedPin(p); e != nil {
		return OwnedBoutState{}, e
	}
	return ownedValue(v, OwnedMaxBoutStateSize, func(v OwnedBoutState) error { return ownedValidateBoutState(p, v) })
}
func MarshalOwnedBoutState(p OwnedPin, v OwnedBoutState) ([]byte, error) {
	v, e := ValidateOwnedBoutState(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseOwnedBoutState(p OwnedPin, raw []byte) (OwnedBoutState, error) {
	var v OwnedBoutState
	if e := ownedDecode(raw, OwnedMaxBoutStateSize, &v, false); e != nil {
		return v, e
	}
	return ValidateOwnedBoutState(p, v)
}
func ValidateOwnedBoutView(p OwnedPin, v OwnedBoutView) (OwnedBoutView, error) {
	if _, e := ValidateOwnedPin(p); e != nil {
		return OwnedBoutView{}, e
	}
	return ownedValue(v, OwnedMaxBoutViewSize, func(v OwnedBoutView) error { return ownedValidateBoutView(p, v) })
}
func MarshalOwnedBoutView(p OwnedPin, v OwnedBoutView) ([]byte, error) {
	v, e := ValidateOwnedBoutView(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseOwnedBoutView(p OwnedPin, raw []byte) (OwnedBoutView, error) {
	var v OwnedBoutView
	if e := ownedDecode(raw, OwnedMaxBoutViewSize, &v, false); e != nil {
		return v, e
	}
	return ValidateOwnedBoutView(p, v)
}
