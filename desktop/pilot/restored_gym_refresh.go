// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/taytr-llc/slug/fightercarrier"
	"github.com/taytr-llc/slug/fighterownership"
)

// A single original operation owns a concrete replay. Its cancellation callback
// joins that replay even while no caller is currently requesting a slice. It
// never modifies session fields; those remain protected by the session gate.
type restoredRefresh struct {
	replay      *fighterownership.Replay
	life        context.Context
	original    context.Context
	cancel      context.CancelFunc
	stopSession func() bool
	joined      chan struct{}
	closeErr    error
	once        sync.Once
	progress    fighterownership.Progress
}

func retainRestoredRefresh(original, session context.Context, replay *fighterownership.Replay) *restoredRefresh {
	life, cancel := context.WithCancel(original)
	r := &restoredRefresh{replay: replay, life: life, original: original, cancel: cancel, joined: make(chan struct{}), progress: fighterownership.Progress{WorkID: replay.WorkID()}}
	r.stopSession = context.AfterFunc(session, cancel)
	context.AfterFunc(life, func() { r.closeErr = replay.Close(); close(r.joined) })
	return r
}
func (r *restoredRefresh) Close() error {
	if r == nil {
		return nil
	}
	r.once.Do(func() { r.cancel(); <-r.joined; r.stopSession() })
	return r.closeErr
}
func (s *RestoredGymSession) clearRefresh() error {
	if s.refresh == nil {
		return nil
	}
	e := s.refresh.Close()
	if e == nil {
		s.refresh = nil
	} else {
		s.poisoned.Store(true)
	}
	return e
}

// BeginRefresh captures one exact original source and node target. No ready
// head is disclosed until RefreshStep completes its concrete verification.
func (s *RestoredGymSession) BeginRefresh(ctx context.Context) (progress fighterownership.Progress, resultErr error) {
	work, end, e := s.acquire(ctx, 30*time.Second)
	if e != nil {
		return progress, e
	}
	defer func() { s.rememberCleanup(resultErr); end() }()
	if e = s.connect(work); e != nil {
		return progress, errors.Join(ErrRestoredGym, e)
	}
	return s.beginRefresh(work, ctx)
}
func (s *RestoredGymSession) beginRefresh(work, original context.Context) (fighterownership.Progress, error) {
	if s.refresh != nil {
		return fighterownership.Progress{}, fightercarrier.ErrBusy
	}
	if e := s.clearDiscovery(); e != nil {
		return fighterownership.Progress{}, e
	}
	source, e := fighterownership.NewRemotePublicSource(work, s.initial.Ownership, s.reader)
	if e != nil {
		return fighterownership.Progress{}, e
	}
	replay, e := s.store.BeginReplay(work, source, s.node)
	if errors.Is(e, fighterownership.ErrStale) {
		pending, inspectErr := s.store.InspectReplay(work)
		if inspectErr != nil {
			return fighterownership.Progress{}, errors.Join(e, inspectErr)
		}
		if e = s.store.AbandonReplay(work, pending.WorkID); e != nil {
			return fighterownership.Progress{}, e
		}
		source, e = fighterownership.NewRemotePublicSource(work, s.initial.Ownership, s.reader)
		if e != nil {
			return fighterownership.Progress{}, e
		}
		replay, e = s.store.BeginReplay(work, source, s.node)
	}
	if e != nil {
		return fighterownership.Progress{}, e
	}
	s.public = source
	s.refresh = retainRestoredRefresh(original, s.life, replay)
	if e = errors.Join(work.Err(), original.Err(), s.life.Err()); e != nil {
		return fighterownership.Progress{}, errors.Join(e, s.clearRefresh())
	}
	return s.refresh.progress, nil
}

// RefreshStep advances at most128 bounded core steps with one30-second slice.
// The exact replay and original caller lifetime persist across successful
// incomplete slices. A new caller cannot rescue cancellation or changed evidence.
func (s *RestoredGymSession) RefreshStep(ctx context.Context, steps uint32) (progress fighterownership.Progress, resultErr error) {
	if steps < 1 || steps > 128 {
		return progress, fightercarrier.ErrInvalid
	}
	work, end, e := s.acquireGate(ctx, 30*time.Second)
	if e != nil {
		return progress, e
	}
	defer func() { s.rememberCleanup(resultErr); end() }()
	return s.stepRefresh(work, steps)
}
func (s *RestoredGymSession) stepRefresh(work context.Context, steps uint32) (progress fighterownership.Progress, resultErr error) {
	r := s.refresh
	if r == nil {
		return progress, fightercarrier.ErrNotFound
	}
	// Bind the slice to the original operation as well as this call/session.
	ctx, cancel := context.WithCancel(work)
	stop := context.AfterFunc(r.life, cancel)
	defer func() { stop(); cancel() }()
	defer func() {
		if resultErr != nil {
			progress.Ready = false
			progress.Head = nil
			progress.Cutoff = nil
			resultErr = errors.Join(resultErr, s.clearRefresh())
		}
	}()
	if e := r.life.Err(); e != nil {
		return progress, e
	}
	for n := uint32(0); n < steps; n++ {
		p, e := r.replay.Step(ctx)
		if e != nil {
			return progress, e
		}
		if p.WorkID != r.progress.WorkID || p.Cutoff != nil {
			return progress, fightercarrier.ErrCorrupt
		}
		r.progress = p
		progress = p
		if p.Ready {
			if p.Head == nil {
				return progress, fightercarrier.ErrCorrupt
			}
			head, e := s.store.Head(ctx)
			if e != nil {
				return progress, e
			}
			if head != *p.Head {
				return progress, fighterownership.ErrStale
			}
			if e = errors.Join(ctx.Err(), r.life.Err(), s.disclose(ctx)); e != nil {
				return progress, e
			}
			// Own the disclosed copy independently of retained core/session data.
			progress.Head = &head
			if e = s.clearRefresh(); e != nil {
				return progress, e
			}
			// Our cleanup cancels only the retained operation. Recheck the
			// original caller/session independently before disclosing completion.
			if e = errors.Join(work.Err(), r.original.Err(), s.life.Err(), s.root.check()); e != nil {
				return progress, e
			}
			return progress, nil
		}
	}
	if e := errors.Join(ctx.Err(), r.life.Err(), s.disclose(ctx)); e != nil {
		return progress, e
	}
	return progress, nil
}

// CancelRefresh cancels and joins only this session's retained exact replay. A
// caller cancellation is not allowed to prevent that required cleanup.
func (s *RestoredGymSession) CancelRefresh(ctx context.Context) error {
	if s == nil || ctx == nil {
		return ErrRestoredGym
	}
	select {
	case s.gate <- struct{}{}:
	default:
		return fightercarrier.ErrBusy
	}
	defer func() { <-s.gate }()
	e := s.clearRefresh()
	s.rememberCleanup(e)
	return e
}
