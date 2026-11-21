// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDailyWalletHelperFlagsNoImplicitHeadlessSelection(t *testing.T) {
	root := filepath.Join(t.TempDir(), "absent")
	args := append([]string{"--daily-recoverable-issue", "--daily-wallet-helper"}, remoteLaunchTestArgs(root)[1:]...)
	if e := validateRemoteLaunchArgs(args); e != nil {
		t.Fatal(e)
	}
	for _, extra := range []string{"--daily", "--daily=false", "--candidate-remote-pool=false", "--daily-wallet-helper=false", "--daily-wallet-helper", "--node-binary=/other"} {
		if e := validateRemoteLaunchArgs(append(append([]string{}, args...), extra)); e == nil {
			t.Fatal("conflicting helper route", extra)
		}
	}
	if validateRemoteLaunchArgs(args[1:]) == nil {
		t.Fatal("interactive flag without selected mode")
	}
	// Recoverable headless remains separately valid and has not acquired a
	// hidden interactive default; selecting the new flag is explicit.
	if e := validateRemoteLaunchArgs(append([]string{args[0]}, args[2:]...)); e != nil {
		t.Fatal("existing headless grammar changed", e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out, diag bytes.Buffer
	e := run(ctx, args, io.NopCloser(strings.NewReader("")), remoteLaunchTestWriteCloser{&out}, &diag)
	if e == nil || out.Len() != 0 || diag.Len() != 0 {
		t.Fatal("canceled interactive owner disclosed or started", e)
	}
	if _, e := os.Lstat(root); !os.IsNotExist(e) {
		t.Fatal("canceled selection touched custody")
	}
}
func TestDailyWalletHelperAbsentOriginalOwnerRefuses(t *testing.T) {
	o := &dailyWalletLaunchOwner{}
	if _, e := o.DailyWalletSnapshot(context.Background()); e == nil {
		t.Fatal("missing original controller")
	}
	if _, e := o.BeginDailyPayoutRead(context.Background(), strings.Repeat("a", 64), 1, 0, 0); e == nil {
		t.Fatal("public view selected replacement owner")
	}
}
