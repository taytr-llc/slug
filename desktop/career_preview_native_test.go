// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import "testing"

func TestCareerPreviewNativeWrongModeTypedFailure(t *testing.T) {
	g := &Gym{}
	for _, r := range []struct {
		name  string
		value func() bool
	}{
		{"snapshot", func() bool {
			v := g.CareerPreviewSnapshot()
			return v.Version == 1 && v.ID == 1 && !v.OK && v.Error == "career_inactive" && v.Job == nil && v.Snapshot == nil
		}},
		{"begin", func() bool {
			v := g.BeginCareerFightPreview("", "", "")
			return v.Version == 1 && v.ID == 1 && !v.OK && v.Error == "career_inactive" && v.Job == nil && v.Snapshot == nil
		}},
		{"job", func() bool {
			v := g.CareerFightPreviewJob("", "")
			return v.Version == 1 && v.ID == 1 && !v.OK && v.Error == "career_inactive" && v.Job == nil && v.Snapshot == nil
		}},
		{"cancel", func() bool {
			v := g.CancelCareerFightPreview("", "")
			return v.Version == 1 && v.ID == 1 && !v.OK && v.Error == "career_inactive" && v.Job == nil && v.Snapshot == nil
		}},
	} {
		if !r.value() {
			t.Fatal(r.name)
		}
	}
	p := g.CareerFightPreviewPortrait("", "", "")
	if p.Version != 1 || p.OK || p.Error != "career_inactive" || p.Portrait != nil {
		t.Fatal(p)
	}
}
