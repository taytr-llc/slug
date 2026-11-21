// Copyright (c) 2026 The SLUG developers. ISC license.
package poolminer

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"github.com/taytr-llc/slug/btcec/v2"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospect"
)

func identifier(s string) bool {
	if len(s) != 64 || s == "0000000000000000000000000000000000000000000000000000000000000000" {
		return false
	}
	b, e := hex.DecodeString(s)
	return e == nil && hex.EncodeToString(b) == s
}
func publicKey(s string) bool {
	if len(s) != 66 {
		return false
	}
	b, e := hex.DecodeString(s)
	if e != nil || hex.EncodeToString(b) != s {
		return false
	}
	k, e := btcec.ParsePubKey(b)
	return e == nil && bytes.Equal(k.SerializeCompressed(), b)
}
func script(s string) bool {
	if len(s) != 44 || s[:4] != "0014" {
		return false
	}
	b, e := hex.DecodeString(s)
	return e == nil && hex.EncodeToString(b) == s
}
func audience(s string) bool    { return s == PoolAudience || s == AdmissionAudience }
func validConfig(c Config) bool { return validConfigVersion(c, Version1) }
func validConfigVersion(c Config, version uint32) bool {
	if (version != Version1 && version != Version2) || c.Version != version || !identifier(c.CredentialID) || !identifier(c.RuntimeID) || !identifier(c.OperatorProfileID) || !identifier(c.SourceID) || !identifier(c.SettlementConfigID) || len(c.Enrollments) < 1 || len(c.Enrollments) > 64 {
		return false
	}
	keys, scripts := map[string]bool{}, map[string]bool{}
	last := ""
	for _, e := range c.Enrollments {
		if !publicKey(e.PublicKey) || !identifier(e.GymID) || e.GymID <= last || !script(e.PayeeScript) || keys[e.PublicKey] || scripts[e.PayeeScript] {
			return false
		}
		keys[e.PublicKey] = true
		scripts[e.PayeeScript] = true
		last = e.GymID
	}
	raw, e := json.Marshal(c)
	return e == nil && len(raw)+1 <= maxMarkerBytes
}

// ValidateConfig is pure preflight. It neither selects a marker's identity nor
// authenticates a concrete source/store, and the constructors repeat it.
func ValidateConfig(c Config, b Binding) error { return validateConfigVersion(c, b, Version1) }
func validateConfigVersion(c Config, b Binding, version uint32) error {
	if !validConfigVersion(c, version) || b.Profile.Validate() != nil || b.Profile.ID() != c.OperatorProfileID || b.Profile.Pin().Source != b.Source.AuthorityPin {
		return ErrInvalid
	}
	sourceID := poolservice.BindingID
	settlementID := poolsettle.ConfigBindingID
	if version == Version2 {
		sourceID = poolshare.DailyConfigID
		settlementID = poolsettle.DailyConfigBindingID
	}
	source, e := sourceID(b.Source)
	if e != nil || source != c.SourceID {
		return ErrInvalid
	}
	settlement, e := settlementID(b.Settlement)
	if e != nil || settlement != c.SettlementConfigID || len(b.Settlement.Destinations) != len(c.Enrollments) {
		return ErrInvalid
	}
	p := b.Source.Policy
	identity := prospect.Context{Version: p.Version, Network: p.Network, Genesis: p.Genesis, DNAVersion: p.DNAVersion}
	for i, en := range c.Enrollments {
		gym, e := gymauth.GymID(identity, en.PublicKey)
		d := b.Settlement.Destinations[i]
		if e != nil || gym != en.GymID || d.GymID != en.GymID || d.Script != en.PayeeScript {
			return ErrInvalid
		}
	}
	return nil
}
func digest(domain string, v any) (string, error) { return digestVersion(domain, v, Version1) }
func digestVersion(domain string, v any, version uint32) (string, error) {
	label := "v1"
	if version == Version2 {
		label = "v2"
	} else if version != Version1 {
		return "", ErrInvalid
	}
	raw, e := json.Marshal(v)
	if e != nil || len(raw) > maxMarkerBytes {
		return "", ErrInvalid
	}
	h := sha256.New()
	h.Write([]byte(domain + "\x00" + label + "\x00"))
	var n [4]byte
	binary.LittleEndian.PutUint32(n[:], uint32(len(raw)))
	h.Write(n[:])
	h.Write(raw)
	return hex.EncodeToString(h.Sum(nil)), nil
}
func configDigest(c Config) (string, error) { return configDigestVersion(c, Version1) }
func configDigestVersion(c Config, version uint32) (string, error) {
	if !validConfigVersion(c, version) {
		return "", ErrInvalid
	}
	return digestVersion("SLUG pool miner enrollment", c, version)
}
func ConfigID(c Config, b Binding) (string, error) {
	if e := ValidateConfig(c, b); e != nil {
		return "", e
	}
	return configDigest(c)
}
func tokenHashes(id, a, token string) (TokenUse, error) {
	return tokenHashesVersion(id, a, token, Version1)
}
func tokenHashesVersion(id, a, token string, version uint32) (TokenUse, error) {
	if !identifier(id) || !audience(a) || !identifier(token) {
		return TokenUse{}, ErrInvalid
	}
	x, e := digestVersion("SLUG pool miner credential", struct {
		ConfigID string `json:"config_id"`
		Audience string `json:"audience"`
		Token    string `json:"token"`
	}{id, a, token}, version)
	if e != nil {
		return TokenUse{}, e
	}
	y, e := digestVersion("SLUG pool miner token reuse", struct {
		ConfigID string `json:"config_id"`
		Token    string `json:"token"`
	}{id, token}, version)
	return TokenUse{x, y}, e
}
