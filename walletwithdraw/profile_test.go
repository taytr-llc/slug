//go:build slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package walletwithdraw

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWithdrawalCheapProfileRefusesBeforeFiles(t *testing.T) {
	p := filepath.Join(t.TempDir(), "must-not-exist")
	for _, open := range []func(context.Context, string, Identity) (*Journal, error){Create, Open} {
		j, e := open(context.Background(), p, Identity{})
		if j != nil || !errors.Is(e, ErrProfile) {
			t.Fatalf("profile %v", e)
		}
	}
	if _, e := os.Lstat(p); !os.IsNotExist(e) {
		t.Fatal("touched path")
	}
}
