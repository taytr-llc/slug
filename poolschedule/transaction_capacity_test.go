// Copyright (c) 2026 The SLUG developers. ISC license.
package poolschedule

import (
	"reflect"
	"testing"
	"time"

	"github.com/taytr-llc/slug/poolsettle"
)

// Conditional scheduler-state coverage: the report is not funding authority
// and this fixture does not manufacture a beyond-V1 settlement ledger.
func TestScheduleTransactionCapacityRetainsRetry(t *testing.T) {
	now := time.Date(2030, 1, 1, 12, 0, 0, 0, time.UTC).UnixMilli()
	c := Config{Version: 1, SchedulerID: repeatID('1'), RuntimeID: repeatID('2'), SettlementBindingID: repeatID('3'), DailyUTCMinute: 720, RetryMS: 30000}
	initial := State{Version: 1, Config: c, Revision: 1, LastClockMS: now}
	running, err := admit(initial, "daily", now)
	if err != nil {
		t.Fatal(err)
	}
	before := cloneState(running)
	report := poolsettle.Report{Day: "2030-01-01", State: "deferred", Reason: "transaction-capacity"}
	finished, err := finish(running, report, nil, now+1)
	if err != nil || finished.DailyCursor != "" || finished.NextWakeMS != now+30001 || finished.Attempt.Result != "returned" || finished.Attempt.ReportState != "deferred" || finished.Attempt.ReportReason != "transaction-capacity" {
		t.Fatal("capacity deferral was rejected or occupied the daily slot", finished, err)
	}
	raw, err := encodeState(finished)
	if err != nil {
		t.Fatal("deferral cannot be retained", err)
	}
	restored, err := decodeState(raw)
	if err != nil || !reflect.DeepEqual(restored, finished) || !dailyDue(restored, restored.NextWakeMS) {
		t.Fatal("retained deferral lost its complete same-day retry", err)
	}
	// Retry retains the original scheduler/config and takes no completed day.
	retry, err := admit(restored, "recovery", restored.NextWakeMS)
	if err != nil || retry.Config != c || retry.DailyCursor != "" || retry.Attempt.Sequence != 2 || retry.Attempt.Kind != "recovery" {
		t.Fatal("retry changed original identity or bypassed recovery", retry, err)
	}
	report.Reason = "transaction-capacity-unrecognized"
	if _, err := finish(running, report, nil, now+1); err != ErrInvalid {
		t.Fatal("unknown reason widened the closed scheduler grammar", err)
	}
	if !reflect.DeepEqual(running, before) {
		t.Fatal("finish mutated the prior retained attempt")
	}
}
