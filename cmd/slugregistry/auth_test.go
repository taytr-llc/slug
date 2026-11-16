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

	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/internal/prospectnodetest"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

type authorizationFixture struct {
	Context       json.RawMessage       `json:"context"`
	Proof         json.RawMessage       `json:"proof"`
	Authorization gymauth.Authorization `json:"authorization"`
	Expected      prospect.Verified     `json:"expected_verified"`
}

func authorizationFiles(t *testing.T) (authorizationFixture, string, string, string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "gymauth", "testdata", "authorizations_v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []authorizationFixture
	if err := json.Unmarshal(data, &fixtures); err != nil || len(fixtures) < 2 {
		t.Fatalf("read independent authorization fixtures: %v", err)
	}
	f := fixtures[0]
	contextPath, proofPath := fixtureFiles(t, fixture{Context: f.Context, Proof: f.Proof})
	authorizationPath := filepath.Join(filepath.Dir(contextPath), "authorization.json")
	authorization, _ := json.Marshal(f.Authorization)
	if err := os.WriteFile(authorizationPath, authorization, 0600); err != nil {
		t.Fatal(err)
	}
	return f, contextPath, proofPath, authorizationPath
}

func initializeAuthenticatedRegistry(t *testing.T, f authorizationFixture) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "authenticated-registry")
	invoke(t, "init-auth", "--db", path, "--network", fmt.Sprint(f.Expected.Network), "--genesis", f.Expected.Genesis)
	return path
}

func TestAuthenticatedRegistryCommands(t *testing.T) {
	f, contextPath, proofPath, authorizationPath := authorizationFiles(t)
	path := initializeAuthenticatedRegistry(t, f)
	args := []string{"record-auth", "--db", path, "--context", contextPath,
		"--proof", proofPath, "--authorization", authorizationPath}
	var first registry.Record
	for i := 0; i < 2; i++ {
		var stdout, stderr bytes.Buffer
		err := run(args, &stdout, &stderr)
		if wire.TestPoW {
			if err == nil || stdout.Len() != 0 || invoke(t, "list", "--db", path) != "[]\n" {
				t.Fatal("cheap profile admitted authenticated production evidence")
			}
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		var result struct {
			Authentication string          `json:"authentication"`
			Inserted       bool            `json:"inserted"`
			Record         registry.Record `json:"record"`
		}
		if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.Authentication != "gym-key-control" || result.Inserted != (i == 0) ||
			result.Record.Version != 2 || result.Record.Sequence != 1 || result.Record.Authorization == nil ||
			!reflect.DeepEqual(*result.Record.Authorization, f.Authorization) || !reflect.DeepEqual(result.Record.Verified, f.Expected) {
			t.Fatalf("wrong authenticated receipt: %+v", result)
		}
		if i == 0 {
			first = result.Record
		} else if !reflect.DeepEqual(first, result.Record) {
			t.Fatal("replay changed first authenticated evidence")
		}
	}
	var got registry.Record
	if err := json.Unmarshal([]byte(invoke(t, "get", "--db", path, "--id", f.Expected.ProspectID)), &got); err != nil ||
		!reflect.DeepEqual(first, got) {
		t.Fatalf("reopen lost authorization: %v", err)
	}
	before := invoke(t, "export", "--db", path)
	var stdout, stderr bytes.Buffer
	err := run([]string{"init-auth", "--db", path, "--network", fmt.Sprint(f.Expected.Network), "--genesis", f.Expected.Genesis}, &stdout, &stderr)
	if err == nil || stdout.Len() != 0 || invoke(t, "export", "--db", path) != before {
		t.Fatal("repeat initialization replaced authenticated evidence")
	}
}

func TestAuthenticationStoreBoundaries(t *testing.T) {
	f, contextPath, proofPath, authorizationPath := authorizationFiles(t)
	authPath := initializeAuthenticatedRegistry(t, f)
	legacyPath := initializeFixtureRegistry(t, fixture{Expected: f.Expected})
	for _, args := range [][]string{
		{"record", "--db", authPath, "--context", contextPath, "--proof", proofPath},
		{"record-auth", "--db", legacyPath, "--context", contextPath, "--proof", proofPath, "--authorization", authorizationPath},
	} {
		var stdout, stderr bytes.Buffer
		if err := run(args, &stdout, &stderr); err == nil || stdout.Len() != 0 {
			t.Fatal("admission crossed the explicit store schema boundary")
		}
	}
	for _, path := range []string{authPath, legacyPath} {
		if invoke(t, "list", "--db", path) != "[]\n" {
			t.Fatal("rejected admission consumed a receipt")
		}
	}
	if err := os.WriteFile(authorizationPath, []byte(`{"version":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	if err := run([]string{"record-auth", "--db", authPath, "--context", contextPath, "--proof", proofPath,
		"--authorization", authorizationPath}, &stdout, &stderr); err == nil || stdout.Len() != 0 ||
		invoke(t, "list", "--db", authPath) != "[]\n" {
		t.Fatal("malformed authorization produced a receipt")
	}
}

func TestAuthenticatedNodeAdmissionAndReorgGuard(t *testing.T) {
	f, _, proofPath, authorizationPath := authorizationFiles(t)
	path := initializeAuthenticatedRegistry(t, f)
	node := prospectnodetest.New(t, f.Context)
	args := append(nodeArguments(path, proofPath, node), "--authorization", authorizationPath)
	args[0] = "record-auth-node"
	node.ReorgAfter(1)
	var stdout, stderr bytes.Buffer
	err := run(args, &stdout, &stderr)
	if err == nil || stdout.Len() != 0 || invoke(t, "list", "--db", path) != "[]\n" {
		t.Fatal("failed authenticated node guard wrote or acknowledged evidence")
	}
	if wire.TestPoW {
		if node.Requests() != 0 {
			t.Fatal("cheap profile contacted the production node")
		}
		return
	}
	if node.Snapshots() != 2 {
		t.Fatal("test did not reach the post-verification guard")
	}
	stable := prospectnodetest.New(t, f.Context)
	args = append(nodeArguments(path, proofPath, stable), "--authorization", authorizationPath)
	args[0] = "record-auth-node"
	var result struct {
		Authentication string          `json:"authentication"`
		ContextSource  string          `json:"context_source"`
		Inserted       bool            `json:"inserted"`
		Record         registry.Record `json:"record"`
	}
	if err := json.Unmarshal([]byte(invoke(t, args...)), &result); err != nil {
		t.Fatal(err)
	}
	if result.Authentication != "gym-key-control" || result.ContextSource != "trusted-node-snapshot" ||
		!result.Inserted || result.Record.Authorization == nil || !reflect.DeepEqual(result.Record.Verified, f.Expected) {
		t.Fatalf("incorrect authenticated node output: %+v", result)
	}
	before := invoke(t, "export", "--db", path)
	stable.ReorgAfter(stable.Snapshots() + 1)
	stdout.Reset()
	err = run(args, &stdout, &stderr)
	if err == nil || stdout.Len() != 0 || invoke(t, "export", "--db", path) != before {
		t.Fatal("failed replay guard replaced or acknowledged authenticated evidence")
	}
}

func TestAuthenticatedOutputFailureAllowsReplay(t *testing.T) {
	f, contextPath, proofPath, authorizationPath := authorizationFiles(t)
	path := initializeAuthenticatedRegistry(t, f)
	args := []string{"record-auth", "--db", path, "--context", contextPath, "--proof", proofPath, "--authorization", authorizationPath}
	want := errors.New("output unavailable")
	err := run(args, failedOutput{want}, io.Discard)
	if wire.TestPoW {
		if err == nil || errors.Is(err, want) {
			t.Fatal("cheap profile must refuse before acknowledgment")
		}
		return
	}
	if !errors.Is(err, want) {
		t.Fatalf("output error = %v, want %v", err, want)
	}
	var result recordResult
	if err := json.Unmarshal([]byte(invoke(t, args...)), &result); err != nil || result.Inserted ||
		result.Record.Sequence != 1 || result.Record.Authorization == nil {
		t.Fatalf("retry after output failure lost authenticated receipt: %v", err)
	}
}

func TestAuthenticatedCommandArgumentsAndHelp(t *testing.T) {
	for _, command := range []string{"init-auth", "record-auth", "record-auth-node"} {
		var stdout, stderr bytes.Buffer
		if err := run([]string{command}, &stdout, &stderr); err == nil || stdout.Len() != 0 {
			t.Fatalf("%s accepted missing arguments", command)
		}
		if err := run([]string{command, "-h"}, &stdout, &stderr); !errors.Is(err, flag.ErrHelp) ||
			!strings.Contains(stderr.String(), "usage: slugregistry "+command) {
			t.Fatalf("help failed for %s: %v", command, err)
		}
	}
}
