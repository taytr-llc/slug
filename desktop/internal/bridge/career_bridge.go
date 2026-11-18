// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os/exec"
	"time"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/internal/careerstate"
)

func LaunchCareers(helper, dataDir, node string) (*StableBridge, error) {
	b, e := startStable(exec.Command(helper, "--candidate-manifests", "--candidate-stable", "--candidate-entries", "--candidate-events", "--candidate-seeds", "--candidate-cards", "--candidate-bouts", "--candidate-results", "--career", "--data-dir", dataDir, "--node-binary", node))
	if e == nil {
		b.entriesEnabled, b.eventsEnabled, b.seedsEnabled, b.cardsEnabled, b.boutsEnabled, b.resultsEnabled, b.careersEnabled = true, true, true, true, true, true, true
	}
	return b, e
}
func careerFailure(id uint64, code string) pilot.CareerResponse {
	if id == 0 {
		id = 1
	}
	return pilot.CareerResponse{ID: id, Error: code}
}
func (b *StableBridge) CareerRequest(ctx context.Context, req pilot.CareerRequest) pilot.CareerResponse {
	if !b.careersEnabled {
		return careerFailure(0, "career_inactive")
	}
	pre := req
	pre.ID = 1
	if _, e := pilot.MarshalCareerRequest(pre); e != nil {
		return careerFailure(0, "invalid_request")
	}
	// Discovery binds the new activation to an independently correlated original
	// Result source. It cannot be supplied by the page or webview.
	a, view, ready := b.resultContext()
	if !ready {
		r := b.ResultRequest(ctx, pilot.ResultRequest{Command: "result_snapshot"})
		if !r.OK {
			code := r.Error
			if !oneOf(code, "unauthorized", "forbidden", "stale_view", "busy", "outcome_unknown") {
				code = "career_inactive"
			}
			return careerFailure(0, code)
		}
		a, view, ready = b.resultContext()
		if !ready {
			return careerFailure(0, b.careerSetupError(req.ViewID, r.Snapshot))
		}
	}
	if view.ViewID != req.ViewID {
		return careerFailure(0, "stale_view")
	}
	id, e := b.begin(ctx)
	if e != nil {
		if errors.Is(e, ErrBusy) {
			return careerFailure(0, "busy")
		}
		return careerFailure(0, "stable_unavailable")
	}
	defer func() { <-b.gate }()
	b.mu.Lock()
	selected := b.careerSelection
	if selected == nil || !selected.ready || selected.jobID != req.JobID || selected.view != view || selected.roster != (req.Command == "career_roster") || selected.after != req.After || selected.first != req.FirstSlot || selected.limit != req.Limit {
		b.mu.Unlock()
		return careerFailure(id, "stable_not_found")
	}
	expected := *selected
	b.mu.Unlock()
	req.ID = id
	raw, e := pilot.MarshalCareerRequest(req)
	if e != nil {
		return careerFailure(id, "invalid_request")
	}
	c, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	frame, e := b.exchangeCareerFamily(c, id, false, false, false, false, false, false, false, false, true, raw)
	if e != nil || frame.career == nil {
		return careerFailure(id, "outcome_unknown")
	}
	r := *frame.career
	if !r.OK {
		return r
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if r.JobID != req.JobID || b.careerSelection == nil || *b.careerSelection != expected || b.view == nil || *b.view != view || r.View == nil || *r.View != view || r.StoreID != b.storeID || b.stablePin == nil {
		return careerFailure(id, "outcome_unknown")
	}
	var activation careerstate.Activation
	if req.Command == "career_inventory" {
		if r.Page == nil || r.Roster != nil || r.Page.Source.After != req.After || len(r.Page.Items) > int(req.Limit) || r.Page.Source.Status.Pin != *b.stablePin {
			return careerFailure(id, "outcome_unknown")
		}
		if careerSourceDigest(r.Page.Source) != expected.sourceDigest {
			return careerFailure(id, "outcome_unknown")
		}
		activation = r.Page.Activation
	} else {
		end := req.FirstSlot + req.Limit
		if end > 10 {
			end = 0
		}
		if r.Roster == nil || r.Page != nil || r.Roster.Source.FirstSlot != req.FirstSlot || r.Roster.Source.NextSlot != end || len(r.Roster.Items) > int(req.Limit) || r.Roster.Source.Status.Pin != *b.stablePin {
			return careerFailure(id, "outcome_unknown")
		}
		if careerSourceDigest(r.Roster.Source) != expected.sourceDigest {
			return careerFailure(id, "outcome_unknown")
		}
		activation = r.Roster.Activation
	}
	if activation.ResultStoreID != a.storeID || activation.ResultCreatedAtMS != a.createdMS || activation.SourceCreatedAtMS != a.sourceCreatedMS || b.careerActivation != nil && *b.careerActivation != activation {
		return careerFailure(id, "outcome_unknown")
	}
	// Only fixed, tiny lifetime metadata is retained; page/transcript payloads are not.
	b.careerActivation = &activation
	return r
}

// CareerEnabled is an immutable launcher choice, not result-derived authority.
func (b *StableBridge) CareerEnabled() bool { return b != nil && b.careersEnabled }

// Result discovery validates the full original View before accepting setup.
// Both parent waiting phases have the same closed setup_incomplete snapshot.
func (b *StableBridge) careerSetupError(viewID string, snapshot *pilot.ResultSnapshot) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failure != nil || b.view == nil || b.view.ViewID != viewID || snapshot == nil || snapshot.State != "setup_incomplete" || snapshot.View == nil || *snapshot.View != *b.view {
		return "stale_view"
	}
	return "career_inactive"
}

type careerPageExpectation struct {
	jobID, sourceDigest string
	view                pilot.ManifestViewPin
	roster, ready       bool
	after               uint64
	first, limit        uint32
}

func careerSourceDigest(source any) string {
	raw, e := json.Marshal(source)
	if e != nil {
		return ""
	}
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:])
}

// Called only after the existing Stable terminal/source correlation succeeds.
func (b *StableBridge) rememberCareerStableJob(req pilot.StableRequest, j pilot.StableJob) bool {
	if !b.careersEnabled {
		return true
	}
	original, ok := b.jobs[j.JobID]
	if !ok {
		return true
	} // Automatic Sync is not a Career page selection.
	b.mu.Lock()
	defer b.mu.Unlock()
	if stableStartsJob(req.Command) {
		b.careerSelection = &careerPageExpectation{jobID: j.JobID, view: j.View}
	}
	selected := b.careerSelection
	if selected == nil || selected.jobID != j.JobID || selected.view != j.View || j.State != "done" {
		return true
	}
	next := careerPageExpectation{jobID: j.JobID, view: j.View, limit: original.request.Limit}
	if j.Kind == "inventory" && j.Page != nil {
		if original.request.Command == "stable_recent" {
			next.limit = 8
		}
		next.after, next.sourceDigest = j.Page.After, careerSourceDigest(*j.Page)
	} else if j.Kind == "roster" && j.Roster != nil {
		next.roster, next.first, next.sourceDigest = true, j.Roster.FirstSlot, careerSourceDigest(*j.Roster)
	} else {
		return true
	}
	next.ready = next.sourceDigest != ""
	if selected.ready && *selected != next {
		return false
	} // Older/mutated terminal cannot replace selected commitments.
	b.careerSelection = &next
	return true
}
