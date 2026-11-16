package pilot

import (
	"errors"
	"testing"

	"github.com/taytr-llc/slug/btcjson"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/wire/v2"
)

type localChainFixture struct {
	info                                *btcjson.GetBlockChainInfoResult
	peers                               []btcjson.GetPeerInfoResult
	network                             wire.BitcoinNet
	genesis                             chainhash.Hash
	best                                chainhash.Hash
	height                              int32
	fail                                string
	calls                               map[string]int
	networkFlip, genesisFlip, peersFlip bool
}

func localChainState() *localChainFixture {
	best := chainhash.Hash{1}
	return &localChainFixture{network: chaincfg.MainNetParams.Net, genesis: *chaincfg.MainNetParams.GenesisHash, best: best, height: 1, peers: []btcjson.GetPeerInfoResult{{Addr: "127.0.0.1:29333"}}, calls: map[string]int{}, info: &btcjson.GetBlockChainInfoResult{Chain: "mainnet", Blocks: 1, Headers: 1, BestBlockHash: best.String(), MedianTime: 1, SoftForks: &btcjson.SoftForks{Bip9SoftForks: map[string]*btcjson.Bip9SoftForkDescription{"csv": {Status: "failed", AlwaysActiveHeight: 62}, "segwit": {Status: "failed", AlwaysActiveHeight: 62}, "taproot": {Status: "failed", AlwaysActiveHeight: 62}}}}}
}
func (f *localChainFixture) call(name string) error {
	f.calls[name]++
	if f.fail == name {
		return errors.New("fixture transport failure")
	}
	return nil
}
func (f *localChainFixture) GetCurrentNet() (wire.BitcoinNet, error) {
	err := f.call("network")
	if f.networkFlip && f.calls["network"] > 1 {
		return 0, err
	}
	return f.network, err
}
func (f *localChainFixture) GetBlockHash(int64) (*chainhash.Hash, error) {
	err := f.call("genesis")
	if f.genesisFlip && f.calls["genesis"] > 1 {
		return &chainhash.Hash{}, err
	}
	return &f.genesis, err
}
func (f *localChainFixture) GetBlockChainInfo() (*btcjson.GetBlockChainInfoResult, error) {
	return f.info, f.call("info")
}
func (f *localChainFixture) GetBestBlock() (*chainhash.Hash, int32, error) {
	return &f.best, f.height, f.call("best")
}
func (f *localChainFixture) GetPeerInfo() ([]btcjson.GetPeerInfoResult, error) {
	err := f.call("peers")
	if f.peersFlip && f.calls["peers"] > 1 {
		return []btcjson.GetPeerInfoResult{{Addr: "203.0.113.1:29333"}}, err
	}
	return f.peers, err
}
func TestOwnedLocalChainReadiness(t *testing.T) {
	if wire.TestPoW {
		if localChainReady(localChainState()) {
			t.Fatal("cheap profile accepted")
		}
		return
	}
	f := localChainState()
	if !localChainReady(f) {
		t.Fatal("old isolated private tip was not scannable")
	}
	f = localChainState()
	f.height = 61
	f.info.Blocks = 61
	f.info.Headers = 61
	for _, rule := range f.info.Bip9SoftForks {
		rule.Status = "active"
	}
	if !localChainReady(f) {
		t.Fatal("activation boundary rejected")
	}
	for _, tc := range []struct {
		name   string
		change func(*localChainFixture)
	}{
		{"network", func(f *localChainFixture) { f.network = 0 }},
		{"genesis", func(f *localChainFixture) { f.genesis = chainhash.Hash{} }},
		{"chain", func(f *localChainFixture) { f.info.Chain = "testnet" }},
		{"missing-rules", func(f *localChainFixture) { f.info.SoftForks = nil }},
		{"old-node", func(f *localChainFixture) { f.info.Bip9SoftForks["segwit"].AlwaysActiveHeight = 0 }},
		{"early-active", func(f *localChainFixture) { f.info.Bip9SoftForks["taproot"].Status = "active" }},
		{"unknown-status", func(f *localChainFixture) { f.info.Bip9SoftForks["csv"].Status = "unknown" }},
		{"headers-ahead", func(f *localChainFixture) { f.info.Headers = 2 }},
		{"changed-tip", func(f *localChainFixture) { f.height = 2 }},
		{"changed-hash", func(f *localChainFixture) { f.best = chainhash.Hash{2} }},
		{"no-peers", func(f *localChainFixture) { f.peers = nil }},
		{"public-peer", func(f *localChainFixture) { f.peers[0].Addr = "203.0.113.1:29333" }},
		{"hostname-peer", func(f *localChainFixture) { f.peers[0].Addr = "localhost:29333" }},
		{"zero-port", func(f *localChainFixture) { f.peers[0].Addr = "127.0.0.1:0" }},
		{"identity-flip", func(f *localChainFixture) { f.networkFlip = true }},
		{"genesis-flip", func(f *localChainFixture) { f.genesisFlip = true }},
		{"peer-flip", func(f *localChainFixture) { f.peersFlip = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := localChainState()
			tc.change(f)
			if localChainReady(f) {
				t.Fatal("unsafe snapshot accepted")
			}
		})
	}
	for _, method := range []string{"network", "genesis", "info", "best", "peers"} {
		t.Run("failure-"+method, func(t *testing.T) {
			f := localChainState()
			f.fail = method
			if localChainReady(f) {
				t.Fatal("RPC failure bypassed")
			}
		})
	}
}
