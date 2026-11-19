// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Only the two snapshot methods are implemented. Any accidental routing to a
// different operation fails the test instead of supplying a permissive mock.
type poolLeagueWireController struct {
	PoolControllerAPI
	entries, events atomic.Int32
}

func (*poolLeagueWireController) Stop()        {}
func (*poolLeagueWireController) Close() error { return nil }
func (c *poolLeagueWireController) EntrySnapshot() EntrySnapshot {
	c.entries.Add(1)
	return EntrySnapshot{Version: 1, State: "unavailable", Error: "entry_unavailable"}
}
func (c *poolLeagueWireController) EventSnapshot() EventSnapshot {
	c.events.Add(1)
	return EventSnapshot{Version: 1, State: "unavailable", Error: "event_unavailable"}
}

func poolLeagueWire(t *testing.T, raw []byte) (*poolLeagueWireController, []byte, error) {
	t.Helper()
	c := &poolLeagueWireController{}
	if _, ok := any(c).(PreviewControllerAPI); ok {
		t.Fatal("pool transport acquired unrelated preview capabilities")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	var output bytes.Buffer
	err := ServePool(ctx, bytes.NewReader(append(raw, '\n')), &output, c)
	return c, bytes.TrimSuffix(output.Bytes(), []byte{'\n'}), err
}

func TestPoolLeagueClosedWireFamilies(t *testing.T) {
	entry, err := MarshalEntryRequest(EntryRequest{ID: 7, Command: "entry_snapshot"})
	if err != nil {
		t.Fatal(err)
	}
	event, err := MarshalEventRequest(EventRequest{ID: 9, Command: "event_snapshot"})
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []struct {
		name string
		raw  []byte
		id   uint64
	}{
		{"entry", entry, 7}, {"event", event, 9},
		{"entry-large-envelope", append(bytes.Repeat([]byte{' '}, 4096), entry...), 7},
		{"event-large-envelope", append(bytes.Repeat([]byte{' '}, 4096), event...), 9},
	} {
		t.Run(value.name, func(t *testing.T) {
			c, raw, err := poolLeagueWire(t, value.raw)
			if err != nil {
				t.Fatal(err)
			}
			if strings.HasPrefix(value.name, "entry") {
				r, e := ParseEntryResponse(raw)
				if e != nil || !r.OK || r.ID != value.id || r.Snapshot == nil || r.Snapshot.State != "unavailable" || c.entries.Load() != 1 || c.events.Load() != 0 {
					t.Fatal("entry family routing", e, string(raw))
				}
			} else {
				r, e := ParseEventResponse(raw)
				if e != nil || !r.OK || r.ID != value.id || r.Snapshot == nil || r.Snapshot.State != "unavailable" || c.events.Load() != 1 || c.entries.Load() != 0 {
					t.Fatal("event family routing", e, string(raw))
				}
			}
			if _, err := ParsePoolResponse(raw); err == nil {
				t.Fatal("closed league reply became a pool snapshot union")
			}
		})
	}
}

func TestPoolLeagueWireRejectsBeforeDispatch(t *testing.T) {
	entry, _ := MarshalEntryRequest(EntryRequest{ID: 7, Command: "entry_snapshot"})
	event, _ := MarshalEventRequest(EventRequest{ID: 9, Command: "event_snapshot"})
	for _, good := range [][]byte{entry, event} {
		for name, raw := range map[string][]byte{
			"duplicate":        append([]byte(`{"id":123,`), good[1:]...),
			"unknown":          append([]byte(`{"unexpected":true,`), good[1:]...),
			"missing":          bytes.Replace(good, []byte(`"view_id":"",`), nil, 1),
			"foreign-field":    append([]byte(`{"fixture":"zero256",`), good[1:]...),
			"family-confusion": bytes.Replace(good, []byte(`"entry_snapshot"`), []byte(`"event_snapshot"`), 1),
		} {
			if bytes.Equal(raw, good) {
				continue
			}
			t.Run(name+"/"+string(good), func(t *testing.T) {
				c, output, err := poolLeagueWire(t, raw)
				if err != nil || c.entries.Load() != 0 || c.events.Load() != 0 {
					t.Fatal("invalid family was dispatched", err)
				}
				var response struct {
					OK    bool   `json:"ok"`
					Error string `json:"error"`
				}
				if json.Unmarshal(output, &response) != nil || response.OK || response.Error != "invalid_request" {
					t.Fatal("invalid family reply", string(output))
				}
			})
		}
	}
	for _, raw := range [][]byte{
		bytes.Replace(entry, []byte(`"id":7`), []byte(`"id":0`), 1),
		bytes.Replace(event, []byte(`"id":9`), []byte(`"id":9007199254740992`), 1),
		append(bytes.Repeat([]byte{' '}, MaxEventRequestSize), event...),
	} {
		c, output, err := poolLeagueWire(t, raw)
		if err == nil || len(output) != 0 || c.entries.Load() != 0 || c.events.Load() != 0 {
			t.Fatal("invalid envelope was handled", err, string(output))
		}
	}
	for _, command := range []string{"preview_snapshot"} {
		raw, _ := json.Marshal(PoolRequest{ID: 1, Command: command})
		c, output, err := poolLeagueWire(t, raw)
		r, parseErr := ParsePoolResponse(output)
		if err != nil || parseErr != nil || r.OK || r.Error != "invalid_request" || c.entries.Load() != 0 || c.events.Load() != 0 {
			t.Fatal("unsupported family enabled", command, err, parseErr)
		}
	}
	pool, _ := json.Marshal(PoolRequest{ID: 1, Command: "snapshot"})
	c, output, err := poolLeagueWire(t, append(bytes.Repeat([]byte{' '}, MaxRequestSize), pool...))
	if err == nil || len(output) != 0 || c.entries.Load() != 0 || c.events.Load() != 0 {
		t.Fatal("larger envelope weakened pool parser's own limit", err, string(output))
	}
}

type lostPoolEntryView struct{ EntrySurfaceAPI }

func (lostPoolEntryView) EntrySnapshot() EntrySnapshot { return EntrySnapshot{} }
func (lostPoolEntryView) BeginEntryApply(context.Context, string, string) (EntryJob, error) {
	return EntryJob{JobID: "not-publishable"}, nil
}

type lostPoolEventView struct{ EventSurfaceAPI }

func (lostPoolEventView) EventSnapshot() EventSnapshot { return EventSnapshot{} }
func (lostPoolEventView) BeginEventCapture(context.Context, string, string) (EventJob, error) {
	return EventJob{JobID: "not-publishable"}, nil
}

func TestPoolLeagueNarrowMutationLostViewPreservesUncertainty(t *testing.T) {
	e := handleEntryRequest(t.Context(), EntryRequest{ID: 1, Command: "entry_apply", ViewID: "original"}, lostPoolEntryView{})
	v := handleEventRequest(t.Context(), EventRequest{ID: 2, Command: "event_capture", ViewID: "original"}, lostPoolEventView{})
	if e.OK || e.Error != "outcome_unknown" || e.Job != nil || e.View != nil || v.OK || v.Error != "outcome_unknown" || v.Job != nil || v.View != nil {
		t.Fatal("lost original view disclosed or denied a possibly committed mutation")
	}
}
