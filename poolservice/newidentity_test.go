// Copyright (c) 2026 The SLUG developers. ISC license.
package poolservice

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/prospectservice"
)

func TestIndependentPoolIdentityContract(t *testing.T) {
	raw, err := os.ReadFile("testdata/identities_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	if hex.EncodeToString(digest[:]) != "361a08179c56d024836c45646de215024096d3a234fab93fe9566dbcd3c37a1c" {
		t.Fatal("frozen independent identity fixture changed")
	}
	var fixture struct {
		Version         uint32       `json:"version"`
		Client          ClientConfig `json:"client"`
		SourceCanonical string       `json:"source_canonical"`
		SourceID        string       `json:"source_id"`
		ManifestTrust   string       `json:"manifest_http_trust_id"`
		PoolTrust       string       `json:"pool_transport_trust_id"`
		Provenance      struct {
			InputSHA256  string `json:"input_sha256"`
			OracleSHA256 string `json:"oracle_sha256"`
		} `json:"provenance"`
	}
	if json.Unmarshal(raw, &fixture) != nil || fixture.Version != 1 || fixture.Provenance.InputSHA256 != "2210d992e618b7a0952736dede1263b678f1a7a2d55d14ccde1cc6b29317de87" {
		t.Fatal("invalid identity provenance")
	}
	oracle, err := os.ReadFile("testdata/identity_reference.py")
	if err != nil {
		t.Fatal(err)
	}
	oracleDigest := sha256.Sum256(oracle)
	if hex.EncodeToString(oracleDigest[:]) != fixture.Provenance.OracleSHA256 {
		t.Fatal("identity oracle changed")
	}
	source, err := json.Marshal(fixture.Client.Source)
	if err != nil || string(source) != fixture.SourceCanonical {
		t.Fatal("source canonical contract changed")
	}
	binding, err := BindingID(fixture.Client.Source)
	if err != nil || binding != fixture.SourceID {
		t.Fatal("source identity differs from independent oracle", err)
	}
	manifest, err := prospectservice.ManifestTransportTrustID(manifestConfig(fixture.Client))
	if err != nil || manifest != fixture.ManifestTrust {
		t.Fatal("manifest HTTP trust differs from independent oracle", err)
	}
	trust, err := TransportTrustID(fixture.Client)
	if err != nil || trust != fixture.PoolTrust {
		t.Fatal("pool transport trust differs from independent oracle", err)
	}
	rotated := fixture.Client
	rotated.Token = strings.Repeat("cd", 32)
	if got, err := TransportTrustID(rotated); err != nil || got != fixture.PoolTrust {
		t.Fatal("token rotation altered public identity", err)
	}
	for name, change := range map[string]func(*ClientConfig){
		"source": func(c *ClientConfig) { c.Source.MaxWorks++ },
		"gym": func(c *ClientConfig) {
			if c.PublicKey[:2] == "02" {
				c.PublicKey = "03" + c.PublicKey[2:]
			} else {
				c.PublicKey = "02" + c.PublicKey[2:]
			}
		},
		"endpoint": func(c *ClientConfig) { c.URL = "http://127.0.0.1:19057" },
	} {
		t.Run(name, func(t *testing.T) {
			changed := fixture.Client
			change(&changed)
			if got, err := TransportTrustID(changed); err != nil || got == fixture.PoolTrust {
				t.Fatal("changed binding did not change identity", err)
			}
		})
	}
}
