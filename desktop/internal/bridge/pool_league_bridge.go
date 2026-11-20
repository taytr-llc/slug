// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"context"
	"errors"
	"time"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymstable"
)

// preparePoolEventContext uses only the actual pool, stable and entry families.
// Each request releases the pipe gate, so compare the complete original view
// at every step and once more when the event request takes that gate.
func (b *PoolBridge) preparePoolEventContext(ctx context.Context) (*pilot.ManifestViewPin, string) {
	m, err := b.PoolRequest(ctx, pilot.PoolRequest{Command: "snapshot"})
	if err != nil {
		if errors.Is(err, ErrBusy) {
			return nil, "busy"
		}
		return nil, "event_unavailable"
	}
	if m.Snapshot == nil {
		return nil, "event_unavailable"
	}
	if m.Snapshot.View == nil {
		return nil, ""
	}
	view := *m.Snapshot.View
	s := b.StableRequest(ctx, pilot.StableRequest{Command: "stable_snapshot"})
	if !s.OK {
		if s.Error == "busy" {
			return nil, "busy"
		}
		return nil, "event_unavailable"
	}
	if s.Snapshot == nil || s.Snapshot.View == nil || *s.Snapshot.View != view || s.Snapshot.Status == nil ||
		s.Snapshot.Status.Pin.Version != gymstable.SharedPinVersion2 || s.Snapshot.Status.Pin.Source != view.AuthorityPin {
		return nil, "stale_view"
	}
	e := b.EntryRequest(ctx, pilot.EntryRequest{Command: "entry_snapshot"})
	if !e.OK {
		if e.Error == "busy" {
			return nil, "busy"
		}
		return nil, "event_unavailable"
	}
	if e.Snapshot == nil || e.Snapshot.View == nil || e.View == nil || *e.Snapshot.View != view || *e.View != view {
		return nil, "stale_view"
	}
	return &view, ""
}

// EventRequest shadows the inherited legacy preparation path. It reuses the
// same typed event correlation and bounded job recovery after pool preflight.
func (b *PoolBridge) EventRequest(ctx context.Context, req pilot.EventRequest) pilot.EventResponse {
	if b == nil || b.StableBridge == nil || !b.poolMode || !b.eventsEnabled {
		return eventFailure(0, "event_unavailable")
	}
	preflight := req
	preflight.ID = 1
	if _, err := pilot.MarshalEventRequest(preflight); err != nil {
		return eventFailure(0, "invalid_request")
	}
	var prepared *pilot.ManifestViewPin
	if req.Command == "event_snapshot" {
		var code string
		prepared, code = b.preparePoolEventContext(ctx)
		if code != "" {
			return eventFailure(0, code)
		}
	}
	id, err := b.begin(ctx)
	if err != nil {
		if errors.Is(err, ErrBusy) {
			return eventFailure(0, "busy")
		}
		return eventFailure(0, "event_unavailable")
	}
	defer func() { <-b.gate }()
	if req.Command == "event_snapshot" && !b.poolEventPrepared(prepared) {
		return eventFailure(id, "stale_view")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return b.eventRequestHeld(ctx, id, req)
}

// The pipe gate is held by the caller. A replacement login cannot lend its
// source contexts to an event snapshot requested by the previous view.
func (b *PoolBridge) poolEventPrepared(view *pilot.ManifestViewPin) bool {
	if view == nil {
		return b.manifestView == nil && b.view == nil
	}
	return b.manifestView != nil && b.view != nil && *b.manifestView == *view && *b.view == *view
}
