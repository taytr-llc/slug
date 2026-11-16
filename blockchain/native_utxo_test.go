package blockchain

import (
	"testing"
	"time"

	"github.com/taytr-llc/slug/blockchain/internal/testhelper"
	"github.com/taytr-llc/slug/btcutil/v2"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/wire/v2"
)

// nativeUtxoParams keeps regtest's fixed easy target for a long storage test.
// The long subsidy interval keeps its spendable outputs useful through the
// 14,131-block replay. These are test parameters, never network parameters.
func nativeUtxoParams() chaincfg.Params {
	params := chaincfg.RegressionNetParams
	params.SubsidyReductionInterval = 1_050_000
	return params
}

// nativeUtxoBlocks replaces the historical Bitcoin storage fixture with a
// chain whose identities and PoW are valid for the selected SLUG hash profile.
// One mature coinbase is spent and then respent in every later block, so crash
// replay must reconstruct spent outputs as well as new coinbase entries.
func nativeUtxoBlocks(t *testing.T, params *chaincfg.Params) []*btcutil.Block {
	t.Helper()
	const height = 14_131
	blocks := make([]*btcutil.Block, height+1)
	blocks[0] = btcutil.NewBlock(params.GenesisBlock)
	blocks[0].SetHeight(0)
	var spend *testhelper.SpendableOut
	for i := int32(1); i <= height; i++ {
		coinbase := testhelper.CreateCoinbaseTx(i, CalcBlockSubsidy(i, params))
		txs := []*wire.MsgTx{coinbase}
		if i == int32(params.CoinbaseMaturity)+1 {
			out := testhelper.MakeSpendableOut(blocks[1].MsgBlock(), 0, 0)
			spend = &out
		}
		if spend != nil {
			tx := testhelper.CreateSpendTx(spend, testhelper.LowFee)
			txs = append(txs, tx)
			coinbase.TxOut[0].Value += int64(testhelper.LowFee)
			out := testhelper.MakeSpendableOutForTx(tx, 0)
			spend = &out
		}
		msg := &wire.MsgBlock{
			Header: wire.BlockHeader{
				Version: 4, PrevBlock: *blocks[i-1].Hash(),
				MerkleRoot: calcMerkleRoot(txs), Bits: params.PowLimitBits,
				// After BIP16 activation, well before the current time.
				Timestamp: time.Unix(1_600_000_000+int64(i), 0),
			},
			Transactions: txs,
		}
		if !testhelper.SolveBlock(&msg.Header) {
			t.Fatalf("cannot solve native storage fixture at height %d", i)
		}
		blocks[i] = btcutil.NewBlock(msg)
		blocks[i].SetHeight(i)
	}
	return blocks
}

// assertNativeSpendTip checks both sides of the replayed spend chain. An
// implementation that merely reconnects coinbases cannot satisfy this check.
func assertNativeSpendTip(t *testing.T, chain *BlockChain, blocks []*btcutil.Block, height int) {
	t.Helper()
	for _, check := range []struct {
		out   wire.OutPoint
		spent bool
	}{
		{testhelper.MakeSpendableOut(blocks[1].MsgBlock(), 0, 0).PrevOut, true},
		{testhelper.MakeSpendableOut(blocks[height-1].MsgBlock(), 1, 0).PrevOut, true},
		{testhelper.MakeSpendableOut(blocks[height].MsgBlock(), 1, 0).PrevOut, false},
	} {
		entry, err := chain.FetchUtxoEntry(check.out)
		if err != nil {
			t.Fatal(err)
		}
		spent := entry == nil || entry.IsSpent()
		if spent != check.spent {
			t.Fatalf("height %d output %v spent=%v, want %v", height, check.out, spent, check.spent)
		}
	}
}
