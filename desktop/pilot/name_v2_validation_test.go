// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/fightercondition"
	"github.com/taytr-llc/slug/fightermanifest"
	"github.com/taytr-llc/slug/gymstable"
)

const nameV2DescriptorFixture = "cd02ae57b5918548244be2cabbe6c0c433feb5a1c165745e2e5c6bf235128e75"

type nameV2ShapeFixture struct {
	Seed           string
	Manifest       fightermanifest.Manifest
	ManifestDigest string `json:"manifest_digest"`
	PortraitDigest string `json:"svg_digest"`
}

func nameV2ShapeFixtures(t *testing.T) []nameV2ShapeFixture {
	t.Helper()
	raw, err := os.ReadFile("../../fighterdescriptor/testdata/name-v2/independent/reference-v2.json")
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "62c4cd102341476548b9d388437af83750be407b8cbf534b34896719e2fa281b" {
		t.Fatal("independent v2 corpus changed")
	}
	var ref struct{ Cases []nameV2ShapeFixture }
	if err := json.Unmarshal(raw, &ref); err != nil || len(ref.Cases) != 764 {
		t.Fatal("independent v2 corpus shape", err)
	}
	return ref.Cases
}

func TestNameV2StableAndEntrySourceShapes(t *testing.T) {
	// Synthetic transport shapes only. These inputs claim no admission, source
	// receipt, ownership or chain evidence. Expected names and digests are from
	// the independently frozen corpus; condition setup uses unchanged DNA v1.
	for n, f := range nameV2ShapeFixtures(t) {
		if !stableManifestShapeFor(f.Manifest, f.Seed, nameV2DescriptorFixture) {
			t.Fatalf("v2 shape refused at case %d", n)
		}
		if stableManifestShapeFor(f.Manifest, f.Seed, manifestDescriptorID) || stableManifestShape(f.Manifest, f.Seed) {
			t.Fatalf("v2 leaked into v1 source or practice at case %d", n)
		}
		if n > 2 && !strings.Contains(f.Manifest.Name.Display, "'") {
			continue
		}
		c, err := fightercondition.Project(1, f.Manifest.DNA, 1, 2, 0)
		if err != nil {
			t.Fatal(err)
		}
		a := gymstable.Assignment{Version: 1, Sequence: 1, ProspectID: strings.Repeat("d", 64), GymID: strings.Repeat("a", 64), WorkHash: f.Seed, BornAtMS: 1, AcceptedAtMS: 2, DescriptorID: nameV2DescriptorFixture, ManifestDigest: f.ManifestDigest, PortraitDigest: f.PortraitDigest, SourceRecordDigest: strings.Repeat("b", 64)}
		i := gymstable.Item{Assignment: a, Manifest: f.Manifest, Condition: c}
		s := gymstable.Status{Checkpoint: 1, Pin: gymstable.Pin{ProjectorID: fightercondition.ProjectorID}}
		s.Pin.Identity.GymID, s.Pin.Source.DescriptorID = a.GymID, nameV2DescriptorFixture
		if !stableItemValid(i, s, 2) || !entryItemShape(i, 2, nameV2DescriptorFixture) {
			t.Fatalf("v2 bound item refused at case %d", n)
		}
		if entryItemShape(i, 2, manifestDescriptorID) {
			t.Fatal("entry accepted a supported foreign source")
		}
		s.Pin.Source.DescriptorID = manifestDescriptorID
		if stableItemValid(i, s, 2) {
			t.Fatal("stable accepted a supported foreign source")
		}
		s.Pin.Source.DescriptorID = nameV2DescriptorFixture
		i.Assignment.DescriptorID = manifestDescriptorID
		if stableItemValid(i, s, 2) || entryItemShape(i, 2, nameV2DescriptorFixture) {
			t.Fatal("assignment descriptor replaced the source")
		}
	}
}

func TestNameV2TransportGrammarAndTupleRefusal(t *testing.T) {
	f := nameV2ShapeFixtures(t)[0]
	for _, bad := range []string{"", "A-", "-A", "A''B", "A--B", "A-'B", "A B", "A\tB", "Aé", strings.Repeat("A", 33)} {
		m := f.Manifest
		m.Name.First, m.Name.Display = bad, bad+" "+m.Name.Last
		if stableManifestShapeFor(m, f.Seed, nameV2DescriptorFixture) {
			t.Fatalf("invalid v2 name accepted: %q", bad)
		}
	}
	for _, change := range []func(*fightermanifest.Manifest){
		func(m *fightermanifest.Manifest) { m.Version = 1 },
		func(m *fightermanifest.Manifest) { m.Generator = fightermanifest.GeneratorID },
		func(m *fightermanifest.Manifest) { m.Tables = fightermanifest.TableHash },
		func(m *fightermanifest.Manifest) { m.Portrait.Version = 2 },
		func(m *fightermanifest.Manifest) { m.DNA.Seed = strings.Repeat("f", 64) },
	} {
		m := f.Manifest
		change(&m)
		if stableManifestShapeFor(m, f.Seed, nameV2DescriptorFixture) {
			t.Fatal("mixed implementation tuple accepted")
		}
	}
}
