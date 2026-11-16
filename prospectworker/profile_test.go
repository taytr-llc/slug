//go:build slugtestpow

// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package prospectworker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkerRefusesCheapProfileBeforeState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "outbox")
	if _, err := CreateOutbox(path, Config{}); err == nil || !strings.Contains(err.Error(), "slugtestpow") {
		t.Fatal("cheap outbox allowed")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("cheap profile created directory")
	}
	if _, err := New(Config{}, nil, nil, nil, nil); err == nil || !strings.Contains(err.Error(), "slugtestpow") {
		t.Fatal("cheap worker allowed")
	}
}
