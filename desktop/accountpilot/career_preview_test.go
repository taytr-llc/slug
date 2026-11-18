// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/internal/careerstate"
	"testing"
)

func TestCareerPreviewParentOriginalChildGate(t *testing.T) {
	original := &pilot.StableController{}
	for _, tc := range []struct {
		name   string
		mutate func(*ManifestController)
		err    error
	}{
		{"closed", func(c *ManifestController) { c.closed = true }, gymstable.ErrStaleView},
		{"busy", func(c *ManifestController) { c.operation = true }, gymstable.ErrStaleView},
		{"signing-in", func(c *ManifestController) { c.state = "signing_in" }, gymstable.ErrStaleView},
		{"resetting", func(c *ManifestController) { c.state = "resetting" }, gymstable.ErrStaleView},
		{"expired", func(c *ManifestController) { c.state = "expired" }, gymstable.ErrStaleView},
		{"successor-view", func(c *ManifestController) { c.viewID = "later" }, gymstable.ErrStaleView},
		{"legacy-route", func(c *ManifestController) { c.careerOwner = nil }, careerstate.ErrInactive},
		{"missing-child", func(c *ManifestController) { c.child = nil }, gymstable.ErrUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := &ManifestController{state: "signed_in", viewID: "original", child: original, careerOwner: &careerAccountOwner{}}
			tc.mutate(base)
			c := &StableController{base}
			calls := []func() (pilot.CareerPreviewJob, error){func() (pilot.CareerPreviewJob, error) {
				return c.BeginCareerFightPreview(context.Background(), "original", "a", "b")
			}, func() (pilot.CareerPreviewJob, error) {
				return c.CareerFightPreviewJob(context.Background(), "original", "job")
			}, func() (pilot.CareerPreviewJob, error) {
				return c.CancelCareerFightPreview(context.Background(), "original", "job")
			}}
			for _, call := range calls {
				j, e := call()
				if !errors.Is(e, tc.err) || j.JobID != "" || j.Result != nil {
					t.Fatal(j, e)
				}
			}
			if tc.name == "successor-view" {
				return
			}
			s := c.CareerPreviewSnapshot()
			if s.State != "unavailable" || s.View != nil || s.Activation != nil || s.ActiveJobID != "" || s.LastJobID != "" {
				t.Fatal(s)
			}
		})
	}
	c := &StableController{&ManifestController{state: "signed_in", viewID: "original", child: original, careerOwner: &careerAccountOwner{}}}
	got, e := c.careerPreviewChild("original")
	if e != nil || got != original {
		t.Fatal(got, e)
	}
	// Capture preserves the original pointer; a later child cannot be substituted.
	successor := &pilot.StableController{}
	c.mu.Lock()
	c.child = successor
	c.viewID = "later"
	c.mu.Unlock()
	if got != original {
		t.Fatal("captured child was replaced")
	}
	if _, e = c.careerPreviewChild("original"); !errors.Is(e, gymstable.ErrStaleView) {
		t.Fatal(e)
	}
}
