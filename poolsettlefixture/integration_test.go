// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettlefixture

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
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
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/poolround"
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/txscript/v2"
	"github.com/taytr-llc/slug/wire/v2"
)

type fixtureNode struct {
	cmd    *exec.Cmd
	done   chan error
	once   sync.Once
	rpc    *Backend
	config prospectnode.RPCConfig
}

func freePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().String()
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	return port
}

func startNode(t *testing.T, ctx context.Context, binary, connect, listen, payout string) *fixtureNode {
	t.Helper()
	home := t.TempDir()
	rpcAddress := freePort(t)
	c := prospectnode.RPCConfig{Version: 1, URL: "http://" + rpcAddress, Username: "public-pool-fixture", Password: "public-pool-fixture-password-not-a-secret"}
	conf := fmt.Sprintf("rpcuser=%s\nrpcpass=%s\nnotls=1\nrpclisten=%s\nnodnsseed=1\nconnect=%s\nlocalwalletpilot=1\ntxindex=1\ndatadir=%s\nlogdir=%s\nminingaddr=%s\n", c.Username, c.Password, rpcAddress, connect, filepath.Join(home, "chain"), filepath.Join(home, "logs"), payout)
	if listen == "" {
		conf += "nolisten=1\n"
	} else {
		conf += "listen=" + listen + "\n"
	}
	path := filepath.Join(home, "fixture.conf")
	if err := os.WriteFile(path, []byte(conf), 0600); err != nil {
		t.Fatal(err)
	}
	log, err := os.OpenFile(filepath.Join(home, "node-output.log"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary, "--configfile="+path)
	cmd.Stdout, cmd.Stderr = log, log
	if err := cmd.Start(); err != nil {
		log.Close()
		t.Fatal(err)
	}
	n := &fixtureNode{cmd: cmd, done: make(chan error, 1), config: c}
	go func() { n.done <- cmd.Wait(); log.Close() }()
	t.Cleanup(func() {
		n.stop(t)
		if root := os.Getenv("SLUG_POOL_SETTLEMENT_ARTIFACTS"); root != "" {
			if !filepath.IsAbs(root) {
				t.Error("absolute fixture artifact path required")
				return
			}
			name := "primary-node.log"
			if listen != "" {
				name = "peer-node.log"
			}
			data, err := os.ReadFile(filepath.Join(home, "node-output.log"))
			if err != nil {
				t.Error(err)
				return
			}
			f, err := os.OpenFile(filepath.Join(root, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			if err != nil {
				t.Error(err)
				return
			}
			_, err = f.Write(data)
			closeErr := f.Close()
			if err != nil || closeErr != nil {
				t.Errorf("preserve owned node log: %v %v", err, closeErr)
			}
		}
	})
	deadline := time.Now().Add(25 * time.Second)
	for time.Now().Before(deadline) {
		if rpc, err := New(ctx, c); err == nil {
			n.rpc = rpc
			return n
		}
		select {
		case err := <-n.done:
			n.done <- err
			data, _ := os.ReadFile(filepath.Join(home, "node-output.log"))
			t.Fatalf("owned fixture node exited: %v\n%s", err, data)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(50 * time.Millisecond):
		}
	}
	data, _ := os.ReadFile(filepath.Join(home, "node-output.log"))
	t.Fatalf("owned fixture node startup deadline\n%s", data)
	return nil
}

func (n *fixtureNode) stop(t *testing.T) {
	t.Helper()
	n.once.Do(func() {
		if n.rpc != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			_ = n.rpc.call(ctx, "stop", nil, nil)
			cancel()
			defer n.rpc.Close()
		} else {
			_ = n.cmd.Process.Signal(os.Interrupt)
		}
		select {
		case err := <-n.done:
			if err != nil {
				t.Errorf("owned fixture node exit: %v", err)
			}
		case <-time.After(20 * time.Second):
			_ = n.cmd.Process.Kill()
			<-n.done
			t.Error("owned fixture node required forced shutdown")
		}
	})
}

func fixturePublic(n byte) (string, string, string) {
	var scalar [32]byte
	scalar[31] = n
	_, pub := btcec.PrivKeyFromBytes(scalar[:])
	public := hex.EncodeToString(pub.SerializeCompressed())
	keyHash := address.Hash160(pub.SerializeCompressed())
	addr, _ := address.NewAddressWitnessPubKeyHash(keyHash, &chaincfg.MainNetParams)
	script := hex.EncodeToString(append([]byte{txscript.OP_0, 20}, keyHash...))
	return public, addr.EncodeAddress(), script
}

type fixtureSessions struct{ keys map[string]string }

func (s fixtureSessions) WithSession(ctx context.Context, token string, fn func(string, func() error) error) error {
	key, ok := s.keys[token]
	if !ok {
		return poolshare.ErrUnauthorized
	}
	return fn(key, ctx.Err)
}

func waitPeers(t *testing.T, ctx context.Context, a, b *fixtureNode) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		var peers []struct {
			Addr string `json:"addr"`
		}
		if a.rpc.call(ctx, "getpeerinfo", nil, &peers) == nil && len(peers) == 1 {
			x, ex := a.rpc.snapshot(ctx)
			y, ey := b.rpc.snapshot(ctx)
			if ex == nil && ey == nil && x == y {
				return
			}
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(50 * time.Millisecond):
		}
	}
	t.Fatal("owned fixture peers did not synchronize")
}

func advance(t *testing.T, ctx context.Context, n *fixtureNode, target uint32) {
	t.Helper()
	for {
		before, err := n.rpc.snapshot(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if before.Height >= target {
			return
		}
		count := target - before.Height
		if count > 8 {
			count = 8
		}
		var hashes []string
		if err := n.rpc.call(ctx, "generate", []any{count}, &hashes); err != nil || len(hashes) != int(count) {
			t.Fatalf("normal fixture generation: %v, %d/%d", err, len(hashes), count)
		}
		t.Logf("normal-PoW fixture advanced to height %d", before.Height+count)
	}
}

func submitWork(t *testing.T, ctx context.Context, pool *poolshare.Pool, token string, wantBlock bool) poolshare.Submission {
	t.Helper()
	w, err := pool.Issue(ctx, token)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := hex.DecodeString(w.Candidate.BlockHex)
	if err != nil {
		t.Fatal(err)
	}
	var block wire.MsgBlock
	if block.Deserialize(bytes.NewReader(raw)) != nil {
		t.Fatal("issued candidate decode")
	}
	target := blockchain.CompactToBig(block.Header.Bits)
	for nonce := uint32(0); nonce < 64; nonce++ {
		if ctx.Err() != nil {
			t.Fatal(ctx.Err())
		}
		block.Header.Nonce = nonce
		h := block.Header.BlockHash()
		if (blockchain.HashToBig(&h).Cmp(target) <= 0) != wantBlock {
			continue
		}
		receipt, err := pool.Submit(ctx, token, w.Job.ID, nonce)
		if err != nil || receipt.AcceptedSequence == 0 || receipt.Block != wantBlock {
			t.Fatalf("actual accepted pool work: %v %+v", err, receipt)
		}
		return receipt
	}
	t.Fatal("bounded normal-PoW fixture nonce search exhausted")
	return poolshare.Submission{}
}

type lostAckBackend struct {
	*Backend
	sends int
	lose  bool
}

func (b *lostAckBackend) Broadcast(ctx context.Context, raw []byte) (string, error) {
	b.sends++
	id, err := b.Backend.Broadcast(ctx, raw)
	if err == nil && b.lose {
		b.lose = false
		return "", ErrFixture
	}
	return id, err
}

// This opt-in milestone creates a new private chain and uses only published
// fixture scalars. It never opens an existing wallet, app directory or key file.
func TestActualNormalPoolSettlement(t *testing.T) {
	binary := os.Getenv("SLUG_POOL_SETTLEMENT_NODE")
	if binary == "" {
		t.Skip("set SLUG_POOL_SETTLEMENT_NODE to an explicit normal-PoW node")
	}
	if wire.TestPoW {
		t.Fatal("actual settlement requires normal PoW")
	}
	if !filepath.IsAbs(binary) {
		t.Fatal("absolute owned node binary required")
	}
	info, err := os.Lstat(binary)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&0111 == 0 {
		t.Fatal("explicit normal node binary unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	_, sink, _ := fixturePublic(4)
	peerAddress := freePort(t)
	peer := startNode(t, ctx, binary, "127.0.0.1:1", peerAddress, sink)
	node := startNode(t, ctx, binary, peerAddress, "", sink)
	waitPeers(t, ctx, node, peer)

	descriptor, err := fighterdescriptor.ID(fighterdescriptor.CandidateDescriptor())
	if err != nil {
		t.Fatal(err)
	}
	issuer, err := registry.CreateManifestAuthority(filepath.Join(t.TempDir(), "issuer"), uint32(chaincfg.MainNetParams.Net), chaincfg.MainNetParams.GenesisHash.String(), descriptor)
	if err != nil {
		t.Fatal(err)
	}
	defer issuer.Close()
	issuerInfo, err := issuer.Info()
	if err != nil {
		t.Fatal(err)
	}
	client, err := prospectnode.NewClient(node.config)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	publicA, _, scriptA := fixturePublic(1)
	publicB, _, scriptB := fixturePublic(2)
	policy := prospectnode.Policy{Version: 1, Network: uint32(chaincfg.MainNetParams.Net), Genesis: chaincfg.MainNetParams.GenesisHash.String(), DNAVersion: 1, ProspectTarget: strings.Repeat("f", 64)}
	sourceConfig := poolshare.Config{Version: 1, AuthorityPin: issuerInfo.Pin, Policy: policy, PayoutAddress: PoolAddress(), PoolTarget: strings.Repeat("f", 64), MaxWorks: 8, MaxSubmissions: 8, MaxBytes: 16 << 20}
	source, err := poolshare.Create(filepath.Join(t.TempDir(), "shares"), sourceConfig, issuer, client, fixtureSessions{map[string]string{"a": publicA, "b": publicB}})
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	ordinary := submitWork(t, ctx, source, "a", false)
	winner := submitWork(t, ctx, source, "b", true)
	if ordinary.AcceptedSequence != 1 || winner.AcceptedSequence != 2 {
		t.Fatal("actual acceptance sequence")
	}
	rounds, err := poolround.Create(filepath.Join(t.TempDir(), "rounds"), poolround.Limits{Version: 1, MaxRounds: 8, MaxBytes: 16 << 20}, source, client)
	if err != nil {
		t.Fatal(err)
	}
	defer rounds.Close()
	if _, err := rounds.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	advance(t, ctx, node, winner.BlockHeight+60)
	round, err := rounds.Refresh(ctx, winner.WorkHash)
	if err != nil || round.Confirmation == nil || round.Confirmation.Descendants != 60 {
		t.Fatalf("actual sixty-descendant confirmation: %v %+v", err, round.Confirmation)
	}
	if round.Round.RewardAtoms != 1000000000 || len(round.Round.Allocations) != 2 {
		t.Fatal("actual equal-share reward")
	}
	// Gym IDs are independently checked by the canonical public-key contract.
	gymA, err := gymauth.GymID(round.Round.Winner.Job.Context, publicA)
	if err != nil {
		t.Fatal(err)
	}
	gymB, err := gymauth.GymID(round.Round.Winner.Job.Context, publicB)
	if err != nil {
		t.Fatal(err)
	}
	config := poolsettle.Config{Version: 1, ChangeScript: PoolScript(), Destinations: []poolsettle.Destination{{GymID: gymA, Script: scriptA}, {GymID: gymB, Script: scriptB}}, MaxRounds: 8, MaxBatches: 8}
	sort.Slice(config.Destinations, func(i, j int) bool { return config.Destinations[i].GymID < config.Destinations[j].GymID })
	backend := &lostAckBackend{Backend: node.rpc}
	path := filepath.Join(t.TempDir(), "settlement")
	store, err := poolsettle.Create(path, config, rounds, backend)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if store != nil {
			_ = store.Close()
		}
	}()
	deferred, err := store.RunDaily(ctx)
	if err != nil || deferred.TxID != "" || len(deferred.Balances) != 2 {
		t.Fatalf("immature daily deferral: %v %+v", err, deferred)
	}
	for _, b := range deferred.Balances {
		if b.Confirmed.Numerator != "495000000" || b.Confirmed.Denominator != "1" || b.Reason == "" || b.PaidAtoms != 0 {
			t.Fatalf("visible exact immature balance: %+v", b)
		}
	}
	if backend.sends != 0 {
		t.Fatal("immature coinbase was broadcast")
	}
	advance(t, ctx, node, winner.BlockHeight+98)
	if report, err := store.RunDaily(ctx); err != nil || report.TxID != "" || backend.sends != 0 {
		t.Fatalf("one block before maturity must defer: %v %+v", err, report)
	}
	advance(t, ctx, node, winner.BlockHeight+99)
	backend.lose = true
	_, _ = store.RunDaily(ctx) // Deliberately lose only the real broadcast acknowledgment.
	if backend.sends != 1 {
		t.Fatalf("actual payout broadcast attempts=%d", backend.sends)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store = nil
	store, err = poolsettle.Open(path, config, rounds, backend)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := store.RunDaily(ctx)
	if err != nil || recovered.TxID == "" || backend.sends != 1 {
		t.Fatalf("lost acknowledgment recovery: %v %+v sends=%d", err, recovered, backend.sends)
	}
	observation, err := node.rpc.ObserveTransaction(ctx, recovered.TxID)
	if err != nil || len(observation.Raw) == 0 || observation.Confirmations != 0 {
		t.Fatalf("actual mempool payout observation: %v %+v", err, observation)
	}
	var payout wire.MsgTx
	if payout.Deserialize(bytes.NewReader(observation.Raw)) != nil || len(payout.TxIn) != 1 || len(payout.TxOut) != 3 {
		t.Fatal("one actual batch transaction shape")
	}
	want := map[string]int64{scriptA: 495000000, scriptB: 495000000, PoolScript(): 9999000}
	for _, out := range payout.TxOut {
		key := hex.EncodeToString(out.PkScript)
		value, ok := want[key]
		if !ok || out.Value != value {
			t.Fatalf("exact payment or operator-funded fee differs: %d %s", out.Value, key)
		}
		delete(want, key)
	}
	if len(want) != 0 {
		t.Fatal("missing exact payout output")
	}
	advance(t, ctx, node, winner.BlockHeight+100)
	confirmed, err := store.RunDaily(ctx)
	if err != nil || confirmed.TxID != recovered.TxID || backend.sends != 1 {
		t.Fatalf("confirmed same-day retry: %v %+v", err, confirmed)
	}
	observation, err = node.rpc.ObserveTransaction(ctx, recovered.TxID)
	if err != nil || observation.Confirmations != 1 {
		t.Fatalf("actual payout confirmation: %v %+v", err, observation)
	}
	for _, b := range confirmed.Balances {
		if b.PaidAtoms != 495000000 || b.Confirmed.Numerator != "0" {
			t.Fatalf("exactly-once paid balance: %+v", b)
		}
	}
	for i := 0; i < 2; i++ {
		retry, err := store.RunDaily(ctx)
		if err != nil || retry.TxID != recovered.TxID || backend.sends != 1 {
			t.Fatalf("same-day repeat paid twice: %v %+v", err, retry)
		}
	}
	waitPeers(t, ctx, node, peer)
	t.Logf("actual normal-PoW pool winner=%s height=%d; round confirmed at60 descendants; rejected payout at60 and98; paid at99 descendants via next-block100 maturity; one tx=%s; two4.95SLUG outputs; fee1000 from1%%; lost-ack restart and repeated retries broadcast once", winner.WorkHash, winner.BlockHeight, recovered.TxID)
}
