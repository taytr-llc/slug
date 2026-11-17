// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymseed"
)

type seedReference struct {
	Pin struct {
		Value   gymseed.Pin `json:"value"`
		StoreID string      `json:"store_id"`
	} `json:"pin"`
	DTOs []struct {
		Kind, Name, Canonical string
		Value                 json.RawMessage
	} `json:"dtos"`
	Cases []struct {
		Name     string                 `json:"name"`
		Request  pilot.SeedRequest      `json:"request"`
		Response json.RawMessage        `json:"response"`
		View     *pilot.ManifestViewPin `json:"current_view"`
		Expected *pilot.SeedJob         `json:"expected_job"`
		Digest   string                 `json:"terminal_digest"`
		Created  *int64                 `json:"expected_created_at_ms"`
		Accepted bool                   `json:"accepted"`
	} `json:"protocol_cases"`
}

func seedRef(t *testing.T) seedReference {
	t.Helper()
	raw, e := os.ReadFile("../../../gymseed/testdata/reference/seed_v1.json")
	if e != nil {
		t.Fatal(e)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "2b8d78e8ef665e372543eb87bb5d25a975026a0ae946b15a871ac97bbf2153f0" {
		t.Fatal("frozen independent seed corpus changed")
	}
	var r seedReference
	if e = json.Unmarshal(raw, &r); e != nil {
		t.Fatal(e)
	}
	return r
}
func TestSeedBridgeIndependentProtocolCorpus(t *testing.T) {
	ref, old := seedRef(t), eventRef(t)
	if len(ref.Cases) != 194 {
		t.Fatal("incomplete frozen protocol corpus")
	}
	for _, c := range ref.Cases {
		t.Run(c.Name, func(t *testing.T) {
			fixture := strings.Split(c.Name, "/")[0]
			if fixture != "zero256" && fixture != "incrementing256" {
				fixture = "zero256"
			}
			b, _, _ := old.setupBridge(t, fixture)
			b.seedsEnabled = true
			var s pilot.SeedSnapshot
			for _, d := range ref.DTOs {
				if d.Kind == "SeedSnapshot" && d.Name == fixture+"/ready" {
					if e := json.Unmarshal(d.Value, &s); e != nil {
						t.Fatal(e)
					}
				}
			}
			if s.CreatedAtMS == 0 {
				t.Fatal("missing reference seed snapshot")
			}
			a := seedAuthority{pin: ref.Pin.Value, storeID: ref.Pin.StoreID, createdMS: s.CreatedAtMS}
			if c.Created != nil {
				a.createdMS = *c.Created
			}
			v := *b.view
			if c.View != nil {
				v = *c.View
			}
			b.view, b.manifestView = &v, &v
			b.entry.view, b.event.view = v, v
			b.seedPin = &a
			b.seed = &seedCorrelation{view: v, jobs: map[string]seedJobExpectation{}, snapshotJobs: map[string]bool{}}
			_, requestErr := pilot.MarshalSeedRequest(c.Request)
			r, e := pilot.ParseSeedResponse(c.Response)
			accepted := requestErr == nil && e == nil && r.ID == c.Request.ID
			if accepted && r.OK {
				if c.Request.Command == "seed_snapshot" {
					if r.Snapshot != nil && r.Snapshot.State == "setup_incomplete" {
						b.seedPin = nil
					}
					accepted = b.rememberSeedSnapshot(r)
				} else {
					accepted = b.correlateSeedEnvelope(c.Request, r, a, v)
					if accepted && r.Job != nil {
						var want seedJobExpectation
						if c.Expected != nil {
							j := c.Expected
							want = seedJobExpectation{eventKey: j.EventKey, saturday: j.Saturday, kind: j.Kind, state: j.State, terminalDigest: c.Digest}
							if j.JobID != "" && j.JobID != r.Job.JobID {
								accepted = false
							}
							if j.View != (pilot.ManifestViewPin{}) && j.View != r.Job.View {
								accepted = false
							}
							if j.StoreID != "" && j.StoreID != r.Job.StoreID {
								accepted = false
							}
						} else {
							event, err := gymevent.EventForDate(a.pin.SourcePin, c.Request.Saturday)
							if err != nil {
								accepted = false
							} else {
								key, err := gymseed.EventKey(a.pin, event)
								accepted = accepted && err == nil
								want = seedJobExpectation{eventKey: key, saturday: c.Request.Saturday, kind: strings.TrimPrefix(c.Request.Command, "seed_")}
							}
						}
						if want.kind == "" {
							want.kind = r.Job.Kind
						}
						if c.Digest != "" && !eventTerminal(want.state) {
							for _, d := range ref.DTOs {
								h := sha256.Sum256([]byte(d.Canonical))
								if d.Kind == "SeedJob" && hex.EncodeToString(h[:]) == c.Digest {
									var j pilot.SeedJob
									if json.Unmarshal(d.Value, &j) != nil {
										t.Fatal("prior terminal DTO")
									}
									want.state = j.State
								}
							}
							if !eventTerminal(want.state) {
								t.Fatal("missing frozen terminal digest")
							}
						}
						accepted = accepted && b.correlateSeedJob(c.Request, *r.Job, want, a, v)
					}
				}
			}
			if accepted != c.Accepted {
				t.Fatalf("accepted=%v want=%v parse=%v request=%v", accepted, c.Accepted, e, requestErr)
			}
		})
	}
}

func (r seedReference) job(t *testing.T, state string) pilot.SeedJob {
	t.Helper()
	for _, d := range r.DTOs {
		if d.Kind == "SeedJob" && d.Name == "zero256/prepare/"+state {
			var j pilot.SeedJob
			if json.Unmarshal(d.Value, &j) != nil {
				t.Fatal("reference job")
			}
			return j
		}
	}
	t.Fatal("missing reference job", state)
	return pilot.SeedJob{}
}

// This child moves frozen public bytes over real helper pipes. It establishes
// transport/correlation coverage, not source journal or node execution.
func TestSeedBridgeReferencePipeProcess(t *testing.T) {
	mode := os.Getenv("SLUG_SEED_BRIDGE_PROCESS")
	if mode == "" {
		return
	}
	ref := seedRef(t)
	queued, complete := ref.job(t, "queued"), ref.job(t, "completed/")
	scan := bufio.NewScanner(os.Stdin)
	count := 0
	for scan.Scan() {
		var q pilot.SeedRequest
		if json.Unmarshal(scan.Bytes(), &q) != nil {
			os.Exit(2)
		}
		j := queued
		if q.Command == "seed_prepare" && mode == "drop" {
			os.Exit(0)
		}
		if q.Command == "seed_job" {
			count++
			j = complete
			if mode == "wrong-view" {
				j.View.Generation++
			}
			if mode == "changed-terminal" && count > 1 {
				j.Result.Observation.ObservedAtMS++
				j.Result.Observation.ChainObservedAtMS++
			}
		}
		response := pilot.SeedResponse{ID: q.ID, OK: true, View: &j.View, StoreID: j.StoreID, Job: &j}
		if mode == "wrong-family" && q.Command == "seed_job" {
			raw, _ := json.Marshal(pilot.EventResponse{ID: q.ID, Error: "event_unavailable"})
			fmt.Println(string(raw))
			continue
		}
		raw, e := pilot.MarshalSeedResponse(response)
		if e != nil {
			os.Exit(3)
		}
		fmt.Println(string(raw))
	}
	os.Exit(0)
}
func TestSeedBridgePipesBindViewAndTerminalBytes(t *testing.T) {
	ref, old := seedRef(t), eventRef(t)
	queued := ref.job(t, "queued")
	for _, mode := range []string{"good", "drop", "wrong-view", "wrong-family", "changed-terminal"} {
		t.Run(mode, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestSeedBridgeReferencePipeProcess$")
			cmd.Env = append(os.Environ(), "SLUG_SEED_BRIDGE_PROCESS="+mode)
			b, e := startStable(cmd)
			if e != nil {
				t.Fatal(e)
			}
			t.Cleanup(func() { _ = b.Close() })
			baseline, _, _ := old.setupBridge(t, "zero256")
			v := *baseline.view
			var created int64
			for _, d := range ref.DTOs {
				if d.Kind == "SeedSnapshot" && d.Name == "zero256/ready" {
					var s pilot.SeedSnapshot
					_ = json.Unmarshal(d.Value, &s)
					created = s.CreatedAtMS
				}
			}
			b.mu.Lock()
			b.seedsEnabled = true
			b.view = &v
			b.seedPin = &seedAuthority{pin: ref.Pin.Value, storeID: ref.Pin.StoreID, createdMS: created}
			b.seed = &seedCorrelation{view: v, jobs: map[string]seedJobExpectation{}, snapshotJobs: map[string]bool{}}
			b.mu.Unlock()
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			r := b.SeedRequest(ctx, pilot.SeedRequest{Command: "seed_prepare", ViewID: v.ViewID, Saturday: queued.Saturday})
			if mode == "drop" {
				if r.OK || r.Error != "outcome_unknown" {
					t.Fatal("lost ACK was not uncertain", r.Error)
				}
				return
			}
			if !r.OK || r.Job == nil || r.Job.State != "queued" {
				t.Fatal("Begin ACK", r.Error)
			}
			q := pilot.SeedRequest{Command: "seed_job", ViewID: v.ViewID, JobID: r.Job.JobID}
			r = b.SeedRequest(ctx, q)
			if mode == "wrong-view" || mode == "wrong-family" {
				if r.OK || r.Error != "outcome_unknown" {
					t.Fatal("foreign frame accepted", r.Error)
				}
				return
			}
			if !r.OK || r.Job == nil || r.Job.State != "completed" {
				t.Fatal("completed job", r.Error)
			}
			again := b.SeedRequest(ctx, q)
			if mode == "changed-terminal" {
				if again.OK || again.Error != "outcome_unknown" {
					t.Fatal("changed terminal accepted", again.Error)
				}
			} else if !again.OK || again.Job == nil || seedJobDigest(*again.Job) != seedJobDigest(*r.Job) {
				t.Fatal("terminal replay changed")
			}
		})
	}
}
