//go:build slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPoolRuntimeCheapProfileRefusesBeforeStorage(t *testing.T) {
	root := filepath.Join(t.TempDir(), "not-created")
	r, e := NewPoolRuntime(context.Background(), Config{DataDir: root, NodeBinary: "/not-a-node"}, nil)
	if r != nil || !errors.Is(e, ErrPoolRuntime) {
		t.Fatal("cheap pool runtime accepted", e)
	}
	if _, e = os.Lstat(root); !os.IsNotExist(e) {
		t.Fatal("cheap runtime touched storage", e)
	}
}
