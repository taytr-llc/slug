package pilot

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/wire/v2"
	"os"
	"reflect"
	"testing"
)

func TestEventIndependentTransport(t *testing.T) {
	raw, e := os.ReadFile("../../gymevent/testdata/reference/event_v1.json")
	if e != nil {
		t.Fatal(e)
	}
	hash := sha256.Sum256(raw)
	if hex.EncodeToString(hash[:]) != "9287f92c0cb46c585b44274c70ae57c5d6c9a4234e7078fd1d69a200644f996a" {
		t.Fatal("frozen corpus pin")
	}
	if wire.TestPoW {
		t.Skip("independent fixture View pins use the production profile")
	}
	var corpus struct {
		Pin struct {
			Value gymevent.Pin `json:"value"`
		}
		DTOs []struct {
			Name, Kind, Canonical string
			Value                 json.RawMessage
		}
		CodecCases []struct {
			Name, Kind, InputHex, Canonical, ValidationScope string
			Accepted                                         bool
		}
	}
	// Snake-case corpus fields are explicit; no implementation-generated expectation.
	var doc map[string]json.RawMessage
	if json.Unmarshal(raw, &doc) != nil {
		t.Fatal("corpus")
	}
	var pinrow map[string]json.RawMessage
	_ = json.Unmarshal(doc["pin"], &pinrow)
	if e = json.Unmarshal(pinrow["value"], &corpus.Pin.Value); e != nil {
		t.Fatal("pin value", e)
	}
	if e = json.Unmarshal(doc["dtos"], &corpus.DTOs); e != nil {
		t.Fatal(e)
	}
	var cases []struct {
		Name, Kind, Canonical string
		InputHex              string `json:"input_hex"`
		ValidationScope       string `json:"validation_scope"`
		Accepted              bool
	}
	if e = json.Unmarshal(doc["codec_cases"], &cases); e != nil {
		t.Fatal(e)
	}
	pin := corpus.Pin.Value
	checked := 0
	for _, c := range corpus.DTOs {
		switch c.Kind {
		case "EventRequest":
			var q EventRequest
			if json.Unmarshal(c.Value, &q) != nil {
				t.Fatal(c.Name)
			}
			got, e := MarshalEventRequest(q)
			if e != nil || string(got) != c.Canonical {
				t.Fatal(c.Name, e)
			}
			checked++
		case "EventResponse":
			r, e := ParseEventResponse(c.Value)
			if e != nil {
				t.Fatal(c.Name, e)
			}
			got, e := MarshalEventResponse(r)
			if e != nil || string(got) != c.Canonical {
				t.Fatalf("%s: canonical %v", c.Name, e)
			}
			if !eventReferenceContext(r, pin) {
				t.Fatal(c.Name, "pinned semantics")
			}
			checked++
		}
	}
	for _, c := range cases {
		if c.Kind != "EventRequest" && c.Kind != "EventResponse" {
			continue
		}
		t.Run(c.Kind+"/"+c.Name, func(t *testing.T) {
			b, e := hex.DecodeString(c.InputHex)
			if e != nil {
				t.Fatal(e)
			}
			var got []byte
			if c.Kind == "EventRequest" {
				var q EventRequest
				q, e = ParseEventRequest(b)
				if e == nil {
					got, e = MarshalEventRequest(q)
				}
			} else {
				var r EventResponse
				r, e = ParseEventResponse(b)
				if e == nil && !eventReferenceContext(r, pin) {
					e = gymevent.ErrInvalid
				}
				if e == nil {
					got, e = MarshalEventResponse(r)
				}
			}
			if (e == nil) != c.Accepted {
				t.Fatalf("accepted=%v want=%v err=%v scope=%s", e == nil, c.Accepted, e, c.ValidationScope)
			}
			if e == nil && !bytes.Equal(got, []byte(c.Canonical)) {
				t.Fatal("canonical bytes")
			}
		})
		checked++
	}
	if checked < 50 {
		t.Fatal("insufficient helper corpus coverage", checked)
	}
	t.Log("independent helper DTO/codec cases", checked)
}

// The corpus labels helper negatives as transport PLUS established-pin
// semantic validation. Keep that layer explicit; the standalone parser never
// imports an authority pin from a non-Snapshot reply or reprojects on polling.
func eventReferenceContext(r EventResponse, p gymevent.Pin) bool {
	if !r.OK {
		return true
	}
	id, e := gymevent.ID(p)
	if e != nil {
		return false
	}
	if r.Snapshot != nil {
		if r.Snapshot.Pin != nil {
			return reflect.DeepEqual(*r.Snapshot.Pin, p) && r.StoreID == id
		}
		return true
	}
	if r.StoreID != id {
		return false
	}
	switch {
	case r.Status != nil:
		_, e = gymevent.ValidateEventStatus(p, *r.Status)
	case r.Record != nil:
		_, e = gymevent.ValidateSnapshotReceipt(p, *r.Record)
	case r.History != nil:
		_, e = gymevent.ValidateHistoryPage(p, *r.History)
	case r.Job != nil:
		var event gymevent.Event
		event, e = gymevent.EventForDate(p, r.Job.Saturday)
		if e == nil && (event.EventID != r.Job.EventID) {
			return false
		}
		found := false
		for _, ep := range p.Participants {
			if ep.StablePin.Identity.GymID == r.Job.ParticipantID && ep.StablePin.AccountID == r.Job.View.AccountID && ep.StablePin.AccountAuthorityID == r.Job.View.AccountAuthorityID && ep.StablePin.Source == r.Job.View.AuthorityPin {
				found = true
			}
		}
		if !found {
			return false
		}
		if e == nil && r.Job.Result != nil {
			_, e = gymevent.ValidateCaptureResult(p, *r.Job.Result)
		}
	}
	return e == nil
}

func TestEventCaptureTransportCreationLowerBound(t *testing.T) {
	if wire.TestPoW {
		t.Skip("independent fixture View pins use the production profile")
	}
	raw, e := os.ReadFile("../../gymevent/testdata/reference/event_v1.json")
	if e != nil {
		t.Fatal(e)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "9287f92c0cb46c585b44274c70ae57c5d6c9a4234e7078fd1d69a200644f996a" {
		t.Fatal("frozen corpus pin")
	}
	var corpus struct {
		DTOs []struct {
			Kind  string
			Value json.RawMessage
		}
	}
	if e = json.Unmarshal(raw, &corpus); e != nil {
		t.Fatal(e)
	}
	for _, row := range corpus.DTOs {
		if row.Kind != "EventResponse" {
			continue
		}
		r, e := ParseEventResponse(row.Value)
		if e != nil || r.Job == nil || r.Job.Result == nil {
			continue
		}
		if _, e = MarshalEventResponse(r); e != nil {
			t.Fatal("independent positive control", e)
		}
		v := r.Job.Result
		v.Status.CreatedAtMS = v.Capture.Evidence.CapturedAtMS + 1
		if v.Status.ObservedAtMS < v.Status.CreatedAtMS {
			v.Status.ObservedAtMS = v.Status.CreatedAtMS
		}
		if v.Status.CompletedAtMS != 0 && v.Status.CompletedAtMS < v.Status.CreatedAtMS {
			v.Status.CompletedAtMS = v.Status.CreatedAtMS
		}
		if !eventStatusShape(v.Status) || !eventParticipantShape(v.Capture.Snapshot, v.Status.Event) || !eventEvidenceShape(v.Capture.Evidence, v.Capture.Snapshot, v.Status.Event) {
			t.Fatal("mutation must isolate capture-before-event-creation relation")
		}
		bad, e := json.Marshal(r)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = ParseEventResponse(bad); e == nil {
			t.Fatal("capture before event creation accepted by standalone parser")
		}
		if _, e = MarshalEventResponse(r); e == nil {
			t.Fatal("capture before event creation accepted by standalone encoder")
		}
		return
	}
	t.Fatal("missing independent completed capture fixture")
}
