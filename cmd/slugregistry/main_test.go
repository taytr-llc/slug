// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

type fixture struct {
	Context  json.RawMessage   `json:"context"`
	Proof    json.RawMessage   `json:"proof"`
	Expected prospect.Verified `json:"expected"`
}

func fixtures(t *testing.T) []fixture {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "prospect", "testdata", "proofs_v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var result []fixture
	if err := json.Unmarshal(data, &result); err != nil || len(result) < 2 {
		t.Fatalf("read independent prospect fixtures: %v", err)
	}
	return result
}

func fixtureFiles(t *testing.T, f fixture) (string, string) {
	t.Helper()
	dir := t.TempDir()
	contextPath, proofPath := filepath.Join(dir, "context.json"), filepath.Join(dir, "proof.json")
	if err := os.WriteFile(contextPath, f.Context, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(proofPath, f.Proof, 0600); err != nil {
		t.Fatal(err)
	}
	return contextPath, proofPath
}

func invoke(t *testing.T, args ...string) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	if err := run(args, &stdout, &stderr); err != nil {
		t.Fatalf("run %v: %v (%s)", args, err, &stderr)
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected diagnostics: %s", &stderr)
	}
	return stdout.String()
}

func initializeFixtureRegistry(t *testing.T, f fixture) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "registry")
	output := invoke(t, "init", "--db", path, "--network",
		fmt.Sprint(f.Expected.Network), "--genesis", strings.ToUpper(f.Expected.Genesis))
	var got struct {
		Initialized bool   `json:"initialized"`
		Network     uint32 `json:"network"`
		Genesis     string `json:"genesis"`
	}
	if err := json.Unmarshal([]byte(output), &got); err != nil {
		t.Fatal(err)
	}
	if !got.Initialized || got.Network != f.Expected.Network || got.Genesis != f.Expected.Genesis {
		t.Fatalf("incorrect initialization output: %+v", got)
	}
	return path
}

type recordResult struct {
	Inserted bool            `json:"inserted"`
	Record   registry.Record `json:"record"`
}

func recordFixture(t *testing.T, path string, f fixture) recordResult {
	t.Helper()
	contextPath, proofPath := fixtureFiles(t, f)
	output := invoke(t, "record", "--db", path, "--context", contextPath, "--proof", proofPath)
	var result recordResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(output, "\n") || !strings.Contains(output, "\n  \"") {
		t.Fatal("record output must be indented JSON with a final newline")
	}
	return result
}

// Every command opens and closes the actual LevelDB store, so this exercises
// persistence and replay through the same boundary used by separate CLI runs.
func TestRegistryCommandsPersistAndReplay(t *testing.T) {
	refs := fixtures(t)
	path := initializeFixtureRegistry(t, refs[0])
	if output := invoke(t, "list", "--db", path); output != "[]\n" {
		t.Fatalf("empty registry list = %q", output)
	}
	if output := invoke(t, "export", "--db", path); output != "" {
		t.Fatalf("empty registry export = %q", output)
	}
	if wire.TestPoW {
		contextPath, proofPath := fixtureFiles(t, refs[0])
		var stdout, stderr bytes.Buffer
		if err := run([]string{"record", "--db", path, "--context", contextPath, "--proof", proofPath}, &stdout, &stderr); err == nil || stdout.Len() != 0 {
			t.Fatal("cheap PoW admitted production evidence")
		}
		if output := invoke(t, "list", "--db", path); output != "[]\n" {
			t.Fatal("failed admission changed the registry")
		}
		return
	}

	first := recordFixture(t, path, refs[0])
	second := recordFixture(t, path, refs[1])
	for i, result := range []recordResult{first, second} {
		if !result.Inserted || result.Record.Version != 1 || result.Record.Sequence != uint64(i+1) ||
			!reflect.DeepEqual(result.Record.Verified, refs[i].Expected) {
			t.Fatalf("incorrect stored record %d: %+v", i, result)
		}
	}

	// A different presentation and wider valid time interval must not
	// replace the first supplied evidence or allocate a new receipt number.
	var context prospect.Context
	var proof prospect.Proof
	if err := json.Unmarshal(refs[0].Context, &context); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(refs[0].Proof, &proof); err != nil {
		t.Fatal(err)
	}
	context.Genesis = strings.ToUpper(context.Genesis)
	context.ParentHash = strings.ToUpper(context.ParentHash)
	context.ProspectTarget = strings.ToUpper(context.ProspectTarget)
	context.MinTimestamp--
	context.MaxTimestamp++
	proof.GymID = strings.ToUpper(proof.GymID)
	proof.Header = strings.ToUpper(proof.Header)
	proof.Coinbase = strings.ToUpper(proof.Coinbase)
	alternate := refs[0]
	alternate.Context, _ = json.Marshal(context)
	alternate.Proof, _ = json.Marshal(proof)
	replayed := recordFixture(t, path, alternate)
	if replayed.Inserted || !reflect.DeepEqual(replayed.Record, first.Record) {
		t.Fatalf("replay changed original record: %+v", replayed)
	}

	var got registry.Record
	output := invoke(t, "get", "--db", path, "--id", strings.ToUpper(first.Record.Verified.ProspectID))
	if err := json.Unmarshal([]byte(output), &got); err != nil || !reflect.DeepEqual(got, first.Record) {
		t.Fatalf("get after reopen differs: %+v (%v)", got, err)
	}
	for i, want := range []registry.Record{first.Record, second.Record} {
		var page []registry.Record
		output := invoke(t, "list", "--db", path, "--after", fmt.Sprint(i), "--limit", "1")
		if err := json.Unmarshal([]byte(output), &page); err != nil || len(page) != 1 || !reflect.DeepEqual(page[0], want) {
			t.Fatalf("wrong page after %d: %+v (%v)", i, page, err)
		}
	}
	if output := invoke(t, "list", "--db", path, "--after", "2"); output != "[]\n" {
		t.Fatalf("unexpected final page: %s", output)
	}
	output = invoke(t, "export", "--db", path)
	lines := strings.Split(strings.TrimSuffix(output, "\n"), "\n")
	if len(lines) != 2 || !strings.HasSuffix(output, "\n") {
		t.Fatalf("expected two NDJSON lines: %q", output)
	}
	for i, want := range []registry.Record{first.Record, second.Record} {
		var got registry.Record
		if err := json.Unmarshal([]byte(lines[i]), &got); err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("incorrect exported record %d: %+v (%v)", i, got, err)
		}
	}

	var stdout, stderr bytes.Buffer
	if err := run([]string{"init", "--db", path, "--network", fmt.Sprint(refs[0].Expected.Network), "--genesis", refs[0].Expected.Genesis}, &stdout, &stderr); err == nil || stdout.Len() != 0 {
		t.Fatal("init must refuse an existing registry")
	}
	if after := invoke(t, "export", "--db", path); after != output {
		t.Fatal("failed initialization changed existing records")
	}
}

func TestInvalidArgumentsDoNotCreateRegistry(t *testing.T) {
	for _, test := range []struct {
		name string
		args []string
	}{
		{name: "no command"},
		{name: "unknown command", args: []string{"delete"}},
		{name: "missing db", args: []string{"list"}},
		{name: "missing network", args: []string{"init", "--db", "DB", "--genesis", strings.Repeat("1", 64)}},
		{name: "zero network", args: []string{"init", "--db", "DB", "--network", "0", "--genesis", strings.Repeat("1", 64)}},
		{name: "network uint32 overflow", args: []string{"init", "--db", "DB", "--network", "4294967296", "--genesis", strings.Repeat("1", 64)}},
		{name: "network uint64 overflow", args: []string{"init", "--db", "DB", "--network", "18446744073709551616"}},
		{name: "negative network", args: []string{"init", "--db", "DB", "--network", "-1"}},
		{name: "missing genesis", args: []string{"init", "--db", "DB", "--network", "1"}},
		{name: "invalid genesis", args: []string{"init", "--db", "DB", "--network", "1", "--genesis", "abcd"}},
		{name: "missing context", args: []string{"record", "--db", "DB", "--proof", "absent"}},
		{name: "missing proof", args: []string{"record", "--db", "DB", "--context", "absent"}},
		{name: "missing id", args: []string{"get", "--db", "DB"}},
		{name: "zero limit", args: []string{"list", "--db", "DB", "--limit", "0"}},
		{name: "negative limit", args: []string{"list", "--db", "DB", "--limit", "-1"}},
		{name: "oversized page", args: []string{"list", "--db", "DB", "--limit", "101"}},
		{name: "negative cursor", args: []string{"list", "--db", "DB", "--after", "-1"}},
		{name: "cursor overflow", args: []string{"list", "--db", "DB", "--after", "18446744073709551616"}},
		{name: "extra positional", args: []string{"export", "--db", "DB", "extra"}},
		{name: "unknown flag", args: []string{"record", "--db", "DB", "--force"}},
		{name: "get missing store", args: []string{"get", "--db", "DB", "--id", strings.Repeat("1", 64)}},
		{name: "list missing store", args: []string{"list", "--db", "DB"}},
		{name: "export missing store", args: []string{"export", "--db", "DB"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "not-created")
			args := append([]string(nil), test.args...)
			for i := range args {
				if args[i] == "DB" {
					args[i] = path
				}
			}
			var stdout, stderr bytes.Buffer
			if err := run(args, &stdout, &stderr); err == nil || errors.Is(err, flag.ErrHelp) {
				t.Fatalf("expected input error, got %v", err)
			}
			if stdout.Len() != 0 {
				t.Fatal("invalid arguments produced success output")
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("invalid command created registry path: %v", err)
			}
		})
	}
}

func TestMalformedFilesDoNotRecord(t *testing.T) {
	f := fixtures(t)[0]
	path := initializeFixtureRegistry(t, f)
	for _, test := range []struct {
		name    string
		context bool
		data    []byte
	}{
		{"null context", true, []byte("null")},
		{"oversized context", true, append(append([]byte(nil), f.Context...), bytes.Repeat([]byte{' '}, prospect.MaxContextSize)...)},
		{"null proof", false, []byte("null")},
		{"oversized proof", false, append(append([]byte(nil), f.Proof...), bytes.Repeat([]byte{' '}, prospect.MaxProofSize)...)},
		{"null branch item", false, []byte(strings.Replace(string(f.Proof), `"merkle_branch": []`, `"merkle_branch": [null]`, 1))},
	} {
		t.Run(test.name, func(t *testing.T) {
			contextPath, proofPath := fixtureFiles(t, f)
			inputPath := proofPath
			if test.context {
				inputPath = contextPath
			}
			if err := os.WriteFile(inputPath, test.data, 0600); err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			if err := run([]string{"record", "--db", path, "--context", contextPath, "--proof", proofPath}, &stdout, &stderr); err == nil || stdout.Len() != 0 {
				t.Fatal("malformed input was acknowledged")
			}
			if output := invoke(t, "list", "--db", path); output != "[]\n" {
				t.Fatal("malformed proof consumed a receipt")
			}
		})
	}
}

func TestBoundedRegularFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "input")
	if err := os.WriteFile(path, []byte("abcd"), 0600); err != nil {
		t.Fatal(err)
	}
	if data, err := readBoundedFile(path, 4); err != nil || string(data) != "abcd" {
		t.Fatalf("exact-size file read failed: %q (%v)", data, err)
	}
	if _, err := readBoundedFile(path, 3); err == nil {
		t.Fatal("read exceeded file limit")
	}
	if _, err := readBoundedFile(dir, 4); err == nil {
		t.Fatal("read a directory")
	}
	if _, err := readBoundedFile(os.DevNull, 4); err == nil {
		t.Fatal("accepted a nonregular device as an input file")
	}
}

func TestHelp(t *testing.T) {
	for _, args := range [][]string{{"help"}, {"--help"}, {"init", "-h"}, {"record", "-h"}, {"get", "-h"}, {"list", "-h"}, {"export", "-h"}} {
		var stdout, stderr bytes.Buffer
		err := run(args, &stdout, &stderr)
		if err != nil && !errors.Is(err, flag.ErrHelp) {
			t.Fatalf("help error: %v", err)
		}
		if !strings.Contains(stdout.String()+stderr.String(), "usage: slugregistry") {
			t.Fatal("missing usage")
		}
	}
}

type failedOutput struct{ err error }

func (w failedOutput) Write(_ []byte) (int, error) { return 0, w.err }

func TestOutputFailureAllowsSafeReplay(t *testing.T) {
	f := fixtures(t)[0]
	path := initializeFixtureRegistry(t, f)
	contextPath, proofPath := fixtureFiles(t, f)
	want := errors.New("output unavailable")
	err := run([]string{"record", "--db", path, "--context", contextPath, "--proof", proofPath}, failedOutput{want}, io.Discard)
	if wire.TestPoW {
		if err == nil || errors.Is(err, want) {
			t.Fatal("cheap profile should refuse admission before producing output")
		}
		return
	}
	if !errors.Is(err, want) {
		t.Fatalf("output error = %v, want %v", err, want)
	}
	result := recordFixture(t, path, f)
	if result.Inserted || result.Record.Sequence != 1 || !reflect.DeepEqual(result.Record.Verified, f.Expected) {
		t.Fatalf("output failure lost or duplicated committed evidence: %+v", result)
	}
	err = run([]string{"export", "--db", path}, failedOutput{want}, io.Discard)
	if !errors.Is(err, want) {
		t.Fatalf("export output error = %v, want %v", err, want)
	}
	// A failed stream must still close the store and release its lock.
	if replay := recordFixture(t, path, f); replay.Inserted {
		t.Fatal("failed export changed the record")
	}
}
