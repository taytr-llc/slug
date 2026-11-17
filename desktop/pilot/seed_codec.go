// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"encoding/json"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/gymstable"
	"io"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"
)

func seedDecode(raw []byte, limit, maxDepth int, out any) (map[string]bool, error) {
	if len(raw) == 0 || len(raw) > limit || !utf8.Valid(raw) {
		return nil, gymseed.ErrInvalid
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
			return gymseed.ErrInvalid
		}
		if t.Kind() == reflect.Pointer {
			if token == nil {
				return nil
			}
			t = t.Elem()
		}
		checkpoint := t == reflect.TypeOf(chaincfg.SignedCheckpoint{})
		if checkpoint {
			t = reflect.TypeOf(seedCheckpointWire{})
		}
		switch t.Kind() {
		case reflect.Struct:
			if token != json.Delim('{') || depth+1 > maxDepth {
				return gymseed.ErrInvalid
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
					return gymseed.ErrInvalid
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
					return gymseed.ErrInvalid
				}
			}
			end, e := d.Token()
			if e != nil || end != json.Delim('}') {
				return gymseed.ErrInvalid
			}
			if path == "" {
				root = seen
			}
		case reflect.Array, reflect.Slice:
			if token != json.Delim('[') || depth+1 > maxDepth {
				return gymseed.ErrInvalid
			}
			count := 0
			bound, exact := seedArrayBound(path)
			if bound < 0 {
				return gymseed.ErrInvalid
			}

			for d.More() {
				if count >= bound {
					return gymseed.ErrInvalid
				}
				if e := walk(t.Elem(), path+"[]", depth+1); e != nil {
					return e
				}
				count++
			}
			if exact && count != bound {
				return gymseed.ErrInvalid
			}
			end, e := d.Token()
			if e != nil || end != json.Delim(']') {
				return gymseed.ErrInvalid
			}
		case reflect.String:
			s, ok := token.(string)
			if !ok || len(s) > 4096 || (strings.Contains(path, "checkpoint.") && (strings.HasSuffix(path, ".genesis") || strings.HasSuffix(path, ".hash")) && !canonicalManifestID(s)) || (strings.Contains(path, "checkpoint.") && strings.HasSuffix(path, ".signature") && !seedHex(s, 64)) {
				return gymseed.ErrInvalid
			}
		case reflect.Bool:
			if _, ok := token.(bool); !ok {
				return gymseed.ErrInvalid
			}
		case reflect.Int, reflect.Int32, reflect.Int64:
			n, ok := token.(json.Number)
			if !ok {
				return gymseed.ErrInvalid
			}
			value, e := strconv.ParseInt(string(n), 10, t.Bits())
			if e != nil || strconv.FormatInt(value, 10) != string(n) || value > int64(gymstable.MaxSafeInteger) || value < 0 && !(value == -1 && seedSignedHeight(path)) {
				return gymseed.ErrInvalid
			}
		case reflect.Uint, reflect.Uint32, reflect.Uint64:
			n, ok := token.(json.Number)
			if !ok {
				return gymseed.ErrInvalid
			}
			value, e := strconv.ParseUint(string(n), 10, t.Bits())
			if e != nil || strconv.FormatUint(value, 10) != string(n) || value > gymstable.MaxSafeInteger {
				return gymseed.ErrInvalid
			}
		default:
			return gymseed.ErrInvalid
		}
		cap := seedNestedCap(t.Name())
		if checkpoint {
			cap = 4096
		}
		if int(d.InputOffset())-start > cap {
			return gymseed.ErrInvalid
		}
		return nil
	}
	rootType := reflect.TypeOf(out).Elem()
	rootPath := ""
	if rootType == reflect.TypeOf(SeedSnapshot{}) {
		rootPath = "snapshot"
	}
	if rootType == reflect.TypeOf(SeedJob{}) {
		rootPath = "job"
	}
	if err := walk(rootType, rootPath, 0); err != nil {
		return nil, err
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, gymseed.ErrInvalid
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return nil, gymseed.ErrInvalid
	}
	return root, nil
}

func MarshalSeedRequest(r SeedRequest) ([]byte, error) {
	if !seedRequestValid(r) {
		return nil, gymseed.ErrInvalid
	}
	raw, e := json.Marshal(r)
	if e != nil || len(raw)+1 > MaxSeedRequestSize {
		return nil, gymseed.ErrInvalid
	}
	return raw, nil
}
func ParseSeedRequest(raw []byte) (SeedRequest, error) {
	var r SeedRequest
	if _, e := seedDecode(raw, MaxSeedRequestSize-1, 16, &r); e != nil || !seedRequestValid(r) {
		return SeedRequest{}, gymseed.ErrInvalid
	}
	return r, nil
}
func ParseSeedResponse(raw []byte) (SeedResponse, error) {
	var r SeedResponse
	if _, e := seedDecode(raw, MaxSeedResponseSize-1, 16, &r); e != nil || !seedResponseValid(&r) {
		return SeedResponse{}, gymseed.ErrInvalid
	}
	return r, nil
}
func MarshalSeedResponse(r SeedResponse) ([]byte, error) {
	// Typed collection bounds precede JSON allocation; the lexical pass then
	// enforces exact paths and closed transport validation on an owned copy.
	budget := MaxSeedResponseSize - 1
	if !seedResponseBudget(reflect.ValueOf(r), &budget, 0) {
		return nil, gymseed.ErrInvalid
	}
	raw, e := json.Marshal(r)
	if e != nil {
		return nil, gymseed.ErrInvalid
	}
	owned, e := ParseSeedResponse(raw)
	if e != nil {
		return nil, e
	}
	return json.Marshal(owned)
}
func seedResponseBudget(v reflect.Value, budget *int, depth int) bool {
	spend := func(n int) bool {
		if n < 0 || n > *budget {
			return false
		}
		*budget -= n
		return true
	}
	if v.Type() == reflect.TypeOf(chaincfg.SignedCheckpoint{}) {
		c := v.Interface().(chaincfg.SignedCheckpoint)
		if len(c.Signature) != 64 {
			return false
		}
		b, e := json.Marshal(c)
		return e == nil && len(b) <= 4096 && spend(len(b))
	}
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return spend(4)
		}
		return seedResponseBudget(v.Elem(), budget, depth)
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
			if !spend(len(f.Tag.Get("json"))+4) || !seedResponseBudget(v.Field(i), budget, depth+1) {
				return false
			}
		}
	case reflect.Array, reflect.Slice:
		if depth >= 16 || v.Len() > 20 || !spend(2) {
			return false
		}
		for i := 0; i < v.Len(); i++ {
			if !spend(1) || !seedResponseBudget(v.Index(i), budget, depth+1) {
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

func seedNestedCap(name string) int {
	switch name {
	case "Record":
		return 229376
	case "SeedJob":
		return MaxSeedJobSize
	case "Pin":
		return 16384
	case "Status":
		return 8192
	case "Declaration":
		return 4096
	case "SignedDeclaration":
		return 5120
	case "Publication":
		return 4096
	case "ScanRestart":
		return 1024
	case "Conflict":
		return 8192
	case "SignedCheckpoint":
		return 4096
	}
	if n := eventNestedCap(name); n < MaxEventResponseSize {
		return n
	}
	return MaxSeedResponseSize
}
func seedRecordPath(path string) (string, bool) {
	for _, prefix := range []string{"record.", "job.result."} {
		if strings.HasPrefix(path, prefix) {
			return strings.TrimPrefix(path, prefix), true
		}
	}
	return "", false
}
func seedArrayBound(path string) (int, bool) {
	if path == "snapshot.pin.source_pin.participants" {
		return 2, true
	}
	if path == "history.events" {
		return 8, false
	}
	r, ok := seedRecordPath(path)
	if !ok {
		return -1, false
	}
	switch r {
	case "source_receipt.input.participants", "source_receipt.captures":
		return 2, true
	case "source_receipt.input.participants[].roster.slots", "source_receipt.input.participants[].roster.assigned_at_ms":
		return 10, true
	case "source_receipt.input.participants[].members", "source_receipt.input.participants[].entry.latest_receipt.members":
		return 5, false
	case "anchor.merkle_branch":
		return 20, false
	case "scan_restarts":
		return 16, false
	case "conflicts":
		return 8, false
	}
	return -1, false
}
func seedSignedHeight(path string) bool {
	if path == "status.scan_height" || path == "status.scan_tip_height" || path == "history.events[].scan_height" || path == "history.events[].scan_tip_height" {
		return true
	}
	r, ok := seedRecordPath(path)
	if !ok {
		return false
	}
	return r == "cursor.height" || r == "cursor.complete_to_height" || r == "scan_restarts[].previous.height" || r == "scan_restarts[].previous.complete_to_height"
}

type seedCheckpointWire struct {
	Version   uint32 `json:"version"`
	Network   uint32 `json:"network"`
	Genesis   string `json:"genesis"`
	Height    int32  `json:"height"`
	Hash      string `json:"hash"`
	Signature string `json:"signature"`
}
