// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"strings"
	"testing"
)

const testSeed = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"

func TestRunJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if err := run([]string{strings.ToUpper(testSeed)}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected diagnostics: %s", &stderr)
	}
	output := stdout.String()
	if !strings.HasSuffix(output, "\n") || !strings.Contains(output, "\n  \"") {
		t.Fatalf("expected indented JSON followed by a newline: %q", output)
	}
	var fields map[string]json.RawMessage
	decoder := json.NewDecoder(&stdout)
	if err := decoder.Decode(&fields); err != nil {
		t.Fatalf("invalid JSON output: %v", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		t.Fatalf("unexpected data after JSON object: %v", err)
	}
	wantFields := []string{
		"dna_version", "seed", "rarity", "talent_budget", "attributes",
		"prime_weeks", "decline_weeks",
	}
	if len(fields) != len(wantFields) {
		t.Fatalf("unexpected JSON fields: %v", fields)
	}
	for _, name := range wantFields {
		if _, ok := fields[name]; !ok {
			t.Errorf("missing JSON field %q", name)
		}
	}
	if string(fields["dna_version"]) != "1" {
		t.Fatalf("default DNA version = %s, want 1", fields["dna_version"])
	}
	var seed string
	if err := json.Unmarshal(fields["seed"], &seed); err != nil || seed != testSeed {
		t.Fatalf("seed must retain its bytes and use lowercase hex: %q (%v)", seed, err)
	}
	var attributes map[string]json.RawMessage
	if err := json.Unmarshal(fields["attributes"], &attributes); err != nil {
		t.Fatal(err)
	}
	if len(attributes) != 5 {
		t.Fatalf("unexpected attribute fields: %v", attributes)
	}
	for _, name := range []string{"power", "speed", "chin", "stamina", "skill"} {
		if _, ok := attributes[name]; !ok {
			t.Errorf("missing attribute %q", name)
		}
	}

	stdout.Reset()
	if err := run([]string{"-dna-version=1", testSeed}, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if stdout.String() != output {
		t.Fatal("explicit DNA version 1 differs from the default")
	}
}

func TestRunRejectsInvalidArguments(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
	}{
		{name: "missing hash"},
		{name: "extra argument", args: []string{testSeed, testSeed}},
		{name: "empty hash", args: []string{""}},
		{name: "short hash", args: []string{testSeed[:62]}},
		{name: "long hash", args: []string{testSeed + "00"}},
		{name: "nonhex hash", args: []string{strings.Repeat("g", 64)}},
		{name: "prefixed hash", args: []string{"0x" + testSeed}},
		{name: "whitespace hash", args: []string{" " + testSeed}},
		{name: "version zero", args: []string{"-dna-version=0", testSeed}},
		{name: "unknown version", args: []string{"-dna-version=2", testSeed}},
		{name: "max uint32 version", args: []string{"-dna-version=4294967295", testSeed}},
		{name: "uint32 overflow", args: []string{"-dna-version=4294967296", testSeed}},
		{name: "uint64 overflow", args: []string{"-dna-version=18446744073709551616", testSeed}},
		{name: "negative version", args: []string{"-dna-version=-1", testSeed}},
		{name: "nonnumeric version", args: []string{"-dna-version=one", testSeed}},
		{name: "unknown flag", args: []string{"-unknown", testSeed}},
		{name: "flag after hash", args: []string{testSeed, "-dna-version=1"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := run(test.args, &stdout, &stderr)
			if err == nil || errors.Is(err, flag.ErrHelp) {
				t.Fatalf("expected argument error, got %v", err)
			}
			if stdout.Len() != 0 {
				t.Fatalf("invalid arguments produced preview output: %s", &stdout)
			}
		})
	}
}

func TestRunHelp(t *testing.T) {
	for _, arg := range []string{"-h", "--help"} {
		var stdout, stderr bytes.Buffer
		if err := run([]string{arg}, &stdout, &stderr); !errors.Is(err, flag.ErrHelp) {
			t.Fatalf("%s returned %v, want flag.ErrHelp", arg, err)
		}
		if stdout.Len() != 0 || !strings.Contains(stderr.String(), usage) ||
			!strings.Contains(stderr.String(), "-dna-version") {
			t.Fatalf("unexpected help output: stdout=%q stderr=%q", &stdout, &stderr)
		}
	}
}

type failingWriter struct{ err error }

func (w failingWriter) Write(_ []byte) (int, error) { return 0, w.err }

func TestRunReportsOutputFailure(t *testing.T) {
	want := errors.New("output unavailable")
	if err := run([]string{testSeed}, failingWriter{want}, io.Discard); !errors.Is(err, want) {
		t.Fatalf("output error = %v, want %v", err, want)
	}
}
