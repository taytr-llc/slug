//go:build desktopintegration && !slugtestpow

// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package pilot

import (
	"bytes"
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/taytr-llc/slug/blockchain"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/txscript/v2"
	"github.com/taytr-llc/slug/wire/v2"
)

// Prepare a real accepted block with an old timestamp. This is explicit fixture
// construction, not the pilot secretly mining a bootstrap block during backup
// verification or bypassing normal header/transaction validation.
func integrationOldBlock(t *testing.T, node *integrationNode, payoutScript string) string {
	t.Helper()
	var height int32
	node.rpc.must(t, "getblockcount", nil, &height)
	if height != 0 {
		t.Fatal("old-tip fixture must start at genesis")
	}
	oldTime := time.Now().Add(-26 * time.Hour).Truncate(time.Second)
	if !oldTime.After(chaincfg.MainNetParams.GenesisBlock.Header.Timestamp) {
		t.Fatal("fixture clock must permit a valid block older than 26 hours")
	}
	script, err := hex.DecodeString(payoutScript)
	if err != nil {
		t.Fatal(err)
	}
	signatureScript, err := txscript.NewScriptBuilder().AddInt64(1).AddData([]byte("PUBLIC-DESKTOP-STALE-FIXTURE")).Script()
	if err != nil {
		t.Fatal(err)
	}
	coinbase := wire.NewMsgTx(2)
	coinbase.AddTxIn(wire.NewTxIn(&wire.OutPoint{Index: wire.MaxPrevOutIndex}, signatureScript, nil))
	coinbase.AddTxOut(wire.NewTxOut(10e8, script))
	block := wire.MsgBlock{Header: wire.BlockHeader{
		Version: 0x20000000, PrevBlock: *chaincfg.MainNetParams.GenesisHash,
		MerkleRoot: coinbase.TxHash(), Timestamp: oldTime,
		Bits: chaincfg.MainNetParams.GenesisBlock.Header.Bits,
	}, Transactions: []*wire.MsgTx{coinbase}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	target := blockchain.CompactToBig(block.Header.Bits)
	var hash string
	for nonce := uint32(0); nonce < 512; nonce++ {
		if err := ctx.Err(); err != nil {
			t.Fatal(err)
		}
		block.Header.Nonce = nonce
		work := block.Header.BlockHash()
		if blockchain.HashToBig(&work).Cmp(target) <= 0 {
			hash = work.String()
			break
		}
	}
	if hash == "" {
		t.Fatal("bounded normal-PoW old-block fixture scan exhausted")
	}
	var encoded bytes.Buffer
	if err := block.Serialize(&encoded); err != nil {
		t.Fatal(err)
	}
	var result any
	node.rpc.must(t, "submitblock", []any{hex.EncodeToString(encoded.Bytes())}, &result)
	if result != nil {
		t.Fatalf("old fixture block rejected: %v", result)
	}
	var best struct {
		Hash   string `json:"hash"`
		Height int32  `json:"height"`
	}
	node.rpc.must(t, "getbestblock", nil, &best)
	if best.Height != 1 || best.Hash != hash {
		t.Fatal("old fixture block did not become canonical")
	}
	// A normal node may intentionally decline to relay old blocks while it
	// considers itself behind. Explicitly seed the same fully validated public
	// fixture into the second retained chain too; no relay-age rule is bypassed.
	var peerHash string
	if node.peer.rpc.call("getblockhash", []any{1}, &peerHash) != nil || peerHash != hash {
		var peerResult any
		if err := node.peer.rpc.call("submitblock", []any{hex.EncodeToString(encoded.Bytes())}, &peerResult); err != nil {
			t.Fatal("prepare old fixture peer:", err)
		}
	}
	deadline := time.Now().Add(20 * time.Second)
	for {
		var peerHash string
		if node.peer.rpc.call("getblockhash", []any{1}, &peerHash) == nil && peerHash == hash {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("old fixture block did not synchronize to the owned peer")
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Logf("explicitly prepared canonical normal-PoW height 1 block %s with timestamp %s (more than 26 hours old)", hash, oldTime.UTC().Format(time.RFC3339))
	return hash
}

func TestWalletPilotStaleTipNormalPoW(t *testing.T) {
	runWalletPilotStaleTip(t)
}

// The default node must keep its ordinary age barrier. This also captures the
// old binary's concrete failure before the isolated pilot override is enabled.
func TestWalletPilotDefaultGBTBarrierNormalPoW(t *testing.T) {
	binary := os.Getenv("SLUG_DESKTOP_NODE")
	if binary == "" {
		t.Skip("set SLUG_DESKTOP_NODE to an explicitly built normal node")
	}
	if !filepath.IsAbs(binary) {
		t.Fatal("explicit absolute normal node required")
	}
	fixture := integrationOracle(t)
	payout, script := fixture.address(t, "m/44'/0'/0'/0/0")
	node := integrationStartRetainedNode(t, binary, filepath.Join(t.TempDir(), "primary-chain"), payout)
	integrationOldBlock(t, node, script)
	var template any
	err := node.rpc.call("getblocktemplate", []any{map[string]any{
		"rules": []string{"segwit"}, "capabilities": []string{"coinbasevalue"},
	}}, &template)
	if err == nil || !strings.Contains(err.Error(), "(-10)") {
		t.Fatalf("default node lost its initial-download age barrier: %v", err)
	}
	node.close(t)
	t.Log("default node with the accepted 26-hour-old block and a connected owned peer correctly rejected GBT with initial-download error -10")
}

func runWalletPilotStaleTip(t *testing.T) {
	t.Helper()
	binary := os.Getenv("SLUG_DESKTOP_NODE")
	if binary == "" {
		t.Skip("set SLUG_DESKTOP_NODE to an explicitly built normal node")
	}
	if !filepath.IsAbs(binary) {
		t.Fatal("explicit absolute normal node required")
	}
	fixture := integrationOracle(t)
	payout, script := fixture.address(t, "m/44'/0'/0'/0/0")
	config := Config{DataDir: filepath.Join(t.TempDir(), "stale-wallet-pilot"), NodeBinary: binary}
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
	if err := controller.Close(); err != nil {
		t.Fatal(err)
	}
	node := integrationStartRetainedNode(t, binary, filepath.Join(config.DataDir, "primary-chain"), payout)
	oldHash := integrationOldBlock(t, node, script)
	node.close(t)

	controller, err = New(config)
	if err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(t.TempDir(), "PUBLIC-OLD-TIP-FIXTURE.json")
	if err := controller.ExportRecovery(backup); err != nil {
		t.Fatal(err)
	}
	if err := controller.VerifyBackup(backup); err != nil {
		t.Fatal("old-tip backup start:", err)
	}
	verified := integrationWaitPilot(t, controller, 2*time.Minute, func(value Snapshot) bool {
		return value.BackupState != "running" && value.BackupState != "idle"
	})
	if verified.BackupState != "succeeded" || verified.Attempts != 0 {
		t.Fatalf("old-tip backup scan failed or mined: %s %s", verified.BackupState, verified.BackupError)
	}
	if err := controller.Close(); err != nil {
		t.Fatal(err)
	}
	node = integrationStartRetainedNode(t, binary, filepath.Join(config.DataDir, "primary-chain"), payout)
	var best struct {
		Height int32  `json:"height"`
		Hash   string `json:"hash"`
	}
	node.rpc.must(t, "getbestblock", nil, &best)
	if best.Height != 1 || best.Hash != oldHash {
		t.Fatal("backup verification changed the 26-hour-old canonical chain")
	}
	node.close(t)
	t.Log("fresh backup restore synchronized the 26-hour-old chain without a new block")

	controller, err = New(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := controller.Start(); err != nil {
		t.Fatal(err)
	}
	resumed := integrationWaitPilot(t, controller, 2*time.Minute, func(value Snapshot) bool {
		return value.Height >= 2 && value.WalletHeight >= 2 && value.ProspectCount > 0
	})
	controller.Stop()
	integrationWaitPilot(t, controller, time.Minute, func(value Snapshot) bool { return value.State == "stopped" })
	if resumed.PayoutAddress != payout || resumed.GymID != fixture.Export.Manifest.GymID || resumed.GymPublicKey != fixture.Export.Manifest.GymPublicKey {
		t.Fatal("old-tip restart changed the wallet identity or payout")
	}
	if err := controller.Close(); err != nil {
		t.Fatal(err)
	}
	t.Logf("explicit isolated pilot resumed normal mining from the 26-hour-old tip; height %d with %d receipts", resumed.Height, resumed.ProspectCount)
}
