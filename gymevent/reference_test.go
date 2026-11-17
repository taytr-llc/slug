// Copyright (c) 2026 The SLUG developers. ISC license.
package gymevent

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

const eventCorpusSHA256 = "9287f92c0cb46c585b44274c70ae57c5d6c9a4234e7078fd1d69a200644f996a"

type referenceDTO struct {
	Name, Kind, Fixture, Canonical string
	Value                          json.RawMessage
}
type referenceHistory struct {
	Name, Fixture, SourceKind, Saturday string
	Snapshot                            ParticipantSnapshot `json:"expected_snapshot"`
	Canonical                           string              `json:"canonical_snapshot"`
}
type eventCorpus struct {
	Scenarios []struct {
		Name        string
		CreatedAtMS int64 `json:"created_at_ms"`
		Steps       []struct {
			Result json.RawMessage
			State  struct {
				ClockMS    int64  `json:"clock_ms"`
				EventCount uint32 `json:"event_count"`
				Events     []struct {
					Status        EventStatus
					CaptureSHA    map[string]string `json:"participant_capture_sha256"`
					ReceiptDigest string            `json:"receipt_digest"`
				}
			}
		}
	}
	Initialization []struct {
		Name, Method, State string
		PostPin             int64 `json:"post_pin_sample_ms"`
		OldCreated          int64 `json:"old_created_at_ms"`
		Accepted            bool
		ExpectedCreated     int64 `json:"expected_created_at_ms"`
	} `json:"initialization_cases"`

	Rules struct{ ID string }
	Pin   struct {
		Value     Pin
		Canonical string
		StoreID   string `json:"store_id"`
	}
	Calendar []struct {
		Saturday           string
		CutoffMS           int64 `json:"cutoff_ms"`
		IncidentDeadlineMS int64 `json:"incident_deadline_ms"`
		Event              Event
	}
	CalendarInvalid []struct{ Saturday string } `json:"calendar_invalid"`
	Histories       []referenceHistory
	Records         []struct {
		Name, Canonical  string
		Value            SnapshotReceipt
		SnapshotDigest   string `json:"snapshot_digest"`
		ReceiptDigest    string `json:"receipt_digest"`
		SnapshotPreimage string `json:"snapshot_preimage"`
		ReceiptPreimage  string `json:"receipt_preimage"`
	}
	DTOs       []referenceDTO `json:"dtos"`
	CodecCases []struct {
		Name, Kind, Canonical string
		InputHex              string `json:"input_hex"`
		Accepted              bool
	} `json:"codec_cases"`
}

func readEventCorpus(t testing.TB) eventCorpus {
	t.Helper()
	raw, e := os.ReadFile("testdata/reference/event_v1.json")
	if e != nil {
		t.Fatal(e)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != eventCorpusSHA256 {
		t.Fatal("independent corpus hash changed")
	}
	var c eventCorpus
	if e = json.Unmarshal(raw, &c); e != nil {
		t.Fatal(e)
	}
	return c
}
func referenceCoreCodec(c eventCorpus, kind string, raw []byte) ([]byte, error, bool) {
	p := c.Pin.Value
	switch kind {
	case "Pin":
		v, e := ParsePin(raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalPin(v)
		return b, e, true
	case "Info":
		v, e := ParseInfo(p, raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalInfo(p, v)
		return b, e, true
	case "Event":
		v, e := ParseEvent(p, raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalEvent(p, v)
		return b, e, true
	case "ParticipantSnapshot":
		v, e := ParseParticipantSnapshot(p, raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalParticipantSnapshot(p, v)
		return b, e, true
	case "ParticipantCapture":
		v, e := ParseParticipantCapture(p, raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalParticipantCapture(p, v)
		return b, e, true
	case "SnapshotInput":
		v, e := ParseSnapshotInput(p, raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalSnapshotInput(p, v)
		return b, e, true
	case "SnapshotReceipt":
		v, e := ParseSnapshotReceipt(p, raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalSnapshotReceipt(p, v)
		return b, e, true
	case "EventStatus":
		v, e := ParseEventStatus(p, raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalEventStatus(p, v)
		return b, e, true
	case "CaptureResult":
		v, e := ParseCaptureResult(p, raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalCaptureResult(p, v)
		return b, e, true
	case "HistoryPage":
		v, e := ParseHistoryPage(p, raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalHistoryPage(p, v)
		return b, e, true
	case "CaptureEvidence":
		var v CaptureEvidence
		if e := decode(raw, MaxEvidenceSize, &v, false); e != nil {
			return nil, e, true
		}
		for _, h := range c.Histories {
			if h.Name == h.Fixture+"/ordinary" && h.Snapshot.ParticipantID == v.ParticipantID {
				if e := validateEvidence(v, p, h.Snapshot); e != nil {
					return nil, e, true
				}
				b, e := json.Marshal(v)
				return b, e, true
			}
		}
		return nil, ErrInvalid, true
	case "Marker":
		var v marker
		if e := decode(raw, MaxMarkerSize, &v, false); e != nil {
			return nil, e, true
		}
		if !equal(v, marker{"slug-candidate-events", 1, c.Pin.StoreID, p}) {
			return nil, ErrInvalid, true
		}
		b, e := json.Marshal(v)
		return b, e, true
	case "Init":
		var v initIntent
		if e := decode(raw, MaxMetadataSize, &v, false); e != nil {
			return nil, e, true
		}
		if !equal(v, initIntent{1, c.Pin.StoreID, p}) {
			return nil, ErrInvalid, true
		}
		b, e := json.Marshal(v)
		return b, e, true
	case "Metadata":
		var v metadata
		if e := decode(raw, MaxMetadataSize, &v, false); e != nil {
			return nil, e, true
		}
		if v.Version != 1 || v.StoreID != c.Pin.StoreID || !equal(v.Pin, p) || !timestamp(v.CreatedAtMS) || !timestamp(v.ClockMS) || v.ClockMS < v.CreatedAtMS || v.EventCount > MaxEvents {
			return nil, ErrInvalid, true
		}
		b, e := json.Marshal(v)
		return b, e, true
	}
	return nil, nil, false
}
func TestEventReferenceCalendarIdentityAndCanonicalCore(t *testing.T) {
	c := readEventCorpus(t)
	if RulesID() != c.Rules.ID {
		t.Fatal("rules id")
	}
	id, e := ID(c.Pin.Value)
	if e != nil || id != c.Pin.StoreID {
		t.Fatal("store id", e)
	}
	if len(c.Calendar) != 3861 {
		t.Fatal("calendar coverage")
	}
	for _, row := range c.Calendar {
		v, e := EventForDate(c.Pin.Value, row.Saturday)
		if e != nil || v != row.Event || v.CutoffMS != row.CutoffMS || v.IncidentDeadlineMS != row.IncidentDeadlineMS {
			t.Fatalf("calendar %s: %v", row.Saturday, e)
		}
	}
	for _, row := range c.CalendarInvalid {
		if _, e := EventForDate(c.Pin.Value, row.Saturday); e == nil {
			t.Fatalf("accepted date %q", row.Saturday)
		}
	}
	n := 0
	for _, row := range c.DTOs {
		got, e, handled := referenceCoreCodec(c, row.Kind, row.Value)
		if !handled {
			continue
		}
		n++
		if e != nil || string(got) != row.Canonical {
			t.Errorf("%s/%s: %v canonical_match=%v", row.Kind, row.Name, e, string(got) == row.Canonical)
		}
	}
	if n != 26 {
		t.Fatalf("core DTO coverage %d, want 26", n)
	}
	for _, row := range c.Histories {
		v, e := ValidateParticipantSnapshot(c.Pin.Value, row.Snapshot)
		if e != nil {
			t.Errorf("history snapshot %s: %v", row.Name, e)
			continue
		}
		b, e := MarshalParticipantSnapshot(c.Pin.Value, v)
		if e != nil || string(b) != row.Canonical {
			t.Errorf("history canonical %s: %v", row.Name, e)
		}
	}
	for _, row := range c.Records {
		b, e := MarshalSnapshotReceipt(c.Pin.Value, row.Value)
		if e != nil || string(b) != row.Canonical {
			t.Errorf("record %s: %v", row.Name, e)
			continue
		}
		d, e := SnapshotDigest(c.Pin.Value, row.Value.Input)
		rd, re := ReceiptDigest(c.Pin.Value, row.Value)
		if e != nil || re != nil || d != row.SnapshotDigest || rd != row.ReceiptDigest {
			t.Errorf("digest %s", row.Name)
		}
		ib, _ := MarshalSnapshotInput(c.Pin.Value, row.Value.Input)
		if hex.EncodeToString(append([]byte("SLUG candidate cutoff input\x00v1\x00"), lp(ib)...)) != row.SnapshotPreimage || hex.EncodeToString(append([]byte("SLUG candidate cutoff receipt\x00v1\x00"), lp(b)...)) != row.ReceiptPreimage {
			t.Errorf("preimage %s", row.Name)
		}
	}
}
func TestEventReferenceCodecMutations(t *testing.T) {
	c := readEventCorpus(t)
	n := 0
	for _, row := range c.CodecCases {
		t.Run(row.Kind+"/"+row.Name, func(t *testing.T) {
			raw, e := hex.DecodeString(row.InputHex)
			if e != nil {
				t.Fatal(e)
			}
			got, e, handled := referenceCoreCodec(c, row.Kind, raw)
			if !handled {
				return
			}
			n++
			if (e == nil) != row.Accepted {
				t.Fatalf("accepted=%v wanted=%v: %v", e == nil, row.Accepted, e)
			}
			if row.Accepted && !bytes.Equal(got, []byte(row.Canonical)) {
				t.Fatal("accepted canonical bytes differ")
			}
		})
	}
	if n != 278 {
		t.Fatalf("core codec coverage %d, want 278", n)
	}
}

func TestEventCaptureResultCannotPredateReservation(t *testing.T) {
	c := readEventCorpus(t)
	for _, row := range c.DTOs {
		if row.Kind != "CaptureResult" {
			continue
		}
		v, e := ParseCaptureResult(c.Pin.Value, row.Value)
		if e != nil {
			t.Fatal("independent positive control", e)
		}
		v.Status.CreatedAtMS = v.Capture.Evidence.CapturedAtMS + 1
		v.Status.ObservedAtMS = max(v.Status.ObservedAtMS, v.Status.CreatedAtMS)
		if v.Status.Phase == "snapshotted" {
			v.Status.CompletedAtMS = max(v.Status.CompletedAtMS, v.Status.CreatedAtMS)
		}
		if validateEventStatus(c.Pin.Value, v.Status) != nil {
			t.Fatal("mutation must preserve otherwise legal status")
		}
		raw, _ := json.Marshal(v)
		if _, e := ParseCaptureResult(c.Pin.Value, raw); e == nil {
			t.Fatal("capture predating reservation accepted")
		}
		return
	}
	t.Fatal("missing independent capture result")
}
