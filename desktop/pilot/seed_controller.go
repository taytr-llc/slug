// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/seednode"
	"time"
)

func cloneSeedJob(v SeedJob) SeedJob {
	b, _ := json.Marshal(v)
	var o SeedJob
	_ = json.Unmarshal(b, &o)
	return o
}
func cloneSeedPin(v gymseed.Pin) gymseed.Pin {
	b, _ := json.Marshal(v)
	var o gymseed.Pin
	_ = json.Unmarshal(b, &o)
	return o
}
func (s *StableController) SeedSnapshot() SeedSnapshot {
	blank := SeedSnapshot{Version: 1, State: "unavailable", Error: "seed_unavailable", NodeState: "offline"}
	out := blank
	e := s.shortView(context.Background(), s.view.ViewID, func() error {
		s.stableMu.Lock()
		defer s.stableMu.Unlock()
		if !s.seedEnabled || !s.seedAttached || !s.installed || s.stableClosed {
			return gymseed.ErrUnavailable
		}
		v := s.viewPin()
		out = SeedSnapshot{Version: 1, State: "setup_incomplete", Error: "seed_setup_incomplete", View: &v, NodeState: "offline"}
		if s.seedStore == nil {
			return nil
		}
		p := cloneSeedPin(s.seedPin)
		out.State = "ready"
		out.Error = ""
		out.Pin = &p
		out.StoreID = s.seedID
		out.CreatedAtMS = s.seedCreated
		out.PublisherAccountID = s.seedPin.PublisherAccountID
		if s.seedNodesReadyLocked() {
			out.NodeState = "ready"
		}
		s.expireQueuedLocked()
		if s.seedLast != nil {
			out.LastJobID = s.seedLast.JobID
		}
		if s.seedFailure != "" {
			out.State = "failed"
			out.Error = s.seedFailure
		}
		if s.active != nil || s.queued != nil {
			out.State = "busy"
			out.Error = ""
		}
		if s.active != nil && s.active.seed != nil {
			out.ActiveJobID = s.active.seed.JobID
		}
		if s.queued != nil && s.queued.seed != nil && s.queued.published {
			out.QueuedJobID = s.queued.seed.JobID
		}
		return nil
	})
	if e != nil {
		return blank
	}
	return out
}
func (s *StableController) seedRead(ctx context.Context, v string, fn func(context.Context, *gymseed.Store, *gymentry.Store) error) error {
	ctx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	if e := s.shortView(ctx, v, func() error { return nil }); e != nil {
		return e
	}
	select {
	case s.laneGate <- struct{}{}:
	default:
		return gymseed.ErrBusy
	}
	defer func() { <-s.laneGate }()
	s.stableMu.Lock()
	valid := s.seedEnabled && s.seedAttached && s.installed && !s.stableClosed && !s.stopping
	store := s.seedStore
	s.stableMu.Unlock()
	if !valid {
		return gymseed.ErrUnavailable
	}
	if store == nil {
		return errSeedSetup
	}
	if s.entryStore == nil {
		return gymseed.ErrBusy
	}
	if ctx.Err() != nil {
		return gymseed.ErrBusy
	}
	if e := fn(ctx, store, s.entryStore); e != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) && !errors.Is(e, gymseed.ErrOutcomeUnknown) {
			return gymseed.ErrBusy
		}
		return e
	}
	return s.shortView(ctx, v, func() error { return nil })
}
func (s *StableController) SeedStatus(ctx context.Context, v, date string) (gymseed.Status, error) {
	if !entryDateValid(date) {
		return gymseed.Status{}, gymseed.ErrCalendar
	}
	var out gymseed.Status
	e := s.seedRead(ctx, v, func(ctx context.Context, x *gymseed.Store, source *gymentry.Store) error {
		var e error
		out, e = x.Status(ctx, s.eventStore, source, s.session(), date)
		return e
	})
	if e != nil {
		return gymseed.Status{}, e
	}
	return out, nil
}
func (s *StableController) SeedRecord(ctx context.Context, v, date string) (gymseed.Record, error) {
	if !entryDateValid(date) {
		return gymseed.Record{}, gymseed.ErrCalendar
	}
	var out gymseed.Record
	e := s.seedRead(ctx, v, func(ctx context.Context, x *gymseed.Store, source *gymentry.Store) error {
		var e error
		out, e = x.Record(ctx, s.eventStore, source, s.session(), date)
		return e
	})
	if e != nil {
		return gymseed.Record{}, e
	}
	return out, nil
}
func (s *StableController) SeedHistory(ctx context.Context, v, after string, limit uint32) (gymseed.HistoryPage, error) {
	if limit < 1 || limit > 8 || after != "" && !canonicalManifestID(after) {
		return gymseed.HistoryPage{}, gymseed.ErrInvalid
	}
	var out gymseed.HistoryPage
	e := s.seedRead(ctx, v, func(ctx context.Context, x *gymseed.Store, source *gymentry.Store) error {
		var e error
		out, e = x.History(ctx, s.eventStore, source, s.session(), after, limit)
		return e
	})
	if e != nil {
		return gymseed.HistoryPage{}, e
	}
	return out, nil
}
func (s *StableController) SeedJob(ctx context.Context, v, id string) (SeedJob, error) {
	if !canonicalManifestID(id) {
		return SeedJob{}, gymseed.ErrInvalid
	}
	var out SeedJob
	e := s.shortView(ctx, v, func() error {
		s.stableMu.Lock()
		defer s.stableMu.Unlock()
		if !s.seedEnabled || s.stableClosed {
			return gymseed.ErrUnavailable
		}
		s.expireQueuedLocked()
		for _, t := range []*stableTask{s.active, s.queued} {
			if t != nil && t.seed != nil && (t == s.active || t.published) && t.seed.JobID == id {
				out = cloneSeedJob(*t.seed)
				return nil
			}
		}
		if s.seedLast != nil && s.seedLast.JobID == id {
			out = cloneSeedJob(*s.seedLast)
			return nil
		}
		return gymseed.ErrNotFound
	})
	if e != nil {
		return SeedJob{}, e
	}
	return out, nil
}
func (s *StableController) CancelSeedJob(ctx context.Context, v, id string) (SeedJob, error) {
	if !canonicalManifestID(id) {
		return SeedJob{}, gymseed.ErrInvalid
	}
	var out SeedJob
	e := s.shortView(ctx, v, func() error {
		s.stableMu.Lock()
		defer s.stableMu.Unlock()
		if !s.seedEnabled || s.stableClosed {
			return gymseed.ErrUnavailable
		}
		if s.active != nil && s.active.seed != nil && s.active.seed.JobID == id {
			s.active.cancel()
			out = cloneSeedJob(*s.active.seed)
			return nil
		}
		if s.queued != nil && s.queued.seed != nil && s.queued.published && s.queued.seed.JobID == id {
			s.cancelQueuedSeedLocked()
			out = cloneSeedJob(*s.seedLast)
			return nil
		}
		if s.seedLast != nil && s.seedLast.JobID == id {
			out = cloneSeedJob(*s.seedLast)
			return nil
		}
		return gymseed.ErrNotFound
	})
	if e != nil {
		return SeedJob{}, e
	}
	return out, nil
}

// Nodes remain owned by ManifestController.run. Lane work is joined before
// unregister returns and the run's existing reverse-order cleanup stops them.
func (s *StableController) registerSeedNodes(n *pilotNodes) {
	s.stableMu.Lock()
	defer s.stableMu.Unlock()
	if s.seedEnabled && s.seedAttached && s.seedStore != nil && s.account.AccountID == s.seedPin.PublisherAccountID && !s.stableClosed {
		s.seedNodes = n
	}
}
func (s *StableController) unregisterSeedNodes(n *pilotNodes) {
	s.stableMu.Lock()
	if s.seedNodes != n {
		s.stableMu.Unlock()
		return
	}
	s.seedNodes = nil
	for _, t := range []*stableTask{s.active, s.queued} {
		if t != nil && (t.seed != nil || t.publication != nil) {
			t.cancel()
		}
	}
	s.stableMu.Unlock()
	s.laneGate <- struct{}{}
	<-s.laneGate
}
func (s *StableController) seedNodesReadyLocked() bool {
	if s.sharedPublisher != nil {
		return sharedPoolNodesReady(s.sharedPublisher)
	}
	n := s.seedNodes
	if n == nil || n.primary == nil || n.peer == nil {
		return false
	}
	select {
	case <-n.primary.done:
		return false
	default:
	}
	select {
	case <-n.peer.done:
		return false
	default:
	}
	return true
}
func (s *StableController) borrowSeedPublisher(ctx context.Context) (*seednode.Publisher, error) {
	s.stableMu.Lock()
	if !s.seedNodesReadyLocked() || s.stableClosed || s.stopping || s.account.AccountID != s.seedPin.PublisherAccountID {
		s.stableMu.Unlock()
		return nil, gymseed.ErrNodeOffline
	}
	if s.sharedPublisher != nil {
		binding := *s.sharedPublisher
		s.stableMu.Unlock()
		return seednode.AttachSharedPool(ctx, binding)
	}
	n := s.seedNodes
	binding := seednode.OwnerBinding{Source: s.seedPin.SourcePin, Home: s.config.DataDir, Primary: seednode.OwnedProcess{Cmd: n.primary.cmd, Done: n.primary.done}, Peer: seednode.OwnedProcess{Cmd: n.peer.cmd, Done: n.peer.done}}
	s.stableMu.Unlock()
	return seednode.AttachOwned(ctx, binding)
}
