// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"encoding/json"
	"github.com/taytr-llc/slug/gymstable"
	"reflect"
)

func historyRequestValid(r CareerHistoryRequest) bool {
	if r.Version != 1 || r.ID == 0 || r.ID > gymstable.MaxSafeInteger || r.After > 256 || r.Limit > 8 {
		return false
	}
	switch r.Command {
	case "career_history_snapshot":
		return r.ViewID == "" && r.ProspectID == "" && r.JobID == "" && r.After == 0 && r.Limit == 0
	case "career_history_begin":
		return canonicalManifestID(r.ViewID) && canonicalManifestID(r.ProspectID) && r.JobID == "" && r.After == 0 && r.Limit == 0
	case "career_history_job", "career_history_cancel":
		return canonicalManifestID(r.ViewID) && canonicalManifestID(r.JobID) && r.ProspectID == "" && r.After == 0 && r.Limit == 0
	case "career_history_page":
		return canonicalManifestID(r.ViewID) && canonicalManifestID(r.JobID) && r.ProspectID == "" && r.Limit >= 1
	}
	return false
}
func ParseCareerHistoryRequest(raw []byte) (CareerHistoryRequest, error) {
	var r CareerHistoryRequest
	_, e := careerHistoryDecode(raw, MaxCareerHistoryRequestSize-1, 1, &r)
	if e != nil || !historyRequestValid(r) {
		return CareerHistoryRequest{}, gymstable.ErrInvalid
	}
	return r, nil
}
func MarshalCareerHistoryRequest(r CareerHistoryRequest) ([]byte, error) {
	if !historyRequestValid(r) {
		return nil, gymstable.ErrInvalid
	}
	b, e := json.Marshal(r)
	if e != nil || len(b) >= MaxCareerHistoryRequestSize {
		return nil, gymstable.ErrInvalid
	}
	return b, nil
}
func historyResponseValid(r CareerHistoryResponse) bool {
	if r.ID == 0 || r.ID > gymstable.MaxSafeInteger || r.Version != 1 {
		return false
	}
	if !r.OK {
		return r.Snapshot == nil && r.Job == nil && r.Page == nil && careerHistoryErrorCode(r.Error)
	}
	if r.Error != "" {
		return false
	}
	n := 0
	if r.Snapshot != nil {
		n++
		if ValidateCareerHistorySnapshot(*r.Snapshot) != nil {
			return false
		}
	}
	if r.Job != nil {
		n++
		if ValidateCareerHistoryJob(*r.Job) != nil {
			return false
		}
	}
	if r.Page != nil {
		n++
		if ValidateCareerHistoryPageResult(*r.Page) != nil {
			return false
		}
	}
	return n == 1
}
func ParseCareerHistoryResponse(raw []byte) (CareerHistoryResponse, error) {
	var r CareerHistoryResponse
	_, e := careerHistoryDecode(raw, MaxCareerHistoryResponseSize-1, 12, &r)
	if e != nil || !historyResponseValid(r) {
		return CareerHistoryResponse{}, gymstable.ErrInvalid
	}
	return r, nil
}
func MarshalCareerHistoryResponse(r CareerHistoryResponse) ([]byte, error) {
	budget := MaxCareerHistoryResponseSize - 1
	if !historyBudget(reflect.ValueOf(r), 0, &budget) || !historyResponseValid(r) {
		return nil, gymstable.ErrInvalid
	}
	b, e := json.Marshal(r)
	if e != nil || len(b) >= MaxCareerHistoryResponseSize {
		return nil, gymstable.ErrInvalid
	}
	return b, nil
}
