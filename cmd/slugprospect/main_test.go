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
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/wire/v2"
)

// These records were constructed by the independent Python/Argon2 reference,
// not by the production Go proof encoder or verifier.
type referenceFixture struct {
	Context          json.RawMessage   `json:"context"`
	Proof            json.RawMessage   `json:"proof"`
	CommitmentScript string            `json:"commitment_script"`
	Expected         prospect.Verified `json:"expected"`
}

func fixtureFiles(t *testing.T) (referenceFixture, string, string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "prospect", "testdata", "proofs_v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []referenceFixture
	if err := json.Unmarshal(data, &fixtures); err != nil || len(fixtures) == 0 {
		t.Fatalf("decode reference fixtures: %v", err)
	}
	fixture := fixtures[0]
	dir := t.TempDir()
	contextPath := filepath.Join(dir, "context.json")
	proofPath := filepath.Join(dir, "proof.json")
	if err := os.WriteFile(contextPath, fixture.Context, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(proofPath, fixture.Proof, 0600); err != nil {
		t.Fatal(err)
	}
	return fixture, contextPath, proofPath
}

func TestCommitmentCommand(t *testing.T) {
	fixture, contextPath, _ := fixtureFiles(t)
	var stdout, stderr bytes.Buffer
	err := run([]string{"commitment", "--context", contextPath,
		"--gym-id", strings.ToUpper(fixture.Expected.GymID)}, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Version   uint32 `json:"version"`
		ScriptHex string `json:"script_hex"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Version != 1 || got.ScriptHex != fixture.CommitmentScript {
		t.Fatalf("commitment differs from independent reference: %+v", got)
	}
	if stderr.Len() != 0 || !strings.HasSuffix(stdout.String(), "\n") ||
		!strings.Contains(stdout.String(), "\n  \"") {
		t.Fatalf("unexpected CLI formatting: stdout=%q stderr=%q", &stdout, &stderr)
	}
}

func TestVerifyCommand(t *testing.T) {
	fixture, contextPath, proofPath := fixtureFiles(t)
	var stdout, stderr bytes.Buffer
	err := run([]string{"verify", "--context", contextPath, "--proof", proofPath}, &stdout, &stderr)
	if wire.TestPoW {
		if err == nil || stdout.Len() != 0 {
			t.Fatal("test-only PoW build must not return a verified record")
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	var got prospect.Verified
	if err := json.Unmarshal(stdout.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, fixture.Expected) {
		t.Fatalf("verified record differs from independent reference:\ngot %+v\nwant %+v", got, fixture.Expected)
	}
	if stderr.Len() != 0 || !strings.HasSuffix(stdout.String(), "\n") {
		t.Fatalf("unexpected CLI output: stdout=%q stderr=%q", &stdout, &stderr)
	}
}

func TestCommandRejectsInvalidArguments(t *testing.T) {
	fixture, contextPath, proofPath := fixtureFiles(t)
	for _, test := range []struct {
		name string
		args []string
	}{
		{name: "missing command"},
		{name: "unknown command", args: []string{"sign"}},
		{name: "commitment missing context", args: []string{"commitment", "--gym-id", fixture.Expected.GymID}},
		{name: "commitment missing gym", args: []string{"commitment", "--context", contextPath}},
		{name: "verify missing context", args: []string{"verify", "--proof", proofPath}},
		{name: "verify missing proof", args: []string{"verify", "--context", contextPath}},
		{name: "invalid gym", args: []string{"commitment", "--context", contextPath, "--gym-id", "1234"}},
		{name: "zero gym", args: []string{"commitment", "--context", contextPath, "--gym-id", strings.Repeat("0", 64)}},
		{name: "extra argument", args: []string{"verify", "--context", contextPath, "--proof", proofPath, "extra"}},
		{name: "unknown flag", args: []string{"verify", "--target", "ff"}},
		{name: "missing flag value", args: []string{"verify", "--context"}},
		{name: "missing context file", args: []string{"verify", "--context", filepath.Join(t.TempDir(), "absent"), "--proof", proofPath}},
		{name: "missing proof file", args: []string{"verify", "--context", contextPath, "--proof", filepath.Join(t.TempDir(), "absent")}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := run(test.args, &stdout, &stderr)
			if err == nil || errors.Is(err, flag.ErrHelp) {
				t.Fatalf("expected command error, got %v", err)
			}
			if stdout.Len() != 0 {
				t.Fatalf("invalid command produced output: %s", &stdout)
			}
		})
	}
}

func TestCommandRejectsMalformedFiles(t *testing.T) {
	for _, test := range []struct {
		name     string
		context  bool
		contents string
	}{
		{"context null", true, "null"},
		{"context missing fields", true, "{}"},
		{"oversized context whitespace", true, strings.Repeat(" ", prospect.MaxContextSize+1)},
		{"proof null", false, "null"},
		{"proof missing fields", false, "{}"},
		{"oversized proof whitespace", false, strings.Repeat(" ", prospect.MaxProofSize+1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, contextPath, proofPath := fixtureFiles(t)
			path := proofPath
			if test.context {
				path = contextPath
			}
			if err := os.WriteFile(path, []byte(test.contents), 0600); err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			if err := run([]string{"verify", "--context", contextPath, "--proof", proofPath}, &stdout, &stderr); err == nil {
				t.Fatal("accepted malformed file")
			}
			if stdout.Len() != 0 {
				t.Fatal("malformed file produced verified output")
			}
		})
	}
}

func TestReadBoundedRegularFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "input.json")
	const size = 16
	if err := os.WriteFile(path, bytes.Repeat([]byte{'x'}, size), 0600); err != nil {
		t.Fatal(err)
	}
	if data, err := readBoundedFile(path, size); err != nil || len(data) != size {
		t.Fatalf("rejected exact file limit: %d bytes, %v", len(data), err)
	}
	if _, err := readBoundedFile(path, size-1); err == nil {
		t.Fatal("accepted oversized file")
	}
	if _, err := readBoundedFile(dir, size); err == nil {
		t.Fatal("accepted a directory")
	}
}

func TestCommandHelp(t *testing.T) {
	for _, args := range [][]string{{"help"}, {"-h"}, {"--help"}, {"commitment", "-h"}, {"verify", "--help"}} {
		var stdout, stderr bytes.Buffer
		err := run(args, &stdout, &stderr)
		if err != nil && !errors.Is(err, flag.ErrHelp) {
			t.Fatalf("help returned an error: %v", err)
		}
		if !strings.Contains(stdout.String()+stderr.String(), "usage: slugprospect") {
			t.Fatal("help did not print usage")
		}
	}
}

type failedOutput struct{ err error }

func (w failedOutput) Write(_ []byte) (int, error) { return 0, w.err }

func TestCommandReportsOutputFailure(t *testing.T) {
	fixture, contextPath, _ := fixtureFiles(t)
	want := errors.New("output unavailable")
	err := run([]string{"commitment", "--context", contextPath, "--gym-id", fixture.Expected.GymID}, failedOutput{want}, io.Discard)
	if !errors.Is(err, want) {
		t.Fatalf("output error = %v, want %v", err, want)
	}
}
