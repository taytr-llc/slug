// Copyright (c) 2026 The SLUG developers. ISC license.

package prospectservice

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/wire/v2"
)

func TestManifestPEMRejectsResynchronization(t *testing.T) {
	corpus := loadManifestReference(t)
	var valid ManifestClientConfig
	for _, fixture := range corpus.Configs {
		if fixture.Kind == "client" && fixture.Accepted {
			config, err := ParseManifestClientConfig([]byte(fixture.Raw))
			if err == nil && config.CACertificate != "" {
				valid = config
				break
			}
		}
	}
	if valid.CACertificate == "" {
		t.Fatal("missing public certificate fixture")
	}
	for _, prefix := range []string{
		"-----BEGIN CERTIFICATE-----\ninvalid!\n",
		"-----BEGIN CERTIFICATE-----\n",
		"-----BEGIN CERTIFICATE-----\n-----BEGIN PRIVATE KEY-----\n",
	} {
		config := valid
		config.CACertificate = prefix + valid.CACertificate
		if got, err := ValidateManifestClientConfig(config); err == nil || !reflect.DeepEqual(got, ManifestClientConfig{}) {
			t.Fatal("malformed first block was skipped")
		}
	}
}

func TestManifestPureConfigOwnershipAndProfile(t *testing.T) {
	f := loadManifestReference(t)
	var server ManifestServerConfig
	for _, c := range f.Configs {
		if c.Kind == "server" && c.Accepted {
			parsed, err := ParseManifestServerConfig([]byte(c.Raw))
			if err == nil && len(parsed.Clients) != 0 {
				server = parsed
				break
			}
		}
	}
	if len(server.Clients) == 0 {
		t.Fatal("missing static config fixture")
	}
	owned, err := ValidateManifestServerConfig(server)
	if err != nil {
		t.Fatal(err)
	}
	server.Clients[0].PublicKey = "mutated caller memory"
	if owned.Clients[0].PublicKey == server.Clients[0].PublicKey {
		t.Fatal("configuration aliases credentials")
	}
	credential := owned.Clients[0]
	owned.Clients = nil
	canonical, err := ValidateManifestServerConfig(owned)
	if err != nil || canonical.Clients == nil {
		t.Fatal("nil programmatic list not normalized", err)
	}
	raw, _ := json.Marshal(canonical)
	if _, err := ParseManifestServerConfig(raw); err != nil {
		t.Fatal("canonical empty list cannot round-trip", err)
	}
	if _, err := ParseManifestServerConfig([]byte(strings.Replace(string(raw), `"clients":[]`, `"clients":null`, 1))); err == nil {
		t.Fatal("stored null clients accepted")
	}
	if _, err := ValidateServerConfig(ServerConfig{Version: Version3, Listen: canonical.Listen, Clients: []Credential{credential}, MaxJobs: 1, MaxRecords: 1}); err == nil {
		t.Fatal("old server silently upgraded")
	}
	if _, err := ParseClientConfig([]byte(`{"version":3,"url":"http://127.0.0.1:80","token":"` + strings.Repeat("ab", 32) + `","ca_certificate":""}`)); err == nil {
		t.Fatal("old client silently upgraded")
	}
	if wire.TestPoW {
		policy := f.Jobs[0].Policy
		config := ManifestClientConfig{Version: Version3, URL: "http://127.0.0.1:1", Token: strings.Repeat("ab", 32), AuthorityPin: f.Pin}
		if _, err := ManifestTransportTrustID(config); err != nil {
			t.Fatal("pure trust helper refused fixture profile", err)
		}
		if c, err := NewManifestClient(config, policy); err == nil || c != nil || !strings.Contains(err.Error(), "production PoW") {
			t.Fatal("client did not refuse hash profile first", err)
		}
		if s, err := NewManifestServer(canonical, policy, nil, nil); err == nil || s != nil || !strings.Contains(err.Error(), "production PoW") {
			t.Fatal("server did not refuse hash profile first", err)
		}
	}
}
