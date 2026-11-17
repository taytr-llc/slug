package pilot

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSeedClosedRequestAndUnavailableCodec(t *testing.T) {
	id := strings.Repeat("a", 64)
	for _, r := range []SeedRequest{{ID: 8, Command: "seed_publish", ViewID: id, Saturday: "2026-11-21"}, {ID: 9, Command: "seed_reconcile", ViewID: id, Saturday: "2026-11-21"}, {ID: 10, Command: "seed_attest", ViewID: id, Saturday: "2026-11-21"}, {ID: 1, Command: "seed_snapshot"}, {ID: 2, Command: "seed_status", ViewID: id, Saturday: "2026-11-21"}, {ID: 3, Command: "seed_record", ViewID: id, Saturday: "2026-11-21"}, {ID: 4, Command: "seed_prepare", ViewID: id, Saturday: "2026-11-21"}, {ID: 5, Command: "seed_history", ViewID: id, Limit: 8}, {ID: 6, Command: "seed_job", ViewID: id, JobID: id}, {ID: 7, Command: "seed_cancel", ViewID: id, JobID: id}} {
		b, e := MarshalSeedRequest(r)
		if e != nil {
			t.Fatal(e)
		}
		got, e := ParseSeedRequest(b)
		if e != nil || got != r {
			t.Fatal("roundtrip", e)
		}
		bad := append([]byte(nil), b...)
		bad = bytes.Replace(bad, []byte(`"id":`), []byte(`"id":1,"i\u0064":`), 1)
		if _, e = ParseSeedRequest(bad); e == nil {
			t.Fatal("decoded duplicate")
		}
		r.AfterEventKey = id
		r.Saturday = "2026-11-21"
		r.JobID = id
		if _, e = MarshalSeedRequest(r); e == nil {
			t.Fatal("unused fields")
		}
	}
	s := SeedSnapshot{Version: 1, State: "unavailable", Error: "seed_unavailable", NodeState: "offline"}
	r := SeedResponse{ID: 1, OK: true, Snapshot: &s}
	b, e := MarshalSeedResponse(r)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = ParseSeedResponse(b); e != nil {
		t.Fatal(e)
	}
	for _, bad := range [][]byte{append([]byte{0xef, 0xbb, 0xbf}, b...), append(b, []byte(`{}`)...), bytes.Replace(b, []byte(`"version":1`), []byte(`"version":1e0`), 1), bytes.Replace(b, []byte(`"error":"seed_unavailable"`), []byte(`"error":"outcome_unknown"`), 1), bytes.Replace(b, []byte(`"store_id":""`), []byte(`"store_id":null`), 1)} {
		if _, e = ParseSeedResponse(bad); e == nil {
			t.Fatal("invalid frame accepted", string(bad))
		}
	}
	old, _ := json.Marshal(EntryRequest{ID: 1, Command: "entry_snapshot"})
	if _, e = ParseSeedRequest(old); e == nil {
		t.Fatal("old family accepted")
	}
	if _, e = ParseEntryRequest([]byte(`{"id":1,"command":"seed_snapshot","view_id":"","saturday":"","after_event_key":"","limit":0,"job_id":""}`)); e == nil {
		t.Fatal("new family admitted by old codec")
	}
}
func TestSeedTransportNoProjectionDependencies(t *testing.T) {
	for _, name := range []string{"seed_codec.go", "seed_validation.go", "seed_protocol.go"} {
		raw, e := os.ReadFile(filepath.Join(".", name))
		if e != nil {
			t.Fatal(e)
		}
		for _, forbidden := range []string{"fightermanifest.Generate", "fightercondition.Project(", "fighterportrait.", "fighterbout.", "gymevent.ValidateCaptureResult(", "gymevent.ValidateSnapshotReceipt(", "gymseed.ValidateRecord(", "seednode.AttachOwned(", ".BlockHash(", "ed25519.Sign("} {
			if bytes.Contains(raw, []byte(forbidden)) {
				t.Fatalf("transport unexpectedly performs %s in %s", forbidden, name)
			}
		}
	}
}
