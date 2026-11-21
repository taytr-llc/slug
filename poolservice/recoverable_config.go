// Copyright (c) 2026 The SLUG developers. ISC license.
package poolservice

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

func recoveryHash(label string, raw []byte) string {
	h := sha256.New()
	h.Write([]byte(label + "\x00v1\x00"))
	var n [4]byte
	binary.LittleEndian.PutUint32(n[:], uint32(len(raw)))
	h.Write(n[:])
	h.Write(raw)
	return hex.EncodeToString(h.Sum(nil))
}
func ValidateRecoverableDailyClientConfig(c RecoverableDailyClientConfig) (RecoverableDailyClientConfig, error) {
	if c.Version != 1 {
		return RecoverableDailyClientConfig{}, errConfig
	}
	base, e := ValidateDailyClientConfig(c.Base)
	if e != nil {
		return RecoverableDailyClientConfig{}, e
	}
	source, e := poolshare.DailyConfigID(base.Source)
	if e != nil || registry.ValidateIssueProfile(c.IssueProfile) != nil || c.IssueProfile.SourceConfigID != source || c.IssueProfile.MaxRequests != base.Source.MaxWorks {
		return RecoverableDailyClientConfig{}, errConfig
	}
	c.Base = base
	raw, e := json.Marshal(c)
	if e != nil || len(raw) > MaxRecoverableClientConfigSize {
		return RecoverableDailyClientConfig{}, errConfig
	}
	return c, nil
}
func RecoverableDailyTransportTrustID(c RecoverableDailyClientConfig) (string, error) {
	c, e := ValidateRecoverableDailyClientConfig(c)
	if e != nil {
		return "", e
	}
	base, e := DailyTransportTrustID(c.Base)
	if e != nil {
		return "", e
	}
	profile, e := registry.IssueProfileID(c.IssueProfile)
	if e != nil {
		return "", errConfig
	}
	return recoveryHash("SLUG pool issue recovery transport", []byte(base+profile)), nil
}
func NewRecoverableDailyClient(config RecoverableDailyClientConfig) (*Client, error) {
	if wire.TestPoW {
		return nil, errConfig
	}
	c, e := ValidateRecoverableDailyClientConfig(config)
	if e != nil {
		return nil, e
	}
	trust, e := RecoverableDailyTransportTrustID(c)
	if e != nil {
		return nil, e
	}
	base, e := DailyTransportTrustID(c.Base)
	if e != nil {
		return nil, e
	}
	binding, e := poolshare.DailyConfigID(c.Base.Source)
	if e != nil {
		return nil, e
	}
	gym, e := gymauth.GymID(identity(c.Base.Source), c.Base.PublicKey)
	if e != nil {
		return nil, errConfig
	}
	profile, e := registry.IssueProfileID(c.IssueProfile)
	if e != nil {
		return nil, errConfig
	}
	client, e := newClient(c.Base.base(), binding, trust, gym, &dailyClientBinding{c.Base.SettlementConfigID, c.Base.RecipientScript})
	if e != nil {
		return nil, e
	}
	client.recoverable = &recoverableClientBinding{RecoverableDailySelection{1, Version2, c.Base.URL, c.Base.PublicKey, c.Base.Source, c.IssueProfile, c.Base.SettlementConfigID, c.Base.RecipientScript, base, trust}, profile}
	return client, nil
}
func validateRecoverableSelection(s RecoverableDailySelection) error {
	id, e := poolshare.DailyConfigID(s.Source)
	profile, p := registry.IssueProfileID(s.IssueProfile)
	if e != nil || p != nil || s.Version != 1 || s.BaseVersion != Version2 || s.IssueProfile.SourceConfigID != id || s.IssueProfile.MaxRequests != s.Source.MaxWorks || !hex64(s.SettlementConfigID, true) || !nativeRecipient(s.RecipientScript) || !hex64(s.BaseTransportTrustID, true) || s.TransportTrustID != recoveryHash("SLUG pool issue recovery transport", []byte(s.BaseTransportTrustID+profile)) {
		return errConfig
	}
	if _, e = gymauth.GymID(identity(s.Source), s.PublicKey); e != nil {
		return errConfig
	}
	return nil
}
func (c *Client) CheckRecoverableDailySelection(s RecoverableDailySelection) error {
	if c == nil || c.recoverable == nil || validateRecoverableSelection(s) != nil || c.recoverable.selection != s {
		return errConfig
	}
	return nil
}
func (c *Client) recoverableBinding(version uint32, binding, settlement, recipient, profile string) bool {
	return c != nil && c.recoverable != nil && version == Version3 && binding == c.binding && settlement == c.daily.settlement && recipient == c.daily.recipient && profile == c.recoverable.profileID
}

// These codecs preserve exact ordered wrapper bytes; unlike HTTP, config bytes
// have no surrounding whitespace or automatic normalization on parse.
func MarshalRecoverableDailyClientConfig(c RecoverableDailyClientConfig) ([]byte, error) {
	c, e := ValidateRecoverableDailyClientConfig(c)
	if e != nil {
		return nil, e
	}
	return json.Marshal(c)
}
func ParseRecoverableDailyClientConfig(raw []byte) (RecoverableDailyClientConfig, error) {
	var c RecoverableDailyClientConfig
	if len(raw) == 0 || len(raw) > MaxRecoverableClientConfigSize || json.Unmarshal(raw, &c) != nil {
		return c, errConfig
	}
	normalized, e := ValidateRecoverableDailyClientConfig(c)
	if e != nil {
		return RecoverableDailyClientConfig{}, e
	}
	expected, e := json.Marshal(normalized)
	if e != nil || string(expected) != string(raw) {
		return RecoverableDailyClientConfig{}, errConfig
	}
	return normalized, nil
}
