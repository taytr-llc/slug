//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package prospectservice

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/taytr-llc/slug/registry"
)

// This serialized authorizer models the established callback contract. Real
// account generation/reset behavior is exercised by the actual-account suite.
type mfSessions struct {
	mu             sync.Mutex
	key            string
	revoked        bool
	afterOperation bool
}

func (s *mfSessions) WithSession(ctx context.Context, token string, fn func(string, func() error) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	check := func() error {
		if token != mbToken || s.revoked || ctx.Err() != nil {
			return errors.New("fixture original session unavailable")
		}
		return nil
	}
	if err := check(); err != nil {
		return err
	}
	err := fn(s.key, check)
	if err == nil && s.afterOperation {
		return errors.New("fixture acknowledgment unavailable")
	}
	return err
}

func mfSetup(t *testing.T) (*ManifestServer, *registry.ManifestStore, *mbNode, *mfSessions, any) {
	t.Helper()
	base, store, node, corpus, _ := mbSetup(t)
	job, err := store.IssueManifestJob(node.initial, corpus.Cases[0].Job.PublicKey, func() error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	proof, auth := mbProof(t, corpus.Cases[0], job, 0)
	sessions := &mfSessions{key: job.PublicKey}
	config := base.config
	config.Clients = nil
	server, err := NewFencedManifestAdmissionServer(config, base.policy, node, store, sessions)
	if err != nil {
		t.Fatal(err)
	}
	return server, store, node, sessions, mbSubmit(proof, auth)
}

func TestFencedManifestAdmissionOnlyRejectsIssuance(t *testing.T) {
	base, store, node, _, _ := mbSetup(t)
	sessions := &mfSessions{key: base.config.Clients[0].PublicKey}
	if _, err := NewFencedManifestAdmissionServer(base.config, base.policy, node, store, sessions); err == nil {
		t.Fatal("fenced admission accepted static credentials")
	}
	config := base.config
	config.Clients = nil
	for _, session := range []SessionAuthorizer{nil, (*mfSessions)(nil)} {
		if _, err := NewFencedManifestAdmissionServer(config, base.policy, node, store, session); err == nil {
			t.Fatal("fenced admission accepted a missing authorizer")
		}
	}
	server, err := NewFencedManifestAdmissionServer(config, base.policy, node, store, sessions)
	if err != nil {
		t.Fatal(err)
	}
	body := &securityUnreadableBody{}
	request := mbRawRequest("/v3/jobs", nil)
	request.Body = body
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	mbError(t, response, 400, "invalid_request", false)
	info, err := store.Info()
	if err != nil || info.Jobs != 0 || body.reads.Load() != 0 || node.resolves.Load() != 0 || node.checks.Load() != 0 {
		t.Fatal("disabled issuance reached work", err)
	}
}

func TestFencedManifestAdmissionNodeWorkReleasesOriginalLease(t *testing.T) {
	for _, change := range []string{"revoke", "replace-key", "cancel"} {
		t.Run(change, func(t *testing.T) {
			server, store, node, sessions, body := mfSetup(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			node.guard = func(context.Context) error {
				if !sessions.mu.TryLock() {
					t.Error("node eligibility ran under the account lease")
					return errors.New("long account lease")
				}
				defer sessions.mu.Unlock()
				switch change {
				case "revoke":
					sessions.revoked = true
				case "replace-key":
					sessions.key = "03" + sessions.key[2:]
				case "cancel":
					cancel()
				}
				return nil
			}
			raw, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			request := mbRawRequest("/v3/admissions", bytes.NewReader(raw)).WithContext(ctx)
			response := httptest.NewRecorder()
			server.ServeHTTP(response, request)
			if change == "cancel" {
				mbError(t, response, 503, "unavailable", false)
			} else {
				mbError(t, response, 401, "unauthorized", false)
			}
			info, err := store.Info()
			if err != nil || info.Records != 0 || node.checks.Load() != 1 {
				t.Fatal("old session committed after revoked node work", info.Records, err)
			}
		})
	}
}

func TestFencedManifestAdmissionUnknownCommitRecoversExactReceipt(t *testing.T) {
	server, store, node, sessions, body := mfSetup(t)
	node.guard = func(context.Context) error {
		if !sessions.mu.TryLock() {
			return errors.New("node work retained account lease")
		}
		sessions.afterOperation = true
		sessions.mu.Unlock()
		return nil
	}
	response := mbRequest(t, server, mbToken, "/v3/admissions", body)
	mbError(t, response, 503, "outcome_unknown", true)
	info, err := store.Info()
	if err != nil || info.Records != 1 {
		t.Fatal("fixture did not commit before lost authorization acknowledgment", err)
	}
	sessions.mu.Lock()
	sessions.afterOperation = false
	sessions.mu.Unlock()
	node.guard = func(context.Context) error {
		t.Error("historical recovery consulted node")
		return errors.New("unexpected node")
	}
	response = mbRequest(t, server, mbToken, "/v3/admissions", body)
	var recovered ManifestAdmissionResult
	if err := json.Unmarshal(response.Body.Bytes(), &recovered); err != nil || response.Code != 200 || recovered.Inserted || recovered.CurrentNodeChecked || recovered.ContextSource != "historical-receipt" {
		t.Fatal("historical recovery unavailable", response.Code, err)
	}
	original, err := store.GetManifestAdmission(recovered.Record.Verified.ProspectID)
	if err != nil || !reflect.DeepEqual(original, recovered.Record) || node.checks.Load() != 1 {
		t.Fatal("recovery changed original receipt or queried current node", err)
	}
}

func TestFencedManifestAdmissionBusyCommitDoesNotWaitUnderLease(t *testing.T) {
	server, store, node, sessions, body := mfSetup(t)
	entered, release, issuerDone := make(chan struct{}), make(chan struct{}), make(chan error, 1)
	var once sync.Once
	defer once.Do(func() { close(release) })
	node.guard = func(context.Context) error {
		go func() {
			_, err := store.IssueManifestJob(node.initial, sessions.key, func() error {
				close(entered)
				<-release
				return nil
			})
			issuerDone <- err
		}()
		select {
		case <-entered:
			return nil
		case <-time.After(2 * time.Second):
			return errors.New("fixture issuer did not enter")
		}
	}
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- mbRequest(t, server, mbToken, "/v3/admissions", body) }()
	select {
	case response := <-done:
		mbError(t, response, 503, "busy", false)
	case <-time.After(3 * time.Second):
		t.Fatal("commit waited for the registry while retaining the account lease")
	}
	if !sessions.mu.TryLock() {
		t.Fatal("busy registry left the original account lease occupied")
	}
	sessions.revoked = true
	sessions.mu.Unlock()
	once.Do(func() { close(release) })
	if err := <-issuerDone; err != nil {
		t.Fatal(err)
	}
	info, err := store.Info()
	if err != nil || info.Records != 0 {
		t.Fatal("busy commit published a receipt", err)
	}
}

func TestFencedManifestAdmissionHistoricalReference(t *testing.T) {
	f := mbLoad(t)
	store, _ := mbSeed(t, f)
	tc := f.Cases[0]
	node := &mbNode{initial: tc.Job.Context, source: "current-parent"}
	config := mbConfig(f.Pin, tc.Job.PublicKey)
	config.Clients = nil
	server, err := NewFencedManifestAdmissionServer(config, mbPolicy(tc.Job.Context), node, store, &mfSessions{key: tc.Job.PublicKey})
	if err != nil {
		t.Fatal(err)
	}
	response := mbRequest(t, server, mbToken, "/v3/admissions", mbSubmit(tc.Proof, tc.Authorization))
	var result ManifestAdmissionResult
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || response.Code != 200 || result.Inserted || result.CurrentNodeChecked || !reflect.DeepEqual(result.Record, tc.Record) || node.checks.Load() != 0 {
		t.Fatal("fenced historical response differs from independent original", response.Code, err)
	}
}
