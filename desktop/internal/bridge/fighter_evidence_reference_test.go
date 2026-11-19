//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/taytr-llc/slug/desktop/internal/recoveryfile"
	"github.com/taytr-llc/slug/desktop/pilot"
	"os"
	"testing"
)

type evidenceWireContext struct {
	Name         string                       `json:"name"`
	View         pilot.ManifestViewPin        `json:"view"`
	StoreID      string                       `json:"store_id"`
	HistoryJobID string                       `json:"history_job_id"`
	HistoryID    string                       `json:"history_id"`
	JobID        string                       `json:"job_id"`
	ProspectID   string                       `json:"prospect_id"`
	Summary      pilot.FighterEvidenceSummary `json:"summary"`
	Selection    recoveryfile.Pin             `json:"selection"`
}
type evidenceWireCase struct {
	Name        string                    `json:"name"`
	Kind        string                    `json:"kind"`
	ContextName string                    `json:"context_name"`
	Command     string                    `json:"command"`
	Previous    *pilot.FighterEvidenceJob `json:"previous"`
	Value       json.RawMessage           `json:"value"`
	Canonical   string                    `json:"canonical"`
	SHA256      string                    `json:"sha256"`
}

func evidenceWireBridge(c evidenceWireContext) *StableBridge {
	j := pilot.FighterEvidenceJob{Version: 1, JobID: c.JobID, State: "running", View: c.View, StoreID: c.StoreID, HistoryJobID: c.HistoryJobID, HistoryID: c.HistoryID, ProspectID: c.ProspectID}
	e := &evidenceExpectation{job: j, sourceDigest: c.Summary.SourceRecordDigest, count: c.Summary.EventCount, asOf: c.Summary.CapturedAsOfMS, selectionDigest: historyDigest(c.Selection)}
	return &StableBridge{view: &c.View, storeID: c.StoreID, evidence: [2]*evidenceExpectation{e, nil}}
}
func evidenceWireAccept(c evidenceWireContext, v evidenceWireCase) bool {
	b := evidenceWireBridge(c)
	if v.Previous != nil {
		b.evidence[0].terminal = historyDigest(*v.Previous)
	}
	raw := []byte(v.Canonical)
	if len(raw) == 0 {
		raw = v.Value
	}
	if v.Kind == "request" {
		r, e := pilot.ParseFighterEvidenceRequest(raw)
		return e == nil && b.evidenceRequestBoundLocked(r)
	}
	if v.Kind == "summary" {
		j := b.evidence[0].job
		j.State = "done"
		fields := map[string]json.RawMessage{}
		base, _ := json.Marshal(j)
		_ = json.Unmarshal(base, &fields)
		fields["summary"] = raw
		job, _ := json.Marshal(fields)
		r := map[string]json.RawMessage{"id": []byte("1"), "version": []byte("1"), "ok": []byte("true"), "snapshot": []byte("null"), "job": job, "error": []byte(`""`)}
		raw, _ = json.Marshal(r)
		v.Kind = "response"
		v.Command = "fighter_evidence_job"
	}
	if v.Kind == "job" {
		raw = append([]byte(`{"id":1,"version":1,"ok":true,"snapshot":null,"job":`), raw...)
		raw = append(raw, []byte(`,"error":""}`)...)
		v.Kind = "response"
		v.Command = "fighter_evidence_job"
	}
	if v.Kind == "snapshot" {
		raw = append([]byte(`{"id":1,"version":1,"ok":true,"snapshot":`), raw...)
		raw = append(raw, []byte(`,"job":null,"error":""}`)...)
		v.Kind = "response"
		v.Command = "fighter_evidence_snapshot"
	}
	if v.Kind != "response" {
		return false
	}
	r, e := pilot.ParseFighterEvidenceResponse(raw)
	if e != nil || r.ID != 1 {
		return false
	}
	if !r.OK {
		return true
	}
	req := pilot.FighterEvidenceRequest{ID: 1, Version: 1, Command: v.Command, ViewID: c.View.ViewID, JobID: c.JobID}
	return b.correlateFighterEvidence(req, r)
}
func TestFighterEvidenceIndependentWireCorpus(t *testing.T) {
	raw, e := os.ReadFile("../../frontend/testdata/fighter-evidence-wire/wire-v1.json")
	if e != nil {
		t.Fatal(e)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "c2489601d6c58440e001839c4ed14ce17dbaec43203a36ca197fa9ff89fc6dcb" {
		t.Fatal("wire oracle changed")
	}
	var corpus struct {
		Contexts  []evidenceWireContext              `json:"contexts"`
		Positives []evidenceWireCase                 `json:"positives"`
		Negatives []evidenceWireCase                 `json:"negatives"`
		Raw       []struct{ Name, Kind, Raw string } `json:"raw_negatives"`
	}
	if e = json.Unmarshal(raw, &corpus); e != nil {
		t.Fatal(e)
	}
	if len(corpus.Positives) != 112 || len(corpus.Negatives) != 38 || len(corpus.Raw) != 4 {
		t.Fatal("incomplete oracle")
	}
	contexts := map[string]evidenceWireContext{}
	for _, c := range corpus.Contexts {
		contexts[c.Name] = c
	}
	for _, v := range corpus.Positives {
		t.Run(v.Name, func(t *testing.T) {
			sum := sha256.Sum256([]byte(v.Canonical))
			if hex.EncodeToString(sum[:]) != v.SHA256 {
				t.Fatal("reference bytes changed")
			}
			if !evidenceWireAccept(contexts[v.ContextName], v) {
				t.Fatal("independent valid wire refused", v.Kind)
			}
		})
	}
	for _, v := range corpus.Negatives {
		t.Run(v.Name, func(t *testing.T) {
			if evidenceWireAccept(contexts[v.ContextName], v) {
				t.Fatal("independent rejection accepted", v.Kind)
			}
		})
	}
	for _, v := range corpus.Raw {
		t.Run(v.Name, func(t *testing.T) {
			if v.Kind == "request" {
				if _, e := pilot.ParseFighterEvidenceRequest([]byte(v.Raw)); e == nil {
					t.Fatal("raw request accepted")
				}
			} else {
				if _, e := pilot.ParseFighterEvidenceResponse([]byte(v.Raw)); e == nil {
					t.Fatal("raw response accepted")
				}
			}
		})
	}
}
