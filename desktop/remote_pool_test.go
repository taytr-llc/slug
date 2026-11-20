// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/desktop/internal/bridge"
)

func nativeRemoteTestArgs(root string) []string {
	return []string{"--candidate-remote-pool", "--data-dir", filepath.Join(root, "original"), "--remote-attachment-root", filepath.Join(root, "attachment"), "--remote-config", filepath.Join(root, "config"), "--remote-node-rpc", filepath.Join(root, "rpc"), "--remote-pool-credential", filepath.Join(root, "pool"), "--remote-admission-credential", filepath.Join(root, "admission")}
}
func TestNativeRemoteExplicitExclusiveSelection(t *testing.T) {
	root := filepath.Join(t.TempDir(), "absent")
	valid := nativeRemoteTestArgs(root)
	for _, name := range []string{"candidate-owned-gym", "candidate-pool", "candidate-hosted-pool", "candidate-hosted-recovery", "candidate-manifests", "candidate-stable", "candidate-entries", "candidate-events", "candidate-seeds", "candidate-cards", "candidate-bouts", "candidate-results", "candidate-careers", "account-fixtures"} {
		for _, value := range []string{"true", "false"} {
			if _, e := parseNativeRemoteArgs(append(append([]string{}, valid...), "--"+name+"="+value)); !errors.Is(e, errNativeRemoteInvalid) {
				t.Fatal("legacy selection accepted", name, value, e)
			}
		}
	}
	for _, extra := range [][]string{{"--candidate-remote-pool=false"}, {"--node-binary="}, {"--node-binary=/node"}, {"--remote-config=/other"}, {"--candidate-name-version=3"}, {"--unknown=PUBLIC-SYNTHETIC-SECRET"}, {"positional"}} {
		_, e := parseNativeRemoteArgs(append(append([]string{}, valid...), extra...))
		if e == nil || strings.Contains(e.Error(), "PUBLIC-SYNTHETIC-SECRET") {
			t.Fatal("invalid selection leaked or accepted", e)
		}
	}
	for i := 1; i < len(valid); i += 2 {
		args := append(append([]string{}, valid[:i]...), valid[i+2:]...)
		if _, e := parseNativeRemoteArgs(args); e == nil {
			t.Fatal("missing path accepted", i)
		}
	}
	for _, mode := range []string{"--candidate-remote-pool", "--candidate-remote-pool=true", "-candidate-remote-pool=true"} {
		args := append([]string{}, valid...)
		args[0] = mode
		if _, e := parseNativeRemoteArgs(args); e != nil {
			t.Fatal(e)
		}
	}
	if _, e := parseNativeRemoteArgs(append(append([]string{}, valid...), "--candidate-name-version=2")); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Lstat(root); !os.IsNotExist(e) {
		t.Fatal("pure selection touched storage")
	}
	if !nativeRemoteRequested([]string{"--candidate-remote-pool=false"}) || !nativeRemoteRequested([]string{"--remote-config=/config"}) || nativeRemoteRequested([]string{"--data-dir", "remote-pool"}) {
		t.Fatal("mode routing")
	}
}
func TestNativeRemoteOnlyFiveMethods(t *testing.T) {
	typ := reflect.TypeOf((*RemoteGym)(nil))
	var names []string
	for i := 0; i < typ.NumMethod(); i++ {
		names = append(names, typ.Method(i).Name)
	}
	if !reflect.DeepEqual(names, []string{"LoginFixture", "Logout", "Snapshot", "Start", "Stop"}) {
		t.Fatal("unexpected webview authority", names)
	}
	g := &RemoteGym{}
	if _, e := g.Start(strings.Repeat("a", 64)); e == nil {
		t.Fatal("public view manufactured authority")
	}
	if e := g.menuAction("start")(); e == nil {
		t.Fatal("empty menu manufactured authority")
	}
}

func TestNativeRemoteBundleRequiresExplicitInputs(t *testing.T) {
	previousArgs, previousBuild := os.Args, remoteBuild
	t.Cleanup(func() { os.Args, remoteBuild = previousArgs, previousBuild })
	remoteBuild = "1"
	os.Args = []string{"public-test-native"}
	t.Setenv("SLUG_PILOT_DATA_DIR", filepath.Join(t.TempDir(), "never-created"))
	if e := run(); !errors.Is(e, errNativeRemoteInvalid) {
		t.Fatal("remote package inherited a legacy default", e)
	}
	if _, e := os.Lstat(os.Getenv("SLUG_PILOT_DATA_DIR")); !os.IsNotExist(e) {
		t.Fatal("remote package created a default account")
	}
}
func TestNativeRemoteQuitVetoAndStaticUnknown(t *testing.T) {
	for _, failure := range []error{nil, errNativeRemoteCleanup} {
		q := &nativeRemoteQuitState{}
		allow, start := q.begin()
		if allow || !start {
			t.Fatal("quit skipped join")
		}
		if allow, start = q.begin(); allow || start {
			t.Fatal("duplicate join")
		}
		if q.finish(failure) != (failure == nil) {
			t.Fatal("finish state")
		}
		allow, start = q.begin()
		if allow != (failure == nil) || start {
			t.Fatal("failed join allowed quit or retry")
		}
	}
	e := &nativeRemoteError{cause: errors.Join(bridge.ErrRemotePoolOutcomeUnknown, errNativeRemoteCleanup, errors.New("PUBLIC-SYNTHETIC-SECRET")), retained: &nativeRemoteOwner{}}
	if !strings.Contains(e.Error(), "outcome is unknown") || strings.Contains(e.Error(), "PUBLIC-SYNTHETIC-SECRET") {
		t.Fatal("uncertainty priority or leaked cause")
	}
}
func TestNativeRemoteCancellationBeforeEffects(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	root := filepath.Join(t.TempDir(), "absent")
	if o, e := selectNativeRemote(ctx, filepath.Join(root, "config")); e == nil || o != nil {
		t.Fatal("canceled selection succeeded")
	}
	if _, e := os.Lstat(root); !os.IsNotExist(e) {
		t.Fatal("canceled selection changed storage")
	}
}
