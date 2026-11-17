// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"context"
	"io"
	"strings"
	"testing"
)

func TestSeedFlagRequiresCompleteRoute(t *testing.T) {
	for _, args := range [][]string{{"--candidate-seeds"}, {"--candidate-seeds", "--candidate-manifests"}, {"--candidate-seeds", "--candidate-manifests", "--candidate-stable"}, {"--candidate-seeds", "--candidate-manifests", "--candidate-stable", "--candidate-entries"}, {"--candidate-seeds", "--candidate-manifests", "--candidate-stable", "--candidate-entries", "--candidate-events", "--account-fixtures"}} {
		args = append(args, "--data-dir", t.TempDir(), "--node-binary", "/not-a-node")
		err := run(context.Background(), args, strings.NewReader(""), io.Discard, io.Discard)
		if err == nil || !strings.Contains(err.Error(), "requires") {
			t.Fatalf("unsafe flag composition: %v", err)
		}
	}
}
