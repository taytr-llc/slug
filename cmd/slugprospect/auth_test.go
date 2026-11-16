// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/internal/prospectnodetest"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/wire/v2"
)

type authorizationFixture struct {
	Context          json.RawMessage       `json:"context"`
	Proof            json.RawMessage       `json:"proof"`
	Authorization    gymauth.Authorization `json:"authorization"`
	ExpectedGymID    string                `json:"expected_gym_id"`
	ExpectedDigest   string                `json:"expected_digest"`
	ExpectedVerified prospect.Verified     `json:"expected_verified"`
	HighS            string                `json:"invalid_high_s_signature"`
}

// Cryptographic expectations come from the independently implemented reference.
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
	dir := t.TempDir()
	contextPath := filepath.Join(dir, "context.json")
	proofPath := filepath.Join(dir, "proof.json")
	authorizationPath := filepath.Join(dir, "authorization.json")
	authorization, _ := json.Marshal(f.Authorization)
	for path, data := range map[string][]byte{contextPath: f.Context, proofPath: f.Proof, authorizationPath: authorization} {
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return f, contextPath, proofPath, authorizationPath
}

func TestPublicAuthorizationPreparation(t *testing.T) {
	f, contextPath, proofPath, _ := authorizationFiles(t)
	var stdout, stderr bytes.Buffer
	err := run([]string{"gym-id", "--context", contextPath, "--public-key", strings.ToUpper(f.Authorization.PublicKey)}, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	var gym struct {
		Version uint32 `json:"version"`
		GymID   string `json:"gym_id"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &gym); err != nil || gym.Version != 1 || gym.GymID != f.ExpectedGymID {
		t.Fatalf("gym ID differs from independent fixture: %+v (%v)", gym, err)
	}
	stdout.Reset()
	err = run([]string{"auth-payload", "--context", contextPath, "--proof", proofPath,
		"--public-key", strings.ToUpper(f.Authorization.PublicKey)}, &stdout, &stderr)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Version   uint32 `json:"version"`
		Action    string `json:"action"`
		GymID     string `json:"gym_id"`
		PublicKey string `json:"public_key"`
		Digest    string `json:"digest"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Version != 1 || payload.Action != "prospect-admission" || payload.GymID != f.ExpectedGymID ||
		payload.PublicKey != f.Authorization.PublicKey || payload.Digest != f.ExpectedDigest {
		t.Fatalf("public signing payload differs from independent fixture: %+v", payload)
	}
	if stderr.Len() != 0 || !strings.HasSuffix(stdout.String(), "\n") {
		t.Fatal("unexpected preparation output")
	}
}

func TestAuthorizationVerificationCommands(t *testing.T) {
	f, contextPath, proofPath, authorizationPath := authorizationFiles(t)
	for _, online := range []bool{false, true} {
		name := "offline"
		if online {
			name = "online"
		}
		t.Run(name, func(t *testing.T) {
			args := []string{"verify-auth", "--context", contextPath, "--proof", proofPath, "--authorization", authorizationPath}
			var node *prospectnodetest.Node
			if online {
				node = prospectnodetest.New(t, f.Context)
				args = []string{"verify-auth-node", "--policy", node.PolicyPath, "--rpc-config", node.ConfigPath,
					"--proof", proofPath, "--authorization", authorizationPath}
			}
			var stdout, stderr bytes.Buffer
			err := run(args, &stdout, &stderr)
			if wire.TestPoW {
				if err == nil || stdout.Len() != 0 || (node != nil && node.Requests() != 0) {
					t.Fatal("cheap profile authenticated production work")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			var result struct {
				Authentication string            `json:"authentication"`
				Source         string            `json:"context_source"`
				Verified       prospect.Verified `json:"verified"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if result.Authentication != "gym-key-control" || !reflect.DeepEqual(result.Verified, f.ExpectedVerified) ||
				result.Verified.Verification != "supplied-context" {
				t.Fatalf("wrong authentication result: %+v", result)
			}
			if online && (result.Source != "trusted-node-snapshot" || node.Snapshots() != 2) {
				t.Fatal("online authorization did not resolve and recheck")
			}
		})
	}
}

func TestAuthorizationRejectsMalformedOrNoncanonicalInput(t *testing.T) {
	for _, name := range []string{"missing fields", "null", "oversized", "high S", "bad signature"} {
		t.Run(name, func(t *testing.T) {
			f, contextPath, proofPath, authorizationPath := authorizationFiles(t)
			var data []byte
			switch name {
			case "missing fields":
				data = []byte(`{}`)
			case "null":
				data = []byte("null")
			case "oversized":
				data = bytes.Repeat([]byte{' '}, gymauth.MaxAuthorizationSize+1)
			case "high S":
				f.Authorization.Signature = f.HighS
				data, _ = json.Marshal(f.Authorization)
			case "bad signature":
				f.Authorization.Signature = f.Authorization.Signature[:len(f.Authorization.Signature)-2] + "00"
				data, _ = json.Marshal(f.Authorization)
			}
			if err := os.WriteFile(authorizationPath, data, 0600); err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			if err := run([]string{"verify-auth", "--context", contextPath, "--proof", proofPath,
				"--authorization", authorizationPath}, &stdout, &stderr); err == nil || stdout.Len() != 0 {
				t.Fatal("invalid authorization produced success output")
			}
		})
	}
}

func TestVerifyAuthNodeRejectsReorg(t *testing.T) {
	f, _, proofPath, authorizationPath := authorizationFiles(t)
	node := prospectnodetest.New(t, f.Context)
	node.ReorgAfter(1)
	var stdout, stderr bytes.Buffer
	err := run([]string{"verify-auth-node", "--policy", node.PolicyPath,
		"--rpc-config", node.ConfigPath, "--proof", proofPath,
		"--authorization", authorizationPath}, &stdout, &stderr)
	if err == nil || stdout.Len() != 0 {
		t.Fatal("changed parent produced authenticated verification output")
	}
	if !wire.TestPoW && node.Snapshots() != 2 {
		t.Fatal("test did not reach the authenticated verification recheck")
	}
}

func TestAuthorizationCommandArgumentsAndHelp(t *testing.T) {
	f, contextPath, proofPath, authorizationPath := authorizationFiles(t)
	for _, args := range [][]string{
		{"gym-id", "--context", contextPath},
		{"auth-payload", "--context", contextPath, "--public-key", f.Authorization.PublicKey},
		{"verify-auth", "--context", contextPath, "--proof", proofPath},
		{"verify-auth-node", "--proof", proofPath, "--authorization", authorizationPath},
		{"gym-id", "--context", contextPath, "--public-key", f.Authorization.PublicKey, "extra"},
		{"auth-payload", "--context", contextPath, "--private-key", "not-supported"},
		{"sign"}, {"keygen"},
	} {
		var stdout, stderr bytes.Buffer
		if err := run(args, &stdout, &stderr); err == nil || stdout.Len() != 0 {
			t.Fatalf("invalid auth arguments accepted: %v", args)
		}
	}
	for _, command := range []string{"gym-id", "auth-payload", "verify-auth", "verify-auth-node"} {
		var stdout, stderr bytes.Buffer
		if err := run([]string{command, "-h"}, &stdout, &stderr); !errors.Is(err, flag.ErrHelp) ||
			!strings.Contains(stderr.String(), "usage: slugprospect "+command) {
			t.Fatalf("authorization help failed: %v", err)
		}
	}
}
