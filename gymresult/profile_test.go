// Copyright (c) 2026 The SLUG developers. ISC license.
package gymresult

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/wire/v2"
	"os"
	"path/filepath"
	"testing"
)

func TestResultCheapProfileRefusesBeforeFilesystemEffects(t *testing.T) {
	if !wire.TestPoW {
		t.Skip("cheap-profile guard")
	}
	path := filepath.Join(t.TempDir(), "uncreated")
	ctx := context.Background()
	for _, fn := range []func() error{func() error { _, e := Create(ctx, path, Pin{}, nil); return e }, func() error { _, e := Open(ctx, path, Pin{}, nil); return e }, func() error { _, e := ResumePrepared(ctx, path, Pin{}, nil); return e }} {
		if e := fn(); !errors.Is(e, ErrProfile) {
			t.Fatal("profile precedence", e)
		}
	}
	if _, e := os.Stat(path); !os.IsNotExist(e) {
		t.Fatal("cheap build touched storage")
	}
}
