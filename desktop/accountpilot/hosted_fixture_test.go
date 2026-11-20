// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymidentity"
	"github.com/taytr-llc/slug/gymprovider"
)

func hostedReference(t *testing.T) struct {
	Records []struct {
		Name, Kind, CanonicalUTF8 string
		Value                     json.RawMessage `json:"value"`
		Canonical                 string          `json:"canonical_utf8"`
	} `json:"records"`
	Tokens []struct {
		Name, Kind string
		Accepted   bool   `json:"fixture_verifier_accepted"`
		Claims     string `json:"claims_utf8"`
		Token      string `json:"compact_jws"`
	} `json:"tokens"`
	Negatives []struct {
		Name, Kind string
		Value      json.RawMessage `json:"value"`
	} `json:"negatives"`
	Raw []struct {
		Name, Kind string
		Valid      bool
		Hex        string `json:"raw_hex"`
	} `json:"raw_cases"`
} {
	t.Helper()
	var out struct {
		Records []struct {
			Name, Kind, CanonicalUTF8 string
			Value                     json.RawMessage `json:"value"`
			Canonical                 string          `json:"canonical_utf8"`
		} `json:"records"`
		Tokens []struct {
			Name, Kind string
			Accepted   bool   `json:"fixture_verifier_accepted"`
			Claims     string `json:"claims_utf8"`
			Token      string `json:"compact_jws"`
		} `json:"tokens"`
		Negatives []struct {
			Name, Kind string
			Value      json.RawMessage `json:"value"`
		} `json:"negatives"`
		Raw []struct {
			Name, Kind string
			Valid      bool
			Hex        string `json:"raw_hex"`
		} `json:"raw_cases"`
	}
	raw, e := os.ReadFile("../../testdata/hosted-current-recovery/provider.json")
	if e != nil {
		t.Fatal(e)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "c376069f80b9a0cf5490e7d7d48cc2c65089f277c6f35888a660b2e9eba6e091" {
		t.Fatal("independent corpus changed")
	}
	if json.Unmarshal(raw, &out) != nil {
		t.Fatal("corpus")
	}
	return out
}
func TestHostedFixtureIndependentProvider(t *testing.T) {
	corpus := hostedReference(t)
	var cfg gymprovider.Config
	for _, r := range corpus.Records {
		if r.Name == "config-coordinator" {
			if json.Unmarshal(r.Value, &cfg) != nil {
				t.Fatal("config")
			}
		}
	}
	accounts := map[string]string{"zero256": strings.Repeat("51", 32), "incrementing256": strings.Repeat("52", 32)}
	coordinator := strings.Repeat("22", 32)
	for _, r := range corpus.Records {
		if r.Kind != "driver_state" {
			continue
		}
		t.Run(r.Name, func(t *testing.T) {
			var s hostedDriverState
			if json.Unmarshal(r.Value, &s) != nil {
				t.Fatal("decode")
			}
			if e := auditHostedDriver(s, cfg, coordinator, accounts); e != nil {
				t.Fatal(e)
			}
			raw, _ := json.Marshal(s)
			if string(append(raw, '\n')) != r.Canonical {
				t.Fatal("canonical independent bytes differ")
			}
		})
	}
	for _, r := range corpus.Negatives {
		if r.Kind != "driver_state" {
			continue
		}
		t.Run(r.Name, func(t *testing.T) {
			var s hostedDriverState
			e := json.Unmarshal(r.Value, &s)
			if e == nil {
				e = auditHostedDriver(s, cfg, coordinator, accounts)
			}
			if e == nil {
				t.Fatal("accepted independent invalid state")
			}
		})
	}
	for _, r := range corpus.Raw {
		if r.Kind != "driver" {
			continue
		}
		t.Run(r.Name, func(t *testing.T) {
			raw, e := hex.DecodeString(r.Hex)
			if e != nil {
				t.Fatal(e)
			}
			var s hostedDriverState
			valid := json.Unmarshal(raw, &s) == nil
			canonical, _ := json.Marshal(s)
			valid = valid && bytes.Equal(raw, append(canonical, '\n')) && auditHostedDriver(s, cfg, coordinator, accounts) == nil
			if valid != r.Valid {
				t.Fatal("raw framing disagreement")
			}
		})
	}
	key, e := hostedFixtureKey()
	if e != nil {
		t.Fatal(e)
	}
	for _, r := range corpus.Tokens {
		if !r.Accepted {
			continue
		}
		t.Run(r.Name, func(t *testing.T) {
			kind := "JWT"
			if r.Kind == "reset" {
				kind = "slug-reset+jwt"
			}
			token, e := hostedSign(key, kind, json.RawMessage(r.Claims))
			if e != nil || token != r.Token {
				t.Fatal("independent RS256 bytes differ", e)
			}
		})
	}
}
func hostedDriverTest(t *testing.T) (*hostedFixtureDriver, *gymaccount.Store, *gymprovider.Coordinator) {
	t.Helper()
	ctx := context.Background()
	root, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	v, e := gymidentity.NewVerifier(gymprovider.FixtureVerifierConfig())
	if e != nil {
		t.Fatal(e)
	}
	cfg := gymaccount.IdentityConfig{Version: 2, Accounts: gymaccount.Config{Version: 1, Principals: []gymaccount.FixturePrincipal{{Issuer: gymprovider.FixtureIssuer, Subject: "zero256", Fixture: "zero256"}, {Issuer: gymprovider.FixtureIssuer, Subject: "incrementing256", Fixture: "incrementing256"}}, MaxSessions: 4096, MaxChallenges: 4096, MaxEvents: 16384}, VerifierConfigHash: v.ConfigDigest(), MaxLoginAttempts: 1024}
	a, e := gymaccount.CreateIdentityPinned(filepath.Join(root, "authority"), cfg, strings.Repeat("11", 32))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = a.Close() })
	pc, e := hostedProviderConfig(a.AuthorityID())
	if e != nil {
		t.Fatal(e)
	}
	c, e := gymprovider.CreatePinned(ctx, filepath.Join(root, "coordinator"), pc, a, v, strings.Repeat("22", 32))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = c.Close() })
	d, e := openHostedDriver(ctx, filepath.Join(root, "driver"), pc, a, c, true)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = d.Close() })
	return d, a, c
}
func TestHostedFixtureResetOriginalReplayAndReopen(t *testing.T) {
	d, a, c := hostedDriverTest(t)
	ctx := context.Background()
	flow, e := c.IssueLogin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	token, e := d.FreshLogin(ctx, flow, "zero256")
	if e != nil {
		t.Fatal(e)
	}
	before := hostedCloneState(d.state)
	again, e := d.FreshLogin(ctx, flow, "zero256")
	if e != nil || len(again) == 0 || d.state.Revision != before.Revision {
		t.Fatal("original auth replay changed", e)
	}
	session, e := c.FinishLogin(ctx, flow.ID, token)
	if e != nil {
		t.Fatal(e)
	}
	request, e := c.IssueReset(ctx)
	if e != nil {
		t.Fatal(e)
	}
	evidence, e := d.BeginResetEvidence(ctx, request, "zero256")
	if e != nil {
		t.Fatal(e)
	}
	pending, e := d.PendingReset(ctx, "zero256")
	if e != nil || pending.Request != request {
		t.Fatal(e)
	}
	if _, e = d.CompleteResetEvidence(ctx, request); !errors.Is(e, gymprovider.ErrConflict) {
		t.Fatal("uncommitted completion", e)
	}
	ack, e := c.BeginReset(ctx, request.EventID, evidence)
	if e != nil {
		t.Fatal(e)
	}
	if e = a.WithSession(ctx, session.Token, gymaccount.View, func(gymaccount.Account, func() error) error { return nil }); !errors.Is(e, gymaccount.ErrForbidden) {
		t.Fatal("pending did not revoke", e)
	}
	if e = d.CommitReset(ctx, request, ack); e != nil {
		t.Fatal(e)
	}
	revision := d.state.Revision
	if e = d.CommitReset(ctx, request, ack); e != nil || revision != d.state.Revision {
		t.Fatal("CAS replay", e)
	}
	path, cfg := d.path, d.state.Config
	if e = d.Close(); e != nil {
		t.Fatal(e)
	}
	d, e = openHostedDriver(ctx, path, cfg, a, c, false)
	if e != nil {
		t.Fatal(e)
	}
	defer d.Close()
	evidence, e = d.CompleteResetEvidence(ctx, request)
	if e != nil {
		t.Fatal(e)
	}
	complete, e := c.CompleteReset(ctx, request.EventID, evidence)
	if e != nil {
		t.Fatal(e)
	}
	if e = d.ObserveResetComplete(ctx, request, complete); e != nil {
		t.Fatal(e)
	}
	if e = a.WithSession(ctx, session.Token, gymaccount.View, func(gymaccount.Account, func() error) error { return nil }); !errors.Is(e, gymaccount.ErrUnauthorized) {
		t.Fatal("completed reset retained old token", e)
	}
	if _, e = d.PendingReset(ctx, "zero256"); !errors.Is(e, gymprovider.ErrNotFound) {
		t.Fatal(e)
	}
	revision = d.state.Revision
	if e = d.ObserveResetComplete(ctx, request, complete); e != nil || d.state.Revision != revision {
		t.Fatal("completion replay", e)
	}
	flow, e = c.IssueLogin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	token, e = d.FreshLogin(ctx, flow, "zero256")
	if e != nil {
		t.Fatal(e)
	}
	next, e := c.FinishLogin(ctx, flow.ID, token)
	if e != nil || next.Account.ID != session.Account.ID || next.Account.Generation != 2 || next.Account.HoldUntilMS != complete.Barrier.HoldUntilMS {
		t.Fatal("same held account", e)
	}
}
func TestHostedFixtureLostWritePoisonAndOriginalRecovery(t *testing.T) {
	d, a, c := hostedDriverTest(t)
	ctx := context.Background()
	flow, e := c.IssueLogin(ctx)
	if e != nil {
		t.Fatal(e)
	}
	d.publish = func(raw []byte) error {
		if e := d.writeState(raw); e != nil {
			return e
		}
		return errors.New("lost directory acknowledgment")
	}
	if token, e := d.FreshLogin(ctx, flow, "zero256"); token != "" || !errors.Is(e, gymprovider.ErrOutcomeUnknown) {
		t.Fatal("uncertain state issued token", e)
	}
	if _, e = d.FreshLogin(ctx, flow, "zero256"); !errors.Is(e, gymprovider.ErrUnavailable) {
		t.Fatal("poisoned owner used", e)
	}
	path, cfg := d.path, d.state.Config
	_ = d.Close()
	d, e = openHostedDriver(ctx, path, cfg, a, c, false)
	if e != nil {
		t.Fatal(e)
	}
	defer d.Close()
	original := d.state.Auth[0]
	if _, e = d.FreshLogin(ctx, flow, "zero256"); e != nil || d.state.Auth[0] != original || len(d.state.Auth) != 1 {
		t.Fatal("changed recovered original", e)
	}
	d.now = func() time.Time { return time.UnixMilli(d.state.ClockHighWaterMS - 1) }
	if _, e = d.FreshLogin(ctx, flow, "zero256"); !errors.Is(e, gymprovider.ErrConflict) {
		t.Fatal("clock rollback accepted", e)
	}
}
