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

	"github.com/taytr-llc/slug/internal/prospectnodetest"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

func nodeArguments(path, proofPath string, node *prospectnodetest.Node) []string {
	return []string{"record-node", "--db", path, "--policy", node.PolicyPath,
		"--rpc-config", node.ConfigPath, "--proof", proofPath}
}

func TestRecordNodePersistsAndReplays(t *testing.T) {
	f := fixtures(t)[0]
	path := initializeFixtureRegistry(t, f)
	_, proofPath := fixtureFiles(t, f)
	node := prospectnodetest.New(t, f.Context)
	args := nodeArguments(path, proofPath, node)
	var first registry.Record
	for i := 0; i < 2; i++ {
		var stdout, stderr bytes.Buffer
		err := run(args, &stdout, &stderr)
		if wire.TestPoW {
			if err == nil || stdout.Len() != 0 || node.Requests() != 0 || invoke(t, "list", "--db", path) != "[]\n" {
				t.Fatal("cheap profile contacted or admitted production node evidence")
			}
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		var result struct {
			Source   string          `json:"context_source"`
			Inserted bool            `json:"inserted"`
			Record   registry.Record `json:"record"`
		}
		if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.Source != "trusted-node-snapshot" || result.Inserted != (i == 0) ||
			result.Record.Sequence != 1 || !reflect.DeepEqual(result.Record.Verified, f.Expected) ||
			result.Record.Verified.Verification != "supplied-context" {
			t.Fatalf("incorrect online receipt: %+v", result)
		}
		if i == 0 {
			first = result.Record
		} else if !reflect.DeepEqual(first, result.Record) {
			t.Fatal("online replay replaced original evidence")
		}
		if node.Snapshots() != int64(2*(i+1)) || stderr.Len() != 0 {
			t.Fatal("admission must resolve and recheck exactly once, including replay")
		}
	}
	var persisted registry.Record
	if err := json.Unmarshal([]byte(invoke(t, "get", "--db", path, "--id", first.Verified.ProspectID)), &persisted); err != nil ||
		!reflect.DeepEqual(persisted, first) {
		t.Fatalf("offline read changed the stored online evidence: %v", err)
	}
}

func TestRecordNodeReorgDoesNotWriteOrAcknowledgeReplay(t *testing.T) {
	f := fixtures(t)[0]
	path := initializeFixtureRegistry(t, f)
	_, proofPath := fixtureFiles(t, f)
	node := prospectnodetest.New(t, f.Context)
	node.ReorgAfter(1)
	var stdout, stderr bytes.Buffer
	err := run(nodeArguments(path, proofPath, node), &stdout, &stderr)
	if err == nil || stdout.Len() != 0 || invoke(t, "list", "--db", path) != "[]\n" {
		t.Fatal("failed node guard wrote or acknowledged a receipt")
	}
	if wire.TestPoW {
		return
	}
	if node.Snapshots() != 2 {
		t.Fatal("test did not reach the admission guard")
	}

	// An existing immutable receipt must still pass this call's node guard
	// before the online command may acknowledge it as a duplicate.
	stable := prospectnodetest.New(t, f.Context)
	invoke(t, nodeArguments(path, proofPath, stable)...)
	before := invoke(t, "export", "--db", path)
	stable.ReorgAfter(stable.Snapshots() + 1)
	stdout.Reset()
	err = run(nodeArguments(path, proofPath, stable), &stdout, &stderr)
	if err == nil || stdout.Len() != 0 || invoke(t, "export", "--db", path) != before {
		t.Fatal("failed replay guard acknowledged or changed the first evidence")
	}
}

func TestRecordNodeOutputFailureAllowsReplay(t *testing.T) {
	f := fixtures(t)[0]
	path := initializeFixtureRegistry(t, f)
	_, proofPath := fixtureFiles(t, f)
	node := prospectnodetest.New(t, f.Context)
	args := nodeArguments(path, proofPath, node)
	want := errors.New("output unavailable")
	err := run(args, failedOutput{want}, io.Discard)
	if wire.TestPoW {
		if err == nil || errors.Is(err, want) {
			t.Fatal("cheap profile must refuse before any acknowledgment")
		}
		return
	}
	if !errors.Is(err, want) {
		t.Fatalf("output error = %v, want %v", err, want)
	}
	var result recordResult
	if err := json.Unmarshal([]byte(invoke(t, args...)), &result); err != nil {
		t.Fatal(err)
	}
	if result.Inserted || result.Record.Sequence != 1 || !reflect.DeepEqual(result.Record.Verified, f.Expected) {
		t.Fatal("output failure lost or duplicated the committed record")
	}
}

func TestRecordNodeInputErrors(t *testing.T) {
	f := fixtures(t)[0]
	contextPath, proofPath := fixtureFiles(t, f)
	node := prospectnodetest.New(t, f.Context)
	path := filepath.Join(t.TempDir(), "not-created")
	for _, args := range [][]string{
		{"record-node"},
		{"record-node", "--db", path, "--proof", proofPath},
		{"record-node", "--db", path, "--policy", node.PolicyPath, "--rpc-config", node.ConfigPath},
		append(nodeArguments(path, proofPath, node), "--context", contextPath),
		append(nodeArguments(path, proofPath, node), "extra"),
	} {
		var stdout, stderr bytes.Buffer
		if err := run(args, &stdout, &stderr); err == nil || stdout.Len() != 0 {
			t.Fatalf("invalid online arguments accepted: %v", args)
		}
	}
	if node.Requests() != 0 {
		t.Fatal("invalid arguments contacted node")
	}
	for _, data := range []string{
		"null", `{"` + prospectnodetest.Password + `":true}`,
		strings.Repeat(" ", prospectnode.MaxRPCConfigSize+1),
	} {
		if err := os.WriteFile(node.ConfigPath, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		err := run(nodeArguments(path, proofPath, node), &stdout, &stderr)
		if err == nil || stdout.Len() != 0 || node.Requests() != 0 {
			t.Fatal("malformed configuration contacted node or produced output")
		}
		if strings.Contains(err.Error()+stderr.String(), prospectnodetest.Password) {
			t.Fatal("configuration error exposed a secret")
		}
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("invalid online command created a registry")
	}
	var stdout, stderr bytes.Buffer
	if err := run([]string{"record-node", "-h"}, &stdout, &stderr); !errors.Is(err, flag.ErrHelp) ||
		!strings.Contains(stderr.String(), "usage: slugregistry record-node") {
		t.Fatalf("online help failed: %v", err)
	}
}
