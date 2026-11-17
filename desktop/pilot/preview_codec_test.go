// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"github.com/taytr-llc/slug/gymstable"
	"io"
	"strings"
	"testing"
)

func TestPreviewRequestClosedFamily(t *testing.T) {
	id := strings.Repeat("1", 64)
	a := strings.Repeat("2", 64)
	b := strings.Repeat("3", 64)
	values := []PreviewRequest{{ID: 1, Version: 1, Command: "preview_snapshot"}, {ID: 2, Version: 1, Command: "preview_begin", ViewID: id, FighterAID: a, FighterBID: b}, {ID: 3, Version: 1, Command: "preview_job", ViewID: id, JobID: a}, {ID: 4, Version: 1, Command: "preview_cancel", ViewID: id, JobID: a}}
	for _, v := range values {
		raw, e := MarshalPreviewRequest(v)
		if e != nil {
			t.Fatal(e)
		}
		got, e := ParsePreviewRequest(append([]byte(" \n"), raw...))
		if e != nil || got != v {
			t.Fatal("roundtrip", e)
		}
		if _, e = ParseStableRequest(raw); e == nil {
			t.Fatal("old route widened")
		}
	}
	raw, _ := MarshalPreviewRequest(values[1])
	bad := [][]byte{bytes.Replace(raw, []byte(`"version":1`), []byte(`"version":1e0`), 1), bytes.Replace(raw, []byte(`"version":1`), []byte(`"version":null`), 1), bytes.Replace(raw, []byte(`"id":2`), []byte(`"id":2,"\u0069d":2`), 1), append(raw, []byte("{}")...), bytes.Replace(raw, []byte(`"fighter_a_id":"`+a), []byte(`"fighter_a_id":"`+b), 1), bytes.Replace(raw, []byte(`"version":1`), []byte(`"version":1,"unknown":0`), 1)}
	for i, r := range bad {
		if _, e := ParsePreviewRequest(r); e == nil {
			t.Fatal("accepted mutation", i)
		}
	}
}
func TestPreviewUnavailableExactAndNoSimulation(t *testing.T) {
	v := PreviewResponse{ID: 1, Version: 1, OK: true, Snapshot: &PreviewSnapshot{Version: 1, State: "unavailable"}}
	raw, e := MarshalPreviewResponse(v)
	if e != nil {
		t.Fatal(e)
	}
	out, e := ParsePreviewResponse(raw)
	if e != nil || out.Snapshot == nil || *out.Snapshot != *v.Snapshot {
		t.Fatal(e)
	}
	for _, bad := range [][]byte{bytes.Replace(raw, []byte(`"engine_id":""`), []byte(`"engine_id":"slug-fighter-bout-candidate-v1"`), 1), bytes.Replace(raw, []byte(`"view":null,`), nil, 1), bytes.Replace(raw, []byte(`"version":1`), []byte(`"version":1,"\u0076ersion":1`), 1), bytes.Replace(raw, []byte(`"snapshot":`), []byte(`"Snapshot":`), 1)} {
		if _, e = ParsePreviewResponse(bad); e == nil {
			t.Fatal("nonblank/alias accepted")
		}
	}
	v.OK = false
	v.Snapshot = nil
	v.Error = "preview_not_found"
	raw, e = MarshalPreviewResponse(v)
	if e != nil {
		t.Fatal(e)
	}
	var fields map[string]json.RawMessage
	json.Unmarshal(raw, &fields)
	if len(fields) != 4 {
		t.Fatal(fields)
	}
}

type previewProtocolController struct{ stableProtocolController }

func (c *previewProtocolController) PreviewSnapshot() PreviewSnapshot {
	return PreviewSnapshot{Version: 1, State: "unavailable"}
}
func (c *previewProtocolController) BeginFightPreview(context.Context, string, string, string) (PreviewJob, error) {
	return PreviewJob{}, c.failure
}
func (c *previewProtocolController) FightPreviewJob(context.Context, string, string) (PreviewJob, error) {
	return PreviewJob{}, c.failure
}
func (c *previewProtocolController) CancelFightPreview(context.Context, string, string) (PreviewJob, error) {
	return PreviewJob{}, c.failure
}
func TestPreviewServePreservesFamiliesAndDrain(t *testing.T) {
	input, client := io.Pipe()
	responses, output := io.Pipe()
	c := &previewProtocolController{stableProtocolController: stableProtocolController{failure: gymstable.ErrNotFound}}
	done := make(chan error, 1)
	go func() { done <- ServePreview(context.Background(), input, output, c) }()
	reader := bufio.NewReader(responses)
	send := func(raw string) []byte {
		t.Helper()
		if _, e := io.WriteString(client, raw+"\n"); e != nil {
			t.Fatal(e)
		}
		line, e := reader.ReadBytes('\n')
		if e != nil {
			t.Fatal(e)
		}
		return bytes.TrimSuffix(line, []byte{'\n'})
	}
	raw := send(`{"id":1,"command":"snapshot"}`)
	var old ManifestResponse
	if json.Unmarshal(raw, &old) != nil || !old.OK || old.Snapshot == nil {
		t.Fatal("manifest family")
	}
	raw = send(`{"id":2,"command":"stable_snapshot"}`)
	if s, e := ParseStableResponse(raw); e != nil || s.Snapshot == nil {
		t.Fatal("stable family", e)
	}
	raw = send(`{"id":3,"version":1,"command":"preview_snapshot"}`)
	if p, e := ParsePreviewResponse(raw); e != nil || p.Snapshot == nil {
		t.Fatal("preview family", e)
	}
	id := strings.Repeat("1", 64)
	raw = send(`{"id":4,"version":1,"command":"preview_job","view_id":"` + id + `","job_id":"` + id + `"}`)
	if p, e := ParsePreviewResponse(raw); e != nil || p.OK || p.Error != "preview_not_found" {
		t.Fatal("new scoped error", e)
	}
	_ = client.Close()
	if e := <-done; e != nil {
		t.Fatal(e)
	}
	responses.Close()
	if c.stopped.Load() == 0 || c.closed.Load() != 1 {
		t.Fatal("shutdown not drained")
	}
}
