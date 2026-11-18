// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"encoding/json"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymstable"
	"io"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"
)

func eventDecode(raw []byte, limit, maxDepth int, out any) (map[string]bool, error) {
	if len(raw) == 0 || len(raw) > limit || !utf8.Valid(raw) {
		return nil, gymevent.ErrInvalid
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
			return gymevent.ErrInvalid
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
				return gymevent.ErrInvalid
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
					return gymevent.ErrInvalid
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
					return gymevent.ErrInvalid
				}
			}
			end, e := d.Token()
			if e != nil || end != json.Delim('}') {
				return gymevent.ErrInvalid
			}
			if path == "" {
				root = seen
			}
		case reflect.Array, reflect.Slice:
			if token != json.Delim('[') || depth+1 > maxDepth {
				return gymevent.ErrInvalid
			}
			count := 0
			bound := 0
			switch {
			case path == "job.ineligible_prospect_ids":
				bound = 10
			case strings.HasSuffix(path, ".members"):
				bound = 5
			case path == "snapshot.pin.participants", path == "record.input.participants", path == "record.captures":
				bound = 2
			case strings.HasSuffix(path, ".captured_participants"):
				bound = 2
			case path == "history.events":
				bound = 8
			case strings.HasSuffix(path, ".slots"), strings.HasSuffix(path, ".assigned_at_ms"):
				bound = 10
			}

			if t.Kind() == reflect.Array {
				bound = t.Len()
			}
			for d.More() {
				if count >= bound {
					return gymevent.ErrInvalid
				}
				if e := walk(t.Elem(), path+"[]", depth+1); e != nil {
					return e
				}
				count++
			}
			if t.Kind() == reflect.Array && count != t.Len() || (path == "snapshot.pin.participants" || path == "record.input.participants" || path == "record.captures") && count != 2 {
				return gymevent.ErrInvalid
			}
			end, e := d.Token()
			if e != nil || end != json.Delim(']') {
				return gymevent.ErrInvalid
			}
		case reflect.String:
			s, ok := token.(string)
			if !ok || len(s) > 4096 {
				return gymevent.ErrInvalid
			}
		case reflect.Bool:
			if _, ok := token.(bool); !ok {
				return gymevent.ErrInvalid
			}
		case reflect.Int, reflect.Int32, reflect.Int64:
			n, ok := token.(json.Number)
			if !ok {
				return gymevent.ErrInvalid
			}
			value, e := strconv.ParseInt(string(n), 10, t.Bits())
			if e != nil || strconv.FormatInt(value, 10) != string(n) || value > int64(gymstable.MaxSafeInteger) || value < 0 {
				return gymevent.ErrInvalid
			}
		case reflect.Uint, reflect.Uint32, reflect.Uint64:
			n, ok := token.(json.Number)
			if !ok {
				return gymevent.ErrInvalid
			}
			value, e := strconv.ParseUint(string(n), 10, t.Bits())
			if e != nil || strconv.FormatUint(value, 10) != string(n) || value > gymstable.MaxSafeInteger {
				return gymevent.ErrInvalid
			}
		default:
			return gymevent.ErrInvalid
		}
		cap := eventNestedCap(t.Name())
		if int(d.InputOffset())-start > cap {
			return gymevent.ErrInvalid
		}
		return nil
	}
	if err := walk(reflect.TypeOf(out).Elem(), "", 0); err != nil {
		return nil, err
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, gymevent.ErrInvalid
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return nil, gymevent.ErrInvalid
	}
	return root, nil
}

func MarshalEventRequest(r EventRequest) ([]byte, error) {
	if !eventRequestValid(r) {
		return nil, gymevent.ErrInvalid
	}
	raw, e := json.Marshal(r)
	if e != nil || len(raw) > MaxEventRequestSize {
		return nil, gymevent.ErrInvalid
	}
	return raw, nil
}
func ParseEventRequest(raw []byte) (EventRequest, error) {
	var r EventRequest
	if _, e := eventDecode(raw, MaxEventRequestSize, 12, &r); e != nil || !eventRequestValid(r) {
		return EventRequest{}, gymevent.ErrInvalid
	}
	return r, nil
}
func ParseEventResponse(raw []byte) (EventResponse, error) {
	var r EventResponse
	if _, e := eventDecode(raw, MaxEventResponseSize, 12, &r); e != nil || !eventResponseValid(&r) {
		return EventResponse{}, gymevent.ErrInvalid
	}
	return r, nil
}
func MarshalEventResponse(r EventResponse) ([]byte, error) {
	// Typed collection bounds precede JSON allocation; the lexical pass then
	// enforces exact paths and closed transport validation on an owned copy.
	budget := MaxEventResponseSize
	if !eventResponseBudget(reflect.ValueOf(r), &budget, 0) {
		return nil, gymevent.ErrInvalid
	}
	raw, e := json.Marshal(r)
	if e != nil {
		return nil, gymevent.ErrInvalid
	}
	owned, e := ParseEventResponse(raw)
	if e != nil {
		return nil, e
	}
	return json.Marshal(owned)
}
func eventResponseBudget(v reflect.Value, budget *int, depth int) bool {
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
		return eventResponseBudget(v.Elem(), budget, depth)
	case reflect.Struct:
		if depth >= 12 || !spend(2) {
			return false
		}
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			f := t.Field(i)
			if f.PkgPath != "" || f.Tag.Get("json") == "-" {
				continue
			}
			if !spend(len(f.Tag.Get("json"))+4) || !eventResponseBudget(v.Field(i), budget, depth+1) {
				return false
			}
		}
	case reflect.Array, reflect.Slice:
		if depth >= 12 || v.Len() > 10 || !spend(2) {
			return false
		}
		for i := 0; i < v.Len(); i++ {
			if !spend(1) || !eventResponseBudget(v.Index(i), budget, depth+1) {
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

func eventNestedCap(name string) int {
	switch name {
	case "Pin":
		return 16384
	case "Event", "EventStatus":
		return 2048
	case "CutoffRoster", "Member":
		return 4096
	case "CutoffEntry":
		return 36864
	case "ParticipantSnapshot":
		return 57344
	case "CaptureEvidence":
		return 2048
	case "ParticipantCapture":
		return 61440
	case "SnapshotInput":
		return 122880
	case "SnapshotReceipt":
		return 126976
	}
	return MaxEventResponseSize
}
