// Copyright (c) 2026 The SLUG developers. ISC license.

package prospectservice

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

// Exercise decoded duplicate names, depth, raw limits and canonical reencoding
// without any Argon, transport or storage work in the fuzz loop.
func FuzzManifestTransportJSON(f *testing.F) {
	raw, err := os.ReadFile("testdata/manifest/manifest_service_v1.json")
	if err != nil {
		f.Fatal(err)
	}
	var seeds manifestReferenceCorpus
	if err := json.Unmarshal(raw, &seeds); err != nil {
		f.Fatal(err)
	}
	added := make(map[uint8]bool)
	for _, seed := range seeds.Configs {
		selector := uint8(0)
		if seed.Kind == "client" {
			selector = 1
		}
		if seed.Accepted && !added[selector] {
			f.Add(selector, []byte(seed.Raw))
			added[selector] = true
		}
	}
	for _, seed := range seeds.Receipts {
		if seed.Accepted {
			f.Add(uint8(2), []byte(seed.Body))
			break
		}
	}
	f.Add(uint8(0), []byte(`{"version":3,"clients":[]}`))
	f.Add(uint8(1), []byte(`{"version":3,"authority_pin":null}`))
	f.Add(uint8(2), []byte(`{"version":3,"record":{"proof":{"merkle_branch":[]}}}`))
	f.Add(uint8(2), []byte(`{"version":3,"\u0076ersion":3}`))
	f.Add(uint8(0), []byte(`{"clients":[[[[[[[]]]]]]]}`))
	f.Fuzz(func(t *testing.T, selector uint8, raw []byte) {
		if len(raw) > MaxResponseSize+1 {
			t.Skip()
		}
		switch selector % 3 {
		case 0:
			value, err := ParseManifestServerConfig(raw)
			if err != nil {
				if !reflect.DeepEqual(value, ManifestServerConfig{}) {
					t.Fatal("partial rejected config")
				}
				return
			}
			canonical, err := json.Marshal(value)
			if err != nil || len(canonical) > MaxServerConfigSize {
				t.Fatal("unbounded valid config")
			}
			again, err := ParseManifestServerConfig(canonical)
			if err != nil || !reflect.DeepEqual(value, again) {
				t.Fatal("server config failed canonical round trip")
			}
		case 1:
			value, err := ParseManifestClientConfig(raw)
			if err != nil {
				if !reflect.DeepEqual(value, ManifestClientConfig{}) {
					t.Fatal("partial rejected config")
				}
				return
			}
			canonical, err := json.Marshal(value)
			if err != nil || len(canonical) > MaxClientConfigSize {
				t.Fatal("unbounded valid config")
			}
			again, err := ParseManifestClientConfig(canonical)
			if err != nil || value != again {
				t.Fatal("client config failed canonical round trip")
			}
			id, err := ManifestTransportTrustID(value)
			if err != nil || !manifestHex(id, 32) {
				t.Fatal("valid config lacks trust identity")
			}
		case 2:
			value, err := parseManifestAdmission(raw)
			if err != nil {
				return
			}
			canonical, err := json.Marshal(value)
			if err != nil || len(canonical) > MaxResponseSize {
				t.Fatal("unbounded parsed response")
			}
			again, err := parseManifestAdmission(canonical)
			if err != nil || !reflect.DeepEqual(value, again) {
				t.Fatal("admission shape failed canonical round trip")
			}
		}
	})
}
