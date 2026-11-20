// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/wire/v2"
)

const nameV2DescriptorFixture = "cd02ae57b5918548244be2cabbe6c0c433feb5a1c165745e2e5c6bf235128e75"

func TestNameV2PortraitUsesExactSourceDomain(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal-profile transport view")
	}
	raw, err := os.ReadFile("../../../fighterdescriptor/testdata/name-v2/independent/reference-v2.json")
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "62c4cd102341476548b9d388437af83750be407b8cbf534b34896719e2fa281b" {
		t.Fatal("independent v2 corpus changed")
	}
	var ref struct {
		Cases []struct {
			SVG            string
			ManifestDigest string `json:"manifest_digest"`
			PortraitDigest string `json:"svg_digest"`
			V1Digest       string `json:"v1_svg_digest"`
		}
	}
	if err = json.Unmarshal(raw, &ref); err != nil || len(ref.Cases) != 764 {
		t.Fatal("independent corpus shape", err)
	}
	view := *manifestBridgeSnapshot(t).View // Explicitly a transport-shape fixture.
	oldDescriptor := view.AuthorityPin.DescriptorID
	view.AuthorityPin.DescriptorID = nameV2DescriptorFixture
	for n, f := range ref.Cases {
		p := pilot.ManifestPortrait{Version: 1, View: view, Sequence: 1, ID: strings.Repeat("e", 64), ManifestDigest: f.ManifestDigest, PortraitDigest: f.PortraitDigest, SVG: f.SVG}
		if !validManifestPortrait(&p) {
			t.Fatalf("independent v2 SVG refused at case %d", n)
		}
		for _, change := range []func(*pilot.ManifestPortrait){
			func(v *pilot.ManifestPortrait) { v.View.AuthorityPin.DescriptorID = oldDescriptor },
			func(v *pilot.ManifestPortrait) { v.View.AuthorityPin.DescriptorID = strings.Repeat("f", 64) },
			func(v *pilot.ManifestPortrait) { v.PortraitDigest = f.V1Digest },
			func(v *pilot.ManifestPortrait) {
				v.SVG = strings.Replace(v.SVG, "slug-fighter-portrait-candidate-v2", "slug-fighter-portrait-candidate-v1", 1)
			},
		} {
			bad := p
			change(&bad)
			if validManifestPortrait(&bad) {
				t.Fatalf("foreign version/domain accepted at case %d", n)
			}
		}
	}
}

func TestNameV2SupportedDescriptorCannotReplaceBoutParticipant(t *testing.T) {
	ref := boutRef(t)
	if len(ref.Compositions) == 0 {
		t.Fatal("missing frozen v1 composition")
	}
	c := ref.Compositions[0]
	id, err := gymbout.ID(c.Pin)
	if err != nil {
		t.Fatal(err)
	}
	a := boutAuthority{pin: c.Pin, storeID: id, createdMS: c.Created, sourceCreatedMS: c.SourceCreated}
	b := &StableBridge{}
	if !b.validBoutRecord(a, c.Record) {
		t.Fatal("unchanged v1 source binding refused")
	}
	// Counterexample only: this edited record is not a purported v2 receipt.
	raw, _ := json.Marshal(c.Record)
	var bad gymbout.Record
	if json.Unmarshal(raw, &bad) != nil {
		t.Fatal("copy")
	}
	bad.Source.Corners[0].Member.Assignment.DescriptorID = nameV2DescriptorFixture
	if b.validBoutRecord(a, bad) {
		t.Fatal("supported descriptor substituted into another participant source")
	}
}
