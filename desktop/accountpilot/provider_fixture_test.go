//go:build desktopintegration && !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.

package accountpilot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymprovider"
)

// This provider is test support, not an adapter. Its separate synced file is
// the simulated credential authority. It stores public fixture identifiers and
// immutable authentication/reset facts, never credentials, JWTs or sessions.
// Deterministic IDs and published RSA signing are deliberately fixture-only.
type durableProviderFixture struct {
	t    *testing.T
	path string
	sign func(string, map[string]any) string
	data providerFixtureState
}

type providerFixtureState struct {
	Version    uint32
	Config     gymprovider.Config
	Serial     uint64
	Epochs     map[string]uint64
	Auth       map[string]providerFixtureAuth
	Operations map[string]providerFixtureOperation
}

type providerFixtureAuth struct {
	ID, Subject string
	Flow        gymprovider.LoginFlow
	Epoch       uint64
	At          int64
}

type providerFixtureOperation struct {
	ID, Subject, AccountID string
	Request                gymprovider.ResetRequest
	From, To               uint64
	VerifiedAt             int64
	Commits                uint64
	Completed              bool
}

func newDurableProviderFixture(t *testing.T, path string, cfg gymprovider.Config, sign func(string, map[string]any) string, reopen bool) *durableProviderFixture {
	t.Helper()
	p := &durableProviderFixture{t: t, path: path, sign: sign}
	if reopen {
		raw, err := os.ReadFile(path)
		if err != nil || json.Unmarshal(raw, &p.data) != nil || p.data.Version != 1 || !reflect.DeepEqual(p.data.Config, cfg) {
			t.Fatal("cannot reopen fixture provider state", err)
		}
		return p
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("fixture provider state must be new")
	}
	p.data = providerFixtureState{Version: 1, Config: cfg, Epochs: map[string]uint64{"zero256": 1, "incrementing256": 1}, Auth: make(map[string]providerFixtureAuth), Operations: make(map[string]providerFixtureOperation)}
	p.persist()
	return p
}

func (p *durableProviderFixture) persist() {
	p.t.Helper()
	raw, err := json.Marshal(p.data)
	if err != nil {
		p.t.Fatal(err)
	}
	f, err := os.CreateTemp(filepath.Dir(p.path), "provider-fixture-")
	if err != nil {
		p.t.Fatal(err)
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(raw)
	}
	if err == nil {
		err = f.Sync()
	}
	err = errors.Join(err, f.Close())
	if err == nil {
		err = os.Rename(f.Name(), p.path)
	}
	if err != nil {
		p.t.Fatal("fixture provider state write failed", err)
	}
	dir, err := os.Open(filepath.Dir(p.path))
	if err != nil {
		p.t.Fatal(err)
	}
	if err = errors.Join(dir.Sync(), dir.Close()); err != nil {
		p.t.Fatal(err)
	}
}

func (p *durableProviderFixture) id(kind string) string {
	p.data.Serial++
	digest := sha256.Sum256([]byte(fmt.Sprintf("SLUG public provider fixture v1\x00%s\x00%d", kind, p.data.Serial)))
	return hex.EncodeToString(digest[:])
}

func (p *durableProviderFixture) common(coordinator, authority, subject string) map[string]any {
	now := time.Now().Unix()
	return map[string]any{"iss": gymprovider.FixtureIssuer, "sub": subject, "iat": now, "exp": now + 120, "provider_version": 1, "coordinator_id": coordinator, "authority_id": authority, "tenant_id": p.data.Config.TenantID, "client_id": p.data.Config.ClientID, "connection_id": p.data.Config.ConnectionID}
}

func (p *durableProviderFixture) fresh(flow gymprovider.LoginFlow, subject string) providerFixtureAuth {
	p.t.Helper()
	epoch, ok := p.data.Epochs[subject]
	if !ok {
		p.t.Fatal("unknown public provider fixture")
	}
	a := providerFixtureAuth{ID: p.id("authentication"), Subject: subject, Flow: flow, Epoch: epoch, At: time.Now().Unix()}
	p.data.Auth[a.ID] = a
	p.persist() // Original epoch/time/flow are durable before any assertion.
	return a
}

func (p *durableProviderFixture) loginEvidence(flow gymprovider.LoginFlow, authID, mode string) string {
	p.t.Helper()
	a, ok := p.data.Auth[authID]
	if !ok {
		p.t.Fatal("unknown original authentication")
	}
	claims := p.common(flow.CoordinatorID, flow.AuthorityID, a.Subject)
	claims["aud"] = "slug-provider-fixture-login"
	claims["auth_time"], claims["slug_auth_epoch"] = a.At, a.Epoch
	claims["nonce"], claims["login_flow_id"] = flow.Nonce, flow.ID
	claims["original_auth_id"], claims["original_flow_id"], claims["auth_mode"] = a.ID, a.Flow.ID, mode
	return p.sign("JWT", claims)
}

func (p *durableProviderFixture) login(ctx context.Context, c *gymprovider.Coordinator, subject string) (gymaccount.Session, providerFixtureAuth) {
	p.t.Helper()
	f, err := c.IssueLogin(ctx)
	if err != nil {
		p.t.Fatal(err)
	}
	a := p.fresh(f, subject)
	s, err := c.FinishLogin(ctx, f.ID, p.loginEvidence(f, a.ID, "fresh"))
	if err != nil {
		p.t.Fatal("coordinated fixture login", err)
	}
	return s, a
}

func (p *durableProviderFixture) verifiedReset(req gymprovider.ResetRequest, subject, accountID string) providerFixtureOperation {
	p.t.Helper()
	from, ok := p.data.Epochs[subject]
	if !ok || from == ^uint64(0) {
		p.t.Fatal("invalid fixture credential epoch")
	}
	op := providerFixtureOperation{ID: p.id("credential-operation"), Subject: subject, AccountID: accountID, Request: req, From: from, To: from + 1, VerifiedAt: time.Now().Unix()}
	p.data.Operations[op.ID] = op
	p.persist() // Simulated verified challenge precedes the coordinator begin.
	return op
}

func (p *durableProviderFixture) resetEvidence(opID, stage string) string {
	p.t.Helper()
	op, ok := p.data.Operations[opID]
	if !ok || (stage == "complete" && op.Commits != 1) {
		p.t.Fatal("fixture completion requires a durable credential commit")
	}
	claims := p.common(op.Request.CoordinatorID, op.Request.AuthorityID, op.Subject)
	claims["aud"], claims["event_id"], claims["stage"] = "slug-provider-fixture-reset", op.Request.EventID, stage
	claims["from_epoch"], claims["to_epoch"], claims["verified_at"] = op.From, op.To, op.VerifiedAt
	claims["reset_nonce"], claims["provider_operation_id"] = op.Request.Nonce, op.ID
	return p.sign("slug-reset+jwt", claims)
}

func (p *durableProviderFixture) begin(ctx context.Context, c *gymprovider.Coordinator, opID string) (gymprovider.ResetResult, error) {
	op := p.data.Operations[opID]
	return c.BeginReset(ctx, op.Request.EventID, p.resetEvidence(opID, "begin"))
}

// A permit is usable only for its exact operation, and only once in this
// simulated provider. A completed historical receipt never authorizes mutation.
func (p *durableProviderFixture) commit(opID string, ack gymprovider.ResetResult) error {
	op, ok := p.data.Operations[opID]
	if !ok || ack.Version != 1 || ack.CoordinatorID != op.Request.CoordinatorID || ack.AuthorityID != op.Request.AuthorityID || ack.EventID != op.Request.EventID || ack.ProviderOperationID != op.ID || ack.Phase != "pending" || !ack.CanCommit || ack.Barrier.Status != "pending" || ack.Barrier.AccountID != op.AccountID || ack.Barrier.FromEpoch != op.From || ack.Barrier.ToEpoch != op.To || ack.Barrier.VerifiedAtMS != op.VerifiedAt*1000 {
		return errors.New("fixture provider denied unbound or historical permit")
	}
	if op.Commits == 1 {
		return nil
	}
	if p.data.Epochs[op.Subject] != op.From {
		return errors.New("fixture credential compare-and-swap failed")
	}
	p.data.Epochs[op.Subject], op.Commits = op.To, 1
	p.data.Operations[op.ID] = op
	p.persist() // One atomic credential mutation and consumed-operation record.
	return nil
}

func (p *durableProviderFixture) complete(ctx context.Context, c *gymprovider.Coordinator, opID string) (gymprovider.ResetResult, error) {
	op := p.data.Operations[opID]
	result, err := c.CompleteReset(ctx, op.Request.EventID, p.resetEvidence(opID, "complete"))
	if err != nil {
		return result, err
	}
	if result.Version != 1 || result.CoordinatorID != op.Request.CoordinatorID || result.AuthorityID != op.Request.AuthorityID || result.EventID != op.Request.EventID || result.ProviderOperationID != op.ID || result.Phase != "completed" || result.CanCommit || result.Barrier.Status != "completed" || result.Barrier.AccountID != op.AccountID || result.Barrier.FromEpoch != op.From || result.Barrier.ToEpoch != op.To || result.Barrier.VerifiedAtMS != op.VerifiedAt*1000 {
		return gymprovider.ResetResult{}, errors.New("fixture completion acknowledgement mismatch")
	}
	op.Completed = true
	p.data.Operations[op.ID] = op
	p.persist()
	return result, nil
}
