// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"bytes"
	"encoding/json"
	"os"
	"strconv"
	"testing"

	"github.com/taytr-llc/slug/fightercondition"
)

func TestConditionPreviewIndependentOutput(t *testing.T) {
	raw, err := os.ReadFile("../../fightercondition/testdata/reference/condition_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var corpus struct {
		DNA []struct {
			Seed string `json:"seed"`
		} `json:"dna_cases"`
		Cases []struct {
			DNAIndex int                       `json:"dna_index"`
			Expected fightercondition.Snapshot `json:"expected"`
		} `json:"cases"`
	}
	// Use the integer-aware expected snapshot; its timestamps and XP are the
	// same inputs, including values beyond JavaScript's safe-integer range.
	if json.Unmarshal(raw, &corpus) != nil || len(corpus.Cases) == 0 {
		t.Fatal("condition corpus")
	}
	selected := map[string]bool{}
	for _, c := range corpus.Cases {
		key := c.Expected.Phase + ":" + strconv.Itoa(c.DNAIndex) + ":" + strconv.Itoa(c.Expected.ProXP)
		if selected[key] && c.Expected.AsOfMS < 9007199254740991 {
			continue
		}
		selected[key] = true
		var out, diagnostics bytes.Buffer
		args := []string{"condition", "--work-hash", corpus.DNA[c.DNAIndex].Seed, "--born-at-ms", strconv.FormatInt(c.Expected.BornAtMS, 10), "--as-of-ms", strconv.FormatInt(c.Expected.AsOfMS, 10), "--pro-xp", strconv.Itoa(c.Expected.ProXP)}
		if err := run(args, &out, &diagnostics); err != nil || diagnostics.Len() != 0 {
			t.Fatal("condition CLI", err)
		}
		var got fightercondition.Snapshot
		if err := json.Unmarshal(out.Bytes(), &got); err != nil || got != c.Expected {
			t.Fatal("condition differs from independent snapshot", err)
		}
	}
	seed := corpus.DNA[0].Seed
	valid := []string{"condition", "--work-hash", seed, "--born-at-ms", "1", "--as-of-ms", "1"}
	for _, extra := range [][]string{{"--version", "2"}, {"--version", "4294967297"}, {"--born-at-ms", "0"}, {"--as-of-ms", "0"}, {"--as-of-ms", "9223372036854775808"}, {"--pro-xp", "16"}, {"extra"}} {
		var out, diagnostics bytes.Buffer
		args := append(append([]string{}, valid...), extra...)
		if err := run(args, &out, &diagnostics); err == nil || out.Len() != 0 {
			t.Fatal("invalid condition emitted output", args, err)
		}
	}
}
