// Copyright (c) 2026 The SLUG developers. ISC license.

package main

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/taytr-llc/slug/blockchain"
	"github.com/taytr-llc/slug/btcjson"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/database"
	"github.com/taytr-llc/slug/wire/v2"
)

// Headers must describe header synchronization independently of downloaded
// blocks. Reporting blocks twice hid the full-block header-view regression.
func TestBlockChainInfoReportsHeaderTip(t *testing.T) {
	params := chaincfg.MainNetParams
	for i := range params.Deployments {
		d := &params.Deployments[i]
		if starter, ok := d.DeploymentStarter.(*chaincfg.MedianTimeDeploymentStarter); ok {
			d.DeploymentStarter = chaincfg.NewMedianTimeDeploymentStarter(starter.StartTime())
		}
		if ender, ok := d.DeploymentEnder.(*chaincfg.MedianTimeDeploymentEnder); ok {
			d.DeploymentEnder = chaincfg.NewMedianTimeDeploymentEnder(ender.EndTime())
		}
	}
	db, err := database.Create("ffldb", filepath.Join(t.TempDir(), "db"), params.Net)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	chain, err := blockchain.New(&blockchain.Config{DB: db, ChainParams: &params,
		TimeSource: blockchain.NewMedianTime()})
	if err != nil {
		t.Fatal(err)
	}
	previous := cfg
	cfg = &config{}
	t.Cleanup(func() { cfg = previous })
	s := &rpcServer{cfg: rpcserverConfig{Chain: chain, ChainParams: &params}}
	assertHeights := func(headers int32) {
		t.Helper()
		response, err := handleGetBlockChainInfo(s, btcjson.NewGetBlockChainInfoCmd(), nil)
		if err != nil {
			t.Fatal(err)
		}
		info := response.(*btcjson.GetBlockChainInfoResult)
		if info.Headers != headers || info.Blocks != 0 || info.BestBlockHash != params.GenesisHash.String() {
			t.Fatalf("header-only synchronization misreported: %+v", info)
		}
	}
	assertHeights(0)
	header := wire.BlockHeader{Version: 4, PrevBlock: *params.GenesisHash,
		MerkleRoot: chainhash.Hash{1}, Bits: params.GenesisBlock.Header.Bits,
		Timestamp: params.GenesisBlock.Header.Timestamp.Add(time.Second)}
	target := blockchain.CompactToBig(header.Bits)
	for ; header.Nonce < 1024; header.Nonce++ {
		hash := header.BlockHash()
		if blockchain.HashToBig(&hash).Cmp(target) <= 0 {
			break
		}
	}
	if header.Nonce == 1024 {
		t.Fatal("header fixture solver exhausted bound")
	}
	if _, err := chain.ProcessBlockHeader(&header, blockchain.BFNone, false); err != nil {
		t.Fatal(err)
	}
	assertHeights(1)
}
