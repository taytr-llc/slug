// Copyright (c) 2026 The SLUG developers. ISC license.
package poolshare

import (
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"

	"github.com/taytr-llc/slug/address/v2"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/prospectwork"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/txscript/v2"
	"github.com/taytr-llc/slug/wire/v2"
)

const (
	application       = "slug-pool-share-journal"
	markerName        = "SLUG-POOL.json"
	maxMetadataSize   = 8192
	maxWorkSize       = prospectwork.MaxTemplateSize + 16384
	maxSubmissionSize = 4096
	maxWorks          = 256
	maxSubmissions    = 4096
)

func canonicalHash(s string, nonzero bool) bool {
	if len(s) != 64 || s != strings.ToLower(s) || (nonzero && s == strings.Repeat("0", 64)) {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

func nonnil(value any) bool {
	if value == nil {
		return false
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return !v.IsNil()
	}
	return true
}

func validateConfig(c Config, authority *registry.ManifestStore, node Node, sessions prospectservice.SessionAuthorizer) error {
	return validateConfigMode(c, authority, node, sessions, false)
}

func validateConfigMode(c Config, authority *registry.ManifestStore, node Node, sessions prospectservice.SessionAuthorizer, auditOnly bool) error {
	if wire.TestPoW {
		return ErrProfile
	}
	if c.Version != Version1 || c.MaxWorks == 0 || c.MaxWorks > maxWorks || c.MaxSubmissions == 0 || c.MaxSubmissions > maxSubmissions || c.MaxBytes < maxSubmissionSize || c.MaxBytes > 1<<30 || !canonicalHash(c.PoolTarget, true) || len(c.PayoutAddress) > 128 || !nonnil(authority) || (!auditOnly && (!nonnil(node) || !nonnil(sessions))) {
		return ErrConfig
	}
	raw, _ := json.Marshal(c.Policy)
	policy, err := prospectnode.ParsePolicy(raw)
	if err != nil || policy != c.Policy || c.AuthorityPin.Version != registry.Version5 || c.AuthorityPin.Network != policy.Network || c.AuthorityPin.Genesis != policy.Genesis || !canonicalHash(c.AuthorityPin.AuthorityID, false) || !canonicalHash(c.AuthorityPin.DescriptorID, false) {
		return ErrConfig
	}
	descriptor, err := fighterdescriptor.ResolveCandidate(c.AuthorityPin.DescriptorID)
	if err != nil || descriptor.DNAVersion != policy.DNAVersion {
		return ErrConfig
	}
	destination, err := address.DecodeAddress(c.PayoutAddress, &chaincfg.MainNetParams)
	if err != nil || !destination.IsForNet(&chaincfg.MainNetParams) || destination.EncodeAddress() != c.PayoutAddress {
		return ErrConfig
	}
	switch destination.(type) {
	case *address.AddressPubKeyHash, *address.AddressScriptHash, *address.AddressWitnessPubKeyHash,
		*address.AddressWitnessScriptHash, *address.AddressTaproot:
	default:
		return ErrConfig
	}
	if _, err := txscript.PayToAddrScript(destination); err != nil {
		return ErrConfig
	}
	info, err := authority.Info()
	if err != nil || (!auditOnly && info.ReadOnly) || info.Pin != c.AuthorityPin {
		return ErrConfig
	}
	return nil
}

func (p *Pool) samePolicy(c prospect.Context) bool {
	x := p.config.Policy
	return c.Version == x.Version && c.Network == x.Network && c.Genesis == x.Genesis && c.DNAVersion == x.DNAVersion && c.ProspectTarget == x.ProspectTarget
}

// Bound adapter-owned nested data before copying it. The concrete node already
// bounds JSON; this also protects against an incorrectly implemented adapter.
func templateBounds(t prospectwork.Template) bool {
	b := t.Block
	if b.CoinbaseTxn != nil || len(b.Transactions) > 16666 || len(b.Mutable) > 1024 || len(b.Capabilities) > 1024 {
		return false
	}
	total := 0
	add := func(n int) bool {
		if n > prospectwork.MaxTemplateSize-total {
			return false
		}
		total += n
		return true
	}
	for _, s := range []string{b.WorkID, b.DefaultWitnessCommitment, b.LongPollID, b.LongPollURI, b.RejectReason, b.Bits, b.PreviousHash, b.Target, b.NonceRange} {
		if !add(len(s)) {
			return false
		}
	}
	if b.CoinbaseAux != nil && !add(len(b.CoinbaseAux.Flags)) {
		return false
	}
	for _, values := range [][]string{b.Mutable, b.Capabilities} {
		for _, s := range values {
			if !add(len(s) + 3) {
				return false
			}
		}
	}
	for _, tx := range b.Transactions {
		if len(tx.Depends) > len(b.Transactions) || len(tx.Data) > 2*wire.MaxBlockPayload || len(tx.TxID) != 64 || len(tx.Hash) != 64 || !add(len(tx.Data)+128+20*len(tx.Depends)) {
			return false
		}
	}
	return true
}
