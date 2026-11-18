// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"context"
	"errors"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/internal/careerstate"
	"strings"
	"testing"
)

func TestCareerPreviewInheritedErrorMappingAndFrameBoundary(t *testing.T) {
	for _, row := range []struct {
		err  error
		code string
	}{
		{gymstable.ErrNotFound, "preview_not_found"}, {gymstable.ErrClassMismatch, "preview_class_mismatch"},
		{gymstable.ErrCapacity, "preview_capacity"}, {context.Canceled, "preview_cancelled"},
		{careerstate.ErrClosed, "career_closed"}, {careerstate.ErrPoisoned, "career_poisoned"},
		{errors.Join(gymstable.ErrOutcomeUnknown, careerstate.ErrClosed, context.Canceled), "preview_observation_unknown"},
	} {
		if got := careerPreviewProtocolError(row.err); got != row.code {
			t.Fatal(got, row.code)
		}
	}
	q := CareerPreviewRequest{ID: 1, Version: 1, Command: "career_preview_snapshot"}
	b, e := MarshalCareerPreviewRequest(q)
	if e != nil {
		t.Fatal(e)
	}
	exact := append(append([]byte{}, b...), bytes.Repeat([]byte(" "), 4095-len(b))...)
	if _, e = ParseCareerPreviewRequest(exact); e != nil {
		t.Fatal("exact JSON cap", e)
	}
	if _, e = ParseCareerPreviewRequest(append(exact, ' ')); e == nil {
		t.Fatal("LF was not reserved")
	}
	for _, name := range []string{"preview_snapshot", "career_inventory", "career_preview_import"} {
		bad := q
		bad.Command = name
		if _, e = MarshalCareerPreviewRequest(bad); e == nil {
			t.Fatal("foreign command accepted", name)
		}
	}
	raw := []byte(`{"id":1,"version":1,"command":"career_preview_begin","view_id":"` + strings.Repeat("1", 64) + `","fighter_a_id":"` + strings.Repeat("3", 64) + `","fighter_b_id":"` + strings.Repeat("2", 64) + `"}`)
	if _, e = ParseCareerPreviewRequest(raw); e == nil {
		t.Fatal("unordered wire selection accepted")
	}
}
