// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"errors"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymresult"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/internal/careerconsumer"
	"github.com/taytr-llc/slug/internal/careerstate"
)

func careerActionCode(s string) bool {
	return stableOne(s, "career_candidate_ineligible", "career_inactive", "career_unfenced", "career_unavailable", "career_capacity")
}
func careerActionError(e error) string {
	if errors.Is(e, gymresult.ErrOutcomeUnknown) || errors.Is(e, gymstable.ErrOutcomeUnknown) || errors.Is(e, gymcard.ErrOutcomeUnknown) || errors.Is(e, gymevent.ErrOutcomeUnknown) || errors.Is(e, gymentry.ErrOutcomeUnknown) {
		return "outcome_unknown"
	}
	if errors.Is(e, careerconsumer.ErrCapacity) {
		return "career_capacity"
	}
	if errors.Is(e, gymstable.ErrCareerCandidateIneligible) {
		return "career_candidate_ineligible"
	}
	if errors.Is(e, careerstate.ErrInactive) {
		return "career_inactive"
	}
	if errors.Is(e, careerstate.ErrUnfenced) || errors.Is(e, gymresult.ErrCareerNotDue) {
		return "career_unfenced"
	}
	if errors.Is(e, careerstate.ErrClosed) || errors.Is(e, careerstate.ErrPoisoned) || errors.Is(e, careerstate.ErrCorrupt) {
		return "career_unavailable"
	}
	return ""
}
func careerIneligibleIDs(e error, code string) []string {
	if code != "career_candidate_ineligible" {
		return nil
	}
	var x *gymstable.CareerIneligibleError
	if !errors.As(e, &x) {
		return nil
	}
	return append([]string(nil), x.ProspectIDs...)
}
func careerIneligibleValid(code string, ids []string) bool {
	if code != "career_candidate_ineligible" {
		return len(ids) == 0
	}
	if len(ids) < 1 || len(ids) > 10 {
		return false
	}
	last := ""
	for _, id := range ids {
		if !canonicalManifestID(id) || id <= last {
			return false
		}
		last = id
	}
	return true
}
