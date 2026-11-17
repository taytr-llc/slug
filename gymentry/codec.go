// Copyright (c) 2026 The SLUG developers. ISC license.
package gymentry

import "encoding/json"

func ValidatePin(v Pin) (Pin, error) { return owned(v, MaxMarkerSize, validatePin) }
func MarshalPin(v Pin) ([]byte, error) {
	v, e := ValidatePin(v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParsePin(raw []byte) (Pin, error) {
	var v Pin
	if e := decode(raw, MaxMarkerSize, &v, false); e != nil {
		return Pin{}, e
	}
	return ValidatePin(v)
}

func ValidateRequest(v Request) (Request, error) { return owned(v, MaxRequestSize, validateRequest) }
func MarshalRequest(v Request) ([]byte, error) {
	v, e := ValidateRequest(v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseRequest(raw []byte) (Request, error) {
	var v Request
	if e := decode(raw, MaxRequestSize, &v, false); e != nil {
		return Request{}, e
	}
	return ValidateRequest(v)
}

func ValidateEvent(v Event) (Event, error) { return owned(v, MaxEventSize, validateEvent) }
func MarshalEvent(v Event) ([]byte, error) {
	v, e := ValidateEvent(v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseEvent(raw []byte) (Event, error) {
	var v Event
	if e := decode(raw, MaxEventSize, &v, false); e != nil {
		return Event{}, e
	}
	return ValidateEvent(v)
}

func ValidateIntent(v Intent) (Intent, error) { return owned(v, MaxOperationSize, validateIntent) }
func MarshalIntent(v Intent) ([]byte, error) {
	v, e := ValidateIntent(v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseIntent(raw []byte) (Intent, error) {
	var v Intent
	if e := decode(raw, MaxOperationSize, &v, false); e != nil {
		return Intent{}, e
	}
	return ValidateIntent(v)
}

func ValidateOperation(v Operation) (Operation, error) {
	return owned(v, MaxOperationSize, validateOperation)
}
func MarshalOperation(v Operation) ([]byte, error) {
	v, e := ValidateOperation(v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseOperation(raw []byte) (Operation, error) {
	var v Operation
	if e := decode(raw, MaxOperationSize, &v, false); e != nil {
		return Operation{}, e
	}
	return ValidateOperation(v)
}

func ValidateReceipt(v Receipt) (Receipt, error) { return owned(v, MaxOperationSize, validateReceipt) }
func MarshalReceipt(v Receipt) ([]byte, error) {
	v, e := ValidateReceipt(v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseReceipt(raw []byte) (Receipt, error) {
	var v Receipt
	if e := decode(raw, MaxOperationSize, &v, false); e != nil {
		return Receipt{}, e
	}
	return ValidateReceipt(v)
}

func ValidateEventStatus(v EventStatus) (EventStatus, error) {
	return owned(v, MaxResponseSize, validateEventStatus)
}
func MarshalEventStatus(v EventStatus) ([]byte, error) {
	v, e := ValidateEventStatus(v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseEventStatus(raw []byte) (EventStatus, error) {
	var v EventStatus
	if e := decode(raw, MaxResponseSize, &v, false); e != nil {
		return EventStatus{}, e
	}
	return ValidateEventStatus(v)
}

func ValidateCheckResult(v CheckResult) (CheckResult, error) {
	return owned(v, MaxResponseSize, validateCheckResult)
}
func MarshalCheckResult(v CheckResult) ([]byte, error) {
	v, e := ValidateCheckResult(v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseCheckResult(raw []byte) (CheckResult, error) {
	var v CheckResult
	if e := decode(raw, MaxResponseSize, &v, false); e != nil {
		return CheckResult{}, e
	}
	return ValidateCheckResult(v)
}

func ValidateHistoryPage(v HistoryPage) (HistoryPage, error) {
	return owned(v, MaxResponseSize, validateHistoryPage)
}
func MarshalHistoryPage(v HistoryPage) ([]byte, error) {
	v, e := ValidateHistoryPage(v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseHistoryPage(raw []byte) (HistoryPage, error) {
	var v HistoryPage
	if e := decode(raw, MaxResponseSize, &v, false); e != nil {
		return HistoryPage{}, e
	}
	return ValidateHistoryPage(v)
}

func ValidateMutationResult(v MutationResult) (MutationResult, error) {
	return owned(v, MaxResponseSize, validateMutationResult)
}
func MarshalMutationResult(v MutationResult) ([]byte, error) {
	v, e := ValidateMutationResult(v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseMutationResult(raw []byte) (MutationResult, error) {
	var v MutationResult
	if e := decode(raw, MaxResponseSize, &v, false); e != nil {
		return MutationResult{}, e
	}
	return ValidateMutationResult(v)
}

func ValidateOperationResult(v OperationResult) (OperationResult, error) {
	return owned(v, MaxResponseSize, validateOperationResult)
}
func MarshalOperationResult(v OperationResult) ([]byte, error) {
	v, e := ValidateOperationResult(v)
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func ParseOperationResult(raw []byte) (OperationResult, error) {
	var v OperationResult
	if e := decode(raw, MaxResponseSize, &v, false); e != nil {
		return OperationResult{}, e
	}
	return ValidateOperationResult(v)
}
