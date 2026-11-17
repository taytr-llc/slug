// Copyright (c) 2026 The SLUG developers. ISC license.
package gymentry

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/wire/v2"
	"os"
	"path/filepath"
	"testing"
)

type entryReference struct {
	Calendar struct {
		CalendarID string `json:"calendar_id"`
		RulesID    string `json:"rules_id"`
		Saturdays  []struct {
			Saturday string
			CutoffMS int64   `json:"cutoff_ms"`
			Before   *string `json:"upcoming_cutoff_minus_1"`
			Exact    *string `json:"upcoming_cutoff_exact"`
			After    *string `json:"upcoming_cutoff_plus_1"`
		} `json:"all_supported_saturdays"`
		Invalid []struct{ Input string } `json:"invalid_dates"`
		Bounds  []struct {
			NowMS int64 `json:"now_ms"`
			Date  string
			Error string
		} `json:"upcoming_bounds"`
	}
	Pins []struct {
		Fixture   string
		Pin       Pin
		Canonical string
		StoreID   string `json:"store_id"`
	}
	Events []struct {
		Fixture   string
		Event     Event
		Canonical string
		EntryID   string `json:"entry_id"`
	}
	DTOs []struct {
		Fixture, Kind, Canonical, Digest string
		Value                            json.RawMessage
		CanonicalReceipt                 string `json:"canonical_receipt"`
		Rows                             []struct {
			KeyHex   string `json:"key_hex"`
			ValueHex string `json:"value_hex"`
		}
	}
	CodecCases []struct {
		Name, Kind, RawHex, Canonical string
		Pin                           Pin
		Accepted                      bool
	} `json:"-"`
}

func readEntryReference(t *testing.T) entryReference {
	t.Helper()
	raw, e := os.ReadFile("testdata/reference/entry_v1.json")
	if e != nil {
		t.Fatal(e)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "d8f226964429e8068bbe8300c73b6061f9a6dfd4e737a06ddde40c5eaa992bee" {
		t.Fatal("independent corpus changed")
	}
	var f entryReference
	if e = json.Unmarshal(raw, &f); e != nil {
		t.Fatal(e)
	}
	var cases struct {
		Cases []struct {
			Name, Kind string
			RawHex     string `json:"raw_hex"`
			Canonical  string
			Pin        Pin
			Accepted   bool
		} `json:"codec_cases"`
	}
	if e = json.Unmarshal(raw, &cases); e != nil {
		t.Fatal(e)
	}
	for _, v := range cases.Cases {
		f.CodecCases = append(f.CodecCases, struct {
			Name, Kind, RawHex, Canonical string
			Pin                           Pin
			Accepted                      bool
		}{v.Name, v.Kind, v.RawHex, v.Canonical, v.Pin, v.Accepted})
	}
	return f
}
func referenceCodec(kind string, raw []byte, p Pin) ([]byte, error, bool) {
	switch kind {
	case "Pin":
		v, e := ParsePin(raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalPin(v)
		return b, e, true
	case "Request":
		v, e := ParseRequest(raw)
		if e == nil {
			ev, ee := EventForDate(p, v.Saturday)
			if ee != nil || ev.EventID != v.EventID {
				e = ErrInvalid
			}
		}
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalRequest(v)
		return b, e, true
	case "Event":
		v, e := ParseEvent(raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalEvent(v)
		return b, e, true
	case "Intent":
		v, e := ParseIntent(raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalIntent(v)
		return b, e, true
	case "Receipt":
		v, e := ParseReceipt(raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalReceipt(v)
		return b, e, true
	case "Operation":
		v, e := ParseOperation(raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalOperation(v)
		return b, e, true
	case "MutationResult":
		v, e := ParseMutationResult(raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalMutationResult(v)
		return b, e, true
	case "OperationResult":
		v, e := ParseOperationResult(raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalOperationResult(v)
		return b, e, true
	case "EventStatus":
		v, e := ParseEventStatus(raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalEventStatus(v)
		return b, e, true
	case "CheckResult":
		v, e := ParseCheckResult(raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalCheckResult(v)
		return b, e, true
	case "HistoryPage":
		v, e := ParseHistoryPage(raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalHistoryPage(v)
		return b, e, true
	case "Marker":
		var v marker
		if e := decode(bytes.TrimSuffix(raw, []byte{'\n'}), MaxMarkerSize, &v, false); e != nil {
			return nil, e, true
		}
		id, e := ID(v.Pin)
		if e != nil || v.Application != "slug-candidate-entries" || v.Version != Version1 || v.StoreID != id {
			return nil, ErrInvalid, true
		}
		b, e := json.Marshal(v)
		return b, e, true
	case "Metadata":
		var v metadata
		if e := decode(raw, MaxMetadataSize, &v, false); e != nil {
			return nil, e, true
		}
		id, e := ID(v.Pin)
		if e != nil || v.Version != Version1 || v.StoreID != id {
			return nil, ErrInvalid, true
		}
		b, e := json.Marshal(v)
		return b, e, true
	case "EventRow":
		var v eventRow
		if e := decode(raw, MaxEventSize, &v, false); e != nil {
			return nil, e, true
		}
		id, _ := ID(p)
		m := metadata{Pin: p, StoreID: id, OperationCount: MaxOperations, ClockMS: MaxTimestampMS}
		if e := validateEventRow(v, m); e != nil {
			return nil, e, true
		}
		b, e := json.Marshal(v)
		return b, e, true
	}
	return nil, nil, false
}
func TestEntryIndependentCalendarAndIdentities(t *testing.T) {
	f := readEntryReference(t)
	if CalendarID() != f.Calendar.CalendarID || RulesID() != f.Calendar.RulesID {
		t.Fatal("calendar/rules digest")
	}
	if len(f.Calendar.Saturdays) != 3861 {
		t.Fatal("calendar coverage")
	}
	for _, c := range f.Calendar.Saturdays {
		n, e := cutoff(c.Saturday)
		if e != nil || n != c.CutoffMS {
			t.Fatal(c.Saturday, n, c.CutoffMS, e)
		}
		for i, want := range []*string{c.Before, c.Exact, c.After} {
			date, e := upcomingDate(n + int64(i) - 1)
			if want == nil {
				if e == nil {
					t.Fatal("unsupported upcoming", c.Saturday)
				}
			} else if e != nil || date != *want {
				t.Fatal("upcoming", c.Saturday, i, date, *want, e)
			}
		}
	}
	for _, c := range f.Calendar.Invalid {
		if _, e := cutoff(c.Input); e == nil {
			t.Fatal("invalid date", c.Input)
		}
	}
	for _, c := range f.Calendar.Bounds {
		date, e := upcomingDate(c.NowMS)
		if (e == nil) != (c.Error == "") || e == nil && date != c.Date {
			t.Fatal("upcoming bound", c.NowMS, date, e)
		}
	}
	if wire.TestPoW {
		return
	}
	pins := map[string]Pin{}
	for _, c := range f.Pins {
		pins[c.Fixture] = c.Pin
		b, e := MarshalPin(c.Pin)
		if e != nil || string(b) != c.Canonical {
			t.Fatal("pin bytes", c.Fixture, e)
		}
		id, e := ID(c.Pin)
		if e != nil || id != c.StoreID {
			t.Fatal("store ID", c.Fixture, id, c.StoreID, e)
		}
	}
	for _, c := range f.Events {
		ev, e := EventForDate(pins[c.Fixture], c.Event.Saturday)
		if e != nil || ev != c.Event {
			t.Fatal("event", c.Event.Saturday, e)
		}
		b, e := MarshalEvent(ev)
		if e != nil || string(b) != c.Canonical {
			t.Fatal("event bytes", e)
		}
		store, _ := ID(pins[c.Fixture])
		id, e := EntryID(store, ev.EventID, pins[c.Fixture].StablePin.Identity.GymID)
		if e != nil || id != c.EntryID {
			t.Fatal("entry ID", e)
		}
	}
}
func TestEntryIndependentCodecsAndDigests(t *testing.T) {
	if wire.TestPoW {
		t.Skip("compiled normal-fixture identities")
	}
	f := readEntryReference(t)
	pins := map[string]Pin{}
	for _, p := range f.Pins {
		pins[p.Fixture] = p.Pin
	}
	count := 0
	for i, c := range f.CodecCases {
		raw, e := hex.DecodeString(c.RawHex)
		if e != nil {
			t.Fatal(e)
		}
		b, e, handled := referenceCodec(c.Kind, raw, c.Pin)
		if !handled {
			continue
		}
		count++
		if (e == nil) != c.Accepted {
			t.Errorf("%d %s: accepted=%v want=%v error=%v", i, c.Name, e == nil, c.Accepted, e)
			continue
		}
		if e == nil && string(b) != c.Canonical {
			t.Errorf("%d %s: canonical bytes differ", i, c.Name)
		}
	}
	if count < 400 {
		t.Fatal("missing codec coverage", count)
	}
	t.Log("core closed-codec cases", count)
	for i, c := range f.DTOs {
		t.Run(fmt.Sprintf("%d/%s", i, c.Kind), func(t *testing.T) {
			if c.Kind == "ReceiptDigest" {
				r, e := ParseReceipt([]byte(c.CanonicalReceipt))
				if e != nil {
					t.Fatal(e)
				}
				digest, e := ReceiptDigest(r)
				if e != nil || digest != c.Digest {
					t.Fatal("receipt digest", digest, e)
				}
				return
			}
			b, e, handled := referenceCodec(c.Kind, []byte(c.Canonical), pins[c.Fixture])
			if !handled {
				return
			}
			if e != nil || string(b) != c.Canonical {
				t.Fatal("DTO canonical", e)
			}
		})
	}
}

// These exact bytes are independent model storage fixtures, not authority
// imported by a public API. The concrete borrowed source still uses published
// normal-proof records; Open exercises only the declared cheap graph audit.
func TestEntryIndependentDiskRowsAudit(t *testing.T) {
	if wire.TestPoW {
		t.Skip("compiled normal fixture")
	}
	f := readEntryReference(t)
	for _, fixture := range []string{"zero256", "incrementing256"} {
		t.Run(fixture, func(t *testing.T) {
			x := newNamedEntryFixture(t, fixture, 3)
			if e := x.s.Close(); e != nil {
				t.Fatal(e)
			}
			x.s = nil
			var markerBytes []byte
			var rows []struct {
				KeyHex   string `json:"key_hex"`
				ValueHex string `json:"value_hex"`
			}
			for _, d := range f.DTOs {
				if d.Fixture != fixture {
					continue
				}
				if d.Kind == "Marker" {
					markerBytes = append([]byte(d.Canonical), '\n')
				}
				if d.Kind == "DiskRows" {
					rows = d.Rows
				}
			}
			if len(markerBytes) == 0 || len(rows) == 0 {
				t.Fatal("missing independent storage fixture")
			}
			if e := os.RemoveAll(x.path); e != nil {
				t.Fatal(e)
			}
			if e := os.MkdirAll(filepath.Join(x.path, "db"), 0700); e != nil {
				t.Fatal(e)
			}
			if e := os.WriteFile(filepath.Join(x.path, markerName), markerBytes, 0600); e != nil {
				t.Fatal(e)
			}
			db, e := leveldb.OpenFile(filepath.Join(x.path, "db"), nil)
			if e != nil {
				t.Fatal(e)
			}
			b := new(leveldb.Batch)
			for _, row := range rows {
				k, e := hex.DecodeString(row.KeyHex)
				if e != nil {
					t.Fatal(e)
				}
				v, e := hex.DecodeString(row.ValueHex)
				if e != nil {
					t.Fatal(e)
				}
				b.Put(k, v)
			}
			if e = db.Write(b, &opt.WriteOptions{Sync: true}); e != nil {
				t.Fatal(e)
			}
			db.Close()
			x.s, e = Open(context.Background(), x.path, x.pin, x.stable)
			if e != nil {
				t.Fatal("independent graph", e)
			}
		})
	}
}
