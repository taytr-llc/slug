// Copyright (c) 2026 The SLUG developers. ISC license.
package pooloperator

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/taytr-llc/slugwallet/recovery"
)

type provisionVector struct {
	Name         string                  `json:"name"`
	Policy       ProvisionPolicy         `json:"policy"`
	PolicyHex    string                  `json:"canonical_policy_hex"`
	PolicyFile   string                  `json:"policy_file_hex"`
	Material     ProvisionPublicMaterial `json:"public_material"`
	Certificate  ProvisionCertificate    `json:"certificate"`
	PolicyID     string                  `json:"policy_id"`
	NamespaceIDs map[string]string       `json:"namespace_ids"`
	Setup        Setup                   `json:"setup"`
	SetupID      string                  `json:"setup_id"`
	SetupHex     string                  `json:"canonical_setup_hex"`
	SetupFile    string                  `json:"public_setup_file_hex"`
	Intent       string                  `json:"intent_hex"`
	Ready        string                  `json:"ready_hex"`
	Backup       provisionBackup         `json:"backup"`
	BackupFile   string                  `json:"backup_file_hex"`
}
type provisionCorpus struct {
	CertificateFixture struct {
		ChainPEM string `json:"chain_pem"`
		CAPEM    string `json:"ca_pem"`
	} `json:"certificate_fixture"`
	Positives []provisionVector `json:"positives"`
	Refusals  []struct {
		Name, Family string
		Value        json.RawMessage
	} `json:"refusals"`
	Raw []struct {
		Name     string
		Hex      string `json:"raw_hex"`
		Accepted bool
	} `json:"raw_cases"`
}

func provisionReference(t *testing.T) provisionCorpus {
	t.Helper()
	f, e := os.Open("testdata/provision/reference-v1.json")
	if e != nil {
		t.Fatal(e)
	}
	raw, e := io.ReadAll(io.LimitReader(f, 543516))
	ce := f.Close()
	if e != nil || ce != nil {
		t.Fatal(e, ce)
	}
	sum := sha256.Sum256(raw)
	if len(raw) != 543515 || hex.EncodeToString(sum[:]) != "8c3cde66c35b69dbc0f4daf516c271fb91358a988b591c363f65a106f25a401f" {
		t.Fatal("independent provision corpus changed")
	}
	var out provisionCorpus
	if e = json.Unmarshal(raw, &out); e != nil {
		t.Fatal(e)
	}
	if len(out.Positives) != 6 || len(out.Refusals) != 61 || len(out.Raw) != 18 {
		t.Fatal("corpus coverage changed")
	}
	return out
}
func provisionHex(t *testing.T, s string) []byte {
	t.Helper()
	b, e := hex.DecodeString(s)
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func provisionCapsule(t *testing.T, v provisionVector) []byte {
	t.Helper()
	e, err := recovery.Entropy(v.Backup.Mnemonic)
	if err != nil {
		t.Fatal(err)
	}
	out := make([]byte, 128)
	copy(out, e[:])
	clear(e[:])
	for i, role := range []string{"admission", "publication", "cutoff"} {
		seed := sha256.Sum256([]byte("SLUG set74 public synthetic operator " + role + "\x00v1\x00"))
		copy(out[32+i*32:], seed[:])
		clear(seed[:])
	}
	return out
}
func TestProvisionIndependentBytes(t *testing.T) {
	refs := provisionReference(t)
	for _, v := range refs.Positives {
		t.Run(v.Name, func(t *testing.T) {
			p, e := ParseProvisionPolicy(provisionHex(t, v.PolicyFile))
			if e != nil || !reflect.DeepEqual(p, v.Policy) {
				t.Fatal("policy", e)
			}
			raw, e := json.Marshal(p)
			if e != nil || !bytes.Equal(raw, provisionHex(t, v.PolicyHex)) {
				t.Fatal("policy canonical", e)
			}
			id, e := ProvisionPolicyID(p)
			if e != nil || id != v.PolicyID {
				t.Fatal("policy ID", e)
			}
			for role, want := range v.NamespaceIDs {
				if provisionNamespace(p.RequestID, role) != want {
					t.Fatal("namespace", role)
				}
			}
			capsule := provisionCapsule(t, v)
			defer clear(capsule)
			material, root, e := deriveProvisionMaterial(capsule, v.Material.CreatedAtMS)
			if e != nil {
				t.Fatal(e)
			}
			defer root.Zero()
			if material != v.Material {
				t.Fatal("independent public derivation")
			}
			certificate, e := provisionCertificate([]byte(refs.CertificateFixture.ChainPEM), p, time.UnixMilli(v.Material.CreatedAtMS))
			if e != nil || certificate != v.Certificate {
				t.Fatal("independent certificate", e)
			}
			setup, e := ComposeProvisionSetup(p, material, certificate)
			if e != nil || !reflect.DeepEqual(setup, v.Setup) {
				t.Fatal("composed Setup", e)
			}
			raw, e = MarshalSetup(setup)
			if e != nil || !bytes.Equal(raw, provisionHex(t, v.SetupHex)) {
				t.Fatal("Setup exact bytes", e)
			}
			id, e = SetupID(setup)
			if e != nil || id != v.SetupID {
				t.Fatal("Setup ID", e)
			}
			intent, e := parseProvisionIntent(provisionHex(t, v.Intent), p, certificate)
			if e != nil || intent.CreatedAtMS != v.Material.CreatedAtMS {
				t.Fatal("intent", e)
			}
			raw, e = provisionJSON(intent, 131072)
			if e != nil || !bytes.Equal(raw, provisionHex(t, v.Intent)) {
				t.Fatal("intent exact", e)
			}
			ready, e := makeProvisionReady(p, setup, material.CreatedAtMS)
			if e != nil {
				t.Fatal(e)
			}
			raw, e = provisionJSON(ready, 1024)
			if e != nil || !bytes.Equal(raw, provisionHex(t, v.Ready)) {
				t.Fatal("READY exact", e)
			}
			entropy, e := recovery.Entropy(v.Backup.Mnemonic)
			if e != nil {
				t.Fatal(e)
			}
			raw, e = provisionBackupBytes(entropy, p, setup, material.CreatedAtMS)
			clear(entropy[:])
			defer clear(raw)
			if e != nil || !bytes.Equal(raw, provisionHex(t, v.BackupFile)) || verifyProvisionBackup(raw, p, setup, material.CreatedAtMS) != nil {
				t.Fatal("independent operator backup", e)
			}
		})
	}
}
func TestProvisionIndependentRefusals(t *testing.T) {
	refs := provisionReference(t)
	v := refs.Positives[0]
	for _, r := range refs.Refusals {
		t.Run(r.Family+"/"+r.Name, func(t *testing.T) {
			var e error
			switch r.Family {
			case "policy":
				var p ProvisionPolicy
				if e = json.Unmarshal(r.Value, &p); e == nil {
					e = ValidateProvisionPolicy(p)
				}
			case "composition":
				var p ProvisionPolicy
				if e = json.Unmarshal(r.Value, &p); e == nil {
					_, e = ComposeProvisionSetup(p, v.Material, v.Certificate)
				}
			case "public_material":
				var m ProvisionPublicMaterial
				if e = json.Unmarshal(r.Value, &m); e == nil {
					_, e = ComposeProvisionSetup(v.Policy, m, v.Certificate)
				}
			case "certificate":
				var c ProvisionCertificate
				if e = json.Unmarshal(r.Value, &c); e == nil {
					_, e = ComposeProvisionSetup(v.Policy, v.Material, c)
				}
			case "backup":
				var backup provisionBackup
				if e = json.Unmarshal(r.Value, &backup); e == nil {
					raw, x := provisionJSON(backup, 8192)
					e = x
					if e == nil {
						e = verifyProvisionBackup(raw, v.Policy, v.Setup, v.Material.CreatedAtMS)
					}
					clear(raw)
				}
			default:
				t.Fatal("unrecognized corpus refusal")
			}
			if e == nil {
				t.Fatal("independent refusal accepted")
			}
		})
	}
	for _, r := range refs.Raw {
		t.Run("raw/"+r.Name, func(t *testing.T) {
			_, e := ParseProvisionPolicy(provisionHex(t, r.Hex))
			if (e == nil) != r.Accepted {
				t.Fatal("raw policy acceptance", e)
			}
		})
	}
}
func TestProvisionCertificateAndCommandRefusals(t *testing.T) {
	refs := provisionReference(t)
	v := refs.Positives[0]
	now := time.UnixMilli(v.Material.CreatedAtMS)
	for _, r := range []struct {
		raw string
		at  time.Time
	}{
		{refs.CertificateFixture.CAPEM, now}, {refs.CertificateFixture.ChainPEM, time.UnixMilli(4102444800000)}, {refs.CertificateFixture.ChainPEM, time.UnixMilli(946684799999)}, {"bad certificate", now},
	} {
		if _, e := provisionCertificate([]byte(r.raw), v.Policy, r.at); e == nil {
			t.Fatal("invalid certificate accepted")
		}
	}
	changed := v.Policy
	changed.Transport.Origin = "https://other.example.test:3443"
	if _, e := provisionCertificate([]byte(refs.CertificateFixture.ChainPEM), changed, now); e == nil {
		t.Fatal("wrong hostname accepted")
	}
	args := []string{"create", "--policy-file", "/private/policy", "--root", "/private/output", "--tls-certificate-file", "/private/cert", "--wallet-public-passphrase-file", "/private/pub", "--wallet-unlock-passphrase-file", "/private/priv"}
	if _, e := parseProvisionCommand(args); e != nil {
		t.Fatal(e)
	}
	for _, bad := range [][]string{append(append([]string(nil), args...), "--entropy-file", "/private/entropy"), append(append([]string(nil), args...), "--checkpoint-seed-file", "/private/key"), append(append([]string(nil), args...), "--root", "/private/replacement"), {"create", "--secret", "private text"}} {
		if _, e := parseProvisionCommand(bad); e == nil {
			t.Fatal("secret/duplicate/unknown selection accepted")
		}
	}
}
