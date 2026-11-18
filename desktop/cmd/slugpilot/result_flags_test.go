// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"context"
	"io"
	"strings"
	"testing"
)

func TestResultFlagRequiresCompleteRoute(t *testing.T) {
	all := []string{"--candidate-manifests", "--candidate-stable", "--candidate-entries", "--candidate-events", "--candidate-seeds", "--candidate-cards", "--candidate-bouts"}
	for omitted := range all {
		args := []string{"--candidate-results"}
		for i, v := range all {
			if i != omitted {
				args = append(args, v)
			}
		}
		args = append(args, "--data-dir", t.TempDir(), "--node-binary", "/not-a-node")
		e := run(context.Background(), args, strings.NewReader(""), io.Discard, io.Discard)
		if e == nil || !strings.Contains(e.Error(), "requires") {
			t.Fatal("unsafe route", omitted, e)
		}
	}
	args := append([]string{"--candidate-results", "--account-fixtures"}, all...)
	args = append(args, "--data-dir", t.TempDir(), "--node-binary", "/not-a-node")
	if e := run(context.Background(), args, strings.NewReader(""), io.Discard, io.Discard); e == nil {
		t.Fatal("mixed legacy mode accepted")
	}
}
