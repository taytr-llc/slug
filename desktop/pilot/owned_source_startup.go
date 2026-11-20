// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"time"

	"github.com/taytr-llc/slug/fighterownership"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/registry"
)

// PrepareOwnedSource is the constructor's bounded source-readiness prerequisite
// before StartServices opens owned league stores. The caller retains the same
// runtime and independent setup pin; no child/service is published yet. It must
// not be called while holding r.gate: RefreshOwnership acquires that gate around
// each actual source/node step. A failed constructor closes the retained runtime,
// including any partial replay, before allowing a replacement owner.
func (r *PoolRuntime) PrepareOwnedSource(ctx context.Context, expected OwnedSourceConfig) error {
	if r == nil || ctx == nil || ValidateOwnedGymSource(expected) != nil {
		return ErrPoolRuntime
	}
	check := func() error {
		work, release, e := r.begin(ctx)
		if e != nil {
			return e
		}
		defer release()
		r.mu.Lock()
		valid := !r.closed && !r.ready && len(r.borrows) == 0 && r.ownedGym != nil && r.owned != nil && r.owned.config == expected && r.owned.store != nil && r.owned.publisher != nil && r.owned.node != nil
		r.mu.Unlock()
		if !valid {
			return ErrPoolRuntime
		}
		return work.Err()
	}
	if e := check(); e != nil {
		return e
	}
	work, cancel := context.WithTimeout(ctx, 180*time.Second)
	stop := context.AfterFunc(r.life, cancel)
	defer func() { stop(); cancel() }()
	for {
		progress, e := r.RefreshOwnership(work, 8)
		if e == nil && progress.Ready {
			if progress.Head == nil {
				return ErrPoolRuntime
			}
			return check()
		}
		// Retry only exact transient leaves. A joined storage/source/unknown
		// outcome cannot become retryable merely by also containing Busy.
		switch e {
		case nil, ErrPoolRuntimeBusy, fighterownership.ErrBusy, fighterownership.ErrStale,
			fighterownership.ErrUnavailable, registry.ErrBusy, prospect.ErrHeaderHashBusy:
		default:
			return e
		}
		timer := time.NewTimer(50 * time.Millisecond)
		select {
		case <-work.Done():
			timer.Stop()
			return work.Err()
		case <-timer.C:
		}
	}
}
