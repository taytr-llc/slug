// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package prospectservice

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/taytr-llc/slug/btcec/v2"
)

func normalizePublicKey(value string) (string, error) {
	if len(value) != 66 {
		return "", errors.New("invalid public key")
	}
	raw, err := hex.DecodeString(value)
	if err != nil || len(raw) != 33 {
		return "", errors.New("invalid public key")
	}
	key, err := btcec.ParsePubKey(raw)
	if err != nil || !bytes.Equal(key.SerializeCompressed(), raw) {
		return "", errors.New("invalid public key")
	}
	return hex.EncodeToString(raw), nil
}

func normalizeToken(value string) (string, error) {
	if len(value) != 64 {
		return "", errors.New("invalid credential")
	}
	raw, err := hex.DecodeString(value)
	if err != nil {
		return "", errors.New("invalid credential")
	}
	return hex.EncodeToString(raw), nil
}

// ValidateServerConfig returns an independent canonical configuration. Port
// zero is permitted for a local ephemeral listener; wildcard binds are not.
func ValidateServerConfig(config ServerConfig) (ServerConfig, error) {
	return validateServerConfig(config, false)
}

func validateServerConfig(config ServerConfig, sessions bool) (ServerConfig, error) {
	invalid := errors.New("invalid service configuration")
	host, port, err := net.SplitHostPort(config.Listen)
	ip := net.ParseIP(host)
	n, portErr := strconv.Atoi(port)
	if err != nil || portErr != nil || n < 0 || n > 65535 || strconv.Itoa(n) != port || ip == nil || !ip.IsLoopback() ||
		(config.Version != Version1 && config.Version != Version2) || (!sessions && len(config.Clients) == 0) || len(config.Clients) > 64 ||
		(sessions && (config.Version != Version2 || len(config.Clients) != 0)) ||
		config.MaxJobs == 0 || config.MaxRecords == 0 || config.MaxJobs > 1000000000 || config.MaxRecords > 1000000000 {
		return ServerConfig{}, invalid
	}
	config.Listen = net.JoinHostPort(ip.String(), port)
	clients := make([]Credential, len(config.Clients))
	keys, hashes := make(map[string]bool), make(map[string]bool)
	for i, client := range config.Clients {
		key, keyErr := normalizePublicKey(client.PublicKey)
		hash, hashErr := normalizeToken(client.TokenSHA256)
		if keyErr != nil || hashErr != nil || keys[key] || hashes[hash] {
			return ServerConfig{}, invalid
		}
		keys[key], hashes[hash] = true, true
		clients[i] = Credential{PublicKey: key, TokenSHA256: hash}
	}
	config.Clients = clients
	return config, nil
}

func ParseServerConfig(data []byte) (ServerConfig, error) {
	invalid := errors.New("invalid service configuration")
	fields, err := parseObject(data, MaxServerConfigSize, "version", "listen", "clients", "max_jobs", "max_records")
	if err != nil {
		return ServerConfig{}, invalid
	}
	var config ServerConfig
	var clients []json.RawMessage
	for name, target := range map[string]any{"version": &config.Version, "listen": &config.Listen, "clients": &clients, "max_jobs": &config.MaxJobs, "max_records": &config.MaxRecords} {
		if decodeField(fields, name, target) != nil {
			return ServerConfig{}, invalid
		}
	}
	if len(clients) == 0 || len(clients) > 64 {
		return ServerConfig{}, invalid
	}
	for _, raw := range clients {
		values, err := parseObject(raw, MaxServerConfigSize, "public_key", "token_sha256")
		var client Credential
		if err != nil || decodeField(values, "public_key", &client.PublicKey) != nil || decodeField(values, "token_sha256", &client.TokenSHA256) != nil {
			return ServerConfig{}, invalid
		}
		config.Clients = append(config.Clients, client)
	}
	return ValidateServerConfig(config)
}

func validateClientConfig(config ClientConfig) (ClientConfig, *url.URL, error) {
	invalid := errors.New("invalid service client configuration")
	token, err := normalizeToken(config.Token)
	if err != nil || (config.Version != Version1 && config.Version != Version2) || len(config.URL)+len(config.Token)+len(config.CACertificate) > MaxClientConfigSize {
		return ClientConfig{}, nil, invalid
	}
	endpoint, err := url.Parse(config.URL)
	if err != nil || endpoint.Host == "" || endpoint.Hostname() == "" || endpoint.Opaque != "" || endpoint.User != nil ||
		endpoint.RawQuery != "" || endpoint.ForceQuery || strings.Contains(config.URL, "#") || strings.Contains(endpoint.Hostname(), "%") ||
		(endpoint.Path != "" && endpoint.Path != "/") || endpoint.RawPath != "" {
		return ClientConfig{}, nil, invalid
	}
	if port := endpoint.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n <= 0 || n > 65535 || strconv.Itoa(n) != port {
			return ClientConfig{}, nil, invalid
		}
	} else if strings.HasSuffix(endpoint.Host, ":") {
		return ClientConfig{}, nil, invalid
	}
	switch endpoint.Scheme {
	case "http":
		ip := net.ParseIP(endpoint.Hostname())
		if ip == nil || !ip.IsLoopback() || config.CACertificate != "" {
			return ClientConfig{}, nil, invalid
		}
		// The local authority checks the Host port against its actual listener.
		// Preserve an omitted HTTP port's standard meaning in that exact form.
		if endpoint.Port() == "" {
			endpoint.Host = net.JoinHostPort(endpoint.Hostname(), "80")
		}
	case "https":
	default:
		return ClientConfig{}, nil, invalid
	}
	endpoint.Path = ""
	config.URL, config.Token = endpoint.String(), token
	return config, endpoint, nil
}

func ParseClientConfig(data []byte) (ClientConfig, error) {
	invalid := errors.New("invalid service client configuration")
	fields, err := parseObject(data, MaxClientConfigSize, "version", "url", "token", "ca_certificate")
	if err != nil {
		return ClientConfig{}, invalid
	}
	var config ClientConfig
	for name, target := range map[string]any{"version": &config.Version, "url": &config.URL, "token": &config.Token, "ca_certificate": &config.CACertificate} {
		if decodeField(fields, name, target) != nil {
			return ClientConfig{}, invalid
		}
	}
	config, _, err = validateClientConfig(config)
	return config, err
}
