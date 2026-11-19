// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

// Python independently frames the shared identities and marker bytes using
// the frozen public Set41 pins. It neither calls Go nor rewrites source proofs.
func TestIndependentSharedStableNamespaces(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal-profile public identities")
	}
	raw, err := os.ReadFile("testdata/shared-reference/shared_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != "29b0aa42752e3db62afed4c5800373a9ada39fb1c9fd4319110d9725eafab225" {
		t.Fatal("frozen independent shared reference changed")
	}
	var corpus struct {
		Version   uint32
		Source    string `json:"source_sha256"`
		Generator string `json:"generator_sha256"`
		Pins      []struct {
			Fixture                         string
			Pin                             Pin
			Canonical, Preimage, ID, Marker string
			LegacyID                        string `json:"legacy_id"`
		}
	}
	if err = json.Unmarshal(raw, &corpus); err != nil {
		t.Fatal(err)
	}
	if corpus.Version != 1 || len(corpus.Pins) != 2 || corpus.Source != "29f6a1302c2e04ade612fe2775c085c65b8bf33587c010060b727833182d2057" || corpus.Generator != "f739c8d27cb2aae6999acca06ee253903de7ba187ee937f3594e7dd57e7c5290" {
		t.Fatal("wrong independent provenance")
	}
	dir := t.TempDir()
	sourcePath := filepath.Join(dir, "source")
	// Empty, structurally assembled source: no receipt relabeling or claims
	// about admitted fighters. Both pins explicitly share this authority.
	sourceRows(t, sourcePath, corpus.Pins[0].Pin, nil)
	source, err := registry.OpenManifestAuthority(sourcePath, corpus.Pins[0].Pin.Source, false)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	for _, fixture := range corpus.Pins {
		t.Run(fixture.Fixture, func(t *testing.T) {
			encoded, err := MarshalPin(fixture.Pin)
			if err != nil || string(encoded) != fixture.Canonical {
				t.Fatal("independent canonical pin differs", err)
			}
			pin, err := ParsePin(encoded)
			if err != nil || pin != fixture.Pin {
				t.Fatal("shared pin roundtrip", err)
			}
			id, err := ID(pin)
			if err != nil || id != fixture.ID || id == fixture.LegacyID {
				t.Fatal("independent shared namespace differs", id, err)
			}
			preimage, err := hex.DecodeString(fixture.Preimage)
			if err != nil || !bytes.HasPrefix(preimage, []byte("SLUG candidate shared stable\x00v1\x00")) {
				t.Fatal("independent domain differs")
			}
			sum := sha256.Sum256(preimage)
			if hex.EncodeToString(sum[:]) != id {
				t.Fatal("independent preimage differs")
			}
			path := filepath.Join(dir, fixture.Fixture)
			store, err := CreateShared(path, pin, source)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			marker, err := os.ReadFile(filepath.Join(path, sharedMarkerName))
			if err != nil || string(marker) != fixture.Marker {
				t.Fatal("independent marker differs", err)
			}
		})
	}
}
