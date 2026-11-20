// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterportrait

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/taytr-llc/slug/fightermanifest"
	"os"
	"testing"
)

func TestNameV2IndependentPortraits(t *testing.T) {
	raw, err := os.ReadFile("../fighterdescriptor/testdata/name-v2/independent/reference-v2.json")
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "62c4cd102341476548b9d388437af83750be407b8cbf534b34896719e2fa281b" {
		t.Fatal("independent corpus changed")
	}
	var ref struct {
		Cases []struct {
			Seed, Canonical, SVG string
			Manifest             fightermanifest.Manifest
			Digest               string `json:"svg_digest"`
			Body                 string `json:"graphics_body_sha256"`
		}
		Invalid []struct {
			Version  uint32 `json:"renderer_version"`
			Manifest string `json:"manifest_hex"`
			Accepted bool
		} `json:"invalid_renderer_pairs"`
	}
	if err = json.Unmarshal(raw, &ref); err != nil {
		t.Fatal(err)
	}
	if len(ref.Cases) != 764 || len(ref.Invalid) != 4 {
		t.Fatal("incomplete corpus")
	}
	apostrophe := false
	for _, c := range ref.Cases {
		svg, err := Render(Version2, c.Manifest)
		if err != nil || !bytes.Equal(svg, []byte(c.SVG)) {
			t.Fatalf("v2 SVG %s: %v", c.Seed, err)
		}
		fromBytes, err := RenderCanonical(Version2, []byte(c.Canonical))
		if err != nil || !bytes.Equal(fromBytes, svg) {
			t.Fatalf("canonical SVG %s", c.Seed)
		}
		digest, err := Digest(Version2, c.Manifest)
		if err != nil || digest != c.Digest {
			t.Fatalf("SVG digest %s", c.Seed)
		}
		old, err := fightermanifest.GenerateText(1, c.Seed)
		if err != nil {
			t.Fatal(err)
		}
		oldSVG, err := Render(Version1, old)
		if err != nil {
			t.Fatal(err)
		}
		body := bytes.SplitN(svg, []byte("</desc>"), 2)
		oldBody := bytes.SplitN(oldSVG, []byte("</desc>"), 2)
		if len(body) != 2 || len(oldBody) != 2 || !bytes.Equal(body[1], oldBody[1]) {
			t.Fatalf("graphics changed %s", c.Seed)
		}
		bh := sha256.Sum256(body[1])
		if hex.EncodeToString(bh[:]) != c.Body {
			t.Fatal("independent body digest")
		}
		apostrophe = apostrophe || bytes.Contains(svg, []byte("&apos;"))
	}
	if !apostrophe {
		t.Fatal("missing actual apostrophe coverage")
	}
	for _, c := range ref.Invalid {
		b, err := hex.DecodeString(c.Manifest)
		if err != nil {
			t.Fatal(err)
		}
		_, err = RenderCanonical(c.Version, b)
		if c.Accepted || err == nil {
			t.Fatal("accepted incompatible renderer tuple", c.Version)
		}
	}
}
