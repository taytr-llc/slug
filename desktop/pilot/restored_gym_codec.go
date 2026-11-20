// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/taytr-llc/slug/fightercarrier"
	"github.com/taytr-llc/slug/fighterownership"
	"github.com/taytr-llc/slugwallet/recovery"
	"github.com/taytr-llc/slugwallet/waddrmgr"
)

var ErrRestoredGym = errors.New("restored gym unavailable")
var ErrRestoredGymStorage = errors.New("restored gym storage unavailable")

const restoredInitName = "SLUG-RESTORED-GYM-INIT.json"
const restoredActivateName = "SLUG-RESTORED-GYM-ACTIVATE.json"
const restoredReadyName = "SLUG-RESTORED-GYM-READY.json"
const restoredInitLimit = 32768
const restoredActivateLimit = 16384
const restoredReadyLimit = 4096

type restoredPublicRead struct {
	URL           string `json:"url"`
	TLSSPKISHA256 string `json:"tls_spki_sha256"`
}
type restoredNodeBinding struct {
	URL                 string `json:"url"`
	CACertificateSHA256 string `json:"ca_certificate_sha256"`
}
type restoredCarrierIdentity struct {
	JournalID string             `json:"journal_id"`
	Pin       fightercarrier.Pin `json:"pin"`
}

func (i restoredCarrierIdentity) identity() fightercarrier.Identity {
	return fightercarrier.Identity{JournalID: i.JournalID, Pin: i.Pin}
}

type restoredSetupInit struct {
	Version                uint32                  `json:"version"`
	Mode                   string                  `json:"mode"`
	SetupID                string                  `json:"setup_id"`
	WalletOwnerID          string                  `json:"wallet_owner_id"`
	CarrierJournalID       string                  `json:"carrier_journal_id"`
	OwnershipStoreID       string                  `json:"ownership_store_id"`
	Ownership              fighterownership.Pin    `json:"ownership"`
	OwnershipPinID         string                  `json:"ownership_pin_id"`
	PublicRead             restoredPublicRead      `json:"public_read"`
	NodeBinding            restoredNodeBinding     `json:"node_binding"`
	OriginalFileBytes      uint32                  `json:"original_file_bytes"`
	OriginalFileSHA256     string                  `json:"original_file_sha256"`
	OriginalManifest       recovery.Manifest       `json:"original_manifest"`
	OriginalManifestSHA256 string                  `json:"original_manifest_sha256"`
	CarrierIdentity        restoredCarrierIdentity `json:"carrier_identity"`
	CarrierBindingID       string                  `json:"carrier_binding_id"`
	RestoreSchema          uint32                  `json:"restore_schema"`
	ActiveSchema           uint32                  `json:"active_schema"`
}
type restoredSetupActivate struct {
	Version     uint32                        `json:"version"`
	SetupID     string                        `json:"setup_id"`
	InitDigest  string                        `json:"init_digest"`
	Guard       waddrmgr.RestoredCarrierGuard `json:"guard"`
	GuardDigest string                        `json:"guard_digest"`
}
type restoredSetupReady struct {
	Version          uint32 `json:"version"`
	SetupID          string `json:"setup_id"`
	InitDigest       string `json:"init_digest"`
	ActivateDigest   string `json:"activate_digest"`
	GuardDigest      string `json:"guard_digest"`
	CarrierBindingID string `json:"carrier_binding_id"`
	OwnershipPinID   string `json:"ownership_pin_id"`
	OwnershipStoreID string `json:"ownership_store_id"`
}

func restoredCanonical(v any) ([]byte, error) {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	if e.Encode(v) != nil {
		return nil, ErrRestoredGym
	}
	return bytes.TrimSuffix(b.Bytes(), []byte{'\n'}), nil
}
func restoredSHA(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func restoredDigest(kind string, v any) string {
	b, _ := restoredCanonical(v)
	return restoredSHA(append([]byte("SLUG restored gym "+kind+"\x00v1\x00"), b...))
}
func restoredEqual(a, b any) bool {
	x, e := restoredCanonical(a)
	y, f := restoredCanonical(b)
	return e == nil && f == nil && bytes.Equal(x, y)
}
func restoredMarkerBytes(v any, limit int) ([]byte, error) {
	b, e := restoredCanonical(v)
	if e != nil || len(b)+1 > limit {
		return nil, ErrRestoredGym
	}
	return append(b, '\n'), nil
}
func restoredParseMarker(raw []byte, limit int, out any) error {
	if len(raw) == 0 || len(raw) > limit || raw[len(raw)-1] != '\n' {
		return ErrRestoredGymStorage
	}
	if _, e := poolDecode(raw, limit, 10, out); e != nil {
		return ErrRestoredGymStorage
	}
	again, e := restoredMarkerBytes(out, limit)
	if e != nil || !bytes.Equal(raw, again) {
		return ErrRestoredGymStorage
	}
	return nil
}
func restoredEndpoint(raw string) (*url.URL, error) {
	u, e := url.Parse(raw)
	if e != nil || len(raw) > 2048 || u.Hostname() == "" || u.User != nil || u.Opaque != "" || u.RawPath != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.ContainsAny(raw, "\r\n\t#") || strings.Contains(u.Hostname(), "%") {
		return nil, ErrRestoredGym
	}
	if p := u.Port(); p != "" {
		n, e := strconv.Atoi(p)
		if e != nil || n < 1 || n > 65535 || strconv.Itoa(n) != p {
			return nil, ErrRestoredGym
		}
	}
	return u, nil
}
func restoredValidateTransport(public restoredPublicRead, node restoredNodeBinding) error {
	u, e := restoredEndpoint(public.URL)
	if e != nil || u.Path != "/slug/ownership/v1" {
		return ErrRestoredGym
	}
	if u.Scheme == "http" {
		ip := net.ParseIP(u.Hostname())
		if ip == nil || !ip.IsLoopback() || public.TLSSPKISHA256 != "" {
			return ErrRestoredGym
		}
	} else if u.Scheme != "https" || !payoutNonzeroID(public.TLSSPKISHA256) {
		return ErrRestoredGym
	}
	u, e = restoredEndpoint(node.URL)
	if e != nil || u.Scheme != "http" || u.Path != "" && u.Path != "/" || node.CACertificateSHA256 != restoredSHA(nil) {
		return ErrRestoredGym
	}
	ip := net.ParseIP(u.Hostname())
	if ip == nil || !ip.IsLoopback() {
		return ErrRestoredGym
	}
	return nil
}
func restoredValidateInit(i restoredSetupInit, expected *restoredSetupInit) error {
	if i.Version != 1 || i.Mode != "file-restored-gym" || i.RestoreSchema != 12 || i.ActiveSchema != 15 || i.OriginalFileBytes < 1 || i.OriginalFileBytes > recovery.MaxExportSize || !canonicalManifestID(i.OriginalFileSHA256) {
		return ErrRestoredGym
	}
	for _, id := range []string{i.SetupID, i.WalletOwnerID, i.CarrierJournalID, i.OwnershipStoreID} {
		if !payoutNonzeroID(id) {
			return ErrRestoredGym
		}
	}
	pid, e := fighterownership.ID(i.Ownership)
	if e != nil || i.OwnershipPinID != pid || restoredValidateTransport(i.PublicRead, i.NodeBinding) != nil {
		return ErrRestoredGym
	}
	m, e := recovery.MarshalManifest(i.OriginalManifest)
	if e != nil || i.OriginalManifestSHA256 != restoredSHA(m) {
		return ErrRestoredGym
	}
	f := i.OriginalManifest
	want := fightercarrier.Pin{Version: 1, OwnerMode: "restored", WalletOwnerID: i.WalletOwnerID, Network: f.Network, Genesis: f.Genesis, WalletProfile: f.Profile, RootPublicKey: f.RootPublicKey, GymPublicKey: f.GymPublicKey, GymID: f.GymID, ManagerSchema: 15, OriginalManifestSHA256: i.OriginalManifestSHA256, OwnershipPinID: pid}
	if i.CarrierIdentity.JournalID != i.CarrierJournalID || i.CarrierIdentity.Pin != want {
		return ErrRestoredGym
	}
	binding, e := fightercarrier.BindingID(i.CarrierIdentity.identity())
	if e != nil || binding != i.CarrierBindingID || expected != nil && !restoredEqual(i, *expected) {
		return ErrRestoredGym
	}
	_, e = restoredMarkerBytes(i, restoredInitLimit)
	return e
}
func restoredValidateActivate(a restoredSetupActivate, i restoredSetupInit, expected *waddrmgr.RestoredCarrierGuard) error {
	if restoredValidateInit(i, nil) != nil || a.Version != 1 || a.SetupID != i.SetupID || a.InitDigest != restoredDigest("setup", i) {
		return ErrRestoredGym
	}
	g := a.Guard
	digest, e := waddrmgr.RestoredCarrierGuardDigest(g)
	if e != nil || digest != a.GuardDigest || g.WalletOwnerID != i.WalletOwnerID || g.CarrierJournalID != i.CarrierJournalID || g.OwnershipPinID != i.OwnershipPinID || g.OriginalManifestSHA256 != i.OriginalManifestSHA256 || !restoredEqual(g.OriginalManifest, i.OriginalManifest) || expected != nil && !restoredEqual(g, *expected) {
		return ErrRestoredGym
	}
	_, e = restoredMarkerBytes(a, restoredActivateLimit)
	return e
}
func restoredValidateReady(r restoredSetupReady, i restoredSetupInit, a restoredSetupActivate) error {
	if restoredValidateActivate(a, i, nil) != nil || r.Version != 1 || r.SetupID != i.SetupID || r.InitDigest != a.InitDigest || r.ActivateDigest != restoredDigest("activation", a) || r.GuardDigest != a.GuardDigest || r.CarrierBindingID != i.CarrierBindingID || r.OwnershipPinID != i.OwnershipPinID || r.OwnershipStoreID != i.OwnershipStoreID {
		return ErrRestoredGym
	}
	_, e := restoredMarkerBytes(r, restoredReadyLimit)
	return e
}
