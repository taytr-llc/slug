// Copyright (c) 2026 The SLUG developers. ISC license.
package seednode

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"math"

	"github.com/taytr-llc/slug/btcutil/v2"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/wire/v2"
)

func (p *ResultPublisher) verifyAnchor(ctx context.Context, a Anchor) error {
	if a.Height > math.MaxInt32 || !hexValue(a.Hash, 32, true) || !hexValue(a.Header, 80, false) || !hexValue(a.TxID, 32, true) || !hexValue(a.RawTxDigest, 32, true) || !hexValue(a.Payload, 73, false) || a.RawTxSize == 0 || a.RawTxSize > MaxTransactionSize || a.TxCount == 0 || a.TxCount > MaxBlockSize/10 || a.TxIndex >= a.TxCount || a.OutputIndex != 0 || len(a.MerkleBranch) > 20 {
		return ErrInvalid
	}
	for _, h := range a.MerkleBranch {
		if !hexValue(h, 32, false) {
			return ErrInvalid
		}
	}
	payload, _ := hex.DecodeString(a.Payload)
	var carrier [73]byte
	copy(carrier[:], payload)
	if !validResultCarrier(carrier) {
		return ErrInvalid
	}
	block, err := p.block(ctx, a.Height)
	if err != nil {
		return err
	}
	if headerHex(&block.Header) != a.Header || uint32(len(block.Transactions)) != a.TxCount {
		return ErrUnavailable
	}
	tx := block.Transactions[a.TxIndex]
	if tx.TxHash().String() != a.TxID || len(tx.TxIn) != 1 || len(tx.TxOut) != 2 || tx.TxOut[1].Value < MinimumChangeSatoshis || tx.TxOut[1].Value > btcutil.MaxSatoshi-FeeSatoshis {
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
	// The fixed fee and change determine the committed input amount. This
	// verifies the purpose-specific signature, not a separate UTXO history.
	_, script, _ := resultFundingKey()
	coin := FundingCoin{Outpoint: Outpoint{TxID: tx.TxIn[0].PreviousOutPoint.Hash.String(), Index: tx.TxIn[0].PreviousOutPoint.Index}, ValueSatoshis: tx.TxOut[1].Value + FeeSatoshis, Script: hex.EncodeToString(script), BlockHeight: a.Height, BlockHash: a.Hash}
	if _, err = validateResultTransaction(carrier, coin, raw.Bytes()); err != nil {
		return ErrUnavailable
	}
	branch := merkleBranch(block.Transactions, a.TxIndex)
	if len(branch) != len(a.MerkleBranch) {
		return ErrUnavailable
	}
	for i, h := range branch {
		if h != a.MerkleBranch[i] {
			return ErrUnavailable
		}
	}
	if hash, e := p.owner.canonical(ctx, a.Height); e != nil || hash != a.Hash {
		return ErrUnavailable
	}
	return nil
}

func resultCheckpoint(identity ChainIdentity, anchor Anchor) (chaincfg.SignedCheckpoint, error) {
	genesis, err := chainhash.NewHashFromStr(identity.Genesis)
	if err != nil {
		return chaincfg.SignedCheckpoint{}, ErrInvalid
	}
	hash, err := chainhash.NewHashFromStr(anchor.Hash)
	if err != nil || anchor.Height > math.MaxInt32 {
		return chaincfg.SignedCheckpoint{}, ErrInvalid
	}
	checkpoint := chaincfg.SignedCheckpoint{Version: 1, Network: wire.BitcoinNet(identity.Network), GenesisHash: *genesis, Height: int32(anchor.Height), Hash: *hash}
	message, err := checkpoint.SigningBytes()
	if err != nil {
		return chaincfg.SignedCheckpoint{}, ErrInvalid
	}
	// Public candidate fixture domain; no operator or wallet key is read.
	seed := sha256.Sum256([]byte("SLUG candidate result checkpoint fixture\x00v1\x00"))
	checkpoint.Signature = ed25519.Sign(ed25519.NewKeyFromSeed(seed[:]), message)
	return checkpoint, nil
}

func (p *ResultPublisher) finalMembership(ctx context.Context, anchor Anchor, tip Tip) error {
	for _, ref := range []HeaderRef{{Height: anchor.Height, Hash: anchor.Hash}, {Height: tip.Height, Hash: tip.Hash}} {
		if h, err := p.owner.canonical(ctx, ref.Height); err != nil || h != ref.Hash {
			return ErrUnavailable
		}
	}
	if ctx.Err() != nil {
		return ErrOffline
	}
	return nil
}

func (p *ResultPublisher) AttestInclusion(ctx context.Context, anchor Anchor) (chaincfg.SignedCheckpoint, Tip, error) {
	var empty chaincfg.SignedCheckpoint
	c, end, err := p.begin(ctx)
	if err != nil {
		return empty, Tip{}, err
	}
	defer end()
	if _, err = p.owner.observe(c); err != nil {
		return empty, Tip{}, err
	}
	if err = p.verifyAnchor(c, anchor); err != nil {
		return empty, Tip{}, err
	}
	tip, err := p.owner.observe(c)
	if err != nil {
		return empty, Tip{}, err
	}
	if tip.Height < anchor.Height || tip.Height-anchor.Height < 60 {
		return empty, Tip{}, ErrNotMature
	}
	if err = p.finalMembership(c, anchor, tip); err != nil {
		return empty, Tip{}, err
	}
	if err = p.finish(c); err != nil {
		return empty, Tip{}, err
	}
	checkpoint, err := resultCheckpoint(p.owner.identity, anchor)
	if err != nil {
		return empty, Tip{}, err
	}
	if err = p.finish(c); err != nil {
		return empty, Tip{}, err
	}
	return checkpoint, tip, nil
}
