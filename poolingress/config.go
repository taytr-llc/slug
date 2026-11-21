// Copyright (c) 2026 The SLUG developers. ISC license.
// Package poolingress exposes only the fixed public pool/admission/ownership
// routes over TLS. It grants no source, miner, signer or payment authority.
package poolingress

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var (
	ErrConfig      = errors.New("pool ingress configuration unavailable")
	ErrClosed      = errors.New("pool ingress closed")
	ErrUnavailable = errors.New("pool ingress unavailable")
)

// Config is public transport configuration selected by the original outer
// operator. Upstream responses cannot change it. The outer setup also binds
// the operator profile, services, enrollment and their original owners.
type Config struct {
	Version         uint32 `json:"version"`
	Listen          string `json:"listen"`
	Origin          string `json:"origin"`
	CertificateSPKI string `json:"certificate_spki"`
	PoolURL         string `json:"pool_url"`
	AdmissionURL    string `json:"admission_url"`
	OwnershipURL    string `json:"ownership_url"`
}

func portOK(p string, zero bool) bool {
	n, e := strconv.ParseUint(p, 10, 16)
	return e == nil && (zero || n > 0) && strconv.FormatUint(n, 10) == p
}
func hostnameOK(h string) bool {
	if h == "" || len(h) > 253 || strings.ToLower(h) != h {
		return false
	}
	if ip := net.ParseIP(h); ip != nil {
		return ip.String() == h
	}
	for _, label := range strings.Split(h, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-') {
				return false
			}
		}
	}
	return true
}
func origin(raw, scheme string, loopback bool) (*url.URL, error) {
	if len(raw) == 0 || len(raw) > 512 {
		return nil, ErrConfig
	}
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != scheme || u.User != nil || u.Opaque != "" || u.Path != "" || u.RawPath != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawFragment != "" || !hostnameOK(u.Hostname()) {
		return nil, ErrConfig
	}
	h := u.Hostname()
	host := h
	if strings.Contains(h, ":") {
		host = "[" + h + "]"
	}
	if p := u.Port(); p != "" {
		if !portOK(p, false) {
			return nil, ErrConfig
		}
		host = net.JoinHostPort(h, p)
	}
	if host != u.Host || u.String() != raw {
		return nil, ErrConfig
	}
	if loopback {
		ip := net.ParseIP(h)
		if ip == nil || !ip.IsLoopback() || u.Port() == "" {
			return nil, ErrConfig
		}
	}
	return u, nil
}

// ValidateConfig checks exact public transport choices without TLS material,
// filesystem access, normalization or network activity.
func ValidateConfig(c Config) error { _, _, e := validateConfig(c); return e }

func validateConfig(c Config) (*url.URL, [3]*url.URL, error) { return validateSelectedConfig(c, 1) }
func validateSelectedConfig(c Config, version uint32) (*url.URL, [3]*url.URL, error) {
	var targets [3]*url.URL
	if (version != 1 && version != 2) || c.Version != version || len(c.Listen) > 128 || len(c.CertificateSPKI) != 64 {
		return nil, targets, ErrConfig
	}
	h, p, e := net.SplitHostPort(c.Listen)
	ip := net.ParseIP(h)
	if e != nil || ip == nil || ip.String() != h || !portOK(p, true) || net.JoinHostPort(h, p) != c.Listen {
		return nil, targets, ErrConfig
	}
	external, e := origin(c.Origin, "https", false)
	if e != nil {
		return nil, targets, e
	}
	raw, e := hex.DecodeString(c.CertificateSPKI)
	if e != nil || len(raw) != 32 || hex.EncodeToString(raw) != c.CertificateSPKI || c.CertificateSPKI == strings.Repeat("0", 64) {
		return nil, targets, ErrConfig
	}
	for i, raw := range []string{c.PoolURL, c.AdmissionURL, c.OwnershipURL} {
		targets[i], e = origin(raw, "http", true)
		if e != nil {
			return nil, targets, e
		}
	}
	return external, targets, nil
}
func validLeaf(leaf *x509.Certificate, now time.Time) bool {
	if leaf == nil || now.Before(leaf.NotBefore) || !now.Before(leaf.NotAfter) || leaf.IsCA {
		return false
	}
	if len(leaf.ExtKeyUsage) == 0 {
		return true
	}
	for _, usage := range leaf.ExtKeyUsage {
		if usage == x509.ExtKeyUsageAny || usage == x509.ExtKeyUsageServerAuth {
			return true
		}
	}
	return false
}
func certificateConfig(c Config, external *url.URL, certPEM, keyPEM []byte) (*tls.Config, error) {
	if len(certPEM) == 0 || len(certPEM) > 65536 || len(keyPEM) == 0 || len(keyPEM) > 16384 {
		return nil, ErrConfig
	}
	// X509KeyPair parses into owned DER/key values; no caller TLS object or
	// callback survives. Do not expose certificates/private keys in Config.
	cert, e := tls.X509KeyPair(certPEM, keyPEM)
	if e != nil || len(cert.Certificate) == 0 {
		return nil, ErrConfig
	}
	leaf, e := x509.ParseCertificate(cert.Certificate[0])
	if e != nil || !validLeaf(leaf, time.Now()) || leaf.VerifyHostname(external.Hostname()) != nil {
		return nil, ErrConfig
	}
	digest := sha256.Sum256(leaf.RawSubjectPublicKeyInfo)
	if hex.EncodeToString(digest[:]) != c.CertificateSPKI {
		return nil, ErrConfig
	}
	cert.Leaf = leaf
	cfg := &tls.Config{MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13, NextProtos: []string{"http/1.1"}}
	cfg.GetCertificate = func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		if !validLeaf(leaf, time.Now()) {
			return nil, ErrConfig
		}
		name := external.Hostname()
		if net.ParseIP(name) != nil {
			if hello.ServerName != "" && hello.ServerName != name {
				return nil, ErrConfig
			}
		} else if hello.ServerName != name {
			return nil, ErrConfig
		}
		return &cert, nil
	}
	return cfg, nil
}
