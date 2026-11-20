// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
)

func ownedCalendarPin(t *testing.T) gymevent.OwnedPin {
	t.Helper()
	// Exact independently produced public pin; no runtime output supplies the
	// expected calendar or participant authority.
	raw, e := os.ReadFile("../../gymstable/testdata/owned-reference/owned-consumers-v1.json")
	if e != nil {
		t.Fatal(e)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "44f3d12b96a94f6d2d071270b6c90296cae1db50249ff4725ec5e28adc9bfa15" {
		t.Fatal("independent owned consumer corpus changed")
	}
	var v struct {
		Contexts map[string]json.RawMessage
	}
	if e = json.Unmarshal(raw, &v); e != nil {
		t.Fatal(e)
	}
	var event struct{ Pin gymevent.OwnedPin }
	if e = json.Unmarshal(v.Contexts["event"], &event); e != nil {
		t.Fatal(e)
	}
	if _, e = gymevent.ValidateOwnedPin(event.Pin); e != nil {
		t.Fatal(e)
	}
	return event.Pin
}

func TestOwnedGymCalendarDates(t *testing.T) {
	pin := ownedCalendarPin(t)
	for _, tc := range []struct {
		date, previous string
		hours          int64
	}{
		{"2026-01-03", "", 0},
		{"2027-03-20", "2027-03-13", 167},
		{"2027-11-13", "2027-11-06", 169},
		{"2027-01-02", "2026-12-26", 168},
	} {
		t.Run(tc.date, func(t *testing.T) {
			event, e := gymentry.OwnedEventForDate(pin.Participants[0], tc.date)
			if e != nil {
				t.Fatal(e)
			}
			got, e := ownedCalendarDates(pin, event)
			want := []string{tc.date}
			if tc.previous != "" {
				want = append(want, tc.previous)
			}
			if e != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("dates %v, %v; want %v", got, e, want)
			}
			if tc.previous != "" {
				previous, e := gymentry.OwnedEventForDate(pin.Participants[0], tc.previous)
				if e != nil || event.CutoffMS-previous.CutoffMS != tc.hours*int64(time.Hour/time.Millisecond) {
					t.Fatalf("calendar interval is not %d hours: %v", tc.hours, e)
				}
			}
		})
	}
}

func TestOwnedGymCalendarRejectsForeignEvent(t *testing.T) {
	pin := ownedCalendarPin(t)
	event, e := gymentry.OwnedEventForDate(pin.Participants[0], "2027-03-13")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = ownedCalendarDates(gymevent.OwnedPin{}, event); e == nil {
		t.Fatal("empty pin accepted")
	}
	for _, mutate := range []func(*gymentry.Event){
		func(v *gymentry.Event) { v.CutoffMS++ },
		func(v *gymentry.Event) { v.EventID = v.NamespaceID },
		func(v *gymentry.Event) { v.Saturday = "2027-03-14" },
	} {
		foreign := event
		mutate(&foreign)
		if _, e = ownedCalendarDates(pin, foreign); e == nil {
			t.Fatal("foreign or non-calendar event accepted")
		}
	}
}
