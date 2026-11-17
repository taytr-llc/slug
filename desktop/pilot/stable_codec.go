// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"encoding/json"
	"io"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/taytr-llc/slug/gymstable"
)

func stableFields(command string) []string {
	switch command {
	case "stable_snapshot":
		return []string{}
	case "stable_sync", "stable_pending", "stable_recent":
		return []string{"view_id"}
	case "stable_inventory":
		return []string{"view_id", "after", "limit"}
	case "stable_roster":
		return []string{"view_id", "first_slot", "limit"}
	case "stable_prepare_slot":
		return []string{"view_id", "expected_revision", "slot", "prospect_id"}
	case "stable_apply_slot", "stable_operation", "stable_abandon_operation":
		return []string{"view_id", "operation_id"}
	case "stable_job", "stable_cancel":
		return []string{"view_id", "job_id"}
	}
	return nil
}
func stableRequestValid(r StableRequest) bool {
	fields := stableFields(r.Command)
	if fields == nil || r.ID == 0 || r.ID > gymstable.MaxSafeInteger {
		return false
	}
	if r.Command != "stable_snapshot" && !canonicalManifestID(r.ViewID) {
		return false
	}
	switch r.Command {
	case "stable_inventory":
		return r.After <= gymstable.MaxAssignments && r.Limit >= 1 && r.Limit <= 8
	case "stable_roster":
		return r.FirstSlot >= 1 && r.FirstSlot <= 10 && r.Limit >= 1 && r.Limit <= 8
	case "stable_prepare_slot":
		_, err := gymstable.ValidateSlotRequest(gymstable.SlotRequest{ExpectedRevision: r.ExpectedRevision, Slot: r.Slot, ProspectID: r.ProspectID})
		return err == nil
	case "stable_apply_slot", "stable_operation", "stable_abandon_operation":
		return canonicalManifestID(r.OperationID)
	case "stable_job", "stable_cancel":
		return canonicalManifestID(r.JobID)
	}
	return true
}

// MarshalStableRequest emits every selected field, including required zero and
// empty clear values. A plain omitempty marshal is not this wire contract.
func MarshalStableRequest(r StableRequest) ([]byte, error) {
	if !stableRequestValid(r) {
		return nil, gymstable.ErrInvalid
	}
	keys := append([]string{"id", "command"}, stableFields(r.Command)...)
	allowed := map[string]bool{}
	for _, k := range keys {
		allowed[k] = true
	}
	v := reflect.ValueOf(r)
	typ := v.Type()
	values := map[string]reflect.Value{}
	for i := 0; i < v.NumField(); i++ {
		name := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]
		values[name] = v.Field(i)
		if !allowed[name] && !v.Field(i).IsZero() {
			return nil, gymstable.ErrInvalid
		}
	}
	var out bytes.Buffer
	out.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			out.WriteByte(',')
		}
		name, _ := json.Marshal(k)
		value, err := json.Marshal(values[k].Interface())
		if err != nil {
			return nil, gymstable.ErrInvalid
		}
		out.Write(name)
		out.WriteByte(':')
		out.Write(value)
	}
	out.WriteByte('}')
	if out.Len() > gymstable.MaxRequestSize {
		return nil, gymstable.ErrInvalid
	}
	return out.Bytes(), nil
}
func ParseStableRequest(raw []byte) (StableRequest, error) {
	var r StableRequest
	fields, err := stableDecode(raw, gymstable.MaxRequestSize, 2, &r)
	if err != nil || !stableRequestValid(r) {
		return StableRequest{}, gymstable.ErrInvalid
	}
	allowed := append([]string{"id", "command"}, stableFields(r.Command)...)
	if len(fields) != len(allowed) {
		return StableRequest{}, gymstable.ErrInvalid
	}
	for _, key := range allowed {
		if !fields[key] {
			return StableRequest{}, gymstable.ErrInvalid
		}
	}
	return r, nil
}
func ParseStableResponse(raw []byte) (StableResponse, error) {
	var r StableResponse
	fields, err := stableDecode(raw, gymstable.MaxResponseSize-1, 9, &r)
	if err != nil || r.ID == 0 || r.ID > gymstable.MaxSafeInteger || len(fields) != 3 || !fields["id"] || !fields["ok"] {
		return StableResponse{}, gymstable.ErrInvalid
	}
	if !r.OK {
		if !fields["error"] || !stableErrorCode(r.Error) {
			return StableResponse{}, gymstable.ErrInvalid
		}
	} else {
		valid := false
		switch {
		case fields["snapshot"]:
			valid = r.Snapshot != nil && stableSnapshotValid(*r.Snapshot)
		case fields["job"]:
			valid = r.Job != nil && stableJobValid(*r.Job)
		case fields["operation"]:
			valid = r.Operation != nil && stableOperationValid(*r.Operation)
		}
		if !valid {
			return StableResponse{}, gymstable.ErrInvalid
		}
	}
	return r, nil
}

// The token walk rejects duplicate decoded keys, unknown fields, null scalar
// values and oversized collections before typed allocation. Depth counts only
// containers, not scalar tokens or Go pointers. It is not an authority check.
func stableDecode(raw []byte, limit, maxDepth int, out any) (map[string]bool, error) {
	if len(raw) == 0 || len(raw) > limit || !utf8.Valid(raw) {
		return nil, gymstable.ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	root := map[string]bool{}
	var walk func(reflect.Type, string, int) error
	walk = func(t reflect.Type, path string, depth int) error {
		token, err := d.Token()
		if err != nil {
			return gymstable.ErrInvalid
		}
		if t.Kind() == reflect.Pointer {
			if token == nil {
				return nil
			}
			t = t.Elem()
		}
		switch t.Kind() {
		case reflect.Struct:
			if token != json.Delim('{') || depth+1 > maxDepth {
				return gymstable.ErrInvalid
			}
			names := map[string]reflect.StructField{}
			required := map[string]bool{}
			for i := 0; i < t.NumField(); i++ {
				f := t.Field(i)
				if f.PkgPath != "" {
					continue
				}
				tag := strings.Split(f.Tag.Get("json"), ",")
				if tag[0] == "-" {
					continue
				}
				names[tag[0]] = f
				if len(tag) == 1 || tag[1] != "omitempty" {
					required[tag[0]] = true
				}
			}
			seen := map[string]bool{}
			for d.More() {
				key, e := d.Token()
				name, ok := key.(string)
				f, known := names[name]
				if e != nil || !ok || len(name) > 64 || !known || seen[name] {
					return gymstable.ErrInvalid
				}
				seen[name] = true
				next := name
				if path != "" {
					next = path + "." + name
				}
				if e = walk(f.Type, next, depth+1); e != nil {
					return e
				}
			}
			for name := range required {
				if !seen[name] {
					return gymstable.ErrInvalid
				}
			}
			end, e := d.Token()
			if e != nil || end != json.Delim('}') {
				return gymstable.ErrInvalid
			}
			if path == "" {
				root = seen
			}
		case reflect.Array, reflect.Slice:
			if token != json.Delim('[') || depth+1 > maxDepth {
				return gymstable.ErrInvalid
			}
			count := 0
			bound := 8
			if t.Kind() == reflect.Array {
				bound = t.Len()
			}
			for d.More() {
				if count >= bound {
					return gymstable.ErrInvalid
				}
				if e := walk(t.Elem(), path+"[]", depth+1); e != nil {
					return e
				}
				count++
			}
			if t.Kind() == reflect.Array && count != t.Len() {
				return gymstable.ErrInvalid
			}
			end, e := d.Token()
			if e != nil || end != json.Delim(']') {
				return gymstable.ErrInvalid
			}
		case reflect.String:
			s, ok := token.(string)
			if !ok || len(s) > 4096 {
				return gymstable.ErrInvalid
			}
		case reflect.Bool:
			if _, ok := token.(bool); !ok {
				return gymstable.ErrInvalid
			}
		case reflect.Int, reflect.Int32, reflect.Int64:
			n, ok := token.(json.Number)
			if !ok {
				return gymstable.ErrInvalid
			}
			value, e := strconv.ParseInt(string(n), 10, t.Bits())
			signed := strings.HasSuffix(path, ".manifest.portrait.shoulder_jitter")
			if e != nil || strconv.FormatInt(value, 10) != string(n) || value > int64(gymstable.MaxSafeInteger) || value < 0 && (!signed || value < -6) || signed && value > 6 {
				return gymstable.ErrInvalid
			}
		case reflect.Uint, reflect.Uint32, reflect.Uint64:
			n, ok := token.(json.Number)
			if !ok {
				return gymstable.ErrInvalid
			}
			value, e := strconv.ParseUint(string(n), 10, t.Bits())
			if e != nil || strconv.FormatUint(value, 10) != string(n) || value > gymstable.MaxSafeInteger {
				return gymstable.ErrInvalid
			}
		default:
			return gymstable.ErrInvalid
		}
		return nil
	}
	if err := walk(reflect.TypeOf(out).Elem(), "", 0); err != nil {
		return nil, err
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, gymstable.ErrInvalid
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return nil, gymstable.ErrInvalid
	}
	return root, nil
}
