//go:build slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManifestCandidateRefusesCheapBeforeStorage(t *testing.T) {
	root := filepath.Join(t.TempDir(), "not-created")
	c, e := NewManifestAccount(Config{DataDir: root, NodeBinary: "/not-a-node"}, ManifestAccountConfig{})
	if e == nil || c != nil {
		t.Fatal("cheap desktop constructor accepted")
	}
	if _, e = os.Lstat(root); !os.IsNotExist(e) {
		t.Fatal("cheap constructor touched storage", e)
	}
}
