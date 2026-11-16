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
	"reflect"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/internal/prospectnodetest"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/wire/v2"
)

func TestNodeCommands(t *testing.T) {
	fixture, _, proofPath := fixtureFiles(t)
	node := prospectnodetest.New(t, fixture.Context)
	base := []string{"--policy", node.PolicyPath, "--rpc-config", node.ConfigPath}
	var stdout, stderr bytes.Buffer
	err := run(append([]string{"context-node"}, base...), &stdout, &stderr)
	if wire.TestPoW {
		if err == nil || stdout.Len() != 0 || node.Requests() != 0 {
			t.Fatal("cheap profile resolved a production node context")
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	var resolved prospect.Context
	if err := json.Unmarshal(stdout.Bytes(), &resolved); err != nil || resolved != node.Context {
		t.Fatalf("resolved context = %+v, error %v", resolved, err)
	}
	stdout.Reset()
	args := append(append([]string{"verify-node"}, base...), "--proof", proofPath)
	if err := run(args, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	var result struct {
		Source   string            `json:"context_source"`
		Context  prospect.Context  `json:"context"`
		Verified prospect.Verified `json:"verified"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Source != "trusted-node-snapshot" || result.Context != node.Context ||
		!reflect.DeepEqual(result.Verified, fixture.Expected) || result.Verified.Verification != "supplied-context" {
		t.Fatalf("node verification changed evidence or its meaning: %+v", result)
	}
	if stderr.Len() != 0 || !strings.HasSuffix(stdout.String(), "\n") {
		t.Fatalf("unexpected output: %q (%q)", &stdout, &stderr)
	}
}

func TestVerifyNodeRejectsReorg(t *testing.T) {
	fixture, _, proofPath := fixtureFiles(t)
	node := prospectnodetest.New(t, fixture.Context)
	node.ReorgAfter(1)
	var stdout, stderr bytes.Buffer
	err := run([]string{"verify-node", "--policy", node.PolicyPath,
		"--rpc-config", node.ConfigPath, "--proof", proofPath}, &stdout, &stderr)
	if err == nil || stdout.Len() != 0 {
		t.Fatal("changed parent produced a verified acknowledgment")
	}
	if !wire.TestPoW && node.Snapshots() < 2 {
		t.Fatal("test did not reach the post-verification node check")
	}
}

func TestNodeCommandInputs(t *testing.T) {
	fixture, contextPath, proofPath := fixtureFiles(t)
	node := prospectnodetest.New(t, fixture.Context)
	for _, args := range [][]string{
		{"context-node"},
		{"context-node", "--policy", node.PolicyPath},
		{"verify-node", "--policy", node.PolicyPath, "--rpc-config", node.ConfigPath},
		{"context-node", "--policy", node.PolicyPath, "--rpc-config", node.ConfigPath, "--context", contextPath},
		{"verify-node", "--policy", node.PolicyPath, "--rpc-config", node.ConfigPath, "--proof", proofPath, "extra"},
	} {
		var stdout, stderr bytes.Buffer
		if err := run(args, &stdout, &stderr); err == nil || stdout.Len() != 0 {
			t.Fatalf("invalid online arguments accepted: %v", args)
		}
	}
	if node.Requests() != 0 {
		t.Fatal("invalid arguments contacted the node")
	}
	for _, command := range []string{"context-node", "verify-node"} {
		var stdout, stderr bytes.Buffer
		if err := run([]string{command, "-h"}, &stdout, &stderr); !errors.Is(err, flag.ErrHelp) {
			t.Fatalf("help error = %v", err)
		}
		if !strings.Contains(stderr.String(), "usage: slugprospect "+command) {
			t.Fatal("missing online command help")
		}
	}
}

func TestNodeCommandRejectsMalformedConfigurationWithoutSecrets(t *testing.T) {
	for _, test := range []struct {
		name   string
		policy bool
		data   string
	}{
		{"null policy", true, "null"},
		{"oversized policy", true, strings.Repeat(" ", prospectnode.MaxPolicySize+1)},
		{"null config", false, "null"},
		{"secret in unknown field", false, `{"` + prospectnodetest.Password + `":true}`},
		{"oversized config", false, strings.Repeat(" ", prospectnode.MaxRPCConfigSize+1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture, _, _ := fixtureFiles(t)
			node := prospectnodetest.New(t, fixture.Context)
			path := node.ConfigPath
			if test.policy {
				path = node.PolicyPath
			}
			if err := os.WriteFile(path, []byte(test.data), 0600); err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			err := run([]string{"context-node", "--policy", node.PolicyPath, "--rpc-config", node.ConfigPath}, &stdout, &stderr)
			if err == nil || stdout.Len() != 0 || node.Requests() != 0 {
				t.Fatal("malformed configuration contacted the node or produced a context")
			}
			if strings.Contains(err.Error()+stderr.String(), prospectnodetest.Password) {
				t.Fatal("configuration error exposed a secret")
			}
		})
	}
}
