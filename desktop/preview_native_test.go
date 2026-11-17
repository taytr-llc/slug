// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import "testing"

func TestPreviewNativeWrongModeKeepsTypedFailure(t *testing.T) {
	g := &Gym{}
	for _, r := range []struct{ command string }{{"snapshot"}, {"begin"}, {"job"}, {"cancel"}} {
		value := g.PreviewSnapshot()
		switch r.command {
		case "begin":
			value = g.BeginFightPreview("", "", "")
		case "job":
			value = g.FightPreviewJob("", "")
		case "cancel":
			value = g.CancelFightPreview("", "")
		}
		if value.Version != 1 || value.ID != 1 || value.OK || value.Error != "preview_unavailable" || value.Job != nil || value.Snapshot != nil {
			t.Fatal(value)
		}
	}
	if r := g.FightPreviewPortrait("", "", ""); r.Version != 1 || r.OK || r.Portrait != nil || r.Error != "preview_unavailable" {
		t.Fatal(r)
	}
}
