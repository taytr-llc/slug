// Copyright (c) 2026 The SLUG developers. ISC license.
package poolschedule

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/taytr-llc/slug/poolsettle"
)

// The table was hand-derived before the production change. It is a conditional
// decision oracle, not proof that a caller-created State is an original owner.
func TestDailyScheduleCompletedRefreshIndependentDecisions(t *testing.T) {
	raw, err := os.ReadFile("testdata/completed-refresh/cases.json")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != "ca155db810675775e9407bda72d9ce5db748404cec081b60ef8f085efe7ec1c0" {
		t.Fatal("independent refresh decisions changed")
	}
	type fields struct {
		StateVersion  *uint32 `json:"state_version"`
		ConfigVersion *uint32 `json:"config_version"`
		Minute        *uint32 `json:"minute"`
		Previous      *string `json:"previous"`
		PreviousMS    *string `json:"previous_ms"`
		Cursor        *string `json:"cursor"`
	}
	var table struct {
		Version  int
		Defaults fields
		Cases    []struct {
			fields
			Name       string
			Now        string
			NowMS      *string `json:"now_ms"`
			RefreshDue bool    `json:"refresh_due"`
			PaymentDue bool    `json:"new_payment_due"`
		} `json:"predicate_cases"`
		Guards []json.RawMessage `json:"store_guard_cases"`
	}
	if err = json.Unmarshal(raw, &table); err != nil || table.Version != 1 || len(table.Cases) != 22 || len(table.Guards) != 8 {
		t.Fatal("independent inventory", err)
	}
	parse := func(stamp string, numeric *string) int64 {
		t.Helper()
		if numeric != nil {
			n, err := strconv.ParseInt(*numeric, 10, 64)
			if err != nil {
				t.Fatal(err)
			}
			return n
		}
		v, err := time.Parse(time.RFC3339Nano, stamp)
		if err != nil {
			t.Fatal(err)
		}
		return v.UnixMilli()
	}
	for _, c := range table.Cases {
		t.Run(c.Name, func(t *testing.T) {
			f := table.Defaults
			if c.StateVersion != nil {
				f.StateVersion = c.StateVersion
			}
			if c.ConfigVersion != nil {
				f.ConfigVersion = c.ConfigVersion
			}
			if c.Minute != nil {
				f.Minute = c.Minute
			}
			if c.Previous != nil {
				f.Previous = c.Previous
			}
			if c.PreviousMS != nil {
				f.PreviousMS = c.PreviousMS
			}
			if c.Cursor != nil {
				f.Cursor = c.Cursor
			}
			if f.StateVersion == nil || f.ConfigVersion == nil || f.Minute == nil || f.Previous == nil || f.Cursor == nil {
				t.Fatal("missing selected field")
			}
			s := State{Version: *f.StateVersion, Config: Config{Version: *f.ConfigVersion, DailyUTCMinute: *f.Minute}, LastClockMS: parse(*f.Previous, f.PreviousMS), DailyCursor: *f.Cursor}
			now := parse(c.Now, c.NowMS)
			if got := completedDailyRefreshDue(s, now); got != c.RefreshDue {
				t.Fatal("accounting refresh decision", got, c.RefreshDue)
			}
			if got := dailyDue(s, now); got != c.PaymentDue {
				t.Fatal("original payment decision changed", got, c.PaymentDue)
			}
		})
	}
}

func TestDailyScheduleCompletedRefreshExpiryRetry(t *testing.T) {
	h := newDailyScheduleHarness(t)
	s, _, _ := dailyScheduleFixture(t, h, 0)
	ctx := context.Background()
	before, err := h.store.View(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// An actual original Store samples its own clock. No clock is replaced to
	// force the midnight case; an already-expired admission has the same guard.
	report, err := s.refreshCompletedDaily(ctx, time.Now().UTC().Add(-24*time.Hour).Format("2006-01-02"))
	if err != ErrUnavailable || !retryable(err) || !reflect.DeepEqual(report, poolsettle.Report{}) {
		t.Fatal("expired admission was not a benign no-effect retry", report, err)
	}
	after, viewErr := h.store.View(ctx)
	if viewErr != nil || !reflect.DeepEqual(before, after) || h.backend.signCalls != 0 || h.backend.feeCalls != 0 || h.backend.broadcasts != 0 {
		t.Fatal("expiry changed financial state", after, viewErr)
	}
	// Persisted finish semantics stay unchanged: failed refresh retains the old
	// cursor. This conditional State is a pure transition control, not storage.
	now := time.Now().UTC().UnixMilli()
	st := cloneState(s.state)
	st.DailyCursor = date(now)
	admitted, admitErr := admit(st, "daily", now)
	if admitErr != nil {
		t.Fatal(admitErr)
	}
	finished, finishErr := finish(admitted, report, err, now)
	if finishErr != nil || finished.DailyCursor != st.DailyCursor || finished.Attempt.Result != "failed" || finished.Attempt.ErrorCode != "unavailable" {
		t.Fatal("expiry reset cursor or recorded success", finished, finishErr)
	}
	if retryable(errors.Join(poolsettle.ErrDailyRefreshExpired, poolsettle.ErrCorrupt)) {
		t.Fatal("mixed storage failure became benign expiry")
	}
}
