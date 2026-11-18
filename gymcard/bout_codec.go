package gymcard

import "encoding/json"

func ValidateBoutCorner(p Pin, v BoutCorner) (BoutCorner, error) {
	if _, e := ValidatePin(p); e != nil {
		return BoutCorner{}, e
	}
	return boutOwned(v, 8192, func(v BoutCorner) error { return validateBoutCorner(p, v) })
}
func MarshalBoutCorner(p Pin, v BoutCorner) ([]byte, error) {
	v, e := ValidateBoutCorner(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseBoutCorner(p Pin, raw []byte) (BoutCorner, error) {
	var v BoutCorner
	if e := boutDecode(raw, 8192, &v, false); e != nil {
		return v, e
	}
	return ValidateBoutCorner(p, v)
}
func ValidateBoutSource(p Pin, v BoutSource) (BoutSource, error) {
	if _, e := ValidatePin(p); e != nil {
		return BoutSource{}, e
	}
	return boutOwned(v, 16384, func(v BoutSource) error { return validateBoutSource(p, v) })
}
func MarshalBoutSource(p Pin, v BoutSource) ([]byte, error) {
	v, e := ValidateBoutSource(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseBoutSource(p Pin, raw []byte) (BoutSource, error) {
	var v BoutSource
	if e := boutDecode(raw, 16384, &v, false); e != nil {
		return v, e
	}
	return ValidateBoutSource(p, v)
}
func ValidateBoutState(p Pin, v BoutState) (BoutState, error) {
	if _, e := ValidatePin(p); e != nil {
		return BoutState{}, e
	}
	return boutOwned(v, 4096, func(v BoutState) error { return validateBoutState(p, v) })
}
func MarshalBoutState(p Pin, v BoutState) ([]byte, error) {
	v, e := ValidateBoutState(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseBoutState(p Pin, raw []byte) (BoutState, error) {
	var v BoutState
	if e := boutDecode(raw, 4096, &v, false); e != nil {
		return v, e
	}
	return ValidateBoutState(p, v)
}
func ValidateBoutRef(p Pin, v BoutRef) (BoutRef, error) {
	if _, e := ValidatePin(p); e != nil {
		return BoutRef{}, e
	}
	return boutOwned(v, 2048, func(v BoutRef) error { return validateBoutRef(p, v) })
}
func MarshalBoutRef(p Pin, v BoutRef) ([]byte, error) {
	v, e := ValidateBoutRef(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseBoutRef(p Pin, raw []byte) (BoutRef, error) {
	var v BoutRef
	if e := boutDecode(raw, 2048, &v, false); e != nil {
		return v, e
	}
	return ValidateBoutRef(p, v)
}
func ValidateBoutView(p Pin, v BoutView) (BoutView, error) {
	if _, e := ValidatePin(p); e != nil {
		return BoutView{}, e
	}
	return boutOwned(v, 32768, func(v BoutView) error { return validateBoutView(p, v) })
}
func MarshalBoutView(p Pin, v BoutView) ([]byte, error) {
	v, e := ValidateBoutView(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseBoutView(p Pin, raw []byte) (BoutView, error) {
	var v BoutView
	if e := boutDecode(raw, 32768, &v, false); e != nil {
		return v, e
	}
	return ValidateBoutView(p, v)
}
