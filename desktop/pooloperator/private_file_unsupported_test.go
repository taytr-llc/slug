//go:build !linux && (!darwin || !cgo)

// Copyright (c) 2026 The SLUG developers. ISC license.
package pooloperator

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestOperatorPrivateFilesUnsupportedRefusal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-created")
	if selected, e := selectPrivateInput(context.Background(), path, 32); selected != nil || !errors.Is(e, ErrUnsupported) {
		t.Fatal("unsupported private custody admitted", e)
	}
	v := opSetupTestReference(t).Positives[0]
	if files, e := openOperatorFiles(path, v.Setup, true); files != nil || !errors.Is(e, ErrUnsupported) {
		t.Fatal("unsupported filesystem custody admitted", e)
	}
	if _, e := os.Lstat(path); !os.IsNotExist(e) {
		t.Fatal("unsupported path created")
	}
}
