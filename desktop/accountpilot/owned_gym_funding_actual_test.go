//go:build desktopintegration && !slugtestpow

package accountpilot

import (
	"bytes"
	"context"
	"encoding/hex"
	"math"
	"testing"
	"time"

	"github.com/taytr-llc/slug/wire/v2"
)

type ownedFundedEvidence struct {
	Fixture       string `json:"fixture"`
	TxID          string `json:"txid"`
	Address       string `json:"address"`
	Script        string `json:"script"`
	FundingHeight uint32 `json:"funding_height"`
	MatureHeight  uint32 `json:"mature_height"`
	MatureHash    string `json:"mature_hash"`
}

// ownedFundedMature reuses the existing qualified, bounded public-fixture
// funding/descendant leaves. It adds no header hashing loop or signing seam.
// The caller owns the actual primary node and serial work lane and must keep
// app mining stopped throughout. Wallet sync/spendability is checked by the
// caller through the ordinary controller after this canonical chain evidence.
func ownedFundedMature(t *testing.T, ctx context.Context, rpc *poolActualRPC, fixture string) ownedFundedEvidence {
	t.Helper()
	if wire.TestPoW || ctx == nil || rpc == nil || (fixture != "zero256" && fixture != "incrementing256") {
		t.Fatal("explicit normal public-fixture funding required")
	}
	ctx, cancel := context.WithTimeout(ctx, 8*time.Minute)
	defer cancel()
	want := poolBackupOracle(t, fixture) // Hash-pinned old recovery_v1 native84/0/0.
	var before uint32
	if e := rpc.call(ctx, "getblockcount", nil, &before); e != nil || before > math.MaxUint32-101 {
		t.Fatal("funding initial height", e)
	}
	var mempool []string
	if e := rpc.call(ctx, "getrawmempool", nil, &mempool); e != nil || len(mempool) != 0 {
		t.Fatal("funding requires empty owned mempool", e)
	}
	txid := hostedActualFund(t, ctx, rpc, want)
	fundingHeight := before + 1
	var height uint32
	var fundingHash string
	if e := rpc.call(ctx, "getblockcount", nil, &height); e != nil || height != fundingHeight {
		t.Fatal("funding advanced exactly one block", e)
	}
	if e := rpc.call(ctx, "getblockhash", []any{fundingHeight}, &fundingHash); e != nil {
		t.Fatal("canonical funding hash", e)
	}
	// Re-read canonical accepted bytes without performing a second Argon hash.
	var rawHex string
	if e := rpc.call(ctx, "getblock", []any{fundingHash, 0}, &rawHex); e != nil || len(rawHex) > 2<<20 {
		t.Fatal("bounded canonical funding block", e)
	}
	raw, e := hex.DecodeString(rawHex)
	var block wire.MsgBlock
	if e != nil || block.Deserialize(bytes.NewReader(raw)) != nil || len(block.Transactions) != 1 || block.Transactions[0].TxHash().String() != txid {
		t.Fatal("exact canonical funding transaction")
	}
	matches := 0
	for _, output := range block.Transactions[0].TxOut {
		if hex.EncodeToString(output.PkScript) == want.Script && output.Value == hostedActualFundingAtoms {
			matches++
		}
	}
	if matches != 1 {
		t.Fatal("canonical coinbase differs from frozen script/amount")
	}
	poolActualMine(t, ctx, rpc, 100) // Existing unrelated-sink, unregistered descendants.
	matureHeight := fundingHeight + 100
	var matureHash, again string
	if e := rpc.call(ctx, "getblockcount", nil, &height); e != nil || height != matureHeight {
		t.Fatal("exact100 maturity descendants", e)
	}
	if e := rpc.call(ctx, "getblockhash", []any{matureHeight}, &matureHash); e != nil {
		t.Fatal("mature canonical tip", e)
	}
	if e := rpc.call(ctx, "getblockhash", []any{fundingHeight}, &again); e != nil || again != fundingHash {
		t.Fatal("funding remains on canonical branch", e)
	}
	if e := rpc.call(ctx, "getblockcount", nil, &height); e != nil || height != matureHeight {
		t.Fatal("maturity observation changed", e)
	}
	return ownedFundedEvidence{Fixture: fixture, TxID: txid, Address: want.Address, Script: want.Script, FundingHeight: fundingHeight, MatureHeight: matureHeight, MatureHash: matureHash}
}
