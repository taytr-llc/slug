// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCareerFlagRequiresCompleteRouteBeforeEffects(t *testing.T) {
	all := []string{"--candidate-manifests", "--candidate-stable", "--candidate-entries", "--candidate-events", "--candidate-seeds", "--candidate-cards", "--candidate-bouts", "--candidate-results"}
	for omitted := range all {
		root := filepath.Join(t.TempDir(), "absent")
		args := []string{"--career"}
		for i, v := range all {
			if i != omitted {
				args = append(args, v)
			}
		}
		args = append(args, "--data-dir", root, "--node-binary", "/not-a-node")
		e := run(context.Background(), args, strings.NewReader(""), io.Discard, io.Discard)
		if e == nil || !strings.Contains(e.Error(), "requires") {
			t.Fatal("unsafe career route", omitted, e)
		}
		if _, e = os.Stat(root); !os.IsNotExist(e) {
			t.Fatal("invalid flags touched data", e)
		}
	}
	args := append([]string{"--career", "--account-fixtures"}, all...)
	if e := run(t.Context(), args, strings.NewReader(""), io.Discard, io.Discard); e == nil {
		t.Fatal("mixed legacy account route")
	}
}
