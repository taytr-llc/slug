// Copyright (c) 2026 The SLUG developers. ISC license.

package prospectservice

import (
	"bytes"
	"crypto/sha256"
	"crypto/x509"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"sort"
	"strings"

	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/registry"
)

var errManifestConfig = errors.New("invalid manifest service configuration")

func manifestHex(s string, size int) bool {
	if len(s) != 2*size || s != strings.ToLower(s) {
		return false
	}
	_, err := hex.DecodeString(s)
	return err == nil
}

func manifestPin(pin registry.ManifestAuthorityPin) (fighterdescriptor.Descriptor, error) {
	if pin.Version != registry.Version5 || !manifestHex(pin.Genesis, 32) || !manifestHex(pin.AuthorityID, 32) || !manifestHex(pin.DescriptorID, 32) {
		return fighterdescriptor.Descriptor{}, errManifestConfig
	}
	descriptor, err := fighterdescriptor.ResolveCandidate(pin.DescriptorID)
	if err != nil {
		return fighterdescriptor.Descriptor{}, errManifestConfig
	}
	// ParsePolicy pins the normal chain identity independently of build tags.
	raw, _ := json.Marshal(prospectnode.Policy{Version: 1, Network: pin.Network, Genesis: pin.Genesis, DNAVersion: descriptor.DNAVersion, ProspectTarget: strings.Repeat("f", 64)})
	if _, err = prospectnode.ParsePolicy(raw); err != nil {
		return fighterdescriptor.Descriptor{}, errManifestConfig
	}
	return descriptor, nil
}

func manifestConfigFits(value any) bool {
	raw, err := json.Marshal(value)
	return err == nil && len(raw) <= MaxClientConfigSize
}

// ValidateManifestServerConfig owns and canonicalizes configuration without
// choosing static/session authentication or touching storage/network state.
func ValidateManifestServerConfig(config ManifestServerConfig) (ManifestServerConfig, error) {
	if config.Version != Version3 || len(config.Listen) > MaxServerConfigSize || len(config.Clients) > 64 {
		return ManifestServerConfig{}, errManifestConfig
	}
	if _, err := manifestPin(config.AuthorityPin); err != nil {
		return ManifestServerConfig{}, errManifestConfig
	}
	old, err := validateServerConfig(ServerConfig{Version: Version2, Listen: config.Listen, Clients: config.Clients, MaxJobs: config.MaxJobs, MaxRecords: config.MaxRecords}, len(config.Clients) == 0)
	if err != nil {
		return ManifestServerConfig{}, errManifestConfig
	}
	config.Listen, config.Clients = old.Listen, old.Clients
	if !manifestConfigFits(config) {
		return ManifestServerConfig{}, errManifestConfig
	}
	return config, nil
}

func parseManifestPin(raw []byte) (registry.ManifestAuthorityPin, error) {
	fields, err := parseObject(raw, registry.MaxManifestMarkerSize, "version", "network", "genesis", "authority_id", "descriptor_id")
	var pin registry.ManifestAuthorityPin
	if err != nil {
		return pin, errManifestConfig
	}
	for name, target := range map[string]any{"version": &pin.Version, "network": &pin.Network, "genesis": &pin.Genesis, "authority_id": &pin.AuthorityID, "descriptor_id": &pin.DescriptorID} {
		if decodeField(fields, name, target) != nil {
			return registry.ManifestAuthorityPin{}, errManifestConfig
		}
	}
	if _, err = manifestPin(pin); err != nil {
		return registry.ManifestAuthorityPin{}, errManifestConfig
	}
	return pin, nil
}

func ParseManifestServerConfig(raw []byte) (ManifestServerConfig, error) {
	if manifestJSONShape(raw, MaxServerConfigSize, 3, map[string]int{"clients": 64}) != nil {
		return ManifestServerConfig{}, errManifestConfig
	}
	fields, err := parseObject(raw, MaxServerConfigSize, "version", "listen", "authority_pin", "clients", "max_jobs", "max_records")
	var config ManifestServerConfig
	if err != nil {
		return config, errManifestConfig
	}
	var clients []json.RawMessage
	for name, target := range map[string]any{"version": &config.Version, "listen": &config.Listen, "clients": &clients, "max_jobs": &config.MaxJobs, "max_records": &config.MaxRecords} {
		if decodeField(fields, name, target) != nil {
			return ManifestServerConfig{}, errManifestConfig
		}
	}
	config.AuthorityPin, err = parseManifestPin(fields["authority_pin"])
	if err != nil || len(clients) > 64 {
		return ManifestServerConfig{}, errManifestConfig
	}
	config.Clients = make([]Credential, 0, len(clients))
	for _, raw := range clients {
		fields, err := parseObject(raw, MaxServerConfigSize, "public_key", "token_sha256")
		var c Credential
		if err != nil || decodeField(fields, "public_key", &c.PublicKey) != nil || decodeField(fields, "token_sha256", &c.TokenSHA256) != nil {
			return ManifestServerConfig{}, errManifestConfig
		}
		config.Clients = append(config.Clients, c)
	}
	return ValidateManifestServerConfig(config)
}

type manifestCertificate struct {
	fingerprint [32]byte
	der         []byte
}

// Only complete, public certificate blocks and ASCII PEM whitespace are
// accepted. In particular pem.Decode must not silently skip leading garbage.
func manifestCertificates(text string) ([]manifestCertificate, string, error) {
	if len(text) > MaxClientConfigSize {
		return nil, "", errManifestConfig
	}
	remaining := []byte(text)
	certificates := make(map[[32]byte][]byte)
	const begin = "-----BEGIN CERTIFICATE-----"
	const end = "-----END CERTIFICATE-----"
	for {
		remaining = bytes.Trim(remaining, " \t\r\n")
		if len(remaining) == 0 {
			break
		}
		if !bytes.HasPrefix(remaining, []byte(begin)) {
			return nil, "", errManifestConfig
		}
		at := bytes.Index(remaining, []byte(end))
		if at < 0 {
			return nil, "", errManifestConfig
		}
		size := at + len(end)
		// pem.Decode resynchronizes at later BEGIN lines after malformed input.
		// This profile requires the first boundary to be the decoded block.
		if bytes.Count(remaining[:size], []byte("-----BEGIN ")) != 1 {
			return nil, "", errManifestConfig
		}
		block, rest := pem.Decode(remaining[:size])
		if block == nil || block.Type != "CERTIFICATE" || len(block.Headers) != 0 || len(bytes.Trim(rest, " \t\r\n")) != 0 {
			return nil, "", errManifestConfig
		}
		if _, err := x509.ParseCertificate(block.Bytes); err != nil {
			return nil, "", errManifestConfig
		}
		fingerprint := sha256.Sum256(block.Bytes)
		if old, ok := certificates[fingerprint]; ok && !bytes.Equal(old, block.Bytes) {
			return nil, "", errManifestConfig
		}
		certificates[fingerprint] = block.Bytes
		remaining = remaining[size:]
	}
	if len(certificates) == 0 {
		return nil, "", errManifestConfig
	}
	result := make([]manifestCertificate, 0, len(certificates))
	for fingerprint, der := range certificates {
		result = append(result, manifestCertificate{fingerprint, der})
	}
	sort.Slice(result, func(i, j int) bool { return bytes.Compare(result[i].fingerprint[:], result[j].fingerprint[:]) < 0 })
	var canonical bytes.Buffer
	for _, certificate := range result {
		_ = pem.Encode(&canonical, &pem.Block{Type: "CERTIFICATE", Bytes: certificate.der})
	}
	return result, canonical.String(), nil
}

// ValidateManifestClientConfig is pure and available even in the incompatible
// test hash build. Successful output fits and round-trips through its parser.
func ValidateManifestClientConfig(config ManifestClientConfig) (ManifestClientConfig, error) {
	if config.Version != Version3 || len(config.URL) > MaxClientConfigSize || len(config.Token) > MaxClientConfigSize || len(config.CACertificate) > MaxClientConfigSize {
		return ManifestClientConfig{}, errManifestConfig
	}
	if _, err := manifestPin(config.AuthorityPin); err != nil {
		return ManifestClientConfig{}, errManifestConfig
	}
	normalized, _, err := validateClientConfig(ClientConfig{Version: Version2, URL: config.URL, Token: config.Token, CACertificate: config.CACertificate})
	if err != nil {
		return ManifestClientConfig{}, errManifestConfig
	}
	config.URL, config.Token = normalized.URL, normalized.Token
	if config.CACertificate != "" {
		_, config.CACertificate, err = manifestCertificates(config.CACertificate)
		if err != nil {
			return ManifestClientConfig{}, errManifestConfig
		}
	}
	if !manifestConfigFits(config) {
		return ManifestClientConfig{}, errManifestConfig
	}
	return config, nil
}

func ParseManifestClientConfig(raw []byte) (ManifestClientConfig, error) {
	if manifestJSONShape(raw, MaxClientConfigSize, 2, nil) != nil {
		return ManifestClientConfig{}, errManifestConfig
	}
	fields, err := parseObject(raw, MaxClientConfigSize, "version", "url", "token", "ca_certificate", "authority_pin")
	var config ManifestClientConfig
	if err != nil {
		return config, errManifestConfig
	}
	for name, target := range map[string]any{"version": &config.Version, "url": &config.URL, "token": &config.Token, "ca_certificate": &config.CACertificate} {
		if decodeField(fields, name, target) != nil {
			return ManifestClientConfig{}, errManifestConfig
		}
	}
	config.AuthorityPin, err = parseManifestPin(fields["authority_pin"])
	if err != nil {
		return ManifestClientConfig{}, errManifestConfig
	}
	return ValidateManifestClientConfig(config)
}

// ManifestTransportTrustID binds normalized origin and CA trust policy, never
// credentials or the independent registry pin. System roots remain OS-managed.
func ManifestTransportTrustID(config ManifestClientConfig) (string, error) {
	config, err := ValidateManifestClientConfig(config)
	if err != nil {
		return "", err
	}
	mode := byte(2)
	var certificates []manifestCertificate
	if strings.HasPrefix(config.URL, "http://") {
		mode = 1
	} else if config.CACertificate != "" {
		mode = 3
		certificates, _, err = manifestCertificates(config.CACertificate)
		if err != nil {
			return "", err
		}
	}
	var preimage bytes.Buffer
	preimage.WriteString("SLUG manifest transport trust\x00v1\x00")
	_ = binary.Write(&preimage, binary.LittleEndian, uint32(len(config.URL)))
	preimage.WriteString(config.URL)
	preimage.WriteByte(mode)
	_ = binary.Write(&preimage, binary.LittleEndian, uint32(len(certificates)))
	for _, certificate := range certificates {
		preimage.Write(certificate.fingerprint[:])
	}
	digest := sha256.Sum256(preimage.Bytes())
	return hex.EncodeToString(digest[:]), nil
}
