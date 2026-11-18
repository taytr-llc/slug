// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"encoding/json"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymstable"
	"io"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"
)

func boutDecode(raw []byte, limit, maxDepth int, out any) (map[string]bool, error) {
	if len(raw) == 0 || len(raw) > limit || !utf8.Valid(raw) {
		return nil, gymbout.ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	root := map[string]bool{}
	var walk func(reflect.Type, string, int) error
	walk = func(t reflect.Type, path string, depth int) error {
		start := int(d.InputOffset())
		for start < len(raw) && (raw[start] == ':' || raw[start] == ',' || raw[start] == ' ' || raw[start] == '\t' || raw[start] == '\r' || raw[start] == '\n') {
			start++
		}
		token, err := d.Token()
		if err != nil {
			return gymbout.ErrInvalid
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
				return gymbout.ErrInvalid
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
					return gymbout.ErrInvalid
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
					return gymbout.ErrInvalid
				}
			}
			end, e := d.Token()
			if e != nil || end != json.Delim('}') {
				return gymbout.ErrInvalid
			}
			if path == "" {
				root = seen
			}
		case reflect.Array, reflect.Slice:
			if token != json.Delim('[') || depth+1 > maxDepth {
				return gymbout.ErrInvalid
			}
			count := 0
			bound, exact := boutArrayBound(path)
			if bound < 0 {
				return gymbout.ErrInvalid
			}

			for d.More() {
				if count >= bound {
					return gymbout.ErrInvalid
				}
				if e := walk(t.Elem(), path+"[]", depth+1); e != nil {
					return e
				}
				count++
			}
			if exact && count != bound {
				return gymbout.ErrInvalid
			}
			end, e := d.Token()
			if e != nil || end != json.Delim(']') {
				return gymbout.ErrInvalid
			}
		case reflect.String:
			s, ok := token.(string)
			for _, b := range []byte(s) {
				if b > 127 {
					return gymbout.ErrInvalid
				}
			}
			if !ok || len(s) > 4096 {
				return gymbout.ErrInvalid
			}
		case reflect.Bool:
			if _, ok := token.(bool); !ok {
				return gymbout.ErrInvalid
			}
		case reflect.Int, reflect.Int32, reflect.Int64:
			n, ok := token.(json.Number)
			if !ok {
				return gymbout.ErrInvalid
			}
			value, e := strconv.ParseInt(string(n), 10, t.Bits())
			if e != nil || strconv.FormatInt(value, 10) != string(n) || value > int64(gymstable.MaxSafeInteger) || value < 0 && !boutSignedNumber(path, value) {
				return gymbout.ErrInvalid
			}
		case reflect.Uint, reflect.Uint32, reflect.Uint64:
			n, ok := token.(json.Number)
			if !ok {
				return gymbout.ErrInvalid
			}
			value, e := strconv.ParseUint(string(n), 10, t.Bits())
			if e != nil || strconv.FormatUint(value, 10) != string(n) || value > gymstable.MaxSafeInteger {
				return gymbout.ErrInvalid
			}
		default:
			return gymbout.ErrInvalid
		}
		cap := boutNestedCap(t.Name())
		if int(d.InputOffset())-start > cap {
			return gymbout.ErrInvalid
		}
		return nil
	}
	rootType := reflect.TypeOf(out).Elem()
	rootPath := ""
	if rootType == reflect.TypeOf(BoutSnapshot{}) {
		rootPath = "snapshot"
	}
	if rootType == reflect.TypeOf(BoutJob{}) {
		rootPath = "job"
	}
	if err := walk(rootType, rootPath, 0); err != nil {
		return nil, err
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, gymbout.ErrInvalid
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return nil, gymbout.ErrInvalid
	}
	return root, nil
}

func MarshalBoutRequest(r BoutRequest) ([]byte, error) {
	if !boutRequestValid(r) {
		return nil, gymbout.ErrInvalid
	}
	raw, e := json.Marshal(r)
	if e != nil || len(raw)+1 > MaxBoutRequestSize {
		return nil, gymbout.ErrInvalid
	}
	return raw, nil
}
func ParseBoutRequest(raw []byte) (BoutRequest, error) {
	var r BoutRequest
	if _, e := boutDecode(raw, MaxBoutRequestSize-1, 16, &r); e != nil || !boutRequestValid(r) {
		return BoutRequest{}, gymbout.ErrInvalid
	}
	return r, nil
}
func ParseBoutResponse(raw []byte) (BoutResponse, error) {
	var r BoutResponse
	if _, e := boutDecode(raw, MaxBoutResponseSize-1, 16, &r); e != nil || !boutResponseValid(&r) {
		return BoutResponse{}, gymbout.ErrInvalid
	}
	return r, nil
}
func MarshalBoutResponse(r BoutResponse) ([]byte, error) {
	// Typed collection bounds precede JSON allocation; the lexical pass then
	// enforces exact paths and closed transport validation on an owned copy.
	budget := MaxBoutResponseSize - 1
	if !boutResponseBudget(reflect.ValueOf(r), &budget, 0) {
		return nil, gymbout.ErrInvalid
	}
	raw, e := json.Marshal(r)
	if e != nil {
		return nil, gymbout.ErrInvalid
	}
	owned, e := ParseBoutResponse(raw)
	if e != nil {
		return nil, e
	}
	return json.Marshal(owned)
}
func boutResponseBudget(v reflect.Value, budget *int, depth int) bool {
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
		return boutResponseBudget(v.Elem(), budget, depth)
	case reflect.Struct:
		if depth >= 16 || !spend(2) {
			return false
		}
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			f := t.Field(i)
			if f.PkgPath != "" || f.Tag.Get("json") == "-" {
				continue
			}
			if !spend(len(f.Tag.Get("json"))+4) || !boutResponseBudget(v.Field(i), budget, depth+1) {
				return false
			}
		}
	case reflect.Array, reflect.Slice:
		if depth >= 16 || v.Len() > 60 || !spend(2) {
			return false
		}
		for i := 0; i < v.Len(); i++ {
			if !spend(1) || !boutResponseBudget(v.Index(i), budget, depth+1) {
				return false
			}
		}
	case reflect.String:
		s := v.String()
		if len(s) > 4096 || !utf8.ValidString(s) {
			return false
		}
		raw, err := json.Marshal(s)
		return err == nil && spend(len(raw))
	case reflect.Bool:
		return spend(5)
	case reflect.Int, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint32, reflect.Uint64:
		return spend(32)
	default:
		return false
	}
	return true
}

func boutNestedCap(name string) int {
	switch name {
	case "Record":
		return 237568
	case "RunResult", "ReadResult":
		return 249856
	case "BoutJob":
		return MaxBoutJobSize
	case "Pin":
		return 24576
	case "BoutSource":
		return 16384
	case "BoutState":
		return 4096
	case "Input":
		return 16384
	case "Result":
		return 196608
	case "SavedBout":
		return 2048
	case "Status":
		return 8192
	case "CardStatus":
		return 16384
	case "Manifest":
		return 8192
	}
	return MaxBoutResponseSize
}
func boutRecordPath(path string) (string, bool) {
	for _, p := range []string{"record.record.", "job.result.record."} {
		if strings.HasPrefix(path, p) {
			return strings.TrimPrefix(path, p), true
		}
	}
	return "", false
}
func boutArrayBound(path string) (int, bool) {
	if path == "snapshot.pin.source_pin.source_pin.source_pin.participants" {
		return 2, true
	}
	if path == "card_status.bouts" {
		return 5, false
	}
	r, ok := boutRecordPath(path)
	if !ok {
		return -1, false
	}
	switch r {
	case "source.corners", "result.initial", "result.final", "result.scores", "result.rounds[].ticks[].states", "result.rounds[].end", "result.rounds[].recovery.states", "result.rounds[].recovery.corner_stopped":
		return 2, true
	case "result.rounds":
		return 3, false
	case "result.rounds[].ticks":
		return 60, false
	}
	return -1, false
}
func boutSignedNumber(path string, value int64) bool {
	r, ok := boutRecordPath(path)
	if !ok {
		return false
	}
	switch r {
	case "input.a.manifest.portrait.shoulder_jitter", "input.b.manifest.portrait.shoulder_jitter":
		return value >= -6 && value <= 6
	case "result.initial[].form_q":
		return value >= -100000 && value <= 100000
	}
	return false
}
