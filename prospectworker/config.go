// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package prospectworker

import (
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/address/v2"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/wire/v2"
	"strings"
)

func validateConfig(config Config) (Config, error) {
	if wire.TestPoW {
		return Config{}, errors.New("automatic mining requires production PoW; slugtestpow is unsupported")
	}
	if config.Version != Version1 || config.MaxRecords == 0 || config.MaxRecords > 1000000 || config.MaxBytes < 64<<20 || config.MaxBytes > 1<<40 {
		return Config{}, errors.New("invalid worker version or durable capacity")
	}
	encoded, _ := json.Marshal(config.Policy)
	policy, err := prospectnode.ParsePolicy(encoded)
	if err != nil {
		return Config{}, errors.New("invalid worker policy")
	}
	config.Policy = policy
	ctx := prospect.Context{Version: 1, Network: policy.Network, Genesis: policy.Genesis}
	if _, err := gymauth.GymID(ctx, config.PublicKey); err != nil {
		return Config{}, errors.New("invalid worker public key")
	}
	config.PublicKey = strings.ToLower(config.PublicKey)
	destination, err := address.DecodeAddress(config.PayoutAddress, &chaincfg.MainNetParams)
	if err != nil || !destination.IsForNet(&chaincfg.MainNetParams) {
		return Config{}, errors.New("invalid worker payout address")
	}
	switch destination.(type) {
	case *address.AddressPubKeyHash, *address.AddressScriptHash, *address.AddressWitnessPubKeyHash, *address.AddressWitnessScriptHash, *address.AddressTaproot:
	default:
		return Config{}, errors.New("unsupported worker payout address")
	}
	config.PayoutAddress = destination.EncodeAddress()
	// Reuse the transport's origin normalization without creating a client or
	// making a request. This placeholder is public syntax validation, not a key.
	raw, _ := json.Marshal(prospectservice.ClientConfig{Version: 2, URL: config.AuthorityOrigin, Token: strings.Repeat("00", 32), CACertificate: ""})
	transport, err := prospectservice.ParseClientConfig(raw)
	if err != nil {
		return Config{}, errors.New("invalid worker authority origin")
	}
	config.AuthorityOrigin = strings.TrimRight(transport.URL, "/")
	return config, nil
}
