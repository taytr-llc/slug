// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/taytr-llc/slug/desktop/pilot"
	"os"
	"testing"
)

type activityReferenceContext struct {
	Binding  pilot.WalletActivityBinding  `json:"expected_binding"`
	Request  pilot.WalletActivityRequest  `json:"request"`
	Capture  *pilot.WalletActivityCapture `json:"capture"`
	Items    []pilot.WalletActivityItem   `json:"captured_items"`
	Selected *pilot.WalletActivityItem    `json:"selected_item"`
	Known    []string                     `json:"known_job_ids"`
}

func activityReferenceBridge(c activityReferenceContext, out pilot.WalletActivityResponse) *PoolBridge {
	b := &PoolBridge{&StableBridge{poolMode: true, activityBinding: &c.Binding}}
	if s := out.Snapshot; s != nil {
		known := func(id string) bool {
			for _, v := range c.Known {
				if v == id {
					return true
				}
			}
			return false
		}
		if s.CaptureJobID != "" && known(s.CaptureJobID) {
			b.activityCapture = &activityExpectation{job: pilot.WalletActivityJob{Version: 1, JobID: s.CaptureJobID, Binding: c.Binding, State: "done"}}
		}
		id := s.ActiveJobID
		if id == "" || id == s.CaptureJobID {
			id = s.LastJobID
		}
		if id != "" && known(id) {
			b.activityAttempt = &activityExpectation{job: pilot.WalletActivityJob{Version: 1, JobID: id, Binding: c.Binding, State: "running"}}
		}
		return b
	}
	b.activityAttempt = &activityExpectation{job: pilot.WalletActivityJob{Version: 1, JobID: c.Request.JobID, Binding: c.Binding, State: "running"}}
	if c.Capture != nil && (c.Request.Command == "wallet_activity_page" || c.Request.Command == "wallet_activity_detail") {
		cap := *c.Capture
		b.activityAttempt.job.State = "done"
		b.activityAttempt.job.Capture = &cap
		b.activityCapture = b.activityAttempt
		p := pilot.WalletActivityPage{Version: 1, Binding: c.Binding, JobID: c.Request.JobID, Capture: cap, Limit: 8}
		if c.Request.Command == "wallet_activity_page" {
			p.After = c.Request.After
			p.Limit = c.Request.Limit
			p.Next = min(uint32(len(c.Items)), p.After+p.Limit)
			if p.After <= p.Next {
				p.Items = c.Items[p.After:p.Next]
			}
			p.HasMore = p.Next < cap.Count
		} else if c.Selected != nil {
			p.Items = []pilot.WalletActivityItem{*c.Selected}
			p.Next = 1
			p.Limit = 1
			p.HasMore = cap.Count > 1
		}
		owned := pilot.CloneWalletActivityPage(p)
		b.activityRows = &owned
	}
	return b
}
func TestWalletActivityIndependentWireCorpus(t *testing.T) {
	raw, e := os.ReadFile("../../frontend/testdata/wallet-activity-wire/wire-v1.json")
	if e != nil {
		t.Fatal(e)
	}
	if fmt.Sprintf("%x", sha256.Sum256(raw)) != "fd0f28ff06b4cb0502e7822ad401330941cdb71e9a4d4b5e31d5a017f911dc2b" {
		t.Fatal("independent source corpus changed")
	}
	type row struct {
		Name, Kind, Scope, Frame string
		Context                  string `json:"context_name"`
		SHA                      string `json:"sha256"`
		Raw                      string `json:"raw_hex"`
		Valid                    bool
	}
	var data struct {
		Contexts             map[string]activityReferenceContext
		Positives, Negatives []row
		Raw                  []row `json:"raw_cases"`
	}
	if e = json.Unmarshal(raw, &data); e != nil {
		t.Fatal(e)
	}
	if len(data.Positives) != 70 || len(data.Negatives) != 120 || len(data.Raw) != 20 {
		t.Fatal("incomplete frozen corpus")
	}
	parse := func(kind string, frame []byte) ([]byte, error) {
		frame = bytes.TrimSuffix(frame, []byte{'\n'})
		if kind == "request" {
			r, e := pilot.ParseWalletActivityRequest(frame)
			if e != nil {
				return nil, e
			}
			return pilot.MarshalWalletActivityRequest(r)
		}
		r, e := pilot.ParseWalletActivityResponse(frame)
		if e != nil {
			return nil, e
		}
		return pilot.MarshalWalletActivityResponse(r)
	}
	correlated := func(row row) bool {
		c, ok := data.Contexts[row.Context]
		if !ok {
			t.Fatal("missing independent original context")
		}
		if row.Kind == "request" {
			r, e := pilot.ParseWalletActivityRequest([]byte(row.Frame))
			return e == nil && (r.Command == "wallet_activity_snapshot" || r.ViewID == c.Binding.View.ViewID)
		}
		r, e := pilot.ParseWalletActivityResponse([]byte(row.Frame))
		if e != nil {
			return false
		}
		if !r.OK {
			return r.ID == c.Request.ID
		}
		return activityReferenceBridge(c, r).correlateWalletActivity(c.Request, r)
	}
	for _, r := range data.Positives {
		t.Run("positive/"+r.Name, func(t *testing.T) {
			if fmt.Sprintf("%x", sha256.Sum256([]byte(r.Frame))) != r.SHA {
				t.Fatal("independent frame checksum")
			}
			got, e := parse(r.Kind, []byte(r.Frame))
			if e != nil {
				t.Fatal(e)
			}
			if !bytes.Equal(append(got, '\n'), []byte(r.Frame)) {
				t.Fatal("canonical bytes differ")
			}
			if !correlated(r) {
				t.Fatal("original context rejected")
			}
		})
	}
	for _, r := range data.Negatives {
		t.Run("negative/"+r.Name, func(t *testing.T) {
			_, e := parse(r.Kind, []byte(r.Frame))
			if r.Scope == "shape" {
				if e == nil {
					t.Fatal("invalid shape accepted")
				}
				return
			}
			if r.Scope != "correlation" || e != nil {
				t.Fatal("expected shaped contextual refusal", e)
			}
			if correlated(r) {
				t.Fatal("foreign original context accepted")
			}
		})
	}
	for _, r := range data.Raw {
		t.Run("raw/"+r.Name, func(t *testing.T) {
			b, e := hex.DecodeString(r.Raw)
			if e != nil {
				t.Fatal(e)
			}
			_, e = parse(r.Kind, b)
			if (e == nil) != r.Valid {
				t.Fatalf("valid=%v error=%v", r.Valid, e)
			}
		})
	}
}
