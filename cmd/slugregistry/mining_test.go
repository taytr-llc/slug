// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package main

import (
	"bytes"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
)

func TestExplicitMiningInitialization(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mining")
	args := []string{"init-mining", "--db", path, "--network", fmt.Sprint(uint32(chaincfg.MainNetParams.Net)), "--genesis", chaincfg.MainNetParams.GenesisHash.String()}
	var out, diagnostics bytes.Buffer
	err := run(args, &out, &diagnostics)
	if wire.TestPoW {
		if err == nil || out.Len() != 0 {
			t.Fatal("cheap profile initialized a mining authority")
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	store, err := registry.Open(path, true)
	if err != nil {
		t.Fatal(err)
	}
	info, err := store.Info()
	if closeErr := store.Close(); err != nil || closeErr != nil || info.Version != registry.Version4 {
		t.Fatalf("explicit mining schema: %+v, %v, %v", info, err, closeErr)
	}
	for _, command := range []string{"init-birth", "init-mining"} {
		args[0] = command
		if err := run(args, &out, &diagnostics); err == nil {
			t.Fatal("reinitialized existing mining registry")
		}
	}
}
