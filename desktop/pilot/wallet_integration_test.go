//go:build desktopintegration && !slugtestpow

// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package pilot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/taytr-llc/slug/btcutil/v2"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/wire/v2"
	"github.com/taytr-llc/slugwallet/recovery"
)

func integrationWaitPilot(t *testing.T, controller *Controller, duration time.Duration, ready func(Snapshot) bool) Snapshot {
	t.Helper()
	deadline := time.Now().Add(duration)
	var reportedHeight uint64
	for {
		value := controller.Snapshot()
		if value.State == "failed" {
			t.Fatalf("fixture pilot failed: %s", value.LastError)
		}
		if ready(value) {
			return value
		}
		if value.Height/10 > reportedHeight/10 {
			reportedHeight = value.Height
			t.Logf("normal pilot height%d wallet%d spendable%d immature%d receipts%d", value.Height, value.WalletHeight, value.SpendableAtoms, value.ImmatureAtoms, value.ProspectCount)
		}
		if time.Now().After(deadline) {
			t.Fatalf("fixture pilot did not reach expected state: %+v", value)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func integrationHistoryIDs(history []map[string]any) []string {
	unique := make(map[string]bool)
	for _, transaction := range history {
		if id, ok := transaction["txid"].(string); ok {
			unique[id] = true
		}
	}
	ids := make([]string, 0, len(unique))
	for id := range unique {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// Fresh initialization commits a wallet identity before it may enable mining.
// An immediate ordinary Quit must drain that transaction, not leave an
// unreopenable partial fixture or launch a first hash after cancellation.
func TestWalletPilotEarlyCloseNormalPoW(t *testing.T) {
	binary := os.Getenv("SLUG_DESKTOP_NODE")
	if binary == "" {
		t.Skip("set SLUG_DESKTOP_NODE to an explicitly built normal node")
	}
	if !filepath.IsAbs(binary) {
		t.Fatal("explicit absolute node required")
	}
	fixture := integrationOracle(t)
	payout, _ := fixture.address(t, "m/44'/0'/0'/0/0")
	config := Config{DataDir: filepath.Join(t.TempDir(), "early-quit-wallet-pilot"), NodeBinary: binary}
	controller, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := controller.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := controller.Start(); err != nil {
		t.Fatal(err)
	}
	controller.Stop()
	started := time.Now()
	if err := controller.Close(); err != nil {
		t.Fatal("immediate initial Quit did not drain initialization:", err)
	}
	if time.Since(started) > 45*time.Second {
		t.Fatal("initial Quit exceeded the bounded initialization drain")
	}
	reopened, err := New(config)
	if err != nil {
		t.Fatal("initial Quit left an unreopenable fixture:", err)
	}
	defer func() {
		if err := reopened.Close(); err != nil {
			t.Error(err)
		}
	}()
	value := reopened.Snapshot()
	if reopened.manifest.Phase != "ready" || value.State != "stopped" || value.Attempts != 0 || value.Height != 0 ||
		value.PayoutAddress != payout || value.GymID != fixture.Export.Manifest.GymID || value.GymPublicKey != fixture.Export.Manifest.GymPublicKey {
		t.Fatal("initial Quit changed identity, lost durable readiness or allowed mining")
	}
	t.Log("immediate Stop/Quit drained actual wallet/node initialization; reopened identical ready fixture without a hash")
}

// This complete gate deliberately uses the published fixture, not real wallet
// material. Expected public identities come from the standalone Python oracle;
// production headers and transactions are validated by actual isolated nodes.
func TestWalletPilotRecoveryNormalPoW(t *testing.T) {
	binary := os.Getenv("SLUG_DESKTOP_NODE")
	if binary == "" {
		t.Skip("set SLUG_DESKTOP_NODE to an explicitly built normal node")
	}
	if !filepath.IsAbs(binary) || wire.TestPoW {
		t.Fatal("explicit absolute normal-PoW node required")
	}
	rawBinary, err := os.ReadFile(binary)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("normal node SHA256 %x", sha256.Sum256(rawBinary))
	fixture := integrationOracle(t)
	payout, payoutScript := fixture.address(t, "m/44'/0'/0'/0/0")
	highAddress, _ := fixture.address(t, "m/44'/0'/0'/0/25")
	config := Config{DataDir: filepath.Join(t.TempDir(), "LocalWalletPilot"), NodeBinary: binary}
	controller, err := New(config)
	if err != nil {
		t.Fatal("create public-fixture pilot:", err)
	}
	t.Cleanup(func() {
		if err := controller.Close(); err != nil {
			t.Error("pilot close:", err)
		}
	})
	if initial := controller.Snapshot(); initial.State != "stopped" || initial.Attempts != 0 {
		t.Fatal("opening fixture pilot started mining")
	}
	if err := controller.Start(); err != nil {
		t.Fatal(err)
	}
	mined := integrationWaitPilot(t, controller, 12*time.Minute, func(value Snapshot) bool {
		return value.Height >= 100 && value.WalletHeight >= 100 && value.SpendableAtoms >= 10e8 && value.ProspectCount > 0
	})
	controller.Stop()
	stopped := integrationWaitPilot(t, controller, time.Minute, func(value Snapshot) bool { return value.State == "stopped" })
	if stopped.PayoutAddress != payout || stopped.GymID != fixture.Export.Manifest.GymID || stopped.GymPublicKey != fixture.Export.Manifest.GymPublicKey {
		t.Fatal("pilot payout or gym differs from independent recovery oracle")
	}
	if stopped.Height < mined.Height || stopped.Attempts < mined.Attempts || stopped.ImmatureAtoms <= 0 || stopped.SpendableAtoms < 10e8 {
		t.Fatal("normal mining reward/maturity disappeared at Stop")
	}
	if len(stopped.Prospects) == 0 {
		t.Fatal("normal mining produced no public receipts")
	}
	receipt, err := controller.Receipt(stopped.Prospects[0].Sequence)
	if err != nil {
		t.Fatal("public receipt:", err)
	}
	if receipt.Authorization == nil || receipt.Authorization.PublicKey != fixture.Export.Manifest.GymPublicKey || receipt.Verified.GymID != fixture.Export.Manifest.GymID ||
		gymauth.VerifyAuthorization(receipt.Context, receipt.Proof, *receipt.Authorization) != nil {
		t.Fatal("mined evidence was not authorized by the wallet gym")
	}
	verified, err := prospect.Verify(receipt.Context, receipt.Proof)
	if err != nil || !reflect.DeepEqual(verified, receipt.Verified) {
		t.Fatal("actual normal work receipt failed independent re-verification:", err)
	}
	coinbaseBytes, err := hex.DecodeString(receipt.Proof.Coinbase)
	var coinbase wire.MsgTx
	if err != nil || coinbase.Deserialize(bytes.NewReader(coinbaseBytes)) != nil {
		t.Fatal("invalid public work coinbase")
	}
	paidWallet := false
	for _, output := range coinbase.TxOut {
		if hex.EncodeToString(output.PkScript) == payoutScript && output.Value >= 10e8 {
			paidWallet = true
		}
	}
	if !paidWallet {
		t.Fatal("normal worker did not pay the independently derived wallet address")
	}
	if err := controller.Close(); err != nil {
		t.Fatal(err)
	}
	t.Logf("actual wallet-backed worker matured rewards at height%d with %d attempts and %d receipts", stopped.Height, stopped.Attempts, stopped.ProspectCount)

	// Independently reopen the original fixture through the actual restricted
	// wallet HTTP adapter. Advance its public cursor beyond gap20, so a later
	// export must read current retained metadata instead of a creation-time copy.
	node := integrationStartRetainedNode(t, binary, filepath.Join(config.DataDir, "primary-chain"), payout)
	var height int32
	node.rpc.must(t, "getblockcount", nil, &height)
	original, err := openFixtureWallet(filepath.Join(config.DataDir, "wallet"), false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := original.Close(); err != nil {
			t.Error(err)
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := original.Attach(ctx, prospectnode.RPCConfig{Version: 1, URL: node.rpc.url, Username: "fixture", Password: integrationFixturePassword}); err != nil {
		t.Fatal(err)
	}
	for {
		view, err := original.Snapshot(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if view.Synced && view.Height == height {
			break
		}
		if ctx.Err() != nil {
			t.Fatal("original wallet resynchronization timed out")
		}
		time.Sleep(50 * time.Millisecond)
	}
	var next string
	for index := 1; index <= 25; index++ {
		if err := original.rpc.call(ctx, "getnewaddress", []any{"default", "legacy"}, &next); err != nil {
			t.Fatal(err)
		}
	}
	if next != highAddress {
		t.Fatal("actual wallet RPC cursor differs from independent index25 address")
	}
	var beforeBalance float64
	var beforeUTXOs []integrationUTXO
	var beforeHistory []map[string]any
	for _, request := range []struct {
		method string
		params []any
		result any
	}{
		{"getbalance", []any{"*", 1}, &beforeBalance},
		{"listunspent", []any{0, 9999999}, &beforeUTXOs},
		{"listtransactions", []any{"*", 1000, 0}, &beforeHistory},
	} {
		if err := original.rpc.call(ctx, request.method, request.params, request.result); err != nil {
			t.Fatal(err)
		}
	}
	counts := original.rpc.callCounts()
	for _, method := range []string{"getnewaddress", "getbalance", "listunspent", "listtransactions"} {
		if counts[method] == 0 {
			t.Fatal("ordinary wallet operation bypassed actual HTTP RPC:", method)
		}
	}
	amount, err := btcutil.NewAmount(beforeBalance)
	if err != nil || int64(amount) != stopped.SpendableAtoms || len(beforeUTXOs) == 0 || len(beforeHistory) == 0 {
		t.Fatal("reopened actual wallet RPC lost matured funds or history")
	}
	if err := original.Close(); err != nil {
		t.Fatal(err)
	}
	node.close(t)

	controller, err = New(config)
	if err != nil {
		t.Fatal("reopen durable wallet pilot:", err)
	}
	reopened := controller.Snapshot()
	if reopened.State != "stopped" || reopened.Attempts != stopped.Attempts || reopened.PayoutAddress != payout ||
		reopened.GymID != stopped.GymID || reopened.ProspectCount != stopped.ProspectCount {
		t.Fatal("reopening changed wallet, payout, mining state or evidence")
	}
	backup := filepath.Join(t.TempDir(), "PUBLIC-FIXTURE-recovery.json")
	if err := controller.ExportRecovery(backup); err != nil {
		t.Fatal("export reopened retained recovery:", err)
	}
	raw, err := os.ReadFile(backup)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := recovery.ParseExport(raw)
	if err != nil || bundle.Mnemonic != fixture.Export.Mnemonic || bundle.Manifest.RootPublicKey != fixture.Export.Manifest.RootPublicKey ||
		bundle.Manifest.GymID != fixture.Export.Manifest.GymID || bundle.Manifest.GymPublicKey != fixture.Export.Manifest.GymPublicKey {
		t.Fatal("native-file export differs from independently derived fixture identity")
	}
	if bundle.Manifest.Accounts[0].Purpose != 44 || bundle.Manifest.Accounts[0].NextExternal != 26 {
		t.Fatal("later retained export lost beyond-gap public RPC cursor")
	}
	if info, err := os.Stat(backup); err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("recovery export did not use owner-only permissions")
	}
	if err := controller.VerifyBackup(backup); err != nil {
		t.Fatal("start asynchronous backup verification:", err)
	}
	checked := integrationWaitPilot(t, controller, 3*time.Minute, func(value Snapshot) bool {
		return value.BackupState != "running" && value.BackupState != "idle"
	})
	if checked.BackupState != "succeeded" || checked.Attempts != stopped.Attempts || checked.State != "stopped" {
		t.Fatalf("backup verification failed or started mining: %s %s", checked.BackupState, checked.BackupError)
	}
	public, err := json.Marshal(checked)
	if err != nil || bytes.Contains(public, []byte(bundle.Mnemonic)) || bytes.Contains(public, []byte(integrationFixturePassword)) {
		t.Fatal("snapshot exposed recovery material")
	}
	if err := controller.Close(); err != nil {
		t.Fatal(err)
	}

	// The original database is now inaccessible at the managed path. Only the
	// exported file and public chain enter the independent restored wallet.
	walletDirectory := filepath.Join(config.DataDir, "wallet")
	if err := os.Rename(walletDirectory, filepath.Join(t.TempDir(), "original-wallet-not-a-restore-input")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(walletDirectory); !os.IsNotExist(err) {
		t.Fatal("original wallet remains accessible at its managed path")
	}
	node = integrationStartRetainedNode(t, binary, filepath.Join(config.DataDir, "primary-chain"), payout)
	restored := integrationRestoreFile(t, backup, node)
	integrationWaitSync(t, restored, height)
	var restoredBalance float64
	var restoredUTXOs []integrationUTXO
	var restoredHistory []map[string]any
	restored.rpc.must(t, "getbalance", []any{"*", 1}, &restoredBalance)
	restored.rpc.must(t, "listunspent", []any{0, 9999999}, &restoredUTXOs)
	restored.rpc.must(t, "listtransactions", []any{"*", 1000, 0}, &restoredHistory)
	if restoredBalance != beforeBalance || !reflect.DeepEqual(integrationNormalizeUTXOs(t, restoredUTXOs), integrationNormalizeUTXOs(t, beforeUTXOs)) ||
		!reflect.DeepEqual(integrationHistoryIDs(restoredHistory), integrationHistoryIDs(beforeHistory)) {
		t.Fatal("file-only restore changed exact matured balance, UTXOs or transaction history")
	}
	restoredFunds, err := restored.wallet.CalculateAccountBalances(0, 1)
	if err != nil || int64(restoredFunds.ImmatureReward) != stopped.ImmatureAtoms {
		t.Fatal("file-only restore changed exact immature rewards")
	}
	var ownership struct {
		IsMine bool `json:"ismine"`
	}
	restored.rpc.must(t, "validateaddress", []any{highAddress}, &ownership)
	if !ownership.IsMine {
		t.Fatal("file-only restore lost beyond-gap address ownership")
	}
	identity, err := restored.wallet.GymIdentity(context.Background())
	if err != nil || identity.PublicKey != fixture.Export.Manifest.GymPublicKey || identity.GymID != fixture.Export.Manifest.GymID {
		t.Fatal("file-only restore changed gym identity")
	}
	restored.rpc.must(t, "walletpassphrase", []any{integrationFixturePassword, 600}, nil)
	signer, err := restored.wallet.ProspectSigner(context.Background(), policy())
	if err != nil {
		t.Fatal(err)
	}
	authorization, err := signer(context.Background(), receipt.Context, receipt.Proof)
	if err != nil || authorization != *receipt.Authorization || gymauth.VerifyAuthorization(receipt.Context, receipt.Proof, authorization) != nil {
		t.Fatal("file-only restored wallet cannot reproduce its automatic admission authorization")
	}
	var sink struct {
		Address string `json:"external_sink"`
	}
	sinkJSON, err := os.ReadFile("../../../slugwallet/integration/testdata/addresses_v1.json")
	if err != nil || json.Unmarshal(sinkJSON, &sink) != nil || sink.Address == "" {
		t.Fatal("independently encoded public recipient fixture missing")
	}
	var spend string
	restored.rpc.must(t, "sendtoaddress", []any{sink.Address, 1}, &spend)
	var hashes []string
	node.rpc.must(t, "generate", []any{1}, &hashes)
	if len(hashes) != 1 {
		t.Fatal("restored fixture spend was not mined")
	}
	integrationWaitSync(t, restored, height+1)
	var transaction struct {
		Confirmations int64  `json:"confirmations"`
		Hex           string `json:"hex"`
	}
	restored.rpc.must(t, "gettransaction", []any{spend}, &transaction)
	if transaction.Confirmations < 1 || transaction.Hex == "" {
		t.Fatal("file-only restored wallet could not make a confirmed RPC spend")
	}
	restored.close(t)
	node.close(t)
	t.Logf("desktop file-only recovery passed: mature normal rewards at%d; stable independently derived payout/gym; actual wallet HTTP address/balance/UTXO/history calls; retained later export cursor26; asynchronous backup validation; original database inaccessible; exact restored funds/history; identical automatic authorization; restored RPC spend %s confirmed at%d", height, spend, height+1)
}
