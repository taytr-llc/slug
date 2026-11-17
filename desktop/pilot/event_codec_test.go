package pilot

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEventClosedRequestAndUnavailableCodec(t *testing.T) {
	id := strings.Repeat("a", 64)
	for _, r := range []EventRequest{{ID: 1, Command: "event_snapshot"}, {ID: 2, Command: "event_status", ViewID: id, Saturday: "2026-11-21"}, {ID: 3, Command: "event_record", ViewID: id, Saturday: "2026-11-21"}, {ID: 4, Command: "event_capture", ViewID: id, Saturday: "2026-11-21"}, {ID: 5, Command: "event_history", ViewID: id, Limit: 8}, {ID: 6, Command: "event_job", ViewID: id, JobID: id}, {ID: 7, Command: "event_cancel", ViewID: id, JobID: id}} {
		b, e := MarshalEventRequest(r)
		if e != nil {
			t.Fatal(e)
		}
		got, e := ParseEventRequest(b)
		if e != nil || got != r {
			t.Fatal("roundtrip", e)
		}
		bad := append([]byte(nil), b...)
		bad = bytes.Replace(bad, []byte(`"id":`), []byte(`"id":1,"i\u0064":`), 1)
		if _, e = ParseEventRequest(bad); e == nil {
			t.Fatal("decoded duplicate")
		}
		r.AfterEventID = id
		r.Saturday = "2026-11-21"
		r.JobID = id
		if _, e = MarshalEventRequest(r); e == nil {
			t.Fatal("unused fields")
		}
	}
	s := EventSnapshot{Version: 1, State: "unavailable", Error: "event_unavailable"}
	r := EventResponse{ID: 1, OK: true, Snapshot: &s}
	b, e := MarshalEventResponse(r)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = ParseEventResponse(b); e != nil {
		t.Fatal(e)
	}
	for _, bad := range [][]byte{append([]byte{0xef, 0xbb, 0xbf}, b...), append(b, []byte(`{}`)...), bytes.Replace(b, []byte(`"version":1`), []byte(`"version":1e0`), 1), bytes.Replace(b, []byte(`"error":"event_unavailable"`), []byte(`"error":"outcome_unknown"`), 1), bytes.Replace(b, []byte(`"store_id":""`), []byte(`"store_id":null`), 1)} {
		if _, e = ParseEventResponse(bad); e == nil {
			t.Fatal("invalid frame accepted", string(bad))
		}
	}
	old, _ := json.Marshal(EntryRequest{ID: 1, Command: "entry_snapshot"})
	if _, e = ParseEventRequest(old); e == nil {
		t.Fatal("old family accepted")
	}
	if _, e = ParseEntryRequest([]byte(`{"id":1,"command":"event_snapshot","view_id":"","saturday":"","after_event_id":"","limit":0,"job_id":""}`)); e == nil {
		t.Fatal("new family admitted by old codec")
	}
}
func TestEventTransportNoProjectionDependencies(t *testing.T) {
	for _, name := range []string{"event_codec.go", "event_validation.go", "event_protocol.go"} {
		raw, e := os.ReadFile(filepath.Join(".", name))
		if e != nil {
			t.Fatal(e)
		}
		for _, forbidden := range []string{"fightermanifest.Generate", "fightercondition.Project(", "fighterportrait.", "fighterbout.", "gymevent.ValidateCaptureResult(", "gymevent.ValidateSnapshotReceipt("} {
			if bytes.Contains(raw, []byte(forbidden)) {
				t.Fatalf("transport unexpectedly performs %s in %s", forbidden, name)
			}
		}
	}
}
