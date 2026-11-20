//go:build desktopintegration && !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/taytr-llc/slug/blockchain"
	"github.com/taytr-llc/slug/btcutil/v2"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/desktop/internal/recoveryfile"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectwork"
	"github.com/taytr-llc/slug/wire/v2"
	"github.com/taytr-llc/slugwallet/chain"
	"github.com/taytr-llc/slugwallet/recovery"
	"github.com/taytr-llc/slugwallet/waddrmgr"
	"github.com/taytr-llc/slugwallet/wallet"
)

const hostedActualSink = "aQfRR2m3DRfSpQhkuQECXXkSBs2JGf6zH6"
const hostedActualSinkScript = "76a91406afd46bcdfd22ef94ac122aa11f241244a37ecc88ac"
const hostedActualFundingAtoms int64 = 1000000000
const hostedActualSendAtoms int64 = 100000000

// Independent txauthor size arithmetic for one native84 input, a P2PKH
// recipient and native84 change: ceil(((10+41+34+31)*4+3+109)/4)=144.
const hostedActualFee int64 = 144

type hostedActualOracle struct {
	Identity                    poolRecoveryOracle
	ChangeAddress, ChangeScript string
	Manifest                    recovery.Manifest
}

func hostedActualOracleFor(t *testing.T, fixture string) hostedActualOracle {
	t.Helper()
	want := hostedActualOracle{Identity: poolBackupOracle(t, fixture)}
	raw, err := os.ReadFile("../../../slugwallet/recovery/testdata/recovery_v1.json")
	if err != nil || hex.EncodeToString(sumPoolBackup(raw)) != "d5141f4a8ee4378d77d483b09f6ecd809f8b328f82e79a69ff83164e73bc799a" {
		t.Fatal("independent recovery input changed")
	}
	defer clear(raw)
	var corpus struct {
		Cases []struct {
			Name      string `json:"name"`
			Addresses []struct {
				Path, Address string
				Script        string `json:"script_pub_key"`
			} `json:"addresses"`
		} `json:"cases"`
	}
	if json.Unmarshal(raw, &corpus) != nil {
		t.Fatal("public recovery input parse")
	}
	for _, f := range corpus.Cases {
		if f.Name != fixture {
			continue
		}
		for _, a := range f.Addresses {
			if a.Path == "m/84'/0'/0'/1/0" {
				want.ChangeAddress, want.ChangeScript = a.Address, a.Script
			}
		}
	}
	if want.ChangeAddress == "" {
		t.Fatal("independent native84 change input missing")
	}
	manifestRaw, err := os.ReadFile("../../../slugwallet/wallet/testdata/own-activity/inputs/public/" + fixture + ".manifest.json")
	manifestHash := map[string]string{"zero256": "f027cb1dfc522b59a6f863ad9921b499d42ea7102fa3d7a17a8153b650be9ed5", "incrementing256": "41776b5beb5b341714a285e62444fceb0cf2a9d408123f05ec1eeeff739db4e6"}[fixture]
	if err != nil || hex.EncodeToString(sumPoolBackup(manifestRaw)) != manifestHash {
		t.Fatal("preimplementation public 1/1 manifest changed")
	}
	want.Manifest, err = recovery.ParseManifest(manifestRaw)
	if err != nil {
		t.Fatal("preimplementation public manifest parse")
	}
	sink, err := os.ReadFile("../../../slugwallet/integration/testdata/addresses_v1.json")
	if err != nil || hex.EncodeToString(sumPoolBackup(sink)) != "f99f42e17cae250374277964621ac356e7d96d654e1ab74f5e986ae6c91b5bc8" || !bytes.Contains(sink, []byte(hostedActualSink)) {
		t.Fatal("independent external sink changed")
	}
	return want
}

func hostedActualID(label string) string {
	h := sha256.Sum256([]byte("SLUG Set71 actual harness public operation\x00" + label))
	return hex.EncodeToString(h[:])
}

func hostedActualWrite(t *testing.T, path string, value any) {
	t.Helper()
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.Write(append(raw, '\n'))
	err = errors.Join(err, f.Sync(), f.Close())
	if err != nil {
		t.Fatal(err)
	}
}

// These two explicit test coinbases use the actual published recipient script.
// They do not enter the pool/share/fighter journals or call the app's Start.
func hostedActualFund(t *testing.T, ctx context.Context, rpc *poolActualRPC, want poolRecoveryOracle) string {
	t.Helper()
	client, err := prospectnode.NewClient(prospectnode.RPCConfig{Version: 1, URL: "http://" + rpc.address, Username: rpc.user, Password: rpc.password})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	policy := pilot.HostedPoolPolicy()
	for attempt := uint64(0); attempt < 64; attempt++ {
		template, err := client.ResolveTemplate(ctx, policy)
		if err != nil {
			t.Fatal("funding template", err)
		}
		job, err := prospectjob.New(template.Context, "0279be667ef9dcbbac55a06295ce870b07029bfcdb2dce28d959f2815b16f81798", strings.Repeat("45", 32), time.Now().UnixMilli())
		if err != nil {
			t.Fatal(err)
		}
		candidate, err := prospectwork.Build(job, template, want.Address, attempt)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := hex.DecodeString(candidate.BlockHex)
		var block wire.MsgBlock
		if err != nil || block.Deserialize(bytes.NewReader(raw)) != nil || len(block.Transactions) != 1 {
			t.Fatal("fresh test funding block")
		}
		matches := 0
		for _, output := range block.Transactions[0].TxOut {
			if hex.EncodeToString(output.PkScript) == want.Script && output.Value == hostedActualFundingAtoms {
				matches++
			}
		}
		if matches != 1 {
			t.Fatal("coinbase differs from frozen native84 script/amount")
		}
		for nonce := uint32(0); nonce < 64; nonce++ {
			if ctx.Err() != nil {
				t.Fatal(ctx.Err())
			}
			block.Header.Nonce = nonce
			hash := block.Header.BlockHash()
			if blockchain.HashToBig(&hash).Cmp(blockchain.CompactToBig(block.Header.Bits)) > 0 {
				continue
			}
			var encoded bytes.Buffer
			if block.Serialize(&encoded) != nil {
				t.Fatal("encode funding block")
			}
			if err := client.SubmitBlock(ctx, template.Context, hex.EncodeToString(encoded.Bytes())); err != nil {
				t.Fatal("submit funding block", err)
			}
			var canonical string
			if err := rpc.call(ctx, "getblockhash", []any{template.Context.ParentHeight + 1}, &canonical); err != nil || canonical != hash.String() {
				t.Fatal("funding block not canonical", err)
			}
			return block.Transactions[0].TxHash().String()
		}
	}
	t.Fatal("bounded actual funding work exhausted")
	return ""
}

func hostedActualAction(t *testing.T, ctx context.Context, c *PoolController, r pilot.WithdrawalRequest) pilot.WithdrawalOperation {
	t.Helper()
	out := c.HandleWithdrawal(ctx, r)
	if !out.OK || out.Action == nil {
		t.Fatal("withdrawal dispatch", r.Command, out.Error)
	}
	query := pilot.WithdrawalRequest{ID: r.ID + 1, Version: 1, Command: "withdrawal_action", ViewID: r.ViewID, OperationID: r.OperationID, ActionID: r.ActionID}
	var operation pilot.WithdrawalOperation
	poolActualWait(t, ctx, r.Command, func(ctx context.Context) bool {
		got := c.HandleWithdrawal(ctx, query)
		if !got.OK || got.Action == nil {
			t.Fatal("withdrawal action lookup", got.Error)
		}
		a := got.Action
		if a.State == "running" {
			return false
		}
		if a.State != "done" || a.Operation == nil {
			t.Fatal("withdrawal action terminal", a.State, a.Error)
		}
		operation = *a.Operation
		return true
	})
	return operation
}

func hostedActualWithdraw(t *testing.T, ctx context.Context, c *PoolController, s pilot.PoolSnapshot, fixture string) pilot.WithdrawalOperation {
	t.Helper()
	r := pilot.WithdrawalRequest{ID: 1, Version: 1, Command: "withdrawal_prepare", ViewID: s.View.ViewID, OperationID: hostedActualID(fixture + "-withdrawal"), ActionID: hostedActualID(fixture + "-prepare"), Intent: &pilot.WithdrawalIntent{Destination: hostedActualSink, RecipientScript: hostedActualSinkScript, AmountAtoms: hostedActualSendAtoms, FeeRateAtomsPerKB: 1000, FeeLimitAtoms: 100000}}
	prepared := hostedActualAction(t, ctx, c, r)
	d := prepared.Draft
	if prepared.Stage != "prepared" || d == nil || d.InputCount != 1 || d.InputAtoms != hostedActualFundingAtoms || d.FeeAtoms != hostedActualFee || d.ChangeAtoms != hostedActualFundingAtoms-hostedActualSendAtoms-hostedActualFee || d.ChangeOutputIndex != 1 || d.RecipientOutputIndex != 0 {
		t.Fatal("reviewed withdrawal differs from independent amount/fee/order expectations")
	}
	review := pilot.WithdrawalRequest{ID: 3, Version: 1, Command: "withdrawal_review", ViewID: r.ViewID, OperationID: r.OperationID, ActionID: hostedActualID(fixture + "-send"), ReviewID: hostedActualID(fixture + "-review"), ReviewKind: "send", DraftDigest: d.DraftDigest}
	out := c.HandleWithdrawal(ctx, review)
	if !out.OK || out.Review == nil || out.Review.Draft != *d || out.Review.Kind != "send" || out.Review.TxID != "" {
		t.Fatal("original reviewed draft unavailable", out.Error)
	}
	// This explicit test approval follows the exact public review above. No
	// signature or node delivery is requested before the review was checked.
	review.Command, review.ReviewKind = "withdrawal_approve", ""
	result := hostedActualAction(t, ctx, c, review)
	if result.Stage != "submitted" || result.TxID != d.UnsignedTxID || result.Dispatch == nil || result.Dispatch.Attempt != 1 || result.Observation == nil || result.Observation.State != "mempool" {
		t.Fatal("exact approved payment was not observed once")
	}
	return result
}

func hostedActualActivity(t *testing.T, ctx context.Context, c *PoolController, s pilot.PoolSnapshot, fixture, txid string) pilot.WalletActivityDetail {
	t.Helper()
	id := hostedActualID(fixture + "-activity-" + txid)
	job, err := c.BeginWalletActivity(ctx, s.View.ViewID, id)
	if err != nil {
		t.Fatal("activity admission", err)
	}
	poolActualWait(t, ctx, "confirmed activity", func(ctx context.Context) bool {
		var err error
		job, err = c.WalletActivityJob(ctx, s.View.ViewID, id)
		if err != nil {
			t.Fatal("activity lookup", err)
		}
		if job.State == "failed" || job.State == "cancelled" {
			t.Fatal("activity failed", job.Error)
		}
		return job.State == "done"
	})
	detail, err := c.WalletActivityDetail(ctx, s.View.ViewID, id, txid)
	if err != nil || detail.Item.Confirmations == 0 || detail.Item.Block == nil || detail.Binding.GymID != s.GymID || detail.Capture.WalletTip != detail.Capture.NodeTip || detail.Capture.WalletHeight != detail.Capture.NodeHeight {
		t.Fatal("exact wallet activity unavailable", err)
	}
	return detail
}

func hostedActualExportVerify(t *testing.T, ctx context.Context, c *PoolController, root, path string, s pilot.PoolSnapshot, want hostedActualOracle) recovery.Manifest {
	t.Helper()
	selected, err := recoveryfile.Capture(path, []string{root}, recoveryfile.CreateMode)
	if err != nil {
		t.Fatal(err)
	}
	err = c.ExportPoolRecovery(ctx, s.View.ViewID, selected.Pin())
	err = errors.Join(err, selected.Close())
	if err != nil {
		t.Fatal("current hosted export", err)
	}
	stat, err := os.Stat(path)
	if err != nil || stat.Mode().Perm() != 0600 || stat.Size() > recovery.MaxExportSize {
		t.Fatal("private export file framing")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal("export read")
	}
	bundle, err := recovery.ParseExport(raw)
	clear(raw)
	if err != nil {
		t.Fatal("export parse")
	}
	bundle.Mnemonic = ""
	m := bundle.Manifest
	if !reflect.DeepEqual(m, want.Manifest) || m.RootPublicKey != want.Identity.Root || m.GymPublicKey != want.Identity.Key || m.GymID != want.Identity.Gym || len(m.Accounts) != 4 {
		t.Fatal("current export identity")
	}
	for _, a := range m.Accounts {
		external, internal := uint32(0), uint32(0)
		if a.Purpose == 84 {
			external, internal = 1, 1
		}
		if a.Coin != 0 || a.Account != 0 || a.NextExternal != external || a.NextInternal != internal {
			t.Fatal("current export failed to retain exact receive/change cursor")
		}
	}
	if c.Snapshot().BackupState != "idle" {
		t.Fatal("export falsely asserted verification")
	}
	read, err := recoveryfile.Capture(path, []string{root}, recoveryfile.ReadMode)
	if err != nil {
		t.Fatal(err)
	}
	err = c.VerifyPoolBackup(ctx, s.View.ViewID, read.Pin())
	err = errors.Join(err, read.Close())
	if err != nil {
		t.Fatal("production schema12 backup dispatch", err)
	}
	poolActualWait(t, ctx, "production file-only schema12 verification", func(context.Context) bool {
		s := c.Snapshot()
		if s.BackupState == "failed" || s.BackupState == "cancelled" {
			t.Fatal("production backup verification", s.BackupError)
		}
		return s.BackupState == "succeeded"
	})
	if s := c.Snapshot(); s.State != "stopped" || s.Attempts != 0 || s.AssignmentCount != 0 {
		t.Fatal("backup verification started mining")
	}
	return m
}

// The independent restorer receives a selected file, fresh disposable path,
// frozen public identities and the already-owned node only. It cannot receive
// or locate the installed database. Its single controlled send sweeps the
// restored change to the frozen sink, avoiding a newly funded change path that
// the still-installed wallet has never issued.
func hostedActualRestoreSpend(t *testing.T, ctx context.Context, path, fresh string, node *poolActualRPC, want hostedActualOracle, withdrawal pilot.WithdrawalOperation) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) > recovery.MaxExportSize {
		t.Fatal("selected backup unavailable")
	}
	bundle, err := recovery.ParseExport(raw)
	clear(raw)
	if err != nil || bundle.Manifest.RootPublicKey != want.Identity.Root || bundle.Manifest.GymID != want.Identity.Gym {
		t.Fatal("independent backup identity")
	}
	defer func() { bundle.Mnemonic = "" }()
	loader := wallet.NewLoader(&chaincfg.MainNetParams, fresh, true, 5*time.Second, 0, wallet.WithWalletSyncRetryInterval(50*time.Millisecond))
	session, err := loader.BeginPhaseARecovery(ctx, []byte(poolRestorePassword), []byte(poolRestorePassword), bundle)
	if err != nil {
		t.Fatal("independent schema12 begin", err)
	}
	defer func() {
		if err := session.Close(); err != nil {
			t.Error("schema12 owner close", err)
		}
	}()
	source, err := wallet.NewRecoveryRPCSource(wallet.RecoveryRPCConfig{URL: "ws://" + node.address + "/ws", Username: node.user, Password: node.password})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := source.Close(); err != nil {
			t.Error(err)
		}
	}()
	progress, err := session.Status(ctx)
	for err == nil && !progress.Ready {
		progress, err = session.Step(ctx, source)
	}
	if err != nil || progress.Phase != "complete" || progress.Target != 162 || progress.Derived != 162 || progress.Issued != 2 {
		t.Fatal("independent bounded restoration", err)
	}
	tip, err := source.BestBlock(ctx)
	if err != nil || tip.Height != progress.ScannedHeight || tip.Hash.String() != progress.ScannedHash {
		t.Fatal("independent restore checkpoint", err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	w, err := loader.OpenExistingWallet([]byte(poolRestorePassword), false)
	if err != nil {
		t.Fatal("open completed schema12", err)
	}
	var backend *chain.RPCClient
	defer func() {
		if err := loader.UnloadWallet(); err != nil {
			t.Error("restored wallet unload", err)
		}
		if backend != nil {
			backend.Stop()
			backend.WaitForShutdown()
		}
	}()
	backend, err = chain.NewRPCClient(&chaincfg.MainNetParams, node.address, node.user, node.password, nil, true, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err = backend.Start(ctx); err != nil {
		t.Fatal(err)
	}
	w.SynchronizeRPC(backend)
	poolActualWait(t, ctx, "independent live sync", func(context.Context) bool {
		return w.ChainSynced() && w.SyncedTo().Height == tip.Height && w.SyncedTo().Hash == tip.Hash
	})
	identity, err := w.GymIdentity(ctx)
	if err != nil || identity.GymID != want.Identity.Gym || identity.PublicKey != want.Identity.Key || w.Manager.RecoveryInventoryLimit() != 32768 {
		t.Fatal("restored schema12 identity/profile", err)
	}
	utxos, err := w.ListUnspent(1, 9999999, "")
	if err != nil || len(utxos) != 1 {
		t.Fatal("restored exact change coin missing", err)
	}
	u := utxos[0]
	amount, err := btcutil.NewAmount(u.Amount)
	if err != nil || int64(amount) != withdrawal.Draft.ChangeAtoms || u.Address != want.ChangeAddress || u.ScriptPubKey != want.ChangeScript || u.TxID != withdrawal.TxID || u.Vout != uint32(withdrawal.Draft.ChangeOutputIndex) {
		t.Fatal("restored coin differs from frozen internal0 and actual approved payment")
	}
	history, err := w.ListTransactions(0, 8)
	if err != nil {
		t.Fatal("restored public history")
	}
	found := false
	for _, row := range history {
		if row.TxID == withdrawal.TxID {
			found = true
		}
	}
	if !found {
		t.Fatal("restored wallet lost prior withdrawal history")
	}
	if err = w.Unlock([]byte(poolRestorePassword), nil); err != nil {
		t.Fatal(err)
	}
	sink, _ := hex.DecodeString(hostedActualSinkScript)
	paid := int64(amount) - hostedActualFee
	tx, err := w.SendOutputs([]*wire.TxOut{{Value: paid, PkScript: sink}}, &waddrmgr.KeyScopeBIP0084, 0, 1, 1000, wallet.CoinSelectionLargest, "public Set71 restore proof")
	if err != nil {
		t.Fatal("controlled restored spend", err)
	}
	previous, err := chainhash.NewHashFromStr(withdrawal.TxID)
	if err != nil || len(tx.TxIn) != 1 || tx.TxIn[0].PreviousOutPoint != (wire.OutPoint{Hash: *previous, Index: uint32(withdrawal.Draft.ChangeOutputIndex)}) || len(tx.TxIn[0].Witness) != 2 || len(tx.TxOut) != 1 || tx.TxOut[0].Value != paid || !bytes.Equal(tx.TxOut[0].PkScript, sink) {
		t.Fatal("restored spend did not consume exact recovered coin into exact sink/fee")
	}
	spend := tx.TxHash().String()
	poolActualConfirm(t, ctx, node, spend)
	poolActualWait(t, ctx, "restored sweep confirms", func(context.Context) bool { return w.ChainSynced() && w.SyncedTo().Height == tip.Height+1 })
	remaining, err := w.CalculateBalance(1)
	if err != nil || remaining != 0 {
		t.Fatal("restored exact sweep not reflected", err)
	}
	hostedActualWrite(t, filepath.Join(filepath.Dir(fresh), "restore-public.json"), struct {
		Version                            uint32                  `json:"version"`
		Progress                           wallet.RecoveryProgress `json:"schema12_progress"`
		RestoredAtoms, SentAtoms, FeeAtoms int64
		FundingTxID, SpendTxID             string
	}{1, progress, int64(amount), paid, hostedActualFee, withdrawal.TxID, spend})
	return spend
}

func hostedActualTree(t *testing.T, root string) map[string]string {
	t.Helper()
	result := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("owned fixture symlink")
		}
		if entry.IsDir() {
			return nil
		}
		if len(result) >= 32 {
			return errors.New("owned wallet file limit")
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(raw)
		clear(raw)
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		result[rel] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		t.Fatal("owned closed wallet inventory", err)
	}
	return result
}

func TestHostedCurrentRecoveryActual(t *testing.T) {
	artifacts, binary := os.Getenv("SLUG_HOSTED_ACTUAL_ROOT"), os.Getenv("SLUG_HOSTED_ACTUAL_NODE")
	if artifacts == "" || binary == "" {
		t.Skip("explicit owned fixture artifacts and normal node required")
	}
	if !filepath.IsAbs(artifacts) || !filepath.IsAbs(binary) {
		t.Fatal("absolute owned inputs required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	defer cancel()
	root := filepath.Join(artifacts, "hosted")
	if _, err := os.Lstat(root); !os.IsNotExist(err) {
		t.Fatal("actual root must be entirely new")
	}
	config := pilot.Config{DataDir: root, NodeBinary: binary}
	c, err := NewHostedFixturePool(ctx, config, pilot.PoolOptions{})
	if err != nil {
		t.Fatal("fresh hosted setup", err)
	}
	defer func() {
		if c != nil {
			if err := c.Close(); err != nil {
				t.Error("hosted owner close", err)
			}
		}
	}()
	init, ready := c.hosted.init, c.hosted.ready
	if !hostedReadyMatches(init, ready) || c.Snapshot().Mode != pilot.HostedPoolMode || c.Snapshot().View != nil {
		t.Fatal("hosted setup did not preserve exact selected mode/pins")
	}
	node := poolActualNode(t, root)
	defer node.client.CloseIdleConnections()
	wants := map[string]hostedActualOracle{}
	funding := map[string]string{}
	for _, f := range hostedFixtures {
		wants[f] = hostedActualOracleFor(t, f)
		funding[f] = hostedActualFund(t, ctx, node, wants[f].Identity)
	}
	t.Log("fresh hosted setup and both exact recipient coinbases complete; advancing only explicit maturity descendants")
	poolActualMine(t, ctx, node, 100)
	withdrawals := map[string]pilot.WithdrawalOperation{}
	manifests := map[string]recovery.Manifest{}
	accounts := map[string]string{}
	for _, fixture := range hostedFixtures {
		s := poolActualLogin(t, ctx, c, fixture)
		want := wants[fixture]
		c.mu.Lock()
		signedSession := c.session.Kind == "login"
		c.mu.Unlock()
		if s.Mode != pilot.HostedPoolMode || s.GymID != want.Identity.Gym || s.PayoutAddress != want.Identity.Address || s.SourceBindingID != ready.SourceBindingID || !signedSession {
			t.Fatal("signed fixture login changed source or identity")
		}
		accounts[fixture] = s.Account.ID
		poolActualWait(t, ctx, "mature installed fixture funds", func(context.Context) bool { return c.Snapshot().SpendableAtoms == hostedActualFundingAtoms })
		withdrawal := hostedActualWithdraw(t, ctx, c, s, fixture)
		withdrawals[fixture] = withdrawal
		poolActualConfirm(t, ctx, node, withdrawal.TxID)
		poolActualWait(t, ctx, "installed confirmed change", func(context.Context) bool { return c.Snapshot().SpendableAtoms == withdrawal.Draft.ChangeAtoms })
		detail := hostedActualActivity(t, ctx, c, s, fixture, withdrawal.TxID)
		if detail.Item.SpentAtoms != hostedActualFundingAtoms || detail.Item.ChangeAtoms != withdrawal.Draft.ChangeAtoms || detail.Item.FeeAtoms == nil || *detail.Item.FeeAtoms != hostedActualFee || len(detail.Outputs) != 2 || detail.Outputs[1].Address != want.ChangeAddress || !detail.Outputs[1].Owned || !detail.Outputs[1].Change {
			t.Fatal("installed change not bound to independent native84/internal0")
		}
		path := filepath.Join(artifacts, fixture+"-current-recovery.json")
		manifests[fixture] = hostedActualExportVerify(t, ctx, c, root, path, s, want)
		hostedActualWrite(t, filepath.Join(artifacts, fixture+"-public-manifest.json"), manifests[fixture])
		t.Logf("%s: signed login, mature receive, reviewed change, current export and production schema12 verification passed", fixture)
		poolActualLogout(t, ctx, c)
	}
	// Only a closed installed wallet is moved. The independent schema12 loader
	// below is given no original path, DB bytes or vault handle.
	original := filepath.Join(root, "shared", "gyms", accounts["zero256"], "wallet")
	retained := filepath.Join(artifacts, "closed-original-wallet")
	before := hostedActualTree(t, original)
	if err = os.Rename(original, retained); err != nil {
		t.Fatal(err)
	}
	returned := false
	defer func() {
		if !returned {
			if err := os.Rename(retained, original); err != nil {
				t.Error(err)
			}
		}
	}()
	if _, err := os.Lstat(original); !os.IsNotExist(err) {
		t.Fatal("original database remained accessible")
	}
	spend := hostedActualRestoreSpend(t, ctx, filepath.Join(artifacts, "zero256-current-recovery.json"), filepath.Join(artifacts, "independent-schema12"), node, wants["zero256"], withdrawals["zero256"])
	if !reflect.DeepEqual(before, hostedActualTree(t, retained)) {
		t.Fatal("independent restore touched installed database")
	}
	if err = os.Rename(retained, original); err != nil {
		t.Fatal(err)
	}
	returned = true
	t.Log("independent schema12 file restore confirmed a controlled spend; closed original database stayed byte-identical")
	s := poolActualLogin(t, ctx, c, "zero256")
	poolActualWait(t, ctx, "original observes controlled restored spend", func(context.Context) bool { return c.Snapshot().SpendableAtoms == 0 })
	c.mu.Lock()
	oldSession := c.session
	c.mu.Unlock()
	if err = c.ResetFixtureAccount(); err != nil {
		t.Fatal("verified reset dispatch", err)
	}
	poolActualWait(t, ctx, "held same-wallet reopen", func(context.Context) bool {
		now := c.Snapshot()
		if now.Account.State == "failed" {
			t.Fatal("reset failed", now.Account.LastError)
		}
		return now.Account.State == "signed_in" && now.Account.Held
	})
	held := c.Snapshot()
	if held.View == nil || held.View.ViewID == s.View.ViewID || held.Account.ID != s.Account.ID || held.Account.Generation != s.Account.Generation+1 || held.GymID != s.GymID || held.PayoutAddress != s.PayoutAddress || held.SourceBindingID != s.SourceBindingID || held.Account.CanExport {
		t.Fatal("reset replaced wallet/source or failed to revoke original view")
	}
	if err := c.store.WithSession(ctx, oldSession.Token, gymaccount.View, func(_ gymaccount.Account, check func() error) error { return check() }); !errors.Is(err, gymaccount.ErrUnauthorized) {
		t.Fatal("old original session survived verified reset", err)
	}
	poolActualHeldExport(t, ctx, c, root, artifacts)
	q := pilot.WithdrawalRequest{ID: 9, Version: 1, Command: "withdrawal_prepare", ViewID: held.View.ViewID, OperationID: hostedActualID("held-operation"), ActionID: hostedActualID("held-action"), Intent: &pilot.WithdrawalIntent{Destination: hostedActualSink, RecipientScript: hostedActualSinkScript, AmountAtoms: hostedActualSendAtoms, FeeRateAtomsPerKB: 1000, FeeLimitAtoms: 100000}}
	if out := c.HandleWithdrawal(ctx, q); out.OK || out.Error != "held" {
		t.Fatal("held view permitted withdrawal", out.Error)
	}
	q.ViewID = s.View.ViewID
	if out := c.HandleWithdrawal(ctx, q); out.OK || out.Error != "stale_view" {
		t.Fatal("old view retained mutation authority", out.Error)
	}
	poolActualLogout(t, ctx, c)
	if err = c.Close(); err != nil {
		t.Fatal(err)
	}
	c = nil
	if conn, err := net.DialTimeout("tcp", node.address, time.Second); err == nil {
		conn.Close()
		t.Fatal("owned node listener survived close")
	}
	c, err = NewHostedFixturePool(ctx, config, pilot.PoolOptions{})
	if err != nil {
		t.Fatal("exact hosted reopen", err)
	}
	if !reflect.DeepEqual(init, c.hosted.init) || !reflect.DeepEqual(ready, c.hosted.ready) {
		t.Fatal("hosted reopen repinned identity")
	}
	for _, fixture := range hostedFixtures {
		got := poolActualLogin(t, ctx, c, fixture)
		if got.Account.ID != accounts[fixture] || got.PayoutAddress != wants[fixture].Identity.Address || got.GymID != wants[fixture].Identity.Gym || got.SourceBindingID != ready.SourceBindingID || got.Account.Held != (fixture == "zero256") {
			t.Fatal("retained hosted identity/hold changed on restart")
		}
		originalOperation := withdrawals[fixture]
		out := c.HandleWithdrawal(ctx, pilot.WithdrawalRequest{ID: 10, Version: 1, Command: "withdrawal_operation", ViewID: got.View.ViewID, OperationID: originalOperation.OperationID})
		if !out.OK || out.Operation == nil || !reflect.DeepEqual(*out.Operation, originalOperation) {
			t.Fatal("reopen changed the original submitted withdrawal or dispatched it again", out.Error)
		}
		poolActualLogout(t, ctx, c)
	}
	if err = c.Close(); err != nil {
		t.Fatal(err)
	}
	c = nil
	// Missing established storage is a refusal, never a new initializer. Keep
	// the original owned store intact under a temporary sibling while testing.
	current := filepath.Join(root, "current-recovery")
	saved := filepath.Join(artifacts, "retained-current-store")
	if err = os.Rename(current, saved); err != nil {
		t.Fatal(err)
	}
	refused, openErr := NewHostedFixturePool(ctx, config, pilot.PoolOptions{})
	if refused != nil {
		_ = refused.Close()
	}
	_, missingErr := os.Lstat(current)
	restoreErr := os.Rename(saved, current)
	if openErr == nil || refused != nil || !os.IsNotExist(missingErr) || restoreErr != nil {
		t.Fatal("missing established store was recreated or adopted", restoreErr)
	}
	hostedActualWrite(t, filepath.Join(artifacts, "actual-public-result.json"), struct {
		Version                                                                               uint32                               `json:"version"`
		Mode                                                                                  string                               `json:"mode"`
		Init                                                                                  HostedInit                           `json:"init"`
		Ready                                                                                 HostedReady                          `json:"ready"`
		Funding                                                                               map[string]string                    `json:"funding_txids"`
		Withdrawals                                                                           map[string]pilot.WithdrawalOperation `json:"withdrawals"`
		Manifests                                                                             map[string]recovery.Manifest         `json:"current_manifests"`
		RestoredSpend                                                                         string                               `json:"restored_spend_txid"`
		FreshSetup, BothVerified, OriginalUnchanged, HeldSameWallet, Reopened, MissingRefused bool
	}{1, pilot.HostedPoolMode, init, ready, funding, withdrawals, manifests, spend, true, true, true, true, true, true})
}
