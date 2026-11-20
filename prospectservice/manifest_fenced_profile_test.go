//go:build slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package prospectservice

import (
	"context"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/prospectnode"
)

type fencedProfileSessions struct{ t *testing.T }

func (s fencedProfileSessions) WithSession(context.Context, string, func(string, func() error) error) error {
	s.t.Fatal("cheap profile consulted account authority")
	return nil
}

func TestFencedManifestAdmissionRefusesCheapProfile(t *testing.T) {
	server, err := NewFencedManifestAdmissionServer(ManifestServerConfig{}, prospectnode.Policy{}, nil, nil, fencedProfileSessions{t})
	if server != nil || err == nil || !strings.Contains(err.Error(), "production PoW") {
		t.Fatal("cheap profile reached fenced initialization", err)
	}
}
