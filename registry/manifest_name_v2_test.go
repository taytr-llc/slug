// Copyright (c) 2026 The SLUG developers. ISC license.
package registry

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/fightermanifest"
	"github.com/taytr-llc/slug/prospect"
	"os"
	"strings"
	"testing"
)

const nameV2BindingID = "cd02ae57b5918548244be2cabbe6c0c433feb5a1c165745e2e5c6bf235128e75"

type nameV2BindingCase struct {
	Seed             string
	Manifest         fightermanifest.Manifest
	ManifestDigest   string `json:"manifest_digest"`
	PortraitDigest   string `json:"svg_digest"`
	V1ManifestDigest string `json:"v1_manifest_digest"`
	V1PortraitDigest string `json:"v1_svg_digest"`
}

func nameV2BindingCases(t *testing.T) (fighterdescriptor.Descriptor, []nameV2BindingCase) {
	t.Helper()
	raw, err := os.ReadFile("../fighterdescriptor/testdata/name-v2/independent/reference-v2.json")
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "62c4cd102341476548b9d388437af83750be407b8cbf534b34896719e2fa281b" {
		t.Fatal("independent corpus changed")
	}
	var ref struct {
		Descriptor struct {
			Descriptor fighterdescriptor.Descriptor
			ID         string
		}
		Cases []nameV2BindingCase
	}
	if err := json.Unmarshal(raw, &ref); err != nil || len(ref.Cases) != 764 || ref.Descriptor.ID != nameV2BindingID {
		t.Fatal("corpus shape", err)
	}
	seeds := map[string]bool{strings.Repeat("0", 64): true, strings.Repeat("f", 64): true, "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f": true}
	selected := []nameV2BindingCase{}
	for _, c := range ref.Cases {
		if seeds[c.Seed] {
			selected = append(selected, c)
			delete(seeds, c.Seed)
		}
	}
	if len(seeds) != 0 {
		t.Fatal("missing prescribed public seeds")
	}
	return ref.Descriptor.Descriptor, selected
}

func TestNameV2IndependentStaticBindings(t *testing.T) {
	d, cases := nameV2BindingCases(t)
	old := fighterdescriptor.CandidateDescriptor()
	oldID, err := fighterdescriptor.ID(old)
	if err != nil {
		t.Fatal(err)
	}
	// These constructed Verified values exercise only a private pure derivation.
	// They are not purported mined proofs, admissions, or ownership evidence.
	for _, f := range cases {
		verified := prospect.Verified{WorkHash: f.Seed, DNA: f.Manifest.DNA}
		for _, v := range []struct {
			id                 string
			descriptor         fighterdescriptor.Descriptor
			manifest, portrait string
		}{
			{nameV2BindingID, d, f.ManifestDigest, f.PortraitDigest}, {oldID, old, f.V1ManifestDigest, f.V1PortraitDigest},
		} {
			got, err := (&ManifestStore{meta: manifestMetadata{DescriptorID: v.id, Descriptor: v.descriptor}}).binding(verified)
			want := StaticBinding{Version: 1, DescriptorID: v.id, ManifestDigest: v.manifest, PortraitDigest: v.portrait}
			if err != nil || got != want {
				t.Fatalf("independent binding mismatch: version %d, seed %s, error %v", v.descriptor.ManifestVersion, f.Seed, err)
			}
		}
		for _, mutate := range []func(*prospect.Verified){
			func(v *prospect.Verified) { v.WorkHash = "bad" },
			func(v *prospect.Verified) { v.DNA.Attributes.Power++ },
			func(v *prospect.Verified) { v.DNA.PrimeWeeks++ },
			func(v *prospect.Verified) { v.DNA.DNAVersion = 2 },
			func(v *prospect.Verified) { v.DNA.Seed = "bad" },
		} {
			bad := verified
			mutate(&bad)
			v := struct {
				id         string
				descriptor fighterdescriptor.Descriptor
			}{nameV2BindingID, d}
			got, err := (&ManifestStore{meta: manifestMetadata{DescriptorID: v.id, Descriptor: v.descriptor}}).binding(bad)
			if err == nil || got != (StaticBinding{}) {
				t.Fatal("bad verified metadata accepted or partially returned")
			}
		}
		for _, id := range []string{"", strings.Repeat("f", 64), strings.ToUpper(nameV2BindingID)} {
			v := struct {
				id         string
				descriptor fighterdescriptor.Descriptor
			}{id, d}
			got, err := (&ManifestStore{meta: manifestMetadata{DescriptorID: v.id, Descriptor: v.descriptor}}).binding(verified)
			if err == nil || got != (StaticBinding{}) {
				t.Fatal("unknown/noncanonical descriptor accepted")
			}
		}
	}
}

func TestNameV2RegistryRetainsCompleteDescriptor(t *testing.T) {
	d, cases := nameV2BindingCases(t)
	verified := prospect.Verified{WorkHash: cases[0].Seed, DNA: cases[0].Manifest.DNA}
	for _, mutate := range []func(*fighterdescriptor.Descriptor){
		func(d *fighterdescriptor.Descriptor) { *d = fighterdescriptor.CandidateDescriptor() },
		func(d *fighterdescriptor.Descriptor) { d.RendererVersion = 1 },
		func(d *fighterdescriptor.Descriptor) { d.RendererID = "other" },
		func(d *fighterdescriptor.Descriptor) { d.ManifestContractHash = strings.Repeat("f", 64) },
	} {
		wrong := d
		mutate(&wrong)
		store := ManifestStore{meta: manifestMetadata{DescriptorID: nameV2BindingID, Descriptor: wrong}}
		got, err := store.binding(verified)
		if err == nil || got != (StaticBinding{}) {
			t.Fatal("stored descriptor tuple substituted")
		}
	}
}
