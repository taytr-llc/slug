// Copyright (c) 2026 The SLUG developers. ISC license.
package poolservice

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"

	"github.com/taytr-llc/slug/address/v2"
	"github.com/taytr-llc/slug/btcec/v2"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/prospectwork"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/txscript/v2"
	"github.com/taytr-llc/slug/wire/v2"
)

var errConfig = errors.New("invalid pool service configuration")

func hex64(s string, nonzero bool) bool {
	if len(s) != 64 || s != strings.ToLower(s) || nonzero && s == strings.Repeat("0", 64) {
		return false
	}
	_, e := hex.DecodeString(s)
	return e == nil
}
func publicKey(s string) bool {
	if len(s) != 66 || s != strings.ToLower(s) {
		return false
	}
	raw, e := hex.DecodeString(s)
	if e != nil {
		return false
	}
	key, e := btcec.ParsePubKey(raw)
	return e == nil && bytes.Equal(raw, key.SerializeCompressed())
}
func validSource(s poolshare.Config) bool {
	if s.Version != poolshare.Version1 || s.MaxWorks == 0 || s.MaxWorks > 256 || s.MaxSubmissions == 0 || s.MaxSubmissions > 4096 || s.MaxBytes < 4096 || s.MaxBytes > 1<<30 || !hex64(s.PoolTarget, true) || len(s.PayoutAddress) > 128 {
		return false
	}
	raw, e := json.Marshal(s.Policy)
	if e != nil {
		return false
	}
	p, e := prospectnode.ParsePolicy(raw)
	if e != nil || p != s.Policy {
		return false
	}
	pin := s.AuthorityPin
	if pin.Version != registry.Version5 || pin.Network != p.Network || pin.Genesis != p.Genesis || !hex64(pin.AuthorityID, false) || !hex64(pin.DescriptorID, false) {
		return false
	}
	d, e := fighterdescriptor.ResolveCandidate(pin.DescriptorID)
	if e != nil || d.DNAVersion != p.DNAVersion {
		return false
	}
	a, e := address.DecodeAddress(s.PayoutAddress, &chaincfg.MainNetParams)
	if e != nil || !a.IsForNet(&chaincfg.MainNetParams) || a.EncodeAddress() != s.PayoutAddress {
		return false
	}
	switch a.(type) {
	case *address.AddressPubKeyHash, *address.AddressScriptHash, *address.AddressWitnessPubKeyHash, *address.AddressWitnessScriptHash, *address.AddressTaproot:
	default:
		return false
	}
	_, e = txscript.PayToAddrScript(a)
	return e == nil
}

// BindingID pins the complete operator source configuration, including its
// explicit bounded journal identity. It is not a signature or source import.
func BindingID(source poolshare.Config) (string, error) {
	if !validSource(source) {
		return "", errConfig
	}
	raw, _ := json.Marshal(source)
	digest := sha256.Sum256(append([]byte("SLUG pool service source\x00v1\x00"), raw...))
	return hex.EncodeToString(digest[:]), nil
}

func manifestConfig(c ClientConfig) prospectservice.ManifestClientConfig {
	return prospectservice.ManifestClientConfig{Version: prospectservice.Version3, URL: c.URL, Token: c.Token, CACertificate: c.CACertificate, AuthorityPin: c.Source.AuthorityPin}
}
func ValidateClientConfig(c ClientConfig) (ClientConfig, error) {
	if c.Version != Version1 || !validSource(c.Source) || !publicKey(c.PublicKey) {
		return ClientConfig{}, errConfig
	}
	m, e := prospectservice.ValidateManifestClientConfig(manifestConfig(c))
	if e != nil {
		return ClientConfig{}, errConfig
	}
	c.URL, c.Token, c.CACertificate = m.URL, m.Token, m.CACertificate
	raw, e := json.Marshal(c)
	if e != nil || len(raw) > prospectservice.MaxClientConfigSize {
		return ClientConfig{}, errConfig
	}
	return c, nil
}

// TransportTrustID binds canonical endpoint/CA policy, pool source and gym.
// Rotating the in-memory bearer token never changes this public identity.
func TransportTrustID(c ClientConfig) (string, error) {
	var e error
	c, e = ValidateClientConfig(c)
	if e != nil {
		return "", e
	}
	base, e := prospectservice.ManifestTransportTrustID(manifestConfig(c))
	if e != nil {
		return "", errConfig
	}
	binding, e := BindingID(c.Source)
	if e != nil {
		return "", e
	}
	digest := sha256.Sum256([]byte("SLUG pool transport trust\x00v1\x00" + base + binding + c.PublicKey))
	return hex.EncodeToString(digest[:]), nil
}

// Bound typed values before JSON copying; receiving a small job must not allow
// a caller-owned template to cause unbounded allocations before validation.
func workBounds(w poolshare.MiningWork) bool {
	b := w.Template.Block
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
	for _, v := range []string{b.WorkID, b.DefaultWitnessCommitment, b.LongPollID, b.LongPollURI, b.RejectReason, b.Bits, b.PreviousHash, b.Target, b.NonceRange} {
		if !add(len(v)) {
			return false
		}
	}
	if b.CoinbaseAux != nil && !add(len(b.CoinbaseAux.Flags)) {
		return false
	}
	for _, list := range [][]string{b.Mutable, b.Capabilities} {
		for _, v := range list {
			if !add(len(v) + 3) {
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

// PrepareWork reconstructs exact extra-nonce-zero pool work from retained
// public inputs. Configured trust authenticates the service; this validates the
// job, template, gym and payout binding without asserting issuance or finality.
func PrepareWork(source poolshare.Config, key string, w poolshare.MiningWork) (*prospectwork.ManifestWork, error) {
	return prepareSelectedWork(source, key, w, false)
}

func prepareSelectedWork(source poolshare.Config, key string, w poolshare.MiningWork, daily bool) (*prospectwork.ManifestWork, error) {
	valid := validSource(source)
	if daily {
		valid = validDailySource(source)
	}
	if wire.TestPoW || !valid || !publicKey(key) || prospectjob.ValidateManifest(w.Job) != nil || !workBounds(w) {
		return nil, errConfig
	}
	j, p, pin := w.Job, source.Policy, source.AuthorityPin
	if j.PublicKey != key || j.AuthorityID != pin.AuthorityID || j.DescriptorID != pin.DescriptorID || j.Context.Version != p.Version || j.Context.Network != p.Network || j.Context.Genesis != p.Genesis || j.Context.DNAVersion != p.DNAVersion || j.Context.ProspectTarget != p.ProspectTarget || w.Template.Context != j.Context {
		return nil, errConfig
	}
	gym, e := gymauth.GymID(identity(source), key)
	if e != nil || gym != j.GymID {
		return nil, errConfig
	}
	raw, e := json.Marshal(w.Template)
	if e != nil || len(raw) > prospectwork.MaxTemplateSize {
		return nil, errConfig
	}
	var owned prospectwork.Template
	if json.Unmarshal(raw, &owned) != nil {
		return nil, errConfig
	}
	return prospectwork.PrepareManifestWork(j, owned, source.PayoutAddress, 0, source.PoolTarget)
}
