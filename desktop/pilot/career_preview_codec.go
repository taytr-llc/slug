// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"encoding/json"
	"github.com/taytr-llc/slug/gymstable"
	"reflect"
)

func careerPreviewFields(c string) []string {
	switch c {
	case "career_preview_snapshot":
		return []string{}
	case "career_preview_begin":
		return []string{"view_id", "fighter_a_id", "fighter_b_id"}
	case "career_preview_job", "career_preview_cancel":
		return []string{"view_id", "job_id"}
	}
	return nil
}
func careerPreviewRequestValid(r CareerPreviewRequest) bool {
	if r.Version != 1 || r.ID == 0 || r.ID > gymstable.MaxSafeInteger || careerPreviewFields(r.Command) == nil {
		return false
	}
	switch r.Command {
	case "career_preview_snapshot":
		return r.ViewID == "" && r.FighterAID == "" && r.FighterBID == "" && r.JobID == ""
	case "career_preview_begin":
		return canonicalManifestID(r.ViewID) && canonicalManifestID(r.FighterAID) && canonicalManifestID(r.FighterBID) && r.FighterAID < r.FighterBID && r.JobID == ""
	default:
		return canonicalManifestID(r.ViewID) && canonicalManifestID(r.JobID) && r.FighterAID == "" && r.FighterBID == ""
	}
}
func MarshalCareerPreviewRequest(r CareerPreviewRequest) ([]byte, error) {
	if !careerPreviewRequestValid(r) {
		return nil, gymstable.ErrInvalid
	}
	b, e := json.Marshal(r)
	if e != nil || len(b) > 4095 {
		return nil, gymstable.ErrInvalid
	}
	return b, nil
}
func ParseCareerPreviewRequest(raw []byte) (CareerPreviewRequest, error) {
	var r CareerPreviewRequest
	fields, e := careerPreviewDecode(raw, 4095, 1, &r)
	if e != nil || !careerPreviewRequestValid(r) {
		return CareerPreviewRequest{}, gymstable.ErrInvalid
	}
	want := append([]string{"id", "version", "command"}, careerPreviewFields(r.Command)...)
	if len(want) != len(fields) {
		return CareerPreviewRequest{}, gymstable.ErrInvalid
	}
	for _, k := range want {
		if !fields[k] {
			return CareerPreviewRequest{}, gymstable.ErrInvalid
		}
	}
	return r, nil
}
func ParseCareerPreviewResponse(raw []byte) (CareerPreviewResponse, error) {
	var r CareerPreviewResponse
	fields, e := careerPreviewDecode(raw, MaxResponseSize-1, 12, &r)
	if e != nil || r.Version != 1 || r.ID == 0 || r.ID > gymstable.MaxSafeInteger || len(fields) != 4 || !fields["id"] || !fields["version"] || !fields["ok"] {
		return CareerPreviewResponse{}, gymstable.ErrInvalid
	}
	if !r.OK {
		if !fields["error"] || !careerPreviewErrorCode(r.Error) {
			return CareerPreviewResponse{}, gymstable.ErrInvalid
		}
	} else if fields["snapshot"] {
		if r.Snapshot == nil || !careerPreviewSnapshotValid(*r.Snapshot) {
			return CareerPreviewResponse{}, gymstable.ErrInvalid
		}
	} else if fields["job"] {
		if r.Job == nil || !careerPreviewJobValid(*r.Job) {
			return CareerPreviewResponse{}, gymstable.ErrInvalid
		}
	} else {
		return CareerPreviewResponse{}, gymstable.ErrInvalid
	}
	return r, nil
}

func MarshalCareerPreviewResponse(r CareerPreviewResponse) ([]byte, error) {
	budget := MaxResponseSize - 1
	if !previewBudget(reflect.ValueOf(r), 0, &budget) {
		return nil, gymstable.ErrInvalid
	}
	raw, e := json.Marshal(r)
	if e != nil {
		return nil, gymstable.ErrInvalid
	}
	if _, e = ParseCareerPreviewResponse(raw); e != nil {
		return nil, e
	}
	return raw, nil
}
