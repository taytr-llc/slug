// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package blockchain

import (
	"encoding/binary"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/taytr-llc/slug/btcutil/v2"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/txscript/v2"
	"github.com/taytr-llc/slug/wire/v2"
)

// These nodes model ancestry/height/time only, not mined block identities.
// Actual tested candidate headers still pass the ordinary context checks.
func activationChain(height int32) *BlockChain {
	params := chaincfg.MainNetParams
	// Deployment interfaces contain mutable clock pointers. Own them before
	// newFakeChain synchronizes clocks, preserving the global network params
	// and any other synthetic chain's clock.
	for i := range params.Deployments {
		deployment := &params.Deployments[i]
		if starter, ok := deployment.DeploymentStarter.(*chaincfg.MedianTimeDeploymentStarter); ok {
			deployment.DeploymentStarter = chaincfg.NewMedianTimeDeploymentStarter(starter.StartTime())
		}
		if ender, ok := deployment.DeploymentEnder.(*chaincfg.MedianTimeDeploymentEnder); ok {
			deployment.DeploymentEnder = chaincfg.NewMedianTimeDeploymentEnder(ender.EndTime())
		}
	}
	b := newFakeChain(&params)
	parent := b.bestChain.Tip()
	start := parent.timestamp
	for h := int32(1); h <= height; h++ {
		var hash chainhash.Hash
		binary.LittleEndian.PutUint32(hash[:4], uint32(h))
		hash[31] = 0xa5
		node := &blockNode{parent: parent, hash: hash, height: h, version: 4,
			bits: params.PowLimitBits, timestamp: start + int64(h)*120, workSum: big.NewInt(int64(h) + 1)}
		b.index.AddNode(node)
		parent = node
	}
	b.bestChain.SetTip(parent)
	b.sigCache = txscript.NewSigCache(100)
	b.hashCache = txscript.NewHashCache(100)
	return b
}

func TestPhaseAActivationBoundaryAndReorg(t *testing.T) {
	b := activationChain(63)
	tip := b.bestChain.Tip()
	for _, height := range []int32{0, 60, 61, 62, 63, 60, 61} {
		b.bestChain.SetTip(tip.Ancestor(height))
		want := ThresholdDefined
		if height >= 61 {
			want = ThresholdActive
		}
		for _, id := range []uint32{chaincfg.DeploymentCSV, chaincfg.DeploymentSegwit, chaincfg.DeploymentTaproot} {
			state, err := b.ThresholdState(id)
			require.NoError(t, err)
			require.Equal(t, want, state, "parent=%d deployment=%d", height, id)
		}
		version, err := b.CalcNextBlockVersion()
		require.NoError(t, err)
		require.GreaterOrEqual(t, version, int32(4))
	}
	// Seed a failed result at a real voting-window ancestor. Without the
	// override this is the exact cache entry thresholdState would consult.
	b = activationChain(int32(b.chainParams.MinerConfirmationWindow) - 1)
	for _, id := range []uint32{chaincfg.DeploymentCSV, chaincfg.DeploymentSegwit, chaincfg.DeploymentTaproot} {
		b.deploymentCaches[id].Update(&b.bestChain.Tip().hash, ThresholdFailed)
		state, err := b.ThresholdState(id)
		require.NoError(t, err)
		require.Equal(t, ThresholdActive, state)
		b.chainParams.Deployments[id].AlwaysActiveHeight = 0
		state, err = b.ThresholdState(id)
		require.NoError(t, err)
		require.Equal(t, ThresholdFailed, state)
	}
}

func TestPhaseABuriedVersionBoundary(t *testing.T) {
	b := activationChain(62)
	for _, height := range []int32{61, 62, 63} {
		parent := b.bestChain.Tip().Ancestor(height - 1)
		for _, version := range []int32{1, 2, 3, 4, 0x20000000} {
			header := wire.BlockHeader{Version: version, PrevBlock: parent.hash, Bits: b.chainParams.PowLimitBits, Timestamp: time.Unix(parent.timestamp+120, 0)}
			err := CheckBlockHeaderContext(&header, parent, BFNone, stubChainCtx{b.chainParams}, true)
			if height >= 62 && version < 4 {
				var rule RuleError
				require.ErrorAs(t, err, &rule)
				require.Equal(t, ErrBlockVersionTooOld, rule.ErrorCode)
			} else {
				require.NoError(t, err)
			}
		}
	}
}

func activationBlock(t *testing.T, b *BlockChain, tx *wire.MsgTx) *btcutil.Block {
	t.Helper()
	parent := b.bestChain.Tip()
	script, err := txscript.NewScriptBuilder().AddInt64(int64(parent.height + 1)).AddData([]byte("PUBLIC-ACTIVATION-FIXTURE")).Script()
	require.NoError(t, err)
	coinbase := wire.NewMsgTx(2)
	coinbase.AddTxIn(wire.NewTxIn(&wire.OutPoint{Index: wire.MaxPrevOutIndex}, script, nil))
	coinbase.AddTxOut(wire.NewTxOut(CalcBlockSubsidy(parent.height+1, b.chainParams), []byte{txscript.OP_TRUE}))
	msg := &wire.MsgBlock{Header: wire.BlockHeader{Version: 4, PrevBlock: parent.hash, Bits: b.chainParams.PowLimitBits, Timestamp: time.Unix(parent.timestamp+120, 0)}, Transactions: []*wire.MsgTx{coinbase}}
	if tx != nil {
		msg.Transactions = append(msg.Transactions, tx)
	}
	block := btcutil.NewBlock(msg)
	if tx != nil && tx.HasWitness() {
		coinbase.TxIn[0].Witness = wire.TxWitness{make([]byte, 32)}
		root := CalcMerkleRoot(block.Transactions(), true)
		var preimage [64]byte
		copy(preimage[:32], root[:])
		commitment := chainhash.DoubleHashB(preimage[:])
		coinbase.AddTxOut(wire.NewTxOut(0, append([]byte{0x6a, 0x24, 0xaa, 0x21, 0xa9, 0xed}, commitment...)))
		block = btcutil.NewBlock(msg)
	}
	msg.Header.MerkleRoot = CalcMerkleRoot(block.Transactions(), false)
	return block
}

func TestPhaseACoinbaseHeightBoundary(t *testing.T) {
	for _, height := range []int32{61, 62, 63} {
		for _, kind := range []string{"valid", "wrong", "truncated", "nonminimal"} {
			t.Run(fmt.Sprintf("%d/%s", height, kind), func(t *testing.T) {
				b := activationChain(height - 1)
				block := activationBlock(t, b, nil)
				switch kind {
				case "wrong":
					block.MsgBlock().Transactions[0].TxIn[0].SignatureScript = []byte{txscript.OP_0, txscript.OP_0}
				case "truncated":
					block.MsgBlock().Transactions[0].TxIn[0].SignatureScript = []byte{2, byte(height)}
				case "nonminimal":
					block.MsgBlock().Transactions[0].TxIn[0].SignatureScript = []byte{2, byte(height), 0}
				}
				err := b.checkBlockContext(block, b.bestChain.Tip(), BFNone)
				if height >= 62 && kind != "valid" {
					want := ErrBadCoinbaseHeight
					if kind == "truncated" {
						want = ErrMissingCoinbaseHeight
					}
					assertActivationRule(t, err, want)
				} else {
					require.NoError(t, err)
				}
			})
		}
	}
}

func TestPhaseAAlwaysChecksBIP30(t *testing.T) {
	b := activationChain(64)
	for _, height := range []int32{60, 61, 62, 63, 64} {
		require.True(t, bip0030CheckNeeded(b.bestChain.Tip().Ancestor(height), b.chainParams))
	}
}
