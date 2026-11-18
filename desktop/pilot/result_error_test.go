// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"fmt"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymresult"
	"github.com/taytr-llc/slug/gymstable"
	"testing"
)

func TestResultTypedErrorsAndUnknownPriority(t *testing.T) {
	for _, e := range []error{gymresult.ErrInvalid, gymresult.ErrUnauthorized, gymresult.ErrForbidden, gymresult.ErrStaleView, gymresult.ErrBusy, gymresult.ErrUnavailable, gymresult.ErrCorrupt, gymresult.ErrCapacity, gymresult.ErrClock, gymresult.ErrCalendar, gymresult.ErrSourceIncomplete, gymresult.ErrSourceChanged, gymresult.ErrSourceConflict, gymresult.ErrNotFound, gymresult.ErrNotPrepared, gymresult.ErrCancelled, gymresult.ErrOutcomeUnknown, gymresult.ErrNodeOffline, gymresult.ErrWaitingForFunds, gymresult.ErrScanIncomplete, gymresult.ErrNotMature, gymresult.ErrConflict, gymresult.ErrProofUnavailable, errResultSetup} {
		if got := resultProtocolError(fmt.Errorf("private diagnostic: %w", e)); got != e.Error() {
			t.Fatal("typed cause lost", e, got)
		}
	}
	for _, e := range []error{errors.Join(gymresult.ErrUnavailable, gymresult.ErrOutcomeUnknown), errors.Join(gymaccount.ErrHeld, gymresult.ErrOutcomeUnknown), errors.Join(context.Canceled, gymresult.ErrOutcomeUnknown), gymstable.ErrOutcomeUnknown} {
		if got := resultProtocolError(e); got != "outcome_unknown" {
			t.Fatal("uncertain work relabeled", got)
		}
	}
	for _, word := range []string{"unauthorized", "forbidden", "outcome_unknown", "result_cancelled", "result_clock"} {
		if got := resultProtocolError(errors.New(word)); got != "result_unavailable" {
			t.Fatal("string became authority verdict", word, got)
		}
	}
	for _, c := range []struct {
		e    error
		want string
	}{{gymaccount.ErrHeld, "forbidden"}, {gymaccount.ErrExpired, "unauthorized"}, {gymevent.ErrCalendar, "result_calendar"}, {gymstable.ErrBusy, "busy"}, {gymresult.ErrClosed, "result_unavailable"}, {gymresult.ErrPoisoned, "result_unavailable"}, {context.Canceled, "result_cancelled"}} {
		if got := resultProtocolError(c.e); got != c.want {
			t.Fatal(c.e, got)
		}
	}
}
