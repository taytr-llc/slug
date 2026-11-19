// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymresult"
	"github.com/taytr-llc/slug/internal/careerstate"
	"github.com/taytr-llc/slug/wire/v2"
)

const historyReferenceSHA = "18e51e25c6760f756159e12a92265b517438152533d439d541d4696044e60283"

type historyProtocol struct {
	ID       string                     `json:"id"`
	Request  pilot.CareerHistoryRequest `json:"request"`
	Response json.RawMessage            `json:"response"`
	Context  struct {
		View       pilot.ManifestViewPin   `json:"view"`
		StoreID    string                  `json:"store_id"`
		Activation careerstate.Activation  `json:"activation"`
		Known      *pilot.CareerHistoryJob `json:"known_job"`
		Selected   *pilot.CareerHistoryJob `json:"selected_job"`
	} `json:"context"`
	Accept bool `json:"accept"`
}
type historyCorpus struct {
	Wire struct {
		Protocols []historyProtocol `json:"protocols"`
	} `json:"wire"`
}

var historyOnce sync.Once
var historyReferenceValue historyCorpus
var historyReferenceErr error

func historyReference(t *testing.T) historyCorpus {
	t.Helper()
	if wire.TestPoW {
		t.Skip("frozen normal candidate identities")
	}
	historyOnce.Do(func() {
		raw, e := os.ReadFile("../../../gymstable/testdata/career-history-reference/corpus.json")
		if e != nil {
			historyReferenceErr = e
			return
		}
		h := sha256.Sum256(raw)
		if hex.EncodeToString(h[:]) != historyReferenceSHA {
			historyReferenceErr = os.ErrInvalid
			return
		}
		historyReferenceErr = json.Unmarshal(raw, &historyReferenceValue)
	})
	if historyReferenceErr != nil {
		t.Fatal(historyReferenceErr)
	}
	return historyReferenceValue
}
func historyProtocolNamed(t *testing.T, name string) historyProtocol {
	t.Helper()
	for _, v := range historyReference(t).Wire.Protocols {
		if v.ID == name {
			return v
		}
	}
	t.Fatal("missing reference", name)
	return historyProtocol{}
}
func historyPageFixture(t *testing.T, name string) (pilot.CareerHistoryPageResult, pilot.CareerHistoryJob) {
	t.Helper()
	p := historyProtocolNamed(t, "page/core/"+name)
	var r pilot.CareerHistoryResponse
	if e := json.Unmarshal(p.Response, &r); e != nil || r.Page == nil || p.Context.Selected == nil {
		t.Fatal("page fixture", e)
	}
	return *r.Page, *p.Context.Selected
}

// Modeled original native discovery facts; no account/DB/authority is created.
func historyModelBridge(t *testing.T, j pilot.CareerHistoryJob) *StableBridge {
	t.Helper()
	if j.Capture == nil {
		t.Fatal("capture required")
	}
	v, p, a := j.View, j.Capture.Status.Pin, j.Capture.Activation
	s := pilot.CareerHistorySnapshot{Version: 1, State: "ready", View: &v, StoreID: j.StoreID, Activation: &a, LastJobID: j.JobID}
	h := &careerHistoryCorrelation{snapshot: &s, selected: j.JobID}
	h.jobs[0] = historyTuple(j)
	return &StableBridge{careersEnabled: true, view: &v, manifestView: &v, stablePin: &p, storeID: j.StoreID, careerActivation: &a, history: h, resultPin: &resultAuthority{storeID: a.ResultStoreID, createdMS: a.ResultCreatedAtMS, sourceCreatedMS: a.SourceCreatedAtMS}, result: &resultCorrelation{view: v}}
}
func TestCareerHistoryFrozenNativeCorrelation(t *testing.T) {
	all := historyReference(t)
	if len(all.Wire.Protocols) != 45 {
		t.Fatal(len(all.Wire.Protocols))
	}
	_, base := historyPageFixture(t, "capacity256")
	for _, c := range all.Wire.Protocols {
		t.Run(c.ID, func(t *testing.T) {
			b := historyModelBridge(t, base)
			v, a := c.Context.View, c.Context.Activation
			b.view = &v
			b.manifestView = &v
			b.storeID = c.Context.StoreID
			b.careerActivation = &a
			b.resultPin = &resultAuthority{storeID: a.ResultStoreID, createdMS: a.ResultCreatedAtMS, sourceCreatedMS: a.SourceCreatedAtMS}
			b.result = &resultCorrelation{view: v}
			if c.Context.Selected != nil {
				j := *c.Context.Selected
				b = historyModelBridge(t, j)
				b.view = &v
				b.manifestView = &v
				b.storeID = c.Context.StoreID
			}
			b.history.jobs = [3]careerHistoryTuple{}
			b.history.selected = ""
			s := b.history.snapshot
			s.View = &v
			s.StoreID = b.storeID
			s.Activation = &a
			s.State = "idle"
			s.LastJobID = ""
			if oneOf(c.Request.Command, "career_history_job", "career_history_cancel") {
				s.State = "ready"
				s.LastJobID = c.Request.JobID
			}
			if c.Context.Known != nil {
				b.history.jobs[0] = historyTuple(*c.Context.Known)
			}
			if c.Context.Selected != nil {
				b.history.jobs[0] = historyTuple(*c.Context.Selected)
				b.history.selected = c.Context.Selected.JobID
			}
			// Supplied contexts are post-native-admission, exactly as the oracle states.
			r, e := pilot.ParseCareerHistoryResponse(c.Response)
			accepted := e == nil
			if accepted && !r.OK {
				accepted = r.ID == c.Request.ID
			} else if accepted {
				accepted = b.correlateCareerHistory(c.Request, r)
			}
			if accepted != c.Accept {
				t.Fatalf("accepted=%v expected=%v parse=%v", accepted, c.Accept, e)
			}
		})
	}
}
func TestCareerHistoryRowsOwnedAndRevisited(t *testing.T) {
	p, j := historyPageFixture(t, "capacity256")
	b := historyModelBridge(t, j)
	q := pilot.CareerHistoryRequest{ID: 1, Version: 1, Command: "career_history_page", ViewID: j.View.ViewID, JobID: j.JobID, After: p.Page.After, Limit: 8}
	r := pilot.CareerHistoryResponse{ID: 1, Version: 1, OK: true, Page: &p}
	if !b.correlateCareerHistory(q, r) {
		t.Fatal("first page")
	}
	original := b.history.page.Page.Items[0]
	p.Page.Items[0].Event.ResultDigest = strings.Repeat("f", 64)
	if b.history.page.Page.Items[0] != original {
		t.Fatal("returned row aliases native cache")
	}
	if pilot.ValidateCareerHistoryPageResult(p) != nil {
		t.Fatal("changed included digest remains locally well-shaped")
	}
	if b.correlateCareerHistory(q, r) {
		t.Fatal("changed seen row accepted")
	}
	if b.history.page.Page.Items[0] != original {
		t.Fatal("failed page overwrote cache")
	}
	// Different offsets must consult the same fixed ordinal commitments.
	p, _ = historyPageFixture(t, "capacity256")
	p.Page.Items = p.Page.Items[1:]
	p.Page.After = 1
	q.After = 1
	q.Limit = 7
	if !b.correlateCareerHistory(q, r) {
		t.Fatal("overlap")
	}
	if !historyMetadataBound(b.history) {
		t.Fatal("metadata cap")
	}
}
func TestCareerHistorySelectionDiscoveryAndFatal(t *testing.T) {
	_, j := historyPageFixture(t, "capacity256")
	b := historyModelBridge(t, j)
	old := j
	newJob := j
	newJob.JobID = strings.Repeat("e", 64)
	newJob.State = "queued"
	newJob.Capture = nil
	q := pilot.CareerHistoryRequest{ID: 1, Version: 1, Command: "career_history_begin", ViewID: j.View.ViewID, ProspectID: j.ProspectID}
	if !b.correlateCareerHistory(q, pilot.CareerHistoryResponse{ID: 1, Version: 1, OK: true, Job: &newJob}) {
		t.Fatal("begin")
	}
	q.Command = "career_history_job"
	q.JobID = old.JobID
	q.ProspectID = ""
	if !b.correlateCareerHistory(q, pilot.CareerHistoryResponse{ID: 1, Version: 1, OK: true, Job: &old}) {
		t.Fatal("historical terminal")
	}
	if b.history.selected != newJob.JobID {
		t.Fatal("old terminal changed selection")
	}
	old.State = "running"
	old.Capture = nil
	if b.correlateCareerHistory(q, pilot.CareerHistoryResponse{ID: 1, Version: 1, OK: true, Job: &old}) {
		t.Fatal("terminal regression")
	}
	unknown := newJob
	unknown.JobID = strings.Repeat("d", 64)
	q.JobID = unknown.JobID
	if b.correlateCareerHistory(q, pilot.CareerHistoryResponse{ID: 1, Version: 1, OK: true, Job: &unknown}) {
		t.Fatal("unnamed lookup")
	}
	q.JobID = newJob.JobID
	newJob.State = "failed"
	newJob.Error = "history_corrupt"
	if !b.correlateCareerHistory(q, pilot.CareerHistoryResponse{ID: 1, Version: 1, OK: true, Job: &newJob}) || !b.historyFatal || b.history != nil {
		t.Fatal("fatal latch")
	}
	b.clearCareerHistory(false)
	if !b.historyFatal {
		t.Fatal("Stop repairs fatal source")
	}
	b.clearStableView()
	if b.historyFatal || b.history != nil {
		t.Fatal("new identity reset")
	}
}
func TestCareerHistoryResultLinkOriginalEvent(t *testing.T) {
	p, _ := historyPageFixture(t, "genuine_win")
	want := p.Page.Items[0].Event
	a := p.Page.Capture.Activation
	for _, name := range []string{"result-finalized.json", "result-conflict.json"} {
		raw, e := os.ReadFile("../../../gymstable/testdata/career-history-reference/inputs/extracted/" + name)
		if e != nil {
			t.Fatal(e)
		}
		var r gymresult.Record
		if json.Unmarshal(raw, &r) != nil {
			t.Fatal("record")
		}
		if !historyResultMatches(want, a, r) {
			t.Fatal("retained finalized/conflicted record", name)
		}
		bad := want
		bad.FinalityDigest = strings.Repeat("f", 64)
		if historyResultMatches(bad, a, r) {
			t.Fatal("wrong first finality")
		}
		bad = want
		bad.SourceRecordDigest = strings.Repeat("f", 64)
		if historyResultMatches(bad, a, r) {
			t.Fatal("wrong source")
		}
		a2 := a
		a2.ActivationID = strings.Repeat("f", 64)
		if historyResultMatches(want, a2, r) {
			t.Fatal("foreign activation")
		}
		r.Finality = nil
		if historyResultMatches(want, a, r) {
			t.Fatal("provisional")
		}
	}
}
