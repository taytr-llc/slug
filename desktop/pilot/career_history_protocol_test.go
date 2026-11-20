// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/internal/careerstate"
)

// Only the new family is implemented. Accidental fallback to an older family
// reaches the nil embedded interface and fails the test.
type historyProtocolStub struct {
	CareerControllerAPI
	calls atomic.Int32
}

func (c *historyProtocolStub) Stop()        {}
func (c *historyProtocolStub) Close() error { return nil }
func (c *historyProtocolStub) CareerHistorySnapshot(context.Context) (CareerHistorySnapshot, error) {
	c.calls.Add(1)
	return CareerHistorySnapshot{Version: 1, State: "unavailable", Error: "career_inactive"}, nil
}
func (c *historyProtocolStub) BeginCareerHistory(context.Context, string, string) (CareerHistoryJob, error) {
	c.calls.Add(1)
	return CareerHistoryJob{}, gymstable.ErrCapacity
}
func (c *historyProtocolStub) CareerHistoryJob(context.Context, string, string) (CareerHistoryJob, error) {
	c.calls.Add(1)
	return CareerHistoryJob{}, gymstable.ErrNotFound
}
func (c *historyProtocolStub) CancelCareerHistory(context.Context, string, string) (CareerHistoryJob, error) {
	c.calls.Add(1)
	return CareerHistoryJob{}, gymstable.ErrOutcomeUnknown
}
func (c *historyProtocolStub) CareerHistoryPage(context.Context, string, string, uint32, uint32) (CareerHistoryPageResult, error) {
	c.calls.Add(1)
	return CareerHistoryPageResult{}, gymaccount.ErrUnauthorized
}

func TestCareerHistoryServeCareersClosedRouting(t *testing.T) {
	id := strings.Repeat("a", 64)
	queries := []CareerHistoryRequest{
		{ID: 1, Version: 1, Command: "career_history_snapshot"},
		{ID: 2, Version: 1, Command: "career_history_begin", ViewID: id, ProspectID: id},
		{ID: 3, Version: 1, Command: "career_history_job", ViewID: id, JobID: id},
		{ID: 4, Version: 1, Command: "career_history_cancel", ViewID: id, JobID: id},
		{ID: 5, Version: 1, Command: "career_history_page", ViewID: id, JobID: id, Limit: 8},
	}
	var in, out bytes.Buffer
	for _, q := range queries {
		b, e := MarshalCareerHistoryRequest(q)
		if e != nil {
			t.Fatal(e)
		}
		in.Write(b)
		in.WriteByte('\n')
	}
	c := &historyProtocolStub{}
	if e := ServeCareers(t.Context(), c, &in, &out); e != nil {
		t.Fatal(e)
	}
	rows := bytes.Split(bytes.TrimSpace(out.Bytes()), []byte("\n"))
	if len(rows) != len(queries) || c.calls.Load() != 5 {
		t.Fatal("new family dispatch", len(rows), c.calls.Load())
	}
	for i, b := range rows {
		r, e := ParseCareerHistoryResponse(b)
		if e != nil || r.ID != queries[i].ID {
			t.Fatal(i, e)
		}
		if i == 0 {
			if !r.OK || r.Snapshot == nil || r.Snapshot.State != "unavailable" {
				t.Fatal(r)
			}
			continue
		}
		want := []string{"", "history_capacity", "history_not_found", "history_observation_unknown", "unauthorized"}[i]
		if r.OK || r.Error != want || r.Job != nil || r.Page != nil || r.Snapshot != nil {
			t.Fatal(i, r)
		}
	}
	// The same valid frame on the legacy Result route must never dispatch History.
	c = &historyProtocolStub{}
	in.Reset()
	out.Reset()
	b, _ := MarshalCareerHistoryRequest(queries[0])
	in.Write(b)
	in.WriteByte('\n')
	if e := ServeResults(t.Context(), c, &in, &out); e != nil {
		t.Fatal(e)
	}
	r, e := ParseCareerHistoryResponse(bytes.TrimSpace(out.Bytes()))
	if e != nil || r.Error != "invalid_request" || c.calls.Load() != 0 {
		t.Fatal(r, e, c.calls.Load())
	}
}
func TestCareerHistoryErrorAndDeadlinePrecedence(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	deadline, done := context.WithDeadline(t.Context(), time.Unix(0, 0))
	defer done()
	for _, r := range []struct {
		e    error
		want string
	}{
		{errors.Join(context.DeadlineExceeded, gymstable.ErrOutcomeUnknown, careerstate.ErrClosed), "history_observation_unknown"},
		{errors.Join(context.DeadlineExceeded, gymstable.ErrCorrupt), "history_corrupt"},
		{errors.Join(context.DeadlineExceeded, gymaccount.ErrUnauthorized), "unauthorized"},
		{context.DeadlineExceeded, "busy"},
	} {
		if got := careerHistoryProtocolError(historyMetadataError(deadline, r.e)); got != r.want {
			t.Fatal(got, r.want)
		}
	}
	if got := careerHistoryProtocolError(historyMetadataError(ctx, context.Canceled)); got != "history_cancelled" {
		t.Fatal(got)
	}
}
