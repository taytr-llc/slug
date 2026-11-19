//go:build desktopintegration && !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/taytr-llc/slug/blockchain"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/poolround"
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/poolsettlefixture"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

func recipientIntegrationPort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	a := l.Addr().String()
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	return a
}

// Existing desktop nodes deliberately retain their original configuration.
// This integration alone owns an explicit txindex-enabled pool node pair.
func recipientIntegrationNode(t *testing.T, ctx context.Context, binary, connect, listen, name string) *integrationNode {
	t.Helper()
	home, rpc := t.TempDir(), recipientIntegrationPort(t)
	conf := fmt.Sprintf("rpcuser=fixture\nrpcpass=%s\nnotls=1\nrpclisten=%s\nnodnsseed=1\nconnect=%s\nlocalwalletpilot=1\ntxindex=1\ndatadir=%s\nlogdir=%s\nminingaddr=%s\n", integrationFixturePassword, rpc, connect, filepath.Join(home, "chain"), filepath.Join(home, "logs"), poolsettlefixture.PoolAddress())
	if listen == "" {
		conf += "nolisten=1\n"
	} else {
		conf += "listen=" + listen + "\n"
	}
	config := filepath.Join(home, "fixture.conf")
	if err := os.WriteFile(config, []byte(conf), 0600); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(home, "node.log")
	log, err := os.OpenFile(logPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	n := &integrationNode{rpc: newIntegrationRPC("http://" + rpc), cmd: exec.Command(binary, "--configfile="+config), done: make(chan error, 1)}
	n.cmd.Stdout, n.cmd.Stderr = log, log
	if err = n.cmd.Start(); err != nil {
		_ = log.Close()
		t.Fatal(err)
	}
	go func() { err := n.cmd.Wait(); _ = log.Close(); n.done <- err }()
	t.Cleanup(func() {
		n.close(t)
		if root := os.Getenv("SLUG_POOL_RECIPIENT_ARTIFACTS"); root != "" {
			if !filepath.IsAbs(root) {
				t.Error("absolute artifact destination required")
				return
			}
			raw, err := os.ReadFile(logPath)
			if err != nil {
				t.Error(err)
				return
			}
			file, err := os.OpenFile(filepath.Join(root, name+"-node.log"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			if err != nil {
				t.Error(err)
				return
			}
			_, writeErr := file.Write(raw)
			closeErr := file.Close()
			if writeErr != nil || closeErr != nil {
				t.Error("preserve owned node log")
			}
		}
	})
	deadline := time.Now().Add(25 * time.Second)
	for time.Now().Before(deadline) {
		var magic uint32
		if n.rpc.call("getcurrentnet", nil, &magic) == nil {
			var genesis string
			n.rpc.must(t, "getblockhash", []any{0}, &genesis)
			if magic != uint32(chaincfg.MainNetParams.Net) || genesis != chaincfg.MainNetParams.GenesisHash.String() {
				t.Fatal("wrong owned-node identity")
			}
			return n
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(50 * time.Millisecond):
		}
	}
	t.Fatal("owned recipient node startup deadline")
	return nil
}

type recipientIntegrationSessions struct{ store *gymaccount.Store }

func (s recipientIntegrationSessions) WithSession(ctx context.Context, token string, fn func(string, func() error) error) error {
	return s.store.WithSession(ctx, token, gymaccount.Mining, func(a gymaccount.Account, check func() error) error {
		if a.Enrollment == nil {
			return gymaccount.ErrUnauthorized
		}
		return fn(a.Enrollment.GymPublicKey, check)
	})
}

type recipientIntegrationBackend struct {
	*poolsettlefixture.Backend
	broadcasts int
}

func (b *recipientIntegrationBackend) Broadcast(ctx context.Context, raw []byte) (string, error) {
	b.broadcasts++
	return b.Backend.Broadcast(ctx, raw)
}

func recipientIntegrationWaitWallet(t *testing.T, ctx context.Context, w *walletRuntime, height uint32) {
	t.Helper()
	deadline := time.Now().Add(time.Minute)
	for time.Now().Before(deadline) {
		v, err := w.Snapshot(ctx)
		if err == nil && v.Synced && v.Height == int32(height) {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(50 * time.Millisecond):
		}
	}
	t.Fatal("actual recipient wallet did not synchronize")
}
func recipientIntegrationAdvance(t *testing.T, ctx context.Context, n *integrationNode, height uint32) {
	t.Helper()
	for {
		if ctx.Err() != nil {
			t.Fatal(ctx.Err())
		}
		var current uint32
		n.rpc.must(t, "getblockcount", nil, &current)
		if current >= height {
			return
		}
		count := min(height-current, uint32(8))
		var hashes []string
		n.rpc.must(t, "generate", []any{count}, &hashes)
		if len(hashes) != int(count) {
			t.Fatal("normal generation count")
		}
		t.Logf("normal recipient fixture height %d", current+count)
	}
}
func recipientIntegrationShare(t *testing.T, ctx context.Context, p *poolshare.Pool, token string, winner bool) poolshare.Submission {
	t.Helper()
	work, err := p.Issue(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := hex.DecodeString(work.Candidate.BlockHex)
	var block wire.MsgBlock
	if err != nil || block.Deserialize(bytes.NewReader(raw)) != nil {
		t.Fatal("issued actual candidate")
	}
	target := blockchain.CompactToBig(block.Header.Bits)
	for nonce := uint32(0); nonce < 64; nonce++ {
		if ctx.Err() != nil {
			t.Fatal(ctx.Err())
		}
		block.Header.Nonce = nonce
		hash := block.Header.BlockHash()
		if (blockchain.HashToBig(&hash).Cmp(target) <= 0) != winner {
			continue
		}
		receipt, err := p.Submit(ctx, token, work.Job.ID, nonce)
		if err != nil || receipt.AcceptedSequence == 0 {
			t.Fatal("actual share acceptance", err)
		}
		return receipt
	}
	t.Fatal("bounded actual nonce search exhausted")
	return poolshare.Submission{}
}

// Actual normal-PoW composition binds both real fixture wallets, settles one
// original mature pool block, and observes the exact payment through wallet RPC.
// The existing service and worker integration suites cover the HTTP mining path.
func TestPoolRecipientSettlementNormalPoW(t *testing.T) {
	binary := os.Getenv("SLUG_POOL_RECIPIENT_NODE")
	if binary == "" {
		t.Skip("set SLUG_POOL_RECIPIENT_NODE to an explicit normal node")
	}
	info, err := os.Lstat(binary)
	if !filepath.IsAbs(binary) || err != nil || !info.Mode().IsRegular() || info.Mode()&0111 == 0 {
		t.Fatal("explicit normal node binary required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	t.Cleanup(cancel)
	peerAddress := recipientIntegrationPort(t)
	peer := recipientIntegrationNode(t, ctx, binary, "127.0.0.1:1", peerAddress, "peer")
	node := recipientIntegrationNode(t, ctx, binary, peerAddress, "", "primary")
	deadline := time.Now().Add(30 * time.Second)
	for {
		var peers []struct{ Addr string }
		var a, b uint32
		if node.rpc.call("getpeerinfo", nil, &peers) == nil && len(peers) == 1 && node.rpc.call("getblockcount", nil, &a) == nil && peer.rpc.call("getblockcount", nil, &b) == nil && a == b {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("owned recipient peers did not connect")
		}
		time.Sleep(50 * time.Millisecond)
	}
	rpcConfig := prospectnode.RPCConfig{Version: 1, URL: node.rpc.url, Username: "fixture", Password: integrationFixturePassword}
	nodeClient, err := prospectnode.NewClient(rpcConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(nodeClient.Close)
	issuer, err := registry.CreateManifestAuthority(filepath.Join(t.TempDir(), "issuer"), policy().Network, genesis, manifestDescriptorID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = issuer.Close() })
	issuerInfo, err := issuer.Info()
	if err != nil {
		t.Fatal(err)
	}
	sourceConfig := poolshare.Config{Version: 1, AuthorityPin: issuerInfo.Pin, Policy: policy(), PayoutAddress: poolsettlefixture.PoolAddress(), PoolTarget: strings.Repeat("f", 64), MaxWorks: 8, MaxSubmissions: 8, MaxBytes: 16 << 20}
	accounts, err := gymaccount.Create(filepath.Join(t.TempDir(), "accounts"), gymaccount.Config{Version: 1, Principals: []gymaccount.FixturePrincipal{{Issuer: "fixture:local", Subject: "alice", Fixture: "zero256"}, {Issuer: "fixture:local", Subject: "bob", Fixture: "incrementing256"}}, MaxSessions: 16, MaxChallenges: 8, MaxEvents: 32})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = accounts.Close() })
	sessions := recipientIntegrationSessions{accounts}
	var wallets []*walletRuntime
	var bindings []PoolRecipientBinding
	var tokens []string
	for i, name := range []string{"zero256", "incrementing256"} {
		root := t.TempDir()
		w, err := openAccountFixtureWallet(filepath.Join(root, "wallet"), name, true)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = w.Close() })
		if err = w.Attach(ctx, rpcConfig); err != nil {
			t.Fatal(err)
		}
		recipientIntegrationWaitWallet(t, ctx, w, 0)
		login, err := accounts.Login(ctx, gymaccount.Principal{Issuer: "fixture:local", Subject: []string{"alice", "bob"}[i], AuthenticatedAtMS: time.Now().UnixMilli()})
		if err != nil {
			t.Fatal(err)
		}
		challenge, err := accounts.BeginChallenge(ctx, login.Token, "enroll")
		if err != nil {
			t.Fatal(err)
		}
		auth, err := w.SignAccountChallenge(ctx, accounts.AuthorityID(), challenge)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err = accounts.CompleteChallenge(ctx, login.Token, challenge.ChallengeID, auth); err != nil {
			t.Fatal(err)
		}
		mining, err := accounts.MintMining(ctx, login.Token)
		if err != nil {
			t.Fatal(err)
		}
		identity, err := gymaccount.FixtureIdentity(name)
		if err != nil {
			t.Fatal(err)
		}
		config := PoolRecipientConfig{Version: 1, AccountAuthorityID: accounts.AuthorityID(), AccountID: login.Account.ID, Fixture: name, Identity: identity, Source: sourceConfig}
		view := ManifestAccountConfig{Account: AccountConfig{Fixture: name, AccountID: login.Account.ID, AuthorityID: accounts.AuthorityID(), Authorizer: sessions, MiningToken: func(context.Context) (string, error) { return mining.Token, nil }}, ViewID: strings.Repeat(fmt.Sprint(i+1), 64), Generation: login.Account.Generation, ViewLease: func(ctx context.Context, fn func(gymaccount.Account, func() error) error) error {
			return accounts.WithSession(ctx, login.Token, gymaccount.View, fn)
		}}
		binding, err := ensurePoolRecipientBinding(ctx, root, config, view, w, true)
		if err != nil {
			t.Fatal("actual immutable recipient binding", err)
		}
		if binding.Recipient != poolPublicRecipient(poolRecipientOracle(t, name)) || ValidatePoolRecipientBinding(binding) != nil {
			t.Fatal("binding differs from independently frozen wallet recipient")
		}
		ready, err := os.ReadFile(filepath.Join(root, poolRecipientReadyName))
		if err != nil {
			t.Fatal(err)
		}
		again, err := ensurePoolRecipientBinding(ctx, root, config, view, w, false)
		after, readErr := os.ReadFile(filepath.Join(root, poolRecipientReadyName))
		if err != nil || readErr != nil || again != binding || !bytes.Equal(ready, after) || w.rpc.callCounts()["getnewaddress"] != 1 {
			t.Fatal("ready binding retry changed recipient or allocated again", err)
		}
		t.Logf("bound actual %s wallet to native84 recipient %s", name, binding.Recipient.Address)
		wallets = append(wallets, w)
		bindings = append(bindings, binding)
		tokens = append(tokens, mining.Token)
	}
	source, err := poolshare.Create(filepath.Join(t.TempDir(), "shares"), sourceConfig, issuer, nodeClient, sessions)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = source.Close() })
	ordinary := recipientIntegrationShare(t, ctx, source, tokens[0], false)
	winner := recipientIntegrationShare(t, ctx, source, tokens[1], true)
	if ordinary.AcceptedSequence != 1 || winner.AcceptedSequence != 2 {
		t.Fatal("actual round membership")
	}
	rounds, err := poolround.Create(filepath.Join(t.TempDir(), "rounds"), poolround.Limits{Version: 1, MaxRounds: 8, MaxBytes: 16 << 20}, source, nodeClient)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = rounds.Close() })
	if _, err = rounds.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	recipientIntegrationAdvance(t, ctx, node, winner.BlockHeight+60)
	confirmed, err := rounds.Refresh(ctx, winner.WorkHash)
	if err != nil || confirmed.Confirmation == nil || confirmed.Confirmation.Descendants != 60 {
		t.Fatal("actual round finality", err)
	}
	t.Log("original pool round confirmed at 60 descendants")
	backend, err := poolsettlefixture.New(ctx, rpcConfig)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(backend.Close)
	counted := &recipientIntegrationBackend{Backend: backend}
	config := poolsettle.Config{Version: 1, ChangeScript: poolsettlefixture.PoolScript(), MaxRounds: 8, MaxBatches: 8}
	for _, b := range bindings {
		config.Destinations = append(config.Destinations, poolsettle.Destination{GymID: b.Config.Identity.GymID, Script: b.Recipient.Script})
	}
	sort.Slice(config.Destinations, func(i, j int) bool { return config.Destinations[i].GymID < config.Destinations[j].GymID })
	store, err := poolsettle.Create(filepath.Join(t.TempDir(), "settlement"), config, rounds, counted)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	deferred, err := store.RunDaily(ctx)
	if err != nil || deferred.TxID != "" || counted.broadcasts != 0 {
		t.Fatal("immature round was paid", err)
	}
	recipientIntegrationAdvance(t, ctx, node, winner.BlockHeight+99)
	paid, err := store.RunDaily(ctx)
	if err != nil || paid.TxID == "" || counted.broadcasts != 1 {
		t.Fatal("mature native84 payment", err)
	}
	t.Logf("mature original pool reward paid in transaction %s", paid.TxID)
	recipientIntegrationAdvance(t, ctx, node, winner.BlockHeight+100)
	retry, err := store.RunDaily(ctx)
	if err != nil || retry.TxID != paid.TxID || counted.broadcasts != 1 {
		t.Fatal("same-day retry paid again", err)
	}
	for i, w := range wallets {
		recipientIntegrationWaitWallet(t, ctx, w, winner.BlockHeight+100)
		state, err := w.VerificationState(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if state.View.Spendable != 495000000 || state.View.Immature != 0 || state.View.Unconfirmed != 0 {
			t.Fatalf("wallet did not receive exact native84 share: %+v", state.View)
		}
		if len(state.UTXOs) != 1 || state.UTXOs[0].TxID != paid.TxID || state.UTXOs[0].Address != bindings[i].Recipient.Address || state.UTXOs[0].ScriptPubKey != bindings[i].Recipient.Script || state.UTXOs[0].Amount != 4.95 || state.UTXOs[0].Confirmations != 1 {
			t.Fatalf("actual wallet UTXO differs from immutable recipient: %+v", state.UTXOs)
		}
		found := 0
		for _, tx := range state.Transactions {
			if tx.TxID == paid.TxID && tx.Category == "receive" && tx.Address == bindings[i].Recipient.Address && tx.Amount == 4.95 {
				found++
			}
		}
		if found != 1 {
			t.Fatal("wallet RPC history lacks exact single pool payment")
		}
	}
	t.Logf("actual native84 payout %s paid both bound wallets 495000000 atoms; one broadcast and same-day idempotent retry", paid.TxID)
	if root := os.Getenv("SLUG_POOL_RECIPIENT_ARTIFACTS"); root != "" {
		observation := struct {
			Bindings    []PoolRecipientBinding
			TxID        string
			AtomsPerGym int64
			Broadcasts  int
			Height      uint32
		}{bindings, paid.TxID, 495000000, counted.broadcasts, winner.BlockHeight + 100}
		raw, err := json.MarshalIndent(observation, "", "  ")
		if err != nil || !filepath.IsAbs(root) {
			t.Fatal("public observation destination")
		}
		f, err := os.OpenFile(filepath.Join(root, "recipient-observation.json"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			t.Fatal(err)
		}
		_, writeErr := f.Write(append(raw, '\n'))
		closeErr := f.Close()
		if writeErr != nil || closeErr != nil {
			t.Fatal("write public recipient observation")
		}
	}
}
