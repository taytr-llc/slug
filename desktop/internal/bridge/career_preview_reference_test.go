// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/internal/careerstate"
	"github.com/taytr-llc/slug/wire/v2"
	"os"
	"sync"
	"testing"
)

const careerPreviewCorpusSHA = "70cec03c5ef89765b5b929f41b4fd4314f5dd3a4dcbbae05521fbffc6cbddc8a"

type careerPreviewCorpus struct {
	DTOs      []struct{ Kind, Name, Canonical string } `json:"dtos"`
	Protocols []struct {
		Name      string
		Request   pilot.CareerPreviewRequest
		Response  json.RawMessage
		Current   *pilot.ManifestViewPin `json:"current_view"`
		Authority *struct {
			StoreID    string `json:"store_id"`
			Activation careerstate.Activation
		}
		Known    json.RawMessage
		Terminal *string `json:"terminal_digest"`
		Accept   bool
	} `json:"protocol_cases"`
	Portraits []struct {
		Name     string
		Response CareerPreviewPortraitResponse
		Current  pilot.ManifestViewPin `json:"current_view"`
		Known    struct {
			CapturedResultDigest string                `json:"captured_result_digest"`
			View                 pilot.ManifestViewPin `json:"view"`
			StoreID              string                `json:"store_id"`
			ActivationID         string                `json:"activation_id"`
			FighterAID           string                `json:"fighter_a_id"`
			FighterBID           string                `json:"fighter_b_id"`
			SVG                  string
			JobID                string `json:"job_id"`
		}
		Fresh  pilot.CareerPreviewJob `json:"fresh_job"`
		Accept bool
	} `json:"portrait_protocol_cases"`
}

var careerPreviewCorpusOnce sync.Once
var careerPreviewCorpusValue careerPreviewCorpus
var careerPreviewCorpusErr error

func careerPreviewReference(t *testing.T) careerPreviewCorpus {
	t.Helper()
	if wire.TestPoW {
		t.Skip("independent normal candidate fixture profile")
	}
	careerPreviewCorpusOnce.Do(func() {
		raw, e := os.ReadFile("../../../gymstable/testdata/career-preview-reference/corpus.json")
		if e != nil {
			careerPreviewCorpusErr = e
			return
		}
		h := sha256.Sum256(raw)
		if hex.EncodeToString(h[:]) != careerPreviewCorpusSHA {
			careerPreviewCorpusErr = os.ErrInvalid
			return
		}
		careerPreviewCorpusErr = json.Unmarshal(raw, &careerPreviewCorpusValue)
	})
	if careerPreviewCorpusErr != nil {
		t.Fatal(careerPreviewCorpusErr)
	}
	return careerPreviewCorpusValue
}
func careerPreviewFixture(t *testing.T, name string) (pilot.CareerPreviewJob, pilot.CareerPreviewSnapshot) {
	t.Helper()
	var j pilot.CareerPreviewJob
	var s pilot.CareerPreviewSnapshot
	for _, d := range careerPreviewReference(t).DTOs {
		if d.Name != name {
			continue
		}
		if d.Kind == "CareerPreviewJob" {
			if e := json.Unmarshal([]byte(d.Canonical), &j); e != nil {
				t.Fatal(e)
			}
		}
		if d.Kind == "CareerPreviewSnapshot" {
			if e := json.Unmarshal([]byte(d.Canonical), &s); e != nil {
				t.Fatal(e)
			}
		}
	}
	if j.Result == nil || s.Activation == nil {
		t.Fatal("missing independently frozen fixture", name)
	}
	return j, s
}

// This supplies modeled, already-correlated parent facts; it opens no store or
// real authority and is not a replacement for actual parent acceptance.
func careerPreviewModelBridge(j pilot.CareerPreviewJob) *StableBridge {
	v, p, a := j.View, j.Result.Pair.Status.Pin, j.Result.Pair.Activation
	return &StableBridge{careersEnabled: true, view: &v, manifestView: &v, storeID: j.StoreID, stablePin: &p, careerActivation: &a, careerPreviewReady: true, resultPin: &resultAuthority{storeID: a.ResultStoreID, createdMS: a.ResultCreatedAtMS, sourceCreatedMS: a.SourceCreatedAtMS}, result: &resultCorrelation{view: v}}
}
func TestCareerPreviewFrozenProtocolCorrelation(t *testing.T) {
	all := careerPreviewReference(t)
	if len(all.Protocols) != 35 {
		t.Fatal(len(all.Protocols))
	}
	for _, c := range all.Protocols {
		t.Run(c.Name, func(t *testing.T) {
			j, _ := careerPreviewFixture(t, "xp1-versus-zero")
			if c.Current != nil && c.Current.AccountID != j.View.AccountID {
				j, _ = careerPreviewFixture(t, "standard-draw-xp")
			}
			b := careerPreviewModelBridge(j)
			if c.Current == nil {
				b.view = nil
				b.manifestView = nil
			} else {
				v := *c.Current
				b.view = &v
				b.manifestView = &v
			}
			if c.Authority != nil {
				a := c.Authority.Activation
				b.careerActivation = &a
				b.storeID = c.Authority.StoreID
				b.resultPin = &resultAuthority{storeID: a.ResultStoreID, createdMS: a.ResultCreatedAtMS, sourceCreatedMS: a.SourceCreatedAtMS}
				if c.Current != nil {
					b.result = &resultCorrelation{view: *c.Current}
				}
			}
			if len(c.Known) > 0 && string(c.Known) != "null" {
				var known struct {
					Snapshot *pilot.CareerPreviewSnapshot `json:"snapshot"`
				}
				if e := json.Unmarshal(c.Known, &known); e != nil {
					t.Fatal(e)
				}
				if known.Snapshot != nil {
					b.rememberCareerPreviewSnapshot(*known.Snapshot)
				} else {
					var k pilot.CareerPreviewJob
					if e := json.Unmarshal(c.Known, &k); e != nil {
						t.Fatal(e)
					}
					if !b.rememberCareerPreviewJob(k) {
						t.Fatal("fixture known job")
					}
				}
			}
			r, e := pilot.ParseCareerPreviewResponse(c.Response)
			got := e == nil && b.correlateCareerPreview(c.Request, r)
			if got != c.Accept {
				t.Fatalf("correlation=%v want=%v parse=%v", got, c.Accept, e)
			}
		})
	}
}
func TestCareerPreviewSharedSelectionAndMetadata(t *testing.T) {
	j, _ := careerPreviewFixture(t, "xp1-versus-zero")
	b := careerPreviewModelBridge(j)
	b.selectPreview(careerPreviewFamily, j.JobID)
	if !b.rememberCareerPreviewJob(j) {
		t.Fatal("initial")
	}
	old := b.presentation
	b.selectPreview(legacyPreviewFamily, "newer")
	marker := &previewPresentation{metadata: previewPresentationMetadata{JobID: "newer"}}
	b.presentation = marker
	if !b.rememberCareerPreviewJob(j) || b.presentation != marker {
		t.Fatal("old Career terminal replaced newer legacy selection")
	}
	b.selectPreview(careerPreviewFamily, "next-career")
	legacy := previewTransportFixtures(t)[0].Response.Job
	if !b.rememberPreviewJob(*legacy) || b.presentation != nil {
		t.Fatal("old legacy terminal replaced Career selection")
	}
	b.previewSelection = nil
	b.previewJobs = nil
	b.careerPreviewJobs = nil
	if !b.rememberCareerPreviewJob(j) || b.presentation == nil || b.presentation == old {
		t.Fatal("owned recovery")
	}
	if !b.previewMetadataBound() {
		t.Fatal("bounded fixture metadata")
	}
	b.previewJobs = nil
	b.careerPreviewJobs = make(map[string]careerPreviewJobExpectation)
	for n := 0; n < 4; n++ {
		b.careerPreviewJobs[string(rune('a'+n))] = careerPreviewJobExpectation{state: "running"}
	}
	if b.previewMetadataBound() || b.canTrackPreview() {
		t.Fatal("correlation capacity widened")
	}
}
func TestCareerPreviewActivationRequiresEstablishedResult(t *testing.T) {
	j, s := careerPreviewFixture(t, "xp1-versus-zero")
	b := careerPreviewModelBridge(j)
	b.careerActivation = nil
	b.resultPin = nil
	if b.correlateCareerPreview(pilot.CareerPreviewRequest{Command: "career_preview_snapshot"}, pilot.CareerPreviewResponse{Snapshot: &s}) {
		t.Fatal("self-asserted activation accepted")
	}
	b = careerPreviewModelBridge(j)
	a := *s.Activation
	a.ResultCreatedAtMS--
	s.Activation = &a
	if b.correlateCareerPreview(pilot.CareerPreviewRequest{Command: "career_preview_snapshot"}, pilot.CareerPreviewResponse{Snapshot: &s}) {
		t.Fatal("changed source incarnation")
	}
}

func TestCareerPreviewFrozenPortraitCorrelation(t *testing.T) {
	all := careerPreviewReference(t)
	if len(all.Portraits) != 34 {
		t.Fatal(len(all.Portraits))
	}
	for _, c := range all.Portraits {
		t.Run(c.Name, func(t *testing.T) {
			name := "xp1-versus-zero"
			if len(c.Name) >= 17 && c.Name[:17] == "standard-draw-xp/" {
				name = "standard-draw-xp"
			}
			j, _ := careerPreviewFixture(t, name)
			b := careerPreviewModelBridge(j)
			if !b.rememberCareerPreviewJob(j) {
				t.Fatal("fixture")
			}
			meta := b.presentation.metadata
			meta.View = c.Known.View
			meta.StoreID = c.Known.StoreID
			meta.JobID = c.Known.JobID
			meta.ActivationID = c.Known.ActivationID
			meta.Members[0].ID = c.Known.FighterAID
			meta.Members[1].ID = c.Known.FighterBID
			meta.CapturedResultDigest = c.Known.CapturedResultDigest
			got := careerPreviewPortraitFresh(meta, c.Current, &c.Fresh) && careerPreviewPortraitMatches(c.Response, meta) && c.Response.Portrait.SVG == c.Known.SVG
			if got != c.Accept {
				t.Fatalf("portrait correlation=%v want=%v", got, c.Accept)
			}
		})
	}
}
