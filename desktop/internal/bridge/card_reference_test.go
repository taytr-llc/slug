package bridge

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/wire/v2"
	"os"
	"os/exec"
	"testing"
	"time"
)

type cardReference struct {
	Pin struct {
		Value   gymcard.Pin
		StoreID string `json:"store_id"`
	}
	DTOs []struct {
		Kind, Name, Canonical, Context string
		Value                          json.RawMessage
	}
	Cases []struct {
		Name, Fixture string
		View          pilot.ManifestViewPin `json:"current_view"`
		Pin           gymcard.Pin           `json:"authority_pin"`
		SourceCreated int64                 `json:"source_created_at_ms"`
		Created       int64                 `json:"created_at_ms"`
		Request       pilot.CardRequest
		Response      json.RawMessage
		Expected      *pilot.CardJob `json:"expected_job"`
		Digest        string         `json:"terminal_digest"`
		Accept        bool
	} `json:"protocol_cases"`
}

func cardRef(t *testing.T) cardReference {
	t.Helper()
	raw, e := os.ReadFile("../../../gymcard/testdata/reference/card_v1.json")
	if e != nil {
		t.Fatal(e)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "c52f513f8dfda34c884312c6a2caaf93b429bbf788aaf7c2cba3fbd901329790" {
		t.Fatal("frozen Card corpus changed")
	}
	if wire.TestPoW {
		t.Skip("independent Card fixture is pinned to the normal profile")
	}
	var r cardReference
	if e = json.Unmarshal(raw, &r); e != nil {
		t.Fatal(e)
	}
	return r
}
func (r cardReference) job(t *testing.T, state string) pilot.CardJob {
	t.Helper()
	for _, d := range r.DTOs {
		if d.Kind == "CardJob" && d.Name == "zero256/"+state {
			var j pilot.CardJob
			if json.Unmarshal(d.Value, &j) != nil {
				t.Fatal("job")
			}
			return j
		}
	}
	t.Fatal("missing independent job", state)
	return pilot.CardJob{}
}
func TestCardBridgeIndependentProtocolCorpus(t *testing.T) {
	r := cardRef(t)
	old := eventRef(t)
	if len(r.Cases) != 108 {
		t.Fatal("protocol count")
	}
	for _, c := range r.Cases {
		t.Run(c.Name, func(t *testing.T) {
			b, _, _ := old.setupBridge(t, c.Fixture)
			v := c.View
			id, e := gymcard.ID(c.Pin)
			if e != nil {
				t.Fatal(e)
			}
			a := cardAuthority{pin: c.Pin, storeID: id, createdMS: c.Created, sourceCreatedMS: c.SourceCreated}
			b.view, b.manifestView = &v, &v
			b.entry.view, b.event.view = v, v
			b.seedsEnabled, b.cardsEnabled = true, true
			b.seedPin = &seedAuthority{pin: c.Pin.SourcePin, storeID: c.Pin.SourceStoreID, createdMS: c.SourceCreated}
			b.seed = &seedCorrelation{view: v, jobs: map[string]seedJobExpectation{}, snapshotJobs: map[string]bool{}}
			b.cardPin = &a
			b.card = &cardCorrelation{view: v, jobs: map[string]cardJobExpectation{}, snapshotJobs: map[string]bool{}}
			_, qerr := pilot.MarshalCardRequest(c.Request)
			out, err := pilot.ParseCardResponse(c.Response)
			accept := qerr == nil && err == nil && out.ID == c.Request.ID
			if accept && out.OK {
				if c.Request.Command == "card_snapshot" {
					if out.Snapshot != nil && out.Snapshot.State == "setup_incomplete" {
						b.cardPin = nil
					}
					accept = b.rememberCardSnapshot(out)
				} else {
					accept = b.correlateCardEnvelope(c.Request, out, a, v)
					if accept && out.Job != nil {
						var want cardJobExpectation
						if c.Expected != nil {
							j := c.Expected
							want = cardJobExpectation{cardID: j.CardID, saturday: j.Saturday, kind: j.Kind, state: j.State, terminalDigest: c.Digest}
							if j.JobID != "" && j.JobID != out.Job.JobID {
								accept = false
							}
							if j.View != (pilot.ManifestViewPin{}) && j.View != out.Job.View {
								accept = false
							}
							if j.StoreID != "" && j.StoreID != out.Job.StoreID {
								accept = false
							}
						} else if c.Request.Command == "card_job" || c.Request.Command == "card_cancel" {
							// No expected tuple models learning the date/kind from this
							// exact same-view Snapshot-discovered job ID.
							j := out.Job
							want = cardJobExpectation{cardID: j.CardID, saturday: j.Saturday, kind: j.Kind}
						} else {
							ev, e := gymevent.EventForDate(a.pin.SourcePin.SourcePin, c.Request.Saturday)
							if e != nil {
								accept = false
							} else {
								key, e := gymcard.CardID(a.pin, ev)
								accept = accept && e == nil
								want = cardJobExpectation{cardID: key, saturday: c.Request.Saturday, kind: "build"}
							}
						}
						accept = accept && b.correlateCardJob(c.Request, *out.Job, want, a, v)
					}
				}
			}
			if accept != c.Accept {
				t.Fatalf("accepted=%v want=%v parse=%v request=%v", accept, c.Accept, err, qerr)
			}
		})
	}
}

// Only the separately frozen reference values cross this real child pipe.
// This is transport coverage, not journal or owned-chain execution.
func TestCardBridgeReferencePipeProcess(t *testing.T) {
	mode := os.Getenv("SLUG_CARD_BRIDGE_PROCESS")
	if mode == "" {
		return
	}
	ref := cardRef(t)
	queued, completed := ref.job(t, "queued"), ref.job(t, "completed")
	scan := bufio.NewScanner(os.Stdin)
	count := 0
	for scan.Scan() {
		var q pilot.CardRequest
		if json.Unmarshal(scan.Bytes(), &q) != nil {
			os.Exit(2)
		}
		if mode == "drop" || mode == "drop-read" {
			os.Exit(0)
		}
		j := queued
		out := pilot.CardResponse{ID: q.ID, OK: true, View: &j.View, StoreID: j.StoreID, Job: &j}
		if mode == "busy" {
			out = pilot.CardResponse{ID: q.ID, Error: "busy"}
		} else if mode == "refusal" {
			out = pilot.CardResponse{ID: q.ID, Error: "card_source_incomplete"}
		} else if q.Command == "card_job" {
			count++
			j = completed
			if mode == "wrong-view" {
				j.View.Generation++
			}
			if mode == "changed-terminal" && count > 1 {
				j.Result.Status.ObservedAtMS++
			}
			if mode == "backwards" {
				j = queued
				if count == 1 {
					j.State = "running"
				}
			}
			out.View = &j.View
		} else if mode == "bad-ack" {
			j = completed
		}
		if mode == "wrong-family" && q.Command == "card_job" {
			fmt.Printf("{\"id\":%d,\"ok\":false,\"error\":\"seed_unavailable\"}\n", q.ID)
			continue
		}
		raw, e := pilot.MarshalCardResponse(out)
		if e != nil {
			os.Exit(3)
		}
		fmt.Println(string(raw))
	}
	os.Exit(0)
}
func cardPipeBridge(t *testing.T, mode string) (*StableBridge, pilot.CardJob) {
	t.Helper()
	ref := cardRef(t)
	j := ref.job(t, "queued")
	var s pilot.CardSnapshot
	for _, d := range ref.DTOs {
		if d.Kind == "CardSnapshot" && d.Name == "zero256/ready" {
			if json.Unmarshal(d.Value, &s) != nil {
				t.Fatal("snapshot")
			}
		}
	}
	if s.Pin == nil {
		t.Fatal("reference snapshot")
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestCardBridgeReferencePipeProcess$")
	cmd.Env = append(os.Environ(), "SLUG_CARD_BRIDGE_PROCESS="+mode)
	b, e := startStable(cmd)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = b.Close() })
	v := j.View
	b.mu.Lock()
	b.cardsEnabled = true
	b.view = &v
	b.cardPin = &cardAuthority{pin: *s.Pin, storeID: s.StoreID, createdMS: s.CreatedAtMS, sourceCreatedMS: s.SourceCreatedAtMS}
	b.card = &cardCorrelation{view: v, jobs: map[string]cardJobExpectation{}, snapshotJobs: map[string]bool{}}
	b.mu.Unlock()
	return b, j
}
func TestCardBridgePipeCorrelationAndUncertainty(t *testing.T) {
	for _, mode := range []string{"good", "drop", "busy", "refusal", "bad-ack", "wrong-view", "wrong-family", "changed-terminal", "backwards"} {
		t.Run(mode, func(t *testing.T) {
			b, q := cardPipeBridge(t, mode)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			r := b.CardRequest(ctx, pilot.CardRequest{Command: "card_build", ViewID: q.View.ViewID, Saturday: q.Saturday})
			switch mode {
			case "drop", "bad-ack":
				if r.OK || r.Error != "outcome_unknown" {
					t.Fatal(r.Error)
				}
				return
			case "busy", "refusal":
				want := "busy"
				if mode == "refusal" {
					want = "card_source_incomplete"
				}
				if r.OK || r.Error != want || len(b.card.jobs) != 0 {
					t.Fatal("definite refusal", r.Error)
				}
				return
			}
			if !r.OK || r.Job.State != "queued" {
				t.Fatal("ACK", r.Error)
			}
			req := pilot.CardRequest{Command: "card_job", ViewID: q.View.ViewID, JobID: r.Job.JobID}
			r = b.CardRequest(ctx, req)
			if mode == "wrong-view" || mode == "wrong-family" {
				if r.OK || r.Error != "card_unavailable" {
					t.Fatal("known job failure", r.Error)
				}
				return
			}
			if !r.OK {
				t.Fatal("job", r.Error)
			}
			again := b.CardRequest(ctx, req)
			if mode == "changed-terminal" || mode == "backwards" {
				if again.OK || again.Error != "card_unavailable" {
					t.Fatal("changed known job accepted", again.Error)
				}
			} else if !again.OK || cardJobDigest(*again.Job) != cardJobDigest(*r.Job) {
				t.Fatal("terminal changed")
			}
		})
	}
	for _, command := range []string{"card_status", "card_record", "card_history"} {
		t.Run(command+"-lost-observation", func(t *testing.T) {
			b, j := cardPipeBridge(t, "drop-read")
			q := pilot.CardRequest{Command: command, ViewID: j.View.ViewID, Saturday: j.Saturday}
			if command == "card_history" {
				q.Saturday = ""
				q.Limit = 8
			}
			r := b.CardRequest(t.Context(), q)
			if r.OK || r.Error != "outcome_unknown" {
				t.Fatal("observation uncertainty", r.Error)
			}
		})
	}
}
func TestCardNativeCancellationAfterSuccessfulPrefetch(t *testing.T) {
	b, j := cardPipeBridge(t, "good")
	b.card.snapshotJobs[j.JobID] = true
	recorded := &entryRecordingInput{WriteCloser: b.input}
	b.input = recorded
	base, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	id, e := b.begin(base)
	if e != nil {
		t.Fatal(e)
	}
	ctx := &entryCancelAfterPrefetchContext{Context: base, cancel: cancel, bridge: b, afterID: id}
	r := b.cardRequestHeld(ctx, id, pilot.CardRequest{Command: "card_cancel", ViewID: j.View.ViewID, JobID: j.JobID})
	<-b.gate
	if r.OK || r.Error != "card_cancelled" || base.Err() != context.Canceled {
		t.Fatal(r.Error, base.Err())
	}
	if len(recorded.commands) != 1 || recorded.commands[0] != "card_job" {
		t.Fatal("dispatched after cancellation", recorded.commands)
	}
}
