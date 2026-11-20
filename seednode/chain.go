// Copyright (c) 2026 The SLUG developers. ISC license.
package seednode

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"math"
	"strings"

	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/wire/v2"
)

func (p *Publisher) checkOwner() error {
	return p.checkOwnerContext(p.ctx)
}

func (p *Publisher) checkOwnerContext(ctx context.Context) error {
	if ctx.Err() != nil {
		return ErrOffline
	}
	if !live(p.binding.Primary) || !live(p.binding.Peer) {
		return ErrOffline
	}
	for i, v := range []struct {
		name    string
		process OwnedProcess
	}{{"primary", p.binding.Primary}, {"peer", p.binding.Peer}} {
		c, e := configForMode(p.binding.Home, v.name, v.process, p.shared != nil || p.ownership != nil)
		if e != nil || c != p.configs[i] {
			return ErrOffline
		}
	}
	if p.shared != nil {
		return p.checkSharedOwner(ctx)
	}
	if p.ownership != nil {
		return p.checkOwnershipOwner(ctx)
	}
	return checkMarker(p.binding, p.identity)
}

func (p *Publisher) observe(ctx context.Context) (Tip, error) {
	if e := p.checkOwnerContext(ctx); e != nil {
		return Tip{}, e
	}
	for i, r := range p.nodes {
		raw, e := r.call(ctx, "getcurrentnet", []any{})
		var network uint32
		if e != nil || json.Unmarshal(raw, &network) != nil || network != p.identity.Network {
			return Tip{}, ErrUnavailable
		}
		raw, e = r.call(ctx, "getblockhash", []any{uint32(0)})
		var genesis string
		if e != nil || json.Unmarshal(raw, &genesis) != nil || genesis != p.identity.Genesis {
			return Tip{}, ErrUnavailable
		}
		raw, e = r.call(ctx, "getblockchaininfo", []any{})
		_, e2 := fields(raw, "chain", "blocks", "bestblockhash", "bip9_softforks")
		var info struct {
			Chain       string                     `json:"chain"`
			Blocks      int32                      `json:"blocks"`
			Best        string                     `json:"bestblockhash"`
			Deployments map[string]json.RawMessage `json:"bip9_softforks"`
		}
		if e != nil || e2 != nil || json.Unmarshal(raw, &info) != nil || info.Chain != "mainnet" || info.Blocks < 0 || !hexValue(info.Best, 32, true) {
			return Tip{}, ErrUnavailable
		}
		for _, name := range []string{"csv", "segwit", "taproot"} {
			d, e := fields(info.Deployments[name], "status", "always_active_height")
			var status string
			var height uint32
			if e != nil || json.Unmarshal(d["status"], &status) != nil || json.Unmarshal(d["always_active_height"], &height) != nil || height != 62 || (status == "active") != (int64(info.Blocks)+1 >= 62) {
				return Tip{}, ErrUnavailable
			}
			switch status {
			case "defined", "started", "locked_in", "active", "failed":
			default:
				return Tip{}, ErrUnavailable
			}
		}
		raw, e = r.call(ctx, "getpeerinfo", []any{})
		var peers []json.RawMessage
		if e != nil || json.Unmarshal(raw, &peers) != nil || len(peers) != 1 {
			return Tip{}, ErrOffline
		}
		peer, e := fields(peers[0], "addr", "inbound")
		var addr string
		var inbound bool
		if e != nil || json.Unmarshal(peer["addr"], &addr) != nil || json.Unmarshal(peer["inbound"], &inbound) != nil || !loopback(addr) || inbound != (i == 1) {
			return Tip{}, ErrOffline
		}
		if i == 0 && addr != p.configs[1].listen {
			return Tip{}, ErrOffline
		}
	}
	raw, e := p.nodes[0].call(ctx, "getbestblock", []any{})
	m, e2 := fields(raw, "height", "hash")
	var tip Tip
	if e != nil || e2 != nil || json.Unmarshal(m["height"], &tip.Height) != nil || json.Unmarshal(m["hash"], &tip.Hash) != nil || tip.Height > math.MaxInt32 || !hexValue(tip.Hash, 32, true) {
		return Tip{}, ErrUnavailable
	}
	if hash, e := p.canonical(ctx, tip.Height); e != nil || hash != tip.Hash {
		return Tip{}, ErrUnavailable
	}
	if ctx.Err() != nil || p.checkOwnerContext(ctx) != nil {
		return Tip{}, ErrOffline
	}
	return tip, nil
}

func (p *Publisher) Observe(ctx context.Context) (Tip, error) {
	c, end, e := p.begin(ctx)
	if e != nil {
		return Tip{}, e
	}
	defer end()
	return p.observe(c)
}

func (p *Publisher) canonical(ctx context.Context, height uint32) (string, error) {
	if height > math.MaxInt32 {
		return "", ErrInvalid
	}
	raw, e := p.nodes[0].call(ctx, "getblockhash", []any{height})
	var hash string
	if e != nil || json.Unmarshal(raw, &hash) != nil || !hexValue(hash, 32, true) {
		return "", ErrUnavailable
	}
	return hash, nil
}
func (p *Publisher) CanonicalHash(ctx context.Context, height uint32) (string, error) {
	c, end, e := p.begin(ctx)
	if e != nil {
		return "", e
	}
	defer end()
	hash, e := p.canonical(c, height)
	if e == nil {
		e = p.finishShared(c)
	}
	if e != nil {
		return "", e
	}
	return hash, nil
}

func headerHex(h *wire.BlockHeader) string {
	var b bytes.Buffer
	_ = h.Serialize(&b)
	return hex.EncodeToString(b.Bytes())
}
func (p *Publisher) verifyHeader(ctx context.Context, h *wire.BlockHeader, expected string) error {
	key := headerHex(h)
	if hash, ok := p.checked[key]; ok {
		if hash != expected {
			return ErrUnavailable
		}
		return nil
	}
	if len(p.checked) >= MaxStepBlocks {
		return ErrCapacity
	}
	if ctx.Err() != nil {
		return ErrOffline
	}
	hash := h.BlockHash().String() // One serial normal256MiB workspace.
	// Failed observations consume the same bounded work as successful ones.
	p.checked[key] = hash
	if ctx.Err() != nil {
		return ErrOffline
	}
	if hash != expected {
		return ErrUnavailable
	}
	return nil
}

func merkleRoot(transactions []*wire.MsgTx) (chainhash.Hash, error) {
	if len(transactions) == 0 || len(transactions) > MaxBlockSize/10 {
		return chainhash.Hash{}, ErrUnavailable
	}
	level := make([]chainhash.Hash, len(transactions))
	for i, tx := range transactions {
		if tx == nil {
			return chainhash.Hash{}, ErrUnavailable
		}
		level[i] = tx.TxHash()
	}
	for len(level) > 1 {
		next := make([]chainhash.Hash, (len(level)+1)/2)
		for i := range next {
			left := level[2*i]
			right := left
			if 2*i+1 < len(level) {
				right = level[2*i+1]
			}
			var pair [64]byte
			copy(pair[:32], left[:])
			copy(pair[32:], right[:])
			next[i] = chainhash.DoubleHashH(pair[:])
		}
		level = next
	}
	return level[0], nil
}

func (p *Publisher) block(ctx context.Context, height uint32) (*wire.MsgBlock, error) {
	hash, e := p.canonical(ctx, height)
	if e != nil {
		return nil, e
	}
	if len(p.checked) >= MaxStepBlocks {
		known := false
		for _, value := range p.checked {
			known = known || value == hash
		}
		if !known {
			return nil, ErrCapacity
		}
	}
	raw, e := p.nodes[0].call(ctx, "getblock", []any{hash, 0})
	if e != nil {
		return nil, e
	}
	// Check the encoded token before allocating its decoded block buffer.
	if len(raw) > MaxBlockSize*2+2 || len(raw) < 2 || raw[0] != '"' || raw[len(raw)-1] != '"' {
		return nil, ErrUnavailable
	}
	value := string(raw[1 : len(raw)-1])
	if len(value)%2 != 0 || len(value)/2 > MaxBlockSize || value != strings.ToLower(value) {
		return nil, ErrUnavailable
	}
	data, e := hex.DecodeString(value)
	if e != nil {
		return nil, ErrUnavailable
	}
	reader := bytes.NewReader(data)
	block := new(wire.MsgBlock)
	if block.Deserialize(reader) != nil || reader.Len() != 0 {
		return nil, ErrUnavailable
	}
	if e = p.verifyHeader(ctx, &block.Header, hash); e != nil {
		return nil, e
	}
	root, e := merkleRoot(block.Transactions)
	if e != nil || root != block.Header.MerkleRoot {
		return nil, ErrUnavailable
	}
	if height == 0 {
		if hash != p.identity.Genesis || block.Header.PrevBlock != (chainhash.Hash{}) {
			return nil, ErrUnavailable
		}
	} else {
		parent, e := p.canonical(ctx, height-1)
		if e != nil || parent != block.Header.PrevBlock.String() {
			return nil, ErrUnavailable
		}
	}
	if final, e := p.canonical(ctx, height); e != nil || final != hash {
		return nil, ErrUnavailable
	}
	if ctx.Err() != nil {
		return nil, ErrOffline
	}
	return block, nil
}
func (p *Publisher) Block(ctx context.Context, height uint32) (*wire.MsgBlock, error) {
	c, end, e := p.begin(ctx)
	if e != nil {
		return nil, e
	}
	defer end()
	block, e := p.block(c, height)
	if e == nil {
		e = p.finishShared(c)
	}
	if e != nil {
		return nil, e
	}
	return block, nil
}

func (p *Publisher) header(ctx context.Context, ref HeaderRef) (wire.BlockHeader, error) {
	var h wire.BlockHeader
	if ref.Height > math.MaxInt32 || !hexValue(ref.Hash, 32, true) || !hexValue(ref.Header, 80, false) {
		return h, ErrInvalid
	}
	canonical, e := p.canonical(ctx, ref.Height)
	if e != nil || canonical != ref.Hash {
		return h, ErrUnavailable
	}
	raw, e := p.nodes[0].call(ctx, "getblockheader", []any{ref.Hash, false})
	var encoded string
	if e != nil || json.Unmarshal(raw, &encoded) != nil || encoded != ref.Header {
		return h, ErrUnavailable
	}
	b, _ := hex.DecodeString(encoded)
	if h.Deserialize(bytes.NewReader(b)) != nil {
		return h, ErrUnavailable
	}
	if e = p.verifyHeader(ctx, &h, ref.Hash); e != nil {
		return h, e
	}
	return h, nil
}
