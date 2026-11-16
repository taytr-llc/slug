// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"testing"

	"github.com/taytr-llc/slug/blockchain"
	"github.com/taytr-llc/slug/btcjson"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/peer"
)

func validLocalPilotConfig() config {
	return config{LocalWalletPilot: true, DisableDNSSeed: true,
		ConnectPeers: []string{"127.0.0.1:29334"},
		Listeners:    []string{"127.0.0.1:29335"},
		RPCListeners: []string{"127.0.0.1:29336"},
		RPCUser:      "public-test-user", RPCPass: "public-test-password"}
}

func TestLocalWalletPilotConfigIsolation(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*config)
	}{
		{"DNS", func(c *config) { c.DisableDNSSeed = false }},
		{"discovery", func(c *config) { c.ConnectPeers = nil }},
		{"addpeer", func(c *config) { c.AddPeers = []string{"127.0.0.1:29334"} }},
		{"UPnP", func(c *config) { c.Upnp = true }},
		{"external-address", func(c *config) { c.ExternalIPs = []string{"127.0.0.1"} }},
		{"proxy", func(c *config) { c.Proxy = "127.0.0.1:9050" }},
		{"onion", func(c *config) { c.OnionProxy = "127.0.0.1:9050" }},
		{"tor", func(c *config) { c.TorIsolation = true }},
		{"profile", func(c *config) { c.Profile = "6060" }},
		{"no-RPC", func(c *config) { c.DisableRPC = true }},
		{"default-RPC", func(c *config) { c.RPCListeners = nil }},
		{"default-P2P", func(c *config) { c.Listeners = nil }},
		{"no-admin", func(c *config) { c.RPCUser = "" }},
		{"no-password", func(c *config) { c.RPCPass = "" }},
		{"regtest", func(c *config) { c.RegressionTest = true }},
		{"testnet", func(c *config) { c.TestNet3 = true }},
		{"testnet4", func(c *config) { c.TestNet4 = true }},
		{"simnet", func(c *config) { c.SimNet = true }},
		{"signet", func(c *config) { c.SigNet = true }},
		{"legacy-checkpoint", func(c *config) { c.AddCheckpoints = []string{"62:00"} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := validLocalPilotConfig()
			test.change(&c)
			if validateLocalWalletPilot(&c, &chaincfg.MainNetParams) == nil {
				t.Fatal("unsafe pilot configuration accepted")
			}
			c.LocalWalletPilot = false
			if err := validateLocalWalletPilot(&c, &chaincfg.MainNetParams); err != nil {
				t.Fatalf("ordinary configuration changed: %v", err)
			}
		})
	}
	for _, endpoint := range []string{"localhost:29333", "0.0.0.0:29333", "[::]:29333", "192.0.2.1:29333", "127.0.0.1", ":29333", "127.0.0.1:0", "127.0.0.1:65536", "127.0.0.1:029333", "[::1%lo0]:29333"} {
		for _, kind := range []string{"connect", "listen", "RPC"} {
			c := validLocalPilotConfig()
			switch kind {
			case "connect":
				c.ConnectPeers = []string{endpoint}
			case "listen":
				c.Listeners = []string{endpoint}
			case "RPC":
				c.RPCListeners = []string{endpoint}
			}
			if validateLocalWalletPilot(&c, &chaincfg.MainNetParams) == nil {
				t.Errorf("accepted %s endpoint %q", kind, endpoint)
			}
		}
	}
	c := validLocalPilotConfig()
	if err := validateLocalWalletPilot(&c, &chaincfg.MainNetParams); err != nil {
		t.Fatal(err)
	}
	c.DisableListen, c.Listeners = true, nil
	if err := validateLocalWalletPilot(&c, &chaincfg.MainNetParams); err != nil {
		t.Fatal(err)
	}
	c.ConnectPeers, c.RPCListeners = []string{"[::1]:29334"}, []string{"[::1]:29336"}
	if err := validateLocalWalletPilot(&c, &chaincfg.MainNetParams); err != nil {
		t.Fatal(err)
	}
	for _, params := range []*chaincfg.Params{nil, &chaincfg.RegressionNetParams} {
		if validateLocalWalletPilot(&c, params) == nil {
			t.Fatal("wrong network accepted")
		}
	}
	params := chaincfg.MainNetParams
	params.Deployments[chaincfg.DeploymentSegwit].AlwaysActiveHeight = 0
	if validateLocalWalletPilot(&c, &params) == nil {
		t.Fatal("missing activation policy accepted")
	}
	params = chaincfg.MainNetParams
	params.Checkpoints = []chaincfg.Checkpoint{{Height: 62}}
	if validateLocalWalletPilot(&c, &params) == nil {
		t.Fatal("legacy checkpoint floor bypassed")
	}
}

type pilotGateChain struct {
	best, after *blockchain.BestState
	header      chainhash.Hash
	height      int32
	reads       int
}

func (c *pilotGateChain) BestSnapshot() *blockchain.BestState {
	c.reads++
	if c.reads > 1 && c.after != nil {
		return c.after
	}
	return c.best
}
func (c *pilotGateChain) BestHeader() (chainhash.Hash, int32) { return c.header, c.height }

func TestLocalWalletPilotTemplateReadiness(t *testing.T) {
	for _, test := range []struct {
		name   string
		change func(*pilotGateChain, *[]pilotTemplatePeer)
		want   bool
	}{
		{"idle-synced", func(*pilotGateChain, *[]pilotTemplatePeer) {}, true},
		{"no-peers", func(_ *pilotGateChain, p *[]pilotTemplatePeer) { *p = nil }, false},
		{"handshake", func(_ *pilotGateChain, p *[]pilotTemplatePeer) { (*p)[0].ready = false }, false},
		{"public-peer", func(_ *pilotGateChain, p *[]pilotTemplatePeer) { (*p)[0].address = "192.0.2.1:29333" }, false},
		{"hostname", func(_ *pilotGateChain, p *[]pilotTemplatePeer) { (*p)[0].address = "localhost:29333" }, false},
		{"peer-ahead", func(_ *pilotGateChain, p *[]pilotTemplatePeer) { (*p)[0].height = 62 }, false},
		{"negative-peer-height", func(_ *pilotGateChain, p *[]pilotTemplatePeer) { (*p)[0].height = -1 }, false},
		{"headers-ahead", func(c *pilotGateChain, _ *[]pilotTemplatePeer) { c.height++ }, false},
		{"header-fork", func(c *pilotGateChain, _ *[]pilotTemplatePeer) { c.header[0]++ }, false},
		{"tip-moved", func(c *pilotGateChain, _ *[]pilotTemplatePeer) { c.after = &blockchain.BestState{Height: 62} }, false},
		{"tip-reorg", func(c *pilotGateChain, _ *[]pilotTemplatePeer) {
			c.after = &blockchain.BestState{Height: 61, Hash: chainhash.Hash{2}}
		}, false},
		{"mixed-peers", func(_ *pilotGateChain, p *[]pilotTemplatePeer) {
			*p = append(*p, pilotTemplatePeer{ready: true, address: "192.0.2.2:29333", height: 61})
		}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			chain := &pilotGateChain{best: &blockchain.BestState{Height: 61, Hash: chainhash.Hash{1}}, header: chainhash.Hash{1}, height: 61}
			peers := []pilotTemplatePeer{{address: "127.0.0.1:29334", height: 61, ready: true}}
			test.change(chain, &peers)
			if got := localPilotTemplateCurrent(chain, peers); got != test.want {
				t.Fatalf("got %v want %v", got, test.want)
			}
		})
	}
}

type pilotGateConnections struct {
	rpcserverConnManager
	count     int32
	peers     []rpcserverPeer
	connected []string
}

func (c *pilotGateConnections) ConnectedCount() int32           { return c.count }
func (c *pilotGateConnections) ConnectedPeers() []rpcserverPeer { return c.peers }
func (c *pilotGateConnections) Connect(addr string, permanent bool) error {
	c.connected = append(c.connected, addr)
	return nil
}

type pilotGateSync struct {
	rpcserverSyncManager
	current bool
	calls   int
}

func (s *pilotGateSync) IsCurrent() bool { s.calls++; return s.current }

type pilotGatePeer struct {
	rpcserverPeer
	p *peer.Peer
}

func (p pilotGatePeer) ToPeer() *peer.Peer { return p.p }

func TestLocalWalletPilotPreservesNormalTemplateGate(t *testing.T) {
	chain := &pilotGateChain{best: &blockchain.BestState{Height: 61}}
	conn := &pilotGateConnections{count: 1}
	sync := &pilotGateSync{}
	requireRPCErrorCode(t, checkTemplateSyncState(&config{}, chain, conn, sync), btcjson.ErrRPCClientInInitialDownload)
	if sync.calls != 1 {
		t.Fatal("normal node did not consult currentness")
	}
	sync.current = true
	if err := checkTemplateSyncState(&config{}, chain, conn, sync); err != nil {
		t.Fatal(err)
	}
	conn.count = 0
	requireRPCErrorCode(t, checkTemplateSyncState(&config{}, chain, conn, sync), btcjson.ErrRPCClientNotConnected)
	conn.count = 1
	chain.best.Height = 0
	sync.current = false
	calls := sync.calls
	if err := checkTemplateSyncState(&config{}, chain, conn, sync); err != nil {
		t.Fatal(err)
	}
	if sync.calls != calls {
		t.Fatal("normal genesis behavior changed")
	}
	pilot := &config{LocalWalletPilot: true}
	requireRPCErrorCode(t, checkTemplateSyncState(pilot, chain, conn, sync), btcjson.ErrRPCClientInInitialDownload)
	p, err := peer.NewOutboundPeer(&peer.Config{}, "127.0.0.1:29334")
	if err != nil {
		t.Fatal(err)
	}
	conn.peers = []rpcserverPeer{pilotGatePeer{p: p}}
	requireRPCErrorCode(t, checkTemplateSyncState(pilot, chain, conn, sync), btcjson.ErrRPCClientInInitialDownload)
	if sync.calls != calls {
		t.Fatal("pilot exception delegated to age heuristic")
	}
}

func TestLocalWalletPilotRPCConnectionIsolation(t *testing.T) {
	previous := cfg
	cfg = &config{LocalWalletPilot: true}
	t.Cleanup(func() { cfg = previous })
	conn := &pilotGateConnections{}
	s := &rpcServer{cfg: rpcserverConfig{ConnMgr: conn, ChainParams: &chaincfg.MainNetParams}}
	for _, target := range []string{"192.0.2.1:29333", "localhost:29333", "[::]:29333", "example.invalid:29333"} {
		for _, subcommand := range []btcjson.AddNodeSubCmd{"add", "onetry"} {
			_, err := handleAddNode(s, btcjson.NewAddNodeCmd(target, subcommand), nil)
			requireRPCErrorCode(t, err, btcjson.ErrRPCInvalidParameter)
		}
		_, err := handleNode(s, &btcjson.NodeCmd{SubCmd: "connect", Target: target}, nil)
		requireRPCErrorCode(t, err, btcjson.ErrRPCInvalidParameter)
	}
	if len(conn.connected) != 0 {
		t.Fatal("unsafe RPC reached connection manager")
	}
	if _, err := handleAddNode(s, btcjson.NewAddNodeCmd("127.0.0.1:29334", "onetry"), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := handleNode(s, &btcjson.NodeCmd{SubCmd: "connect", Target: "[::1]:29335"}, nil); err != nil {
		t.Fatal(err)
	}
	if len(conn.connected) != 2 {
		t.Fatal("loopback RPC connections rejected")
	}
	cfg.LocalWalletPilot = false
	if _, err := handleAddNode(s, btcjson.NewAddNodeCmd("example.invalid:29333", "onetry"), nil); err != nil {
		t.Fatal(err)
	}
	if len(conn.connected) != 3 {
		t.Fatal("normal connection RPC behavior changed")
	}
}
