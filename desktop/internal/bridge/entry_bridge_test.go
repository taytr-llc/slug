// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/wire/v2"
)

type entryReferenceCase struct {
	Name     string                `json:"name"`
	Pin      gymentry.Pin          `json:"pin"`
	View     pilot.ManifestViewPin `json:"current_view"`
	Request  pilot.EntryRequest    `json:"request"`
	Response json.RawMessage       `json:"response"`
	Expected *pilot.EntryJob       `json:"expected_job"`
	Digest   string                `json:"terminal_digest"`
	Accepted bool                  `json:"accepted"`
}
type entryBridgeReference struct {
	Cases []entryReferenceCase `json:"protocol_cases"`
	DTOs  []struct {
		Fixture, Kind string
		Value         json.RawMessage
	} `json:"dtos"`
}

func entryReference(t *testing.T) entryBridgeReference {
	t.Helper()
	if wire.TestPoW {
		t.Skip("independent corpus uses normal candidate identities")
	}
	raw, err := os.ReadFile("../../../gymentry/testdata/reference/entry_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != "d8f226964429e8068bbe8300c73b6061f9a6dfd4e737a06ddde40c5eaa992bee" {
		t.Fatal("independently frozen entry corpus changed")
	}
	var ref entryBridgeReference
	if err := json.Unmarshal(raw, &ref); err != nil {
		t.Fatal(err)
	}
	return ref
}
func (r entryBridgeReference) fixtureIntent(t *testing.T, fixture string) gymentry.Intent {
	t.Helper()
	for _, row := range r.DTOs {
		if row.Fixture == fixture && row.Kind == "Intent" {
			var i gymentry.Intent
			if err := json.Unmarshal(row.Value, &i); err != nil {
				t.Fatal(err)
			}
			return i
		}
	}
	t.Fatal("missing independent intent")
	return gymentry.Intent{}
}
func (r entryBridgeReference) named(t *testing.T, name string) entryReferenceCase {
	t.Helper()
	for _, c := range r.Cases {
		if c.Name == name {
			return c
		}
	}
	t.Fatal("missing independent protocol case", name)
	return entryReferenceCase{}
}
func referenceExpectation(j pilot.EntryJob, intent gymentry.Intent, digest string) entryJobExpectation {
	e := entryJobExpectation{kind: j.Kind, eventID: j.EventID, saturday: j.Saturday,
		operationID: j.OperationID, state: j.State, terminalDigest: digest}
	if j.Kind == "prepare" {
		e.request = cloneEntryRequest(j.Intent.Request)
	}
	if j.Kind == "apply" {
		e.request, e.intent = cloneEntryRequest(intent.Request), cloneEntryIntent(intent)
	}
	return e
}

// The complete response bytes, full expected job and terminal digest are from
// the separately authored/frozen Python oracle, never from this Go bridge.
// Its five-class Check cases are structural examples, not proof admission.
func TestEntryBridgeIndependentProtocolCorpus(t *testing.T) {
	ref := entryReference(t)
	if len(ref.Cases) != 74 {
		t.Fatal("protocol coverage changed")
	}
	for _, c := range ref.Cases {
		t.Run(c.Name, func(t *testing.T) {
			fixture := strings.SplitN(c.Name, "/", 2)[0]
			intent := ref.fixtureIntent(t, fixture)
			b := &StableBridge{view: &c.View}
			store := ""
			// StoreID is an oracle value even in deliberately foreign replies.
			base := ref.named(t, fixture+"/completed-prepare")
			store = base.Expected.StoreID
			r, err := pilot.ParseEntryResponse(c.Response)
			accepted := err == nil && r.ID == c.Request.ID
			if accepted && r.OK {
				accepted = b.correlateEntryEnvelope(c.Request, r, c.Pin, store)
				if accepted && r.Job != nil {
					var e entryJobExpectation
					if c.Expected != nil {
						e = referenceExpectation(*c.Expected, intent, c.Digest)
					} else {
						// A queued acknowledgement binds the caller's request.
						e = entryJobExpectation{eventID: intent.Request.EventID, saturday: intent.Request.Saturday}
						switch c.Request.Command {
						case "entry_prepare":
							e.kind = "prepare"
							e.request = cloneEntryRequest(*c.Request.Request)
						case "entry_apply":
							e.kind = "apply"
							e.operationID = c.Request.OperationID
							e.intent = cloneEntryIntent(intent)
							e.request = cloneEntryRequest(intent.Request)
						case "entry_check":
							e.kind = "check"
							e.saturday = c.Request.Saturday
						}
					}
					accepted = b.correlateEntryJob(c.Request, *r.Job, e, c.Pin, store)
				}
			}
			if accepted != c.Accepted {
				t.Fatalf("accepted=%v want=%v parse=%v", accepted, c.Accepted, err)
			}
		})
	}
}

// This helper plays frozen successful payloads through real bounded pipes.
// Fault modes alter transport/correlation only; no source proof is evaluated.
func TestEntryBridgeProcess(t *testing.T) {
	mode := os.Getenv("SLUG_ENTRY_BRIDGE_PROCESS")
	if mode == "" {
		return
	}
	ref := entryReference(t)
	base := ref.named(t, "zero256/completed-prepare")
	intent := ref.fixtureIntent(t, "zero256")
	view := base.View
	store := base.Expected.StoreID
	scan := bufio.NewScanner(os.Stdin)
	var job pilot.EntryJob
	var lastID uint64
	for scan.Scan() {
		var req pilot.EntryRequest
		if json.Unmarshal(scan.Bytes(), &req) != nil || req.ID <= lastID {
			os.Exit(2)
		}
		lastID = req.ID
		var reply any
		r := pilot.EntryResponse{ID: req.ID, OK: true, View: &view, StoreID: store}
		switch req.Command {
		case "snapshot":
			s := manifestBridgeSnapshot(t)
			s.View = &view
			s.Account.ID = view.AccountID
			s.Account.Generation = view.Generation
			reply = pilot.ManifestResponse{ID: req.ID, OK: true, Snapshot: &s}
		case "stable_snapshot":
			s := pilot.StableSnapshot{Version: 1, State: "ready", View: &view, Status: &gymstable.Status{Version: 1, StoreID: base.Pin.StableStoreID, Pin: base.Pin.StablePin, CaughtUp: true}}
			reply = pilot.StableResponse{ID: req.ID, OK: true, Snapshot: &s}
		case "entry_snapshot":
			s := pilot.EntrySnapshot{Version: 1, State: "idle", View: &view, StoreID: store}
			if strings.HasPrefix(mode, "discover-") {
				kind := strings.SplitN(strings.TrimPrefix(mode, "discover-"), "-", 2)[0]
				c := ref.named(t, "zero256/completed-"+kind)
				job = *c.Expected
				s.LastJobID = job.JobID
			}
			if mode == "wrong-store" {
				s.StoreID = strings.Repeat("a", 64)
				r.StoreID = s.StoreID
			}
			r.Snapshot = &s
		case "entry_operation", "entry_pending":
			if strings.HasPrefix(mode, "discover-apply-lookup-") {
				code := strings.TrimPrefix(mode, "discover-apply-lookup-")
				if code == "drop" {
					os.Exit(0)
				}
				r = entryFailure(req.ID, code)
				break
			}
			i := intent
			if mode == "historical" {
				i.Generation++
				i.ViewID = strings.Repeat("b", 64)
			}
			r.Operation = &gymentry.OperationResult{Version: 1, StoreID: store, Operation: &gymentry.Operation{Version: 1, Intent: i, State: "prepared"}}
		case "entry_prepare", "entry_apply":
			if mode == "drop" {
				os.Exit(0)
			}
			if mode == "busy" {
				r = entryFailure(req.ID, "busy")
				break
			}
			kind := strings.TrimPrefix(req.Command, "entry_")
			c := ref.named(t, "zero256/completed-"+kind)
			job = *c.Expected
			job.State = "queued"
			job.Intent = nil
			job.Mutation = nil
			if kind == "prepare" {
				job.OperationID = ""
			}
			if mode == "wrong-event" {
				job.Saturday = "2026-11-28"
			}
			r.Job = &job
		case "entry_job", "entry_cancel":
			if strings.HasSuffix(mode, "-drop-job") {
				os.Exit(0)
			}
			if strings.HasSuffix(mode, "-malformed-job") {
				fmt.Println("{")
				os.Exit(0)
			}
			if req.JobID != job.JobID {
				os.Exit(3)
			}
			c := ref.named(t, "zero256/completed-"+job.Kind)
			job = *c.Expected
			r.Job = &job
		default:
			os.Exit(4)
		}
		if reply == nil {
			reply = r
		}
		raw, _ := json.Marshal(reply)
		fmt.Println(string(raw))
	}
	os.Exit(0)
}
func entryTestBridge(t *testing.T, mode string) *StableBridge {
	t.Helper()
	if wire.TestPoW {
		t.Skip("normal candidate transport profile")
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestEntryBridgeProcess$", "-test.count=1")
	cmd.Env = append(os.Environ(), "SLUG_ENTRY_BRIDGE_PROCESS="+mode)
	b, err := startStable(cmd)
	if err != nil {
		t.Fatal(err)
	}
	b.entriesEnabled = true
	t.Cleanup(func() {
		if err := b.Close(); err != nil {
			t.Error(err)
		}
	})
	return b
}
func establishEntries(t *testing.T, b *StableBridge) string {
	t.Helper()
	ctx := context.Background()
	if _, err := b.Request(ctx, "snapshot", 0); err != nil {
		t.Fatal(err)
	}
	s := b.StableRequest(ctx, pilot.StableRequest{Command: "stable_snapshot"})
	if !s.OK {
		t.Fatal(s)
	}
	r := b.EntryRequest(ctx, pilot.EntryRequest{Command: "entry_snapshot"})
	if !r.OK {
		t.Fatal(r)
	}
	return s.Snapshot.View.ViewID
}
func TestEntryNativeApplyPrefetchAndHistoricalRefusal(t *testing.T) {
	ref := entryReference(t)
	intent := ref.fixtureIntent(t, "zero256")
	for _, mode := range []string{"valid", "historical"} {
		t.Run(mode, func(t *testing.T) {
			b := entryTestBridge(t, mode)
			view := establishEntries(t, b)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			query := b.EntryRequest(ctx, pilot.EntryRequest{Command: "entry_operation", ViewID: view, OperationID: intent.OperationID})
			if !query.OK {
				t.Fatal("historical read refused", query)
			}
			r := b.EntryRequest(ctx, pilot.EntryRequest{Command: "entry_apply", ViewID: view, OperationID: intent.OperationID})
			if mode == "historical" {
				if r.Error != "stale_view" {
					t.Fatal(r)
				}
				return
			}
			if !r.OK || r.Job == nil || r.Job.State != "queued" {
				t.Fatal("prefetch recursed or reused transport ID", r)
			}
			r = b.EntryRequest(ctx, pilot.EntryRequest{Command: "entry_job", ViewID: view, JobID: r.Job.JobID})
			if !r.OK || r.Job.State != "complete" || r.Job.Mutation.Receipt.OperationID != intent.OperationID {
				t.Fatal(r)
			}
		})
	}
}
func TestEntryNativeUncertainMutationVersusBusy(t *testing.T) {
	ref := entryReference(t)
	intent := ref.fixtureIntent(t, "zero256")
	for _, mode := range []string{"drop", "wrong-event", "busy"} {
		t.Run(mode, func(t *testing.T) {
			b := entryTestBridge(t, mode)
			view := establishEntries(t, b)
			r := b.EntryRequest(context.Background(), pilot.EntryRequest{Command: "entry_prepare", ViewID: view, Request: &intent.Request})
			want := "outcome_unknown"
			if mode == "busy" {
				want = "busy"
			}
			if r.Error != want {
				t.Fatalf("got %q want %q", r.Error, want)
			}
		})
	}
}
func TestEntryNativeSnapshotDiscoveryAndLegacyRoute(t *testing.T) {
	for _, kind := range []string{"prepare", "apply"} {
		t.Run(kind, func(t *testing.T) {
			b := entryTestBridge(t, "discover-"+kind)
			view := establishEntries(t, b)
			ref := entryReference(t)
			job := ref.named(t, "zero256/completed-"+kind).Expected
			r := b.EntryRequest(context.Background(), pilot.EntryRequest{Command: "entry_job", ViewID: view, JobID: job.JobID})
			if !r.OK || r.Job.State != "complete" {
				t.Fatal(r)
			}
			if e, known, _ := b.entryExpectation(job.JobID); !known || e.request == nil || e.terminalDigest == "" {
				t.Fatal("incomplete discovered tuple")
			}
		})
	}
	b := &StableBridge{}
	if r := b.EntryRequest(context.Background(), pilot.EntryRequest{Command: "entry_snapshot"}); r.Error != "entry_unavailable" {
		t.Fatal(r)
	}
}

func TestEntryNativeDiscoveredMutationRecoveryFailure(t *testing.T) {
	ref := entryReference(t)
	job := ref.named(t, "zero256/completed-apply").Expected
	for _, mode := range []string{
		"discover-apply-lookup-entry_not_found",
		"discover-apply-lookup-entry_unavailable",
		"discover-apply-lookup-drop",
		"discover-apply-lookup-busy",
		"discover-apply-drop-job",
		"discover-apply-malformed-job",
	} {
		t.Run(mode, func(t *testing.T) {
			b := entryTestBridge(t, mode)
			view := establishEntries(t, b)
			r := b.EntryRequest(context.Background(), pilot.EntryRequest{Command: "entry_job", ViewID: view, JobID: job.JobID})
			want := "outcome_unknown"
			if mode == "discover-apply-lookup-busy" {
				want = "busy"
			}
			if r.OK || r.Error != want {
				t.Fatalf("got ok=%v error=%q want=%q", r.OK, r.Error, want)
			}
		})
	}
}

type entryRecordingInput struct {
	io.WriteCloser
	commands []string
}

func (w *entryRecordingInput) Write(raw []byte) (int, error) {
	var req pilot.EntryRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return 0, err
	}
	w.commands = append(w.commands, req.Command)
	return w.WriteCloser.Write(raw)
}

// Report cancellation at the deterministic boundary after a successful
// prefetch allocates the next transport ID. The real context's Done channel
// closes too. This exercises the already-gate-held production path without a
// timer race or a runtime fault hook. It proves no subsequent frame is sent
// after observing cancellation, not atomic cancellation of an in-flight write.
type entryCancelAfterPrefetchContext struct {
	context.Context
	cancel  context.CancelFunc
	bridge  *StableBridge
	afterID uint64
}

func (c *entryCancelAfterPrefetchContext) Err() error {
	if c.bridge.nextID > c.afterID {
		c.cancel()
	}
	return c.Context.Err()
}

func TestEntryNativeCancellationAfterSuccessfulPrefetch(t *testing.T) {
	ref := entryReference(t)
	intent := ref.fixtureIntent(t, "zero256")
	for _, command := range []string{"entry_apply", "entry_cancel"} {
		t.Run(command, func(t *testing.T) {
			mode := "valid"
			if command == "entry_cancel" {
				mode = "discover-prepare"
			}
			b := entryTestBridge(t, mode)
			view := establishEntries(t, b)
			recorded := &entryRecordingInput{WriteCloser: b.input}
			b.input = recorded
			base, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			id, err := b.begin(base)
			if err != nil {
				t.Fatal(err)
			}
			ctx := &entryCancelAfterPrefetchContext{Context: base, cancel: cancel, bridge: b, afterID: id}
			q := pilot.EntryRequest{Command: command, ViewID: view}
			wantPrefetch := "entry_operation"
			if command == "entry_apply" {
				q.OperationID = intent.OperationID
			} else {
				q.JobID = ref.named(t, "zero256/completed-prepare").Expected.JobID
				wantPrefetch = "entry_job"
			}
			r := b.entryRequestHeld(ctx, id, q)
			<-b.gate
			if r.OK || r.Error != "entry_cancelled" || base.Err() != context.Canceled {
				t.Fatalf("got ok=%v error=%q context=%v", r.OK, r.Error, base.Err())
			}
			if len(recorded.commands) != 1 || recorded.commands[0] != wantPrefetch {
				t.Fatalf("dispatched after cancelled prefetch: %v", recorded.commands)
			}
			if command == "entry_cancel" {
				// The earlier Prepare really completed. Refusing a new Cancel
				// dispatch must not claim that this durable operation never ran.
				e, known, _ := b.entryExpectation(q.JobID)
				if !known || e.state != "complete" || e.intent == nil || e.intent.OperationID != intent.OperationID {
					t.Fatal("cancel refusal lost the completed prepared operation")
				}
			}
		})
	}
}

func TestEntryMetadataCommandCannotSubstituteOperationState(t *testing.T) {
	ref := entryReference(t)
	c := ref.named(t, "zero256/completed-prepare")
	i := ref.fixtureIntent(t, "zero256")
	b := &StableBridge{view: &c.View}
	for _, command := range []string{"entry_pending", "entry_abandon", "entry_operation"} {
		for _, state := range []string{"prepared", "abandoned", "committed"} {
			o := gymentry.Operation{Version: 1, Intent: i, State: state}
			if state == "abandoned" {
				o.AbandonedAtMS = i.PreparedAtMS + 1
			}
			if state == "committed" {
				r := ref.named(t, "zero256/completed-apply").Expected.Mutation.Receipt
				o.Receipt = &r
			}
			r := pilot.EntryResponse{ID: 1, OK: true, View: &c.View, StoreID: c.Expected.StoreID, Operation: &gymentry.OperationResult{Version: 1, StoreID: c.Expected.StoreID, Operation: &o}}
			q := pilot.EntryRequest{ID: 1, Command: command, ViewID: c.View.ViewID}
			if command != "entry_pending" {
				q.OperationID = i.OperationID
			}
			want := command == "entry_operation" || command == "entry_pending" && state == "prepared" || command == "entry_abandon" && state != "prepared"
			if got := b.correlateEntryEnvelope(q, r, c.Pin, c.Expected.StoreID); got != want {
				t.Fatalf("%s/%s accepted=%v want=%v", command, state, got, want)
			}
		}
	}
}
