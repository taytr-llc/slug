// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymresult"
	"time"
)

// Result metadata reads include durable account and source-clock observations.
// This admission/operation budget preserves a shorter caller deadline; a Sync
// already in progress must still join and can return observation uncertainty.
// Cached polls, short View checks, other families and jobs keep their budgets.
const resultMetadataTimeout = 500 * time.Millisecond

func (s *StableController) ResultSnapshot() ResultSnapshot {
	blank := ResultSnapshot{Version: 1, State: "unavailable", Error: "result_unavailable", NodeState: "offline", NodeOwner: "none"}
	out := blank
	e := s.shortView(context.Background(), s.view.ViewID, func() error {
		s.stableMu.Lock()
		defer s.stableMu.Unlock()
		if !s.resultEnabled || !s.resultAttached || !s.installed || s.stableClosed {
			return gymresult.ErrUnavailable
		}
		v := s.viewPin()
		out = ResultSnapshot{Version: 1, State: "setup_incomplete", Error: "result_setup_incomplete", View: &v, NodeState: "offline", NodeOwner: "none"}
		if s.resultStore == nil {
			return nil
		}
		p := cloneResultPin(s.resultPin)
		out.State = "ready"
		out.Error = ""
		out.Pin = &p
		out.StoreID = s.resultID
		out.CreatedAtMS = s.resultCreated
		out.SourceCreatedAtMS = s.resultSourceCreated
		out.PublisherAccountID = s.resultPin.PublisherAccountID
		if s.seedNodesReadyLocked() {
			out.NodeState = "online"
			out.NodeOwner = "mining"
			if s.resultNodeOwner != nil && s.resultNodeOwner.ready && s.resultNodeOwner.nodes == s.seedNodes {
				out.NodeOwner = "publication"
			}
		} else if s.resultNodeOwner != nil && !s.resultNodeOwner.ready {
			out.NodeState = "starting"
			out.NodeOwner = "publication"
		}

		s.expireQueuedLocked()
		if s.resultLast != nil {
			out.LastJobID = s.resultLast.JobID
		}
		if s.resultFailure != "" {
			out.State = "failed"
			out.Error = s.resultFailure
		}
		if s.active != nil || s.queued != nil {
			out.State = "busy"
			out.Error = ""
		}
		if s.active != nil && s.active.publication != nil {
			out.ActiveJobID = s.active.publication.JobID
		}
		if s.queued != nil && s.queued.publication != nil && s.queued.published {
			out.QueuedJobID = s.queued.publication.JobID
		}
		return nil
	})
	if e != nil {
		return blank
	}
	return out
}
func (s *StableController) resultRead(ctx context.Context, v string, fn func(context.Context, *gymresult.Store, *gymentry.Store) error) error {
	ctx, cancel := context.WithTimeout(ctx, resultMetadataTimeout)
	defer cancel()
	if e := s.viewWithBudget(ctx, v, resultMetadataTimeout, func() error { return nil }); e != nil {
		return e
	}
	select {
	case s.laneGate <- struct{}{}:
	default:
		return gymresult.ErrBusy
	}
	defer func() { <-s.laneGate }()
	s.stableMu.Lock()
	valid := s.resultEnabled && s.resultAttached && s.installed && !s.stableClosed && !s.stopping
	store := s.resultStore
	s.stableMu.Unlock()
	if !valid {
		return gymresult.ErrUnavailable
	}
	if store == nil {
		return errResultSetup
	}
	if s.entryStore == nil {
		return gymresult.ErrBusy
	}
	if ctx.Err() != nil {
		return gymresult.ErrBusy
	}
	if e := fn(ctx, store, s.entryStore); e != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) && !errors.Is(e, gymresult.ErrOutcomeUnknown) {
			return gymresult.ErrBusy
		}
		return e
	}
	if e := s.viewWithBudget(ctx, v, resultMetadataTimeout, func() error { return nil }); e != nil {
		return gymresult.ErrOutcomeUnknown
	}
	return nil
}
func (s *StableController) ResultStatus(ctx context.Context, v, date, bid string) (gymresult.Status, error) {
	if !entryDateValid(date) {
		return gymresult.Status{}, gymresult.ErrCalendar
	}
	if !canonicalManifestID(bid) {
		return gymresult.Status{}, gymresult.ErrInvalid
	}
	var out gymresult.Status
	e := s.resultRead(ctx, v, func(ctx context.Context, x *gymresult.Store, source *gymentry.Store) error {
		var e error
		out, e = x.Status(ctx, s.boutStore, s.cardStore, s.seedStore, s.eventStore, source, s.session(), date, bid)
		return e
	})
	if e != nil {
		return gymresult.Status{}, e
	}
	return out, nil
}
func (s *StableController) ResultRecord(ctx context.Context, v, date, bid string) (gymresult.ReadResult, error) {
	if !entryDateValid(date) {
		return gymresult.ReadResult{}, gymresult.ErrCalendar
	}
	if !canonicalManifestID(bid) {
		return gymresult.ReadResult{}, gymresult.ErrInvalid
	}
	var out gymresult.ReadResult
	e := s.resultRead(ctx, v, func(ctx context.Context, x *gymresult.Store, source *gymentry.Store) error {
		var e error
		out, e = x.Read(ctx, s.boutStore, s.cardStore, s.seedStore, s.eventStore, source, s.session(), date, bid)
		return e
	})
	if e != nil {
		return gymresult.ReadResult{}, e
	}
	return out, nil
}
func (s *StableController) ResultJob(ctx context.Context, v, id string) (ResultJob, error) {
	if !canonicalManifestID(id) {
		return ResultJob{}, gymresult.ErrInvalid
	}
	var out ResultJob
	e := s.shortView(ctx, v, func() error {
		s.stableMu.Lock()
		defer s.stableMu.Unlock()
		if !s.resultEnabled || s.stableClosed {
			return gymresult.ErrUnavailable
		}
		s.expireQueuedLocked()
		for _, t := range []*stableTask{s.active, s.queued} {
			if t != nil && t.publication != nil && (t == s.active || t.published) && t.publication.JobID == id {
				out = cloneResultJob(*t.publication)
				return nil
			}
		}
		if s.resultLast != nil && s.resultLast.JobID == id {
			out = cloneResultJob(*s.resultLast)
			return nil
		}
		return gymresult.ErrNotFound
	})
	if e != nil {
		return ResultJob{}, e
	}
	return out, nil
}
func (s *StableController) CancelResultJob(ctx context.Context, v, id string) (ResultJob, error) {
	if !canonicalManifestID(id) {
		return ResultJob{}, gymresult.ErrInvalid
	}
	var out ResultJob
	e := s.shortView(ctx, v, func() error {
		s.stableMu.Lock()
		defer s.stableMu.Unlock()
		if !s.resultEnabled || s.stableClosed {
			return gymresult.ErrUnavailable
		}
		if s.active != nil && s.active.publication != nil && s.active.publication.JobID == id {
			s.active.cancel()
			out = cloneResultJob(*s.active.publication)
			return nil
		}
		if s.queued != nil && s.queued.publication != nil && s.queued.published && s.queued.publication.JobID == id {
			s.cancelQueuedResultLocked()
			out = cloneResultJob(*s.resultLast)
			return nil
		}
		if s.resultLast != nil && s.resultLast.JobID == id {
			out = cloneResultJob(*s.resultLast)
			return nil
		}
		return gymresult.ErrNotFound
	})
	if e != nil {
		return ResultJob{}, e
	}
	return out, nil
}

func cloneResultPin(p gymresult.Pin) gymresult.Pin {
	raw, _ := json.Marshal(p)
	var out gymresult.Pin
	_ = json.Unmarshal(raw, &out)
	return out
}
func cloneResultJob(j ResultJob) ResultJob {
	raw, _ := json.Marshal(j)
	var out ResultJob
	_ = json.Unmarshal(raw, &out)
	return out
}
