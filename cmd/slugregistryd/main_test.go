// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

func daemonInputs(t *testing.T, create bool) ([]string, string, string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "prospectjob", "testdata", "jobs_v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Job prospectjob.Job `json:"job"`
	}
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatal(err)
	}
	job := fixtures[0].Job
	dir := t.TempDir()
	db := filepath.Join(dir, "registry")
	if create && !wire.TestPoW {
		store, err := registry.CreateBirth(db, job.Context.Network, job.Context.Genesis)
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Close(); err != nil {
			t.Fatal(err)
		}
	}
	policy, rpc, service := filepath.Join(dir, "policy.json"), filepath.Join(dir, "rpc.json"), filepath.Join(dir, "service.json")
	token := bytes.Repeat([]byte{0x42}, 32)
	digest := sha256.Sum256(token)
	for path, value := range map[string]any{
		policy:  map[string]any{"version": 1, "network": job.Context.Network, "genesis": job.Context.Genesis, "dna_version": 1, "prospect_target": job.Context.ProspectTarget},
		rpc:     map[string]any{"version": 1, "url": "http://127.0.0.1:1", "username": "fixture", "password": "fixture-rpc-secret", "ca_certificate": ""},
		service: map[string]any{"version": 1, "listen": "127.0.0.1:0", "clients": []map[string]any{{"public_key": job.PublicKey, "token_sha256": hex.EncodeToString(digest[:])}}, "max_jobs": 100, "max_records": 100},
	} {
		data, _ := json.Marshal(value)
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	return []string{"--db", db, "--policy", policy, "--rpc-config", rpc, "--service-config", service}, db, service
}

type cancelWriter struct {
	bytes.Buffer
	cancel  context.CancelFunc
	failure error
}

func (writer *cancelWriter) Write(data []byte) (int, error) {
	writer.cancel()
	if writer.failure != nil {
		return 0, writer.failure
	}
	return writer.Buffer.Write(data)
}

func TestDaemonReadyShutdownAndOutputFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "ready", true: "output failure"}[fail], func(t *testing.T) {
			args, db, _ := daemonInputs(t, true)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			writer := &cancelWriter{cancel: cancel}
			want := errors.New("output unavailable")
			if fail {
				writer.failure = want
			}
			var stderr bytes.Buffer
			err := run(ctx, args, writer, &stderr)
			if wire.TestPoW {
				if err == nil || writer.Len() != 0 {
					t.Fatal("cheap profile started authority")
				}
				return
			}
			if fail {
				if !errors.Is(err, want) {
					t.Fatalf("output error %v", err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				var ready struct {
					Version uint32 `json:"version"`
					Listen  string `json:"listen"`
				}
				if err := json.Unmarshal(writer.Bytes(), &ready); err != nil || ready.Version != 1 || !strings.HasPrefix(ready.Listen, "http://127.0.0.1:") {
					t.Fatalf("invalid ready output: %q", writer.String())
				}
			}
			if stderr.Len() != 0 || strings.Contains(writer.String(), "fixture-rpc-secret") {
				t.Fatal("unexpected diagnostics or secret")
			}
			store, err := registry.Open(db, false)
			if err != nil {
				t.Fatalf("daemon retained DB lock: %v", err)
			}
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestDaemonConfigurationAndNoCreation(t *testing.T) {
	args, db, service := daemonInputs(t, false)
	var stdout, stderr bytes.Buffer
	if err := run(context.Background(), args, &stdout, &stderr); err == nil || stdout.Len() != 0 {
		t.Fatal("missing registry was accepted")
	}
	if _, err := os.Stat(db); !os.IsNotExist(err) {
		t.Fatal("daemon created a registry")
	}
	if err := os.WriteFile(service, []byte(`{"fixture-rpc-secret":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	err := run(context.Background(), args, &stdout, &stderr)
	if err == nil || strings.Contains(err.Error()+stderr.String(), "fixture-rpc-secret") || stdout.Len() != 0 {
		t.Fatal("configuration error exposed data")
	}
	for _, args := range [][]string{nil, {"--db", db}, {"--private-key", "unsupported"}, {"--listen", "0.0.0.0:80"}} {
		if err := run(context.Background(), args, io.Discard, io.Discard); err == nil {
			t.Fatal("invalid daemon arguments accepted")
		}
	}
	if err := run(context.Background(), []string{"-h"}, &stdout, &stderr); !errors.Is(err, flag.ErrHelp) || !strings.Contains(stderr.String(), "usage: slugregistryd") {
		t.Fatal("missing daemon help")
	}
}

func TestDaemonRejectsNonregularInput(t *testing.T) {
	if _, err := readBoundedFile(os.DevNull, 1024); err == nil {
		t.Fatal("accepted a nonregular device as a configuration file")
	}
	if _, err := readBoundedFile(t.TempDir(), 1024); err == nil {
		t.Fatal("accepted a directory as a configuration file")
	}
}
