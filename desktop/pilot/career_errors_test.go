// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"errors"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymresult"
	"github.com/taytr-llc/slug/gymstable"
	"strings"
	"testing"
)

func TestCareerIneligibleIDsAndMutationUncertainty(t *testing.T) {
	original := []string{strings.Repeat("a", 64), strings.Repeat("b", 64)}
	refusal := &gymstable.CareerIneligibleError{ProspectIDs: original}
	if careerActionError(refusal) != "career_candidate_ineligible" {
		t.Fatal("lost typed refusal")
	}
	ids := careerIneligibleIDs(refusal, "career_candidate_ineligible")
	if !careerIneligibleValid("career_candidate_ineligible", ids) {
		t.Fatal("valid sorted IDs refused")
	}
	ids[0] = strings.Repeat("c", 64)
	if original[0] != strings.Repeat("a", 64) {
		t.Fatal("borrowed error IDs")
	}
	for _, unknown := range []error{gymstable.ErrOutcomeUnknown, gymentry.ErrOutcomeUnknown, gymevent.ErrOutcomeUnknown, gymcard.ErrOutcomeUnknown, gymresult.ErrOutcomeUnknown} {
		e := errors.Join(refusal, unknown)
		code := careerActionError(e)
		if code != "outcome_unknown" || len(careerIneligibleIDs(e, code)) != 0 {
			t.Fatal("unknown was downgraded", code)
		}
	}
	for _, bad := range [][]string{nil, {original[1], original[0]}, {original[0], original[0]}, {"not an ID"}} {
		if careerIneligibleValid("career_candidate_ineligible", bad) {
			t.Fatal("accepted affected IDs", bad)
		}
	}
}
