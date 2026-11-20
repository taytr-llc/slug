// Copyright (c) 2026 The SLUG developers. ISC license.
package poolshare

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

func TestOperatorAdmissionPublicCapacityOriginalGateOnly(t *testing.T) {
	h := setupPool(t, "", 8)
	ctx := context.Background()
	a, err := h.pool.AttachOperatorAdmission(ctx, h.config, 4)
	if err != nil {
		t.Fatal(err)
	}
	if err = a.Enable(ctx); err != nil {
		t.Fatal(err)
	}
	before := h.pool.meta
	// Exercise the original operation wrapper at its concrete issuer error
	// boundary; registry tests separately establish the sentinel's provenance.
	mixed := errors.Join(registry.ErrPublicCapacity, registry.ErrCorrupt)
	err = h.pool.withSession(ctx, poolTestToken, func(context.Context, string, *poolSession) error { return mixed })
	if err != mixed {
		t.Fatal("mixed issuer failure altered", err)
	}
	if status, e := a.Status(ctx); e != nil || status.Mode != "enabled" {
		t.Fatal("mixed integrity error became drain", status, e)
	}
	err = h.pool.withSession(ctx, poolTestToken, func(context.Context, string, *poolSession) error { return registry.ErrPublicCapacity })
	if err != registry.ErrPublicCapacity || h.pool.meta != before {
		t.Fatal("public capacity changed original result/evidence", err)
	}
	if status, e := a.Status(ctx); e != nil || status.Mode != "draining" {
		t.Fatal("attached issuer capacity did not close intake", status, e)
	}
	legacy := setupPool(t, "", 8)
	err = legacy.pool.withSession(ctx, poolTestToken, func(context.Context, string, *poolSession) error { return registry.ErrPublicCapacity })
	if err != registry.ErrPublicCapacity || legacy.pool.operatorAdmission != nil {
		t.Fatal("legacy owner silently attached", err)
	}
}

func TestOperatorAdmissionFiniteRowsAndSavedRecovery(t *testing.T) {
	h := setupPool(t, "", 8)
	ctx := context.Background()
	a, err := h.pool.AttachOperatorAdmission(ctx, h.config, 2)
	if err != nil {
		t.Fatal(err)
	}
	before := h.pool.meta
	if _, err = h.pool.Issue(ctx, poolTestToken); err != ErrBusy {
		t.Fatal("paused issue", err)
	}
	if _, err = h.pool.Submit(ctx, poolTestToken, h.source.Job.ID, h.ordinary.Nonce); err != ErrBusy {
		t.Fatal("paused submit", err)
	}
	if h.pool.meta != before || h.node.currents != 0 || h.node.broadcasts != 0 {
		t.Fatal("paused intake had effects")
	}
	if err = a.Enable(ctx); err != nil {
		t.Fatal(err)
	}
	// Two distinct frozen nonces in the SAME job reserve separate rows.
	first, err := h.pool.Submit(ctx, poolTestToken, h.source.Job.ID, h.ordinary.Nonce)
	if err != nil || first.AcceptedSequence != 1 {
		t.Fatal(first, err)
	}
	h.now = h.source.Job.ExpiresAtMS
	late, err := h.pool.Submit(ctx, poolTestToken, h.source.Job.ID, h.winner.Nonce)
	if err != nil || late.Sequence != 2 || late.AcceptedSequence != 0 {
		t.Fatal(late, err)
	}
	before = h.pool.meta
	broadcasts, currents := h.node.broadcasts, h.node.currents
	if _, err = h.pool.Submit(ctx, poolTestToken, h.source.Job.ID, 17); err != ErrCapacity {
		t.Fatal("N+1 admitted", err)
	}
	if _, err = h.pool.Issue(ctx, poolTestToken); err != ErrCapacity {
		t.Fatal("full source issued", err)
	}
	if h.pool.meta != before || h.node.broadcasts != broadcasts || h.node.currents != currents {
		t.Fatal("capacity refusal reached retention/node")
	}
	h.reopen(t)
	if _, err = a.Status(ctx); err != ErrClosed {
		t.Fatal("old attachment rebound", err)
	}
	a, err = h.pool.AttachOperatorAdmission(ctx, h.config, 2)
	if err != nil {
		t.Fatal(err)
	}
	h.node.canonicalErr = nil
	h.now++
	recovered, err := h.pool.Submit(ctx, poolTestToken, h.source.Job.ID, h.winner.Nonce)
	if err != nil || recovered.AcceptedSequence != 2 || recovered.Sequence != 2 || recovered.WorkHash != late.WorkHash {
		t.Fatal("full journal blocked exact late winner", recovered, err)
	}
	rows, err := h.pool.AccountingPage(ctx, 0, 0, 2)
	if err != nil || len(rows.Entries) != 2 || rows.Entries[0].Share.DuplicateKey != first.DuplicateKey || rows.Entries[1].Share.DuplicateKey != late.DuplicateKey {
		t.Fatal("accepted prefix changed", err)
	}
	status, err := a.Status(ctx)
	if err != nil || status.Mode != "draining" || status.Submissions != 2 || status.OverBound {
		t.Fatal(status, err)
	}
	if err = a.Enable(ctx); err != ErrCapacity {
		t.Fatal("re-enabled drained source", err)
	}
}

func TestOperatorAdmissionOriginalBindingCountersAndLegacyHold(t *testing.T) {
	h := setupPool(t, "", 8)
	ctx := context.Background()
	wrong := h.config
	wrong.MaxWorks++
	if _, err := h.pool.AttachOperatorAdmission(ctx, wrong, 2); err != ErrConfig {
		t.Fatal(err)
	}
	if _, err := h.pool.Submit(ctx, poolTestToken, h.source.Job.ID, h.ordinary.Nonce); err != nil {
		t.Fatal(err)
	}
	if _, err := h.pool.Submit(ctx, poolTestToken, h.source.Job.ID, h.winner.Nonce); err != nil {
		t.Fatal(err)
	}
	a, err := h.pool.AttachOperatorAdmission(ctx, h.config, 1)
	if err != nil {
		t.Fatal(err)
	}
	status, err := a.Status(ctx)
	if err != nil || !status.OverBound || status.Mode != "draining" {
		t.Fatal(status, err)
	}
	if err = a.Enable(ctx); err != ErrCapacity {
		t.Fatal("old journal adopted larger authority", err)
	}
	if _, err = h.pool.AttachOperatorAdmission(ctx, h.config, 8); err != ErrConfig {
		t.Fatal("replaced ceiling", err)
	}
	if _, err = h.pool.Lookup(ctx, poolTestToken, h.source.Job.ID, h.ordinary.Nonce); err != nil {
		t.Fatal(err)
	}
	h.reopen(t)
	// Original Open already audited evidence. Attachment independently refuses
	// physical counter divergence rather than adopting newly read metadata.
	h.pool.meta.Bytes++
	if _, err = h.pool.AttachOperatorAdmission(ctx, h.config, 2); err != ErrCorrupt {
		t.Fatal("counter mismatch accepted", err)
	}
}

func TestOperatorAdmissionDrainJoinsOriginalWriter(t *testing.T) {
	h := setupPool(t, "", 8)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a, err := h.pool.AttachOperatorAdmission(ctx, h.config, 4)
	if err != nil {
		t.Fatal(err)
	}
	if err = a.Enable(ctx); err != nil {
		t.Fatal(err)
	}
	entered, release, writeDone := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	var writeErr error
	defer func() { once.Do(func() { close(release) }); cancel(); <-writeDone }()
	h.node.onCurrent = func() { close(entered); <-release }
	go func() {
		defer close(writeDone)
		_, writeErr = h.pool.Submit(ctx, poolTestToken, h.source.Job.ID, h.ordinary.Nonce)
	}()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("writer did not enter")
	}
	waiting, endWait := context.WithTimeout(ctx, 50*time.Millisecond)
	err = a.Drain(waiting)
	endWait()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("drain did not wait for original writer", err)
	}
	once.Do(func() { close(release) })
	<-writeDone
	if writeErr != nil {
		t.Fatal(writeErr)
	}
	if err = a.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = h.pool.Submit(ctx, poolTestToken, h.source.Job.ID, h.winner.Nonce); err != ErrCapacity {
		t.Fatal(err)
	}
	if _, err = h.pool.Lookup(ctx, poolTestToken, h.source.Job.ID, h.ordinary.Nonce); err != nil {
		t.Fatal("drain closed receipt read", err)
	}
	stopped, end := context.WithCancel(ctx)
	end()
	if err = a.Enable(stopped); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestOperatorAdmissionCheapRefusal(t *testing.T) {
	if !wire.TestPoW {
		t.Skip("cheap profile only")
	}
	if a, err := (*Pool)(nil).AttachOperatorAdmission(context.Background(), Config{}, 0); a != nil || err != ErrProfile {
		t.Fatal(a, err)
	}
}
