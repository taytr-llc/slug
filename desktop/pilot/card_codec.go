// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"encoding/json"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymstable"
	"io"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"
)

func cardDecode(raw []byte, limit, maxDepth int, out any) (map[string]bool, error) {
	if len(raw) == 0 || len(raw) > limit || !utf8.Valid(raw) {
		return nil, gymcard.ErrInvalid
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
			return gymcard.ErrInvalid
		}
		if t.Kind() == reflect.Pointer {
			if token == nil {
				return nil
			}
			t = t.Elem()
		}
		checkpoint := t == reflect.TypeOf(chaincfg.SignedCheckpoint{})
		if checkpoint {
			t = reflect.TypeOf(cardCheckpointWire{})
		}
		switch t.Kind() {
		case reflect.Struct:
			if token != json.Delim('{') || depth+1 > maxDepth {
				return gymcard.ErrInvalid
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
					return gymcard.ErrInvalid
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
					return gymcard.ErrInvalid
				}
			}
			end, e := d.Token()
			if e != nil || end != json.Delim('}') {
				return gymcard.ErrInvalid
			}
			if path == "" {
				root = seen
			}
		case reflect.Array, reflect.Slice:
			if token != json.Delim('[') || depth+1 > maxDepth {
				return gymcard.ErrInvalid
			}
			count := 0
			bound, exact := cardArrayBound(path)
			if bound < 0 {
				return gymcard.ErrInvalid
			}

			for d.More() {
				if count >= bound {
					return gymcard.ErrInvalid
				}
				if e := walk(t.Elem(), path+"[]", depth+1); e != nil {
					return e
				}
				count++
			}
			if exact && count != bound {
				return gymcard.ErrInvalid
			}
			end, e := d.Token()
			if e != nil || end != json.Delim(']') {
				return gymcard.ErrInvalid
			}
		case reflect.String:
			s, ok := token.(string)
			for _, b := range []byte(s) {
				if b > 127 {
					return gymcard.ErrInvalid
				}
			}
			if !ok || len(s) > 4096 || (strings.Contains(path, "checkpoint.") && (strings.HasSuffix(path, ".genesis") || strings.HasSuffix(path, ".hash")) && !canonicalManifestID(s)) || (strings.Contains(path, "checkpoint.") && strings.HasSuffix(path, ".signature") && !seedHex(s, 64)) {
				return gymcard.ErrInvalid
			}
		case reflect.Bool:
			if _, ok := token.(bool); !ok {
				return gymcard.ErrInvalid
			}
		case reflect.Int, reflect.Int32, reflect.Int64:
			n, ok := token.(json.Number)
			if !ok {
				return gymcard.ErrInvalid
			}
			value, e := strconv.ParseInt(string(n), 10, t.Bits())
			if e != nil || strconv.FormatInt(value, 10) != string(n) || value > int64(gymstable.MaxSafeInteger) || value < 0 && !(value == -1 && cardSignedHeight(path)) {
				return gymcard.ErrInvalid
			}
		case reflect.Uint, reflect.Uint32, reflect.Uint64:
			n, ok := token.(json.Number)
			if !ok {
				return gymcard.ErrInvalid
			}
			value, e := strconv.ParseUint(string(n), 10, t.Bits())
			if e != nil || strconv.FormatUint(value, 10) != string(n) || value > gymstable.MaxSafeInteger {
				return gymcard.ErrInvalid
			}
		default:
			return gymcard.ErrInvalid
		}
		cap := cardNestedCap(t.Name())
		if checkpoint {
			cap = 4096
		}
		if int(d.InputOffset())-start > cap {
			return gymcard.ErrInvalid
		}
		return nil
	}
	rootType := reflect.TypeOf(out).Elem()
	rootPath := ""
	if rootType == reflect.TypeOf(CardSnapshot{}) {
		rootPath = "snapshot"
	}
	if rootType == reflect.TypeOf(CardJob{}) {
		rootPath = "job"
	}
	if err := walk(rootType, rootPath, 0); err != nil {
		return nil, err
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, gymcard.ErrInvalid
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return nil, gymcard.ErrInvalid
	}
	return root, nil
}

func MarshalCardRequest(r CardRequest) ([]byte, error) {
	if !cardRequestValid(r) {
		return nil, gymcard.ErrInvalid
	}
	raw, e := json.Marshal(r)
	if e != nil || len(raw)+1 > MaxCardRequestSize {
		return nil, gymcard.ErrInvalid
	}
	return raw, nil
}
func ParseCardRequest(raw []byte) (CardRequest, error) {
	var r CardRequest
	if _, e := cardDecode(raw, MaxCardRequestSize-1, 16, &r); e != nil || !cardRequestValid(r) {
		return CardRequest{}, gymcard.ErrInvalid
	}
	return r, nil
}
func ParseCardResponse(raw []byte) (CardResponse, error) {
	var r CardResponse
	if _, e := cardDecode(raw, MaxCardResponseSize-1, 16, &r); e != nil || !cardResponseValid(&r) {
		return CardResponse{}, gymcard.ErrInvalid
	}
	return r, nil
}
func MarshalCardResponse(r CardResponse) ([]byte, error) {
	// Typed collection bounds precede JSON allocation; the lexical pass then
	// enforces exact paths and closed transport validation on an owned copy.
	budget := MaxCardResponseSize - 1
	if !cardResponseBudget(reflect.ValueOf(r), &budget, 0) {
		return nil, gymcard.ErrInvalid
	}
	raw, e := json.Marshal(r)
	if e != nil {
		return nil, gymcard.ErrInvalid
	}
	owned, e := ParseCardResponse(raw)
	if e != nil {
		return nil, e
	}
	return json.Marshal(owned)
}
func cardResponseBudget(v reflect.Value, budget *int, depth int) bool {
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
		return cardResponseBudget(v.Elem(), budget, depth)
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
			if !spend(len(f.Tag.Get("json"))+4) || !cardResponseBudget(v.Field(i), budget, depth+1) {
				return false
			}
		}
	case reflect.Array, reflect.Slice:
		if depth >= 16 || v.Len() > 20 || !spend(2) {
			return false
		}
		for i := 0; i < v.Len(); i++ {
			if !spend(1) || !cardResponseBudget(v.Index(i), budget, depth+1) {
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

func cardNestedCap(name string) int {
	switch name {
	case "Record":
		return 196608
	case "BuildResult", "ReadResult":
		return 229376
	case "CardJob":
		return MaxCardJobSize
	case "Pin":
		return 20480
	case "CardSource":
		return 163840
	case "CardState":
		return 12288
	case "CardView":
		return 122880
	case "Plan":
		return 16384
	case "Status":
		return 16384
	case "HistoryPage":
		return 139264
	case "Declaration":
		return 4096
	case "SignedDeclaration":
		return 5120
	case "SignedCheckpoint":
		return 4096
	}
	if n := eventNestedCap(name); n < MaxEventResponseSize {
		return n
	}
	return MaxCardResponseSize
}
func cardRecordPath(path string) (string, bool) {
	for _, prefix := range []string{"record.record.", "job.result.record."} {
		if strings.HasPrefix(path, prefix) {
			return strings.TrimPrefix(path, prefix), true
		}
	}
	return "", false
}
func cardArrayBound(path string) (int, bool) {
	if path == "job.ineligible_prospect_ids" {
		return 10, false
	}
	if path == "snapshot.pin.source_pin.source_pin.participants" {
		return 2, true
	}
	if path == "history.cards" {
		return 8, false
	}
	r, ok := cardRecordPath(path)
	if !ok {
		return -1, false
	}
	switch r {
	case "source.source_receipt.input.participants", "source.source_receipt.captures", "plan.gyms":
		return 2, true
	case "source.source_receipt.input.participants[].roster.slots", "source.source_receipt.input.participants[].roster.assigned_at_ms":
		return 10, true
	case "source.source_receipt.input.participants[].members", "source.source_receipt.input.participants[].entry.latest_receipt.members", "plan.bouts":
		return 5, false
	case "source.anchor.merkle_branch":
		return 20, false
	case "plan.entrants":
		return 10, false
	}
	return -1, false
}
func cardSignedHeight(path string) bool {
	for _, prefix := range []string{"status.", "record.status.", "job.result.status.", "history.cards[]."} {
		if path == prefix+"source.status.scan_height" || path == prefix+"source.status.scan_tip_height" {
			return true
		}
	}
	return false
}

type cardCheckpointWire struct {
	Version   uint32 `json:"version"`
	Network   uint32 `json:"network"`
	Genesis   string `json:"genesis"`
	Height    int32  `json:"height"`
	Hash      string `json:"hash"`
	Signature string `json:"signature"`
}
