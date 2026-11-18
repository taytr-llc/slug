// Copyright (c) 2026 The SLUG developers. ISC license.
package gymcard

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
func ValidateProfessionalRecord(p Pin, v ProfessionalRecord) (ProfessionalRecord, error) {
	if _, e := ValidatePin(p); e != nil {
		return ProfessionalRecord{}, e
	}
	return owned(v, 1024, func(v ProfessionalRecord) error { return validateProfessionalRecord(p, v) })
}
func MarshalProfessionalRecord(p Pin, v ProfessionalRecord) ([]byte, error) {
	v, e := ValidateProfessionalRecord(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseProfessionalRecord(p Pin, raw []byte) (ProfessionalRecord, error) {
	var v ProfessionalRecord
	if e := decode(raw, 1024, &v, false); e != nil {
		return v, e
	}
	return ValidateProfessionalRecord(p, v)
}
func ValidateReasonCounts(p Pin, v ReasonCounts) (ReasonCounts, error) {
	if _, e := ValidatePin(p); e != nil {
		return ReasonCounts{}, e
	}
	return owned(v, 1024, func(v ReasonCounts) error { return validateReasonCounts(p, v) })
}
func MarshalReasonCounts(p Pin, v ReasonCounts) ([]byte, error) {
	v, e := ValidateReasonCounts(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseReasonCounts(p Pin, raw []byte) (ReasonCounts, error) {
	var v ReasonCounts
	if e := decode(raw, 1024, &v, false); e != nil {
		return v, e
	}
	return ValidateReasonCounts(p, v)
}
func ValidateCounts(p Pin, v Counts) (Counts, error) {
	if _, e := ValidatePin(p); e != nil {
		return Counts{}, e
	}
	return owned(v, 1024, func(v Counts) error { return validateCounts(p, v) })
}
func MarshalCounts(p Pin, v Counts) ([]byte, error) {
	v, e := ValidateCounts(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseCounts(p Pin, raw []byte) (Counts, error) {
	var v Counts
	if e := decode(raw, 1024, &v, false); e != nil {
		return v, e
	}
	return ValidateCounts(p, v)
}
func ValidateGymCounts(p Pin, v GymCounts) (GymCounts, error) {
	if _, e := ValidatePin(p); e != nil {
		return GymCounts{}, e
	}
	return owned(v, 1024, func(v GymCounts) error { return validateGymCounts(p, v) })
}
func MarshalGymCounts(p Pin, v GymCounts) ([]byte, error) {
	v, e := ValidateGymCounts(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseGymCounts(p Pin, raw []byte) (GymCounts, error) {
	var v GymCounts
	if e := decode(raw, 1024, &v, false); e != nil {
		return v, e
	}
	return ValidateGymCounts(p, v)
}
func ValidateEntrant(p Pin, v Entrant) (Entrant, error) {
	if _, e := ValidatePin(p); e != nil {
		return Entrant{}, e
	}
	return owned(v, 2048, func(v Entrant) error { return validateEntrant(p, v) })
}
func MarshalEntrant(p Pin, v Entrant) ([]byte, error) {
	v, e := ValidateEntrant(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseEntrant(p Pin, raw []byte) (Entrant, error) {
	var v Entrant
	if e := decode(raw, 2048, &v, false); e != nil {
		return v, e
	}
	return ValidateEntrant(p, v)
}
func ValidateBout(p Pin, v Bout) (Bout, error) {
	if _, e := ValidatePin(p); e != nil {
		return Bout{}, e
	}
	return owned(v, 1024, func(v Bout) error { return validateBout(p, v) })
}
func MarshalBout(p Pin, v Bout) ([]byte, error) {
	v, e := ValidateBout(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseBout(p Pin, raw []byte) (Bout, error) {
	var v Bout
	if e := decode(raw, 1024, &v, false); e != nil {
		return v, e
	}
	return ValidateBout(p, v)
}
func ValidatePlan(p Pin, v Plan) (Plan, error) {
	if _, e := ValidatePin(p); e != nil {
		return Plan{}, e
	}
	return owned(v, MaxPlanSize, func(v Plan) error { return validatePlan(p, v) })
}
func MarshalPlan(p Pin, v Plan) ([]byte, error) {
	v, e := ValidatePlan(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParsePlan(p Pin, raw []byte) (Plan, error) {
	var v Plan
	if e := decode(raw, MaxPlanSize, &v, false); e != nil {
		return v, e
	}
	return ValidatePlan(p, v)
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
func ValidateBuildResult(p Pin, v BuildResult) (BuildResult, error) {
	if _, e := ValidatePin(p); e != nil {
		return BuildResult{}, e
	}
	return owned(v, MaxBuildResultSize, func(v BuildResult) error { return validateBuildResult(p, v) })
}
func MarshalBuildResult(p Pin, v BuildResult) ([]byte, error) {
	v, e := ValidateBuildResult(p, v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseBuildResult(p Pin, raw []byte) (BuildResult, error) {
	var v BuildResult
	if e := decode(raw, MaxBuildResultSize, &v, false); e != nil {
		return v, e
	}
	return ValidateBuildResult(p, v)
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
func ValidateHistoryPage(p Pin, v HistoryPage) (HistoryPage, error) {
	if _, e := ValidatePin(p); e != nil {
		return HistoryPage{}, e
	}
	return owned(v, MaxHistoryPageSize, func(v HistoryPage) error { return validateHistoryPage(p, v) })
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
	if e := decode(raw, MaxHistoryPageSize, &v, false); e != nil {
		return v, e
	}
	return ValidateHistoryPage(p, v)
}
