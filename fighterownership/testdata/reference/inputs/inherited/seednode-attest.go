// Copyright (c) 2026 The SLUG developers. ISC license.
package seednode

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"math"

	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/wire/v2"
)

func merkleBranch(txs []*wire.MsgTx, index uint32) []string {
	level := make([]chainhash.Hash, len(txs))
	for i, tx := range txs {
		level[i] = tx.TxHash()
	}
	branch := []string{}
	for len(level) > 1 {
		sibling := index ^ 1
		if sibling >= uint32(len(level)) {
			sibling = index
		}
		branch = append(branch, level[sibling].String())
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
		index /= 2
		level = next
	}
	return branch
}

func (p *Publisher) verifyAnchor(ctx context.Context, a Anchor) error {
	if a.Height >= math.MaxInt32 || !hexValue(a.Hash, 32, true) || !hexValue(a.Header, 80, false) || !hexValue(a.TxID, 32, true) || !hexValue(a.RawTxDigest, 32, true) || !hexValue(a.Payload, 73, false) || a.RawTxSize > MaxBlockSize || a.TxCount == 0 || a.TxCount > MaxBlockSize/10 || a.TxIndex >= a.TxCount || len(a.MerkleBranch) > 20 {
		return ErrInvalid
	}
	payload, _ := hex.DecodeString(a.Payload)
	var carrier [73]byte
	copy(carrier[:], payload)
	if !validCarrier(carrier) {
		return ErrInvalid
	}
	block, e := p.block(ctx, a.Height)
	if e != nil {
		return e
	}
	if headerHex(&block.Header) != a.Header || uint32(len(block.Transactions)) != a.TxCount {
		return ErrUnavailable
	}
	tx := block.Transactions[a.TxIndex]
	if tx.TxHash().String() != a.TxID || uint64(a.OutputIndex) >= uint64(len(tx.TxOut)) {
		return ErrUnavailable
	}
	out := tx.TxOut[a.OutputIndex]
	if !bytes.Equal(out.PkScript, carrierScript(carrier)) {
		return ErrUnavailable
	}
	var raw bytes.Buffer
	if tx.Serialize(&raw) != nil {
		return ErrUnavailable
	}
	digest := sha256.Sum256(raw.Bytes())
	if uint32(raw.Len()) != a.RawTxSize || hex.EncodeToString(digest[:]) != a.RawTxDigest {
		return ErrUnavailable
	}
	branch := merkleBranch(block.Transactions, a.TxIndex)
	if len(branch) != len(a.MerkleBranch) {
		return ErrUnavailable
	}
	for i, v := range branch {
		if v != a.MerkleBranch[i] {
			return ErrUnavailable
		}
	}
	if hash, e := p.canonical(ctx, a.Height); e != nil || hash != a.Hash {
		return ErrUnavailable
	}
	return nil
}

func (p *Publisher) AttestTarget(ctx context.Context, anchor Anchor, target HeaderRef) (chaincfg.SignedCheckpoint, Tip, error) {
	var empty chaincfg.SignedCheckpoint
	c, end, e := p.begin(ctx)
	if e != nil {
		return empty, Tip{}, e
	}
	defer end()
	if anchor.Height >= math.MaxInt32 || target.Height != anchor.Height+1 {
		return empty, Tip{}, ErrInvalid
	}
	if _, e = p.observe(c); e != nil {
		return empty, Tip{}, e
	}
	if e = p.verifyAnchor(c, anchor); e != nil {
		return empty, Tip{}, e
	}
	header, e := p.header(c, target)
	if e != nil {
		return empty, Tip{}, e
	}
	if header.PrevBlock.String() != anchor.Hash {
		return empty, Tip{}, ErrUnavailable
	}
	tip, e := p.observe(c)
	if e != nil {
		return empty, Tip{}, e
	}
	if tip.Height < target.Height || tip.Height-target.Height < 60 {
		return empty, Tip{}, ErrNotMature
	}
	if e := p.finalMembership(c, anchor, target, tip); e != nil {
		return empty, Tip{}, e
	}
	if e := p.checkOwnerContext(c); e != nil {
		return empty, Tip{}, e
	}
	genesis, _ := chainhash.NewHashFromStr(p.identity.Genesis)
	hash, _ := chainhash.NewHashFromStr(target.Hash)
	checkpoint := chaincfg.SignedCheckpoint{Version: 1, Network: wire.BitcoinNet(p.identity.Network), GenesisHash: *genesis, Height: int32(target.Height), Hash: *hash}
	message, e := checkpoint.SigningBytes()
	if e != nil {
		return empty, Tip{}, ErrInvalid
	}
	// Published candidate fixture only; never read an operator key or key file.
	seed := sha256.Sum256([]byte("SLUG candidate seed checkpoint fixture\x00v1\x00"))
	key := ed25519.NewKeyFromSeed(seed[:])
	checkpoint.Signature = ed25519.Sign(key, message)
	if c.Err() != nil {
		return empty, Tip{}, ErrOffline
	}
	return checkpoint, tip, nil
}

// The depth witness is the captured tip, not merely membership of A and A+1.
// An ordinary extension preserves it; a replacement above the target does not.
func (p *Publisher) finalMembership(ctx context.Context, anchor Anchor, target HeaderRef, tip Tip) error {
	for _, ref := range []HeaderRef{{Height: anchor.Height, Hash: anchor.Hash}, target, {Height: tip.Height, Hash: tip.Hash}} {
		if h, err := p.canonical(ctx, ref.Height); err != nil || h != ref.Hash {
			return ErrUnavailable
		}
	}
	if ctx.Err() != nil {
		return ErrOffline
	}
	return nil
}
