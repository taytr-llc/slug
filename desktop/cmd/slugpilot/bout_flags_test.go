// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"context"
	"io"
	"strings"
	"testing"
)

func TestBoutFlagRequiresCompleteRoute(t *testing.T) {
	for _, args := range [][]string{{"--candidate-bouts", "--candidate-manifests", "--candidate-stable", "--candidate-entries", "--candidate-events", "--candidate-seeds"}, {"--candidate-bouts"}, {"--candidate-bouts", "--candidate-manifests"}, {"--candidate-bouts", "--candidate-manifests", "--candidate-stable"}, {"--candidate-bouts", "--candidate-manifests", "--candidate-stable", "--candidate-entries"}, {"--candidate-bouts", "--candidate-manifests", "--candidate-stable", "--candidate-entries", "--candidate-events", "--candidate-seeds", "--account-fixtures"}} {
		args = append(args, "--data-dir", t.TempDir(), "--node-binary", "/not-a-node")
		err := run(context.Background(), args, strings.NewReader(""), io.Discard, io.Discard)
		if err == nil || !strings.Contains(err.Error(), "requires") {
			t.Fatalf("unsafe flag composition: %v", err)
		}
	}
}
