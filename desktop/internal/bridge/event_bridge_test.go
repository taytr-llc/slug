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
	"testing"
	"time"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/wire/v2"
)

type eventReference struct {
	Pin struct {
		Value   gymevent.Pin `json:"value"`
		StoreID string       `json:"store_id"`
	} `json:"pin"`
	Fixtures []struct {
		Fixture       string                `json:"fixture"`
		EntryPin      gymentry.Pin          `json:"entry_pin"`
		View          pilot.ManifestViewPin `json:"current_view"`
		ParticipantID string                `json:"participant_id"`
	} `json:"fixtures"`
	DTOs []struct {
		Name, Kind, Fixture, Canonical string
		Value                          json.RawMessage
	} `json:"dtos"`
	Cases []struct {
		Name     string                `json:"name"`
		Fixture  string                `json:"fixture"`
		Request  pilot.EventRequest    `json:"request"`
		Response json.RawMessage       `json:"response"`
		View     pilot.ManifestViewPin `json:"current_view"`
		Expected *pilot.EventJob       `json:"expected_job"`
		Digest   string                `json:"terminal_digest"`
		Accepted bool                  `json:"accepted"`
	} `json:"protocol_cases"`
}

func eventRef(t *testing.T) eventReference {
	t.Helper()
	if wire.TestPoW {
		t.Skip("frozen candidate authority uses the normal PoW profile")
	}
	raw, err := os.ReadFile("../../../gymevent/testdata/reference/event_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "9287f92c0cb46c585b44274c70ae57c5d6c9a4234e7078fd1d69a200644f996a" {
		t.Fatal("independently frozen event corpus changed")
	}
	var ref eventReference
	if err := json.Unmarshal(raw, &ref); err != nil {
		t.Fatal(err)
	}
	return ref
}

func (r eventReference) snapshot(t *testing.T, fixture string) pilot.EventSnapshot {
	t.Helper()
	for _, d := range r.DTOs {
		if d.Kind == "EventSnapshot" && d.Fixture == fixture {
			var s pilot.EventSnapshot
			if json.Unmarshal(d.Value, &s) == nil && s.State == "ready" {
				return s
			}
		}
	}
	t.Fatal("missing independent ready snapshot", fixture)
	return pilot.EventSnapshot{}
}
func (r eventReference) job(t *testing.T, fixture, state string) pilot.EventJob {
	t.Helper()
	for _, d := range r.DTOs {
		if d.Kind == "EventJob" && d.Fixture == fixture {
			var j pilot.EventJob
			if json.Unmarshal(d.Value, &j) == nil && j.State == state {
				return j
			}
		}
	}
	t.Fatal("missing independent job", fixture, state)
	return pilot.EventJob{}
}
func (r eventReference) setupBridge(t *testing.T, fixture string) (*StableBridge, eventAuthority, string) {
	t.Helper()
	s := r.snapshot(t, fixture)
	for _, f := range r.Fixtures {
		if f.Fixture != fixture {
			continue
		}
		entryStore, err := gymentry.ID(f.EntryPin)
		if err != nil {
			t.Fatal(err)
		}
		a := eventAuthority{pin: r.Pin.Value, storeID: r.Pin.StoreID, createdMS: s.CoordinatorCreatedAtMS}
		v, p := f.View, f.EntryPin.StablePin
		b := &StableBridge{view: &v, manifestView: &v, stablePin: &p, storeID: f.EntryPin.StableStoreID,
			entriesEnabled: true, eventsEnabled: true, previewReady: true, eventPin: &a,
			entry: &entryCorrelation{view: v, pin: f.EntryPin, storeID: entryStore},
			event: &eventCorrelation{view: v, participantID: f.ParticipantID, jobs: make(map[string]eventJobExpectation), snapshotJobs: make(map[string]bool)}}
		return b, a, f.ParticipantID
	}
	t.Fatal("missing independent fixture", fixture)
	return nil, eventAuthority{}, ""
}

func TestEventBridgeIndependentProtocolCorpus(t *testing.T) {
	ref := eventRef(t)
	if len(ref.Cases) < 20 {
		t.Fatal("insufficient independent protocol coverage")
	}
	for _, c := range ref.Cases {
		t.Run(c.Name, func(t *testing.T) {
			b, a, participant := ref.setupBridge(t, c.Fixture)
			v := c.View
			b.view, b.manifestView = &v, &v
			b.entry.view, b.event.view = v, v
			r, err := pilot.ParseEventResponse(c.Response)
			accepted := err == nil && r.ID == c.Request.ID
			if accepted && r.OK {
				if c.Request.Command == "event_snapshot" {
					// setup-incomplete is valid only before first coordinator.
					if r.Snapshot != nil && r.Snapshot.State == "setup_incomplete" {
						b.eventPin = nil
					}
					accepted = b.rememberEventSnapshot(r)
				} else {
					accepted = b.correlateEventEnvelope(c.Request, r, a, participant)
					if accepted && r.Job != nil {
						var e eventJobExpectation
						if c.Expected != nil {
							j := c.Expected
							e = eventJobExpectation{eventID: j.EventID, saturday: j.Saturday, participantID: j.ParticipantID, state: j.State, terminalDigest: c.Digest}
							// The oracle supplies identity expectations and a prior terminal
							// digest separately. Recover its terminal state from the frozen DTO.
							if c.Digest != "" && !eventTerminal(e.state) {
								for _, d := range ref.DTOs {
									h := sha256.Sum256([]byte(d.Canonical))
									if d.Kind == "EventJob" && hex.EncodeToString(h[:]) == c.Digest {
										var prior pilot.EventJob
										if err := json.Unmarshal(d.Value, &prior); err != nil {
											t.Fatal(err)
										}
										e.state = prior.State
									}
								}
								if !eventTerminal(e.state) {
									t.Fatal("missing independent prior terminal DTO")
								}
							}
						} else {
							event, err := gymevent.EventForDate(a.pin, c.Request.Saturday)
							if err != nil {
								accepted = false
							} else {
								e = eventJobExpectation{eventID: event.EventID, saturday: event.Saturday, participantID: participant}
							}
						}
						accepted = accepted && b.correlateEventJob(c.Request, *r.Job, e, a, participant)
					}
				}
			}
			if accepted != c.Accepted {
				t.Fatalf("accepted=%v want=%v parse=%v", accepted, c.Accepted, err)
			}
		})
	}
}

// Real helper pipes carrying frozen independent public frames. These test
// transport/correlation only, not source authenticity or cutoff execution.
func TestEventBridgeProcess(t *testing.T) {
	mode := os.Getenv("SLUG_EVENT_BRIDGE_PROCESS")
	if mode == "" {
		return
	}
	ref := eventRef(t)
	snapshot := ref.snapshot(t, "zero256")
	queued, complete := ref.job(t, "zero256", "queued"), ref.job(t, "zero256", "completed")
	b, a, _ := ref.setupBridge(t, "zero256")
	view := *b.view
	entryStore := b.entry.storeID
	scan := bufio.NewScanner(os.Stdin)
	var previousID uint64
	jobReads := 0
	for scan.Scan() {
		var req pilot.EventRequest
		if json.Unmarshal(scan.Bytes(), &req) != nil || req.ID <= previousID {
			os.Exit(2)
		}
		previousID = req.ID
		var reply any
		r := pilot.EventResponse{ID: req.ID, OK: true, View: &view, StoreID: a.storeID}
		switch req.Command {
		case "snapshot":
			s := manifestBridgeSnapshot(t)
			s.View, s.Account.ID, s.Account.Generation = &view, view.AccountID, view.Generation
			s.GymID, s.GymPublicKey = b.stablePin.Identity.GymID, b.stablePin.Identity.GymPublicKey
			reply = pilot.ManifestResponse{ID: req.ID, OK: true, Snapshot: &s}
		case "stable_snapshot":
			s := pilot.StableSnapshot{Version: 1, State: "ready", View: &view, Status: &gymstable.Status{Version: 1, StoreID: b.storeID, Pin: *b.stablePin, CaughtUp: true}}
			reply = pilot.StableResponse{ID: req.ID, OK: true, Snapshot: &s}
		case "entry_snapshot":
			s := pilot.EntrySnapshot{Version: 1, State: "idle", View: &view, StoreID: entryStore}
			reply = pilot.EntryResponse{ID: req.ID, OK: true, View: &view, StoreID: entryStore, Snapshot: &s}
		case "event_snapshot":
			r.Snapshot = &snapshot
		case "event_capture":
			if mode == "drop" {
				os.Exit(0)
			}
			r.Job = &queued
			snapshot.LastJobID = complete.JobID
		case "event_job", "event_cancel":
			jobReads++
			j := complete
			if mode == "wrong-view" {
				j.View.Generation++
			}
			if mode == "changed-terminal" && jobReads > 1 {
				v := *complete.Result
				v.Status.ObservedAtMS++
				j.Result = &v
			}
			if mode == "wrong-family" {
				reply = pilot.EntryResponse{ID: req.ID, Error: "entry_not_found"}
			} else {
				r.Job = &j
			}
		default:
			r = eventFailure(req.ID, "invalid_request")
		}
		if reply == nil {
			reply = r
		}
		raw, err := json.Marshal(reply)
		if err != nil {
			os.Exit(3)
		}
		fmt.Println(string(raw))
	}
	os.Exit(0)
}

func TestEventBridgePipesBindOriginalViewAndTerminalBytes(t *testing.T) {
	ref := eventRef(t)
	job := ref.job(t, "zero256", "queued")
	for _, mode := range []string{"good", "drop", "wrong-view", "wrong-family", "changed-terminal"} {
		t.Run(mode, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestEventBridgeProcess$")
			cmd.Env = append(os.Environ(), "SLUG_EVENT_BRIDGE_PROCESS="+mode)
			b, err := startStable(cmd)
			if err != nil {
				t.Fatal(err)
			}
			b.eventsEnabled, b.entriesEnabled = true, true
			t.Cleanup(func() { _ = b.Close() })
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			s := b.EventRequest(ctx, pilot.EventRequest{Command: "event_snapshot"})
			if !s.OK || s.Snapshot == nil || s.Snapshot.State != "ready" {
				t.Fatalf("snapshot failed: %s", s.Error)
			}
			q := b.EventRequest(ctx, pilot.EventRequest{Command: "event_capture", ViewID: job.View.ViewID, Saturday: job.Saturday})
			if mode == "drop" {
				if q.OK || q.Error != "outcome_unknown" {
					t.Fatal("lost capture ACK was not uncertain", q.Error)
				}
				return
			}
			if !q.OK || q.Job == nil || q.Job.State != "queued" {
				t.Fatal("capture ACK failed", q.Error)
			}
			q = b.EventRequest(ctx, pilot.EventRequest{Command: "event_job", ViewID: job.View.ViewID, JobID: q.Job.JobID})
			if oneOf(mode, "wrong-view", "wrong-family") {
				if q.OK || q.Error != "outcome_unknown" {
					t.Fatal("foreign result was not refused as uncertain")
				}
				return
			}
			if !q.OK || q.Job == nil || q.Job.State != "completed" {
				t.Fatal("completed capture failed", q.Error)
			}
			again := b.EventRequest(ctx, pilot.EventRequest{Command: "event_job", ViewID: job.View.ViewID, JobID: q.Job.JobID})
			if mode == "changed-terminal" {
				if again.OK || again.Error != "outcome_unknown" {
					t.Fatal("rewritten terminal result was accepted")
				}
			} else if !again.OK || again.Job == nil || eventJobDigest(*again.Job) != eventJobDigest(*q.Job) {
				t.Fatal("original terminal capture did not replay")
			}
		})
	}
}
