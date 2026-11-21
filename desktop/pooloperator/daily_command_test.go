// Copyright (c) 2026 The SLUG developers. ISC license.
package pooloperator

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func dailyCommandArguments(root string) []string {
	out := []string{"status"}
	for _, name := range []string{"setup-file", "root", "wallet-root", "wallet-public-passphrase-file", "wallet-unlock-passphrase-file", "admission-seed-file", "publication-seed-file", "cutoff-seed-file", "rpc-config-file", "tls-certificate-file", "tls-key-file"} {
		out = append(out, "--"+name, filepath.Join(root, name))
	}
	return out
}
func TestDailyOperatorCommandExplicitSelection(t *testing.T) {
	args := dailyCommandArguments(filepath.Join(t.TempDir(), "absent"))
	old, e := parseCommand(args)
	if e != nil || old.daily {
		t.Fatal("legacy default changed", e)
	}
	for _, flag := range []string{"--daily", "--daily=true"} {
		got, e := parseCommand(append(append([]string{}, args...), flag))
		if e != nil || !got.daily {
			t.Fatal("daily selection", e)
		}
		got.daily = false
		if !reflect.DeepEqual(got, old) {
			t.Fatal("daily selection changed inputs")
		}
	}
	for _, extra := range [][]string{{"--daily=false"}, {"--daily", "--daily"}, {"--daily=TRUE"}, {"--daily", "true"}} {
		if _, e := parseCommand(append(append([]string{}, args...), extra...)); !errors.Is(e, ErrCommandArguments) {
			t.Fatal("ambiguous daily selector", e)
		}
	}
	p := []string{"verify", "--policy-file", "/fixture/policy", "--root", "/fixture/root", "--tls-certificate-file", "/fixture/cert", "--wallet-public-passphrase-file", "/fixture/pub", "--wallet-unlock-passphrase-file", "/fixture/unlock"}
	if v, e := parseProvisionCommand(append(append([]string{}, p...), "--daily")); e != nil || !v.daily {
		t.Fatal("daily provision selector", e)
	}
	for _, extra := range [][]string{{"--daily=false"}, {"--daily", "--daily"}, {"--daily=TRUE"}, {"--daily", "true"}} {
		if _, e := parseProvisionCommand(append(append([]string{}, p...), extra...)); !errors.Is(e, ErrCommandArguments) {
			t.Fatal("ambiguous provision selector", e)
		}
	}
}
func TestDailyOperatorCanceledCommandBeforeEffects(t *testing.T) {
	root := filepath.Join(t.TempDir(), "absent")
	args := append(dailyCommandArguments(root), "--daily")
	out, e := os.CreateTemp(t.TempDir(), "out")
	if e != nil {
		t.Fatal(e)
	}
	defer out.Close()
	diag, e := os.CreateTemp(t.TempDir(), "diag")
	if e != nil {
		t.Fatal(e)
	}
	defer diag.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e = RunCommand(ctx, args, out, diag); e == nil {
		t.Fatal("canceled command succeeded")
	}
	if _, e = os.Lstat(root); !os.IsNotExist(e) {
		t.Fatal("canceled command created root")
	}
}
