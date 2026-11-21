// Copyright (c) 2026 The SLUG developers. ISC license.
package poolround

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/taytr-llc/slug/poolshare"
)

func TestRoundSourceExactBusyClassification(t *testing.T) {
	unknown := errors.New("unknown source failure")
	cases := []struct {
		name         string
		source, want error
	}{
		{"exact-busy", poolshare.ErrBusy, ErrBusy},
		{"wrapped-busy-keeps-prior-unavailable", fmt.Errorf("source: %w", poolshare.ErrBusy), ErrSourceUnavailable},
		{"mixed-unknown", errors.Join(poolshare.ErrBusy, unknown), ErrSourceUnavailable},
		{"corrupt", poolshare.ErrCorrupt, ErrSourceChanged},
		{"mixed-corrupt", errors.Join(poolshare.ErrBusy, poolshare.ErrCorrupt), ErrSourceChanged},
		{"invalid", poolshare.ErrInvalid, ErrSourceChanged},
		{"mixed-profile", errors.Join(poolshare.ErrBusy, poolshare.ErrProfile), ErrProfile},
		{"unknown", unknown, ErrSourceUnavailable},
		{"closed", poolshare.ErrClosed, ErrSourceUnavailable},
		{"poisoned", poolshare.ErrPoisoned, ErrSourceUnavailable},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := sourceError(c.source); got != c.want {
				t.Fatal(got, c.want)
			}
		})
	}
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
		mixed := errors.Join(poolshare.ErrBusy, cause)
		if sourceError(mixed) != mixed {
			t.Fatal("original cancellation/deadline tree lost", cause)
		}
	}
}
