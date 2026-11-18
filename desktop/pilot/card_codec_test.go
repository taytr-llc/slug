package pilot

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCardClosedRequestAndUnavailableCodec(t *testing.T) {
	id := strings.Repeat("a", 64)
	for _, r := range []CardRequest{{ID: 8, Command: "card_build", ViewID: id, Saturday: "2026-11-21"}, {ID: 9, Command: "card_build", ViewID: id, Saturday: "2026-11-21"}, {ID: 10, Command: "card_build", ViewID: id, Saturday: "2026-11-21"}, {ID: 1, Command: "card_snapshot"}, {ID: 2, Command: "card_status", ViewID: id, Saturday: "2026-11-21"}, {ID: 3, Command: "card_record", ViewID: id, Saturday: "2026-11-21"}, {ID: 4, Command: "card_build", ViewID: id, Saturday: "2026-11-21"}, {ID: 5, Command: "card_history", ViewID: id, Limit: 8}, {ID: 6, Command: "card_job", ViewID: id, JobID: id}, {ID: 7, Command: "card_cancel", ViewID: id, JobID: id}} {
		b, e := MarshalCardRequest(r)
		if e != nil {
			t.Fatal(e)
		}
		got, e := ParseCardRequest(b)
		if e != nil || got != r {
			t.Fatal("roundtrip", e)
		}
		bad := append([]byte(nil), b...)
		bad = bytes.Replace(bad, []byte(`"id":`), []byte(`"id":1,"i\u0064":`), 1)
		if _, e = ParseCardRequest(bad); e == nil {
			t.Fatal("decoded duplicate")
		}
		r.AfterCardID = id
		r.Saturday = "2026-11-21"
		r.JobID = id
		if _, e = MarshalCardRequest(r); e == nil {
			t.Fatal("unused fields")
		}
	}
	s := CardSnapshot{Version: 1, State: "unavailable", Error: "card_unavailable"}
	r := CardResponse{ID: 1, OK: true, Snapshot: &s}
	b, e := MarshalCardResponse(r)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = ParseCardResponse(b); e != nil {
		t.Fatal(e)
	}
	for _, bad := range [][]byte{append([]byte{0xef, 0xbb, 0xbf}, b...), append(b, []byte(`{}`)...), bytes.Replace(b, []byte(`"version":1`), []byte(`"version":1e0`), 1), bytes.Replace(b, []byte(`"error":"card_unavailable"`), []byte(`"error":"outcome_unknown"`), 1), bytes.Replace(b, []byte(`"store_id":""`), []byte(`"store_id":null`), 1)} {
		if _, e = ParseCardResponse(bad); e == nil {
			t.Fatal("invalid frame accepted", string(bad))
		}
	}
	old, _ := json.Marshal(EntryRequest{ID: 1, Command: "entry_snapshot"})
	if _, e = ParseCardRequest(old); e == nil {
		t.Fatal("old family accepted")
	}
	if _, e = ParseEntryRequest([]byte(`{"id":1,"command":"card_snapshot","view_id":"","saturday":"","after_card_id":"","limit":0,"job_id":""}`)); e == nil {
		t.Fatal("new family admitted by old codec")
	}
}
func TestCardTransportNoProjectionDependencies(t *testing.T) {
	for _, name := range []string{"card_codec.go", "card_validation.go", "card_protocol.go"} {
		raw, e := os.ReadFile(filepath.Join(".", name))
		if e != nil {
			t.Fatal(e)
		}
		for _, forbidden := range []string{"fightermanifest.Generate", "fightercondition.Project(", "fighterportrait.", "fighterbout.", "gymevent.ValidateCaptureResult(", "gymevent.ValidateSnapshotReceipt(", "gymcard.ValidateRecord(", "gymcard.DerivePlan(", "gymcard.ValidateBuildResult(", "gymcard.ValidateReadResult(", "ed25519.Verify(", "seedSignedShape(", "seedCheckpointShape(", "cardnode.AttachOwned(", ".BlockHash(", "ed25519.Sign("} {
			if bytes.Contains(raw, []byte(forbidden)) {
				t.Fatalf("transport unexpectedly performs %s in %s", forbidden, name)
			}
		}
	}
}
