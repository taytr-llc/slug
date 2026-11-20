// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"net/url"
	"path/filepath"
	"sort"
	"time"

	"github.com/taytr-llc/slug/poolround"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/poolsettlefixture"
	"github.com/taytr-llc/slug/prospectservice"
)

func (r *PoolRuntime) readyRecord() (poolRuntimeReady, error) {
	core, e := r.coreRecord()
	if e != nil {
		return poolRuntimeReady{}, e
	}
	if len(r.recipients) != 2 {
		return poolRuntimeReady{}, ErrPoolRuntime
	}
	values := []PoolRecipientBinding{r.recipients["zero256"], r.recipients["incrementing256"]}
	if ValidatePoolRecipientBinding(values[0]) != nil || ValidatePoolRecipientBinding(values[1]) != nil || values[0].Config.AccountID == values[1].Config.AccountID {
		return poolRuntimeReady{}, ErrPoolRuntime
	}
	sort.Slice(values, func(i, j int) bool { return values[i].Config.Identity.GymID < values[j].Config.Identity.GymID })
	return poolRuntimeReady{1, core.Binding, values}, nil
}

// StartServices requires both wallets to have been opened and verified in this
// runtime epoch. Reopening public markers alone does not unlock or adopt them.
func (r *PoolRuntime) StartServices(ctx context.Context) error {
	work, end, e := r.begin(ctx)
	if e != nil {
		return e
	}
	defer end()
	r.mu.Lock()
	if r.ready {
		r.mu.Unlock()
		return nil
	}
	if !r.verified["zero256"] || !r.verified["incrementing256"] {
		r.mu.Unlock()
		return ErrPoolRuntime
	}
	ready, e := r.readyRecord()
	r.mu.Unlock()
	if e != nil {
		return e
	}
	if e = r.prepareServiceStores(work, ready); e != nil {
		return e
	}
	sessions := poolRuntimeSessions{r.accounts}
	poolOrigin, admissionOrigin := poolRuntimeOrigin(r.marker.PoolPort), poolRuntimeOrigin(r.marker.AdmissionPort)
	poolURL, _ := url.Parse(poolOrigin)
	admissionURL, _ := url.Parse(admissionOrigin)
	poolServer, e := poolservice.NewServer(poolservice.ServerConfig{Version: 1, Listen: poolURL.Host}, r.pool, r.settlement, sessions)
	if e != nil {
		return e
	}
	admission, e := prospectservice.NewFencedManifestAdmissionServer(prospectservice.ManifestServerConfig{Version: 3, Listen: admissionURL.Host, AuthorityPin: r.source.AuthorityPin, MaxJobs: r.source.MaxWorks, MaxRecords: 1000}, r.source.Policy, r.node, r.issuer, sessions)
	if e != nil {
		return e
	}
	life, cancel := context.WithCancel(r.life)
	r.mu.Lock()
	r.servicesCancel = cancel
	r.serviceFailed = false
	r.mu.Unlock()
	type serverResult struct {
		which  int
		origin string
		err    error
	}
	started := make(chan serverResult, 2)
	finished := make(chan serverResult, 2)
	run := func(which int, serve func(context.Context, func(string)) error) {
		r.services.Add(1)
		go func() {
			defer r.services.Done()
			err := serve(life, func(origin string) { started <- serverResult{which: which, origin: origin} })
			r.mu.Lock()
			r.serviceFailed = true
			fail := r.ready && !r.closed && r.life.Err() == nil
			if fail {
				r.lastError = "service_unavailable"
				r.cancel()
			}
			r.mu.Unlock()
			finished <- serverResult{which: which, err: err}
		}()
	}
	run(0, poolServer.Run)
	run(1, admission.Run)
	fail := func(e error) error { cancel(); r.services.Wait(); return errors.Join(ErrPoolRuntime, e) }
	seen := map[int]bool{}
	for len(seen) < 2 {
		select {
		case value := <-started:
			want := poolOrigin
			if value.which == 1 {
				want = admissionOrigin
			}
			if seen[value.which] || value.origin != want {
				return fail(ErrPoolRuntime)
			}
			seen[value.which] = true
		case value := <-finished:
			return fail(value.err)
		case <-work.Done():
			return fail(work.Err())
		}
	}
	r.mu.Lock()
	if r.closed || r.serviceFailed || work.Err() != nil {
		r.mu.Unlock()
		return fail(ErrPoolRuntime)
	}
	r.ready = true
	r.lastError = ""
	r.mu.Unlock()
	// One joined scheduler owns all accounting cycles. No node generate RPC is
	// used, and a deferred/prepared day is retried through RunDaily itself.
	r.services.Add(1)
	go func() { defer r.services.Done(); r.schedulePool(life) }()
	return nil
}

func (r *PoolRuntime) schedulePool(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	var cursor uint64
	for {
		work, stop := context.WithTimeout(ctx, 30*time.Second)
		e := r.maintainPool(work, &cursor)
		stop()
		r.mu.Lock()
		if e != nil && ctx.Err() == nil {
			r.lastError = "accounting_unavailable"
		} else if e == nil {
			r.lastError = ""
		}
		r.mu.Unlock()
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (r *PoolRuntime) maintainPool(ctx context.Context, cursor *uint64) error {
	summary, e := r.rounds.Sync(ctx)
	if e != nil {
		return e
	}
	if *cursor >= summary.Rounds {
		*cursor = 0
	}
	rows, e := r.rounds.List(ctx, *cursor, poolround.MaxPage)
	if e != nil {
		return e
	}
	for _, row := range rows {
		if _, e = r.rounds.Refresh(ctx, row.Round.BlockHash); e != nil {
			return e
		}
		*cursor = row.Round.Sequence
	}
	_, e = r.settlement.RunDaily(ctx)
	return e
}

// PrepareHostedRecoveryServices establishes every journal before the outer
// allocation/hosted READY markers. It launches no server or scheduler.
func (r *PoolRuntime) PrepareHostedRecoveryServices(ctx context.Context) error {
	work, end, e := r.begin(ctx)
	if e != nil {
		return e
	}
	defer end()
	if r.hosted == nil || r.hosted.allocation == nil || !r.verified["zero256"] || !r.verified["incrementing256"] {
		return ErrPoolRuntime
	}
	ready, e := r.readyRecord()
	if e != nil {
		return e
	}
	return r.prepareServiceStores(work, ready)
}
func (r *PoolRuntime) prepareServiceStores(ctx context.Context, ready poolRuntimeReady) error {
	var e error
	if r.ownedGym != nil {
		if e = r.prepareOwnedLeague(ctx); e != nil {
			return e
		}
	} else {
		if e = r.prepareLeague(ctx); e != nil {
			return e
		}
		if e = r.preparePipeline(ctx); e != nil {
			return e
		}
	}
	if r.backend == nil {
		r.backend, e = poolsettlefixture.New(ctx, r.nodes.rpcConfig())
		if e != nil {
			return e
		}
	}
	c := poolsettle.Config{Version: 1, ChangeScript: poolsettlefixture.PoolScript(), MaxRounds: 256, MaxBatches: 366}
	for _, b := range ready.Recipients {
		c.Destinations = append(c.Destinations, poolsettle.Destination{GymID: b.Config.Identity.GymID, Script: b.Recipient.Script})
	}
	path := filepath.Join(r.config.DataDir, "settlement")
	if r.settlement == nil {
		exists, err := poolRuntimeExists(path)
		if err != nil {
			return err
		}
		if exists {
			r.settlement, e = poolsettle.Open(path, c, r.rounds, r.backend)
		} else {
			r.settlement, e = poolsettle.Create(path, c, r.rounds, r.backend)
		}
		if e != nil {
			return e
		}
	}
	if e = writePoolRuntimeRecord(filepath.Join(r.config.DataDir, poolRuntimeReadyName), ready); e != nil {
		return e
	}

	return nil
}
