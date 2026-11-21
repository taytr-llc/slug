// Copyright (c) 2026 The SLUG developers. ISC license.
package poolschedule

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/wire/v2"
)

type dailyScheduleRow struct {
	Name string `json:"name"`
	Kind string `json:"kind"`
	Raw  string `json:"raw_hex"`
}
type dailyScheduleReference struct {
	Version  int `json:"version"`
	Bindings []struct {
		Fixture string            `json:"fixture"`
		Config  poolsettle.Config `json:"config"`
		Source  poolshare.Config  `json:"source"`
		ID      string            `json:"id"`
		Other   string            `json:"other_domain_id"`
	} `json:"bindings"`
	BindingRefusals []struct {
		Name   string            `json:"name"`
		Config poolsettle.Config `json:"config"`
		Source poolshare.Config  `json:"source"`
	} `json:"binding_refusals"`
	Positives  []dailyScheduleRow `json:"positives"`
	Refusals   []dailyScheduleRow `json:"refusals"`
	Selections []struct {
		Name     string `json:"name"`
		Expected Config `json:"expected"`
		Marker   string `json:"marker_hex"`
	} `json:"selection_refusals"`
	Finish []struct {
		Name     string            `json:"name"`
		Args     []json.RawMessage `json:"args"`
		Expected *string           `json:"expected_cursor"`
		Error    string            `json:"expected_error"`
	} `json:"finish_cases"`
	Due []struct {
		Name     string `json:"name"`
		Now      int64  `json:"now_ms"`
		Cursor   string `json:"cursor"`
		Minute   uint32 `json:"minute"`
		Expected bool   `json:"expected"`
	} `json:"due_cases"`
}

func readDailyScheduleReference(t *testing.T) dailyScheduleReference {
	t.Helper()
	raw, err := os.ReadFile("testdata/daily-reference/v2.json")
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "e2d493d4dd4db21306e5d78d4b7bfcad7b96ccd20255d1d7ed4eba48d2c1a108" {
		t.Fatal("independent daily scheduler reference changed")
	}
	var f dailyScheduleReference
	if json.Unmarshal(raw, &f) != nil || f.Version != 2 || len(f.Bindings) != 3 || len(f.BindingRefusals) != 3 || len(f.Positives) != 43 || len(f.Refusals) != 41 || len(f.Selections) != 5 || len(f.Finish) != 13 || len(f.Due) != 4 {
		t.Fatal("incomplete independent reference")
	}
	return f
}

func TestDailyScheduleIndependentBindings(t *testing.T) {
	f := readDailyScheduleReference(t)
	for _, tc := range f.Bindings {
		id, err := dailyBinding(tc.Config, tc.Source)
		if wire.TestPoW {
			if id != "" || !errors.Is(err, ErrProfile) {
				t.Fatal("cheap binding", err)
			}
			continue
		}
		if err != nil || id != tc.ID || id == tc.Other {
			t.Fatal("independent full configuration/domain binding", tc.Fixture, id, err)
		}
	}
	for _, tc := range f.BindingRefusals {
		if id, err := dailyBinding(tc.Config, tc.Source); id != "" || err == nil {
			t.Fatal("legacy component adopted", tc.Name)
		}
	}
}

func TestDailyScheduleIndependentCodecAndMode(t *testing.T) {
	f := readDailyScheduleReference(t)
	for _, tc := range f.Positives {
		t.Run(tc.Name, func(t *testing.T) {
			raw, err := hex.DecodeString(tc.Raw)
			if err != nil {
				t.Fatal(err)
			}
			var encoded []byte
			if tc.Kind == "marker" {
				c, e := decodeMarkerSelected(raw, Version2)
				if e != nil {
					t.Fatal(e)
				}
				encoded, err = encodeMarkerSelected(c, Version2)
				if _, e = decodeMarker(raw); e == nil {
					t.Fatal("V1 marker adopted V2")
				}
			} else {
				s, e := decodeStateSelected(raw, Version2)
				if e != nil {
					t.Fatal(e)
				}
				encoded, err = encodeStateSelected(s, Version2)
				if _, e = decodeState(raw); e == nil {
					t.Fatal("V1 state adopted V2")
				}
			}
			if err != nil || !bytes.Equal(raw, encoded) {
				t.Fatal("canonical bytes changed", err)
			}
		})
	}
	for _, tc := range f.Refusals {
		t.Run(tc.Name, func(t *testing.T) {
			raw, err := hex.DecodeString(tc.Raw)
			if err != nil {
				t.Fatal(err)
			}
			if tc.Kind == "marker" {
				_, err = decodeMarkerSelected(raw, Version2)
			} else {
				_, err = decodeStateSelected(raw, Version2)
			}
			if err == nil {
				t.Fatal("malformed/mismatched mode admitted")
			}
		})
	}
	for _, tc := range f.Selections {
		raw, err := hex.DecodeString(tc.Marker)
		if err != nil {
			t.Fatal(err)
		}
		c, err := decodeMarkerSelected(raw, Version2)
		if err != nil || c == tc.Expected {
			t.Fatal("lost independent exact-selection mismatch", tc.Name, err)
		}
	}
	// Existing corpus is still exercised by TestScheduleIndependentCodec; the
	// explicit daily decoder also refuses its valid V1 identities without repair.
	raw, err := os.ReadFile("testdata/reference/v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var old struct {
		Positives []dailyScheduleRow `json:"positives"`
	}
	if json.Unmarshal(raw, &old) != nil || len(old.Positives) != 37 {
		t.Fatal("legacy corpus")
	}
	for _, tc := range old.Positives {
		b, e := hex.DecodeString(tc.Raw)
		if e != nil {
			t.Fatal(e)
		}
		if tc.Kind == "marker" {
			_, e = decodeMarkerSelected(b, Version2)
		} else {
			_, e = decodeStateSelected(b, Version2)
		}
		if e == nil {
			t.Fatal("daily decoder adopted legacy identity", tc.Name)
		}
	}
}

func TestDailyScheduleIndependentCursorAndD84(t *testing.T) {
	f := readDailyScheduleReference(t)
	for _, tc := range f.Finish {
		t.Run(tc.Name, func(t *testing.T) {
			if len(tc.Args) != 8 {
				t.Fatal("finish arguments")
			}
			var kind, day, state, reason, cursor, code string
			var start, end int64
			values := []any{&kind, &start, &end, &day, &state, &reason, &cursor, &code}
			for i, v := range values {
				if json.Unmarshal(tc.Args[i], v) != nil {
					t.Fatal("finish field")
				}
			}
			config := Config{Version: Version2, SchedulerID: repeatID('1'), RuntimeID: repeatID('2'), SettlementBindingID: f.Bindings[0].ID, DailyUTCMinute: 720, RetryMS: 30000}
			initial := State{Version: Version2, Config: config, Revision: 1, LastClockMS: start, DailyCursor: cursor}
			running, err := admit(initial, kind, start)
			if err != nil {
				t.Fatal(err)
			}
			before := cloneState(running)
			var callErr error
			switch code {
			case "":
			case "busy":
				callErr = ErrBusy
			case "deadline":
				callErr = context.DeadlineExceeded
			default:
				t.Fatal("unknown conditional error")
			}
			finished, err := finish(running, poolsettle.Report{Day: day, State: state, Reason: reason}, callErr, end)
			if !reflect.DeepEqual(before, running) {
				t.Fatal("completion changed original attempt")
			}
			if tc.Expected == nil {
				if err == nil || (tc.Error == "clock" && !errors.Is(err, ErrClock)) || (tc.Error == "invalid" && !errors.Is(err, ErrInvalid)) {
					t.Fatal("expected refusal", tc.Error, err)
				}
				return
			}
			if err != nil || finished.DailyCursor != *tc.Expected {
				t.Fatal("cursor changed", finished.DailyCursor, err)
			}
			if code != "" && (finished.Attempt.Result != "failed" || finished.Attempt.ErrorCode != code) {
				t.Fatal("failed attempt grammar")
			}
			raw, err := encodeStateSelected(finished, Version2)
			if err != nil {
				t.Fatal(err)
			}
			restored, err := decodeStateSelected(raw, Version2)
			if err != nil || !reflect.DeepEqual(restored, finished) {
				t.Fatal("retained cursor", err)
			}
			if reason == "transaction-capacity" {
				if !dailyDue(restored, restored.NextWakeMS) {
					t.Fatal("whole deferral occupied daily slot")
				}
				next, e := admit(restored, "recovery", restored.NextWakeMS)
				if e != nil || next.DailyCursor != cursor || next.Config != config || next.Attempt.Kind != "recovery" {
					t.Fatal("deferral retry lost recovery/original identity", e)
				}
			}
		})
	}
	for _, tc := range f.Due {
		s := State{Version: Version2, Config: Config{Version: Version2, DailyUTCMinute: tc.Minute}, LastClockMS: tc.Now, DailyCursor: tc.Cursor}
		if dailyDue(s, tc.Now) != tc.Expected {
			t.Fatal("independent UTC boundary", tc.Name)
		}
	}
}
