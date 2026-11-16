//go:build slugtestpow

// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package prospectservice

import (
	"strings"
	"testing"

	"github.com/taytr-llc/slug/prospectnode"
)

func TestServerRefusesCheapProfileBeforeInitialization(t *testing.T) {
	server, err := NewServer(ServerConfig{}, prospectnode.Policy{}, nil, nil)
	if err == nil || server != nil || !strings.Contains(err.Error(), "slugtestpow") {
		t.Fatal("cheap authority did not fail before initialization")
	}
}
