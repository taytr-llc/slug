// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettlenode

import (
	"bytes"
	"encoding/hex"
	"github.com/taytr-llc/slug/address/v2"
	"github.com/taytr-llc/slug/btcec/v2"
	"github.com/taytr-llc/slug/chaincfg/v2"
)

type FeePolicy struct {
	Mode               string `json:"mode"` // node or fixed; no automatic fallback
	ConfirmationTarget uint32 `json:"confirmation_target"`
	AtomsPerKVB        int64  `json:"atoms_per_kvb"` // fixed mode only
	MaxAtomsPerKVB     int64  `json:"max_atoms_per_kvb"`
	MaxFeeAtoms        int64  `json:"max_fee_atoms"`
}
type Config struct {
	Version       uint32    `json:"version"`
	Network       uint32    `json:"network"`
	Genesis       string    `json:"genesis"`
	PoolPublicKey string    `json:"pool_public_key"`
	PoolScript    string    `json:"pool_script"`
	Fee           FeePolicy `json:"fee"`
}

func ValidateConfig(c Config) error {
	if c.Version != 1 || c.Network != uint32(chaincfg.MainNetParams.Net) || c.Genesis != chaincfg.MainNetParams.GenesisHash.String() || len(c.PoolPublicKey) != 66 || len(c.PoolScript) != 44 {
		return ErrInvalid
	}
	raw, err := hex.DecodeString(c.PoolPublicKey)
	if err != nil || hex.EncodeToString(raw) != c.PoolPublicKey {
		return ErrInvalid
	}
	pub, err := btcec.ParsePubKey(raw)
	if err != nil || !bytes.Equal(pub.SerializeCompressed(), raw) {
		return ErrInvalid
	}
	expected := append([]byte{0, 20}, address.Hash160(raw)...)
	if hex.EncodeToString(expected) != c.PoolScript {
		return ErrInvalid
	}
	f := c.Fee
	// Finite operational ceilings, not a miner fee or an implicit quote.
	if f.MaxAtomsPerKVB < 1 || f.MaxAtomsPerKVB > 1000000000 || f.MaxFeeAtoms < 1 || f.MaxFeeAtoms > 21000000*100000000 {
		return ErrInvalid
	}
	switch f.Mode {
	case "node":
		if f.AtomsPerKVB != 0 || f.ConfirmationTarget < 1 || f.ConfirmationTarget > 25 {
			return ErrInvalid
		}
	case "fixed":
		if f.ConfirmationTarget != 0 || f.AtomsPerKVB < 1 || f.AtomsPerKVB > f.MaxAtomsPerKVB {
			return ErrInvalid
		}
	default:
		return ErrInvalid
	}
	return nil
}
