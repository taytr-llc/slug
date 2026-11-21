// Copyright (c) 2026 The SLUG developers. ISC license.
package poolservice

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/prospectwork"
	"github.com/taytr-llc/slug/wire/v2"
)

// DailyClientConfig independently pins the source and enrolled money binding.
// Token rotation does not change these public selections or transport identity.
type DailyClientConfig struct {
	Version            uint32           `json:"version"`
	URL                string           `json:"url"`
	Token              string           `json:"token"`
	CACertificate      string           `json:"ca_certificate"`
	PublicKey          string           `json:"public_key"`
	Source             poolshare.Config `json:"source"`
	SettlementConfigID string           `json:"settlement_config_id"`
	RecipientScript    string           `json:"recipient_script"`
}

type dailyClientBinding struct{ settlement, recipient string }

func (c DailyClientConfig) base() ClientConfig {
	return ClientConfig{c.Version, c.URL, c.Token, c.CACertificate, c.PublicKey, c.Source}
}
func validDailySource(c poolshare.Config) bool {
	_, err := poolshare.DailyConfigID(c)
	return err == nil
}
func nativeRecipient(s string) bool {
	if len(s) != 44 || !strings.HasPrefix(s, "0014") || s != strings.ToLower(s) {
		return false
	}
	b, err := hex.DecodeString(s)
	return err == nil && len(b) == 22
}
func ValidateDailyClientConfig(c DailyClientConfig) (DailyClientConfig, error) {
	if c.Version != Version2 || !validDailySource(c.Source) || !publicKey(c.PublicKey) || !hex64(c.SettlementConfigID, true) || !nativeRecipient(c.RecipientScript) {
		return DailyClientConfig{}, errConfig
	}
	m, err := prospectservice.ValidateManifestClientConfig(manifestConfig(c.base()))
	if err != nil {
		return DailyClientConfig{}, errConfig
	}
	c.URL, c.Token, c.CACertificate = m.URL, m.Token, m.CACertificate
	raw, err := json.Marshal(c)
	if err != nil || len(raw) > prospectservice.MaxClientConfigSize {
		return DailyClientConfig{}, errConfig
	}
	return c, nil
}
func DailyTransportTrustID(c DailyClientConfig) (string, error) {
	c, err := ValidateDailyClientConfig(c)
	if err != nil {
		return "", err
	}
	base, err := prospectservice.ManifestTransportTrustID(manifestConfig(c.base()))
	if err != nil {
		return "", errConfig
	}
	source, err := poolshare.DailyConfigID(c.Source)
	if err != nil {
		return "", errConfig
	}
	h := sha256.Sum256([]byte("SLUG pool transport trust\x00v2\x00" + base + source + c.PublicKey + c.SettlementConfigID + c.RecipientScript))
	return hex.EncodeToString(h[:]), nil
}
func NewDailyClient(c DailyClientConfig) (*Client, error) {
	if wire.TestPoW {
		return nil, errConfig
	}
	c, err := ValidateDailyClientConfig(c)
	if err != nil {
		return nil, err
	}
	binding, err := poolshare.DailyConfigID(c.Source)
	if err != nil {
		return nil, errConfig
	}
	trust, err := DailyTransportTrustID(c)
	if err != nil {
		return nil, err
	}
	gym, err := gymauth.GymID(identity(c.Source), c.PublicKey)
	if err != nil {
		return nil, errConfig
	}
	return newClient(c.base(), binding, trust, gym, &dailyClientBinding{c.SettlementConfigID, c.RecipientScript})
}

// PrepareDailyWork retains the original ManifestJob2/template/extraNonce0 rules.
func PrepareDailyWork(source poolshare.Config, key string, w poolshare.MiningWork) (*prospectwork.ManifestWork, error) {
	return prepareSelectedWork(source, key, w, true)
}
func SubmissionMatchesDaily(source poolshare.Config, w poolshare.MiningWork, nonce uint32, check prospectwork.ShareCheck, receipt poolshare.Submission) bool {
	return validDailySource(source) && validSubmission(source, w, nonce, check, receipt)
}
