// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/fightercondition"
	"github.com/taytr-llc/slug/fightermanifest"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

func TestStableStrictRequestCommands(t *testing.T) {
	id := strings.Repeat("a", 64)
	requests := []StableRequest{
		{Command: "stable_snapshot"}, {Command: "stable_recent", ViewID: id}, {Command: "stable_sync", ViewID: id}, {Command: "stable_inventory", ViewID: id, After: 0, Limit: 8},
		{Command: "stable_roster", ViewID: id, FirstSlot: 9, Limit: 2}, {Command: "stable_prepare_slot", ViewID: id, ExpectedRevision: 0, Slot: 10, ProspectID: ""},
		{Command: "stable_apply_slot", ViewID: id, OperationID: id}, {Command: "stable_job", ViewID: id, JobID: id}, {Command: "stable_operation", ViewID: id, OperationID: id},
		{Command: "stable_pending", ViewID: id}, {Command: "stable_abandon_operation", ViewID: id, OperationID: id}, {Command: "stable_cancel", ViewID: id, JobID: id},
	}
	for _, r := range requests {
		r.ID = 1
		raw, err := MarshalStableRequest(r)
		if err != nil {
			t.Fatal(r.Command, err)
		}
		got, err := ParseStableRequest(raw)
		if err != nil || got != r {
			t.Fatal("required zeros lost", string(raw), err)
		}
		if _, err = parseManifestRequest(raw); err == nil {
			t.Fatal("old route accepted stable command")
		}
		for _, bad := range [][]byte{bytes.Replace(raw, []byte(`"id":1`), []byte(`"id":1,"\u0069d":2`), 1), bytes.Replace(raw, []byte(`"id":1`), []byte(`"id":1e0`), 1), append(append([]byte{}, raw...), []byte(" {}")...)} {
			if _, err = ParseStableRequest(bad); err == nil {
				t.Fatal("malformed selected command accepted")
			}
		}
	}
	for _, raw := range []string{
		`{"id":1,"command":"stable_inventory","view_id":"` + id + `","limit":8}`,
		`{"id":1,"command":"stable_prepare_slot","view_id":"` + id + `","slot":1,"expected_revision":0}`,
		`{"id":1,"command":"stable_snapshot","after":0}`,
		`{"id":1,"command":"stable_pending","view_id":null}`,
		`{"id":1,"command":"stable_inventory","view_id":"` + id + `","after":-0,"limit":8}`,
		`{"id":1,"command":"stable_inventory","view_id":"` + id + `","after":0,"limit":9}`,
		`{"id":1,"command":"stable_prepare_slot","view_id":"` + id + `","expected_revision":0,"slot":1,"prospect_id":"","xp":0}`,
	} {
		if _, err := ParseStableRequest([]byte(raw)); err == nil {
			t.Fatal("invalid selected fields accepted", raw)
		}
	}
	r := requests[0]
	r.ID = 1
	r.Limit = 8
	if _, err := MarshalStableRequest(r); err == nil {
		t.Fatal("encoder ignored unrelated field")
	}
}

func stableTransportReference(t *testing.T) (ManifestViewPin, gymstable.Status, []gymstable.Item) {
	t.Helper()
	raw, err := os.ReadFile("../../gymstable/testdata/reference/stable_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var c struct {
		Pins []struct {
			Name string
			Pin  gymstable.Pin
			ID   string `json:"expected_id"`
		}
		Cases []struct {
			Fixture    string
			Record     registry.ManifestRecord
			Assignment gymstable.Assignment
			Manifest   fightermanifest.Manifest
			Conditions []struct {
				AsOf     int64 `json:"as_of_ms"`
				Expected fightercondition.Snapshot
			}
		}
	}
	if json.Unmarshal(raw, &c) != nil {
		t.Fatal("reference decode")
	}
	input, err := os.ReadFile("../../gymstable/testdata/reference/inputs/public-candidate-records.json")
	if err != nil {
		t.Fatal(err)
	}
	var original struct {
		Records []struct{ View ManifestViewPin }
	}
	if json.Unmarshal(input, &original) != nil {
		t.Fatal("input decode")
	}
	view := original.Records[0].View
	pin := c.Pins[0]
	s := gymstable.Status{Version: 1, StoreID: pin.ID, Pin: pin.Pin, Checkpoint: 3, ObservedSourceCount: 3, CaughtUp: true}
	items := []gymstable.Item{}
	for _, r := range c.Cases {
		if r.Fixture != pin.Name {
			continue
		}
		items = append(items, gymstable.Item{Assignment: r.Assignment, Manifest: r.Manifest, Condition: r.Conditions[0].Expected})
	}
	return view, s, items
}
func TestStableFullNestedResponseAndBoundaries(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal candidate reference profile")
	}
	view, status, items := stableTransportReference(t)
	validSnapshot := StableSnapshot{Version: 1, State: "ready", View: &view, Status: &status}
	rawSnapshot, _ := json.Marshal(StableResponse{ID: 1, OK: true, Snapshot: &validSnapshot})
	if _, err := ParseStableResponse(rawSnapshot); err != nil {
		t.Fatal("valid ready snapshot", err)
	}
	validSnapshot.State = "unavailable"
	rawSnapshot, _ = json.Marshal(StableResponse{ID: 1, OK: true, Snapshot: &validSnapshot})
	if _, err := ParseStableResponse(rawSnapshot); err == nil {
		t.Fatal("unavailable snapshot disclosed retained account state")
	}
	validSnapshot.State = "syncing"
	validSnapshot.ActiveJobID = strings.Repeat("b", 64)
	validSnapshot.QueuedJobID = validSnapshot.ActiveJobID
	rawSnapshot, _ = json.Marshal(StableResponse{ID: 1, OK: true, Snapshot: &validSnapshot})
	if _, err := ParseStableResponse(rawSnapshot); err == nil {
		t.Fatal("one job appeared both active and queued")
	}

	for _, item := range items {
		s := status
		s.LastAsOfMS = item.Condition.AsOfMS
		s.RosterRevision = 1
		s.OperationCount = 1
		s.LastOperationID = strings.Repeat("e", 64)
		s.Slots[0] = item.Assignment.ProspectID
		page := gymstable.RosterPage{Version: 1, Status: s, AsOfMS: s.LastAsOfMS, FirstSlot: 1, NextSlot: 9, Items: []gymstable.RosterItem{{Slot: 1, Item: item}}}
		job := StableJob{Version: 1, JobID: strings.Repeat("f", 64), Kind: "roster", State: "done", View: view, StoreID: s.StoreID, Roster: &page}
		result := StableResponse{ID: 1, OK: true, Job: &job}
		raw, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = ParseStableResponse(raw); err != nil {
			t.Fatal("valid container-depth9 roster rejected", err, item.Manifest.Portrait.ShoulderJitter)
		}
		var output bytes.Buffer
		if err = respondStable(&output, result); err != nil {
			t.Fatal("typed publisher rejects parser-positive DTO", err)
		}
		if !bytes.Equal(output.Bytes(), append(raw, '\n')) {
			t.Fatal("canonical response/LF changed")
		}
		for _, bad := range [][]byte{
			bytes.Replace(raw, []byte(`"id":1`), []byte(`"id":1,"id":1`), 1),
			bytes.Replace(raw, []byte(`"ok":true`), []byte(`"ok":true,"snapshot":null`), 1),
			bytes.Replace(raw, []byte(`"as_of_ms":`), []byte(`"As_of_ms":`), 1),
			bytes.Replace(raw, []byte(`"pro_xp":0`), []byte(`"pro_xp":1`), 1),
			bytes.Replace(raw, []byte(`"slot":1`), []byte(`"slot":2`), 1),
		} {
			if _, err = ParseStableResponse(bad); err == nil {
				t.Fatal("invalid typed nested relation accepted")
			}
		}
		job.Roster.Items = append(job.Roster.Items, job.Roster.Items[0])
		bad, _ := json.Marshal(result)
		if _, err = ParseStableResponse(bad); err == nil {
			t.Fatal("duplicate roster detail accepted")
		}
	}
	s := StableSnapshot{Version: 1, State: "unavailable"}
	raw, _ := json.Marshal(StableResponse{ID: 1, OK: true, Snapshot: &s})
	if _, err := ParseStableResponse(raw); err != nil {
		t.Fatal("blank snapshot", err)
	}
	op := StableOperation{Version: 1, View: view, StoreID: status.StoreID}
	raw, _ = json.Marshal(StableResponse{ID: 1, OK: true, Operation: &op})
	if _, err := ParseStableResponse(raw); err != nil {
		t.Fatal("Pending null shape", err)
	}
	for _, raw := range []string{`{"id":1,"ok":false,"error":"private file path"}`, `{"id":1,"ok":true,"job":null}`, `{"id":1,"ok":false,"error":"busy","job":null}`, `{"id":9007199254740992,"ok":false,"error":"busy"}`} {
		if _, err := ParseStableResponse([]byte(raw)); err == nil {
			t.Fatal("open result envelope", raw)
		}
	}
}

type stableProtocolController struct {
	manifestProtocolController
	failure  error
	prepared gymstable.SlotRequest
}

func (c *stableProtocolController) StableSnapshot() StableSnapshot {
	return StableSnapshot{Version: 1, State: "unavailable"}
}
func (c *stableProtocolController) BeginStableSync(context.Context, string) (StableJob, error) {
	return StableJob{}, c.failure
}
func (c *stableProtocolController) BeginStableInventory(context.Context, string, uint64, uint32) (StableJob, error) {
	return StableJob{}, c.failure
}
func (c *stableProtocolController) BeginStableRoster(context.Context, string, uint32, uint32) (StableJob, error) {
	return StableJob{}, c.failure
}
func (c *stableProtocolController) BeginStablePrepare(_ context.Context, _ string, r gymstable.SlotRequest) (StableJob, error) {
	c.prepared = r
	return StableJob{}, c.failure
}
func (c *stableProtocolController) BeginStableApply(context.Context, string, string) (StableJob, error) {
	return StableJob{}, c.failure
}
func (c *stableProtocolController) StableJob(context.Context, string, string) (StableJob, error) {
	return StableJob{}, c.failure
}
func (c *stableProtocolController) StableOperation(context.Context, string, string) (StableOperation, error) {
	return StableOperation{}, c.failure
}
func (c *stableProtocolController) StablePending(context.Context, string) (StableOperation, error) {
	return StableOperation{}, c.failure
}
func (c *stableProtocolController) AbandonStableOperation(context.Context, string, string) (StableOperation, error) {
	return StableOperation{}, c.failure
}
func (c *stableProtocolController) CancelStableJob(context.Context, string, string) (StableJob, error) {
	return StableJob{}, c.failure
}
func TestStableServeSelectsExactFamilyAndPreservesUnknown(t *testing.T) {
	input, client := io.Pipe()
	responses, output := io.Pipe()
	controller := &stableProtocolController{failure: gymstable.ErrOutcomeUnknown}
	done := make(chan error, 1)
	go func() { done <- ServeStable(context.Background(), input, output, controller) }()
	reader := bufio.NewReader(responses)
	send := func(raw string) []byte {
		t.Helper()
		if _, err := io.WriteString(client, raw+"\n"); err != nil {
			t.Fatal(err)
		}
		line, err := reader.ReadBytes('\n')
		if err != nil {
			t.Fatal(err)
		}
		return bytes.TrimSuffix(line, []byte{'\n'})
	}
	raw := send(`{"id":1,"command":"snapshot"}`)
	var old ManifestResponse
	if json.Unmarshal(raw, &old) != nil || !old.OK || old.Snapshot == nil {
		t.Fatal("old family changed")
	}
	raw = send(`{"id":2,"command":"stable_snapshot"}`)
	if v, err := ParseStableResponse(raw); err != nil || !v.OK || v.Snapshot == nil {
		t.Fatal("new family", err)
	}
	id := strings.Repeat("a", 64)
	raw = send(`{"id":3,"command":"stable_apply_slot","view_id":"` + id + `","operation_id":"` + id + `"}`)
	if v, err := ParseStableResponse(raw); err != nil || v.OK || v.Error != "outcome_unknown" {
		t.Fatal("unknown became definite", err)
	}
	raw = send(`{"id":4,"command":"stable_prepare_slot","view_id":"` + id + `","expected_revision":0,"slot":10,"prospect_id":""}`)
	if v, err := ParseStableResponse(raw); err != nil || v.Error != "outcome_unknown" || controller.prepared.Slot != 10 {
		t.Fatal("clear request dropped required fields", err)
	}
	raw = send(`{"id":5,"command":"stable_snapshot","after":0}`)
	if v, err := ParseStableResponse(raw); err != nil || v.Error != "invalid_request" {
		t.Fatal("unknown field reached controller", err)
	}
	_ = client.Close()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	_ = responses.Close()
	if controller.stopped.Load() == 0 || controller.closed.Load() != 1 {
		t.Fatal("real controller drain not joined")
	}
	if stableProtocolError(errors.New("private backend failure")) != "stable_unavailable" {
		t.Fatal("backend details leaked")
	}
}

func (c *stableProtocolController) BeginStableRecent(context.Context, string) (StableJob, error) {
	return StableJob{}, c.failure
}
