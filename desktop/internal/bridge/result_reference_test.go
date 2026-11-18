// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymresult"
	"github.com/taytr-llc/slug/wire/v2"
	"os"
	"strings"
	"testing"
)

type resultProtocolCase struct {
	Category, Name string
	View           *pilot.ManifestViewPin `json:"current_view"`
	Pin            gymresult.Pin          `json:"authority_pin"`
	SourceCreated  int64                  `json:"source_created_at_ms"`
	Created        int64                  `json:"created_at_ms"`
	Request        pilot.ResultRequest
	Response       json.RawMessage
	Expected       *pilot.ResultJob `json:"expected_job"`
	Digest         string           `json:"terminal_digest"`
	Source         *struct {
		Record string `json:"source_record_digest"`
		Input  string `json:"input_id"`
		Result string `json:"result_digest"`
	} `json:"expected_source"`
	Known *struct {
		Statement  string `json:"statement_id"`
		TxID       string `json:"txid"`
		Raw        string `json:"raw_tx"`
		Checkpoint *chaincfg.SignedCheckpoint
		Finality   *gymresult.Finality
	} `json:"known_publication"`
	Accept bool
}

func resultProtocolCases(t *testing.T) []resultProtocolCase {
	t.Helper()
	raw, e := os.ReadFile("../../../gymresult/testdata/reference/fragments/wire.json")
	if e != nil {
		t.Fatal(e)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "aa60fdb0267a77365039d95545b61e8a4adb256bad8366325d89fdf081611408" {
		t.Fatal("frozen wire fragment changed")
	}
	if wire.TestPoW {
		t.Skip("normal profile reference")
	}
	var doc struct{ Cases []resultProtocolCase }
	if e = json.Unmarshal(raw, &doc); e != nil {
		t.Fatal(e)
	}
	var out []resultProtocolCase
	for _, c := range doc.Cases {
		if c.Category == "protocol" {
			out = append(out, c)
		}
	}
	if len(out) != 138 {
		t.Fatal("protocol count", len(out))
	}
	return out
}
func TestResultBridgeIndependentProtocol(t *testing.T) {
	for _, c := range resultProtocolCases(t) {
		t.Run(c.Name, func(t *testing.T) {
			id, e := gymresult.ID(c.Pin)
			if e != nil {
				t.Fatal(e)
			}
			a := resultAuthority{pin: c.Pin, storeID: id, createdMS: c.Created, sourceCreatedMS: c.SourceCreated}
			b := &StableBridge{view: c.View, manifestView: c.View, resultsEnabled: true, boutsEnabled: true, resultPin: &a}
			if c.View != nil {
				v := *c.View
				b.result = &resultCorrelation{view: v, jobs: map[string]resultJobExpectation{}}
				b.boutPin = &boutAuthority{pin: c.Pin.SourcePin, storeID: c.Pin.SourceStoreID, createdMS: c.SourceCreated}
				b.bout = &boutCorrelation{view: v, jobs: map[string]boutJobExpectation{}}
			}
			var known resultCommitments
			if k := c.Known; k != nil {
				known.statement = k.Statement
				known.txid = k.TxID
				known.raw = resultDigest(k.Raw)
				if k.Checkpoint != nil {
					known.checkpoint = resultDigest(k.Checkpoint)
				}
				if k.Finality != nil {
					known.finality = resultDigest(k.Finality)
					known.finalizedMS = k.Finality.ObservedAtMS
				}
			}
			_, qe := pilot.MarshalResultRequest(c.Request)
			var buf bytes.Buffer
			if e = json.Compact(&buf, c.Response); e != nil {
				t.Fatal(e)
			}
			r, pe := pilot.ParseResultResponse(buf.Bytes())
			ok := qe == nil && pe == nil && r.ID == c.Request.ID
			if ok && c.View == nil {
				ok = c.Request.Command == "result_snapshot" && r.OK && r.Snapshot != nil && r.Snapshot.State == "unavailable" && b.rememberResultSnapshot(r)
			} else if ok {
				v := *c.View
				ok = c.Request.Command == "result_snapshot" || c.Request.ViewID == v.ViewID
				if ok && r.OK {
					if c.Request.Command == "result_snapshot" {
						if r.Snapshot != nil && r.Snapshot.State == "setup_incomplete" {
							b.resultPin = nil
						}
						ok = b.rememberResultSnapshot(r)
					} else {
						ok = b.correlateResultEnvelope(c.Request, r, a, v)
						if ok && r.Status != nil {
							ok = known.matchesStatus(*r.Status)
						}
						if ok && r.Record != nil {
							ok = known.matches(r.Record.Record) && known.matchesStatus(r.Record.Status)
						}
						if ok && r.Job != nil {
							j := r.Job
							want := resultJobExpectation{cardID: j.CardID, saturday: j.Saturday, boutID: j.BoutID, kind: j.Kind, known: known}
							if c.Expected != nil {
								x := c.Expected
								want = resultJobExpectation{cardID: x.CardID, saturday: x.Saturday, boutID: x.BoutID, kind: x.Kind, state: x.State, terminalDigest: c.Digest, known: known}
								ok = x.JobID == j.JobID && x.View == j.View && x.StoreID == j.StoreID && x.ResultKey == j.ResultKey
							} else if resultBegin(c.Request.Command) {
								ev, err := gymevent.EventForDate(a.pin.SourcePin.SourcePin.SourcePin.SourcePin, c.Request.Saturday)
								if err != nil {
									ok = false
								} else {
									key, err := gymcard.CardID(a.pin.SourcePin.SourcePin, ev)
									ok = err == nil
									want.cardID = key
									want.saturday = c.Request.Saturday
									want.boutID = c.Request.BoutID
									want.kind = strings.TrimPrefix(c.Request.Command, "result_")
								}
							}
							// Frozen expected_source carries three immutable saved commitments. The
							// live bridge captures the complete source tuple from actual BoutStatus.
							if c.Source != nil {
								if j.Result != nil {
									want.source = resultSourceFrom(j.Result.Status.Source)
								} else if j.Export != nil {
									want.source = resultSourceExpectation{set: true}
								}
								want.source.record = c.Source.Record
								want.source.input = c.Source.Input
								want.source.result = c.Source.Result
							}
							ok = ok && b.correlateResultJob(c.Request, *j, want, a, v)
							if ok && c.Digest != "" {
								ok = resultJobDigest(*j) == c.Digest
							}
						}
						if ok && c.Source != nil {
							var st *gymresult.Status
							if r.Status != nil {
								st = r.Status
							}
							if r.Record != nil {
								st = &r.Record.Status
							}
							if r.Job != nil && r.Job.Result != nil {
								st = &r.Job.Result.Status
							}
							if st != nil && st.Exists {
								ok = st.SourceRecordDigest == c.Source.Record && st.InputID == c.Source.Input && st.ResultDigest == c.Source.Result
							}
						}
					}
				}
			}
			if ok != c.Accept {
				t.Fatalf("accept %v want %v parse=%v request=%v", ok, c.Accept, pe, qe)
			}
		})
	}
}

// Keep this import tied to the concrete source DTO in the frozen correlation
// rather than silently substituting a result response as a new authority.
var _ gymbout.Status
