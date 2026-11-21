// Copyright (c) 2026 The SLUG developers. ISC license.
package pooloperator

import (
	"context"
	"errors"
	"time"

	"github.com/taytr-llc/slug/fighterownership"
	"github.com/taytr-llc/slug/internal/operatorauthority"
	"github.com/taytr-llc/slug/poolround"
	"github.com/taytr-llc/slug/registry"
)

// publicMaintenance is used only by the runtime's joined serial public loop.
// Its original concrete owners survive every slice; no callback replaces them.
type publicMaintenance struct {
	profile     registry.OperatorProfile
	owner       *operatorauthority.Owner
	publisher   *registry.ManifestPublication
	core        *fighterownership.Store
	node        *fighterownership.OwnedChainSource
	source      *fighterownership.PublicSource
	replay      *fighterownership.Replay
	observation fighterownership.ChainObservation
	progress    fighterownership.Progress
}

func (m *publicMaintenance) close() error {
	if m.replay == nil {
		return nil
	}
	e := m.replay.Close()
	if e == nil {
		m.replay = nil
	}
	return e
}

// step retains successful incremental work across calls. Returned progress is a
// dated observation supplied to status by the caller, never a spending permit.
func (m *publicMaintenance) step(ctx context.Context) (p fighterownership.Progress, err error) {
	defer func() {
		if err != nil {
			p.Ready = false
			p.Head = nil
			p.Cutoff = nil
		}
	}()
	if m.replay == nil {
		source, e := fighterownership.NewOperatorPublicSource(ctx, m.profile, m.publisher, m.owner)
		if e != nil {
			return m.progress, e
		}
		if source, _, e = m.core.ResumeChainPublication(ctx, source, m.node); e != nil {
			return m.progress, e
		}
		if source, _, e = m.core.ResumeCutoffPublication(ctx, source); e != nil {
			return m.progress, e
		}
		status, e := m.publisher.Status(ctx)
		if e != nil {
			return m.progress, e
		}
		// Reopen must not briefly re-enable intake from a fully consumed
		// publication prefix merely because the current node tip is unchanged.
		if status.PublicationSequence == m.profile.Pin().Limits.Publications {
			return m.progress, registry.ErrPublicCapacity
		}
		for n := 0; n < 8 && status.PublicEvidencePending; n++ {
			status, e = m.publisher.PublishNext(ctx)
			if e != nil {
				return m.progress, e
			}
		}
		if status.PublicEvidencePending {
			return m.progress, fighterownership.ErrUnavailable
		}
		m.source, e = fighterownership.NewOperatorPublicSource(ctx, m.profile, m.publisher, m.owner)
		if e != nil {
			return m.progress, e
		}
		observation, e := m.node.Observe(ctx)
		if e != nil {
			return m.progress, e
		}
		unchanged := observation == m.observation
		if !unchanged {
			m.source, _, e = m.core.RecordChainObservation(ctx, m.source, m.node)
			if e != nil {
				return m.progress, e
			}
			m.observation = observation
			status, e = m.publisher.Status(ctx)
			if e != nil {
				return m.progress, e
			}
		}
		if h, e := m.core.Head(ctx); e == nil && unchanged && m.progress.Ready && h.ChainHeight == observation.Height && h.ChainHash == observation.Hash && h.PublicationSequence == status.PublicationSequence && h.AdmissionSequence == status.SignedAdmissionSequence {
			return m.progress, nil
		}
		m.replay, e = m.core.BeginReplay(ctx, m.source, m.node)
		if errors.Is(e, fighterownership.ErrStale) {
			partial, x := m.core.InspectReplay(ctx)
			if x != nil {
				return m.progress, x
			}
			if e = m.core.AbandonReplay(ctx, partial.WorkID); e != nil {
				return m.progress, e
			}
			m.replay, e = m.core.BeginReplay(ctx, m.source, m.node)
		}
		if e != nil {
			return m.progress, e
		}
		m.progress = fighterownership.Progress{WorkID: m.replay.WorkID()}
	}
	for n := 0; n < 8; n++ {
		p, e := m.replay.Step(ctx)
		if p.WorkID != "" {
			m.progress = p
		}
		if e != nil {
			// Contention or a slice deadline need not discard completed in-memory
			// verification. Source/tip changes are detected again by the same Step.
			if !errors.Is(e, fighterownership.ErrBusy) && !errors.Is(e, operatorauthority.ErrBusy) && !errors.Is(e, context.DeadlineExceeded) {
				e = errors.Join(e, m.close())
			}
			m.progress.Ready = false
			return m.progress, e
		}
		m.progress = p
		if p.Ready {
			return p, m.close()
		}
	}
	return m.progress, nil
}

type roundMaintenance struct {
	store  *poolround.Store
	cursor uint64
	live   *poolround.Live // selected only before Run; same original store
}

func (m *roundMaintenance) step(ctx context.Context) (poolround.Summary, error) {
	return m.stepSelected(ctx, false)
}

func (m *roundMaintenance) stepSelected(ctx context.Context, daily bool) (poolround.Summary, error) {
	if daily {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}
	summary, e := dailyRoundCall(ctx, daily, func(work context.Context) (poolround.Summary, error) { return m.store.Sync(work) })
	if e != nil {
		return poolround.Summary{}, e
	}
	if m.cursor >= summary.Rounds {
		m.cursor = 0
	}
	return summary, m.refreshSelected(ctx, daily)
}

// refresh never imports a blocked tail or invents new rounds. List verifies the
// retained Through prefix; Refresh rechecks each exact original winning block.
func (m *roundMaintenance) refresh(ctx context.Context) error {
	return m.refreshSelected(ctx, false)
}

func (m *roundMaintenance) refreshSelected(ctx context.Context, daily bool) error {
	if daily {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}
	if m.live != nil {
		return m.refreshLiveSelected(ctx, daily)
	}
	list := func(after uint64) ([]poolround.Record, error) {
		return dailyRoundCall(ctx, daily, func(work context.Context) ([]poolround.Record, error) {
			return m.store.List(work, after, poolround.MaxPage)
		})
	}
	rows, e := list(m.cursor)
	if e != nil {
		return e
	}
	if len(rows) == 0 && m.cursor != 0 {
		m.cursor = 0
		rows, e = list(0)
		if e != nil {
			return e
		}
	}
	for _, row := range rows {
		if _, e = dailyRoundCall(ctx, daily, func(work context.Context) (poolround.Record, error) {
			return m.store.Refresh(work, row.Round.BlockHash)
		}); e != nil {
			return e
		}
		m.cursor = row.Round.Sequence
	}
	return ctx.Err()
}
