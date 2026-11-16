// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package blockchain

import (
	"math/big"
	"testing"

	"github.com/taytr-llc/slug/btcutil/v2"
	"github.com/taytr-llc/slug/chaincfg/v2"
)

func fullBlockHeaderChain(t *testing.T) (*BlockChain, *chaincfg.Params) {
	t.Helper()
	params := chaincfg.MainNetParams
	// Deployment clocks are mutable interfaces; own them before New binds them.
	for i := range params.Deployments {
		d := &params.Deployments[i]
		if starter, ok := d.DeploymentStarter.(*chaincfg.MedianTimeDeploymentStarter); ok {
			d.DeploymentStarter = chaincfg.NewMedianTimeDeploymentStarter(starter.StartTime())
		}
		if ender, ok := d.DeploymentEnder.(*chaincfg.MedianTimeDeploymentEnder); ok {
			d.DeploymentEnder = chaincfg.NewMedianTimeDeploymentEnder(ender.EndTime())
		}
	}
	chain, close, err := chainSetup(t.Name(), &params)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(close)
	return chain, &params
}
func assertHeaderTip(t *testing.T, chain *BlockChain, block *btcutil.Block) {
	t.Helper()
	hash, height := chain.BestHeader()
	if hash != *block.Hash() || height != block.Height() {
		t.Fatalf("header tip %s/%d, expected %s/%d", hash, height, block.Hash(), block.Height())
	}
	node := chain.index.LookupNode(block.Hash())
	if node == nil || chain.bestHeader.Tip() != node || !chain.bestHeader.Contains(node) {
		t.Fatal("best header lost canonical index node")
	}
}
func acceptFullBlock(t *testing.T, chain *BlockChain, block *btcutil.Block, wantMain bool) {
	t.Helper()
	main, orphan, err := chain.ProcessBlock(block, BFNone)
	if err != nil || orphan || main != wantMain {
		t.Fatalf("full block %d main=%v orphan=%v error=%v", block.Height(), main, orphan, err)
	}
}

func TestFullBlockAcceptanceUpdatesBestHeader(t *testing.T) {
	chain, params := fullBlockHeaderChain(t)
	tip := nativeTestGenesis(params)
	notifications := 0
	chain.Subscribe(func(notification *Notification) {
		if notification.Type == NTBlockAccepted {
			notifications++
			block := notification.Data.(*btcutil.Block)
			assertHeaderTip(t, chain, block)
		}
	})
	// No header message precedes these blocks: RPC submission and full-block
	// peer delivery must leave the same header view as headers-first sync.
	for height := 1; height <= 3; height++ {
		tip = newNativeTestBlock(t, params, tip, 0)
		acceptFullBlock(t, chain, tip, true)
		assertHeaderTip(t, chain, tip)
		if chain.BestSnapshot().Hash != *tip.Hash() {
			t.Fatal("best chain did not connect")
		}
	}
	if notifications != 3 {
		t.Fatal("missing full-block acceptance notification")
	}
	// A duplicate/stale block cannot move either view backwards.
	first, err := chain.BlockByHeight(1)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = chain.ProcessBlock(first, BFNone); err == nil {
		t.Fatal("duplicate full block accepted")
	}
	assertHeaderTip(t, chain, tip)
}

func TestFullBlockHeaderViewPreservesAheadHeadersAndReorgs(t *testing.T) {
	chain, params := fullBlockHeaderChain(t)
	genesis := nativeTestGenesis(params)
	a1 := newNativeTestBlock(t, params, genesis, 0)
	a2 := newNativeTestBlock(t, params, a1, 0)
	acceptFullBlock(t, chain, a1, true)
	acceptFullBlock(t, chain, a2, true)
	// A heavier header-only fork is retained while full blocks catch up.
	b := []*btcutil.Block{genesis}
	for i := 1; i <= 4; i++ {
		block := newNativeTestBlock(t, params, b[i-1], 1)
		b = append(b, block)
		if _, err := chain.ProcessBlockHeader(&block.MsgBlock().Header, BFNone, false); err != nil {
			t.Fatal(err)
		}
	}
	ahead := chain.index.LookupNode(b[4].Hash())
	assertHeaderTip(t, chain, b[4])
	for i := 1; i <= 4; i++ {
		headerNode := chain.index.LookupNode(b[i].Hash())
		acceptFullBlock(t, chain, b[i], i >= 3)
		if chain.index.LookupNode(b[i].Hash()) != headerNode {
			t.Fatal("full block replaced pre-existing header node")
		}
		assertHeaderTip(t, chain, b[4])
		if chain.bestHeader.Tip() != ahead {
			t.Fatal("catch-up replaced or rewound ahead header tip")
		}
	}
	if chain.BestSnapshot().Hash != *b[4].Hash() {
		t.Fatal("full-block fork did not reorganize")
	}
	// The old chain can regain the lead using full blocks only. An orphan and
	// lower/equal-work side branch must not replace the selected header tip.
	a3 := newNativeTestBlock(t, params, a2, 0)
	a4 := newNativeTestBlock(t, params, a3, 0)
	a5 := newNativeTestBlock(t, params, a4, 0)
	main, orphan, err := chain.ProcessBlock(a4, BFNone)
	if err != nil || main || !orphan {
		t.Fatal("unconnected full block was not an orphan")
	}
	assertHeaderTip(t, chain, b[4])
	acceptFullBlock(t, chain, a3, false)
	assertHeaderTip(t, chain, b[4])
	if chain.index.LookupNode(a4.Hash()) == nil {
		t.Fatal("orphan was not accepted after its parent")
	}
	acceptFullBlock(t, chain, a5, true)
	assertHeaderTip(t, chain, a5)
	if chain.BestSnapshot().Hash != *a5.Hash() {
		t.Fatal("higher-work full-block chain did not regain the lead")
	}
}

// A header can be sane while its full block overclaims the subsidy. Failure in
// connection validation must not publish that new index node as the header tip.
func TestFullBlockRejectionDoesNotAdvanceBestHeader(t *testing.T) {
	chain, params := fullBlockHeaderChain(t)
	genesis := nativeTestGenesis(params)
	valid := newNativeTestBlock(t, params, genesis, 0)
	acceptFullBlock(t, chain, valid, true)
	candidate := newNativeTestBlock(t, params, valid, 0)
	msg := candidate.MsgBlock()
	msg.Transactions[0].TxOut[0].Value++
	msg.Header.MerkleRoot = msg.Transactions[0].TxHash()
	target := CompactToBig(msg.Header.Bits)
	var invalid *btcutil.Block
	for nonce := uint32(0); nonce < 1024; nonce++ {
		msg.Header.Nonce = nonce
		hash := nativeTestHeaderHash(&msg.Header)
		var bigEndian [32]byte
		for i := range hash {
			bigEndian[len(hash)-1-i] = hash[i]
		}
		if new(big.Int).SetBytes(bigEndian[:]).Cmp(target) <= 0 {
			invalid = btcutil.NewBlock(msg) // Fresh hash cache after mutation.
			if *invalid.Hash() != hash {
				t.Fatal("independent fixture hash mismatch")
			}
			break
		}
	}
	if invalid == nil {
		t.Fatal("invalid-body fixture solver exhausted bound")
	}
	_, _, err := chain.ProcessBlock(invalid, BFNone)
	rule, ok := err.(RuleError)
	if !ok || rule.ErrorCode != ErrBadCoinbaseValue {
		t.Fatalf("expected connection rejection, got %v", err)
	}
	node := chain.index.LookupNode(invalid.Hash())
	if node == nil || !chain.index.NodeStatus(node).KnownInvalid() {
		t.Fatal("failed body was not marked invalid")
	}
	assertHeaderTip(t, chain, valid)
	if chain.BestSnapshot().Hash != *valid.Hash() {
		t.Fatal("invalid full block changed best chain")
	}
	if _, err := chain.ProcessBlockHeader(&msg.Header, BFNone, false); err == nil {
		t.Fatal("known-invalid header was accepted")
	}
	assertHeaderTip(t, chain, valid)
}
