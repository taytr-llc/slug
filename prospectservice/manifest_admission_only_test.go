//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package prospectservice

import (
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"testing"
)

func TestManifestAdmissionOnlyRejectsIssuanceBeforeAuthentication(t *testing.T) {
	original, store, node, _, _ := mbSetup(t)
	cfg := original.config
	sessions := &mbSessions{key: cfg.Clients[0].PublicKey}
	if _, err := NewManifestAdmissionServer(cfg, original.policy, node, store, sessions); err == nil {
		t.Fatal("admission-only authority accepted static credential fallback")
	}
	cfg.Clients = nil
	if _, err := NewManifestAdmissionServer(cfg, original.policy, node, store, nil); err == nil {
		t.Fatal("admission-only authority accepted missing session authority")
	}
	server, err := NewManifestAdmissionServer(cfg, original.policy, node, store, sessions)
	if err != nil {
		t.Fatal(err)
	}
	before, err := store.Info()
	if err != nil {
		t.Fatal(err)
	}
	body := &securityUnreadableBody{}
	request := mbRawRequest("/v3/jobs", nil)
	request.Body = body
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	mbError(t, response, 400, "invalid_request", false)
	after, err := store.Info()
	if err != nil || before.Jobs != after.Jobs || body.reads.Load() != 0 || sessions.calls.Load() != 0 || node.resolves.Load() != 0 {
		t.Fatal("disabled issuance reached body, authentication, node or registry", err)
	}
}

func TestManifestAdmissionOnlyRecoversOriginalIssuerReceipt(t *testing.T) {
	f := mbLoad(t)
	store, _ := mbSeed(t, f)
	tc := f.Cases[0]
	node := &mbNode{initial: tc.Job.Context, source: "current-parent"}
	cfg := mbConfig(f.Pin, tc.Job.PublicKey)
	cfg.Clients = nil
	sessions := &mbSessions{key: tc.Job.PublicKey}
	server, err := NewManifestAdmissionServer(cfg, mbPolicy(tc.Job.Context), node, store, sessions)
	if err != nil {
		t.Fatal(err)
	}
	before, err := store.Info()
	if err != nil {
		t.Fatal(err)
	}
	w := mbRequest(t, server, mbToken, "/v3/admissions", mbSubmit(tc.Proof, tc.Authorization))
	var result ManifestAdmissionResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil || w.Code != 200 || result.Inserted || result.CurrentNodeChecked || result.ContextSource != "historical-receipt" || !reflect.DeepEqual(result.Record, tc.Record) {
		t.Fatal("original issuer receipt was not recovered unchanged", w.Code, err)
	}
	after, err := store.Info()
	if err != nil || before.Jobs != after.Jobs || before.Records != after.Records || node.resolves.Load() != 0 || node.checks.Load() != 0 {
		t.Fatal("historical recovery issued work, changed records or queried current node", err)
	}
	sessions.revoked.Store(true)
	w = mbRequest(t, server, mbToken, "/v3/admissions", mbSubmit(tc.Proof, tc.Authorization))
	mbError(t, w, 401, "unauthorized", false)
}
