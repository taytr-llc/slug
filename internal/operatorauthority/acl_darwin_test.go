//go:build darwin && cgo

// Copyright (c) 2026 The SLUG developers. ISC license.
package operatorauthority

import (
	"context"
	"os/exec"
	"testing"
	"time"
)

func addSyntheticACL(t *testing.T, path string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	// Only this test's temporary public-synthetic fixture receives an ACL.
	cmd := exec.CommandContext(ctx, "/bin/chmod", "+a", "everyone allow read", path)
	if e := cmd.Run(); e != nil {
		t.Fatal("could not set bounded synthetic ACL", e)
	}
}
