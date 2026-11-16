// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package prospectnode

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/url"
	"strings"
	"unicode/utf8"

	"github.com/taytr-llc/slug/fighter"
	"github.com/taytr-llc/slug/wire/v2"
)

const (
	Version1         uint32 = 1
	MaxPolicySize           = 4096
	MaxRPCConfigSize        = 65536

	// These identify the production hash profile, independently of build tags.
	mainnetNetwork uint32 = 1397511495
	mainnetGenesis        = "7893676099cf9985bb198dfb6a347d9422a19f2422cf3326a999a7230ce246cd"
)

// Policy must come from the operator, separately from claimant proof data.
type Policy struct {
	Version        uint32 `json:"version"`
	Network        uint32 `json:"network"`
	Genesis        string `json:"genesis"`
	DNAVersion     uint32 `json:"dna_version"`
	ProspectTarget string `json:"prospect_target"`
}

// RPCConfig is sensitive configuration. Errors never include its contents.
// An empty CACertificate uses system roots; a nonempty value is a PEM root set.
type RPCConfig struct {
	Version       uint32 `json:"version"`
	URL           string `json:"url"`
	Username      string `json:"username"`
	Password      string `json:"password"`
	CACertificate string `json:"ca_certificate"`
}

// ParsePolicy requires every version-1 field exactly once and validates it.
func ParsePolicy(data []byte) (Policy, error) {
	var policy Policy
	err := decodeFields(data, MaxPolicySize, map[string]any{
		"version": &policy.Version, "network": &policy.Network,
		"genesis": &policy.Genesis, "dna_version": &policy.DNAVersion,
		"prospect_target": &policy.ProspectTarget,
	}, false)
	if err != nil {
		return Policy{}, errors.New("invalid policy JSON")
	}
	return validatePolicy(policy)
}

// ParseRPCConfig checks the strict schema and endpoint rules without connecting.
func ParseRPCConfig(data []byte) (RPCConfig, error) {
	var config RPCConfig
	err := decodeFields(data, MaxRPCConfigSize, map[string]any{
		"version": &config.Version, "url": &config.URL,
		"username": &config.Username, "password": &config.Password,
		"ca_certificate": &config.CACertificate,
	}, false)
	if err != nil {
		return RPCConfig{}, errors.New("invalid RPC configuration JSON")
	}
	if _, err := validateRPCConfig(config); err != nil {
		return RPCConfig{}, err
	}
	return config, nil
}

func productionOnly() error {
	if wire.TestPoW {
		return errors.New("prospect node verification is unavailable with slugtestpow")
	}
	return nil
}

func normalizeHash(value string, nonzero bool) (string, error) {
	if len(value) != 64 {
		return "", errors.New("invalid 32-byte hexadecimal value")
	}
	decoded, err := hex.DecodeString(value)
	if err != nil || (nonzero && bytes.Equal(decoded, make([]byte, 32))) {
		return "", errors.New("invalid 32-byte hexadecimal value")
	}
	return strings.ToLower(value), nil
}

func validatePolicy(policy Policy) (Policy, error) {
	if policy.Version != Version1 || policy.Network != mainnetNetwork || policy.DNAVersion != fighter.Version1 {
		return Policy{}, errors.New("unsupported prospect node policy")
	}
	genesis, err := normalizeHash(policy.Genesis, true)
	if err != nil || genesis != mainnetGenesis {
		return Policy{}, errors.New("policy requires production SLUG mainnet genesis")
	}
	target, err := normalizeHash(policy.ProspectTarget, true)
	if err != nil {
		return Policy{}, errors.New("invalid prospect policy target")
	}
	policy.Genesis, policy.ProspectTarget = genesis, target
	return policy, nil
}

func validateRPCConfig(config RPCConfig) (*url.URL, error) {
	if config.Version != Version1 || config.Username == "" || config.Password == "" ||
		strings.Contains(config.Username, ":") || len(config.URL)+len(config.Username)+len(config.Password)+len(config.CACertificate) > MaxRPCConfigSize {
		return nil, errors.New("invalid RPC configuration")
	}
	endpoint, err := url.Parse(config.URL)
	if err != nil || endpoint.Host == "" || endpoint.Hostname() == "" || endpoint.Opaque != "" ||
		endpoint.User != nil || endpoint.RawQuery != "" || endpoint.ForceQuery || strings.Contains(config.URL, "#") ||
		strings.Contains(endpoint.Hostname(), "%") {
		return nil, errors.New("invalid RPC endpoint")
	}
	switch endpoint.Scheme {
	case "http":
		ip := net.ParseIP(endpoint.Hostname())
		if ip == nil || !ip.IsLoopback() || config.CACertificate != "" {
			return nil, errors.New("plain HTTP RPC requires a literal loopback address and no TLS certificate")
		}
	case "https":
	default:
		return nil, errors.New("RPC requires HTTP loopback or verified HTTPS")
	}
	return endpoint, nil
}

// object rejects duplicate decoded keys, including escaped aliases. Callers can
// permit unknown node result fields while still requiring all critical fields.
func object(data []byte, maxSize int) (map[string]json.RawMessage, error) {
	if len(data) > maxSize || !utf8.Valid(data) {
		return nil, errors.New("invalid JSON object size or encoding")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, errors.New("expected JSON object")
	}
	values := make(map[string]json.RawMessage)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, errors.New("invalid JSON key")
		}
		key, ok := token.(string)
		if !ok {
			return nil, errors.New("invalid JSON key")
		}
		if _, duplicate := values[key]; duplicate {
			return nil, errors.New("duplicate JSON key")
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, errors.New("invalid JSON value")
		}
		values[key] = value
	}
	if _, err := decoder.Token(); err != nil {
		return nil, errors.New("invalid JSON object")
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, errors.New("trailing JSON data")
	}
	return values, nil
}

func isNull(value []byte) bool { return bytes.Equal(bytes.TrimSpace(value), []byte("null")) }

func decodeFields(data []byte, maxSize int, fields map[string]any, allowUnknown bool) error {
	values, err := object(data, maxSize)
	if err != nil {
		return err
	}
	if !allowUnknown && len(values) != len(fields) {
		return errors.New("unknown or missing JSON field")
	}
	for key, target := range fields {
		value, exists := values[key]
		if !exists || isNull(value) {
			return errors.New("missing or null JSON field")
		}
		if err := json.Unmarshal(value, target); err != nil {
			return errors.New("invalid JSON field type")
		}
	}
	return nil
}
