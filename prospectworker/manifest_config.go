// Copyright (c) 2026 The SLUG developers. ISC license.
package prospectworker

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/address/v2"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
	"io"
	"strings"
	"unicode/utf8"
)

func manifestProduction() error {
	if wire.TestPoW {
		return errors.New("manifest mining requires production PoW; slugtestpow is unsupported")
	}
	return nil
}
func manifestHashShape(s string) bool {
	if len(s) != 64 || s != strings.ToLower(s) {
		return false
	}
	_, e := hex.DecodeString(s)
	return e == nil
}

// ValidateManifestConfig is pure canonical public configuration validation.
func ValidateManifestConfig(c ManifestConfig) (ManifestConfig, error) {
	fail := errors.New("invalid manifest worker configuration")
	if c.Version != Version2 || c.MaxRecords == 0 || c.MaxRecords > 1000000 || c.MaxBytes < 64<<20 || c.MaxBytes > 1<<40 || len(c.AuthorityOrigin) > MaxManifestConfigSize || len(c.PublicKey) != 66 || len(c.PayoutAddress) > 128 || !manifestHashShape(c.TransportTrustID) || c.AuthorityPin.Version != registry.Version5 || !manifestHashShape(c.AuthorityPin.AuthorityID) || !manifestHashShape(c.AuthorityPin.DescriptorID) || !manifestHashShape(c.AuthorityPin.Genesis) || len(c.Policy.Genesis) != 64 || len(c.Policy.ProspectTarget) != 64 {
		return ManifestConfig{}, fail
	}
	d, err := fighterdescriptor.ResolveCandidate(c.AuthorityPin.DescriptorID)
	if err != nil {
		return ManifestConfig{}, fail
	}
	raw, _ := json.Marshal(c.Policy)
	p, err := prospectnode.ParsePolicy(raw)
	if err != nil || c.AuthorityPin.Network != p.Network || c.AuthorityPin.Genesis != p.Genesis || d.DNAVersion != p.DNAVersion {
		return ManifestConfig{}, fail
	}
	c.Policy = p
	if _, err := gymauth.GymID(prospect.Context{Version: 1, Network: p.Network, Genesis: p.Genesis}, c.PublicKey); err != nil {
		return ManifestConfig{}, fail
	}
	c.PublicKey = strings.ToLower(c.PublicKey)
	a, err := address.DecodeAddress(c.PayoutAddress, &chaincfg.MainNetParams)
	if err != nil || !a.IsForNet(&chaincfg.MainNetParams) {
		return ManifestConfig{}, fail
	}
	switch a.(type) {
	case *address.AddressPubKeyHash, *address.AddressScriptHash, *address.AddressWitnessPubKeyHash, *address.AddressWitnessScriptHash, *address.AddressTaproot:
	default:
		return ManifestConfig{}, fail
	}
	c.PayoutAddress = a.EncodeAddress()
	// Public placeholder is only syntax validation; no client or request exists.
	transport, err := prospectservice.ValidateManifestClientConfig(prospectservice.ManifestClientConfig{Version: 3, URL: c.AuthorityOrigin, Token: strings.Repeat("00", 32), AuthorityPin: c.AuthorityPin})
	if err != nil {
		return ManifestConfig{}, fail
	}
	c.AuthorityOrigin = transport.URL
	raw, err = json.Marshal(c)
	if err != nil || len(raw) > MaxManifestConfigSize {
		return ManifestConfig{}, fail
	}
	return c, nil
}

// manifestJSONShape bounds containers before allocation-heavy typed decoding.
// Exact field names and required/omitted shape are checked by canonical roundtrip
// for journal rows, or an explicit key tree for the public config parser.
func manifestJSONShape(raw []byte, limit int) error {
	if len(raw) == 0 || len(raw) > limit || !utf8.Valid(raw) {
		return ErrCorrupt
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var visit func(int, string) error
	visit = func(depth int, field string) error {
		if depth > 8 {
			return ErrCorrupt
		}
		tok, err := d.Token()
		if err != nil || tok == nil {
			return ErrCorrupt
		}
		delimiter, ok := tok.(json.Delim)
		if !ok {
			return nil
		}
		switch delimiter {
		case '{':
			seen := map[string]bool{}
			for d.More() {
				tok, e := d.Token()
				k, ok := tok.(string)
				if e != nil || !ok || seen[k] || len(seen) >= 64 {
					return ErrCorrupt
				}
				seen[k] = true
				if visit(depth+1, k) != nil {
					return ErrCorrupt
				}
			}
			tok, err = d.Token()
			if err != nil || tok != json.Delim('}') {
				return ErrCorrupt
			}
		case '[':
			maximum := 0
			switch field {
			case "merkle_branch":
				maximum = 20
			case "transactions", "depends":
				maximum = 16666
			case "mutable", "capabilities":
				maximum = limit / 3 // Every encoded string needs at least two quotes and a separator.
			default:
				return ErrCorrupt
			}
			count := 0
			for d.More() {
				count++
				if count > maximum || visit(depth+1, "") != nil {
					return ErrCorrupt
				}
			}
			tok, err = d.Token()
			if err != nil || tok != json.Delim(']') {
				return ErrCorrupt
			}
		default:
			return ErrCorrupt
		}
		return nil
	}
	if visit(1, "") != nil {
		return ErrCorrupt
	}
	if _, err := d.Token(); err != io.EOF {
		return ErrCorrupt
	}
	return nil
}
func manifestExactObject(raw []byte, keys ...string) (map[string]json.RawMessage, error) {
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil || len(m) != len(keys) {
		return nil, ErrCorrupt
	}
	for _, k := range keys {
		if _, ok := m[k]; !ok {
			return nil, ErrCorrupt
		}
	}
	return m, nil
}
func ParseManifestConfig(raw []byte) (ManifestConfig, error) {
	fail := errors.New("invalid manifest worker configuration")
	if manifestJSONShape(raw, MaxManifestConfigSize) != nil {
		return ManifestConfig{}, fail
	}
	m, e := manifestExactObject(raw, "version", "policy", "public_key", "payout_address", "authority_origin", "authority_pin", "transport_trust_id", "max_records", "max_bytes")
	if e != nil {
		return ManifestConfig{}, fail
	}
	if _, e = manifestExactObject(m["policy"], "version", "network", "genesis", "dna_version", "prospect_target"); e != nil {
		return ManifestConfig{}, fail
	}
	if _, e = manifestExactObject(m["authority_pin"], "version", "network", "genesis", "authority_id", "descriptor_id"); e != nil {
		return ManifestConfig{}, fail
	}
	var c ManifestConfig
	if json.Unmarshal(raw, &c) != nil {
		return ManifestConfig{}, fail
	}
	return ValidateManifestConfig(c)
}
