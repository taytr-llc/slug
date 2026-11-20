// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"reflect"
	"strings"

	"github.com/taytr-llc/slug/internal/ownedcodec"
	"github.com/taytr-llc/slug/poolminer"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/prospectworker"
	"github.com/taytr-llc/slug/registry"
)

const RemotePoolMaxConfigBytes = 262144
const remotePoolApplication = "slug-remote-pool-worker"
const remotePoolIntentName = "SLUG-REMOTE-POOL-INTENT.json"
const remotePoolReadyName = "SLUG-REMOTE-POOL-READY.json"

var ErrRemotePoolConfig = errors.New("remote pool configuration unavailable")

// These are public expected identities, never proof of a live wallet or source.
// OriginalSourceBindingID remains the original local wallet's source.
type RemotePoolLocalBinding struct {
	Version                 uint32               `json:"version"`
	ManagerSchema           uint32               `json:"manager_schema"`
	WalletOwnerID           string               `json:"wallet_owner_id"`
	AllocationBindingID     string               `json:"allocation_binding_id"`
	OriginalSourceBindingID string               `json:"original_source_binding_id"`
	Recipient               PoolRecipientBinding `json:"recipient"`
}

type RemotePoolConfig struct {
	Version       uint32                     `json:"version"`
	ConnectionID  string                     `json:"connection_id"`
	OperatorPin   registry.PublicManifestPin `json:"operator_pin"`
	Source        poolshare.Config           `json:"source"`
	Settlement    poolsettle.Config          `json:"settlement"`
	Miners        poolminer.Config           `json:"miners"`
	Origin        string                     `json:"origin"`
	CACertificate string                     `json:"ca_certificate"`
	ExpectedLocal RemotePoolLocalBinding     `json:"expected_local"`
	MaxRecords    uint64                     `json:"max_records"`
	MaxBytes      uint64                     `json:"max_bytes"`
}

func remotePoolID(s string) bool {
	return ownedcodec.Hex(s, 32) && s != strings.Repeat("0", 64)
}

func remotePoolOrigin(u *url.URL, text string) bool {
	if u == nil || u.Scheme != "https" || u.Host == "" || u.User != nil || u.Path != "" || u.RawPath != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawFragment != "" || u.Opaque != "" || u.String() != text || strings.ToLower(u.Host) != u.Host {
		return false
	}
	h := u.Hostname()
	if ip := net.ParseIP(h); ip != nil {
		return ip.String() == h
	}
	if len(h) == 0 || len(h) > 253 {
		return false
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

// Bound typed caller input before marshaling or invoking inherited validators.
func remotePoolValueBound(v reflect.Value, depth int, left *int) bool {
	if depth > 24 || !v.IsValid() || *left < 0 {
		return false
	}
	switch v.Kind() {
	case reflect.String:
		if v.Len() > *left {
			return false
		}
		*left -= v.Len()
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if !remotePoolValueBound(v.Field(i), depth+1, left) {
				return false
			}
		}
	case reflect.Slice, reflect.Array:
		if v.Len() > 128 {
			return false
		}
		for i := 0; i < v.Len(); i++ {
			if !remotePoolValueBound(v.Index(i), depth+1, left) {
				return false
			}
		}
	case reflect.Bool, reflect.Int, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint32, reflect.Uint64:
	default:
		return false
	}
	return true
}

// ValidateRemotePoolConfig is pure. It grants no local signing/wallet authority,
// remote enrollment acceptance, current chain evidence or ownership history.
func ValidateRemotePoolConfig(c RemotePoolConfig) error {
	budget := RemotePoolMaxConfigBytes
	if !remotePoolValueBound(reflect.ValueOf(c), 0, &budget) || c.Version != 1 || !remotePoolID(c.ConnectionID) ||
		len(c.Miners.Enrollments) < 1 || len(c.Miners.Enrollments) > 64 || len(c.Settlement.Destinations) > 64 ||
		c.MaxRecords < 1 || c.MaxRecords > 4096 || c.MaxBytes < 64<<20 || c.MaxBytes > 1<<30 {
		return ErrRemotePoolConfig
	}
	p, e := registry.NewOperatorProfile(c.OperatorPin)
	if e != nil || poolminer.ValidateConfig(c.Miners, poolminer.Binding{Profile: p, Source: c.Source, Settlement: c.Settlement}) != nil {
		return ErrRemotePoolConfig
	}
	l := c.ExpectedLocal
	if l.Version != 1 || l.ManagerSchema != 14 || !remotePoolID(l.WalletOwnerID) || !remotePoolID(l.AllocationBindingID) ||
		ValidatePoolRecipientBinding(l.Recipient) != nil || l.OriginalSourceBindingID != l.Recipient.SourceBindingID ||
		l.Recipient.Config.Source.Policy != c.Source.Policy {
		return ErrRemotePoolConfig
	}
	identity := l.Recipient.Config.Identity
	matched := 0
	for _, en := range c.Miners.Enrollments {
		if en.GymID != identity.GymID {
			continue
		}
		if en.PublicKey != identity.GymPublicKey || en.PayeeScript != l.Recipient.Recipient.Script {
			return ErrRemotePoolConfig
		}
		matched++
	}
	if matched != 1 {
		return ErrRemotePoolConfig
	}
	u, e := url.Parse(c.Origin)
	if e != nil || !remotePoolOrigin(u, c.Origin) || c.CACertificate == "" {
		return ErrRemotePoolConfig
	}
	pc, e := poolservice.ValidateClientConfig(remotePoolClientConfig(c, strings.Repeat("1", 64)))
	if e != nil || pc.URL != c.Origin || pc.CACertificate != c.CACertificate {
		return ErrRemotePoolConfig
	}
	ac, e := prospectservice.ValidateManifestClientConfig(remotePoolAdmissionConfig(c, strings.Repeat("1", 64)))
	if e != nil || ac.URL != c.Origin || ac.CACertificate != c.CACertificate {
		return ErrRemotePoolConfig
	}
	w, e := remotePoolWorkerConfig(c)
	if e != nil {
		return ErrRemotePoolConfig
	}
	if _, e = prospectworker.ValidatePoolConfig(w); e != nil {
		return ErrRemotePoolConfig
	}
	b, e := json.Marshal(c)
	if e != nil || len(b) > RemotePoolMaxConfigBytes {
		return ErrRemotePoolConfig
	}
	for _, x := range b {
		if x > 127 {
			return ErrRemotePoolConfig
		}
	}
	return nil
}

func remotePoolClientConfig(c RemotePoolConfig, token string) poolservice.ClientConfig {
	return poolservice.ClientConfig{Version: 1, URL: c.Origin, Token: token, CACertificate: c.CACertificate, PublicKey: c.ExpectedLocal.Recipient.Config.Identity.GymPublicKey, Source: c.Source}
}
func remotePoolAdmissionConfig(c RemotePoolConfig, token string) prospectservice.ManifestClientConfig {
	return prospectservice.ManifestClientConfig{Version: 3, URL: c.Origin, Token: token, CACertificate: c.CACertificate, AuthorityPin: c.Source.AuthorityPin}
}
func remotePoolWorkerConfig(c RemotePoolConfig) (prospectworker.PoolConfig, error) {
	pt, e := poolservice.TransportTrustID(remotePoolClientConfig(c, strings.Repeat("1", 64)))
	if e != nil {
		return prospectworker.PoolConfig{}, ErrRemotePoolConfig
	}
	at, e := prospectservice.ManifestTransportTrustID(remotePoolAdmissionConfig(c, strings.Repeat("1", 64)))
	if e != nil {
		return prospectworker.PoolConfig{}, ErrRemotePoolConfig
	}
	return prospectworker.PoolConfig{Version: 1, Source: c.Source, PublicKey: c.ExpectedLocal.Recipient.Config.Identity.GymPublicKey, AuthorityOrigin: c.Origin, TransportTrustID: pt, AdmissionOrigin: c.Origin, AdmissionTrustID: at, MaxRecords: c.MaxRecords, MaxBytes: c.MaxBytes}, nil
}

func MarshalRemotePoolConfig(c RemotePoolConfig) ([]byte, error) {
	if ValidateRemotePoolConfig(c) != nil {
		return nil, ErrRemotePoolConfig
	}
	return json.Marshal(c)
}
func ParseRemotePoolConfig(b []byte) (RemotePoolConfig, error) {
	var c RemotePoolConfig
	if ownedcodec.Decode(b, RemotePoolMaxConfigBytes, &c) != nil || ValidateRemotePoolConfig(c) != nil {
		return RemotePoolConfig{}, ErrRemotePoolConfig
	}
	return c, nil
}
func RemotePoolConfigID(c RemotePoolConfig) (string, error) {
	b, e := MarshalRemotePoolConfig(c)
	if e != nil {
		return "", e
	}
	h := sha256.New()
	h.Write([]byte("SLUG remote pool attachment\x00v1\x00"))
	var n [4]byte
	binary.LittleEndian.PutUint32(n[:], uint32(len(b)))
	h.Write(n[:])
	h.Write(b)
	return hex.EncodeToString(h.Sum(nil)), nil
}

type remotePoolIntent struct {
	Version     uint32           `json:"version"`
	Application string           `json:"application"`
	ConfigID    string           `json:"config_id"`
	Config      RemotePoolConfig `json:"config"`
}
type remotePoolReady struct {
	Version      uint32 `json:"version"`
	Application  string `json:"application"`
	ConfigID     string `json:"config_id"`
	IntentSHA256 string `json:"intent_sha256"`
}

func remotePoolMarkerBytes(c RemotePoolConfig) ([]byte, []byte, error) {
	id, e := RemotePoolConfigID(c)
	if e != nil {
		return nil, nil, e
	}
	i, e := json.Marshal(remotePoolIntent{1, remotePoolApplication, id, c})
	if e != nil {
		return nil, nil, ErrRemotePoolConfig
	}
	i = append(i, '\n')
	h := sha256.Sum256(i)
	r, e := json.Marshal(remotePoolReady{1, remotePoolApplication, id, hex.EncodeToString(h[:])})
	if e != nil {
		return nil, nil, ErrRemotePoolConfig
	}
	return i, append(r, '\n'), nil
}
func remotePoolValidateMarkers(intent, ready []byte, expected RemotePoolConfig) error {
	if len(intent) > RemotePoolMaxConfigBytes+512 || len(ready) > 1024 {
		return ErrRemotePoolConfig
	}
	i, r, e := remotePoolMarkerBytes(expected)
	if e != nil || !bytes.Equal(intent, i) || !bytes.Equal(ready, r) {
		return ErrRemotePoolConfig
	}
	return nil
}
