// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"encoding/json"

	"github.com/taytr-llc/slug/fighterbout"
	"github.com/taytr-llc/slug/internal/careerstate"
)

func careerPreviewErrorCode(s string) bool {
	return previewErrorCode(s) || stableOne(s, "career_invalid", "career_inactive", "career_unfenced", "career_closed", "career_poisoned", "career_cancelled", "career_corrupt")
}
func careerPreviewUnavailableError(s string) bool {
	return stableOne(s, "", "preview_unavailable", "career_inactive", "career_unfenced", "career_closed", "career_poisoned", "career_corrupt")
}
func careerPreviewTerminalError(s string) bool {
	return s != "outcome_unknown" && careerPreviewErrorCode(s)
}

func careerPreviewSnapshotValid(s CareerPreviewSnapshot) bool {
	if s.Version != 1 || !careerPreviewBounded(s, 4096) {
		return false
	}
	if s.State == "unavailable" {
		return careerPreviewUnavailableError(s.Error) && s.View == nil && s.Activation == nil && s.StoreID == "" && s.EngineVersion == 0 && s.EngineID == "" && s.ActiveJobID == "" && s.QueuedJobID == "" && s.LastJobID == ""
	}
	if s.View == nil || !stableViewValid(*s.View) || !careerPreviewBounded(*s.View, 1024) || s.Activation == nil || careerstate.ValidateActivation(*s.Activation) != nil || !careerPreviewBounded(*s.Activation, 512) || !canonicalManifestID(s.StoreID) || s.EngineVersion != 1 || s.EngineID != careerPreviewEngineID || !stableOptionalID(s.ActiveJobID) || !stableOptionalID(s.QueuedJobID) || !stableOptionalID(s.LastJobID) || s.ActiveJobID != "" && s.QueuedJobID != "" {
		return false
	}
	switch s.State {
	case "idle":
		return s.Error == "" && s.ActiveJobID == "" && s.QueuedJobID == "" && s.LastJobID == ""
	case "busy":
		return s.Error == "" && (s.ActiveJobID != "" || s.QueuedJobID != "") && s.LastJobID == ""
	case "ready":
		return s.Error == "" && s.ActiveJobID == "" && s.QueuedJobID == "" && s.LastJobID != ""
	case "failed", "cancelled":
		return careerPreviewTerminalError(s.Error) && s.ActiveJobID == "" && s.QueuedJobID == "" && s.LastJobID != "" && (s.State != "cancelled" || s.Error == "preview_cancelled")
	}
	return false
}

func careerPreviewJobValid(j CareerPreviewJob) bool {
	if j.Version != 1 || !canonicalManifestID(j.JobID) || !canonicalManifestID(j.StoreID) || !stableViewValid(j.View) || !careerPreviewBounded(j.View, 1024) || j.EngineVersion != 1 || j.EngineID != careerPreviewEngineID || !canonicalManifestID(j.ActivationID) || !canonicalManifestID(j.FighterAID) || !canonicalManifestID(j.FighterBID) || j.FighterAID >= j.FighterBID {
		return false
	}
	metadata := j
	metadata.Result = nil
	if !careerPreviewBounded(metadata, 2048) || !careerPreviewBounded(j, 231424) {
		return false
	}
	if j.State != "done" {
		if j.Result != nil {
			return false
		}
		switch j.State {
		case "queued", "running":
			return j.Error == ""
		case "cancelled":
			return j.Error == "preview_cancelled"
		case "failed":
			return careerPreviewTerminalError(j.Error)
		}
		return false
	}
	if j.Error != "" || j.Result == nil {
		return false
	}
	r := j.Result
	if r.Version != 1 || !careerPreviewPairValid(r.Pair, j.View, j.StoreID) || r.Pair.Activation.ActivationID != j.ActivationID || r.Pair.Fighters[0].Assignment.ProspectID != j.FighterAID || r.Pair.Fighters[1].Assignment.ProspectID != j.FighterBID {
		return false
	}
	recipe, e := careerPreviewRecipe(r.Pair)
	if e != nil || recipe != r.Recipe {
		return false
	}
	seed, e := CareerPreviewSeed(recipe)
	if e != nil || seed != r.Seed {
		return false
	}
	input, e := careerPreviewInput(r.Pair, seed)
	if e != nil {
		return false
	}
	id, e := fighterbout.InputID(input)
	if e != nil || id != r.EngineResult.InputID || !previewEngineShape(r.EngineResult, j.FighterAID, j.FighterBID) {
		return false
	}
	// Transport validates bounded shape, input identity and exact transcript
	// bytes. Only the explicit work job calls the engine's full Digest checker.
	raw, e := json.Marshal(r.EngineResult)
	if e != nil || len(raw) > MaxCareerPreviewEngineSize || previewHash("SLUG fighter bout result\x00v1\x00", raw, false) != r.EngineResultDigest {
		return false
	}
	return careerPreviewBounded(r, MaxCareerPreviewResultSize)
}
