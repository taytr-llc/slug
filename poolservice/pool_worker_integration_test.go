// Copyright (c) 2026 The SLUG developers. ISC license.
package poolservice_test

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/taytr-llc/slug/btcec/v2"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/poolround"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/poolsettlefixture"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/prospectworker"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

// This wrapper counts calls without replacing HTTP verification. The relay
// below drops the first committed share receipt, and this wrapper stops that
// worker only when the real client reports the uncertain transport result.
type serviceWorkerAuthority struct {
	*poolservice.Client
	issues, submits, lookups int
	stop                     context.CancelFunc
	work                     poolshare.MiningWork
	nonce                    uint32
	recoveredExact           bool
}

func (a *serviceWorkerAuthority) Issue(ctx context.Context) (poolshare.MiningWork, error) {
	a.issues++
	return a.Client.Issue(ctx)
}
func (a *serviceWorkerAuthority) Submit(ctx context.Context, work poolshare.MiningWork, nonce uint32) (poolshare.Submission, error) {
	a.submits++
	a.work, a.nonce = work, nonce
	result, err := a.Client.Submit(ctx, work, nonce)
	var failure *poolservice.ClientError
	if a.stop != nil && errors.As(err, &failure) && failure.OutcomeUnknown {
		a.stop()
		a.stop = nil
	}
	return result, err
}
func (a *serviceWorkerAuthority) Lookup(ctx context.Context, work poolshare.MiningWork, nonce uint32) (poolshare.Submission, error) {
	a.lookups++
	result, err := a.Client.Lookup(ctx, work, nonce)
	if err == nil && a.submits > 0 {
		a.recoveredExact = reflect.DeepEqual(a.work, work) && a.nonce == nonce
	}
	return result, err
}

func runWorkerService(t *testing.T, ctx context.Context, run func(context.Context, func(string)) error) string {
	t.Helper()
	runCtx, stop := context.WithCancel(ctx)
	done, ready := make(chan error, 1), make(chan string, 1)
	go func() { done <- run(runCtx, func(origin string) { ready <- origin }) }()
	t.Cleanup(func() {
		stop()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(40 * time.Second):
			t.Error("owned worker service did not drain")
		}
	})
	select {
	case origin := <-ready:
		return origin
	case err := <-done:
		done <- err
		t.Fatalf("owned worker service startup: %v", err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	return ""
}

// Opt-in composition uses the automatic worker, two actual HTTP services, a
// shared concrete issuer, real fixture-account leases and owned normal nodes.
// It deliberately stops after the relay loses a committed share receipt, then
// reopens the outbox at capacity and proves recovery without a replacement job.
func TestActualNormalPoolWorker(t *testing.T) {
	binary := os.Getenv("SLUG_POOL_SERVICE_NODE")
	if binary == "" {
		t.Skip("set SLUG_POOL_SERVICE_NODE to an explicit normal-PoW node")
	}
	if wire.TestPoW {
		t.Fatal("actual pool worker test requires normal PoW")
	}
	info, err := os.Lstat(binary)
	if !filepath.IsAbs(binary) || err != nil || !info.Mode().IsRegular() || info.Mode()&0111 == 0 {
		t.Fatal("explicit normal node binary required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	t.Cleanup(cancel)
	peerAddress := servicePort(t)
	peer := startServiceNode(t, ctx, binary, "127.0.0.1:1", peerAddress)
	node := startServiceNode(t, ctx, binary, peerAddress, "")
	waitServicePeers(t, ctx, node, peer)
	accounts, err := gymaccount.Create(filepath.Join(t.TempDir(), "accounts"), gymaccount.Config{Version: 1, Principals: []gymaccount.FixturePrincipal{{Issuer: "fixture:local", Subject: "alice", Fixture: "zero256"}, {Issuer: "fixture:local", Subject: "bob", Fixture: "incrementing256"}}, MaxSessions: 16, MaxChallenges: 8, MaxEvents: 32})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = accounts.Close() })
	_, mining, identity := enrollServiceGym(t, ctx, accounts, "alice", "enroll")
	sessions := serviceSessions{accounts}
	descriptor, err := fighterdescriptor.ID(fighterdescriptor.CandidateDescriptor())
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := registry.CreateManifestAuthority(filepath.Join(t.TempDir(), "issuer"), uint32(chaincfg.MainNetParams.Net), chaincfg.MainNetParams.GenesisHash.String(), descriptor)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = issuer.Close() })
	issuerInfo, err := issuer.Info()
	if err != nil {
		t.Fatal(err)
	}
	nodeClient, err := prospectnode.NewClient(node.config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(nodeClient.Close)
	policy := prospectnode.Policy{Version: 1, Network: uint32(chaincfg.MainNetParams.Net), Genesis: chaincfg.MainNetParams.GenesisHash.String(), DNAVersion: 1, ProspectTarget: strings.Repeat("f", 64)}
	sourceConfig := poolshare.Config{Version: 1, AuthorityPin: issuerInfo.Pin, Policy: policy, PayoutAddress: poolsettlefixture.PoolAddress(), PoolTarget: strings.Repeat("f", 64), MaxWorks: 8, MaxSubmissions: 8, MaxBytes: 16 << 20}
	source, err := poolshare.Create(filepath.Join(t.TempDir(), "shares"), sourceConfig, issuer, nodeClient, sessions)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = source.Close() })
	rounds, err := poolround.Create(filepath.Join(t.TempDir(), "rounds"), poolround.Limits{Version: 1, MaxRounds: 8, MaxBytes: 16 << 20}, source, nodeClient)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rounds.Close() })
	backend, err := poolsettlefixture.New(ctx, node.config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(backend.Close)
	settlement, err := poolsettle.Create(filepath.Join(t.TempDir(), "settlement"), poolsettle.Config{Version: 1, ChangeScript: poolsettlefixture.PoolScript(), Destinations: []poolsettle.Destination{{GymID: identity.GymID, Script: serviceScript(identity.GymPublicKey)}}, MaxRounds: 8, MaxBatches: 8}, rounds, backend)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = settlement.Close() })
	server, err := poolservice.NewServer(poolservice.ServerConfig{Version: 1, Listen: "127.0.0.1:0"}, source, settlement, sessions)
	if err != nil {
		t.Fatal(err)
	}
	poolOrigin := runWorkerService(t, ctx, server.Run)
	admissionServer, err := prospectservice.NewFencedManifestAdmissionServer(prospectservice.ManifestServerConfig{Version: 3, Listen: "127.0.0.1:0", AuthorityPin: issuerInfo.Pin, MaxJobs: 8, MaxRecords: 8}, policy, nodeClient, issuer, sessions)
	if err != nil {
		t.Fatal(err)
	}
	admissionOrigin := runWorkerService(t, ctx, admissionServer.Run)
	relay := lostServiceReceipt(t, poolOrigin)
	client, err := poolservice.NewClient(poolservice.ClientConfig{Version: 1, URL: relay.URL, Token: mining.Token, PublicKey: identity.GymPublicKey, Source: sourceConfig})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	admission, err := prospectservice.NewManifestClient(prospectservice.ManifestClientConfig{Version: 3, URL: admissionOrigin, Token: mining.Token, AuthorityPin: issuerInfo.Pin}, policy)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admission.Close)
	config := prospectworker.PoolConfig{Version: 1, Source: sourceConfig, PublicKey: identity.GymPublicKey, AuthorityOrigin: relay.URL, TransportTrustID: client.TransportTrustID(), AdmissionOrigin: admissionOrigin, AdmissionTrustID: admission.TransportTrustID(), MaxRecords: 1, MaxBytes: 64 << 20}
	path := filepath.Join(t.TempDir(), "worker-outbox")
	outbox, err := prospectworker.CreatePoolOutbox(path, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = outbox.Close() })
	// The zero256 gym scalar is published in gymaccount/store_test.go.
	raw, _ := hex.DecodeString("9d3035e7440dd3737fe58ffb69ca3576257d424b6e6172b3199a66a8897bddf6")
	key, _ := btcec.PrivKeyFromBytes(raw)
	clear(raw)
	t.Cleanup(key.Zero)
	signs := 0
	signer := func(ctx context.Context, c prospect.Context, p prospect.Proof) (gymauth.Authorization, error) {
		if err := ctx.Err(); err != nil {
			return gymauth.Authorization{}, err
		}
		signs++
		return gymauth.SignAdmission(c, p, key)
	}
	firstCtx, stopFirst := context.WithCancel(ctx)
	t.Cleanup(stopFirst)
	authority := &serviceWorkerAuthority{Client: client, stop: stopFirst}
	worker, err := prospectworker.NewPool(config, authority, admission, nodeClient, signer, outbox)
	if err != nil {
		t.Fatal(err)
	}
	if err = worker.Run(firstCtx); err != nil {
		t.Fatal(err)
	}
	rows, err := outbox.List(0, 8)
	if err != nil || len(rows) != 1 || rows[0].PoolReceipt != nil || rows[0].PoolState != "pending" || rows[0].Fighter == nil || rows[0].Fighter.Authorization != nil || authority.submits != 1 || authority.issues != 1 || signs != 0 || worker.Status().Attempts != 1 {
		t.Fatalf("lost HTTP receipt was not retained before signing: rows=%+v authority=%+v status=%+v signs=%d err=%v", rows, authority, worker.Status(), signs, err)
	}
	original := rows[0]
	if err = outbox.Close(); err != nil {
		t.Fatal(err)
	}
	outbox, err = prospectworker.OpenPoolOutbox(path, config)
	if err != nil {
		t.Fatal(err)
	}
	secondCtx, stopSecond := context.WithCancel(ctx)
	t.Cleanup(stopSecond)
	observed := 0
	worker, err = prospectworker.NewPoolWithHooks(config, authority, admission, nodeClient, signer, outbox, func(_ context.Context, record registry.ManifestRecord) error {
		observed++
		if record.Job != original.Work.Job || record.Verified.WorkHash != original.WorkHash || record.Birth.BornAtMS != original.Work.Job.IssuedAtMS {
			return errors.New("recovered fighter changed original job, proof or birth")
		}
		stopSecond()
		return nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err = worker.Run(secondCtx); err != nil {
		t.Fatal(err)
	}
	rows, err = outbox.List(0, 8)
	if err != nil || len(rows) != 1 || rows[0].PoolReceipt == nil || rows[0].PoolState != "accepted" || rows[0].Fighter == nil || rows[0].Fighter.Receipt == nil || rows[0].Fighter.AdmissionState != "accepted" || rows[0].WorkHash != original.WorkHash || !reflect.DeepEqual(rows[0].Work, original.Work) || rows[0].Nonce != original.Nonce {
		t.Fatalf("HTTP restart did not recover independent original receipts: %+v %v", rows, err)
	}
	if authority.submits != 1 || authority.issues != 1 || !authority.recoveredExact || signs != 1 || observed != 1 || worker.Status().Attempts != 0 {
		t.Fatalf("HTTP recovery reissued, started new mining, resigned or resubmitted: authority=%+v signs=%d observed=%d status=%+v", authority, signs, observed, worker.Status())
	}
	page, err := source.AccountingPage(ctx, 0, 0, 8)
	if err != nil || page.Through != 1 || len(page.Entries) != 1 || page.Entries[0].Share.DuplicateKey != original.DuplicateKey {
		t.Fatalf("recovery duplicated accepted pool work: %+v %v", page, err)
	}
	issuerInfo, err = issuer.Info()
	if err != nil || issuerInfo.Jobs != 1 || issuerInfo.Records != 1 {
		t.Fatalf("shared issuer created duplicate birth/job: %+v %v", issuerInfo, err)
	}
	if root := os.Getenv("SLUG_POOL_SERVICE_ARTIFACTS"); root != "" {
		observation := struct {
			JobID, WorkHash            string
			Nonce                      uint32
			PoolSequence, FighterCount uint64
			Block                      bool
			Issued, Submitted, Signed  int
			RecoveryAttempts           uint64
		}{original.Work.Job.ID, original.WorkHash, original.Nonce, page.Through, issuerInfo.Records, rows[0].PoolReceipt.Block, authority.issues, authority.submits, signs, worker.Status().Attempts}
		raw, err := json.MarshalIndent(observation, "", "  ")
		if err != nil || !filepath.IsAbs(root) {
			t.Fatal("invalid worker observation destination")
		}
		file, err := os.OpenFile(filepath.Join(root, "worker-observation.json"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			t.Fatal(err)
		}
		_, writeErr := file.Write(append(raw, '\n'))
		if err = errors.Join(writeErr, file.Close()); err != nil {
			t.Fatal(err)
		}
	}
}
