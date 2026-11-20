// Copyright (c) 2026 The SLUG developers. ISC license.
package gymresult

import "encoding/json"

func ValidateOwnedPin(v OwnedPin) (OwnedPin, error) {
	return ownedValue(v, OwnedMaxPinSize, func(v OwnedPin) error { return validateOwnedPin(v) })
}
func MarshalOwnedPin(v OwnedPin) ([]byte, error) {
	v, e := ValidateOwnedPin(v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseOwnedPin(raw []byte) (OwnedPin, error) {
	var v OwnedPin
	if e := ownedDecode(raw, OwnedMaxPinSize, &v, false); e != nil {
		return v, e
	}
	return ValidateOwnedPin(v)
}
func ValidateOwnedInfo(p OwnedPin, v OwnedInfo) (OwnedInfo, error) {
	if _, e := ValidateOwnedPin(p); e != nil {
		return OwnedInfo{}, e
	}
	return ownedValue(v, OwnedMaxPinSize, func(v OwnedInfo) error { return ownedValidateInfo(p, v) })
}
func MarshalOwnedInfo(p OwnedPin, v OwnedInfo) ([]byte, error) {
	v, e := ValidateOwnedInfo(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseOwnedInfo(p OwnedPin, raw []byte) (OwnedInfo, error) {
	var v OwnedInfo
	if e := ownedDecode(raw, OwnedMaxPinSize, &v, false); e != nil {
		return v, e
	}
	return ValidateOwnedInfo(p, v)
}
func ValidateOwnedStatement(p OwnedPin, v OwnedStatement) (OwnedStatement, error) {
	if _, e := ValidateOwnedPin(p); e != nil {
		return OwnedStatement{}, e
	}
	return ownedValue(v, OwnedMaxStatementSize, func(v OwnedStatement) error { return ownedValidateStatement(p, v) })
}
func MarshalOwnedStatement(p OwnedPin, v OwnedStatement) ([]byte, error) {
	v, e := ValidateOwnedStatement(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseOwnedStatement(p OwnedPin, raw []byte) (OwnedStatement, error) {
	var v OwnedStatement
	if e := ownedDecode(raw, OwnedMaxStatementSize, &v, false); e != nil {
		return v, e
	}
	return ValidateOwnedStatement(p, v)
}
func ValidateOwnedSignedStatement(p OwnedPin, v OwnedSignedStatement) (OwnedSignedStatement, error) {
	if _, e := ValidateOwnedPin(p); e != nil {
		return OwnedSignedStatement{}, e
	}
	return ownedValue(v, OwnedMaxSignedStatementSize, func(v OwnedSignedStatement) error { return ownedValidateSignedStatement(p, v) })
}
func MarshalOwnedSignedStatement(p OwnedPin, v OwnedSignedStatement) ([]byte, error) {
	v, e := ValidateOwnedSignedStatement(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseOwnedSignedStatement(p OwnedPin, raw []byte) (OwnedSignedStatement, error) {
	var v OwnedSignedStatement
	if e := ownedDecode(raw, OwnedMaxSignedStatementSize, &v, false); e != nil {
		return v, e
	}
	return ValidateOwnedSignedStatement(p, v)
}
func ValidateOwnedPublication(p OwnedPin, v Publication) (Publication, error) {
	if _, e := ValidateOwnedPin(p); e != nil {
		return Publication{}, e
	}
	return ownedValue(v, MaxPublicationSize, func(v Publication) error { return ownedValidatePublication(p, v) })
}
func MarshalOwnedPublication(p OwnedPin, v Publication) ([]byte, error) {
	v, e := ValidateOwnedPublication(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseOwnedPublication(p OwnedPin, raw []byte) (Publication, error) {
	var v Publication
	if e := ownedDecode(raw, MaxPublicationSize, &v, false); e != nil {
		return v, e
	}
	return ValidateOwnedPublication(p, v)
}
func ValidateOwnedInclusion(p OwnedPin, v Inclusion) (Inclusion, error) {
	if _, e := ValidateOwnedPin(p); e != nil {
		return Inclusion{}, e
	}
	return ownedValue(v, MaxInclusionSize, func(v Inclusion) error { return ownedValidateInclusion(p, v) })
}
func MarshalOwnedInclusion(p OwnedPin, v Inclusion) ([]byte, error) {
	v, e := ValidateOwnedInclusion(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseOwnedInclusion(p OwnedPin, raw []byte) (Inclusion, error) {
	var v Inclusion
	if e := ownedDecode(raw, MaxInclusionSize, &v, false); e != nil {
		return v, e
	}
	return ValidateOwnedInclusion(p, v)
}
func ValidateOwnedPriorInclusion(p OwnedPin, v PriorInclusion) (PriorInclusion, error) {
	if _, e := ValidateOwnedPin(p); e != nil {
		return PriorInclusion{}, e
	}
	return ownedValue(v, MaxPriorInclusionSize, func(v PriorInclusion) error { return ownedValidatePriorInclusion(p, v) })
}
func MarshalOwnedPriorInclusion(p OwnedPin, v PriorInclusion) ([]byte, error) {
	v, e := ValidateOwnedPriorInclusion(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseOwnedPriorInclusion(p OwnedPin, raw []byte) (PriorInclusion, error) {
	var v PriorInclusion
	if e := ownedDecode(raw, MaxPriorInclusionSize, &v, false); e != nil {
		return v, e
	}
	return ValidateOwnedPriorInclusion(p, v)
}
func ValidateOwnedFinality(p OwnedPin, v Finality) (Finality, error) {
	if _, e := ValidateOwnedPin(p); e != nil {
		return Finality{}, e
	}
	return ownedValue(v, 8192, func(v Finality) error { return ownedValidateFinality(p, v) })
}
func MarshalOwnedFinality(p OwnedPin, v Finality) ([]byte, error) {
	v, e := ValidateOwnedFinality(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseOwnedFinality(p OwnedPin, raw []byte) (Finality, error) {
	var v Finality
	if e := ownedDecode(raw, 8192, &v, false); e != nil {
		return v, e
	}
	return ValidateOwnedFinality(p, v)
}
func ValidateOwnedConflict(p OwnedPin, v OwnedConflict) (OwnedConflict, error) {
	if _, e := ValidateOwnedPin(p); e != nil {
		return OwnedConflict{}, e
	}
	return ownedValue(v, MaxConflictSize, func(v OwnedConflict) error { return ownedValidateConflict(p, v) })
}
func MarshalOwnedConflict(p OwnedPin, v OwnedConflict) ([]byte, error) {
	v, e := ValidateOwnedConflict(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseOwnedConflict(p OwnedPin, raw []byte) (OwnedConflict, error) {
	var v OwnedConflict
	if e := ownedDecode(raw, MaxConflictSize, &v, false); e != nil {
		return v, e
	}
	return ValidateOwnedConflict(p, v)
}
func ValidateOwnedObservation(p OwnedPin, v Observation) (Observation, error) {
	if _, e := ValidateOwnedPin(p); e != nil {
		return Observation{}, e
	}
	return ownedValue(v, 1024, func(v Observation) error { return ownedValidateObservation(p, v) })
}
func MarshalOwnedObservation(p OwnedPin, v Observation) ([]byte, error) {
	v, e := ValidateOwnedObservation(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseOwnedObservation(p OwnedPin, raw []byte) (Observation, error) {
	var v Observation
	if e := ownedDecode(raw, 1024, &v, false); e != nil {
		return v, e
	}
	return ValidateOwnedObservation(p, v)
}
func ValidateOwnedRecord(p OwnedPin, v OwnedRecord) (OwnedRecord, error) {
	if _, e := ValidateOwnedPin(p); e != nil {
		return OwnedRecord{}, e
	}
	return ownedValue(v, OwnedMaxRecordSize, func(v OwnedRecord) error { return ownedValidateRecord(p, v) })
}
func MarshalOwnedRecord(p OwnedPin, v OwnedRecord) ([]byte, error) {
	v, e := ValidateOwnedRecord(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseOwnedRecord(p OwnedPin, raw []byte) (OwnedRecord, error) {
	var v OwnedRecord
	if e := ownedDecode(raw, OwnedMaxRecordSize, &v, false); e != nil {
		return v, e
	}
	return ValidateOwnedRecord(p, v)
}
func ValidateOwnedStatus(p OwnedPin, v OwnedStatus) (OwnedStatus, error) {
	if _, e := ValidateOwnedPin(p); e != nil {
		return OwnedStatus{}, e
	}
	return ownedValue(v, MaxStatusSize, func(v OwnedStatus) error { return ownedValidateStatus(p, v) })
}
func MarshalOwnedStatus(p OwnedPin, v OwnedStatus) ([]byte, error) {
	v, e := ValidateOwnedStatus(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseOwnedStatus(p OwnedPin, raw []byte) (OwnedStatus, error) {
	var v OwnedStatus
	if e := ownedDecode(raw, MaxStatusSize, &v, false); e != nil {
		return v, e
	}
	return ValidateOwnedStatus(p, v)
}
func ValidateOwnedActionResult(p OwnedPin, v OwnedActionResult) (OwnedActionResult, error) {
	if _, e := ValidateOwnedPin(p); e != nil {
		return OwnedActionResult{}, e
	}
	return ownedValue(v, OwnedMaxResponseSize, func(v OwnedActionResult) error { return ownedValidateActionResult(p, v) })
}
func MarshalOwnedActionResult(p OwnedPin, v OwnedActionResult) ([]byte, error) {
	v, e := ValidateOwnedActionResult(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseOwnedActionResult(p OwnedPin, raw []byte) (OwnedActionResult, error) {
	var v OwnedActionResult
	if e := ownedDecode(raw, OwnedMaxResponseSize, &v, false); e != nil {
		return v, e
	}
	return ValidateOwnedActionResult(p, v)
}
func ValidateOwnedReadResult(p OwnedPin, v OwnedReadResult) (OwnedReadResult, error) {
	if _, e := ValidateOwnedPin(p); e != nil {
		return OwnedReadResult{}, e
	}
	return ownedValue(v, OwnedMaxResponseSize, func(v OwnedReadResult) error { return ownedValidateReadResult(p, v) })
}
func MarshalOwnedReadResult(p OwnedPin, v OwnedReadResult) ([]byte, error) {
	v, e := ValidateOwnedReadResult(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseOwnedReadResult(p OwnedPin, raw []byte) (OwnedReadResult, error) {
	var v OwnedReadResult
	if e := ownedDecode(raw, OwnedMaxResponseSize, &v, false); e != nil {
		return v, e
	}
	return ValidateOwnedReadResult(p, v)
}
func ValidateOwnedTip(p OwnedPin, v Tip) (Tip, error) {
	if _, e := ValidateOwnedPin(p); e != nil {
		return Tip{}, e
	}
	return ownedValue(v, 256, func(v Tip) error { return ownedValidateTip(p, v) })
}
func MarshalOwnedTip(p OwnedPin, v Tip) ([]byte, error) {
	v, e := ValidateOwnedTip(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseOwnedTip(p OwnedPin, raw []byte) (Tip, error) {
	var v Tip
	if e := ownedDecode(raw, 256, &v, false); e != nil {
		return v, e
	}
	return ValidateOwnedTip(p, v)
}
func ValidateOwnedOutpoint(p OwnedPin, v Outpoint) (Outpoint, error) {
	if _, e := ValidateOwnedPin(p); e != nil {
		return Outpoint{}, e
	}
	return ownedValue(v, 256, func(v Outpoint) error { return ownedValidateOutpoint(p, v) })
}
func MarshalOwnedOutpoint(p OwnedPin, v Outpoint) ([]byte, error) {
	v, e := ValidateOwnedOutpoint(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseOwnedOutpoint(p OwnedPin, raw []byte) (Outpoint, error) {
	var v Outpoint
	if e := ownedDecode(raw, 256, &v, false); e != nil {
		return v, e
	}
	return ValidateOwnedOutpoint(p, v)
}
func ValidateOwnedFundingCoin(p OwnedPin, v FundingCoin) (FundingCoin, error) {
	if _, e := ValidateOwnedPin(p); e != nil {
		return FundingCoin{}, e
	}
	return ownedValue(v, 1024, func(v FundingCoin) error { return ownedValidateFundingCoin(p, v) })
}
func MarshalOwnedFundingCoin(p OwnedPin, v FundingCoin) ([]byte, error) {
	v, e := ValidateOwnedFundingCoin(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseOwnedFundingCoin(p OwnedPin, raw []byte) (FundingCoin, error) {
	var v FundingCoin
	if e := ownedDecode(raw, 1024, &v, false); e != nil {
		return v, e
	}
	return ValidateOwnedFundingCoin(p, v)
}
func ValidateOwnedAnchor(p OwnedPin, v Anchor) (Anchor, error) {
	if _, e := ValidateOwnedPin(p); e != nil {
		return Anchor{}, e
	}
	return ownedValue(v, MaxAnchorSize, func(v Anchor) error { return ownedValidateAnchor(p, v) })
}
func MarshalOwnedAnchor(p OwnedPin, v Anchor) ([]byte, error) {
	v, e := ValidateOwnedAnchor(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseOwnedAnchor(p OwnedPin, raw []byte) (Anchor, error) {
	var v Anchor
	if e := ownedDecode(raw, MaxAnchorSize, &v, false); e != nil {
		return v, e
	}
	return ValidateOwnedAnchor(p, v)
}
func ValidateOwnedCursor(p OwnedPin, v Cursor) (Cursor, error) {
	if _, e := ValidateOwnedPin(p); e != nil {
		return Cursor{}, e
	}
	return ownedValue(v, 512, func(v Cursor) error { return ownedValidateCursor(p, v) })
}
func MarshalOwnedCursor(p OwnedPin, v Cursor) ([]byte, error) {
	v, e := ValidateOwnedCursor(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseOwnedCursor(p OwnedPin, raw []byte) (Cursor, error) {
	var v Cursor
	if e := ownedDecode(raw, 512, &v, false); e != nil {
		return v, e
	}
	return ValidateOwnedCursor(p, v)
}
func ValidateOwnedScanRestart(p OwnedPin, v ScanRestart) (ScanRestart, error) {
	if _, e := ValidateOwnedPin(p); e != nil {
		return ScanRestart{}, e
	}
	return ownedValue(v, MaxScanRestartSize, func(v ScanRestart) error { return ownedValidateScanRestart(p, v) })
}
func MarshalOwnedScanRestart(p OwnedPin, v ScanRestart) ([]byte, error) {
	v, e := ValidateOwnedScanRestart(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseOwnedScanRestart(p OwnedPin, raw []byte) (ScanRestart, error) {
	var v ScanRestart
	if e := ownedDecode(raw, MaxScanRestartSize, &v, false); e != nil {
		return v, e
	}
	return ValidateOwnedScanRestart(p, v)
}
