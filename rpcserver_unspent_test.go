// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"path/filepath"
	"testing"

	"github.com/taytr-llc/slug/blockchain"
	"github.com/taytr-llc/slug/btcjson"
	"github.com/taytr-llc/slug/btcutil/v2"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/database"
	"github.com/taytr-llc/slug/mempool/mempooltest"
	"github.com/taytr-llc/slug/wire/v2"
)

// These are RPC transport fixtures, not claims that the synthetic pool
// transactions were accepted by consensus. Full carrier relay is tested on
// the actual isolated nodes separately.
func TestGetTxOutExcludesMempoolSpends(t *testing.T) {
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
	db, err := database.Create("ffldb", filepath.Join(t.TempDir(), "chain"), params.Net)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	chain, err := blockchain.New(&blockchain.Config{DB: db, ChainParams: &params, TimeSource: blockchain.NewMedianTime()})
	if err != nil {
		t.Fatal(err)
	}
	parent := wire.NewMsgTx(1)
	parent.AddTxOut(wire.NewTxOut(500000, []byte{0x51}))
	tx := btcutil.NewTx(parent)
	out := wire.OutPoint{Hash: *tx.Hash(), Index: 0}
	spender := btcutil.NewTx(wire.NewMsgTx(1))
	for _, test := range []struct {
		name                                string
		before, during, include, wantOutput bool
	}{
		{"already spent", true, false, true, false},
		{"spent during lookup", false, true, true, false},
		{"unspent pool parent", false, false, true, true},
		{"chain only excludes pool lookup", false, false, false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			pool := &mempooltest.MockTxMempool{}
			if test.include {
				var first, second *btcutil.Tx
				if test.before {
					first = spender
				}
				if test.during {
					second = spender
				}
				pool.On("CheckSpend", out).Return(first).Once()
				if !test.before {
					pool.On("HaveTransaction", tx.Hash()).Return(true).Once()
					pool.On("FetchTransaction", tx.Hash()).Return(tx, nil).Once()
					pool.On("CheckSpend", out).Return(second).Once()
				}
			}
			s := &rpcServer{cfg: rpcserverConfig{Chain: chain, ChainParams: &params, TxMemPool: pool}}
			got, err := handleGetTxOut(s, btcjson.NewGetTxOutCmd(tx.Hash().String(), 0, &test.include), nil)
			if err != nil {
				t.Fatal(err)
			}
			if test.wantOutput {
				value, ok := got.(*btcjson.GetTxOutResult)
				if !ok || value.Value != 0.005 || value.ScriptPubKey.Hex != "51" || value.Confirmations != 0 {
					t.Fatalf("wrong unspent output: %#v", got)
				}
			} else if got != nil {
				t.Fatalf("spent/missing output returned: %#v", got)
			}
			pool.AssertExpectations(t)
		})
	}
}
