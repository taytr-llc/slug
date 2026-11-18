// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"encoding/json"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/gymresult"
	"github.com/taytr-llc/slug/gymstable"
	"io"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"
)

func resultDecode(raw []byte, limit, maxDepth int, out any) (map[string]bool, error) {
	if len(raw) == 0 || len(raw) > limit || !utf8.Valid(raw) || !resultJSONSurrogates(raw) {
		return nil, gymresult.ErrInvalid
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
			return gymresult.ErrInvalid
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
				return gymresult.ErrInvalid
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
					return gymresult.ErrInvalid
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
					return gymresult.ErrInvalid
				}
			}
			end, e := d.Token()
			if e != nil || end != json.Delim('}') {
				return gymresult.ErrInvalid
			}
			if path == "" {
				root = seen
			}
		case reflect.Array, reflect.Slice:
			if token != json.Delim('[') || depth+1 > maxDepth {
				return gymresult.ErrInvalid
			}
			count := 0
			bound, exact := resultArrayBound(path)
			if bound < 0 {
				return gymresult.ErrInvalid
			}

			for d.More() {
				if count >= bound {
					return gymresult.ErrInvalid
				}
				if e := walk(t.Elem(), path+"[]", depth+1); e != nil {
					return e
				}
				count++
			}
			if exact && count != bound {
				return gymresult.ErrInvalid
			}
			end, e := d.Token()
			if e != nil || end != json.Delim(']') {
				return gymresult.ErrInvalid
			}
		case reflect.String:
			s, ok := token.(string)
			if !ok || len(s) > 4096 || path != "path" && !resultASCII(s) || path == "path" && s != "" && !resultPathValid(s) || (strings.Contains(path, "checkpoint.") && (strings.HasSuffix(path, ".genesis") || strings.HasSuffix(path, ".hash")) && !canonicalManifestID(s)) || (strings.Contains(path, "checkpoint.") && strings.HasSuffix(path, ".signature") && !seedHex(s, 64)) {
				return gymresult.ErrInvalid
			}
		case reflect.Bool:
			if _, ok := token.(bool); !ok {
				return gymresult.ErrInvalid
			}
		case reflect.Int, reflect.Int32, reflect.Int64:
			n, ok := token.(json.Number)
			if !ok {
				return gymresult.ErrInvalid
			}
			value, e := strconv.ParseInt(string(n), 10, t.Bits())
			if e != nil || strconv.FormatInt(value, 10) != string(n) || value > int64(gymstable.MaxSafeInteger) || value < 0 && !(value == -1 && resultSignedHeight(path)) {
				return gymresult.ErrInvalid
			}
		case reflect.Uint, reflect.Uint32, reflect.Uint64:
			n, ok := token.(json.Number)
			if !ok {
				return gymresult.ErrInvalid
			}
			value, e := strconv.ParseUint(string(n), 10, t.Bits())
			if e != nil || strconv.FormatUint(value, 10) != string(n) || value > gymstable.MaxSafeInteger {
				return gymresult.ErrInvalid
			}
		default:
			return gymresult.ErrInvalid
		}
		cap := resultNestedCap(t.Name())
		if checkpoint {
			cap = 4096
		}
		if int(d.InputOffset())-start > cap {
			return gymresult.ErrInvalid
		}
		return nil
	}
	rootType := reflect.TypeOf(out).Elem()
	rootPath := ""
	if rootType == reflect.TypeOf(ResultSnapshot{}) {
		rootPath = "snapshot"
	}
	if rootType == reflect.TypeOf(ResultJob{}) {
		rootPath = "job"
	}
	if err := walk(rootType, rootPath, 0); err != nil {
		return nil, err
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, gymresult.ErrInvalid
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return nil, gymresult.ErrInvalid
	}
	return root, nil
}

func MarshalResultRequest(r ResultRequest) ([]byte, error) {
	if !resultRequestValid(r) {
		return nil, gymresult.ErrInvalid
	}
	raw, e := json.Marshal(r)
	if e != nil || len(raw)+1 > MaxResultRequestSize {
		return nil, gymresult.ErrInvalid
	}
	return raw, nil
}
func ParseResultRequest(raw []byte) (ResultRequest, error) {
	var r ResultRequest
	if _, e := resultDecode(raw, MaxResultRequestSize-1, 20, &r); e != nil || !resultRequestValid(r) {
		return ResultRequest{}, gymresult.ErrInvalid
	}
	return r, nil
}
func ParseResultResponse(raw []byte) (ResultResponse, error) {
	var r ResultResponse
	if _, e := resultDecode(raw, MaxResultResponseSize-1, 20, &r); e != nil || !resultResponseValid(&r) {
		return ResultResponse{}, gymresult.ErrInvalid
	}
	return r, nil
}
func MarshalResultResponse(r ResultResponse) ([]byte, error) {
	// Typed collection bounds precede JSON allocation; the lexical pass then
	// enforces exact paths and closed transport validation on an owned copy.
	budget := MaxResultResponseSize - 1
	if !resultResponseBudget(reflect.ValueOf(r), &budget, 0) {
		return nil, gymresult.ErrInvalid
	}
	raw, e := json.Marshal(r)
	if e != nil {
		return nil, gymresult.ErrInvalid
	}
	owned, e := ParseResultResponse(raw)
	if e != nil {
		return nil, e
	}
	return json.Marshal(owned)
}
func resultResponseBudget(v reflect.Value, budget *int, depth int) bool {
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
		return resultResponseBudget(v.Elem(), budget, depth)
	case reflect.Struct:
		if depth >= 20 || !spend(2) {
			return false
		}
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			f := t.Field(i)
			if f.PkgPath != "" || f.Tag.Get("json") == "-" {
				continue
			}
			if !spend(len(f.Tag.Get("json"))+4) || !resultResponseBudget(v.Field(i), budget, depth+1) {
				return false
			}
		}
	case reflect.Array, reflect.Slice:
		if depth >= 20 || v.Len() > 20 || !spend(2) {
			return false
		}
		for i := 0; i < v.Len(); i++ {
			if !spend(1) || !resultResponseBudget(v.Index(i), budget, depth+1) {
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

func resultNestedCap(name string) int {
	switch name {
	case "Pin":
		return 32768
	case "Info":
		return 36864
	case "ResultSource":
		return 20480
	case "Statement":
		return 24576
	case "SignedStatement":
		return 25600
	case "Publication":
		return 4096
	case "Anchor":
		return 2048
	case "Inclusion":
		return 3072
	case "PriorInclusion":
		return 4096
	case "Finality":
		return 8192
	case "ScanRestart":
		return 1024
	case "Conflict":
		return 2048
	case "Record":
		return 196608
	case "Status":
		return 16384
	case "ActionResult", "ReadResult":
		return 229376
	case "ProofSummary":
		return 2048
	case "ResultJob":
		return MaxResultJobSize
	case "ResultSnapshot":
		return MaxResultMetadataSize
	case "BoutSource":
		return 16384
	case "BoutState":
		return 4096
	case "SavedBout":
		return 2048
	case "Tip", "Outpoint":
		return 256
	case "Cursor":
		return 512
	case "FundingCoin", "Observation":
		return 1024
	}
	return MaxResultResponseSize
}
func resultRecordPath(path string) (string, bool) {
	for _, pre := range []string{"record.record.", "job.result.record."} {
		if strings.HasPrefix(path, pre) {
			return strings.TrimPrefix(path, pre), true
		}
	}
	return "", false
}
func resultArrayBound(path string) (int, bool) {
	if path == "snapshot.pin.source_pin.source_pin.source_pin.source_pin.participants" {
		return 2, true
	}
	r, ok := resultRecordPath(path)
	if !ok {
		return -1, false
	}
	switch r {
	case "statement.statement.source.source.corners", "equivocation.statement.source.source.corners":
		return 2, true
	case "scan_restarts", "prior_inclusions":
		return 16, false
	case "conflicts":
		return 8, false
	case "inclusion.anchor.merkle_branch", "prior_inclusions[].inclusion.anchor.merkle_branch", "finality.inclusion.anchor.merkle_branch":
		return 20, false
	}
	return -1, false
}
func resultSignedHeight(path string) bool {
	for _, prefix := range []string{"status.", "record.status.", "job.result.status."} {
		if path == prefix+"scan_height" || path == prefix+"scan_tip_height" {
			return true
		}
	}
	r, ok := resultRecordPath(path)
	if !ok {
		return false
	}
	return r == "cursor.height" || r == "cursor.complete_to_height" || r == "scan_restarts[].previous.height" || r == "scan_restarts[].previous.complete_to_height"
}
func resultASCII(s string) bool {
	for _, b := range []byte(s) {
		if b > 127 {
			return false
		}
	}
	return true
}

// encoding/json replaces lone UTF-16 surrogates; the native UTF-8 Path grammar
// instead refuses malformed scalar escapes before decoding.
func resultJSONSurrogates(raw []byte) bool {
	quoted := false
	for i := 0; i < len(raw); i++ {
		if !quoted {
			if raw[i] == '"' {
				quoted = true
			}
			continue
		}
		if raw[i] == '"' {
			quoted = false
			continue
		}
		if raw[i] != '\\' {
			continue
		}
		i++
		if i >= len(raw) {
			return false
		}
		if raw[i] != 'u' {
			continue
		}
		if i+4 >= len(raw) {
			return false
		}
		v, e := strconv.ParseUint(string(raw[i+1:i+5]), 16, 16)
		if e != nil {
			return false
		}
		i += 4
		if v >= 0xdc00 && v <= 0xdfff {
			return false
		}
		if v >= 0xd800 && v <= 0xdbff {
			if i+6 >= len(raw) || raw[i+1] != '\\' || raw[i+2] != 'u' {
				return false
			}
			w, e := strconv.ParseUint(string(raw[i+3:i+7]), 16, 16)
			if e != nil || w < 0xdc00 || w > 0xdfff {
				return false
			}
			i += 6
		}
	}
	return true
}
