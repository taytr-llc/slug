package main

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestNativeDailyWallet2ExplicitSelectionAndFixedMethods(t *testing.T) {
	root := filepath.Join(t.TempDir(), "absent")
	base := append([]string{"--daily-recoverable-issue", "--daily-wallet-helper"}, nativeRemoteTestArgs(root)[1:]...)
	args := append(append([]string{}, base...), "--daily-wallet-helper-version=2")
	if _, e := parseNativeDailyWalletArgs2(args); e != nil {
		t.Fatal(e)
	}
	if _, e := parseNativeDailyWalletArgs(args); e == nil {
		t.Fatal("old selector adopted new version")
	}
	for _, v := range []string{"1", "02", "0", "3", "false", ""} {
		if _, e := parseNativeDailyWalletArgs2(append(append([]string{}, base...), "--daily-wallet-helper-version="+v)); e == nil {
			t.Fatal("unselected version", v)
		}
	}
	if _, e := parseNativeDailyWalletArgs2(append(args, "--daily-wallet-helper-version=2")); e == nil {
		t.Fatal("duplicate version")
	}
	if _, e := parseNativeDailyWalletArgs2(base); e == nil {
		t.Fatal("implicit version2")
	}
	typ := reflect.TypeOf((*DailyRemoteGym2)(nil))
	var names []string
	for i := 0; i < typ.NumMethod(); i++ {
		names = append(names, typ.Method(i).Name)
	}
	want := []string{"BeginDailyPayoutRead", "BeginWalletActivityRead", "CancelDailyPayoutRead", "CancelWalletActivityRead", "DailyPayoutRead", "LoginFixture", "Logout", "ObserveDailyPayout", "Snapshot", "Start", "Stop", "WalletActivityDetail", "WalletActivityPage", "WalletActivityRead"}
	if !reflect.DeepEqual(names, want) {
		t.Fatal("unexpected web authority", names)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if o, e := selectNativeDailyWallet2(ctx, filepath.Join(root, "config")); e == nil || o != nil {
		t.Fatal("canceled original selection")
	}
	if _, e := os.Lstat(root); !os.IsNotExist(e) {
		t.Fatal("refusal touched wallet")
	}
}
