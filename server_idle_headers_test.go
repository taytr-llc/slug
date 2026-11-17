// Copyright (c) 2026 The SLUG developers. ISC license.

package main

import (
	"net"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/taytr-llc/slug/blockchain"
	"github.com/taytr-llc/slug/btcjson"
	"github.com/taytr-llc/slug/btcutil/v2"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/database"
	"github.com/taytr-llc/slug/netsync"
	"github.com/taytr-llc/slug/peer"
	"github.com/taytr-llc/slug/txscript/v2"
	"github.com/taytr-llc/slug/wire/v2"
)

type idleHeaderClock struct{ now time.Time }

func (c idleHeaderClock) AdjustedTime() time.Time         { return c.now }
func (c idleHeaderClock) AddTimeSample(string, time.Time) {}
func (c idleHeaderClock) Offset() time.Duration           { return 0 }

// These are disposable deterministic block fixtures, never submitted to a node
// or public network. They pass the selected build's real PoW and full block
// checks; no validation flags, currentness overrides or consensus edits apply.
func idleHeaderBlock(t *testing.T, params *chaincfg.Params, parent *wire.MsgBlock, height int32) *wire.MsgBlock {
	t.Helper()
	script, err := txscript.NewScriptBuilder().AddInt64(int64(height)).AddInt64(0).Script()
	if err != nil {
		t.Fatal(err)
	}
	coinbase := wire.NewMsgTx(1)
	coinbase.AddTxIn(wire.NewTxIn(&wire.OutPoint{Index: ^uint32(0)}, script, nil))
	coinbase.AddTxOut(wire.NewTxOut(blockchain.CalcBlockSubsidy(height, params), []byte{txscript.OP_TRUE}))
	block := &wire.MsgBlock{
		Header: wire.BlockHeader{Version: 4, PrevBlock: parent.BlockHash(),
			MerkleRoot: coinbase.TxHash(), Timestamp: parent.Header.Timestamp.Add(params.TargetTimePerBlock),
			Bits: params.PowLimitBits},
		Transactions: []*wire.MsgTx{coinbase},
	}
	target := blockchain.CompactToBig(block.Header.Bits)
	for nonce := uint32(0); nonce < 1024; nonce++ {
		block.Header.Nonce = nonce
		hash := block.BlockHash()
		if blockchain.HashToBig(&hash).Cmp(target) <= 0 {
			return block
		}
	}
	t.Fatal("bounded historical fixture solver exhausted")
	return nil
}

func TestIdleTipServesHeadersWithoutBecomingCurrent(t *testing.T) {
	params := chaincfg.MainNetParams
	// Deployment clocks acquire the chain as their source. Own them rather
	// than mutate global parameter objects shared with other tests.
	for i := range params.Deployments {
		d := &params.Deployments[i]
		if s, ok := d.DeploymentStarter.(*chaincfg.MedianTimeDeploymentStarter); ok {
			d.DeploymentStarter = chaincfg.NewMedianTimeDeploymentStarter(s.StartTime())
		}
		if e, ok := d.DeploymentEnder.(*chaincfg.MedianTimeDeploymentEnder); ok {
			d.DeploymentEnder = chaincfg.NewMedianTimeDeploymentEnder(e.EndTime())
		}
	}
	db, err := database.Create("ffldb", filepath.Join(t.TempDir(), "chain"), params.Net)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	chain, err := blockchain.New(&blockchain.Config{DB: db, ChainParams: &params,
		TimeSource: idleHeaderClock{params.GenesisBlock.Header.Timestamp.Add(50 * time.Hour)}})
	if err != nil {
		t.Fatal(err)
	}
	first := idleHeaderBlock(t, &params, params.GenesisBlock, 1)
	second := idleHeaderBlock(t, &params, first, 2)
	for _, block := range []*wire.MsgBlock{first, second} {
		mainChain, orphan, err := chain.ProcessBlock(btcutil.NewBlock(block), blockchain.BFNone)
		if err != nil || orphan || !mainChain {
			t.Fatalf("fixture block was not fully accepted: main=%v orphan=%v err=%v", mainChain, orphan, err)
		}
	}
	// A higher header-only tip represents an actively downloading node. Normal
	// locator replies must remain limited to its connected bestChain prefix;
	// the existing explicit known-stop request may fetch the single header.
	third := idleHeaderBlock(t, &params, second, 3)
	if _, err := chain.ProcessBlockHeader(&third.Header, blockchain.BFNone, false); err != nil {
		t.Fatal(err)
	}
	syncer, err := netsync.New(&netsync.Config{Chain: chain, ChainParams: &params, MaxPeers: 1})
	if err != nil {
		t.Fatal(err)
	}
	syncer.Start()
	t.Cleanup(func() { _ = syncer.Stop() })
	assertNotCurrent := func() {
		t.Helper()
		if chain.IsCurrent() || syncer.IsCurrent() {
			t.Fatal("serving history must not change freshness or synchronization status")
		}
		if chain.BestSnapshot().Height != 2 {
			t.Fatal("header-only fixture entered the connected best chain")
		}
		if _, height := chain.BestHeader(); height != 3 {
			t.Fatal("missing higher validated header fixture")
		}
		requireRPCErrorCode(t, checkTemplateSyncState(&config{}, chain,
			&pilotGateConnections{count: 1}, &pilotGateSync{current: syncer.IsCurrent()}),
			btcjson.ErrRPCClientInInitialDownload)
	}
	assertNotCurrent()
	before := *chain.BestSnapshot()
	beforeHeader, beforeHeaderHeight := chain.BestHeader()

	received := make(chan *wire.MsgHeaders, 1)
	handshakes := make(chan struct{}, 2)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	sp := &serverPeer{server: &server{chain: chain, syncManager: syncer}}
	sp.Peer = peer.NewInboundPeer(&peer.Config{ChainParams: &params, AllowSelfConns: true,
		Listeners: peer.MessageListeners{OnGetHeaders: sp.OnGetHeaders,
			OnVerAck: func(*peer.Peer, *wire.MsgVerAck) { handshakes <- struct{}{} }}})
	requester, err := peer.NewOutboundPeer(&peer.Config{ChainParams: &params, AllowSelfConns: true,
		Listeners: peer.MessageListeners{
			OnVerAck:  func(*peer.Peer, *wire.MsgVerAck) { handshakes <- struct{}{} },
			OnHeaders: func(_ *peer.Peer, msg *wire.MsgHeaders) { received <- msg },
		}}, listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	right, err := net.DialTimeout("tcp", listener.Addr().String(), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = right.Close() })
	left, err := listener.Accept()
	if err != nil {
		t.Fatal(err)
	}
	_ = listener.Close()
	t.Cleanup(func() {
		sp.Disconnect()
		requester.Disconnect()
		_ = left.Close()
		_ = right.Close()
		sp.WaitForDisconnect()
		requester.WaitForDisconnect()
	})
	sp.AssociateConnection(left)
	requester.AssociateConnection(right)
	for i := 0; i < 2; i++ {
		select {
		case <-handshakes:
		case <-time.After(5 * time.Second):
			t.Fatal("real peer handshake did not complete")
		}
	}
	genesis, firstHash, secondHash, thirdHash := *params.GenesisHash, first.BlockHash(), second.BlockHash(), third.BlockHash()
	unknown := chainhash.Hash{0xff}
	for _, test := range []struct {
		name    string
		locator []*chainhash.Hash
		stop    chainhash.Hash
		want    []wire.BlockHeader
	}{
		{"connected-prefix", []*chainhash.Hash{&genesis}, chainhash.Hash{}, []wire.BlockHeader{first.Header, second.Header}},
		{"inclusive-stop", []*chainhash.Hash{&genesis}, firstHash, []wire.BlockHeader{first.Header}},
		{"common-locator", []*chainhash.Hash{&unknown, &firstHash}, chainhash.Hash{}, []wire.BlockHeader{second.Header}},
		{"unknown-locator", []*chainhash.Hash{&unknown}, chainhash.Hash{}, []wire.BlockHeader{first.Header, second.Header}},
		{"at-connected-tip", []*chainhash.Hash{&secondHash}, chainhash.Hash{}, nil},
		{"explicit-genesis", nil, genesis, []wire.BlockHeader{params.GenesisBlock.Header}},
		{"explicit-header-only", nil, thirdHash, []wire.BlockHeader{third.Header}},
		{"unknown-explicit-stop", nil, unknown, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			requester.QueueMessage(&wire.MsgGetHeaders{ProtocolVersion: requester.ProtocolVersion(),
				BlockLocatorHashes: test.locator, HashStop: test.stop}, nil)
			select {
			case response := <-received:
				if len(response.Headers) != len(test.want) {
					t.Fatalf("got %d headers, want %d", len(response.Headers), len(test.want))
				}
				for i, header := range response.Headers {
					if !reflect.DeepEqual(*header, test.want[i]) {
						t.Fatalf("header %d differs from exact stored history", i)
					}
				}
			case <-time.After(2 * time.Second):
				t.Fatal("idle node suppressed the required headers response")
			}
		})
	}
	assertNotCurrent()
	if !reflect.DeepEqual(*chain.BestSnapshot(), before) {
		t.Fatal("serving headers mutated the connected chain snapshot")
	}
	if hash, height := chain.BestHeader(); hash != beforeHeader || height != beforeHeaderHeight {
		t.Fatal("serving headers mutated the best header")
	}
	if !sp.Connected() || !requester.Connected() {
		t.Fatal("header exchange disconnected the peer pair")
	}
}
