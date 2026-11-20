// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type poolCycleWireController struct {
	PoolControllerAPI
	calls []string
}

func (*poolCycleWireController) Stop()        {}
func (*poolCycleWireController) Close() error { return nil }
func (c *poolCycleWireController) SeedSnapshot() SeedSnapshot {
	c.calls = append(c.calls, "seed")
	return SeedSnapshot{Version: 1, State: "unavailable", Error: "seed_unavailable", NodeState: "offline"}
}
func (c *poolCycleWireController) CardSnapshot() CardSnapshot {
	c.calls = append(c.calls, "card")
	return CardSnapshot{Version: 1, State: "unavailable", Error: "card_unavailable"}
}
func (c *poolCycleWireController) BoutSnapshot() BoutSnapshot {
	c.calls = append(c.calls, "bout")
	return BoutSnapshot{Version: 1, State: "unavailable", Error: "bout_unavailable"}
}
func (c *poolCycleWireController) ResultSnapshot() ResultSnapshot {
	c.calls = append(c.calls, "result")
	return ResultSnapshot{Version: 1, State: "unavailable", Error: "result_unavailable", NodeState: "offline", NodeOwner: "none"}
}
func (c *poolCycleWireController) CareerHistorySnapshot(context.Context) (CareerHistorySnapshot, error) {
	c.calls = append(c.calls, "history")
	return CareerHistorySnapshot{Version: 1, State: "unavailable", Error: "career_inactive"}, nil
}

func TestPoolCycleClosedWireFamilies(t *testing.T) {
	seed, _ := MarshalSeedRequest(SeedRequest{ID: 1, Command: "seed_snapshot"})
	card, _ := MarshalCardRequest(CardRequest{ID: 2, Command: "card_snapshot"})
	bout, _ := MarshalBoutRequest(BoutRequest{ID: 3, Command: "bout_snapshot"})
	result, _ := MarshalResultRequest(ResultRequest{ID: 4, Command: "result_snapshot"})
	history, _ := MarshalCareerHistoryRequest(CareerHistoryRequest{ID: 5, Version: 1, Command: "career_history_snapshot"})
	for _, v := range []struct {
		name  string
		raw   []byte
		parse func([]byte) error
	}{
		{"seed", seed, func(b []byte) error { _, e := ParseSeedResponse(b); return e }},
		{"card", card, func(b []byte) error { _, e := ParseCardResponse(b); return e }},
		{"bout", bout, func(b []byte) error { _, e := ParseBoutResponse(b); return e }},
		{"result", result, func(b []byte) error { _, e := ParseResultResponse(b); return e }},
		{"history", history, func(b []byte) error { _, e := ParseCareerHistoryResponse(b); return e }},
	} {
		t.Run(v.name, func(t *testing.T) {
			c := &poolCycleWireController{}
			var output bytes.Buffer
			if err := ServePool(t.Context(), bytes.NewReader(append(v.raw, '\n')), &output, c); err != nil {
				t.Fatal(err)
			}
			raw := bytes.TrimSpace(output.Bytes())
			if err := v.parse(raw); err != nil || len(c.calls) != 1 || c.calls[0] != v.name {
				t.Fatal("wrong typed route", err, string(raw), c.calls)
			}
			if _, err := ParsePoolResponse(raw); err == nil {
				t.Fatal("cycle reply accepted as pool union")
			}
			for _, bad := range [][]byte{append([]byte(`{"unexpected":true,`), v.raw[1:]...), append([]byte(`{"id":99,`), v.raw[1:]...)} {
				c = &poolCycleWireController{}
				output.Reset()
				if err := ServePool(t.Context(), bytes.NewReader(append(bad, '\n')), &output, c); err != nil {
					t.Fatal(err)
				}
				var got struct {
					OK    bool
					Error string
				}
				if json.Unmarshal(output.Bytes(), &got) != nil || got.OK || got.Error != "invalid_request" || len(c.calls) != 0 {
					t.Fatal("invalid route reached controller")
				}
			}
		})
	}
	// Practice and caller-selected nodes/finality do not enter the pool surface.
	for _, command := range []string{"career_preview_snapshot", "result_connect", "seed_set_finalized"} {
		raw, _ := json.Marshal(map[string]any{"id": 1, "command": command, "node": "http://127.0.0.1:1"})
		c := &poolCycleWireController{}
		var output bytes.Buffer
		if err := ServePool(t.Context(), bytes.NewReader(append(raw, '\n')), &output, c); err != nil {
			t.Fatal(err)
		}
		if len(c.calls) != 0 || !strings.Contains(output.String(), `"error":"invalid_request"`) {
			t.Fatal("unsupported control dispatched")
		}
	}
}

type poolCycleLostResultView struct{ ResultSurfaceAPI }

func (poolCycleLostResultView) ResultSnapshot() ResultSnapshot { return ResultSnapshot{} }
func (poolCycleLostResultView) BeginResultPublish(context.Context, string, string, string) (ResultJob, error) {
	return ResultJob{JobID: "possibly-committed"}, nil
}
func TestPoolCycleResultLostViewPreservesUncertainty(t *testing.T) {
	r := handleResultRequest(t.Context(), ResultRequest{ID: 1, Command: "result_publish", ViewID: "original"}, poolCycleLostResultView{})
	if r.OK || r.Error != "outcome_unknown" || r.Job != nil || r.View != nil {
		t.Fatal("old view disclosed result or lost uncertainty")
	}
}
