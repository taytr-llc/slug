//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"testing"
	"time"
)

func TestPoolPipelineTaggedClockCoversActivationAndReopen(t *testing.T) {
	r := poolLeagueTest(t)
	now := time.Now().Add(48 * time.Hour).Truncate(time.Millisecond)
	r.leagueFactories = poolLeagueTestFactories(func() time.Time { return now })
	ctx := context.Background()
	if e := r.prepareLeague(ctx); e != nil {
		t.Fatal(e)
	}
	if e := r.preparePipeline(ctx); e != nil {
		t.Fatal(e)
	}
	p := r.pipeline.Prepared
	for _, s := range []poolPipelineStage{p.Seed, p.Card, p.Bout, p.Result} {
		if s.CreatedMS != now.UnixMilli() {
			t.Fatal("factory retained real clock", s)
		}
	}
	if r.pipeline.Career.ActivatedAtMS != now.UnixMilli() {
		t.Fatal("activation retained real clock")
	}
	original := *r.pipeline
	pipelineRelease(t, r)
	now = now.Add(7 * 24 * time.Hour)
	if e := r.prepareLeague(ctx); e != nil {
		t.Fatal(e)
	}
	if e := r.preparePipeline(ctx); e != nil {
		t.Fatal(e)
	}
	if *r.pipeline != original {
		t.Fatal("calendar advance replaced original activation")
	}
}
