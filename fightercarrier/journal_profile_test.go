//go:build slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package fightercarrier

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCarrierJournalCheapProfileRefusesBeforeFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "must-not-exist")
	for _, open := range []func(context.Context, string, Identity) (*Journal, error){Create, Open, OpenAudit} {
		j, err := open(context.Background(), path, Identity{})
		if j != nil || !errors.Is(err, ErrProfile) {
			t.Fatalf("profile refusal: %v", err)
		}
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatal("refused profile touched path")
	}
}
