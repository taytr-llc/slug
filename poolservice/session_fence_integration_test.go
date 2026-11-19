// Copyright (c) 2026 The SLUG developers. ISC license.
package poolservice_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/taytr-llc/slug/btcec/v2"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/poolsettlefixture"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/prospectwork"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

type fencedAccountNode struct {
	template         prospectwork.Template
	stage            string
	entered, release chan struct{}
	once             sync.Once
}

func (n *fencedAccountNode) Resolve(ctx context.Context, _ prospectnode.Policy) (prospect.Context, error) {
	return n.template.Context, ctx.Err()
}
func (n *fencedAccountNode) RecheckAdmission(ctx context.Context, _ prospect.Context, _ prospect.Proof, _ prospect.Verified) (string, error) {
	return "current-parent", n.wait(ctx, "admission")
}

func TestRealAccountFencedAdmissionAllowsVerifiedResetDuringNodeCheck(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal candidate proof and account")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	accounts, e := gymaccount.Create(filepath.Join(t.TempDir(), "accounts"), gymaccount.Config{Version: 1, Principals: []gymaccount.FixturePrincipal{{Issuer: "fixture:local", Subject: "alice", Fixture: "zero256"}, {Issuer: "fixture:local", Subject: "bob", Fixture: "incrementing256"}}, MaxSessions: 32, MaxChallenges: 16, MaxEvents: 64})
	if e != nil {
		t.Fatal(e)
	}
	defer accounts.Close()
	_, miningA, _ := enrollServiceGym(t, ctx, accounts, "alice", "enroll")
	viewB, _, _ := enrollServiceGym(t, ctx, accounts, "bob", "enroll")
	template := accountFenceTemplate(t)
	descriptor, e := fighterdescriptor.ID(fighterdescriptor.CandidateDescriptor())
	if e != nil {
		t.Fatal(e)
	}
	issuer, e := registry.CreateManifestAuthority(filepath.Join(t.TempDir(), "issuer"), template.Context.Network, template.Context.Genesis, descriptor)
	if e != nil {
		t.Fatal(e)
	}
	defer issuer.Close()
	info, e := issuer.Info()
	if e != nil {
		t.Fatal(e)
	}
	node := &fencedAccountNode{template: template, entered: make(chan struct{}), release: make(chan struct{})}
	policy := prospectnode.Policy{Version: 1, Network: template.Context.Network, Genesis: template.Context.Genesis, DNAVersion: 1, ProspectTarget: template.Context.ProspectTarget}
	cfg := poolshare.Config{Version: 1, AuthorityPin: info.Pin, Policy: policy, PayoutAddress: poolsettlefixture.PoolAddress(), PoolTarget: strings.Repeat("f", 64), MaxWorks: 8, MaxSubmissions: 8, MaxBytes: 16 << 20}
	pool, e := poolshare.Create(filepath.Join(t.TempDir(), "pool"), cfg, issuer, node, serviceSessions{accounts})
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	work, e := pool.Issue(ctx, miningA.Token)
	if e != nil {
		t.Fatal(e)
	}
	// The independent template and genuine issued job provide the proof inputs;
	// normal verification happens inside the server, not in a fabricated receipt.
	proof := work.Candidate.Proof
	rawKey, _ := hex.DecodeString("9d3035e7440dd3737fe58ffb69ca3576257d424b6e6172b3199a66a8897bddf6")
	key, _ := btcec.PrivKeyFromBytes(rawKey)
	clear(rawKey)
	defer key.Zero()
	auth, e := gymauth.SignAdmission(work.Job.Context, proof, key)
	if e != nil {
		t.Fatal(e)
	}
	body, e := json.Marshal(struct {
		Proof         prospect.Proof        `json:"proof"`
		Authorization gymauth.Authorization `json:"authorization"`
	}{proof, auth})
	if e != nil {
		t.Fatal(e)
	}
	server, e := prospectservice.NewFencedManifestAdmissionServer(prospectservice.ManifestServerConfig{Version: 3, Listen: "127.0.0.1:0", AuthorityPin: info.Pin, MaxJobs: 8, MaxRecords: 8}, policy, node, issuer, serviceSessions{accounts})
	if e != nil {
		t.Fatal(e)
	}
	node.stage = "admission"
	request := httptest.NewRequest(http.MethodPost, "/v3/admissions", bytes.NewReader(body)).WithContext(ctx)
	request.Host = "127.0.0.1:12345"
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+miningA.Token)
	response := httptest.NewRecorder()
	done := make(chan struct{})
	go func() { server.ServeHTTP(response, request); close(done) }()
	var release sync.Once
	defer func() {
		release.Do(func() { close(node.release) })
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("admission failed to drain")
		}
	}()
	select {
	case <-node.entered:
	case <-done:
		t.Fatal("admission never reached node", response.Code)
	case <-ctx.Done():
		t.Fatal("admission did not verify")
	}
	short, stop := context.WithTimeout(ctx, time.Second)
	e = accounts.WithSession(short, viewB.Token, gymaccount.View, func(_ gymaccount.Account, check func() error) error { return check() })
	stop()
	if e != nil {
		t.Fatal("other gym blocked by fighter admission", e)
	}
	short, stop = context.WithTimeout(ctx, time.Second)
	reset, inserted, e := accounts.ApplyReset(short, gymaccount.Principal{Issuer: "fixture:local", Subject: "alice", AuthenticatedAtMS: time.Now().UnixMilli(), RecoveryVerified: true, RecoveryEventID: "admission-fence-reset"})
	stop()
	if e != nil || !inserted || reset.Generation != 2 {
		t.Fatal("verified reset blocked by fighter admission", e)
	}
	release.Do(func() { close(node.release) })
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("admission failed to return")
	}
	var failure prospectservice.ErrorResponse
	if e = json.Unmarshal(response.Body.Bytes(), &failure); e != nil || response.Code != http.StatusUnauthorized || failure.Error.Code != "unauthorized" {
		t.Fatal("old admission did not refuse", response.Code, e)
	}
	got, e := issuer.Info()
	if e != nil || got.Records != 0 || got.Jobs != 1 {
		t.Fatal("old session created a fighter", got.Records, got.Jobs, e)
	}
}

func (n *fencedAccountNode) wait(ctx context.Context, stage string) error {
	if stage != n.stage {
		return nil
	}
	n.once.Do(func() { close(n.entered) })
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-n.release:
		return nil
	}
}
func (n *fencedAccountNode) ResolveTemplate(ctx context.Context, _ prospectnode.Policy) (prospectwork.Template, error) {
	e := n.wait(ctx, "resolve")
	return n.template, e
}
func (n *fencedAccountNode) Proposal(ctx context.Context, _ prospect.Context, _ string) error {
	return n.wait(ctx, "proposal")
}
func (n *fencedAccountNode) RecheckContext(ctx context.Context, _ prospect.Context) error {
	return ctx.Err()
}
func (n *fencedAccountNode) SubmitBlock(ctx context.Context, _ prospect.Context, _ string) error {
	return ctx.Err()
}
func (n *fencedAccountNode) CanonicalBlock(ctx context.Context, _ prospect.Context, _ prospect.Proof, _ string) error {
	return errors.New("test has no canonical chain")
}

func accountFenceTemplate(t *testing.T) prospectwork.Template {
	t.Helper()
	raw, e := os.ReadFile("../prospectwork/testdata/pool_share_reference/pool_share_v1.json")
	if e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != "2210d992e618b7a0952736dede1263b678f1a7a2d55d14ccde1cc6b29317de87" {
		t.Fatal("frozen public work input changed")
	}
	var corpus struct {
		Sources []struct {
			Name     string
			Template prospectwork.Template
		}
	}
	if e = json.Unmarshal(raw, &corpus); e != nil {
		t.Fatal(e)
	}
	for _, s := range corpus.Sources {
		if s.Name == "coinbase-only" {
			return s.Template
		}
	}
	t.Fatal("missing coinbase-only input")
	return prospectwork.Template{}
}

// Actual account/reset serialization, with a deliberately stalled trusted node
// adapter. The adapter makes no chain/finality claim and performs no networking.
func TestRealAccountPoolSlowIssueAllowsOtherGymAndVerifiedReset(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal candidate account/issuer")
	}
	for _, stage := range []string{"resolve", "proposal"} {
		t.Run(stage, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			accounts, e := gymaccount.Create(filepath.Join(t.TempDir(), "accounts"), gymaccount.Config{Version: 1, Principals: []gymaccount.FixturePrincipal{{Issuer: "fixture:local", Subject: "alice", Fixture: "zero256"}, {Issuer: "fixture:local", Subject: "bob", Fixture: "incrementing256"}}, MaxSessions: 32, MaxChallenges: 16, MaxEvents: 64})
			if e != nil {
				t.Fatal(e)
			}
			defer accounts.Close()
			_, miningA, _ := enrollServiceGym(t, ctx, accounts, "alice", "enroll")
			viewB, _, _ := enrollServiceGym(t, ctx, accounts, "bob", "enroll")
			template := accountFenceTemplate(t)
			descriptor, e := fighterdescriptor.ID(fighterdescriptor.CandidateDescriptor())
			if e != nil {
				t.Fatal(e)
			}
			issuer, e := registry.CreateManifestAuthority(filepath.Join(t.TempDir(), "issuer"), template.Context.Network, template.Context.Genesis, descriptor)
			if e != nil {
				t.Fatal(e)
			}
			defer issuer.Close()
			info, e := issuer.Info()
			if e != nil {
				t.Fatal(e)
			}
			node := &fencedAccountNode{template: template, stage: stage, entered: make(chan struct{}), release: make(chan struct{})}
			policy := prospectnode.Policy{Version: 1, Network: template.Context.Network, Genesis: template.Context.Genesis, DNAVersion: 1, ProspectTarget: template.Context.ProspectTarget}
			cfg := poolshare.Config{Version: 1, AuthorityPin: info.Pin, Policy: policy, PayoutAddress: poolsettlefixture.PoolAddress(), PoolTarget: strings.Repeat("f", 64), MaxWorks: 8, MaxSubmissions: 8, MaxBytes: 16 << 20}
			pool, e := poolshare.Create(filepath.Join(t.TempDir(), "pool"), cfg, issuer, node, serviceSessions{accounts})
			if e != nil {
				t.Fatal(e)
			}
			defer pool.Close()
			type outcome struct {
				work poolshare.Work
				err  error
			}
			done := make(chan outcome, 1)
			go func() { w, e := pool.Issue(ctx, miningA.Token); done <- outcome{w, e} }()
			var release sync.Once
			joined := false
			defer func() {
				release.Do(func() { close(node.release) })
				if !joined {
					select {
					case <-done:
					case <-time.After(3 * time.Second):
						t.Error("stalled operation failed to drain")
					}
				}
			}()
			select {
			case <-node.entered:
			case result := <-done:
				joined = true
				t.Fatal("issue never reached slow node", result.err)
			case <-ctx.Done():
				t.Fatal("node fixture deadline")
			}
			short, stop := context.WithTimeout(ctx, time.Second)
			e = accounts.WithSession(short, viewB.Token, gymaccount.View, func(_ gymaccount.Account, check func() error) error { return check() })
			stop()
			if e != nil {
				t.Fatal("other gym blocked by slow pool request", e)
			}
			short, stop = context.WithTimeout(ctx, time.Second)
			reset, inserted, e := accounts.ApplyReset(short, gymaccount.Principal{Issuer: "fixture:local", Subject: "alice", AuthenticatedAtMS: time.Now().UnixMilli(), RecoveryVerified: true, RecoveryEventID: "pool-fence-" + stage})
			stop()
			if e != nil || !inserted || reset.Generation != 2 {
				t.Fatal("verified reset blocked by slow pool request", e, inserted)
			}
			release.Do(func() { close(node.release) })
			select {
			case result := <-done:
				joined = true
				if result.err == nil || result.work.Job.ID != "" {
					t.Fatal("reset operation disclosed or issued work", result.err)
				}
			case <-ctx.Done():
				t.Fatal("old operation did not drain")
			}
			got, e := issuer.Info()
			if e != nil {
				t.Fatal(e)
			}
			want := uint64(0)
			if stage == "proposal" {
				want = 1
			} // Authorized issuer commit preceded this stall.
			if got.Jobs != want || got.Records != 0 {
				t.Fatal("reset produced a new job/admission", got.Jobs, got.Records, want)
			}
			page, e := pool.AccountingPage(ctx, 0, 0, 1)
			if e != nil || len(page.Entries) != 0 {
				t.Fatal("reset produced share credit", e)
			}
			if e = accounts.WithSession(ctx, miningA.Token, gymaccount.Mining, func(gymaccount.Account, func() error) error { return nil }); e == nil {
				t.Fatal("original mining credential survived reset")
			}
		})
	}
}
