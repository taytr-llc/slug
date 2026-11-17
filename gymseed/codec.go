// Copyright (c) 2026 The SLUG developers. ISC license.
package gymseed

import "encoding/json"

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
		return Pin{}, e
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
		return Info{}, e
	}
	return ValidateInfo(p, v)
}
func ValidateDeclaration(p Pin, v Declaration) (Declaration, error) {
	if _, e := ValidatePin(p); e != nil {
		return Declaration{}, e
	}
	return owned(v, MaxDeclarationSize, func(v Declaration) error { return validateDeclaration(p, v) })
}
func MarshalDeclaration(p Pin, v Declaration) ([]byte, error) {
	v, e := ValidateDeclaration(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseDeclaration(p Pin, raw []byte) (Declaration, error) {
	var v Declaration
	if e := decode(raw, MaxDeclarationSize, &v, false); e != nil {
		return Declaration{}, e
	}
	return ValidateDeclaration(p, v)
}
func ValidateSignedDeclaration(p Pin, v SignedDeclaration) (SignedDeclaration, error) {
	if _, e := ValidatePin(p); e != nil {
		return SignedDeclaration{}, e
	}
	return owned(v, MaxSignedDeclarationSize, func(v SignedDeclaration) error { return validateSignedDeclaration(p, v) })
}
func MarshalSignedDeclaration(p Pin, v SignedDeclaration) ([]byte, error) {
	v, e := ValidateSignedDeclaration(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseSignedDeclaration(p Pin, raw []byte) (SignedDeclaration, error) {
	var v SignedDeclaration
	if e := decode(raw, MaxSignedDeclarationSize, &v, false); e != nil {
		return SignedDeclaration{}, e
	}
	return ValidateSignedDeclaration(p, v)
}
func ValidateOutpoint(p Pin, v Outpoint) (Outpoint, error) {
	if _, e := ValidatePin(p); e != nil {
		return Outpoint{}, e
	}
	return owned(v, 256, func(v Outpoint) error { return validateOutpoint(p, v) })
}
func MarshalOutpoint(p Pin, v Outpoint) ([]byte, error) {
	v, e := ValidateOutpoint(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseOutpoint(p Pin, raw []byte) (Outpoint, error) {
	var v Outpoint
	if e := decode(raw, 256, &v, false); e != nil {
		return Outpoint{}, e
	}
	return ValidateOutpoint(p, v)
}
func ValidateFundingCoin(p Pin, v FundingCoin) (FundingCoin, error) {
	if _, e := ValidatePin(p); e != nil {
		return FundingCoin{}, e
	}
	return owned(v, 512, func(v FundingCoin) error { return validateFundingCoin(p, v) })
}
func MarshalFundingCoin(p Pin, v FundingCoin) ([]byte, error) {
	v, e := ValidateFundingCoin(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseFundingCoin(p Pin, raw []byte) (FundingCoin, error) {
	var v FundingCoin
	if e := decode(raw, 512, &v, false); e != nil {
		return FundingCoin{}, e
	}
	return ValidateFundingCoin(p, v)
}
func ValidatePublication(p Pin, v Publication) (Publication, error) {
	if _, e := ValidatePin(p); e != nil {
		return Publication{}, e
	}
	return owned(v, MaxPublicationSize, func(v Publication) error { return validatePublication(p, v) })
}
func MarshalPublication(p Pin, v Publication) ([]byte, error) {
	v, e := ValidatePublication(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParsePublication(p Pin, raw []byte) (Publication, error) {
	var v Publication
	if e := decode(raw, MaxPublicationSize, &v, false); e != nil {
		return Publication{}, e
	}
	return ValidatePublication(p, v)
}
func ValidateCursor(p Pin, v Cursor) (Cursor, error) {
	if _, e := ValidatePin(p); e != nil {
		return Cursor{}, e
	}
	return owned(v, 512, func(v Cursor) error { return validateCursor(p, v) })
}
func MarshalCursor(p Pin, v Cursor) ([]byte, error) {
	v, e := ValidateCursor(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseCursor(p Pin, raw []byte) (Cursor, error) {
	var v Cursor
	if e := decode(raw, 512, &v, false); e != nil {
		return Cursor{}, e
	}
	return ValidateCursor(p, v)
}
func ValidateTip(p Pin, v Tip) (Tip, error) {
	if _, e := ValidatePin(p); e != nil {
		return Tip{}, e
	}
	return owned(v, 256, func(v Tip) error { return validateTip(p, v) })
}
func MarshalTip(p Pin, v Tip) ([]byte, error) {
	v, e := ValidateTip(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseTip(p Pin, raw []byte) (Tip, error) {
	var v Tip
	if e := decode(raw, 256, &v, false); e != nil {
		return Tip{}, e
	}
	return ValidateTip(p, v)
}
func ValidateAnchor(p Pin, v Anchor) (Anchor, error) {
	if _, e := ValidatePin(p); e != nil {
		return Anchor{}, e
	}
	return owned(v, 4096, func(v Anchor) error { return validateAnchor(p, v) })
}
func MarshalAnchor(p Pin, v Anchor) ([]byte, error) {
	v, e := ValidateAnchor(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseAnchor(p Pin, raw []byte) (Anchor, error) {
	var v Anchor
	if e := decode(raw, 4096, &v, false); e != nil {
		return Anchor{}, e
	}
	return ValidateAnchor(p, v)
}
func ValidateScanRestart(p Pin, v ScanRestart) (ScanRestart, error) {
	if _, e := ValidatePin(p); e != nil {
		return ScanRestart{}, e
	}
	return owned(v, MaxScanRestartSize, func(v ScanRestart) error { return validateScanRestart(p, v) })
}
func MarshalScanRestart(p Pin, v ScanRestart) ([]byte, error) {
	v, e := ValidateScanRestart(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseScanRestart(p Pin, raw []byte) (ScanRestart, error) {
	var v ScanRestart
	if e := decode(raw, MaxScanRestartSize, &v, false); e != nil {
		return ScanRestart{}, e
	}
	return ValidateScanRestart(p, v)
}
func ValidateTarget(p Pin, v Target) (Target, error) {
	if _, e := ValidatePin(p); e != nil {
		return Target{}, e
	}
	return owned(v, 1024, func(v Target) error { return validateTarget(p, v) })
}
func MarshalTarget(p Pin, v Target) ([]byte, error) {
	v, e := ValidateTarget(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseTarget(p Pin, raw []byte) (Target, error) {
	var v Target
	if e := decode(raw, 1024, &v, false); e != nil {
		return Target{}, e
	}
	return ValidateTarget(p, v)
}
func ValidateFinality(p Pin, v Finality) (Finality, error) {
	if _, e := ValidatePin(p); e != nil {
		return Finality{}, e
	}
	return owned(v, 8192, func(v Finality) error { return validateFinality(p, v) })
}
func MarshalFinality(p Pin, v Finality) ([]byte, error) {
	v, e := ValidateFinality(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseFinality(p Pin, raw []byte) (Finality, error) {
	var v Finality
	if e := decode(raw, 8192, &v, false); e != nil {
		return Finality{}, e
	}
	return ValidateFinality(p, v)
}
func ValidateConflict(p Pin, v Conflict) (Conflict, error) {
	if _, e := ValidatePin(p); e != nil {
		return Conflict{}, e
	}
	return owned(v, MaxConflictSize, func(v Conflict) error { return validateConflict(p, v) })
}
func MarshalConflict(p Pin, v Conflict) ([]byte, error) {
	v, e := ValidateConflict(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseConflict(p Pin, raw []byte) (Conflict, error) {
	var v Conflict
	if e := decode(raw, MaxConflictSize, &v, false); e != nil {
		return Conflict{}, e
	}
	return ValidateConflict(p, v)
}
func ValidateObservation(p Pin, v Observation) (Observation, error) {
	if _, e := ValidatePin(p); e != nil {
		return Observation{}, e
	}
	return owned(v, 1024, func(v Observation) error { return validateObservation(p, v) })
}
func MarshalObservation(p Pin, v Observation) ([]byte, error) {
	v, e := ValidateObservation(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseObservation(p Pin, raw []byte) (Observation, error) {
	var v Observation
	if e := decode(raw, 1024, &v, false); e != nil {
		return Observation{}, e
	}
	return ValidateObservation(p, v)
}
func ValidateRecord(p Pin, v Record) (Record, error) {
	if _, e := ValidatePin(p); e != nil {
		return Record{}, e
	}
	return owned(v, MaxRecordSize, func(v Record) error { return validateRecord(p, v) })
}
func MarshalRecord(p Pin, v Record) ([]byte, error) {
	v, e := ValidateRecord(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseRecord(p Pin, raw []byte) (Record, error) {
	var v Record
	if e := decode(raw, MaxRecordSize, &v, false); e != nil {
		return Record{}, e
	}
	return ValidateRecord(p, v)
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
		return Status{}, e
	}
	return ValidateStatus(p, v)
}
func ValidateHistoryPage(p Pin, v HistoryPage) (HistoryPage, error) {
	if _, e := ValidatePin(p); e != nil {
		return HistoryPage{}, e
	}
	return owned(v, MaxResponseSize, func(v HistoryPage) error { return validateHistoryPage(p, v) })
}
func MarshalHistoryPage(p Pin, v HistoryPage) ([]byte, error) {
	v, e := ValidateHistoryPage(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseHistoryPage(p Pin, raw []byte) (HistoryPage, error) {
	var v HistoryPage
	if e := decode(raw, MaxResponseSize, &v, false); e != nil {
		return HistoryPage{}, e
	}
	return ValidateHistoryPage(p, v)
}
