// Copyright (c) 2026 The SLUG developers. ISC license.
package gymresult

import "encoding/json"

func ValidatePin(v Pin) (Pin, error) {
	return owned(v, MaxPinSize, func(v Pin) error { return validatePin(v) })
}
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
		return v, e
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
		return v, e
	}
	return ValidateInfo(p, v)
}
func ValidateStatement(p Pin, v Statement) (Statement, error) {
	if _, e := ValidatePin(p); e != nil {
		return Statement{}, e
	}
	return owned(v, MaxStatementSize, func(v Statement) error { return validateStatement(p, v) })
}
func MarshalStatement(p Pin, v Statement) ([]byte, error) {
	v, e := ValidateStatement(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseStatement(p Pin, raw []byte) (Statement, error) {
	var v Statement
	if e := decode(raw, MaxStatementSize, &v, false); e != nil {
		return v, e
	}
	return ValidateStatement(p, v)
}
func ValidateSignedStatement(p Pin, v SignedStatement) (SignedStatement, error) {
	if _, e := ValidatePin(p); e != nil {
		return SignedStatement{}, e
	}
	return owned(v, MaxSignedStatementSize, func(v SignedStatement) error { return validateSignedStatement(p, v) })
}
func MarshalSignedStatement(p Pin, v SignedStatement) ([]byte, error) {
	v, e := ValidateSignedStatement(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseSignedStatement(p Pin, raw []byte) (SignedStatement, error) {
	var v SignedStatement
	if e := decode(raw, MaxSignedStatementSize, &v, false); e != nil {
		return v, e
	}
	return ValidateSignedStatement(p, v)
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
		return v, e
	}
	return ValidatePublication(p, v)
}
func ValidateInclusion(p Pin, v Inclusion) (Inclusion, error) {
	if _, e := ValidatePin(p); e != nil {
		return Inclusion{}, e
	}
	return owned(v, MaxInclusionSize, func(v Inclusion) error { return validateInclusion(p, v) })
}
func MarshalInclusion(p Pin, v Inclusion) ([]byte, error) {
	v, e := ValidateInclusion(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseInclusion(p Pin, raw []byte) (Inclusion, error) {
	var v Inclusion
	if e := decode(raw, MaxInclusionSize, &v, false); e != nil {
		return v, e
	}
	return ValidateInclusion(p, v)
}
func ValidatePriorInclusion(p Pin, v PriorInclusion) (PriorInclusion, error) {
	if _, e := ValidatePin(p); e != nil {
		return PriorInclusion{}, e
	}
	return owned(v, MaxPriorInclusionSize, func(v PriorInclusion) error { return validatePriorInclusion(p, v) })
}
func MarshalPriorInclusion(p Pin, v PriorInclusion) ([]byte, error) {
	v, e := ValidatePriorInclusion(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParsePriorInclusion(p Pin, raw []byte) (PriorInclusion, error) {
	var v PriorInclusion
	if e := decode(raw, MaxPriorInclusionSize, &v, false); e != nil {
		return v, e
	}
	return ValidatePriorInclusion(p, v)
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
		return v, e
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
		return v, e
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
		return v, e
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
		return v, e
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
		return v, e
	}
	return ValidateStatus(p, v)
}
func ValidateActionResult(p Pin, v ActionResult) (ActionResult, error) {
	if _, e := ValidatePin(p); e != nil {
		return ActionResult{}, e
	}
	return owned(v, MaxActionResultSize, func(v ActionResult) error { return validateActionResult(p, v) })
}
func MarshalActionResult(p Pin, v ActionResult) ([]byte, error) {
	v, e := ValidateActionResult(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseActionResult(p Pin, raw []byte) (ActionResult, error) {
	var v ActionResult
	if e := decode(raw, MaxActionResultSize, &v, false); e != nil {
		return v, e
	}
	return ValidateActionResult(p, v)
}
func ValidateReadResult(p Pin, v ReadResult) (ReadResult, error) {
	if _, e := ValidatePin(p); e != nil {
		return ReadResult{}, e
	}
	return owned(v, MaxReadResultSize, func(v ReadResult) error { return validateReadResult(p, v) })
}
func MarshalReadResult(p Pin, v ReadResult) ([]byte, error) {
	v, e := ValidateReadResult(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseReadResult(p Pin, raw []byte) (ReadResult, error) {
	var v ReadResult
	if e := decode(raw, MaxReadResultSize, &v, false); e != nil {
		return v, e
	}
	return ValidateReadResult(p, v)
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
		return v, e
	}
	return ValidateTip(p, v)
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
		return v, e
	}
	return ValidateOutpoint(p, v)
}
func ValidateFundingCoin(p Pin, v FundingCoin) (FundingCoin, error) {
	if _, e := ValidatePin(p); e != nil {
		return FundingCoin{}, e
	}
	return owned(v, 1024, func(v FundingCoin) error { return validateFundingCoin(p, v) })
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
	if e := decode(raw, 1024, &v, false); e != nil {
		return v, e
	}
	return ValidateFundingCoin(p, v)
}
func ValidateAnchor(p Pin, v Anchor) (Anchor, error) {
	if _, e := ValidatePin(p); e != nil {
		return Anchor{}, e
	}
	return owned(v, MaxAnchorSize, func(v Anchor) error { return validateAnchor(p, v) })
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
	if e := decode(raw, MaxAnchorSize, &v, false); e != nil {
		return v, e
	}
	return ValidateAnchor(p, v)
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
		return v, e
	}
	return ValidateCursor(p, v)
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
		return v, e
	}
	return ValidateScanRestart(p, v)
}
func ValidateProofSummary(p Pin, v ProofSummary) (ProofSummary, error) {
	if _, e := ValidatePin(p); e != nil {
		return ProofSummary{}, e
	}
	return owned(v, MaxProofSummarySize, func(v ProofSummary) error { return validateProofSummary(p, v) })
}
func MarshalProofSummary(p Pin, v ProofSummary) ([]byte, error) {
	v, e := ValidateProofSummary(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseProofSummary(p Pin, raw []byte) (ProofSummary, error) {
	var v ProofSummary
	if e := decode(raw, MaxProofSummarySize, &v, false); e != nil {
		return v, e
	}
	return ValidateProofSummary(p, v)
}
