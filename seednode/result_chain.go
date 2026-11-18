// Copyright (c) 2026 The SLUG developers. ISC license.
package seednode

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"math"

	"github.com/taytr-llc/slug/blockchain"
	"github.com/taytr-llc/slug/btcutil/v2"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/wire/v2"
)

// validateResultBody adds only result-purpose structural and witness checks.
// It is not a full UTXO/consensus validation of an imported chain.
func validateResultBody(block *wire.MsgBlock) error {
	if block == nil || len(block.Transactions) == 0 || len(block.Transactions) > MaxBlockSize/10 {
		return ErrUnavailable
	}
	for i, tx := range block.Transactions {
		if tx == nil {
			return ErrUnavailable
		}
		for _, in := range tx.TxIn {
			if in == nil {
				return ErrUnavailable
			}
		}
		for _, out := range tx.TxOut {
			if out == nil {
				return ErrUnavailable
			}
		}
		if blockchain.IsCoinBaseTx(tx) != (i == 0) {
			return ErrUnavailable
		}
	}
	root, err := merkleRoot(block.Transactions)
	if err != nil || root != block.Header.MerkleRoot || blockchain.ValidateWitnessCommitment(btcutil.NewBlock(block)) != nil {
		return ErrUnavailable
	}
	return nil
}

// blockByHash retains no decoded bodies. The shared header-work map charges
// attempted normal Argon hashes, including failed comparisons, up to eight.
func (p *ResultPublisher) blockByHash(ctx context.Context, ref HeaderRef) (*wire.MsgBlock, []byte, error) {
	if ref.Height > math.MaxInt32 || !hexValue(ref.Hash, 32, true) || ref.Header != "" && !hexValue(ref.Header, 80, false) {
		return nil, nil, ErrInvalid
	}
	if len(p.owner.checked) >= MaxStepBlocks {
		known := false
		for _, hash := range p.owner.checked {
			known = known || hash == ref.Hash
		}
		if !known {
			return nil, nil, ErrCapacity
		}
	}
	raw, err := p.owner.nodes[0].call(ctx, "getblock", []any{ref.Hash, 0})
	if err != nil {
		return nil, nil, err
	}
	if len(raw) < 2 || len(raw) > 2*MaxBlockSize+2 || raw[0] != '"' || raw[len(raw)-1] != '"' {
		return nil, nil, ErrUnavailable
	}
	value := string(raw[1 : len(raw)-1])
	if len(value)%2 != 0 || !hexValue(value, len(value)/2, false) {
		return nil, nil, ErrUnavailable
	}
	data, err := hex.DecodeString(value)
	if err != nil {
		return nil, nil, ErrUnavailable
	}
	r := bytes.NewReader(data)
	block := new(wire.MsgBlock)
	if block.Deserialize(r) != nil || r.Len() != 0 {
		return nil, nil, ErrUnavailable
	}
	if err = p.owner.verifyHeader(ctx, &block.Header, ref.Hash); err != nil {
		return nil, nil, err
	}
	// Check the target using the already verified hash, without another Argon.
	target := blockchain.CompactToBig(block.Header.Bits)
	hash, _ := chainhash.NewHashFromStr(ref.Hash)
	if target.Sign() <= 0 || target.Cmp(chaincfg.MainNetParams.PowLimit) > 0 || blockchain.HashToBig(hash).Cmp(target) > 0 {
		return nil, nil, ErrUnavailable
	}
	if ref.Header != "" && headerHex(&block.Header) != ref.Header {
		return nil, nil, ErrUnavailable
	}
	if (ref.Height == 0) != (block.Header.PrevBlock == (chainhash.Hash{})) || ref.Height == 0 && ref.Hash != p.owner.identity.Genesis {
		return nil, nil, ErrUnavailable
	}
	if err = validateResultBody(block); err != nil {
		return nil, nil, err
	}
	var encoded bytes.Buffer
	if block.Serialize(&encoded) != nil || !bytes.Equal(data, encoded.Bytes()) {
		return nil, nil, ErrUnavailable
	}
	if ctx.Err() != nil {
		return nil, nil, ErrOffline
	}
	return block, data, nil
}

func (p *ResultPublisher) block(ctx context.Context, height uint32) (*wire.MsgBlock, error) {
	hash, err := p.owner.canonical(ctx, height)
	if err != nil {
		return nil, err
	}
	block, _, err := p.blockByHash(ctx, HeaderRef{Height: height, Hash: hash})
	if err != nil {
		return nil, err
	}
	if height > 0 {
		parent, e := p.owner.canonical(ctx, height-1)
		if e != nil || parent != block.Header.PrevBlock.String() {
			return nil, ErrUnavailable
		}
	}
	if final, e := p.owner.canonical(ctx, height); e != nil || final != hash {
		return nil, ErrUnavailable
	}
	return block, nil
}

func (p *ResultPublisher) Block(ctx context.Context, height uint32) (*wire.MsgBlock, error) {
	c, end, err := p.begin(ctx)
	if err != nil {
		return nil, err
	}
	defer end()
	block, err := p.block(c, height)
	if err != nil {
		return nil, err
	}
	if err = p.finish(c); err != nil {
		return nil, err
	}
	return block, nil
}

// ExportHeader returns structurally checked historical evidence by hash. It
// makes no normal-PoW or canonical-main-branch claim for header-only bytes.
func (p *ResultPublisher) ExportHeader(ctx context.Context, ref HeaderRef) (HeaderRef, error) {
	c, end, err := p.begin(ctx)
	if err != nil {
		return HeaderRef{}, err
	}
	defer end()
	if ref.Height > math.MaxInt32 || !hexValue(ref.Hash, 32, true) || ref.Header != "" && !hexValue(ref.Header, 80, false) {
		return HeaderRef{}, ErrInvalid
	}
	raw, err := p.owner.nodes[0].call(c, "getblockheader", []any{ref.Hash, false})
	var encoded string
	if err != nil || json.Unmarshal(raw, &encoded) != nil || !hexValue(encoded, 80, false) || ref.Header != "" && ref.Header != encoded {
		return HeaderRef{}, ErrUnavailable
	}
	data, _ := hex.DecodeString(encoded)
	var header wire.BlockHeader
	if header.Deserialize(bytes.NewReader(data)) != nil || headerHex(&header) != encoded || (ref.Height == 0) != (header.PrevBlock == (chainhash.Hash{})) || ref.Height == 0 && ref.Hash != p.owner.identity.Genesis {
		return HeaderRef{}, ErrUnavailable
	}
	if err = p.finish(c); err != nil {
		return HeaderRef{}, err
	}
	ref.Header = encoded
	return ref, nil
}

func (p *ResultPublisher) ExportBlock(ctx context.Context, ref HeaderRef) ([]byte, error) {
	c, end, err := p.begin(ctx)
	if err != nil {
		return nil, err
	}
	defer end()
	_, raw, err := p.blockByHash(c, ref)
	if err != nil {
		return nil, err
	}
	if err = p.finish(c); err != nil {
		return nil, err
	}
	return raw, nil
}
