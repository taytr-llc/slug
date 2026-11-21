//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package prospectservice

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/registry"
)

// The original persisted record count, not a remembered server drain flag,
// enforces the same selected MaxRecords after physical issuer reopen. This is
// a conditional one-record service limit; it does not model exhausting a signed
// public-authority cap or change the independent fixture's authority limits.
func TestDailyFencedAdmissionLiveCapacityReopen(t *testing.T) {
	base, store, node, corpus, path := mbSetup(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	job, err := store.IssueManifestJob(node.initial, corpus.Cases[0].Job.PublicKey, func() error { return ctx.Err() })
	if err != nil {
		t.Fatal(err)
	}
	proof, auth := mbProof(t, corpus.Cases[0], job, 0)
	otherProof, otherAuth := mbProof(t, corpus.Cases[0], job, 1)
	firstVerified, err := prospect.Verify(job.Context, proof)
	if err != nil || gymauth.VerifyAuthorization(job.Context, proof, auth) != nil {
		t.Fatal("first original normal-PoW proof", err)
	}
	otherVerified, err := prospect.Verify(job.Context, otherProof)
	if err != nil || gymauth.VerifyAuthorization(job.Context, otherProof, otherAuth) != nil || firstVerified.ProspectID == otherVerified.ProspectID || firstVerified.WorkHash == otherVerified.WorkHash {
		t.Fatal("distinct valid same-job normal-PoW proof", err)
	}
	config := base.config
	config.Clients = nil
	config.MaxRecords = 1
	sessions := &mfSessions{key: job.PublicKey}
	server, err := NewPausedDailyFencedManifestAdmissionServer(config, base.policy, node, store, sessions)
	if err != nil {
		t.Fatal(err)
	}
	if err = server.EnableOperatorIntake(ctx); err != nil {
		t.Fatal(err)
	}
	w := mbRequest(t, server, mbToken, "/v3/admissions", mbSubmit(proof, auth))
	var first ManifestAdmissionResult
	if err = json.Unmarshal(w.Body.Bytes(), &first); err != nil || w.Code != 200 || !first.Inserted || !first.CurrentNodeChecked || first.Record.Job != job || first.Record.Sequence != 1 || first.Record.Verified != firstVerified {
		t.Fatal("first selected admission", w.Body.String(), err)
	}
	before, err := store.Info()
	if err != nil || before.Jobs != 1 || before.Records != 1 || node.checks.Load() != 1 {
		t.Fatal("original admitted count", before, err)
	}
	// No explicit drain or second request is made before closing. The new
	// server starts paused and cannot inherit any old in-memory capacity state.
	if err = store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := registry.OpenManifestAuthority(path, config.AuthorityPin, false)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := reopened.Close(); err != nil {
			t.Error(err)
		}
	}()
	offline := &mbNode{initial: node.initial, source: "current-parent", guard: func(context.Context) error {
		return errors.New("capacity and history must not contact the node")
	}}
	restarted, err := NewPausedDailyFencedManifestAdmissionServer(config, base.policy, offline, reopened, sessions)
	if err != nil {
		t.Fatal(err)
	}
	mode, err := restarted.OperatorIntakeMode(ctx)
	if err != nil || mode != "paused" {
		t.Fatal("reopened selected owner adopted old mode", mode, err)
	}
	if err = restarted.EnableOperatorIntake(ctx); err != nil {
		t.Fatal(err)
	}
	if time.Now().UnixMilli() >= job.ExpiresAtMS {
		t.Fatal("live capacity control exceeded the original job lifetime")
	}
	mbError(t, mbRequest(t, restarted, mbToken, "/v3/admissions", mbSubmit(otherProof, otherAuth)), 503, "capacity", false)
	mode, err = restarted.OperatorIntakeMode(ctx)
	if err != nil || mode != "draining" || restarted.EnableOperatorIntake(ctx) != ErrIntakeDraining {
		t.Fatal("persisted count did not establish permanent new-owner drain", mode, err)
	}
	if _, err = reopened.GetManifestAdmission(otherVerified.ProspectID); err != registry.ErrNotFound {
		t.Fatal("live capacity refusal created another birth", err)
	}
	w = mbRequest(t, restarted, mbToken, "/v3/admissions", mbSubmit(proof, auth))
	var historical ManifestAdmissionResult
	if err = json.Unmarshal(w.Body.Bytes(), &historical); err != nil || w.Code != 200 || historical.Inserted || historical.CurrentNodeChecked || historical.ContextSource != "historical-receipt" || !reflect.DeepEqual(historical.Record, first.Record) {
		t.Fatal("full reopened owner changed original receipt", w.Body.String(), err)
	}
	after, err := reopened.Info()
	if err != nil || after != before || offline.resolves.Load() != 0 || offline.checks.Load() != 0 {
		t.Fatal("capacity/history changed counts or reached node", after, err)
	}
}
