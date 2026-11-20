// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func remoteLaunchTestArgs(root string) []string {
	return []string{"--candidate-remote-pool", "--data-dir", filepath.Join(root, "original"), "--remote-attachment-root", filepath.Join(root, "attachment"), "--remote-config", filepath.Join(root, "config"), "--remote-node-rpc", filepath.Join(root, "rpc"), "--remote-pool-credential", filepath.Join(root, "pool"), "--remote-admission-credential", filepath.Join(root, "admission")}
}
func TestRemoteHelperFlagsExclusiveBeforeEffects(t *testing.T) {
	for _, mode := range []string{"candidate-owned-gym", "candidate-hosted-recovery", "candidate-hosted-pool", "candidate-pool", "account-fixtures", "candidate-manifests", "candidate-stable", "candidate-entries", "candidate-events", "candidate-seeds", "candidate-cards", "candidate-bouts", "candidate-results", "career"} {
		for _, value := range []string{"true", "false"} {
			root := filepath.Join(t.TempDir(), "absent")
			args := append(remoteLaunchTestArgs(root), "--"+mode+"="+value)
			var out, diagnostics bytes.Buffer
			e := run(context.Background(), args, strings.NewReader(""), &out, &diagnostics)
			if !errors.Is(e, errRemoteLaunchInvalid) || out.Len() != 0 || diagnostics.Len() != 0 {
				t.Fatal("conflicting selector not refused privately", mode, value, e)
			}
			if _, e = os.Lstat(root); !os.IsNotExist(e) {
				t.Fatal("invalid selection changed storage")
			}
		}
	}
}
func TestRemoteHelperFlagsClosedGrammar(t *testing.T) {
	root := filepath.Join(t.TempDir(), "absent")
	valid := remoteLaunchTestArgs(root)
	for _, extra := range [][]string{{"--node-binary=/not-a-node"}, {"--node-binary="}, {"--candidate-remote-pool=false"}, {"--data-dir", root}, {"--candidate-name-version=3"}, {"positional"}, {"--unknown=PUBLIC-TEST-SECRET"}, {"--remote-config=relative"}} {
		var out, diagnostics bytes.Buffer
		e := run(context.Background(), append(append([]string{}, valid...), extra...), strings.NewReader(""), &out, &diagnostics)
		if e == nil || out.Len() != 0 || diagnostics.Len() != 0 || strings.Contains(e.Error(), "PUBLIC-TEST-SECRET") {
			t.Fatal("invalid flags leaked or dispatched", e)
		}
	}
	for i := 1; i < len(valid); i += 2 {
		missing := append(append([]string{}, valid[:i]...), valid[i+2:]...)
		if validateRemoteLaunchArgs(missing) == nil {
			t.Fatal("missing explicit path accepted", i)
		}
	}
	for _, first := range []string{"--candidate-remote-pool", "--candidate-remote-pool=true", "-candidate-remote-pool=true"} {
		args := append([]string{}, valid...)
		args[0] = first
		if validateRemoteLaunchArgs(args) != nil {
			t.Fatal("explicit remote selector refused")
		}
	}
	if e := validateRemoteLaunchArgs(append(append([]string{}, valid...), "--candidate-name-version=2")); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Lstat(root); !os.IsNotExist(e) {
		t.Fatal("invalid flags changed storage")
	}
	if remoteLaunchRequested([]string{"--node-binary", "remote-not-a-flag"}) {
		t.Fatal("legacy flag value changed mode")
	}
}
func TestRemoteHelperCheapRefusalBeforeEffects(t *testing.T) {
	root := filepath.Join(t.TempDir(), "absent")
	args := remoteLaunchTestArgs(root)
	var out, diag bytes.Buffer
	if e := run(context.Background(), args, strings.NewReader(""), &out, &diag); !errors.Is(e, errRemoteLaunchInvalid) {
		t.Fatal("unowned streams accepted", e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	e := run(ctx, args, io.NopCloser(strings.NewReader("")), remoteLaunchTestWriteCloser{&out}, &diag)
	if e == nil || out.Len() != 0 || diag.Len() != 0 {
		t.Fatal("canceled constructor disclosed or succeeded", e)
	}
	if _, e := os.Lstat(root); !os.IsNotExist(e) {
		t.Fatal("canceled constructor changed storage")
	}
}

type remoteLaunchTestWriteCloser struct{ io.Writer }

func (remoteLaunchTestWriteCloser) Close() error { return nil }

func TestRemoteHelperRPCOriginalGrammarAndLocalProfile(t *testing.T) {
	base := `{"version":1,"url":"http://127.0.0.1:29332","username":"public-test-user","password":"public-test-password","ca_certificate":""}`
	if c, e := parseRemoteLaunchRPC([]byte(base)); e != nil || c.URL != "http://127.0.0.1:29332" {
		t.Fatal("local explicit RPC refused", e)
	}
	cases := []string{
		strings.Replace(base, `"version":1`, `"version":1,"version":1`, 1),
		strings.Replace(base, `"password":"public-test-password",`, "", 1),
		strings.Replace(base, `"ca_certificate":""`, `"ca_certificate":null`, 1),
		strings.Replace(base, `"ca_certificate":""`, `"ca_certificate":"PUBLIC CA"`, 1),
		base + "{}",
	}
	for _, u := range []string{"http://localhost:29332", "http://[::1]:29332", "https://127.0.0.1:29332", "http://127.0.0.1", "http://127.0.0.1:029332", "http://127.0.0.1:65536", "http://127.0.0.1:29332/", "http://127.0.0.1:29332?", "http://user@127.0.0.1:29332", "http://127.0.0.2:29332"} {
		cases = append(cases, strings.Replace(base, "http://127.0.0.1:29332", u, 1))
	}
	for i, b := range cases {
		if _, e := parseRemoteLaunchRPC([]byte(b)); e == nil || strings.Contains(e.Error(), "public-test-password") {
			t.Fatal("RPC unsafe grammar/profile", i, e)
		}
	}
}
