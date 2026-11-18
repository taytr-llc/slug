// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"encoding/json"
	"github.com/taytr-llc/slug/fighterbout"
)

func previewErrorCode(s string) bool {
	return stableOne(s, "invalid_request", "unauthorized", "forbidden", "stale_view", "busy", "stable_corrupt", "stable_clock", "stable_conflict", "stable_sync_required", "outcome_unknown", "preview_not_found", "preview_class_mismatch", "preview_capacity", "preview_cancelled", "preview_unavailable", "preview_observation_unknown")
}
func previewSnapshotValid(s PreviewSnapshot) bool {
	if s.Version != 1 {
		return false
	}
	if s.State == "unavailable" {
		return s.Error == "" && s.View == nil && s.StoreID == "" && s.EngineVersion == 0 && s.EngineID == "" && s.ActiveJobID == "" && s.QueuedJobID == "" && s.LastJobID == ""
	}
	if s.View == nil || !stableViewValid(*s.View) || !canonicalManifestID(s.StoreID) || s.EngineVersion != 1 || s.EngineID != previewEngineID || !stableOptionalID(s.ActiveJobID) || !stableOptionalID(s.QueuedJobID) || !stableOptionalID(s.LastJobID) || s.ActiveJobID != "" && s.QueuedJobID != "" {
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
		return previewTerminalError(s.Error) && s.ActiveJobID == "" && s.QueuedJobID == "" && s.LastJobID != "" && (s.State != "cancelled" || s.Error == "preview_cancelled")
	}
	return false
}
func previewJobValid(j PreviewJob) bool {
	if j.Version != 1 || !canonicalManifestID(j.JobID) || !canonicalManifestID(j.StoreID) || !stableViewValid(j.View) || j.EngineVersion != 1 || j.EngineID != previewEngineID || !canonicalManifestID(j.FighterAID) || !canonicalManifestID(j.FighterBID) || j.FighterAID >= j.FighterBID {
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
			return previewTerminalError(j.Error)
		}
		return false
	}
	if j.Error != "" || j.Result == nil {
		return false
	}
	r := j.Result
	if r.Version != 1 || !previewPairValid(r.Pair, j.View, j.StoreID) || r.Pair.Fighters[0].Assignment.ProspectID != j.FighterAID || r.Pair.Fighters[1].Assignment.ProspectID != j.FighterBID {
		return false
	}
	recipe, e := previewRecipe(r.Pair)
	if e != nil || recipe != r.Recipe {
		return false
	}
	seed, e := PreviewSeed(recipe)
	if e != nil || seed != r.Seed {
		return false
	}
	input, e := previewInput(r.Pair, seed)
	if e != nil {
		return false
	}
	id, e := fighterbout.InputID(input)
	if e != nil || id != r.EngineResult.InputID || !previewEngineShape(r.EngineResult, j.FighterAID, j.FighterBID) {
		return false
	}
	raw, e := json.Marshal(r.EngineResult)
	if e != nil || len(raw) > MaxPreviewEngineSize || previewHash("SLUG fighter bout result\x00v1\x00", raw, false) != r.EngineResultDigest {
		return false
	}
	raw, e = json.Marshal(r)
	if e != nil || len(raw) > MaxPreviewResultSize {
		return false
	}
	copy := j
	copy.Result = nil
	raw, e = json.Marshal(copy)
	return e == nil && len(raw) <= 8192
}
func previewStateValid(s fighterbout.State) bool {
	return s.StaminaQ >= 0 && s.StaminaQ <= 214000000 && s.DamageTakenQ >= 0 && s.DamageTakenQ < 2700000000 && s.Thrown <= 180 && s.Landed <= s.Thrown && s.Significant <= s.Landed && s.DamageDealtQ >= 0 && s.DamageDealtQ < 2700000000 && s.Knockdowns <= 180 && s.ControlQ >= 0 && s.ControlQ <= 630000000 && s.RoundKnockdowns <= 3
}

// This checks bounded transport structure and continuity, not fight math. Only
// the helper's tracked execution calls the authoritative simulation validator.
func previewEngineShape(r fighterbout.Result, a, b string) bool {
	if r.Version != 1 || r.EngineID != previewEngineID || !canonicalManifestID(r.InputID) || len(r.Rounds) < 1 || len(r.Rounds) > 3 || r.EndRound != uint32(len(r.Rounds)) || r.EndTick < 1 || r.EndTick > 60 {
		return false
	}
	draw := r.Method == "decision_draw" || r.Method == "double_corner_stoppage"
	if !stableOne(r.Method, "decision", "decision_draw", "ko", "tko_three_knockdowns", "tko_referee", "tko_corner", "double_corner_stoppage") || draw != (r.Winner == "") || !draw && r.Winner != a && r.Winner != b {
		return false
	}
	if stableOne(r.Method, "decision", "decision_draw") && (r.EndRound != 3 || r.EndTick != 60) {
		return false
	}
	if stableOne(r.Method, "tko_corner", "double_corner_stoppage") && (r.EndRound >= 3 || r.EndTick != 60) {
		return false
	}
	for _, i := range r.Initial {
		q := i.Effective
		if i.FormQ < -100000 || i.FormQ > 100000 || i.JitterQ < 920000 || i.JitterQ > 1080000 || q.Power < 720000 || q.Power > 110000000 || q.Speed < 540000 || q.Speed > 110000000 || q.Chin < 630000 || q.Chin > 110000000 || q.Stamina < 630000 || q.Stamina > 110000000 || q.Skill < 810000 || q.Skill > 130000000 || i.MaxStaminaQ < 60000000 || i.MaxStaminaQ > 214000000 || i.KOThresholdQ < 64000000 || i.KOThresholdQ > 130000000 {
			return false
		}
	}
	ticks := 0
	for n, round := range r.Rounds {
		last := n == len(r.Rounds)-1
		if round.Number != uint32(n+1) || len(round.Ticks) < 1 || len(round.Ticks) > 60 || !last && len(round.Ticks) != 60 {
			return false
		}
		ticks += len(round.Ticks)
		for k, t := range round.Ticks {
			if t.Number != uint32(k+1) || t.Actor > 1 || !stableOne(t.Strike, "jab", "cross", "hook", "uppercut", "body") || t.DamageQ < 0 || t.DamageQ >= 9000000 || !stableOne(t.Stoppage, "", "ko", "tko_three_knockdowns", "tko_referee") || !t.Hit && (t.DamageQ != 0 || t.Significant || t.Knockdown || t.Stoppage != "") || t.Knockdown && !t.Significant {
				return false
			}
			if t.Stoppage != "" && (!last || k != len(round.Ticks)-1 || t.Stoppage != r.Method) {
				return false
			}
			for _, s := range t.States {
				if !previewStateValid(s) {
					return false
				}
			}
		}
		finalTick := round.Ticks[len(round.Ticks)-1]
		if last && stableOne(r.Method, "ko", "tko_three_knockdowns", "tko_referee") && (finalTick.Stoppage != r.Method || !finalTick.Significant || !finalTick.Hit || finalTick.Actor == 0 && r.Winner != a || finalTick.Actor == 1 && r.Winner != b) {
			return false
		}
		if round.End != finalTick.States {
			return false
		}
		needRecovery := len(round.Ticks) == 60 && round.Number < 3 && finalTick.Stoppage == ""
		if needRecovery != (round.Recovery != nil) {
			return false
		}
		if round.Recovery != nil {
			for _, s := range round.Recovery.States {
				if !previewStateValid(s) {
					return false
				}
			}
			stopped := round.Recovery.CornerStopped
			if !last && (stopped[0] || stopped[1]) {
				return false
			}
			if last {
				if r.Method == "double_corner_stoppage" {
					if !stopped[0] || !stopped[1] {
						return false
					}
				} else if r.Method == "tko_corner" {
					if stopped[0] == stopped[1] || stopped[0] && r.Winner != b || stopped[1] && r.Winner != a {
						return false
					}
				} else {
					return false
				}
			}
		}
		if last {
			if r.EndTick != uint32(len(round.Ticks)) {
				return false
			}
			end := round.End
			if round.Recovery != nil {
				end = round.Recovery.States
			}
			if end != r.Final {
				return false
			}
		}
	}
	if ticks > 180 {
		return false
	}
	for i, s := range r.Scores {
		final := r.Final[i]
		accuracy := uint32(0)
		if final.Thrown != 0 {
			accuracy = uint32(previewRound(250*int64(final.Landed), int64(final.Thrown)))
		}
		if s.DamageTenths != uint32(previewRound(final.DamageDealtQ, 100000)) || s.KnockdownTenths != 100*final.Knockdowns || s.AccuracyTenths != accuracy || s.VolumeTenths != 6*final.Significant || s.ControlTenths != uint32(previewRound(7*final.ControlQ, 2000000)) {
			return false
		}
		if s.DamageTenths >= 27000 || s.KnockdownTenths > 18000 || s.AccuracyTenths > 250 || s.VolumeTenths > 1080 || s.ControlTenths > 2205 || s.TotalTenths != s.DamageTenths+s.KnockdownTenths+s.AccuracyTenths+s.VolumeTenths+s.ControlTenths {
			return false
		}
	}
	if r.Method == "decision_draw" && r.Scores[0].TotalTenths != r.Scores[1].TotalTenths {
		return false
	}
	if r.Method == "decision" {
		if r.Scores[0].TotalTenths == r.Scores[1].TotalTenths || r.Scores[0].TotalTenths > r.Scores[1].TotalTenths && r.Winner != a || r.Scores[1].TotalTenths > r.Scores[0].TotalTenths && r.Winner != b {
			return false
		}
	}
	d := r.DrawCounts
	if uint64(d.FormA)+uint64(d.FormB)+uint64(d.ChinA)+uint64(d.ChinB)+uint64(d.Combat) > 65536 {
		return false
	}
	return d.FormA >= 1 && d.FormA <= 65536 && d.FormB >= 1 && d.FormB <= 65536 && d.ChinA >= 1 && d.ChinA <= 65536 && d.ChinB >= 1 && d.ChinB <= 65536 && d.Combat >= 1 && d.Combat <= 65536
}

// Inputs are nonnegative and bounded by previewStateValid.
func previewRound(n, d int64) int64 {
	q, r := n/d, n%d
	if 2*r > d || 2*r == d && q%2 == 1 {
		q++
	}
	return q
}

func previewTerminalError(s string) bool { return s != "outcome_unknown" && previewErrorCode(s) }
