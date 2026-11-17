// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"bytes"
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
	for _, args := range [][]string{{"preview"}, {"preview", "--work-hash", c.Seed, "--version", "2"}, {"preview", "--work-hash", c.Seed, "--version", "4294967297"}, {"preview", "--work-hash", c.Seed, "extra"}} {
		var out, diagnostics bytes.Buffer
		if err := run(args, &out, &diagnostics); err == nil || out.Len() != 0 {
			t.Fatal("invalid preview emitted output", args, err)
		}
	}
}
