//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package prospectservice

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/taytr-llc/slug/registry"
)

func TestOperatorAdmissionPublicCapacityErrorBoundary(t *testing.T) {
	base, store, node, sessions, _ := mfSetup(t)
	s, err := NewPausedFencedManifestAdmissionServer(base.config, base.policy, node, store, sessions)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if err = s.EnableOperatorIntake(ctx); err != nil {
		t.Fatal(err)
	}
	// Same permit held by the real admission path while this helper classifies
	// its immediate registry error. Mixed causes must retain failure semantics.
	s.active <- struct{}{}
	failure := fencedAdmissionFailure(s, errors.Join(registry.ErrPublicCapacity, registry.ErrCorrupt))
	<-s.active
	if failure.code == "capacity" {
		t.Fatal("mixed storage failure became capacity")
	}
	if mode, e := s.OperatorIntakeMode(ctx); e != nil || mode != "enabled" {
		t.Fatal(mode, e)
	}
	s.active <- struct{}{}
	failure = fencedAdmissionFailure(s, registry.ErrPublicCapacity)
	<-s.active
	if failure.code != "capacity" || failure.unknown {
		t.Fatal("exact prewrite limit not classified", failure)
	}
	if mode, e := s.OperatorIntakeMode(ctx); e != nil || mode != "draining" {
		t.Fatal(mode, e)
	}
	base.active <- struct{}{}
	failure = fencedAdmissionFailure(base, registry.ErrPublicCapacity)
	<-base.active
	if failure.code != "rejected" || base.operatorIntake != nil {
		t.Fatal("legacy error behavior changed")
	}
}

func TestOperatorAdmissionPausedDrainAndHistoricalReceipt(t *testing.T) {
	base, store, node, sessions, body := mfSetup(t)
	s, err := NewPausedFencedManifestAdmissionServer(base.config, base.policy, node, store, sessions)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	before, err := store.Info()
	if err != nil {
		t.Fatal(err)
	}
	response := mbRequest(t, s, mbToken, "/v3/admissions", body)
	mbError(t, response, 503, "busy", false)
	after, err := store.Info()
	if err != nil || after.Records != before.Records || node.checks.Load() != 0 {
		t.Fatal("paused admission mutated/reached node", err)
	}
	if err = s.EnableOperatorIntake(ctx); err != nil {
		t.Fatal(err)
	}
	response = mbRequest(t, s, mbToken, "/v3/admissions", body)
	var first ManifestAdmissionResult
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &first) != nil || !first.Inserted {
		t.Fatal("original admission failed", response.Code)
	}
	if err = s.DrainOperatorIntake(ctx); err != nil {
		t.Fatal(err)
	}
	if err = s.EnableOperatorIntake(ctx); err != ErrIntakeDraining {
		t.Fatal("drain re-enabled", err)
	}
	node.guard = func(context.Context) error {
		t.Error("historical receipt used node")
		return errors.New("unexpected node")
	}
	response = mbRequest(t, s, mbToken, "/v3/admissions", body)
	var recovered ManifestAdmissionResult
	if response.Code != 200 || json.Unmarshal(response.Body.Bytes(), &recovered) != nil || recovered.Inserted || recovered.CurrentNodeChecked || recovered.Record.Verified.ProspectID != first.Record.Verified.ProspectID {
		t.Fatal("exact receipt lost in drain", response.Code)
	}
	// A different valid, still uncommitted proof remains refused. Reuse a
	// separate real fixture so rate limiting and old insertion cannot mask it.
	other, os, on, session, pending := mfSetup(t)
	closed, e := NewPausedFencedManifestAdmissionServer(other.config, other.policy, on, os, session)
	if e != nil {
		t.Fatal(e)
	}
	if e = closed.DrainOperatorIntake(ctx); e != nil {
		t.Fatal(e)
	}
	mbError(t, mbRequest(t, closed, mbToken, "/v3/admissions", pending), 503, "capacity", false)
	info, e := os.Info()
	if e != nil || info.Records != 0 || on.checks.Load() != 0 {
		t.Fatal("drain admitted new fighter", e)
	}
}

func TestOperatorAdmissionControllerJoinsAndCloses(t *testing.T) {
	base, store, node, sessions, _ := mfSetup(t)
	s, err := NewPausedFencedManifestAdmissionServer(base.config, base.policy, node, store, sessions)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// Hold the same permit acquired by ServeHTTP across proof/node/commit.
	s.active <- struct{}{}
	released := false
	defer func() {
		if !released {
			<-s.active
		}
	}()
	waiting, endWait := context.WithTimeout(ctx, 50*time.Millisecond)
	err = s.DrainOperatorIntake(waiting)
	endWait()
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("drain did not join admitted operation", err)
	}
	<-s.active
	released = true
	if err = s.DrainOperatorIntake(ctx); err != nil {
		t.Fatal(err)
	}
	s.stopAdmission()
	if _, err = s.OperatorIntakeMode(context.Background()); err != ErrIntakeUnavailable {
		t.Fatal("closed server controlled", err)
	}
	cancelled, end := context.WithCancel(context.Background())
	end()
	if err = s.EnableOperatorIntake(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
