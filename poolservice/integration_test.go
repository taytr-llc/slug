// Copyright (c) 2026 The SLUG developers. ISC license.
package poolservice_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/taytr-llc/slug/address/v2"
	"github.com/taytr-llc/slug/blockchain"
	"github.com/taytr-llc/slug/btcec/v2"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/poolround"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/poolsettlefixture"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectwork"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

// This test RPC is restricted by construction to newly owned loopback nodes.
// Credentials and all signing scalars below are explicitly public fixtures.
type serviceNode struct {
	config prospectnode.RPCConfig
	cmd    *exec.Cmd
	done   chan error
	once   sync.Once
}

func servicePort(t *testing.T) string {
	t.Helper()
	l, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	value := l.Addr().String()
	if e = l.Close(); e != nil {
		t.Fatal(e)
	}
	return value
}
func (n *serviceNode) call(ctx context.Context, method string, params []any, out any) error {
	raw, e := json.Marshal(map[string]any{"jsonrpc": "1.0", "id": 1, "method": method, "params": params})
	if e != nil {
		return e
	}
	r, e := http.NewRequestWithContext(ctx, "POST", n.config.URL, bytes.NewReader(raw))
	if e != nil {
		return e
	}
	r.SetBasicAuth(n.config.Username, n.config.Password)
	r.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true}}
	defer client.CloseIdleConnections()
	response, e := client.Do(r)
	if e != nil {
		return errors.New("owned fixture RPC unavailable")
	}
	defer response.Body.Close()
	data, e := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if e != nil || response.StatusCode != 200 {
		return errors.New("owned fixture RPC response unavailable")
	}
	var envelope struct {
		Result json.RawMessage
		Error  *struct {
			Code    int
			Message string
		}
	}
	if json.Unmarshal(data, &envelope) != nil || envelope.Error != nil {
		return errors.New("owned fixture RPC refused")
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(envelope.Result, out)
}
func startServiceNode(t *testing.T, ctx context.Context, binary, connect, listen string) *serviceNode {
	t.Helper()
	home := t.TempDir()
	rpc := servicePort(t)
	config := prospectnode.RPCConfig{Version: 1, URL: "http://" + rpc, Username: "public-pool-service-fixture", Password: "public-fixture-password-not-secret"}
	conf := fmt.Sprintf("rpcuser=%s\nrpcpass=%s\nnotls=1\nrpclisten=%s\nnodnsseed=1\nconnect=%s\nlocalwalletpilot=1\ntxindex=1\ndatadir=%s\nlogdir=%s\nminingaddr=%s\n", config.Username, config.Password, rpc, connect, filepath.Join(home, "chain"), filepath.Join(home, "logs"), poolsettlefixture.PoolAddress())
	if listen == "" {
		conf += "nolisten=1\n"
	} else {
		conf += "listen=" + listen + "\n"
	}
	path := filepath.Join(home, "fixture.conf")
	if e := os.WriteFile(path, []byte(conf), 0600); e != nil {
		t.Fatal(e)
	}
	logPath := filepath.Join(home, "node.log")
	log, e := os.OpenFile(logPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		t.Fatal(e)
	}
	n := &serviceNode{config: config, cmd: exec.Command(binary, "--configfile="+path), done: make(chan error, 1)}
	n.cmd.Stdout, n.cmd.Stderr = log, log
	if e = n.cmd.Start(); e != nil {
		log.Close()
		t.Fatal(e)
	}
	go func() { e := n.cmd.Wait(); log.Close(); n.done <- e }()
	t.Cleanup(func() {
		n.stop(t)
		if root := os.Getenv("SLUG_POOL_SERVICE_ARTIFACTS"); root != "" {
			if !filepath.IsAbs(root) {
				t.Error("absolute fixture artifact path required")
				return
			}
			name := "primary-node.log"
			if listen != "" {
				name = "peer-node.log"
			}
			data, e := os.ReadFile(logPath)
			if e != nil {
				t.Error(e)
				return
			}
			f, e := os.OpenFile(filepath.Join(root, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			if e != nil {
				t.Error(e)
				return
			}
			_, e = f.Write(data)
			if errors.Join(e, f.Close()) != nil {
				t.Error("preserve owned node log")
			}
		}
	})
	deadline := time.Now().Add(25 * time.Second)
	for time.Now().Before(deadline) {
		var height uint32
		if n.call(ctx, "getblockcount", nil, &height) == nil {
			return n
		}
		select {
		case e := <-n.done:
			n.done <- e
			data, _ := os.ReadFile(logPath)
			t.Fatalf("owned fixture node exited: %v\n%s", e, data)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(50 * time.Millisecond):
		}
	}
	data, _ := os.ReadFile(logPath)
	t.Fatalf("owned fixture startup deadline\n%s", data)
	return nil
}
func (n *serviceNode) stop(t *testing.T) {
	t.Helper()
	n.once.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		e := n.call(ctx, "stop", nil, nil)
		cancel()
		if e != nil {
			_ = n.cmd.Process.Signal(os.Interrupt)
		}
		select {
		case e := <-n.done:
			if e != nil {
				t.Errorf("owned node shutdown: %v", e)
			}
		case <-time.After(20 * time.Second):
			_ = n.cmd.Process.Kill()
			<-n.done
			t.Error("owned node forced shutdown")
		}
	})
}

type serviceTip struct {
	Height uint32 `json:"height"`
	Hash   string `json:"hash"`
}

func waitServicePeers(t *testing.T, ctx context.Context, a, b *serviceNode) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		var x, y serviceTip
		var peers []struct{ Addr string }
		if a.call(ctx, "getbestblock", nil, &x) == nil && b.call(ctx, "getbestblock", nil, &y) == nil && a.call(ctx, "getpeerinfo", nil, &peers) == nil && len(peers) == 1 && x == y {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(50 * time.Millisecond):
		}
	}
	t.Fatal("owned fixture peers did not synchronize")
}
func advanceServiceChain(t *testing.T, ctx context.Context, n *serviceNode, target uint32) {
	t.Helper()
	for {
		var tip serviceTip
		if e := n.call(ctx, "getbestblock", nil, &tip); e != nil {
			t.Fatal(e)
		}
		if tip.Height >= target {
			return
		}
		count := min(target-tip.Height, uint32(8))
		var hashes []string
		if e := n.call(ctx, "generate", []any{count}, &hashes); e != nil || len(hashes) != int(count) {
			t.Fatalf("normal fixture generation: %v %d/%d", e, len(hashes), count)
		}
		t.Logf("normal pool HTTP fixture advanced to height %d", tip.Height+count)
	}
}

type serviceSessions struct{ store *gymaccount.Store }

func (s serviceSessions) WithSession(ctx context.Context, token string, fn func(string, func() error) error) error {
	return s.store.WithSession(ctx, token, gymaccount.Mining, func(a gymaccount.Account, check func() error) error {
		if a.Enrollment == nil {
			return gymaccount.ErrUnauthorized
		}
		return fn(a.Enrollment.GymPublicKey, check)
	})
}
func enrollServiceGym(t *testing.T, ctx context.Context, accounts *gymaccount.Store, subject, action string) (gymaccount.Session, gymaccount.Session, gymaccount.Identity) {
	t.Helper()
	session, e := accounts.Login(ctx, gymaccount.Principal{Issuer: "fixture:local", Subject: subject, AuthenticatedAtMS: time.Now().UnixMilli()})
	if e != nil {
		t.Fatal(e)
	}
	challenge, e := accounts.BeginChallenge(ctx, session.Token, action)
	if e != nil {
		t.Fatal(e)
	}
	// Published gym scalars from gymaccount/store_test.go, derived from the
	// explicit zero256/incrementing256 reference fixtures, never user keys.
	scalar := "9d3035e7440dd3737fe58ffb69ca3576257d424b6e6172b3199a66a8897bddf6"
	fixture := "zero256"
	if subject == "bob" {
		scalar = "948996ecaa9fa9707693267d6d4abefe0b5fb28b6ebae34f542e106244d254e1"
		fixture = "incrementing256"
	}
	raw, _ := hex.DecodeString(scalar)
	key, _ := btcec.PrivKeyFromBytes(raw)
	clear(raw)
	defer key.Zero()
	auth, e := gymaccount.SignChallenge(challenge, key)
	if e != nil {
		t.Fatal(e)
	}
	if _, _, e = accounts.CompleteChallenge(ctx, session.Token, challenge.ChallengeID, auth); e != nil {
		t.Fatal(e)
	}
	mining, e := accounts.MintMining(ctx, session.Token)
	if e != nil {
		t.Fatal(e)
	}
	identity, e := gymaccount.FixtureIdentity(fixture)
	if e != nil {
		t.Fatal(e)
	}
	return session, mining, identity
}
func serviceScript(public string) string {
	raw, _ := hex.DecodeString(public)
	return hex.EncodeToString(append([]byte{0, 20}, address.Hash160(raw)...))
}
func serviceNonce(t *testing.T, ctx context.Context, w poolshare.MiningWork, c poolshare.Config, wantBlock bool) uint32 {
	t.Helper()
	prepared, e := prospectwork.PrepareManifestWork(w.Job, w.Template, c.PayoutAddress, 0, c.PoolTarget)
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := hex.DecodeString(prepared.Candidate().Proof.Header)
	var header wire.BlockHeader
	if header.Deserialize(bytes.NewReader(raw)) != nil {
		t.Fatal("owned candidate header")
	}
	target := blockchain.CompactToBig(header.Bits)
	for nonce := uint32(0); nonce < 64; nonce++ {
		if ctx.Err() != nil {
			t.Fatal(ctx.Err())
		}
		header.Nonce = nonce
		h := header.BlockHash()
		if (blockchain.HashToBig(&h).Cmp(target) <= 0) == wantBlock {
			return nonce
		}
	}
	t.Fatal("bounded real nonce search exhausted")
	return 0
}

// The proxy deliberately drops one response only after reading the committed
// upstream receipt. It does not alter work, credentials, or receipt bytes.
func lostServiceReceipt(t *testing.T, target string) *httptest.Server {
	t.Helper()
	origin, e := url.Parse(target)
	if e != nil {
		t.Fatal(e)
	}
	transport := &http.Transport{Proxy: nil, DisableKeepAlives: true}
	lost := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		request := r.Clone(r.Context())
		request.URL.Scheme, request.URL.Host = origin.Scheme, origin.Host
		request.Host = origin.Host
		request.RequestURI = ""
		response, e := transport.RoundTrip(request)
		if e != nil {
			http.Error(w, "fixture relay unavailable", 503)
			return
		}
		defer response.Body.Close()
		raw, e := io.ReadAll(io.LimitReader(response.Body, poolservice.MaxWorkResponseSize+1))
		if e != nil {
			http.Error(w, "fixture response unavailable", 503)
			return
		}
		if !lost && r.URL.Path == "/pool/v1/submit" && response.StatusCode == 200 {
			lost = true
			connection, _, e := w.(http.Hijacker).Hijack()
			if e == nil {
				connection.Close()
			}
			return
		}
		for key, values := range response.Header {
			for _, value := range values {
				w.Header().Add(key, value)
			}
		}
		w.WriteHeader(response.StatusCode)
		_, _ = w.Write(raw)
	}))
	t.Cleanup(func() { server.Close(); transport.CloseIdleConnections() })
	return server
}

// Opt-in proof crosses the real HTTP service with two real fixture-account
// leases, then the concrete round/settlement stores and an owned normal chain.
func TestActualNormalPoolService(t *testing.T) {
	binary := os.Getenv("SLUG_POOL_SERVICE_NODE")
	if binary == "" {
		t.Skip("set SLUG_POOL_SERVICE_NODE to an explicit normal-PoW node")
	}
	if wire.TestPoW {
		t.Fatal("actual pool HTTP test requires normal PoW")
	}
	info, e := os.Lstat(binary)
	if !filepath.IsAbs(binary) || e != nil || !info.Mode().IsRegular() || info.Mode()&0111 == 0 {
		t.Fatal("explicit normal node binary required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	peerAddress := servicePort(t)
	peer := startServiceNode(t, ctx, binary, "127.0.0.1:1", peerAddress)
	node := startServiceNode(t, ctx, binary, peerAddress, "")
	waitServicePeers(t, ctx, node, peer)
	accounts, e := gymaccount.Create(filepath.Join(t.TempDir(), "accounts"), gymaccount.Config{Version: 1, Principals: []gymaccount.FixturePrincipal{{Issuer: "fixture:local", Subject: "alice", Fixture: "zero256"}, {Issuer: "fixture:local", Subject: "bob", Fixture: "incrementing256"}}, MaxSessions: 32, MaxChallenges: 16, MaxEvents: 64})
	if e != nil {
		t.Fatal(e)
	}
	defer accounts.Close()
	_, miningA, identityA := enrollServiceGym(t, ctx, accounts, "alice", "enroll")
	_, miningB, identityB := enrollServiceGym(t, ctx, accounts, "bob", "enroll")
	sessions := serviceSessions{accounts}
	descriptor, e := fighterdescriptor.ID(fighterdescriptor.CandidateDescriptor())
	if e != nil {
		t.Fatal(e)
	}
	issuer, e := registry.CreateManifestAuthority(filepath.Join(t.TempDir(), "issuer"), uint32(chaincfg.MainNetParams.Net), chaincfg.MainNetParams.GenesisHash.String(), descriptor)
	if e != nil {
		t.Fatal(e)
	}
	defer issuer.Close()
	issuerInfo, e := issuer.Info()
	if e != nil {
		t.Fatal(e)
	}
	nodeClient, e := prospectnode.NewClient(node.config)
	if e != nil {
		t.Fatal(e)
	}
	defer nodeClient.Close()
	policy := prospectnode.Policy{Version: 1, Network: uint32(chaincfg.MainNetParams.Net), Genesis: chaincfg.MainNetParams.GenesisHash.String(), DNAVersion: 1, ProspectTarget: strings.Repeat("f", 64)}
	sourceConfig := poolshare.Config{Version: 1, AuthorityPin: issuerInfo.Pin, Policy: policy, PayoutAddress: poolsettlefixture.PoolAddress(), PoolTarget: strings.Repeat("f", 64), MaxWorks: 16, MaxSubmissions: 8, MaxBytes: 16 << 20}
	source, e := poolshare.Create(filepath.Join(t.TempDir(), "shares"), sourceConfig, issuer, nodeClient, sessions)
	if e != nil {
		t.Fatal(e)
	}
	defer source.Close()
	rounds, e := poolround.Create(filepath.Join(t.TempDir(), "rounds"), poolround.Limits{Version: 1, MaxRounds: 8, MaxBytes: 16 << 20}, source, nodeClient)
	if e != nil {
		t.Fatal(e)
	}
	defer rounds.Close()
	backend, e := poolsettlefixture.New(ctx, node.config)
	if e != nil {
		t.Fatal(e)
	}
	defer backend.Close()
	payoutConfig := poolsettle.Config{Version: 1, ChangeScript: poolsettlefixture.PoolScript(), Destinations: []poolsettle.Destination{{GymID: identityA.GymID, Script: serviceScript(identityA.GymPublicKey)}, {GymID: identityB.GymID, Script: serviceScript(identityB.GymPublicKey)}}, MaxRounds: 8, MaxBatches: 8}
	sort.Slice(payoutConfig.Destinations, func(i, j int) bool { return payoutConfig.Destinations[i].GymID < payoutConfig.Destinations[j].GymID })
	settlement, e := poolsettle.Create(filepath.Join(t.TempDir(), "settlement"), payoutConfig, rounds, backend)
	if e != nil {
		t.Fatal(e)
	}
	defer settlement.Close()
	server, e := poolservice.NewServer(poolservice.ServerConfig{Version: 1, Listen: "127.0.0.1:0"}, source, settlement, sessions)
	if e != nil {
		t.Fatal(e)
	}
	runCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	ready := make(chan string, 1)
	go func() { done <- server.Run(runCtx, func(origin string) { ready <- origin }) }()
	defer func() {
		stop()
		if e := <-done; e != nil {
			t.Error(e)
		}
	}()
	var origin string
	select {
	case origin = <-ready:
	case e := <-done:
		done <- e
		t.Fatalf("pool service startup: %v", e)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	newClient := func(origin, token, public string) *poolservice.Client {
		client, e := poolservice.NewClient(poolservice.ClientConfig{Version: 1, URL: origin, Token: token, PublicKey: public, Source: sourceConfig})
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(client.Close)
		return client
	}
	clientA := newClient(origin, miningA.Token, identityA.GymPublicKey)
	clientB := newClient(origin, miningB.Token, identityB.GymPublicKey)
	wA, e := clientA.Issue(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if wA.Job.PublicKey != identityA.GymPublicKey {
		t.Fatal("wrong issued gym")
	}
	recovered, e := clientA.Work(ctx, wA.Job.ID)
	if e != nil || !reflect.DeepEqual(wA, recovered) {
		t.Fatalf("HTTP original work recovery: %v", e)
	}
	if _, e = clientB.Work(ctx, wA.Job.ID); e == nil {
		t.Fatal("other gym recovered original work")
	}
	owned, e := source.RecoverMiningWork(ctx, miningA.Token, wA.Job.ID)
	if e != nil {
		t.Fatal(e)
	}
	if owned.Template.Block.CoinbaseValue == nil {
		t.Fatal("issued coinbase amount missing")
	}
	(*owned.Template.Block.CoinbaseValue)++
	again, e := source.RecoverMiningWork(ctx, miningA.Token, wA.Job.ID)
	if e != nil || !reflect.DeepEqual(again, wA) {
		t.Fatal("mining template export aliases stored work")
	}
	nonceA := serviceNonce(t, ctx, wA, sourceConfig, false)
	ordinary, e := clientA.Submit(ctx, wA, nonceA)
	if e != nil || ordinary.AcceptedSequence != 1 {
		t.Fatalf("ordinary HTTP pool share: %v %+v", e, ordinary)
	}
	if _, e = clientB.Lookup(ctx, wA, nonceA); e == nil {
		t.Fatal("other gym read original receipt")
	}
	wB, e := clientB.Issue(ctx)
	if e != nil {
		t.Fatal(e)
	}
	nonceB := serviceNonce(t, ctx, wB, sourceConfig, true)
	proxy := lostServiceReceipt(t, origin)
	lossClient := newClient(proxy.URL, miningB.Token, identityB.GymPublicKey)
	if _, e = lossClient.Submit(ctx, wB, nonceB); e == nil {
		t.Fatal("deliberately lost share receipt was acknowledged")
	} else {
		var failure *poolservice.ClientError
		if !errors.As(e, &failure) || !failure.OutcomeUnknown {
			t.Fatalf("lost receipt did not preserve outcome uncertainty: %v", e)
		}
	}
	winner, e := clientB.Lookup(ctx, wB, nonceB)
	if e != nil || winner.AcceptedSequence != 2 || !winner.Block {
		t.Fatalf("recover exact durable winning share: %v %+v", e, winner)
	}
	same, e := clientB.Submit(ctx, wB, nonceB)
	if e != nil || same.AcceptedSequence != winner.AcceptedSequence || same.DuplicateKey != winner.DuplicateKey {
		t.Fatal("HTTP retry granted duplicate share")
	}
	if _, e = rounds.Sync(ctx); e != nil {
		t.Fatal(e)
	}
	advanceServiceChain(t, ctx, node, winner.BlockHeight+60)
	record, e := rounds.Refresh(ctx, winner.WorkHash)
	if e != nil || record.Confirmation == nil {
		t.Fatalf("HTTP winner round confirmation: %v", e)
	}
	if _, e = settlement.RunDaily(ctx); e != nil {
		t.Fatal(e)
	}
	for _, client := range []*poolservice.Client{clientA, clientB} {
		balance, e := client.Balance(ctx)
		if e != nil || balance.Balance.Confirmed != (poolsettle.Fraction{Numerator: "495000000", Denominator: "1"}) || balance.Balance.Reason != "immature" || balance.ObservedAtMS == 0 {
			t.Fatalf("visible HTTP immature balance: %v %+v", e, balance)
		}
	}
	advanceServiceChain(t, ctx, node, winner.BlockHeight+99)
	paid, e := settlement.RunDaily(ctx)
	if e != nil || paid.TxID == "" {
		t.Fatalf("actual HTTP-origin earnings payout: %v %+v", e, paid)
	}
	advanceServiceChain(t, ctx, node, winner.BlockHeight+100)
	confirmed, e := settlement.RunDaily(ctx)
	if e != nil || confirmed.TxID != paid.TxID {
		t.Fatal("payout changed on retry")
	}
	for i, client := range []*poolservice.Client{clientA, clientB} {
		balance, e := client.Balance(ctx)
		wantGym := []string{identityA.GymID, identityB.GymID}[i]
		if e != nil || balance.Balance.GymID != wantGym || balance.Balance.PaidAtoms != 495000000 || balance.Balance.Confirmed != (poolsettle.Fraction{Numerator: "0", Denominator: "1"}) {
			t.Fatalf("own HTTP paid balance: %v %+v", e, balance)
		}
	}
	observation, e := backend.ObserveTransaction(ctx, paid.TxID)
	if e != nil || observation.Confirmations != 1 {
		t.Fatalf("actual payment missing: %v", e)
	}
	var tx wire.MsgTx
	if tx.Deserialize(bytes.NewReader(observation.Raw)) != nil || len(tx.TxOut) != 3 {
		t.Fatal("actual payment shape")
	}
	expected := map[string]int64{serviceScript(identityA.GymPublicKey): 495000000, serviceScript(identityB.GymPublicKey): 495000000, poolsettlefixture.PoolScript(): 9999000}
	for _, out := range tx.TxOut {
		script := hex.EncodeToString(out.PkScript)
		value, ok := expected[script]
		if !ok || value != out.Value {
			t.Fatal("actual payments or fee differ")
		}
		delete(expected, script)
	}
	if len(expected) != 0 {
		t.Fatal("missing actual payment")
	}
	reset := gymaccount.Principal{Issuer: "fixture:local", Subject: "alice", AuthenticatedAtMS: time.Now().UnixMilli(), RecoveryVerified: true, RecoveryEventID: "pool-http-reset"}
	if _, _, e = accounts.ApplyReset(ctx, reset); e != nil {
		t.Fatal(e)
	}
	if _, e = clientA.Balance(ctx); e == nil {
		t.Fatal("reset did not revoke old balance token")
	}
	if _, e = clientA.Work(ctx, wA.Job.ID); e == nil {
		t.Fatal("reset did not revoke old work token")
	}
	if _, e = clientB.Balance(ctx); e != nil {
		t.Fatalf("other gym revoked: %v", e)
	}
	_, freshA, _ := enrollServiceGym(t, ctx, accounts, "alice", "recover")
	freshClient := newClient(origin, freshA.Token, identityA.GymPublicKey)
	balance, e := freshClient.Balance(ctx)
	if e != nil || balance.Balance.PaidAtoms != 495000000 {
		t.Fatalf("held recovered account lost balance access: %v", e)
	}
	if _, e = freshClient.Issue(ctx); e != nil {
		t.Fatalf("held recovered account lost mining access: %v", e)
	}
	waitServicePeers(t, ctx, node, peer)
	t.Logf("actual HTTP pool winner=%s; durable lost-receipt lookup and duplicate share retry preserve accepted_sequence=2; 60-descendant immature balance -> one mature payout=%s; two authenticated gyms each paid495000000atoms; reset revokes old token, preserves other gym and held-account viewing/mining", winner.WorkHash, paid.TxID)
}
