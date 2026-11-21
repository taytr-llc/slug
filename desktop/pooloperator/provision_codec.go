// Copyright (c) 2026 The SLUG developers. ISC license.
package pooloperator

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"io"
	"net"
	"net/url"
	"time"

	"github.com/taytr-llc/slug/btcutil/v2/hdkeychain"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slugwallet/recovery"
	"github.com/taytr-llc/slugwallet/waddrmgr"
)

const provisionIntentName = "SLUG-POOL-PROVISION-INTENT-V1.json"
const provisionReadyName = "SLUG-POOL-PROVISION-READY-V1.json"
const provisionBackupName = "SLUG-OPERATOR-POOL-WALLET-BACKUP-V1.json"
const provisionMnemonicProfile = "bip39-english-256-empty-passphrase"
const provisionFundingPath = "m/84'/0'/0'/0/0"

type provisionIntent struct {
	Version           uint32          `json:"version"`
	Policy            ProvisionPolicy `json:"policy"`
	PolicyID          string          `json:"policy_id"`
	CertificateSHA256 string          `json:"certificate_sha256"`
	CreatedAtMS       int64           `json:"created_at_ms"`
}
type provisionReady struct {
	Version       uint32 `json:"version"`
	RequestID     string `json:"request_id"`
	PolicyID      string `json:"policy_id"`
	SetupID       string `json:"setup_id"`
	WalletGuardID string `json:"wallet_guard_id"`
	CreatedAtMS   int64  `json:"created_at_ms"`
}
type provisionBackup struct {
	Version         uint32                       `json:"version"`
	Kind            string                       `json:"kind"`
	RequestID       string                       `json:"request_id"`
	PolicyID        string                       `json:"policy_id"`
	SetupID         string                       `json:"setup_id"`
	CreatedAtMS     int64                        `json:"created_at_ms"`
	MnemonicProfile string                       `json:"mnemonic_profile"`
	Mnemonic        string                       `json:"mnemonic"`
	DerivationID    string                       `json:"derivation_id"`
	FundingPath     string                       `json:"funding_path"`
	Network         uint32                       `json:"network"`
	Genesis         string                       `json:"genesis"`
	WalletGuard     waddrmgr.PoolSettlementGuard `json:"wallet_guard"`
	WalletGuardID   string                       `json:"wallet_guard_id"`
}

func (provisionBackup) String() string   { return "protected operator wallet backup" }
func (provisionBackup) GoString() string { return "protected operator wallet backup" }

func provisionDecode(raw []byte, limit int, out any) error {
	if len(raw) < 1 || len(raw) > limit {
		return ErrProvisionInvalid
	}
	for _, b := range raw {
		if b > 127 {
			return ErrProvisionInvalid
		}
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	tokens := 0
	var walk func(int) error
	walk = func(depth int) error {
		tokens++
		if depth > 8 || tokens > 8192 {
			return ErrProvisionInvalid
		}
		token, e := d.Token()
		if e != nil {
			return ErrProvisionInvalid
		}
		switch x := token.(type) {
		case json.Delim:
			if x != '{' && x != '[' {
				return ErrProvisionInvalid
			}
			seen := map[string]bool{}
			count := 0
			for d.More() {
				count++
				if count > 64 || x == '{' && count > 16 {
					return ErrProvisionInvalid
				}
				if x == '{' {
					t, e := d.Token()
					k, ok := t.(string)
					if e != nil || !ok || len(k) > 64 || seen[k] {
						return ErrProvisionInvalid
					}
					seen[k] = true
					tokens++
				}
				if e := walk(depth + 1); e != nil {
					return e
				}
			}
			t, e := d.Token()
			if e != nil || x == '{' && t != json.Delim('}') || x == '[' && t != json.Delim(']') {
				return ErrProvisionInvalid
			}
		case string:
			if len(x) > 4096 {
				return ErrProvisionInvalid
			}
		case json.Number:
			if len(x) < 1 || len(x) > 16 || len(x) > 1 && x[0] == '0' {
				return ErrProvisionInvalid
			}
			for _, b := range []byte(x) {
				if b < '0' || b > '9' {
					return ErrProvisionInvalid
				}
			}
		default:
			return ErrProvisionInvalid
		}
		return nil
	}
	if walk(0) != nil {
		return ErrProvisionInvalid
	}
	if _, e := d.Token(); e != io.EOF {
		return ErrProvisionInvalid
	}
	td := json.NewDecoder(bytes.NewReader(raw))
	td.DisallowUnknownFields()
	if td.Decode(out) != nil {
		return ErrProvisionInvalid
	}
	canonical, e := json.Marshal(out)
	if e != nil || !bytes.Equal(canonical, raw) {
		return ErrProvisionInvalid
	}
	return nil
}
func ParseProvisionPolicy(raw []byte) (ProvisionPolicy, error) {
	return parseProvisionPolicyVersion(raw, 1)
}
func parseProvisionPolicyVersion(raw []byte, version uint32) (ProvisionPolicy, error) {
	var p ProvisionPolicy
	if len(raw) > MaxProvisionPolicyBytes+1 {
		return p, ErrProvisionInvalid
	}
	if len(raw) > 0 && raw[len(raw)-1] == '\n' {
		raw = raw[:len(raw)-1]
	}
	if provisionDecode(raw, MaxProvisionPolicyBytes, &p) != nil || validateProvisionPolicyVersion(p, version) != nil {
		return ProvisionPolicy{}, ErrProvisionInvalid
	}
	return p, nil
}
func provisionListen(raw string) bool {
	_, port, e := net.SplitHostPort(raw)
	return e == nil && port != "0"
}
func provisionJSON(v any, limit int) ([]byte, error) {
	raw, e := json.Marshal(v)
	if e != nil || len(raw)+1 > limit {
		return nil, ErrProvisionInvalid
	}
	return append(raw, '\n'), nil
}
func provisionJSONFile(raw []byte, limit int, out any) error {
	if len(raw) < 2 || len(raw) > limit || raw[len(raw)-1] != '\n' {
		return ErrProvisionInvalid
	}
	return provisionDecode(raw[:len(raw)-1], limit-1, out)
}
func parseProvisionIntent(raw []byte, p ProvisionPolicy, c ProvisionCertificate) (provisionIntent, error) {
	var v provisionIntent
	if provisionJSONFile(raw, 131072, &v) != nil {
		return v, ErrProvisionInvalid
	}
	id, e := provisionPolicyIDVersion(p, p.Version)
	want, _ := json.Marshal(p)
	got, _ := json.Marshal(v.Policy)
	if e != nil || v.Version != p.Version || v.PolicyID != id || v.CertificateSHA256 != c.CertificateSHA256 || !provisionTime(v.CreatedAtMS) || !bytes.Equal(want, got) {
		return provisionIntent{}, ErrProvisionInvalid
	}
	return v, nil
}
func makeProvisionReady(p ProvisionPolicy, s Setup, created int64) (provisionReady, error) {
	pid, e := provisionPolicyIDVersion(p, p.Version)
	if e != nil {
		return provisionReady{}, e
	}
	sid, e := setupIDVersion(s, p.Version)
	if e != nil {
		return provisionReady{}, e
	}
	gid, e := waddrmgr.PoolSettlementGuardDigest(s.WalletGuard)
	if e != nil || !provisionTime(created) {
		return provisionReady{}, ErrProvisionInvalid
	}
	return provisionReady{p.Version, p.RequestID, pid, sid, gid, created}, nil
}
func deriveProvisionMaterial(capsule []byte, created int64) (m ProvisionPublicMaterial, root *hdkeychain.ExtendedKey, err error) {
	if len(capsule) != 128 || !provisionTime(created) {
		return m, nil, ErrProvisionInvalid
	}
	var entropy [32]byte
	copy(entropy[:], capsule[:32])
	defer clear(entropy[:])
	seed := recovery.SeedFromEntropy(entropy)
	defer clear(seed[:])
	root, err = hdkeychain.NewMaster(seed[:], &chaincfg.MainNetParams)
	if err != nil {
		return m, nil, ErrProvisionInvalid
	}
	ok := false
	defer func() {
		if !ok {
			root.Zero()
			root = nil
		}
	}()
	pub, e := root.Neuter()
	if e != nil {
		return m, root, ErrProvisionInvalid
	}
	// The public alias borrows root buffers; never Zero it before root is finished.
	m = ProvisionPublicMaterial{Version: 1, CreatedAtMS: created, RootPublicKey: pub.String()}
	current := root
	for _, index := range []uint32{84 + hdkeychain.HardenedKeyStart, hdkeychain.HardenedKeyStart, hdkeychain.HardenedKeyStart, 0, 0} {
		next, e := current.Derive(index)
		if current != root {
			current.Zero()
		}
		if e != nil {
			return m, root, ErrProvisionInvalid
		}
		current = next
	}
	point, e := current.ECPubKey()
	if e != nil {
		current.Zero()
		return m, root, ErrProvisionInvalid
	}
	m.FundingPublicKey = hex.EncodeToString(point.SerializeCompressed())
	current.Zero()
	keys := []*string{&m.AdmissionKey, &m.PublicationKey, &m.CutoffKey}
	for i, dst := range keys {
		private := ed25519.NewKeyFromSeed(capsule[32+i*32 : 64+i*32])
		*dst = hex.EncodeToString(private.Public().(ed25519.PublicKey))
		clear(private)
	}
	ok = true
	return m, root, nil
}
func provisionCertificate(raw []byte, p ProvisionPolicy, now time.Time) (ProvisionCertificate, error) {
	if len(raw) < 1 || len(raw) > 65536 {
		return ProvisionCertificate{}, ErrProvisionInvalid
	}
	digest := sha256.Sum256(raw)
	remain := raw
	var leaf *x509.Certificate
	count := 0
	for len(bytes.TrimSpace(remain)) > 0 {
		b, next := pem.Decode(remain)
		if b == nil || b.Type != "CERTIFICATE" || len(b.Headers) != 0 {
			return ProvisionCertificate{}, ErrProvisionInvalid
		}
		cert, e := x509.ParseCertificate(b.Bytes)
		if e != nil {
			return ProvisionCertificate{}, ErrProvisionInvalid
		}
		if leaf == nil {
			leaf = cert
		}
		count++
		if count > 16 {
			return ProvisionCertificate{}, ErrProvisionInvalid
		}
		remain = next
	}
	u, e := url.Parse(p.Transport.Origin)
	if e != nil || leaf == nil || leaf.IsCA || now.Before(leaf.NotBefore) || !now.Before(leaf.NotAfter) || leaf.VerifyHostname(u.Hostname()) != nil {
		return ProvisionCertificate{}, ErrProvisionInvalid
	}
	usage := len(leaf.ExtKeyUsage) == 0
	for _, v := range leaf.ExtKeyUsage {
		if v == x509.ExtKeyUsageAny || v == x509.ExtKeyUsageServerAuth {
			usage = true
		}
	}
	if !usage {
		return ProvisionCertificate{}, ErrProvisionInvalid
	}
	spki := sha256.Sum256(leaf.RawSubjectPublicKeyInfo)
	return ProvisionCertificate{1, hex.EncodeToString(digest[:]), hex.EncodeToString(spki[:])}, nil
}
func provisionBackupBytes(entropy [32]byte, p ProvisionPolicy, s Setup, created int64) ([]byte, error) {
	defer clear(entropy[:])
	ready, e := makeProvisionReady(p, s, created)
	if e != nil {
		return nil, e
	}
	// Recheck private-to-public correspondence; a parsed public guard alone is not enough.
	var capsule [128]byte
	copy(capsule[:32], entropy[:])
	m, root, e := deriveProvisionMaterial(capsule[:], created)
	clear(capsule[:])
	if e != nil {
		return nil, e
	}
	defer root.Zero()
	if m.RootPublicKey != s.WalletGuard.RootPublicKey || m.FundingPublicKey != s.WalletGuard.FundingPublicKey {
		return nil, ErrProvisionInvalid
	}
	v := provisionBackup{p.Version, "slug-operator-pool-wallet-backup", p.RequestID, ready.PolicyID, ready.SetupID, created, provisionMnemonicProfile, recovery.Mnemonic(entropy), waddrmgr.PoolSettlementDerivation, provisionFundingPath, p.Network, p.Genesis, s.WalletGuard, ready.WalletGuardID}
	return provisionJSON(v, 8192)
}
func verifyProvisionBackup(raw []byte, p ProvisionPolicy, s Setup, created int64) error {
	var v provisionBackup
	if provisionJSONFile(raw, 8192, &v) != nil {
		return ErrProvisionInvalid
	}
	entropy, e := recovery.Entropy(v.Mnemonic)
	if e != nil {
		return ErrProvisionInvalid
	}
	defer clear(entropy[:])
	expected, e := provisionBackupBytes(entropy, p, s, created)
	defer clear(expected)
	if e != nil || !bytes.Equal(raw, expected) {
		return ErrProvisionInvalid
	}
	return nil
}
