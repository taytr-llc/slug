// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/wire/v2"
	"os"
	"testing"
)

type boutReference struct {
	Compositions []struct {
		Pin           gymbout.Pin
		Record        gymbout.Record
		Views         []pilot.ManifestViewPin
		SourceCreated int64 `json:"source_store_created_at_ms"`
		Created       int64 `json:"store_created_at_ms"`
	}
	DTOs []struct {
		Kind, Name, Canonical string
		Value                 json.RawMessage
	}
	Cases []struct {
		Name          string
		View          pilot.ManifestViewPin `json:"current_view"`
		Pin           gymbout.Pin           `json:"authority_pin"`
		SourceCreated int64                 `json:"source_created_at_ms"`
		Created       int64                 `json:"created_at_ms"`
		Request       pilot.BoutRequest
		Response      json.RawMessage
		Expected      *pilot.BoutJob `json:"expected_job"`
		Digest        string         `json:"terminal_digest"`
		Accept        bool
	} `json:"protocol_cases"`
}

func boutRef(t *testing.T) boutReference {
	t.Helper()
	raw, e := os.ReadFile("../../../gymbout/testdata/reference/bout_v1.json")
	if e != nil {
		t.Fatal(e)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "09f0430789328b9226fc4a24aae3cdb07beeb8b41d61b42cd33e4ba3121fa2ec" {
		t.Fatal("frozen corpus changed")
	}
	if wire.TestPoW {
		t.Skip("normal profile reference")
	}
	var v boutReference
	if json.Unmarshal(raw, &v) != nil {
		t.Fatal("corpus")
	}
	return v
}
func TestBoutBridgeIndependentProtocol(t *testing.T) {
	ref := boutRef(t)
	if len(ref.Cases) != 74 {
		t.Fatal("protocol count")
	}
	for _, c := range ref.Cases {
		t.Run(c.Name, func(t *testing.T) {
			id, e := gymbout.ID(c.Pin)
			if e != nil {
				t.Fatal(e)
			}
			a := boutAuthority{pin: c.Pin, storeID: id, createdMS: c.Created, sourceCreatedMS: c.SourceCreated}
			v := c.View
			b := &StableBridge{view: &v, manifestView: &v, boutsEnabled: true, cardsEnabled: true, boutPin: &a, bout: &boutCorrelation{view: v, jobs: map[string]boutJobExpectation{}}, cardPin: &cardAuthority{pin: c.Pin.SourcePin, storeID: c.Pin.SourceStoreID, createdMS: c.SourceCreated}, card: &cardCorrelation{view: v, jobs: map[string]cardJobExpectation{}}}
			_, qe := pilot.MarshalBoutRequest(c.Request)
			var wire bytes.Buffer
			if e := json.Compact(&wire, c.Response); e != nil {
				t.Fatal(e)
			}
			r, pe := pilot.ParseBoutResponse(wire.Bytes())
			ok := qe == nil && pe == nil && r.ID == c.Request.ID
			if ok && r.OK {
				if c.Request.Command == "bout_snapshot" {
					if r.Snapshot != nil && r.Snapshot.State == "setup_incomplete" {
						b.boutPin = nil
					}
					ok = b.rememberBoutSnapshot(r)
				} else {
					ok = c.Request.ViewID == v.ViewID && b.correlateBoutEnvelope(c.Request, r, a, v)
					if ok && r.Job != nil {
						var want boutJobExpectation
						if c.Expected != nil {
							j := c.Expected
							want = boutJobExpectation{cardID: j.CardID, saturday: j.Saturday, boutID: j.BoutID, kind: j.Kind, state: j.State, terminalDigest: c.Digest}
							ok = j.JobID == r.Job.JobID && j.View == r.Job.View && j.StoreID == r.Job.StoreID
						} else if c.Request.Command == "bout_job" || c.Request.Command == "bout_cancel" {
							j := r.Job
							want = boutJobExpectation{cardID: j.CardID, saturday: j.Saturday, boutID: j.BoutID, kind: j.Kind}
						} else {
							ev, e := gymevent.EventForDate(a.pin.SourcePin.SourcePin.SourcePin, c.Request.Saturday)
							if e != nil {
								ok = false
							} else {
								key, e := gymcard.CardID(a.pin.SourcePin, ev)
								ok = e == nil
								want = boutJobExpectation{cardID: key, saturday: c.Request.Saturday, boutID: c.Request.BoutID, kind: "run"}
							}
						}
						ok = ok && b.correlateBoutJob(c.Request, *r.Job, want, a, v)
					}
				}
			} else if ok && r.View != nil {
				ok = *r.View == v
			}
			if ok != c.Accept {
				t.Fatalf("accept%v want%v parse=%v request=%v", ok, c.Accept, pe, qe)
			}
		})
	}
}
