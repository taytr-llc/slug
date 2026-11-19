//go:build desktopintegration && !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/taytr-llc/slug/btcjson"
	"github.com/taytr-llc/slug/btcutil/v2"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slugwallet/recovery"
	"github.com/taytr-llc/slugwallet/wallet"
)

var phaseAActualTip = localTip{Hash: "765630b06f7fdd63e92f0d5787c956198ad887d56431f0c19c19b87f416a533e", Height: 110}

type phaseAActualFixture struct {
	name, manifestSHA, exportSHA, ownTx, otherTx string
	balance, change                              int64
}

// Original Set68 public evidence plus the preimplementation OLD-API intake.
// These values were frozen before the new scanner, never harvested from it.
var phaseAActualFixtures = []phaseAActualFixture{
	{"zero256", "f027cb1dfc522b59a6f863ad9921b499d42ea7102fa3d7a17a8153b650be9ed5", "ac78a612c592b01a4e8bc75d6212087c5b1fbf6e0c3dcd1722f237e6231a22a8", "ef498d2473c8106ec0618459201c41d65e0d097f58eda5934edf0e8acce9bbe9", "758ea94d1a81356178b236abfbb74124e8d70b5e937418a3678d7a5d0e3fefa9", 2969999859, 2869999859},
	{"incrementing256", "41776b5beb5b341714a285e62444fceb0cf2a9d408123f05ec1eeeff739db4e6", "665b62956001953fcfaa0ae77a6163b4201b925a2500423836eecea2b3c46691", "758ea94d1a81356178b236abfbb74124e8d70b5e937418a3678d7a5d0e3fefa9", "ef498d2473c8106ec0618459201c41d65e0d097f58eda5934edf0e8acce9bbe9", 4949999859, 4849999859},
}

func phaseAActualFile(t *testing.T, path, hash string) []byte {
	t.Helper()
	st, err := os.Lstat(path)
	if err != nil || !st.Mode().IsRegular() || st.Mode().Perm() != 0600 || st.Size() > 65536 {
		t.Fatal("invalid owned fixture input")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("fixture input unavailable")
	}
	digest := sha256.Sum256(raw)
	if hex.EncodeToString(digest[:]) != hash {
		clear(raw)
		t.Fatal("frozen fixture input differs")
	}
	return raw
}

func phaseAActualNodes(t *testing.T, ctx context.Context, binary, root string, epoch int) *integrationNode {
	t.Helper()
	peerAddress := recipientIntegrationPort(t)
	peer := payoutActualOneNode(t, ctx, binary, root, epoch, "peer", "peer-chain", "127.0.0.1:1", peerAddress)
	node := payoutActualOneNode(t, ctx, binary, root, epoch, "primary", "primary-chain", peerAddress, "")
	node.peer = peer
	deadline := time.NewTimer(25 * time.Second)
	defer deadline.Stop()
	for {
		var left, right string
		var lh, rh int32
		var lp, rp []btcjson.GetPeerInfoResult
		if node.rpc.call("getbestblockhash", nil, &left) == nil && peer.rpc.call("getbestblockhash", nil, &right) == nil && node.rpc.call("getblockcount", nil, &lh) == nil && peer.rpc.call("getblockcount", nil, &rh) == nil && node.rpc.call("getpeerinfo", nil, &lp) == nil && peer.rpc.call("getpeerinfo", nil, &rp) == nil && left == phaseAActualTip.Hash && right == left && lh == 110 && rh == 110 && len(lp) == 1 && len(rp) == 1 && onlyPilotPeers(lp) && onlyPilotPeers(rp) {
			return node
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-deadline.C:
			t.Fatal("owned copied nodes did not reach original exact tip")
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func phaseAActualWaitWallet(t *testing.T, ctx context.Context, w *walletRuntime) {
	t.Helper()
	deadline := time.NewTimer(45 * time.Second)
	defer deadline.Stop()
	for {
		view, e := w.Snapshot(ctx)
		tip, tipErr := w.poolBackupSyncedTip(ctx)
		if e == nil && tipErr == nil && view.Synced && view.Height == 110 && tip == phaseAActualTip {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-deadline.C:
			t.Fatal("fixture wallet did not synchronize exact hash and height")
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func phaseAActualState(t *testing.T, ctx context.Context, w *walletRuntime, f phaseAActualFixture) walletVerificationState {
	t.Helper()
	state, err := w.VerificationState(ctx)
	if err != nil || !state.View.Synced || state.View.Height != 110 || state.View.Spendable != f.balance || state.View.Immature != 0 || state.View.Unconfirmed != 0 || len(state.UTXOs) != 2 {
		t.Fatal("actual wallet balance or inventory differs from preimplementation source", err)
	}
	want := map[string]int64{f.ownTx: f.change, f.otherTx: 100000000}
	for _, output := range state.UTXOs {
		amount, e := btcutil.NewAmount(output.Amount)
		expected, ok := want[output.TxID]
		if e != nil || !ok || int64(amount) != expected || output.Confirmations != 1 || output.TxID == f.ownTx && output.Vout != 1 || output.TxID == f.otherTx && output.Vout != 0 {
			t.Fatal("actual change or reciprocal payment differs")
		}
		delete(want, output.TxID)
	}
	seen := map[string]bool{}
	for _, tx := range state.Transactions {
		if tx.Confirmations <= 0 {
			t.Fatal("unconfirmed activity cannot be omitted from verification")
		}
		seen[tx.TxID] = true
	}
	if len(want) != 0 || !seen[f.ownTx] || !seen[f.otherTx] || !seen["eb3bd019318cb8ce5635794caae4b970fbb0480f905ea61e1033289a9ab3ed26"] {
		t.Fatal("original payout and reciprocal transaction history incomplete")
	}
	return state
}

func phaseAActualResume(t *testing.T, ctx context.Context, root string, bundle recovery.Export, rpc prospectnode.RPCConfig) {
	t.Helper()
	dir := filepath.Join(root, "pending-close-open")
	loader := wallet.NewLoader(&chaincfg.MainNetParams, dir, true, 5*time.Second, 0)
	session, err := loader.BeginPhaseARecovery(ctx, []byte(fixtureWalletPublicPass), []byte(fixtureWalletPrivatePass), bundle)
	if err != nil {
		t.Fatal("pending session creation failed", err)
	}
	t.Cleanup(func() {
		if e := session.Close(); e != nil {
			t.Error(e)
		}
	})
	config, err := poolBackupRecoveryRPC(rpc)
	if err != nil {
		t.Fatal(err)
	}
	source, err := wallet.NewRecoveryRPCSource(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if e := source.Close(); e != nil {
			t.Error(e)
		}
	})
	progress, err := session.Step(ctx, source)
	if err != nil || progress.Derived != 128 || progress.Target != 162 || progress.Issued != 2 || progress.Ready {
		t.Fatal("first greedy derivation chunk differs from fixed 1/1 cursor plan", progress, err)
	}
	if err = session.Close(); err != nil {
		t.Fatal(err)
	}
	if err = source.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := loader.OpenPhaseARecovery(ctx, []byte(fixtureWalletPublicPass), bundle.Manifest)
	if err != nil {
		t.Fatal("exact pending session reopen failed", err)
	}
	got, statusErr := reopened.Status(ctx)
	closeErr := reopened.Close()
	if statusErr != nil || closeErr != nil || got != progress {
		t.Fatal("pending progress changed across Close/Open", statusErr, closeErr)
	}
	payoutActualWrite(t, filepath.Join(root, "pending-resume-public.json"), progress)
	if err = os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
}

// Uses current production bounded helper with recovery files exported by an
// independently retained OLD binary. No application installation or send occurs.
func TestPhaseABackupRetainedActual(t *testing.T) {
	root, binary := os.Getenv("SLUG_PHASE_A_ACTUAL_ROOT"), os.Getenv("SLUG_PHASE_A_ACTUAL_NODE")
	if root == "" || binary == "" {
		t.Skip("explicit owned fixture inputs required")
	}
	if !filepath.IsAbs(root) || !filepath.IsAbs(binary) {
		t.Fatal("absolute owned paths required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	node := phaseAActualNodes(t, ctx, binary, root, 0)
	defer node.close(t)
	rpc := prospectnode.RPCConfig{Version: 1, URL: node.rpc.url, Username: "fixture", Password: integrationFixturePassword}
	baselines := make(map[string]walletVerificationState)
	for index, f := range phaseAActualFixtures {
		func() {
			raw := phaseAActualFile(t, filepath.Join(root, "inputs", f.name+".recovery.json"), f.exportSHA)
			defer clear(raw)
			bundle, err := recovery.ParseExport(raw)
			clear(raw)
			if err != nil {
				t.Fatal("old fixture export parse failed")
			}
			defer func() { bundle.Mnemonic = "" }()
			manifest := phaseAActualFile(t, filepath.Join(root, "inputs", f.name+".manifest.json"), f.manifestSHA)
			encoded, err := recovery.MarshalManifest(bundle.Manifest)
			if err != nil || string(encoded) != string(manifest) {
				t.Fatal("old export and retained manifest differ")
			}
			original, err := openAccountFixtureWallet(filepath.Join(root, "wallets", f.name), f.name, false)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if e := original.Close(); e != nil {
					t.Error(e)
				}
			}()
			if err = original.Attach(ctx, rpc); err != nil {
				t.Fatal(err)
			}
			phaseAActualWaitWallet(t, ctx, original)
			baseline := phaseAActualState(t, ctx, original, f)
			if !reflect.DeepEqual(baseline.Accounts, bundle.Manifest.Accounts) {
				t.Fatal("old exported cursors differ from original copy")
			}
			baselines[f.name] = baseline
			// Publish this full original state BEFORE invoking the new scanner.
			payoutActualWrite(t, filepath.Join(root, "baseline-"+f.name+".json"), baseline)
			if index == 0 {
				phaseAActualResume(t, ctx, root, bundle, rpc)
			}
			scratch := filepath.Join(root, "scratch-"+f.name)
			if err = os.Mkdir(scratch, 0700); err != nil {
				t.Fatal(err)
			}
			owner := &poolBackupRestore{}
			defer func() {
				if e := owner.Close(); e != nil {
					t.Error(e)
					return
				}
				if e := os.RemoveAll(scratch); e != nil {
					t.Error(e)
				}
			}()
			if err = owner.Run(ctx, filepath.Join(scratch, "wallet"), f.name, bundle, rpc); err != nil {
				t.Fatal("bounded production restore failed", err)
			}
			restored := owner.runtime
			if restored == nil || restored.Identity() != original.Identity() {
				t.Fatal("restored public identity differs")
			}
			if err = restored.Attach(ctx, rpc); err != nil {
				t.Fatal(err)
			}
			phaseAActualWaitWallet(t, ctx, restored)
			recipient, err := expectedPoolRecipient(original.Identity())
			if err != nil {
				t.Fatal(err)
			}
			if _, err = restored.EnsurePoolRecipient(ctx, &recipient); err != nil {
				t.Fatal("restored original recipient unavailable", err)
			}
			actual := phaseAActualState(t, ctx, restored, f)
			initial, proof, err := backupSigningProof(phaseAActualTip, original.Identity())
			if err != nil {
				t.Fatal(err)
			}
			signer, err := restored.Signer(ctx, policy())
			if err != nil {
				t.Fatal(err)
			}
			auth, err := signer(ctx, initial, proof)
			if err != nil || gymauth.VerifyAuthorization(initial, proof, auth) != nil {
				t.Fatal("restored gym proof failed")
			}
			resampled := phaseAActualState(t, ctx, original, f)
			before, e1 := original.poolBackupSyncedTip(ctx)
			after, e2 := restored.poolBackupSyncedTip(ctx)
			if e1 != nil || e2 != nil || before != phaseAActualTip || after != before || comparePoolBackupState(baseline, actual, resampled, before, after) != "" {
				t.Fatal("full state or exact tip comparison failed", e1, e2)
			}
			payoutActualWrite(t, filepath.Join(root, "restored-"+f.name+".json"), actual)
			if err = owner.Close(); err != nil {
				t.Fatal(err)
			}
			if err = original.Close(); err != nil {
				t.Fatal(err)
			}
		}()
	}
	node.close(t)
	node = phaseAActualNodes(t, ctx, binary, root, 1)
	defer node.close(t)
	rpc.URL = node.rpc.url
	for _, f := range phaseAActualFixtures {
		w, err := openAccountFixtureWallet(filepath.Join(root, "wallets", f.name), f.name, false)
		if err != nil {
			t.Fatal(err)
		}
		func() {
			defer func() {
				if e := w.Close(); e != nil {
					t.Error(e)
				}
			}()
			if err = w.Attach(ctx, rpc); err != nil {
				t.Fatal(err)
			}
			phaseAActualWaitWallet(t, ctx, w)
			state := phaseAActualState(t, ctx, w, f)
			if !reflect.DeepEqual(state, baselines[f.name]) {
				t.Fatal("original wallet changed across actual verification/reopen")
			}
			payoutActualWrite(t, filepath.Join(root, "reopened-"+f.name+".json"), state)
		}()
	}
	node.close(t)
	payoutActualWrite(t, filepath.Join(root, "actual-public-result.json"), struct {
		Tip        localTip `json:"tip"`
		Fixtures   []string `json:"fixtures"`
		NoMining   bool     `json:"no_mining"`
		NoDelivery bool     `json:"no_delivery"`
		Scope      string   `json:"scope"`
	}{phaseAActualTip, []string{"zero256", "incrementing256"}, true, true, "Preimplementation current files through bounded production helper; full state, exact tip, key proof and original reopen. No installation or native-visible-dialog claim."})
}
