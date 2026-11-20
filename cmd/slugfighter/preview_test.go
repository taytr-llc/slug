// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"
)

func TestManifestPreviewIndependentOutput(t *testing.T) {
	raw, err := os.ReadFile("../../fightermanifest/testdata/manifests_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Cases []struct{ Seed, Canonical, Digest string } `json:"cases"`
	}
	if json.Unmarshal(raw, &doc) != nil || len(doc.Cases) == 0 {
		t.Fatal("independent manifest corpus")
	}
	c := doc.Cases[0]
	for _, digest := range []bool{false, true} {
		var out, diagnostics bytes.Buffer
		args := []string{"preview", "--work-hash", c.Seed}
		want := c.Canonical
		if digest {
			args = append(args, "--digest")
			want = c.Digest + "\n"
		}
		if err := run(args, &out, &diagnostics); err != nil || out.String() != want || diagnostics.Len() != 0 {
			t.Fatal("preview differs from independent bytes", err)
		}
	}
	// The shipped DNA route is covered by the unchanged main_test.go suite.
	// New subcommand failures must not emit a partial canonical record.
	for _, args := range [][]string{{"preview"}, {"preview", "--work-hash", c.Seed, "--version", "3"}, {"preview", "--work-hash", c.Seed, "--version", "4294967297"}, {"preview", "--work-hash", c.Seed, "extra"}} {
		var out, diagnostics bytes.Buffer
		if err := run(args, &out, &diagnostics); err == nil || out.Len() != 0 {
			t.Fatal("invalid preview emitted output", args, err)
		}
	}
}

func TestManifestPreviewV2IndependentOutput(t *testing.T) {
	raw, err := os.ReadFile("../../fighterdescriptor/testdata/name-v2/independent/reference-v2.json")
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(raw)
	if hex.EncodeToString(hash[:]) != "62c4cd102341476548b9d388437af83750be407b8cbf534b34896719e2fa281b" {
		t.Fatal("independent names-v2 corpus changed")
	}
	var corpus struct {
		Cases []struct {
			Seed      string
			Canonical string
			Digest    string `json:"manifest_digest"`
		}
	}
	if json.Unmarshal(raw, &corpus) != nil || len(corpus.Cases) != 764 {
		t.Fatal("independent names-v2 corpus incomplete")
	}
	for _, c := range corpus.Cases {
		for _, digest := range []bool{false, true} {
			var out, diagnostics bytes.Buffer
			args := []string{"preview", "--work-hash", c.Seed, "--version", "2"}
			want := c.Canonical
			if digest {
				args = append(args, "--digest")
				want = c.Digest + "\n"
			}
			if err := run(args, &out, &diagnostics); err != nil || out.String() != want || diagnostics.Len() != 0 {
				t.Fatal("v2 preview differs from independent bytes", c.Seed, err)
			}
		}
	}
}
