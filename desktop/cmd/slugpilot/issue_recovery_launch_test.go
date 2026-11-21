// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"context"
	"io"
	"testing"
)

func TestRecoverableIssueHeadlessSoleSelection(t *testing.T) {
	args := []string{"--daily-recoverable-issue", "--data-dir=/selected/owned", "--remote-attachment-root=/selected/attachment", "--remote-config=/selected/config.json", "--remote-node-rpc=/selected/rpc.json", "--remote-pool-credential=/selected/pool.json", "--remote-admission-credential=/selected/admission.json", "--candidate-name-version=1"}
	if !remoteLaunchRequested(args) || validateRemoteLaunchArgs(args) != nil {
		t.Fatal("explicit sole mode rejected")
	}
	for _, flag := range []string{"--daily", "--daily=false", "--candidate-remote-pool", "--candidate-remote-pool=false", "--candidate-pool=false", "--daily-recoverable-issue=false"} {
		bad := append(append([]string{}, args...), flag)
		if validateRemoteLaunchArgs(bad) == nil {
			t.Fatal("conflicting mode accepted", flag)
		}
	}
	// Closed argument validation precedes any selected file/controller creation.
	for _, flag := range []string{"--daily=false", "--candidate-remote-pool=false"} {
		bad := append(append([]string{}, args...), flag)
		if e := run(context.Background(), bad, nil, io.Discard, io.Discard); e != errRemoteLaunchInvalid {
			t.Fatal("conflict fell into old route", e)
		}
	}
}
