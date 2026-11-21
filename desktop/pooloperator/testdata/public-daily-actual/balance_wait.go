package main

import (
	"context"
	"path/filepath"
	"time"

	"github.com/taytr-llc/slug/poolservice"
)

// Diagnostic evidence only. Values come from the existing authenticated read;
// there are no added requests, retries, replacement contexts or acceptance rules.
type dailyBalanceWaitSample struct {
	Sample       uint64                            `json:"sample"`
	GymIndex     int                               `json:"gym_index"`
	ObservedAtMS int64                             `json:"observed_at_ms"`
	Failure      *lateServiceFailure               `json:"failure,omitempty"`
	Balance      *poolservice.DailyBalanceResponse `json:"balance,omitempty"`
}

type dailyBalanceWaitRecord struct {
	Name              string                   `json:"name"`
	BudgetMS          int64                    `json:"budget_ms"`
	StartedAtMS       int64                    `json:"started_at_ms"`
	EndedAtMS         int64                    `json:"ended_at_ms"`
	ElapsedMS         int64                    `json:"elapsed_ms"`
	ContextDeadlineMS int64                    `json:"context_deadline_ms"`
	AfterMS           int64                    `json:"after_ms"`
	CheckReasons      bool                     `json:"check_reasons"`
	TotalSamples      uint64                   `json:"total_samples"`
	Last              []dailyBalanceWaitSample `json:"last"`
	Terminal          string                   `json:"terminal"`
}

type dailyBalanceWaitEvidence struct {
	Version uint32                   `json:"version"`
	Waits   []dailyBalanceWaitRecord `json:"waits"`
	path    string
}

func newDailyBalanceWaitEvidence(root string) *dailyBalanceWaitEvidence {
	return &dailyBalanceWaitEvidence{Version: 1, Waits: []dailyBalanceWaitRecord{}, path: filepath.Join(root, "balance-wait-evidence.json")}
}

func (r *dailyBalanceWaitRecord) sample(gym int, v poolservice.DailyBalanceResponse, err error) {
	r.TotalSamples++
	s := dailyBalanceWaitSample{Sample: r.TotalSamples, GymIndex: gym, ObservedAtMS: time.Now().UnixMilli()}
	if err != nil {
		failure := lateFailure(err) // Closed code/unknown only, never raw error text.
		s.Failure = &failure
	} else {
		// DailyBalance has already authenticated the complete source, settlement,
		// recipient and public balance before returning this owned typed value.
		s.Balance = &v
	}
	if len(r.Last) == 32 {
		copy(r.Last, r.Last[1:])
		r.Last = r.Last[:31]
	}
	r.Last = append(r.Last, s)
}

func (e *dailyBalanceWaitEvidence) begin(ctx context.Context, name string, after int64, budget time.Duration, reasons bool) (*dailyBalanceWaitRecord, func()) {
	require(e != nil && len(e.Waits) < 4, "bounded public balance wait evidence")
	started := time.Now()
	r := &dailyBalanceWaitRecord{Name: name, BudgetMS: budget.Milliseconds(), StartedAtMS: started.UnixMilli(), AfterMS: after, CheckReasons: reasons, Last: []dailyBalanceWaitSample{}, Terminal: "failure"}
	if deadline, ok := ctx.Deadline(); ok {
		r.ContextDeadlineMS = deadline.UnixMilli()
	}
	return r, func() {
		r.EndedAtMS, r.ElapsedMS = time.Now().UnixMilli(), time.Since(started).Milliseconds()
		e.Waits = append(e.Waits, *r)
		// Runs on the original wait's panic path too, before outer owner cleanup.
		write(e.path, e)
	}
}
