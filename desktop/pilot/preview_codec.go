// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"encoding/json"
	"github.com/taytr-llc/slug/fighterbout"
	"github.com/taytr-llc/slug/gymstable"
	"reflect"
)

func previewFields(c string) []string {
	switch c {
	case "preview_snapshot":
		return []string{}
	case "preview_begin":
		return []string{"view_id", "fighter_a_id", "fighter_b_id"}
	case "preview_job", "preview_cancel":
		return []string{"view_id", "job_id"}
	}
	return nil
}
func previewRequestValid(r PreviewRequest) bool {
	if r.Version != 1 || r.ID == 0 || r.ID > gymstable.MaxSafeInteger || previewFields(r.Command) == nil {
		return false
	}
	switch r.Command {
	case "preview_snapshot":
		return r.ViewID == "" && r.FighterAID == "" && r.FighterBID == "" && r.JobID == ""
	case "preview_begin":
		return canonicalManifestID(r.ViewID) && canonicalManifestID(r.FighterAID) && canonicalManifestID(r.FighterBID) && r.FighterAID < r.FighterBID && r.JobID == ""
	default:
		return canonicalManifestID(r.ViewID) && canonicalManifestID(r.JobID) && r.FighterAID == "" && r.FighterBID == ""
	}
}
func MarshalPreviewRequest(r PreviewRequest) ([]byte, error) {
	if !previewRequestValid(r) {
		return nil, gymstable.ErrInvalid
	}
	b, e := json.Marshal(r)
	if e != nil || len(b) > 4096 {
		return nil, gymstable.ErrInvalid
	}
	return b, nil
}
func ParsePreviewRequest(raw []byte) (PreviewRequest, error) {
	var r PreviewRequest
	fields, e := previewDecode(raw, 4096, 1, &r)
	if e != nil || !previewRequestValid(r) {
		return PreviewRequest{}, gymstable.ErrInvalid
	}
	want := append([]string{"id", "version", "command"}, previewFields(r.Command)...)
	if len(want) != len(fields) {
		return PreviewRequest{}, gymstable.ErrInvalid
	}
	for _, k := range want {
		if !fields[k] {
			return PreviewRequest{}, gymstable.ErrInvalid
		}
	}
	return r, nil
}
func ParsePreviewResponse(raw []byte) (PreviewResponse, error) {
	var r PreviewResponse
	fields, e := previewDecode(raw, MaxResponseSize-1, 12, &r)
	if e != nil || r.Version != 1 || r.ID == 0 || r.ID > gymstable.MaxSafeInteger || len(fields) != 4 || !fields["id"] || !fields["version"] || !fields["ok"] {
		return PreviewResponse{}, gymstable.ErrInvalid
	}
	if !r.OK {
		if !fields["error"] || !previewErrorCode(r.Error) {
			return PreviewResponse{}, gymstable.ErrInvalid
		}
	} else if fields["snapshot"] {
		if r.Snapshot == nil || !previewSnapshotValid(*r.Snapshot) {
			return PreviewResponse{}, gymstable.ErrInvalid
		}
	} else if fields["job"] {
		if r.Job == nil || !previewJobValid(*r.Job) {
			return PreviewResponse{}, gymstable.ErrInvalid
		}
	} else {
		return PreviewResponse{}, gymstable.ErrInvalid
	}
	return r, nil
}

// The budget walk precedes serialization of caller-owned slices/strings. Typed
// fixed arrays are bounded by their declaration; only rounds/ticks are slices.
func previewBudget(v reflect.Value, depth int, budget *int) bool {
	spend := func(n int) bool {
		if n < 0 || n > *budget {
			return false
		}
		*budget -= n
		return true
	}
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return spend(4)
		}
		return previewBudget(v.Elem(), depth, budget)
	case reflect.Struct:
		if depth >= 12 || !spend(2) {
			return false
		}
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			f := t.Field(i)
			if f.PkgPath != "" {
				continue
			}
			if !spend(len(f.Tag.Get("json"))+4) || !previewBudget(v.Field(i), depth+1, budget) {
				return false
			}
		}
	case reflect.Array, reflect.Slice:
		bound := v.Len()
		if v.Kind() == reflect.Slice {
			switch v.Type().Elem() {
			case reflect.TypeOf(fighterbout.Round{}):
				bound = 3
			case reflect.TypeOf(fighterbout.Tick{}):
				bound = 60
			default:
				return false
			}
		}
		if depth >= 12 || v.Len() > bound || !spend(2) {
			return false
		}
		for i := 0; i < v.Len(); i++ {
			if !spend(1) || !previewBudget(v.Index(i), depth+1, budget) {
				return false
			}
		}
	case reflect.String:
		return v.Len() <= 4096 && spend(v.Len()*6+2)
	case reflect.Bool:
		return spend(5)
	case reflect.Int, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint32, reflect.Uint64:
		return spend(24)
	default:
		return false
	}
	return true
}
func MarshalPreviewResponse(r PreviewResponse) ([]byte, error) {
	budget := MaxResponseSize - 1
	if !previewBudget(reflect.ValueOf(r), 0, &budget) {
		return nil, gymstable.ErrInvalid
	}
	raw, e := json.Marshal(r)
	if e != nil {
		return nil, gymstable.ErrInvalid
	}
	if _, e = ParsePreviewResponse(raw); e != nil {
		return nil, e
	}
	return raw, nil
}
