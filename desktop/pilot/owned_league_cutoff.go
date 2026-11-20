// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/fighterownership"
	"github.com/taytr-llc/slug/gymevent"
)

// advanceCutoff receives a date from the original pinned entry calendar, not a
// browser timestamp. The actual core captures its own two-observation bracket.
// Missing that bracket never permits a current head or a synthetic past lock.
// Caller owns the concrete child action gate; runtime.gate serializes public
// replay with this exact retained historical replay across both fixture views.
func (l *OwnedLeague) advanceCutoff(ctx context.Context, date string) (bool, error) {
	if l == nil || l.ready == nil || l.life.Err() != nil {
		return false, ErrPoolRuntime
	}
	ev, e := gymevent.OwnedEventForDate(l.config.EventPin, date)
	if e != nil {
		return false, e
	}
	work, done, e := l.runtime.begin(ctx)
	if e != nil {
		return false, e
	}
	defer done()
	o := l.runtime.owned
	if o == nil || o.store != l.source || o.config != l.config.Source {
		return false, ErrPoolRuntime
	}
	if l.cutoffReplay != nil {
		if l.cutoffEvent != ev.EventID {
			return false, ErrPoolRuntimeBusy
		}
		return l.stepCutoff(work)
	}
	// The runtime gate excludes every other current replay step. Join only
	// this runtime's current work before asking the concrete core to observe
	// the real bracket. Close retains its exact durable partial work; the
	// existing stale-work branch may retire it only after an authentic boundary.
	if o.replay != nil {
		ce := o.replay.Close()
		o.replay = nil
		o.progress.Ready, o.progress.Head, o.progress.Cutoff = false, nil, nil
		if ce != nil {
			return false, ce
		}
	}
	source, e := fighterownership.NewPublicSource(work, l.config.Source.Ownership, o.publisher)
	if e != nil {
		return false, e
	}
	if _, e = o.store.Cutoff(work, ev.EventID); e == nil {
		_, _, e = o.store.PublishCutoff(work, source, ev.EventID)
		return e == nil, e
	} else if !errors.Is(e, fighterownership.ErrNotFound) && e != fighterownership.ErrUnavailable {
		return false, e
	}
	// An incomplete current head is not cutoff authority. CaptureBoundary
	// independently requires the actual pre/post observations or exact retained
	// intent; in particular it still refuses a missed pre-noon bracket.
	boundary, e := o.store.CaptureBoundary(work, source, o.node, ev.EventID, ev.CutoffMS)
	if e != nil {
		return false, e
	}
	// Only the accepted source prefix frozen at the bracket is required here.
	// The publisher chooses exact next retained records; no caller payload enters.
	status, e := o.publisher.Status(work)
	if e != nil {
		return false, e
	}
	for n := 0; n < 8 && status.SignedAdmissionSequence < boundary.Intent.AcceptedAdmissionSequence; n++ {
		status, e = o.publisher.PublishNext(work)
		if e != nil {
			return false, e
		}
	}
	if status.SignedAdmissionSequence < boundary.Intent.AcceptedAdmissionSequence {
		return false, fighterownership.ErrUnavailable
	}
	source, e = fighterownership.NewPublicSource(work, l.config.Source.Ownership, o.publisher)
	if e != nil {
		return false, e
	}
	replay, e := o.store.BeginCutoffReplay(work, source, o.node, boundary)
	if errors.Is(e, fighterownership.ErrStale) {
		// Retire only this closed durable work record, never immutable boundary or
		// admission evidence. A live current replay remains Busy and is not replaced.
		previous, inspectErr := o.store.InspectReplay(work)
		if inspectErr != nil {
			return false, inspectErr
		}
		if e = o.store.AbandonReplay(work, previous.WorkID); e != nil {
			return false, e
		}
		replay, e = o.store.BeginCutoffReplay(work, source, o.node, boundary)
	}
	if e != nil {
		return false, e
	}
	l.cutoffReplay = replay
	l.cutoffSource = source
	l.cutoffEvent = ev.EventID
	l.cutoffDate = date
	return l.stepCutoff(work)
}
func (l *OwnedLeague) stepCutoff(ctx context.Context) (bool, error) {
	for n := 0; n < 8; n++ {
		progress, e := l.cutoffReplay.Step(ctx)
		if e != nil {
			ce := l.cutoffReplay.Close()
			l.cutoffReplay = nil
			l.cutoffSource = nil
			l.cutoffEvent = ""
			l.cutoffDate = ""
			return false, errors.Join(e, ce)
		}
		if progress.Ready {
			event, source := l.cutoffEvent, l.cutoffSource
			valid := progress.Cutoff != nil && progress.Cutoff.EventID == event
			ce := l.cutoffReplay.Close()
			l.cutoffReplay = nil
			l.cutoffSource = nil
			l.cutoffEvent = ""
			l.cutoffDate = ""
			if !valid {
				return false, errors.Join(fighterownership.ErrCorrupt, ce)
			}
			if ce != nil {
				return false, ce
			}
			_, _, e = l.source.PublishCutoff(ctx, source, event)
			// A concurrent admission may advance the publication prefix after replay.
			// The historical cutoff remains retained; the next exact attempt republishes
			// against the same concrete publisher's fresh status without recalculation.
			return e == nil, e
		}
	}
	return false, nil
}

// resumeCutoff is always invoked before ordinary RefreshOwnership. A retained
// historical job receives its bounded next slice even while current replay
// would report Busy. No caller can select another event for that live handle.
func (l *OwnedLeague) resumeCutoff(ctx context.Context) (date string, retained, ready bool, resultErr error) {
	if l == nil || l.runtime == nil || l.life.Err() != nil {
		return "", false, false, ErrPoolRuntime
	}
	work, done, e := l.runtime.begin(ctx)
	if e != nil {
		return "", false, false, e
	}
	defer done()
	if l.cutoffReplay == nil {
		return "", false, false, nil
	}
	date = l.cutoffDate
	if date == "" {
		return "", true, false, fighterownership.ErrCorrupt
	}
	ready, e = l.stepCutoff(work)
	return date, true, ready, e
}
