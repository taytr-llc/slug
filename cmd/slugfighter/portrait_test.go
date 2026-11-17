// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

func TestPortraitPreviewIndependentOutput(t *testing.T) {
	raw, err := os.ReadFile("../../fighterportrait/testdata/reference/portraits_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		Cases []struct {
			Seed   string `json:"seed"`
			SVG    string `json:"svg"`
			Digest string `json:"svg_digest"`
		} `json:"cases"`
	}
	if json.Unmarshal(raw, &corpus) != nil || len(corpus.Cases) == 0 {
		t.Fatal("independent portrait corpus")
	}
	for _, c := range corpus.Cases {
		for _, digest := range []bool{false, true} {
			var out, diagnostics bytes.Buffer
			args := []string{"portrait", "--work-hash", c.Seed}
			want := c.SVG
			if digest {
				args = append(args, "--digest")
				want = c.Digest + "\n"
			}
			if err := run(args, &out, &diagnostics); err != nil || out.String() != want || diagnostics.Len() != 0 {
				t.Fatal("portrait differs from independent bytes", c.Seed, err)
			}
		}
	}
	seed := corpus.Cases[0].Seed
	for _, args := range [][]string{{"portrait"}, {"portrait", "--work-hash", seed, "--version", "2"}, {"portrait", "--work-hash", seed, "--version", "4294967297"}, {"portrait", "--work-hash", seed, "extra"}, {"portrait", "--work-hash", "<svg/>"}} {
		var out, diagnostics bytes.Buffer
		if err := run(args, &out, &diagnostics); err == nil || out.Len() != 0 {
			t.Fatal("invalid portrait emitted output", args, err)
		}
	}
}
