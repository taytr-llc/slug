// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"github.com/taytr-llc/slug/fightercareer"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/internal/careerstate"
)

func historySize(v any, n int) bool { b, e := json.Marshal(v); return e == nil && len(b) <= n }
func careerHistoryErrorCode(s string) bool {
	return stableOne(s, "invalid_request", "unauthorized", "forbidden", "stale_view", "busy", "outcome_unknown", "career_inactive", "history_unavailable", "history_corrupt", "history_clock", "history_capacity", "history_not_found", "history_source_changed", "history_cancelled", "history_observation_unknown")
}
func historyTerminalError(s string) bool {
	return careerHistoryErrorCode(s) && s != "busy" && s != "outcome_unknown"
}
func historyUnavailableError(s string) bool {
	return stableOne(s, "history_unavailable", "career_inactive", "history_clock", "history_corrupt")
}
func historyTotals(t fightercareer.Totals, n uint32) bool {
	count := uint64(t.Wins) + uint64(t.Losses) + uint64(t.Draws)
	return count == uint64(n) && n <= 256 && t.XP == min(uint32(15), n) && t.BasePoints >= 10*t.Wins+4*t.Draws && t.BasePoints <= 14*t.Wins+4*t.Draws && (t.BasePoints-10*t.Wins-4*t.Draws)%4 == 0
}
func historyID(c gymstable.CareerHistoryCapture) string {
	h := sha256.New()
	h.Write([]byte("SLUG candidate fighter history\x00v1\x00"))
	for _, s := range []string{c.Status.StoreID, c.Activation.ActivationID, c.CareerDigest} {
		b, _ := hex.DecodeString(s)
		h.Write(b)
	}
	put := func(v uint32) { var b [4]byte; binary.LittleEndian.PutUint32(b[:], v); h.Write(b[:]) }
	lp := func(v any) { b, _ := json.Marshal(v); put(uint32(len(b))); h.Write(b) }
	lp(c.Assignment)
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], uint64(c.AsOfMS))
	h.Write(b[:])
	put(c.PrefixCount)
	put(c.EventCount)
	lp(c.Totals)
	return hex.EncodeToString(h.Sum(nil))
}
func historyCaptureValid(c gymstable.CareerHistoryCapture, v ManifestViewPin) bool {
	a := c.Assignment
	s := c.Status
	if c.Version != 1 || !stableStatusValid(s, v, s.StoreID) || careerstate.ValidateActivation(c.Activation) != nil || !canonicalManifestID(c.CareerDigest) || !stableTime(c.AsOfMS) || c.AsOfMS != s.LastAsOfMS || c.AsOfMS < c.Activation.ActivatedAtMS || c.AsOfMS < a.AcceptedAtMS || c.PrefixCount > 256 || c.EventCount > c.PrefixCount || !historyTotals(c.Totals, c.EventCount) {
		return false
	}
	if a.Version != 1 || a.Sequence == 0 || a.Sequence > s.Checkpoint || a.GymID != s.Pin.Identity.GymID || a.DescriptorID != s.Pin.Source.DescriptorID || !stableTime(a.BornAtMS) || a.AcceptedAtMS < a.BornAtMS || a.AcceptedAtMS-a.BornAtMS >= 300000 {
		return false
	}
	for _, id := range []string{a.ProspectID, a.GymID, a.WorkHash, a.DescriptorID, a.ManifestDigest, a.PortraitDigest, a.SourceRecordDigest} {
		if !canonicalManifestID(id) {
			return false
		}
	}
	return c.HistoryID == historyID(c) && historySize(c, 8192) && historySize(s, 4096) && historySize(a, 2048) && historySize(c.Activation, 512) && historySize(c.Totals, 128)
}
func historyRowValid(r gymstable.CareerHistoryRow, c gymstable.CareerHistoryCapture) bool {
	if r.Ordinal == 0 || r.Ordinal > c.EventCount || !historySize(r, 1536) || !historySize(r.Event, 1344) {
		return false
	}
	e := r.Event
	if _, err := careerstate.ValidateIndex(c.Activation, []careerstate.Event{e}); err != nil || e.EffectiveAtMS > c.AsOfMS || len(e.Saturday) != 10 || e.Saturday[:4] == "0000" {
		return false
	}
	winner := "draw"
	if e.Winner == e.CornerA.ProspectID {
		winner = "a"
	} else if e.Winner == e.CornerB.ProspectID {
		winner = "b"
	}
	ds, err := fightercareer.Award(winner, e.Method, "standard")
	if err != nil {
		return false
	}
	side := -1
	for i, x := range []careerstate.Corner{e.CornerA, e.CornerB} {
		if x.ProspectID == c.Assignment.ProspectID && x.AccountID == c.Status.Pin.AccountID && x.GymID == c.Assignment.GymID {
			side = i
		}
	}
	if side < 0 || r.Earned != ds[side] || r.XPBefore != min(uint32(15), r.Ordinal-1) || r.XPApplied != min(uint32(15), r.Ordinal)-r.XPBefore || !historyTotals(r.TotalsAfter, r.Ordinal) {
		return false
	}
	t, d := r.TotalsAfter, r.Earned
	if t.Wins < d.Wins || t.Losses < d.Losses || t.Draws < d.Draws || t.BasePoints < d.BasePoints {
		return false
	}
	prior := fightercareer.Totals{Wins: t.Wins - d.Wins, Losses: t.Losses - d.Losses, Draws: t.Draws - d.Draws, XP: r.XPBefore, BasePoints: t.BasePoints - d.BasePoints}
	return historyTotals(prior, r.Ordinal-1) && (r.Ordinal != c.EventCount || r.TotalsAfter == c.Totals)
}
func historyPageValid(p gymstable.CareerHistoryPage, v ManifestViewPin) bool {
	if p.Version != 1 || !historyCaptureValid(p.Capture, v) || p.After > p.Capture.EventCount || p.Items == nil || len(p.Items) > 8 || uint32(len(p.Items)) > p.Capture.EventCount-p.After || p.Next != p.After+uint32(len(p.Items)) || p.HasMore != (p.Next < p.Capture.EventCount) || (len(p.Items) == 0) != (p.After == p.Capture.EventCount) || !historySize(p, MaxCareerHistoryPageSize) {
		return false
	}
	keys, awards := map[string]bool{}, map[string]bool{}
	for i, r := range p.Items {
		if !historyRowValid(r, p.Capture) || r.Ordinal != p.Capture.EventCount-p.After-uint32(i) || keys[r.Event.ResultKey] || awards[r.Event.AwardID] {
			return false
		}
		keys[r.Event.ResultKey] = true
		awards[r.Event.AwardID] = true
		if i > 0 {
			newer := p.Items[i-1]
			if r.Event.EffectiveAtMS > newer.Event.EffectiveAtMS || r.Event.EffectiveAtMS == newer.Event.EffectiveAtMS && r.Event.AwardID >= newer.Event.AwardID {
				return false
			}
			total, e := fightercareer.Apply(r.TotalsAfter, newer.Earned)
			if e != nil || total != newer.TotalsAfter {
				return false
			}
		}
	}
	return true
}
func ValidateCareerHistorySnapshot(s CareerHistorySnapshot) error {
	if s.Version != 1 || !historySize(s, 4096) {
		return gymstable.ErrInvalid
	}
	for _, id := range []string{s.StoreID, s.ActiveJobID, s.QueuedJobID, s.LastJobID} {
		if !stableOptionalID(id) {
			return gymstable.ErrInvalid
		}
	}
	if s.State == "unavailable" {
		if s.View == nil && s.Activation == nil && s.StoreID == "" && s.ActiveJobID == "" && s.QueuedJobID == "" && s.LastJobID == "" && historyUnavailableError(s.Error) {
			return nil
		}
		return gymstable.ErrInvalid
	}
	if s.View == nil || !stableViewValid(*s.View) || s.Activation == nil || careerstate.ValidateActivation(*s.Activation) != nil || !canonicalManifestID(s.StoreID) {
		return gymstable.ErrInvalid
	}
	a, q, l := s.ActiveJobID != "", s.QueuedJobID != "", s.LastJobID != ""
	ok := false
	switch s.State {
	case "idle":
		ok = !a && !q && !l && s.Error == ""
	case "busy":
		ok = a != q && !l && s.Error == ""
	case "ready":
		ok = !a && !q && l && s.Error == ""
	case "cancelled":
		ok = !a && !q && l && s.Error == "history_cancelled"
	case "failed":
		ok = !a && !q && l && historyTerminalError(s.Error) && s.Error != "history_cancelled"
	}
	if !ok {
		return gymstable.ErrInvalid
	}
	return nil
}
func ValidateCareerHistoryJob(j CareerHistoryJob) error {
	if j.Version != 1 || !stableViewValid(j.View) || !historySize(j, MaxCareerHistoryJobSize) {
		return gymstable.ErrInvalid
	}
	for _, id := range []string{j.JobID, j.StoreID, j.ActivationID, j.ProspectID} {
		if !canonicalManifestID(id) {
			return gymstable.ErrInvalid
		}
	}
	if j.State == "done" {
		if j.Error != "" || j.Capture == nil || !historyCaptureValid(*j.Capture, j.View) || j.Capture.Status.StoreID != j.StoreID || j.Capture.Activation.ActivationID != j.ActivationID || j.Capture.Assignment.ProspectID != j.ProspectID {
			return gymstable.ErrInvalid
		}
		return nil
	}
	if j.Capture != nil || !historySize(j, 2048) {
		return gymstable.ErrInvalid
	}
	if (j.State == "queued" || j.State == "running") && j.Error == "" || j.State == "cancelled" && j.Error == "history_cancelled" || j.State == "failed" && historyTerminalError(j.Error) && j.Error != "history_cancelled" {
		return nil
	}
	return gymstable.ErrInvalid
}
func ValidateCareerHistoryPageResult(r CareerHistoryPageResult) error {
	if !stableViewValid(r.View) || !canonicalManifestID(r.JobID) || r.StoreID != r.Page.Capture.Status.StoreID || !historyPageValid(r.Page, r.View) {
		return gymstable.ErrInvalid
	}
	return nil
}
