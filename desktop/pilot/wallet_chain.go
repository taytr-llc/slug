package pilot

import (
	"net"
	"strconv"

	"github.com/taytr-llc/slug/btcjson"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/wire/v2"
	"github.com/taytr-llc/slugwallet/chain"
)

// ownedLocalChain changes only the wallet's readiness-to-scan heuristic for
// the two isolated, owned pilot nodes. An old valid tip is the entire private
// chain; requiring a recent timestamp would prevent both a backup scan and
// the miner responsible for advancing it from starting. Full node validation,
// rescan completion, chain identity and activation checks remain required.
// This type is private to the fixture desktop, never a general wallet option.
type ownedLocalChain struct{ *chain.RPCClient }

var _ chain.Interface = (*ownedLocalChain)(nil)

func (c *ownedLocalChain) IsCurrent() bool { return localChainReady(c.RPCClient) }

type localChainReader interface {
	GetCurrentNet() (wire.BitcoinNet, error)
	GetBlockHash(int64) (*chainhash.Hash, error)
	GetBlockChainInfo() (*btcjson.GetBlockChainInfoResult, error)
	GetBestBlock() (*chainhash.Hash, int32, error)
	GetPeerInfo() ([]btcjson.GetPeerInfoResult, error)
}

func localChainReady(node localChainReader) bool {
	if wire.TestPoW {
		return false
	}
	network, err := node.GetCurrentNet()
	if err != nil || network != chaincfg.MainNetParams.Net {
		return false
	}
	genesisHash, err := node.GetBlockHash(0)
	if err != nil || genesisHash == nil || !genesisHash.IsEqual(chaincfg.MainNetParams.GenesisHash) {
		return false
	}
	info, err := node.GetBlockChainInfo()
	if err != nil || info == nil || info.Chain != "mainnet" || info.Blocks < 0 || info.Headers != info.Blocks || info.SoftForks == nil {
		return false
	}
	for _, name := range []string{"csv", "segwit", "taproot"} {
		rule := info.Bip9SoftForks[name]
		if rule == nil || rule.AlwaysActiveHeight != chaincfg.PhaseAActivationHeight {
			return false
		}
		switch rule.Status {
		case "defined", "started", "locked_in", "active", "failed":
		default:
			return false
		}
		if (rule.Status == "active") != (uint64(info.Blocks)+1 >= chaincfg.PhaseAActivationHeight) {
			return false
		}
	}
	peers, err := node.GetPeerInfo()
	if err != nil || !onlyPilotPeers(peers) {
		return false
	}
	best, height, err := node.GetBestBlock()
	if err != nil || best == nil || height != info.Blocks || best.String() != info.BestBlockHash {
		return false
	}
	network, err = node.GetCurrentNet()
	if err != nil || network != chaincfg.MainNetParams.Net {
		return false
	}
	genesisHash, err = node.GetBlockHash(0)
	if err != nil || genesisHash == nil || !genesisHash.IsEqual(chaincfg.MainNetParams.GenesisHash) {
		return false
	}
	peers, err = node.GetPeerInfo()
	return err == nil && onlyPilotPeers(peers)
}
func onlyPilotPeers(peers []btcjson.GetPeerInfoResult) bool {
	if len(peers) == 0 {
		return false
	}
	for _, peer := range peers {
		host, port, err := net.SplitHostPort(peer.Addr)
		if err != nil || host != "127.0.0.1" {
			return false
		}
		number, err := strconv.ParseUint(port, 10, 16)
		if err != nil || number == 0 {
			return false
		}
	}
	return true
}
