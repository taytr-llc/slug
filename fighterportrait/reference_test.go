// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterportrait

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/taytr-llc/slug/fightermanifest"
)

// These bytes were generated independently with Python Fraction arithmetic,
// frozen manifest/table inputs and a separate high-precision curly calculation.
// Do not replace them with renderer output. Full provenance and regeneration
// code are preserved under testdata/reference.
type referenceCorpus struct {
	RendererVersion uint32 `json:"renderer_version"`
	RendererID      string `json:"renderer_id"`
	DomainHex       string `json:"digest_domain_hex"`
	ContractHash    string `json:"approved_contract_sha256"`
	Cases           []struct {
		Name           string                   `json:"name"`
		Seed           string                   `json:"seed"`
		Manifest       fightermanifest.Manifest `json:"manifest"`
		ManifestDigest string                   `json:"manifest_digest"`
		SVG            string                   `json:"svg"`
		SHA256         string                   `json:"svg_sha256"`
		Digest         string                   `json:"svg_digest"`
		Bytes          int                      `json:"byte_length"`
		Graphics       int                      `json:"graphics_count"`
		Elements       int                      `json:"element_count"`
		Depth          int                      `json:"depth"`
	} `json:"cases"`
	Rounding []struct {
		S  int64 `json:"shoulder_width"`
		CI int64 `json:"collar_in"`
		CO int64 `json:"collar_out"`
		PO int64 `json:"pec_out"`
		PM int64 `json:"pec_mid"`
		PD int64 `json:"pec_dip"`
		NX int64 `json:"nipple_x"`
	} `json:"rounding_vectors"`
	Curly []struct {
		Name    string      `json:"head_shape"`
		Centers [6][2]int64 `json:"centers"`
		Radius  string      `json:"radius"`
	} `json:"curly_vectors"`
	Jitter []struct {
		Jitter int64 `json:"jitter"`
		Half   int64 `json:"rounded_half"`
	} `json:"jitter_rounding"`
}

func readReference(t *testing.T) referenceCorpus {
	t.Helper()
	raw, err := os.ReadFile("testdata/reference/portraits_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "03df75cbf923a6dfc8fcac78e98d704641c0edf65ce563cf307f20a52f250670" {
		t.Fatal("independent corpus changed")
	}
	var corpus referenceCorpus
	if err = json.Unmarshal(raw, &corpus); err != nil {
		t.Fatal(err)
	}
	if corpus.RendererVersion != 1 || corpus.RendererID != RendererID || corpus.DomainHex != hex.EncodeToString([]byte(digestDomain)) || corpus.ContractHash != "8ae3c8dbe92fbef719880227e107cb98cbfd6419d984cad08ebb5d5e0a9c0df5" {
		t.Fatal("independent contract mismatch")
	}
	if len(corpus.Cases) != 50 || len(corpus.Rounding) != 27 || len(corpus.Curly) != 4 || len(corpus.Jitter) != 13 {
		t.Fatal("incomplete corpus")
	}
	return corpus
}

func TestReferenceSVG(t *testing.T) {
	corpus := readReference(t)
	coverage := map[string]map[int]bool{}
	textCoverage := map[string]map[string]bool{}
	for _, tc := range corpus.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			m, err := fightermanifest.GenerateText(1, tc.Seed)
			if err != nil || m != tc.Manifest {
				t.Fatal("manifest changed", err)
			}
			md, err := fightermanifest.Digest(m)
			if err != nil || md != tc.ManifestDigest {
				t.Fatal("manifest bytes changed", err)
			}
			got, err := Render(1, m)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, []byte(tc.SVG)) {
				i := 0
				for i < len(got) && i < len(tc.SVG) && got[i] == tc.SVG[i] {
					i++
				}
				t.Fatalf("independent SVG mismatch at byte %d; got length %d, want %d", i, len(got), len(tc.SVG))
			}
			file, err := os.ReadFile(filepath.Join("testdata/reference/svg", tc.Name+".svg"))
			if err != nil || !bytes.Equal(got, file) {
				t.Fatal("standalone independent SVG mismatch", err)
			}
			raw, err := fightermanifest.Marshal(m)
			if err != nil {
				t.Fatal(err)
			}
			byteInput, err := RenderCanonical(1, raw)
			if err != nil || !bytes.Equal(byteInput, got) {
				t.Fatal("canonical entrypoint mismatch", err)
			}
			h := sha256.Sum256(got)
			if hex.EncodeToString(h[:]) != tc.SHA256 {
				t.Fatal("raw SVG digest mismatch")
			}
			digest, err := Digest(1, m)
			if err != nil || digest != tc.Digest {
				t.Fatal("domain-separated SVG digest mismatch", err)
			}
			if len(got) != tc.Bytes || assertSafeDocument(t, got) != tc.Graphics || tc.Elements != tc.Graphics+3 || tc.Depth != 2 {
				t.Fatal("document structure mismatch")
			}
			for key, value := range map[string]int{"skin": m.Portrait.Skin, "hair": m.Portrait.HairStyle, "hair_color": m.Portrait.HairColor, "face": m.Portrait.FacialHair, "shape": m.Portrait.HeadShape, "mouth": m.Portrait.Mouth, "brow": m.Portrait.Brow, "background": m.Portrait.Background, "jitter": m.Portrait.ShoulderJitter} {
				if coverage[key] == nil {
					coverage[key] = map[int]bool{}
				}
				coverage[key][value] = true
			}
			for key, value := range map[string]string{"class": m.Physical.Class, "rarity": m.DNA.Rarity, "scar": m.Portrait.Scar} {
				if textCoverage[key] == nil {
					textCoverage[key] = map[string]bool{}
				}
				textCoverage[key][value] = true
			}
		})
	}
	for key, count := range map[string]int{"skin": 6, "hair": 8, "hair_color": 6, "face": 5, "shape": 4, "mouth": 4, "brow": 3, "background": 8, "jitter": 13} {
		if len(coverage[key]) != count {
			t.Fatal("missing branch coverage", key)
		}
	}
	for key, count := range map[string]int{"class": 5, "rarity": 5, "scar": 3} {
		if len(textCoverage[key]) != count {
			t.Fatal("missing branch coverage", key)
		}
	}
}

func TestReferenceRoundingAndCurly(t *testing.T) {
	corpus := readReference(t)
	for _, v := range corpus.Rounding {
		got := [6]int64{roundEven(22*v.S, 100), roundEven(46*v.S, 100), roundEven(40*v.S, 100), roundEven(20*v.S, 100), roundEven(16800+13*v.S, 100), roundEven(26*v.S, 100)}
		if got != [6]int64{v.CI, v.CO, v.PO, v.PM, v.PD, v.NX} {
			t.Fatal("independent chest rounding mismatch", v.S, got)
		}
	}
	for _, v := range corpus.Jitter {
		if roundEven(v.Jitter, 2) != v.Half {
			t.Fatal("independent jitter mismatch", v)
		}
	}
	for i, v := range corpus.Curly {
		if tables.Head[i].Name != v.Name || curlyCenters(i) != v.Centers || number(850) != v.Radius {
			t.Fatal("independent curly mismatch", v.Name)
		}
	}
}

func TestPrivateMaximumGeometry(t *testing.T) {
	// Synthetic private geometry only; no public API may bless this altered
	// tuple. Curly+goatee+grimace+scar attains the frozen 43-graphic bound.
	m := zeroManifest(t)
	m.Portrait.HairStyle = 5
	m.Portrait.FacialHair = 3
	m.Portrait.Mouth = 3
	m.Portrait.Scar = "left_brow"
	if out, err := Render(1, m); err == nil || out != nil {
		t.Fatal("synthetic tuple accepted publicly")
	}
	for shape := 0; shape < 4; shape++ {
		for jitter := -6; jitter <= 6; jitter++ {
			for _, class := range []string{"light", "welter", "middle", "light_heavy", "heavy"} {
				m.Portrait.HeadShape = shape
				m.Portrait.ShoulderJitter = jitter
				m.Physical.Class = class
				out, err := render(m)
				if err != nil {
					t.Fatal("bounded synthetic geometry failed", err)
				}
				if assertSafeDocument(t, out) != 43 {
					t.Fatal("maximum element count changed")
				}
			}
		}
	}
}
