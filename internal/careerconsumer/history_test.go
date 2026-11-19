// Copyright (c) 2026 The SLUG developers. ISC license.
package careerconsumer

import (
	"context"
	"github.com/taytr-llc/slug/internal/careerstate"
	"strings"
	"testing"
)

func TestHistoryRequiresInstalledBindingAndLiveOriginalScope(t *testing.T) {
	s, r := owner(t)
	g, _ := New(nil, false)
	pid := strings.Repeat("a", 64)
	if h, e := g.CaptureHistory(t.Context(), pid, 100); e != careerstate.ErrInactive || h != nil {
		t.Fatal("unbound capture", e)
	}
	_, install, e := g.PrepareBind(r, "stable", s.Scope.StableStoreIDs[0])
	if e != nil {
		t.Fatal(e)
	}
	if h, e := g.CaptureHistory(t.Context(), pid, 100); e != careerstate.ErrInactive || h != nil {
		t.Fatal("unsynced binding exposed capture", e)
	}
	install()
	h, e := g.CaptureHistory(t.Context(), pid, 100)
	if e != nil || h.EventCount() != 0 {
		t.Fatal(e)
	}
	if a, e := g.CheckHistory(t.Context(), "stable", s.Scope.StableStoreIDs[0]); e != nil || a != s.Activation {
		t.Fatal(e)
	}
	if _, e := g.CheckHistory(t.Context(), "stable", strings.Repeat("9", 64)); e != careerstate.ErrInvalid {
		t.Fatal("foreign store accepted", e)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, e := g.CheckHistory(ctx, "stable", s.Scope.StableStoreIDs[0]); e != careerstate.ErrCancelled {
		t.Fatal(e)
	}
	s.ClockMS = 0
	if h, e := g.CaptureHistory(t.Context(), pid, 100); e != careerstate.ErrCorrupt || h != nil {
		t.Fatal("corrupt reader fallback", e)
	}
	if _, e := g.CheckHistory(t.Context(), "stable", s.Scope.StableStoreIDs[0]); e != careerstate.ErrCorrupt {
		t.Fatal("liveness failure hidden", e)
	}
	if len(g.fences) != 0 {
		t.Fatal("history created cutoff fences")
	}
}
