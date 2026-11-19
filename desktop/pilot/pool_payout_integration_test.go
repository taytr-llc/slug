//go:build desktopintegration && !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/btcjson"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/poolround"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slugwallet/recovery"
)

// This test is deliberately not a current PoolRuntime startup: that would
// provision additional league state into the old source. Its runner supplies
// only isolated copies of public journals/chain, never old wallet/account DBs.
// Expected payments and responses precede this implementation in the frozen
// independently produced corpus. The new wallet observer is queried only after
// an ordinary legacy gettransaction baseline on a fresh fixture restore.
func TestPoolPayoutRetainedActual(t *testing.T) {
	root, nodeBinary := os.Getenv("SLUG_PAYOUT_ACTUAL_ROOT"), os.Getenv("SLUG_PAYOUT_ACTUAL_NODE")
	if root == "" || nodeBinary == "" {
		t.Skip("explicit copied source and qualified normal node required")
	}
	if !filepath.IsAbs(root) || !filepath.IsAbs(nodeBinary) {
		t.Fatal("absolute owned inputs required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	var old struct {
		Config  poolsettle.Config  `json:"config"`
		Source  poolshare.Config   `json:"source"`
		Batches []poolsettle.Batch `json:"batches"`
	}
	payoutActualRead(t, "../../poolsettle/testdata/payout-history/inputs/original-state-public.json", &old)
	if len(old.Batches) != 1 || old.Batches[0].TxID != "eb3bd019318cb8ce5635794caae4b970fbb0480f905ea61e1033289a9ab3ed26" || len(old.Batches[0].Payments) != 2 {
		t.Fatal("frozen original batch unavailable")
	}
	var oracle struct {
		Core struct {
			Positives []struct {
				Name string `json:"name"`
				JSON string `json:"response_json"`
			} `json:"positives"`
		} `json:"core"`
	}
	payoutActualRead(t, "../../poolsettle/testdata/payout-history/reference-v1.json", &oracle)
	wantJSON := make(map[string]string)
	for _, tc := range oracle.Core.Positives {
		wantJSON[tc.Name] = tc.JSON
	}
	shared := filepath.Join(root, "shared")
	var marker struct {
		Limits poolround.Limits `json:"limits"`
		Source poolshare.Config `json:"source"`
	}
	payoutActualRead(t, filepath.Join(shared, "rounds", "SLUG-POOL-ROUNDS.json"), &marker)
	if marker.Source != old.Source {
		t.Fatal("original round source differs")
	}
	before := payoutActualJournalSnapshot(t, shared)
	accounts, err := gymaccount.Create(filepath.Join(root, "fresh-accounts"), gymaccount.Config{Version: 1, Principals: []gymaccount.FixturePrincipal{{Issuer: "fixture:local", Subject: "alice", Fixture: "zero256"}, {Issuer: "fixture:local", Subject: "bob", Fixture: "incrementing256"}}, MaxSessions: 16, MaxChallenges: 8, MaxEvents: 32})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := accounts.Close(); err != nil {
			t.Error(err)
		}
	}()
	sessions := recipientIntegrationSessions{accounts}
	var baselineTip string
	var baselineHeight uint32
	type sample struct {
		Epoch                 int                               `json:"epoch"`
		Fixture               string                            `json:"fixture"`
		Response              poolservice.PayoutHistoryResponse `json:"response"`
		BaselineConfirmations int64                             `json:"baseline_confirmations"`
		BaselineBlockHash     string                            `json:"baseline_block_hash"`
		Wallet                walletPayoutRead                  `json:"wallet"`
	}
	var samples []sample
	for epoch := 0; epoch < 2; epoch++ {
		func() {
			node := payoutActualNode(t, ctx, nodeBinary, root, epoch)
			defer node.close(t)
			var height uint32
			var tip string
			var peers []json.RawMessage
			node.rpc.must(t, "getblockcount", nil, &height)
			node.rpc.must(t, "getbestblockhash", nil, &tip)
			node.rpc.must(t, "getpeerinfo", nil, &peers)
			if len(peers) != 1 || height != 109 {
				t.Fatal("owned loopback pair is not at original height109")
			}
			if epoch == 0 {
				baselineHeight, baselineTip = height, tip
			} else if height != baselineHeight || tip != baselineTip {
				t.Fatal("copied chain changed on reopen")
			}
			rpcConfig := prospectnode.RPCConfig{Version: 1, URL: node.rpc.url, Username: "fixture", Password: integrationFixturePassword}
			nodeClient, err := prospectnode.NewClient(rpcConfig)
			if err != nil {
				t.Fatal(err)
			}
			defer nodeClient.Close()
			// The existing pool constructor requires a writable issuer capability.
			// This is an isolated copy; the test invokes no issuance/admission and
			// proves its entire logical key/value inventory remains unchanged.
			issuer, err := registry.OpenManifestAuthority(filepath.Join(shared, "issuer"), old.Source.AuthorityPin, false)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if e := issuer.Close(); e != nil {
					t.Error(e)
				}
			}()
			shares, err := poolshare.Open(filepath.Join(shared, "shares"), old.Source, issuer, nodeClient, sessions)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if e := shares.Close(); e != nil {
					t.Error(e)
				}
			}()
			rounds, err := poolround.Open(filepath.Join(shared, "rounds"), marker.Limits, shares, nodeClient)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if e := rounds.Close(); e != nil {
					t.Error(e)
				}
			}()
			settlement, err := poolsettle.Open(filepath.Join(shared, "settlement"), old.Config, rounds, payoutActualForbiddenBackend{})
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if e := settlement.Close(); e != nil {
					t.Error(e)
				}
			}()
			server, err := poolservice.NewServer(poolservice.ServerConfig{Version: 1, Listen: "127.0.0.1:0"}, shares, settlement, sessions)
			if err != nil {
				t.Fatal(err)
			}
			serveCtx, stop := context.WithCancel(ctx)
			ready := make(chan string, 1)
			done := make(chan error, 1)
			go func() { done <- server.Run(serveCtx, func(url string) { ready <- url }) }()
			defer func() {
				stop()
				select {
				case e := <-done:
					if e != nil {
						t.Error(e)
					}
				case <-time.After(10 * time.Second):
					t.Error("owned payout server did not join")
				}
			}()
			var url string
			select {
			case url = <-ready:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			for i, name := range []string{"zero256", "incrementing256"} {
				func() {
					walletPath := filepath.Join(root, "fresh-wallet-"+name)
					var wallet *walletRuntime
					if epoch == 0 {
						// Public fixture entropy and retained native84 index0 only. Set the
						// high-water before backend attachment, eliminating address/rescan races.
						var entropy [32]byte
						if name == "incrementing256" {
							for n := range entropy {
								entropy[n] = byte(n)
							}
						}
						accts := recovery.DefaultAccounts()
						accts[2].NextExternal = 1
						bundle, e := recovery.NewExport(entropy, accts)
						clear(entropy[:])
						if e != nil {
							t.Fatal(e)
						}
						wallet, err = restoreAccountFixtureWallet(walletPath, name, bundle)
						bundle.Mnemonic = ""
					} else {
						wallet, err = openAccountFixtureWallet(walletPath, name, false)
					}
					if err != nil {
						t.Fatal(err)
					}
					defer func() {
						if e := wallet.Close(); e != nil {
							t.Error(e)
						}
					}()
					if err = wallet.Attach(ctx, rpcConfig); err != nil {
						t.Fatal(err)
					}
					recipientIntegrationWaitWallet(t, ctx, wallet, height)
					identity := wallet.Identity()
					paymentIndex := -1
					for j, p := range old.Batches[0].Payments {
						if p.GymID == identity.GymID {
							paymentIndex = j
						}
					}
					if paymentIndex < 0 {
						t.Fatal("restored fixture does not own original destination")
					}
					payment := old.Batches[0].Payments[paymentIndex]
					if payment.Script != poolRecipientOracle(t, name).Script {
						t.Fatal("original recipient differs from independent derivation")
					}
					// Existing old RPC first; no new observer output is an expected value.
					var baseline btcjson.GetTransactionResult
					stampBefore := wallet.wallet.Manager.SyncedTo()
					if err = wallet.rpc.call(ctx, "gettransaction", []any{old.Batches[0].TxID}, &baseline); err != nil {
						t.Fatal("legacy baseline", err)
					}
					stampAfter := wallet.wallet.Manager.SyncedTo()
					if stampBefore.Hash != stampAfter.Hash || stampBefore.Height != stampAfter.Height || stampAfter.Hash.String() != tip || uint32(stampAfter.Height) != height || baseline.Hex != old.Batches[0].Raw || baseline.TxID != old.Batches[0].TxID || baseline.Confirmations <= 0 || baseline.BlockHash != "484db6919991b669066251626d502fb2b304559429e648da1db993c812221795" {
						t.Fatal("legacy actual transaction baseline differs from frozen payment")
					}
					payoutActualWrite(t, filepath.Join(root, fmt.Sprintf("legacy-baseline-epoch%d-%s.json", epoch, name)), struct {
						Transaction  btcjson.GetTransactionResult `json:"transaction"`
						BeforeHeight int32                        `json:"before_height"`
						BeforeTip    string                       `json:"before_tip"`
						AfterHeight  int32                        `json:"after_height"`
						AfterTip     string                       `json:"after_tip"`
					}{baseline, stampBefore.Height, stampBefore.Hash.String(), stampAfter.Height, stampAfter.Hash.String()})
					selection := walletPayoutSelection{TxID: old.Batches[0].TxID, OutputIndex: uint32(paymentIndex), Atoms: payment.Atoms, RecipientScript: payment.Script}
					early := time.Now().UnixMilli()
					observed, e := wallet.ObservePoolPayout(ctx, selection)
					late := time.Now().UnixMilli()
					if e != nil || observed.State != "confirmed" || observed.ObservedAtMS < early || observed.ObservedAtMS > late || observed.WalletHeight != height || observed.WalletTip != tip || int64(observed.Confirmations) != baseline.Confirmations || observed.BlockHash != baseline.BlockHash {
						t.Fatal("new observer differs from independent old RPC baseline", e)
					}
					login, e := accounts.Login(ctx, gymaccount.Principal{Issuer: "fixture:local", Subject: []string{"alice", "bob"}[i], AuthenticatedAtMS: time.Now().UnixMilli()})
					if e != nil {
						t.Fatal(e)
					}
					if epoch == 0 {
						challenge, e := accounts.BeginChallenge(ctx, login.Token, "enroll")
						if e != nil {
							t.Fatal(e)
						}
						auth, e := wallet.SignAccountChallenge(ctx, accounts.AuthorityID(), challenge)
						if e != nil {
							t.Fatal(e)
						}
						if _, _, e = accounts.CompleteChallenge(ctx, login.Token, challenge.ChallengeID, auth); e != nil {
							t.Fatal(e)
						}
					}
					mining, e := accounts.MintMining(ctx, login.Token)
					if e != nil {
						t.Fatal(e)
					}
					client, e := poolservice.NewClient(poolservice.ClientConfig{Version: 1, URL: url, Token: mining.Token, PublicKey: identity.GymPublicKey, Source: old.Source})
					if e != nil {
						t.Fatal(e)
					}
					defer client.Close()
					response, e := client.Payouts(ctx)
					if e != nil {
						t.Fatal("authenticated actual payout history", e)
					}
					encoded, e := json.Marshal(response)
					if e != nil || string(encoded) != wantJSON["actual_"+name] {
						t.Fatal("actual HTTP projection differs from frozen independent original-journal response", e)
					}
					if len(response.History.Items) != 1 || response.History.Items[0].OutputIndex != uint32(paymentIndex) || response.History.Items[0].Atoms != payment.Atoms {
						t.Fatal("actual original output position/amount changed")
					}
					samples = append(samples, sample{epoch, name, response, baseline.Confirmations, baseline.BlockHash, observed})
					t.Logf("epoch%d %s: original output%d, %d atoms, historical confirmed; current wallet confirms%d at height%d", epoch, name, paymentIndex, payment.Atoms, observed.Confirmations, height)
				}()
			}
			var endHeight uint32
			var endTip string
			node.rpc.must(t, "getblockcount", nil, &endHeight)
			node.rpc.must(t, "getbestblockhash", nil, &endTip)
			if endHeight != height || endTip != tip {
				t.Fatal("read-only acceptance changed chain")
			}
			node.peer.rpc.must(t, "getblockcount", nil, &endHeight)
			node.peer.rpc.must(t, "getbestblockhash", nil, &endTip)
			if endHeight != height || endTip != tip {
				t.Fatal("read-only acceptance changed peer chain")
			}
		}()
		if after := payoutActualJournalSnapshot(t, shared); !reflect.DeepEqual(before, after) {
			t.Fatal("history read or reopen mutated logical accounting records")
		}
	}
	payoutActualWrite(t, filepath.Join(root, "actual-public-result.json"), struct {
		Height   uint32            `json:"height"`
		Tip      string            `json:"tip"`
		Journals map[string]string `json:"journal_logical_sha256"`
		Samples  []sample          `json:"samples"`
	}{baselineHeight, baselineTip, before, samples})
}

func payoutActualRead(t *testing.T, path string, out any) {
	t.Helper()
	raw, e := os.ReadFile(path)
	if e != nil || json.Unmarshal(raw, out) != nil {
		t.Fatal("actual public input unavailable", path, e)
	}
}
func payoutActualWrite(t *testing.T, path string, value any) {
	t.Helper()
	raw, e := json.MarshalIndent(value, "", "  ")
	if e != nil {
		t.Fatal(e)
	}
	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		t.Fatal(e)
	}
	_, e = f.Write(append(raw, '\n'))
	e = errors.Join(e, f.Sync(), f.Close())
	if e != nil {
		t.Fatal(e)
	}
}

func payoutActualJournalSnapshot(t *testing.T, shared string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, name := range []string{"issuer", "shares", "rounds", "settlement"} {
		db, e := leveldb.OpenFile(filepath.Join(shared, name, "db"), &opt.Options{ReadOnly: true, ErrorIfMissing: true, Strict: opt.StrictAll})
		if e != nil {
			t.Fatal(e)
		}
		h := sha256.New()
		iter := db.NewIterator(nil, nil)
		var size [8]byte
		for iter.Next() {
			for _, value := range [][]byte{iter.Key(), iter.Value()} {
				binary.LittleEndian.PutUint64(size[:], uint64(len(value)))
				h.Write(size[:])
				h.Write(value)
			}
		}
		e = iter.Error()
		iter.Release()
		e = errors.Join(e, db.Close())
		if e != nil {
			t.Fatal(e)
		}
		out[name] = hex.EncodeToString(h.Sum(nil))
	}
	return out
}

func payoutActualNode(t *testing.T, ctx context.Context, binaryPath, root string, epoch int) *integrationNode {
	t.Helper()
	// Existing ownedLocalChain scan readiness requires an owned loopback peer.
	// Both nodes start from independent byte-identical copies of the old chain;
	// no chain work is fabricated and neither node has a mining address.
	peerAddress := recipientIntegrationPort(t)
	peer := payoutActualOneNode(t, ctx, binaryPath, root, epoch, "peer", "peer-chain", "127.0.0.1:1", peerAddress)
	node := payoutActualOneNode(t, ctx, binaryPath, root, epoch, "primary", "primary-chain", peerAddress, "")
	node.peer = peer
	deadline := time.NewTimer(20 * time.Second)
	defer deadline.Stop()
	for {
		var peers []btcjson.GetPeerInfoResult
		var peerPeers []btcjson.GetPeerInfoResult
		var left, right string
		var leftHeight, rightHeight uint32
		if node.rpc.call("getpeerinfo", nil, &peers) == nil && len(peers) == 1 && onlyPilotPeers(peers) && peer.rpc.call("getpeerinfo", nil, &peerPeers) == nil && len(peerPeers) == 1 && onlyPilotPeers(peerPeers) && node.rpc.call("getbestblockhash", nil, &left) == nil && peer.rpc.call("getbestblockhash", nil, &right) == nil && left == right && node.rpc.call("getblockcount", nil, &leftHeight) == nil && peer.rpc.call("getblockcount", nil, &rightHeight) == nil && leftHeight == 109 && rightHeight == 109 {
			return node
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-deadline.C:
			t.Fatal("owned copied pair did not connect at exact tip")
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func payoutActualOneNode(t *testing.T, ctx context.Context, binaryPath, root string, epoch int, name, chainDirectory, connect, listen string) *integrationNode {
	t.Helper()
	home := filepath.Join(root, fmt.Sprintf("%s-epoch%d", name, epoch))
	if e := os.Mkdir(home, 0700); e != nil {
		t.Fatal(e)
	}
	rpc := recipientIntegrationPort(t)
	conf := fmt.Sprintf("rpcuser=fixture\nrpcpass=%s\nnotls=1\nrpclisten=%s\nnodnsseed=1\nconnect=%s\nlocalwalletpilot=1\ntxindex=1\ndatadir=%s\nlogdir=%s\n", integrationFixturePassword, rpc, connect, filepath.Join(root, "shared", chainDirectory), filepath.Join(home, "logs"))
	if listen == "" {
		conf += "nolisten=1\n"
	} else {
		conf += "listen=" + listen + "\n"
	}
	config := filepath.Join(home, "fixture.conf")
	if e := os.WriteFile(config, []byte(conf), 0600); e != nil {
		t.Fatal(e)
	}
	log, e := os.OpenFile(filepath.Join(home, "node.log"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		t.Fatal(e)
	}
	n := &integrationNode{rpc: newIntegrationRPC("http://" + rpc), cmd: exec.Command(binaryPath, "--configfile="+config), done: make(chan error, 1)}
	n.rpc.client.Timeout = 5 * time.Second
	n.cmd.Stdout, n.cmd.Stderr = log, log
	if e = n.cmd.Start(); e != nil {
		log.Close()
		t.Fatal(e)
	}
	go func() { e := n.cmd.Wait(); _ = log.Close(); n.done <- e }()
	t.Cleanup(func() { n.close(t) })
	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	for {
		var magic uint32
		if n.rpc.call("getcurrentnet", nil, &magic) == nil {
			var genesis string
			n.rpc.must(t, "getblockhash", []any{0}, &genesis)
			if magic != uint32(chaincfg.MainNetParams.Net) || genesis != chaincfg.MainNetParams.GenesisHash.String() {
				t.Fatal("wrong copied-node identity")
			}
			return n
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-deadline.C:
			t.Fatal("copied node startup deadline")
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// Any payment/chain-funding backend call would violate this read-only test.
type payoutActualForbiddenBackend struct{}

func (payoutActualForbiddenBackend) Inspect(context.Context, []poolsettle.Coin) ([]poolsettle.Funding, error) {
	panic("history called funding inspection")
}
func (payoutActualForbiddenBackend) EstimateFee(context.Context, []poolsettle.Coin, []poolsettle.Payment, string) (int64, error) {
	panic("history estimated payment fee")
}
func (payoutActualForbiddenBackend) Sign(context.Context, []poolsettle.Coin, []byte) ([]byte, error) {
	panic("history signed payment")
}
func (payoutActualForbiddenBackend) ObserveTransaction(context.Context, string) (poolsettle.TransactionObservation, error) {
	panic("history observed settlement transaction")
}
func (payoutActualForbiddenBackend) Broadcast(context.Context, []byte) (string, error) {
	panic("history broadcast payment")
}
