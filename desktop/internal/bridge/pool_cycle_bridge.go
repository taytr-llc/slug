// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/desktop/pilot"
	"time"
)

// SeedRequest discovers its exact lower authority through the pool route.
func (b *PoolBridge) SeedRequest(ctx context.Context, req pilot.SeedRequest) pilot.SeedResponse {
	if b == nil || b.StableBridge == nil || !b.poolMode || !b.seedsEnabled {
		return seedFailure(0, "seed_unavailable")
	}
	pre := req
	pre.ID = 1
	if _, err := pilot.MarshalSeedRequest(pre); err != nil {
		return seedFailure(0, "invalid_request")
	}
	var prepared *pilot.ManifestViewPin
	if req.Command == "seed_snapshot" {
		r := b.EventRequest(ctx, pilot.EventRequest{Command: "event_snapshot"})
		if !r.OK {
			code := "seed_unavailable"
			if oneOf(r.Error, "busy", "stale_view", "unauthorized", "forbidden") {
				code = r.Error
			}
			return seedFailure(0, code)
		}
		if r.View != nil {
			v := *r.View
			prepared = &v
		}
	}
	id, err := b.begin(ctx)
	if err != nil {
		if errors.Is(err, ErrBusy) {
			return seedFailure(0, "busy")
		}
		return seedFailure(0, "seed_unavailable")
	}
	defer func() { <-b.gate }()
	if req.Command == "seed_snapshot" && !b.poolEventPrepared(prepared) {
		return seedFailure(id, "stale_view")
	}
	work, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return b.seedRequestHeld(work, id, req)
}

// CardRequest discovers its exact lower authority through the pool route.
func (b *PoolBridge) CardRequest(ctx context.Context, req pilot.CardRequest) pilot.CardResponse {
	if b == nil || b.StableBridge == nil || !b.poolMode || !b.cardsEnabled {
		return cardFailure(0, "card_unavailable")
	}
	pre := req
	pre.ID = 1
	if _, err := pilot.MarshalCardRequest(pre); err != nil {
		return cardFailure(0, "invalid_request")
	}
	var prepared *pilot.ManifestViewPin
	if req.Command == "card_snapshot" {
		r := b.SeedRequest(ctx, pilot.SeedRequest{Command: "seed_snapshot"})
		if !r.OK {
			code := "card_unavailable"
			if oneOf(r.Error, "busy", "stale_view", "unauthorized", "forbidden") {
				code = r.Error
			}
			return cardFailure(0, code)
		}
		if r.View != nil {
			v := *r.View
			prepared = &v
		}
	}
	id, err := b.begin(ctx)
	if err != nil {
		if errors.Is(err, ErrBusy) {
			return cardFailure(0, "busy")
		}
		return cardFailure(0, "card_unavailable")
	}
	defer func() { <-b.gate }()
	if req.Command == "card_snapshot" && !b.poolEventPrepared(prepared) {
		return cardFailure(id, "stale_view")
	}
	work, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return b.cardRequestHeld(work, id, req)
}

// BoutRequest discovers its exact lower authority through the pool route.
func (b *PoolBridge) BoutRequest(ctx context.Context, req pilot.BoutRequest) pilot.BoutResponse {
	if b == nil || b.StableBridge == nil || !b.poolMode || !b.boutsEnabled {
		return boutFailure(0, "bout_unavailable")
	}
	pre := req
	pre.ID = 1
	if _, err := pilot.MarshalBoutRequest(pre); err != nil {
		return boutFailure(0, "invalid_request")
	}
	var prepared *pilot.ManifestViewPin
	if req.Command == "bout_snapshot" {
		r := b.CardRequest(ctx, pilot.CardRequest{Command: "card_snapshot"})
		if !r.OK {
			code := "bout_unavailable"
			if oneOf(r.Error, "busy", "stale_view", "unauthorized", "forbidden") {
				code = r.Error
			}
			return boutFailure(0, code)
		}
		if r.View != nil {
			v := *r.View
			prepared = &v
		}
	}
	id, err := b.begin(ctx)
	if err != nil {
		if errors.Is(err, ErrBusy) {
			return boutFailure(0, "busy")
		}
		return boutFailure(0, "bout_unavailable")
	}
	defer func() { <-b.gate }()
	if req.Command == "bout_snapshot" && !b.poolEventPrepared(prepared) {
		return boutFailure(id, "stale_view")
	}
	work, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return b.boutRequestHeld(work, id, req)
}

// ResultRequest discovers its exact lower authority through the pool route.
func (b *PoolBridge) ResultRequest(ctx context.Context, req pilot.ResultRequest) pilot.ResultResponse {
	if b == nil || b.StableBridge == nil || !b.poolMode || !b.resultsEnabled {
		return resultFailure(0, "result_unavailable")
	}
	pre := req
	pre.ID = 1
	if _, err := pilot.MarshalResultRequest(pre); err != nil {
		return resultFailure(0, "invalid_request")
	}
	var prepared *pilot.ManifestViewPin
	if req.Command == "result_snapshot" {
		r := b.BoutRequest(ctx, pilot.BoutRequest{Command: "bout_snapshot"})
		if !r.OK {
			code := "result_unavailable"
			if oneOf(r.Error, "busy", "stale_view", "unauthorized", "forbidden") {
				code = r.Error
			}
			return resultFailure(0, code)
		}
		if r.View != nil {
			v := *r.View
			prepared = &v
		}
	}
	id, err := b.begin(ctx)
	if err != nil {
		if errors.Is(err, ErrBusy) {
			return resultFailure(0, "busy")
		}
		return resultFailure(0, "result_unavailable")
	}
	defer func() { <-b.gate }()
	if req.Command == "result_snapshot" && !b.poolEventPrepared(prepared) {
		return resultFailure(id, "stale_view")
	}
	work, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	return b.resultRequestHeld(work, id, req)
}

func (b *PoolBridge) CareerRequest(ctx context.Context, req pilot.CareerRequest) pilot.CareerResponse {
	if b == nil || b.StableBridge == nil || !b.poolMode {
		return careerFailure(0, "career_inactive")
	}
	return b.careerRequest(ctx, req, b.ResultRequest)
}
func (b *PoolBridge) CareerHistoryRequest(ctx context.Context, req pilot.CareerHistoryRequest) pilot.CareerHistoryResponse {
	if b == nil || b.StableBridge == nil || !b.poolMode {
		return historyFailure(0, "career_inactive")
	}
	return b.careerHistoryRequest(ctx, req, b.ResultRequest)
}
func (b *PoolBridge) ExportResultProof(ctx context.Context, view, date, bid, path string) pilot.ResultResponse {
	return b.ResultRequest(ctx, pilot.ResultRequest{Command: "result_export", ViewID: view, Saturday: date, BoutID: bid, Path: path})
}

func (b *PoolBridge) BoutPortrait(ctx context.Context, view, date, bid, prospectID string) BoutPortraitResponse {
	if b == nil || b.StableBridge == nil || !b.poolMode {
		return BoutPortraitResponse{Version: 1, Error: "bout_unavailable"}
	}
	return b.StableBridge.BoutPortrait(ctx, view, date, bid, prospectID)
}

func (b *PoolBridge) CareerHistoryResult(ctx context.Context, view, job string, ordinal uint32) pilot.ResultResponse {
	if b == nil || b.StableBridge == nil || !b.poolMode {
		return resultFailure(0, "result_unavailable")
	}
	return b.StableBridge.CareerHistoryResult(ctx, view, job, ordinal)
}
