//go:build slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterownership

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestOwnershipCheapProfileRefusesBeforeFilesystem(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent")
	if owner, e := Create(path, Pin{}, StoreIdentity{}); !errors.Is(e, ErrProfile) || owner != nil {
		t.Fatal("create did not refuse profile first", e)
	}
	if _, e := os.Lstat(path); !os.IsNotExist(e) {
		t.Fatal("profile refusal touched path", e)
	}
	if owner, e := Open(path, Pin{}, StoreIdentity{}); !errors.Is(e, ErrProfile) || owner != nil {
		t.Fatal("open did not refuse profile first", e)
	}
}
