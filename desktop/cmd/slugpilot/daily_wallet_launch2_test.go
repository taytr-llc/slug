package main

import (
	"context"
	"path/filepath"
	"testing"
)

func TestDailyWallet2HelperExplicitVersionSelection(t *testing.T) {
	root := filepath.Join(t.TempDir(), "absent")
	base := append([]string{"--daily-recoverable-issue", "--daily-wallet-helper"}, remoteLaunchTestArgs(root)[1:]...)
	for _, tail := range [][]string{{"--daily-wallet-helper-version=2"}, {"--daily-wallet-helper-version", "2"}} {
		if e := validateRemoteLaunchArgs(append(append([]string{}, base...), tail...)); e != nil {
			t.Fatal(e)
		}
	}
	for _, v := range []string{"1", "02", "0", "3", "false", ""} {
		if e := validateRemoteLaunchArgs(append(append([]string{}, base...), "--daily-wallet-helper-version="+v)); e == nil {
			t.Fatal("unselected version", v)
		}
	}
	args := append(append([]string{}, base...), "--daily-wallet-helper-version=2")
	if validateRemoteLaunchArgs(append(args, "--daily-wallet-helper-version=2")) == nil {
		t.Fatal("duplicate version")
	}
	headless := append([]string{base[0]}, base[2:]...)
	if validateRemoteLaunchArgs(append(headless, "--daily-wallet-helper-version=2")) == nil {
		t.Fatal("implicit interactive route")
	}
	if e := validateRemoteLaunchArgs(base); e != nil {
		t.Fatal("original helper changed", e)
	}
	o := &dailyWalletLaunchOwner2{}
	if _, e := o.BeginWalletActivityRead(context.Background(), "view", 1); e == nil {
		t.Fatal("missing original owner")
	}
	if _, e := o.ObserveDailyPayout(context.Background(), "view", 1, 1, "", 0); e == nil {
		t.Fatal("missing original wallet")
	}
}
