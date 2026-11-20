// Copyright (c) 2026 The SLUG developers. ISC license.
package poolschedule

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/taytr-llc/slug/poolsettle"
)

func TestScheduleIndependentCodec(t *testing.T) {
	raw, e := os.ReadFile("testdata/reference/v1.json")
	if e != nil {
		t.Fatal(e)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "2d9d589d38453759f8b334c8ca154ce6c35168ff83537bec87eca90363817aeb" {
		t.Fatal("independent corpus changed")
	}
	type row struct {
		Name string `json:"name"`
		Kind string `json:"kind"`
		Raw  string `json:"raw_hex"`
	}
	var corpus struct {
		Positives []row `json:"positives"`
		Refusals  []row `json:"refusals"`
		Finish    []struct {
			Name     string            `json:"name"`
			Args     []json.RawMessage `json:"args"`
			Expected *string           `json:"expected_cursor"`
		} `json:"finish_cases"`
	}
	if e = json.Unmarshal(raw, &corpus); e != nil {
		t.Fatal(e)
	}
	if len(corpus.Positives) != 37 || len(corpus.Refusals) != 35 || len(corpus.Finish) != 6 {
		t.Fatal("incomplete corpus")
	}
	for _, group := range []struct {
		Rows  []row
		Valid bool
	}{{corpus.Positives, true}, {corpus.Refusals, false}} {
		for _, r := range group.Rows {
			t.Run(r.Name, func(t *testing.T) {
				b, e := hex.DecodeString(r.Raw)
				if e != nil {
					t.Fatal(e)
				}
				if r.Kind == "marker" {
					_, e = decodeMarker(b)
				} else {
					_, e = decodeState(b)
				}
				if (e == nil) != group.Valid {
					t.Fatalf("valid=%v error=%v", group.Valid, e)
				}
			})
		}
	}
	for _, r := range corpus.Finish {
		t.Run(r.Name, func(t *testing.T) {
			if len(r.Args) != 7 {
				t.Fatal("finish shape")
			}
			var kind, day, state, reason, cursor string
			var start, end int64
			targets := []any{&kind, &start, &end, &day, &state, &reason, &cursor}
			for i := range targets {
				if e = json.Unmarshal(r.Args[i], targets[i]); e != nil {
					t.Fatal(e)
				}
			}
			c := Config{Version: 1, SchedulerID: repeatID('1'), RuntimeID: repeatID('2'), SettlementBindingID: repeatID('3'), DailyUTCMinute: 720, RetryMS: 30000}
			s := State{Version: 1, Config: c, Revision: 2, LastClockMS: start, DailyCursor: cursor, Attempt: &Attempt{Sequence: 1, Kind: kind, AdmittedDay: date(start), StartedAtMS: start, Result: "running"}}
			out, e := finish(s, poolsettle.Report{Day: day, State: state, Reason: reason}, nil, end)
			if r.Expected == nil {
				if e == nil {
					t.Fatal("future day accepted")
				}
				return
			}
			if e != nil || out.DailyCursor != *r.Expected {
				t.Fatalf("cursor=%q expected=%q err=%v", out.DailyCursor, *r.Expected, e)
			}
		})
	}
}
func repeatID(b byte) string {
	v := make([]byte, 64)
	for i := range v {
		v[i] = b
	}
	return string(v)
}
func TestScheduleTimingBoundaries(t *testing.T) {
	c := Config{Version: 1, SchedulerID: repeatID('1'), RuntimeID: repeatID('2'), SettlementBindingID: repeatID('3'), DailyUTCMinute: 720, RetryMS: 30000}
	noon := int64(1893499200000)
	s := State{Version: 1, Config: c, Revision: 1, LastClockMS: noon - 1}
	if dailyDue(s, noon-1) || !dailyDue(s, noon) {
		t.Fatal("daily minute boundary")
	}
	if wake, e := nextWake(c, noon-1); e != nil || wake != noon {
		t.Fatal("due must shorten wake")
	}
	if wake, e := nextWake(c, noon); e != nil || wake != noon+30000 {
		t.Fatal("past due must not spin")
	}
	if _, e := admit(s, "recovery", noon-2); e != ErrClock {
		t.Fatal("backward clock")
	}
	s.Revision = ^uint32(0)
	if _, e := admit(s, "recovery", noon); e != ErrCapacity {
		t.Fatal("revision overflow")
	}
	s.Revision = ^uint32(0) - 1
	if _, e := admit(s, "recovery", noon); e != ErrCapacity {
		t.Fatal("must reserve completion revision before dispatch")
	}
	c.DailyUTCMinute = 0
	tomorrow := int64(1893542400000)
	if wake, e := nextWake(c, tomorrow-1); e != nil || wake != tomorrow {
		t.Fatal("next-day due must shorten retry", wake, e)
	}
	if _, e := nextWake(c, MaxTimeMS); e != ErrClock {
		t.Fatal("timestamp overflow")
	}
}
