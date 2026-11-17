// Copyright (c) 2026 The SLUG developers. ISC license.
package gymentry

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/wire/v2"
	"os"
	"path/filepath"
	"testing"
)

func TestEntryCheapProfileRefusesBeforeFilesystem(t *testing.T) {
	if !wire.TestPoW {
		t.Skip("cheap-build refusal")
	}
	path := filepath.Join(t.TempDir(), "must-not-exist")
	if s, e := Create(context.Background(), path, Pin{}, nil); s != nil || !errors.Is(e, ErrProfile) {
		t.Fatal(s, e)
	}
	if s, e := Open(context.Background(), path, Pin{}, nil); s != nil || !errors.Is(e, ErrProfile) {
		t.Fatal(s, e)
	}
	if _, e := os.Lstat(path); !os.IsNotExist(e) {
		t.Fatal("profile mutated path", e)
	}
}
