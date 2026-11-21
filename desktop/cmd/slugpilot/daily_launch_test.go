// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDailyHeadlessFlagsAndCancellationBeforeEffects(t *testing.T) {
	root := filepath.Join(t.TempDir(), "absent")
	base := append(remoteLaunchTestArgs(root), "--daily")
	if e := validateRemoteLaunchArgs(base); e != nil {
		t.Fatal(e)
	}
	for _, extra := range [][]string{{"--daily=false"}, {"--daily=true"}, {"--node-binary=/node"}, {"--candidate-pool=false"}, {"--candidate-owned-gym=false"}} {
		var out, diag bytes.Buffer
		if e := run(context.Background(), append(append([]string{}, base...), extra...), strings.NewReader(""), &out, &diag); !errors.Is(e, errRemoteLaunchInvalid) || out.Len() != 0 || diag.Len() != 0 {
			t.Fatal("unreviewed mode entered", e)
		}
	}
	var out, diag bytes.Buffer
	if e := run(context.Background(), []string{"--daily"}, strings.NewReader(""), &out, &diag); !errors.Is(e, errRemoteLaunchInvalid) {
		t.Fatal("daily without explicit remote paths", e)
	}
	if e := run(context.Background(), base, strings.NewReader(""), &out, &diag); !errors.Is(e, errRemoteLaunchInvalid) {
		t.Fatal("unowned output accepted", e)
	}
	output, e := os.CreateTemp(t.TempDir(), "out")
	if e != nil {
		t.Fatal(e)
	}
	defer output.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e = run(ctx, base, strings.NewReader(""), output, &diag); e == nil {
		t.Fatal("canceled launch accepted")
	}
	if _, e = os.Stat(root); !os.IsNotExist(e) {
		t.Fatal("effects before complete selection")
	}
}
func TestDailyHeadlessOutputCancellationJoins(t *testing.T) {
	reader, writer, e := os.Pipe()
	if e != nil {
		t.Fatal(e)
	}
	defer reader.Close()
	defer writer.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	output := &dailyLaunchOutput{file: writer}
	// A reader deliberately does not drain. Bounded repeated public writes fill
	// the original pipe; cancellation closes that exact descriptor and joins.
	var got error
	for i := 0; i < 65536 && got == nil; i++ {
		got = output.write(ctx, strings.Repeat("p", 400))
	}
	if got == nil {
		t.Fatal("blocked output was not interrupted")
	}
	output.mu.Lock()
	interrupted, closeErr := output.interrupted, output.closeErr
	output.mu.Unlock()
	// Cancellation can win between writes. In that case no blocked writer exists;
	// either the joined interrupt ran or the operation refused before a write.
	if interrupted && closeErr != nil {
		t.Fatal("original output cleanup", closeErr)
	}
	if ctx.Err() == nil {
		t.Fatal("unexpected non-cancellation failure", got)
	}
}
