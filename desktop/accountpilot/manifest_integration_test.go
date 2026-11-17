//go:build desktopintegration && !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.

package accountpilot

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/gymidentity"
	"github.com/taytr-llc/slug/gymprovider"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/prospectworker"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slugwallet/recoveryvault"
)

// This composition uses only the two published recovery fixtures. It does not
// activate a desktop route, provision a real provider, or contact a public node.
type manifestTestSessions struct{ store *gymaccount.Store }

func (s manifestTestSessions) WithSession(ctx context.Context, token string, fn func(string, func() error) error) error {
	return s.store.WithSession(ctx, token, gymaccount.Mining, func(a gymaccount.Account, check func() error) error {
		if a.Enrollment == nil {
			return gymaccount.ErrForbidden
		}
		return fn(a.Enrollment.GymPublicKey, check)
	})
}

type manifestTestNode struct {
	*prospectnode.Client
	serviceCalls atomic.Uint64
	mu           sync.Mutex
	broadcasts   map[string]bool
}

func (n *manifestTestNode) Resolve(ctx context.Context, p prospectnode.Policy) (prospect.Context, error) {
	n.serviceCalls.Add(1)
	return n.Client.Resolve(ctx, p)
}
func (n *manifestTestNode) RecheckContext(ctx context.Context, c prospect.Context) error {
	n.serviceCalls.Add(1)
	return n.Client.RecheckContext(ctx, c)
}
func (n *manifestTestNode) RecheckAdmission(ctx context.Context, c prospect.Context, p prospect.Proof, v prospect.Verified) (string, error) {
	n.serviceCalls.Add(1)
	return n.Client.RecheckAdmission(ctx, c, p, v)
}
func (n *manifestTestNode) SubmitBlock(ctx context.Context, c prospect.Context, b string) error {
	n.mu.Lock()
	if len(b) >= 160 {
		n.broadcasts[b[:160]] = true
	}
	n.mu.Unlock()
	return n.Client.SubmitBlock(ctx, c, b)
}
func (n *manifestTestNode) broadcastBeforeSign(header string) bool {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.broadcasts[header]
}

type manifestTestAuthority struct {
	*prospectservice.ManifestClient
	after func(prospectservice.ManifestAdmissionResult) (prospectservice.ManifestAdmissionResult, error)
}

func (a *manifestTestAuthority) SubmitManifest(ctx context.Context, j prospectjob.ManifestJob, p prospect.Proof, s gymauth.Authorization) (prospectservice.ManifestAdmissionResult, error) {
	r, e := a.ManifestClient.SubmitManifest(ctx, j, p, s)
	if e != nil {
		return r, e
	}
	return a.after(r)
}

type manifestTestProcess struct {
	cmd              *exec.Cmd
	done             chan error
	ctl, dir         string
	rpcPort, p2pPort int
	stopped          bool
}

func manifestTestPort(t *testing.T) int {
	t.Helper()
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	p := l.Addr().(*net.TCPAddr).Port
	if e = l.Close(); e != nil {
		t.Fatal(e)
	}
	return p
}
func (n *manifestTestProcess) rpc(ctx context.Context, args ...string) (json.RawMessage, error) {
	limit := 30 * time.Second
	if len(args) > 0 && args[0] == "generate" {
		limit = 180 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	a := []string{"--notls", "--rpcuser=u", "--rpcpass=p", fmt.Sprintf("--rpcserver=127.0.0.1:%d", n.rpcPort), "--configfile=" + filepath.Join(n.dir, "ctl.conf")}
	a = append(a, args...)
	c := exec.CommandContext(ctx, n.ctl, a...)
	raw, e := c.Output()
	return raw, e
}
func manifestTestStartNode(t *testing.T, root, name, node, ctl string, connect int) *manifestTestProcess {
	t.Helper()
	n := &manifestTestProcess{ctl: ctl, dir: filepath.Join(root, name), rpcPort: manifestTestPort(t), p2pPort: manifestTestPort(t), done: make(chan error, 1)}
	if e := os.MkdirAll(n.dir, 0700); e != nil {
		t.Fatal(e)
	}
	f, e := os.Create(filepath.Join(n.dir, "node.log"))
	if e != nil {
		t.Fatal(e)
	}
	n.cmd = exec.Command(node, "--notls", "--rpcuser=u", "--rpcpass=p", fmt.Sprintf("--rpclisten=127.0.0.1:%d", n.rpcPort), fmt.Sprintf("--listen=127.0.0.1:%d", n.p2pPort), fmt.Sprintf("--connect=127.0.0.1:%d", connect), "--nodnsseed", "--datadir="+filepath.Join(n.dir, "data"), "--logdir="+filepath.Join(n.dir, "logs"), "--configfile="+filepath.Join(n.dir, "node.conf"), "--miningaddr=amkchND9pP6STAzbb4M6U4SzU2anReC4R4")
	n.cmd.Stdout = f
	n.cmd.Stderr = f
	n.cmd.Env = append(os.Environ(), "GOMAXPROCS=4")
	if e = n.cmd.Start(); e != nil {
		_ = f.Close()
		t.Fatal(e)
	}
	go func() { e := n.cmd.Wait(); _ = f.Close(); n.done <- e }()
	t.Cleanup(func() { n.stop(t) })
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_, e := n.rpc(ctx, "getblockcount")
		cancel()
		if e == nil {
			return n
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("owned manifest node did not become ready")
	return nil
}
func (n *manifestTestProcess) stop(t *testing.T) {
	t.Helper()
	if n.stopped {
		return
	}
	n.stopped = true
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	_, e := n.rpc(ctx, "stop")
	cancel()
	if e != nil {
		_ = n.cmd.Process.Kill()
	}
	select {
	case e := <-n.done:
		if e != nil {
			t.Error("owned node exit", e)
		}
	case <-time.After(30 * time.Second):
		_ = n.cmd.Process.Kill()
		<-n.done
		t.Error("owned node required forced shutdown")
	}
	for _, p := range []int{n.rpcPort, n.p2pPort} {
		c, e := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", p), time.Second)
		if e == nil {
			_ = c.Close()
			t.Error("owned node listener remained open")
		}
	}
}

func TestManifestWorkerVaultResetAndHistoricalRecoveryNormalPoW(t *testing.T) {
	nodeBinary, ctlBinary := os.Getenv("SLUG_MANIFEST_NODE"), os.Getenv("SLUG_MANIFEST_CTL")
	if nodeBinary == "" || ctlBinary == "" {
		t.Skip("explicit frozen normal-PoW node and control binaries required")
	}
	if !filepath.IsAbs(nodeBinary) || !filepath.IsAbs(ctlBinary) {
		t.Fatal("absolute fixture binaries required")
	}
	root := t.TempDir()
	if out := os.Getenv("SLUG_ACCOUNT_TEST_ARTIFACTS"); out != "" {
		var e error
		root, e = os.MkdirTemp(out, "manifest-normal-")
		if e != nil {
			t.Fatal(e)
		}
	}
	t.Log("public manifest fixture artifacts", root)
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	firstNode := manifestTestStartNode(t, root, "node-a", nodeBinary, ctlBinary, 9)
	secondNode := manifestTestStartNode(t, root, "node-b", nodeBinary, ctlBinary, firstNode.p2pPort)
	if _, e := firstNode.rpc(ctx, "generate", "61"); e != nil {
		t.Fatal("private bootstrap mining", e)
	}
	for end := time.Now().Add(60 * time.Second); ; {
		raw, e := secondNode.rpc(ctx, "getblockcount")
		var height int64
		if e == nil && json.Unmarshal(raw, &height) == nil && height == 61 {
			break
		}
		if time.Now().After(end) {
			t.Fatal("second private node failed to sync")
		}
		time.Sleep(200 * time.Millisecond)
	}
	policy := prospectnode.Policy{Version: 1, Network: gymaccount.FixtureNetwork, Genesis: gymaccount.FixtureGenesis, DNAVersion: 1, ProspectTarget: strings.Repeat("f", 64)}
	rpcNode, e := prospectnode.NewClient(prospectnode.RPCConfig{Version: 1, URL: fmt.Sprintf("http://127.0.0.1:%d", firstNode.rpcPort), Username: "u", Password: "p"})
	if e != nil {
		t.Fatal(e)
	}
	defer rpcNode.Close()
	node := &manifestTestNode{Client: rpcNode, broadcasts: make(map[string]bool)}
	_, sign := identityTestVerifier(t)
	verifier, e := gymidentity.NewVerifier(gymprovider.FixtureVerifierConfig())
	if e != nil {
		t.Fatal(e)
	}
	accountConfig := gymaccount.IdentityConfig{Version: 2, VerifierConfigHash: verifier.ConfigDigest(), MaxLoginAttempts: 256, Accounts: gymaccount.Config{Version: 1, MaxSessions: 512, MaxChallenges: 256, MaxEvents: 2048, Principals: []gymaccount.FixturePrincipal{{Issuer: identityTestIssuer, Subject: "zero256", Fixture: "zero256"}, {Issuer: identityTestIssuer, Subject: "incrementing256", Fixture: "incrementing256"}}}}
	accounts, e := gymaccount.CreateIdentity(filepath.Join(root, "accounts"), accountConfig)
	if e != nil {
		t.Fatal(e)
	}
	defer accounts.Close()
	providerConfig := gymprovider.Config{Version: 1, Provider: gymprovider.ProviderFixture, Issuer: gymprovider.FixtureIssuer, TenantID: "fixture-tenant", ClientID: "fixture-client", ConnectionID: "fixture-database", AuthorityID: accounts.AuthorityID(), VerifierConfigHash: verifier.ConfigDigest(), MaxLoginFlows: 128, MaxResetFlows: 16}
	coordinator, e := gymprovider.Create(ctx, filepath.Join(root, "coordinator"), providerConfig, accounts, verifier)
	if e != nil {
		t.Fatal(e)
	}
	defer coordinator.Close()
	provider := newDurableProviderFixture(t, filepath.Join(root, "provider.json"), providerConfig, sign, false)
	wrapper := new(vaultIntegrationWrapper)
	vaultConfig := recoveryvault.Config{Version: 1, WrapperID: wrapper.ID(), RecoveryKeyRef: "fixture-recovery-v1", GymKeyRef: "fixture-gym-v1", AllowedRecoveryKeyRefs: []string{"fixture-recovery-v1", "fixture-recovery-v2"}, AllowedGymKeyRefs: []string{"fixture-gym-v1", "fixture-gym-v2"}, MaxOperations: 128, Policy: policy}
	vault, e := recoveryvault.Create(ctx, filepath.Join(root, "vault"), vaultConfig, accounts, wrapper)
	if e != nil {
		t.Fatal(e)
	}
	defer vault.Close()
	descriptorID, e := fighterdescriptor.ID(fighterdescriptor.CandidateDescriptor())
	if e != nil {
		t.Fatal(e)
	}
	registryPath := filepath.Join(root, "manifest-authority")
	authority, e := registry.CreateManifestAuthority(registryPath, policy.Network, policy.Genesis, descriptorID)
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = authority.Close() }()
	info, e := authority.Info()
	if e != nil {
		t.Fatal(e)
	}
	pin := info.Pin
	if pin.AuthorityID == accounts.AuthorityID() {
		t.Fatal("separate authority namespaces coincided")
	}
	listen := fmt.Sprintf("127.0.0.1:%d", manifestTestPort(t))
	origin := "http://" + listen
	var stopService context.CancelFunc
	var serviceDone chan error
	startService := func(p prospectnode.Policy) {
		t.Helper()
		server, e := prospectservice.NewManifestSessionServer(prospectservice.ManifestServerConfig{Version: 3, Listen: listen, AuthorityPin: pin, Clients: []prospectservice.Credential{}, MaxJobs: 256, MaxRecords: 256}, p, node, authority, manifestTestSessions{accounts})
		if e != nil {
			t.Fatal(e)
		}
		var runCtx context.Context
		runCtx, stopService = context.WithCancel(ctx)
		serviceDone = make(chan error, 1)
		ready := make(chan string, 1)
		go func() { serviceDone <- server.Run(runCtx, func(s string) { ready <- s }) }()
		select {
		case got := <-ready:
			if got != origin {
				t.Fatal("service origin changed")
			}
		case e := <-serviceDone:
			t.Fatal("service exited before readiness", e)
		case <-time.After(10 * time.Second):
			t.Fatal("service readiness timed out")
		}
	}
	stop := func() {
		t.Helper()
		if stopService == nil {
			return
		}
		stopService()
		stopService = nil
		select {
		case e := <-serviceDone:
			if e != nil {
				t.Fatal(e)
			}
		case <-time.After(35 * time.Second):
			t.Fatal("service did not drain")
		}
		connection, err := net.DialTimeout("tcp", listen, time.Second)
		if err == nil {
			_ = connection.Close()
			t.Fatal("owned manifest service listener remained open")
		}
	}
	defer stop()
	startService(policy)
	type flow struct {
		fixture, path             string
		session                   gymaccount.Session
		mining                    gymaccount.Session
		config                    prospectworker.ManifestConfig
		lost                      prospectservice.ManifestAdmissionResult
		entries                   []prospectworker.ManifestEntry
		recoveryBefore, gymBefore uint64
	}
	flows := make([]*flow, 0, 2)
	for _, fixture := range []string{"zero256", "incrementing256"} {
		session, _ := provider.login(ctx, coordinator, fixture)
		retained, inserted, e := vault.ProvisionFixture(ctx, session.Token)
		if e != nil || !inserted {
			t.Fatal("provision public fixture", e)
		}
		if got, e := vault.EnsureEnrollment(ctx, session.Token); e != nil || !reflect.DeepEqual(got, retained) {
			t.Fatal("fixture enrollment", e)
		}
		mining, e := accounts.MintMining(ctx, session.Token)
		if e != nil {
			t.Fatal(e)
		}
		client, e := prospectservice.NewManifestClient(prospectservice.ManifestClientConfig{Version: 3, URL: origin, Token: mining.Token, AuthorityPin: pin}, policy)
		if e != nil {
			t.Fatal(e)
		}
		ref := recoveryReferences(t)[fixture]
		payout, _ := ref.payout(t)
		f := &flow{fixture: fixture, path: filepath.Join(root, "outbox-"+fixture), session: session, mining: mining}
		f.config = prospectworker.ManifestConfig{Version: 2, Policy: policy, PublicKey: ref.Export.Manifest.GymPublicKey, PayoutAddress: payout, AuthorityOrigin: origin, AuthorityPin: pin, TransportTrustID: client.TransportTrustID(), MaxRecords: 256, MaxBytes: 64 << 20}
		box, e := prospectworker.CreateManifestOutbox(f.path, f.config)
		if e != nil {
			t.Fatal(e)
		}
		runCtx, runCancel := context.WithTimeout(ctx, 90*time.Second)
		seen := make(map[string]bool)
		adapter := &manifestTestAuthority{ManifestClient: client}
		adapter.after = func(result prospectservice.ManifestAdmissionResult) (prospectservice.ManifestAdmissionResult, error) {
			if !result.Inserted {
				t.Fatal("new worker unexpectedly recovered existing work")
			}
			if seen["current-parent"] && seen["canonical-winning-block"] {
				f.lost = result
				runCancel()
				return prospectservice.ManifestAdmissionResult{}, &prospectservice.ClientError{Code: "outcome_unknown", OutcomeUnknown: true}
			}
			seen[result.Record.Eligibility.Source] = true
			return result, nil
		}
		signer := func(c context.Context, initial prospect.Context, proof prospect.Proof) (gymauth.Authorization, error) {
			rows, e := box.List(0, 100)
			if e != nil {
				return gymauth.Authorization{}, e
			}
			found := false
			for _, row := range rows {
				if row.Candidate.Proof.Header == proof.Header {
					found = true
					if row.BlockWin && !node.broadcastBeforeSign(proof.Header) {
						return gymauth.Authorization{}, errors.New("signing preceded block broadcast")
					}
				}
			}
			if !found {
				return gymauth.Authorization{}, errors.New("signing preceded durable found work")
			}
			return vault.SignAdmission(c, mining.Token, initial, proof)
		}
		worker, e := prospectworker.NewManifest(f.config, adapter, node, signer, box)
		if e != nil {
			t.Fatal(e)
		}
		e = worker.Run(runCtx)
		runCancel()
		if e != nil {
			t.Fatal("manifest automatic worker", e)
		}
		f.entries, e = box.List(0, 100)
		if e != nil {
			t.Fatal(e)
		}
		if f.lost.Record.Sequence == 0 || !seen["current-parent"] || !seen["canonical-winning-block"] {
			t.Fatal("both admission sources and lost ACK not reached")
		}
		pending := 0
		for _, row := range f.entries {
			if row.AdmissionState == "pending" {
				pending++
				if row.Authorization == nil || row.Receipt != nil || row.Job.ID != f.lost.Record.Job.ID || row.Candidate.Proof.Header != f.lost.Record.Proof.Header {
					t.Fatal("lost ACK did not retain exact public evidence")
				}
			}
		}
		if pending != 1 {
			t.Fatal("expected exactly one recoverable pending admission")
		}
		if e = box.Close(); e != nil {
			t.Fatal(e)
		}
		client.Close()
		flows = append(flows, f)
		t.Log("automatic manifest mining and lost ACK", fixture, "receipts", len(f.entries))
	}
	for i, f := range flows {
		request, e := coordinator.IssueReset(ctx)
		if e != nil {
			t.Fatal(e)
		}
		operation := provider.verifiedReset(request, f.fixture, f.session.Account.ID)
		pending, e := provider.begin(ctx, coordinator, operation.ID)
		if e != nil || !pending.CanCommit {
			t.Fatal("reset barrier", e)
		}
		old, e := prospectservice.NewManifestClient(prospectservice.ManifestClientConfig{Version: 3, URL: origin, Token: f.mining.Token, AuthorityPin: pin}, policy)
		if e != nil {
			t.Fatal(e)
		}
		_, e = old.SubmitManifest(ctx, f.lost.Record.Job, f.lost.Record.Proof, f.lost.Record.Authorization)
		var denied *prospectservice.ClientError
		if !errors.As(e, &denied) || denied.Code != "unauthorized" {
			t.Fatal("pending reset disclosed historical receipt", e)
		}
		old.Close()
		if i == 0 {
			if e = accounts.WithSession(ctx, flows[1].mining.Token, gymaccount.Mining, func(gymaccount.Account, func() error) error { return nil }); e != nil {
				t.Fatal("reset revoked other gym", e)
			}
		}
		f.recoveryBefore, f.gymBefore, _ = wrapper.counts()
		if e = provider.commit(operation.ID, pending); e != nil {
			t.Fatal(e)
		}
		completed, e := provider.complete(ctx, coordinator, operation.ID)
		if e != nil {
			t.Fatal(e)
		}
		recovered, _ := provider.login(ctx, coordinator, f.fixture)
		if recovered.Account.ID != f.session.Account.ID || !recovered.Account.RecoveryRequired || recovered.Account.HoldUntilMS != completed.Barrier.HoldUntilMS {
			t.Fatal("reset changed account/hold")
		}
		if _, e = vault.EnsureEnrollment(ctx, recovered.Token); e != nil {
			t.Fatal("held same-wallet recovery", e)
		}
		if e = vault.ExportRecoveryFile(ctx, recovered.Token, filepath.Join(root, "denied-"+f.fixture+".json")); !errors.Is(e, gymaccount.ErrHeld) {
			t.Fatal("held recovery export not refused", e)
		}
		if _, _, e = vault.Rewrap(ctx, recovered.Token, "fixture-recovery-v2", "fixture-gym-v2"); !errors.Is(e, gymaccount.ErrHeld) {
			t.Fatal("held rewrap not refused", e)
		}
		f.mining, e = accounts.MintMining(ctx, recovered.Token)
		if e != nil {
			t.Fatal("held mining session", e)
		}
		f.session = recovered
		// The next admission uses the same pinned service and gym-only vault path
		// during the hold. A separate namespace preserves the pending old outbox.
		client, e := prospectservice.NewManifestClient(prospectservice.ManifestClientConfig{Version: 3, URL: origin, Token: f.mining.Token, AuthorityPin: pin}, policy)
		if e != nil {
			t.Fatal(e)
		}
		box, e := prospectworker.CreateManifestOutbox(filepath.Join(root, "held-outbox-"+f.fixture), f.config)
		if e != nil {
			t.Fatal(e)
		}
		runCtx, runCancel := context.WithTimeout(ctx, 60*time.Second)
		accepted := false
		adapter := &manifestTestAuthority{ManifestClient: client, after: func(r prospectservice.ManifestAdmissionResult) (prospectservice.ManifestAdmissionResult, error) {
			accepted = true
			runCancel()
			return r, nil
		}}
		worker, e := prospectworker.NewManifest(f.config, adapter, node, func(c context.Context, initial prospect.Context, proof prospect.Proof) (gymauth.Authorization, error) {
			return vault.SignAdmission(c, f.mining.Token, initial, proof)
		}, box)
		if e != nil {
			t.Fatal(e)
		}
		e = worker.Run(runCtx)
		runCancel()
		if e != nil || !accepted {
			t.Fatal("held automatic mining", e)
		}
		if e = box.Close(); e != nil {
			t.Fatal(e)
		}
		client.Close()
		recoveryAfter, gymAfter, _ := wrapper.counts()
		if recoveryAfter != f.recoveryBefore || gymAfter <= f.gymBefore {
			t.Fatal("held work did not stay within gym envelope")
		}
	}
	stop()
	if e = authority.Close(); e != nil {
		t.Fatal(e)
	}
	firstNode.stop(t)
	secondNode.stop(t)
	latestExpiry := int64(0)
	for _, f := range flows {
		if f.lost.Record.Job.ExpiresAtMS > latestExpiry {
			latestExpiry = f.lost.Record.Job.ExpiresAtMS
		}
	}
	for time.Now().UnixMilli() <= latestExpiry {
		wait := time.Until(time.UnixMilli(latestExpiry + 1))
		if wait > 30*time.Second {
			wait = 30 * time.Second
		}
		t.Log("waiting original real manifest job expiry", wait)
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(wait):
		}
	}
	authority, e = registry.OpenManifestAuthority(registryPath, pin, false)
	if e != nil {
		t.Fatal(e)
	}
	changedPolicy := policy
	changedPolicy.ProspectTarget = "7" + strings.Repeat("f", 63)
	startService(changedPolicy)
	callsBefore := node.serviceCalls.Load()
	publicRows := make([]prospectworker.ManifestEntry, 0)
	for _, f := range flows {
		// Mining grants and jobs both last five minutes. Refresh from the
		// retained post-reset login so this tests expired work, not an
		// incidental race against the old mining grant's expiration.
		f.mining, e = accounts.MintMining(ctx, f.session.Token)
		if e != nil {
			t.Fatal("fresh held mining grant for historical recovery", e)
		}
		client, e := prospectservice.NewManifestClient(prospectservice.ManifestClientConfig{Version: 3, URL: origin, Token: f.mining.Token, AuthorityPin: pin}, changedPolicy)
		if e != nil {
			t.Fatal(e)
		}
		box, e := prospectworker.OpenManifestOutbox(f.path, f.config)
		if e != nil {
			t.Fatal(e)
		}
		runCtx, runCancel := context.WithTimeout(ctx, 45*time.Second)
		recovered := false
		adapter := &manifestTestAuthority{ManifestClient: client, after: func(r prospectservice.ManifestAdmissionResult) (prospectservice.ManifestAdmissionResult, error) {
			if r.Inserted || r.CurrentNodeChecked || !reflect.DeepEqual(r.Record, f.lost.Record) {
				t.Fatal("historical recovery changed immutable receipt")
			}
			recovered = true
			runCancel()
			return r, nil
		}}
		worker, e := prospectworker.NewManifest(f.config, adapter, node, func(context.Context, prospect.Context, prospect.Proof) (gymauth.Authorization, error) {
			return gymauth.Authorization{}, errors.New("historical recovery must reuse saved signature")
		}, box)
		if e != nil {
			t.Fatal(e)
		}
		e = worker.Run(runCtx)
		runCancel()
		if e != nil || !recovered {
			t.Fatal("expired offline manifest recovery", e)
		}
		rows, e := box.List(0, 100)
		if e != nil {
			t.Fatal(e)
		}
		for _, row := range rows {
			if row.AdmissionState != "accepted" || row.Receipt == nil {
				t.Fatal("pending record not recovered")
			}
			publicRows = append(publicRows, row)
		}
		if e = box.Close(); e != nil {
			t.Fatal(e)
		}
		client.Close()
	}
	if node.serviceCalls.Load() != callsBefore {
		t.Fatal("historical recovery consulted offline node")
	}
	stop()
	if t.Failed() {
		t.Fatal("refusing success artifact after a failed assertion or cleanup")
	}
	ownedNodes := make([]map[string]any, 0, 2)
	for _, owned := range []*manifestTestProcess{firstNode, secondNode} {
		ownedNodes = append(ownedNodes, map[string]any{"pid": owned.cmd.Process.Pid, "rpc_port": owned.rpcPort, "p2p_port": owned.p2pPort, "exited": owned.cmd.ProcessState.Exited(), "exit_code": owned.cmd.ProcessState.ExitCode(), "listeners_closed": true})
	}
	raw, e := json.MarshalIndent(map[string]any{"status": "passed", "fixture_only": true, "public_chain_mined": false, "real_expiry": true, "node_outage": true, "changed_current_target": true, "held_mining_without_recovery_unwrap": true, "fresh_held_grant_for_recovery": true, "owned_nodes": ownedNodes, "service_listener_closed": true, "authority_pin": pin, "entries": publicRows}, "", "  ")
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(root, "manifest-public-result.json"), append(raw, '\n'), 0600); e != nil {
		t.Fatal(e)
	}
}
