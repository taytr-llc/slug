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
)

func TestNativeDailyWalletExplicitSelectionBeforeEffects(t *testing.T) {
	root := filepath.Join(t.TempDir(), "absent")
	args := append([]string{"--daily-recoverable-issue", "--daily-wallet-helper"}, nativeRemoteTestArgs(root)[1:]...)
	if _, e := parseNativeDailyWalletArgs(args); e != nil {
		t.Fatal(e)
	}
	for _, extra := range []string{"--daily", "--daily=false", "--candidate-remote-pool=false", "--daily-wallet-helper=false", "--daily-recoverable-issue=false", "--remote-config=/other", "--node-binary=/other", "--unknown=PUBLIC-SYNTHETIC-SECRET"} {
		_, e := parseNativeDailyWalletArgs(append(append([]string{}, args...), extra))
		if e == nil || strings.Contains(e.Error(), "PUBLIC-SYNTHETIC-SECRET") {
			t.Fatal("mixed selector or leaked input", extra, e)
		}
	}
	for _, missing := range [][]string{args[1:], append(append([]string{}, args[:1]...), args[2:]...)} {
		if _, e := parseNativeDailyWalletArgs(missing); e == nil {
			t.Fatal("missing explicit pair")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if o, e := selectNativeDailyWallet(ctx, filepath.Join(root, "config")); e == nil || o != nil {
		t.Fatal("canceled selection")
	}
	if _, e := os.Lstat(root); !os.IsNotExist(e) {
		t.Fatal("pure refusal touched custody")
	}
}
func TestNativeDailyWalletOnlyEightFixedMethodsAndExactStrings(t *testing.T) {
	typ := reflect.TypeOf((*DailyRemoteGym)(nil))
	var names []string
	for i := 0; i < typ.NumMethod(); i++ {
		names = append(names, typ.Method(i).Name)
	}
	if !reflect.DeepEqual(names, []string{"BeginDailyPayoutRead", "CancelDailyPayoutRead", "DailyPayoutRead", "LoginFixture", "Logout", "Snapshot", "Start", "Stop"}) {
		t.Fatal("unexpected web authority", names)
	}
	for _, s := range []string{"9007199254740993", "18446744073709551615", "1"} {
		n, e := dailyWalletUint(s, false)
		if e != nil || n == 0 {
			t.Fatal("lossless sequence refused", s, e)
		}
	}
	for _, s := range []string{"", "0", "01", "+1", "1e1", "1.0", "18446744073709551616", "-1"} {
		if _, e := dailyWalletUint(s, false); e == nil {
			t.Fatal("noncanonical sequence accepted", s)
		}
	}
	g := &DailyRemoteGym{}
	v := strings.Repeat("a", 64)
	if _, e := g.BeginDailyPayoutRead(v, "1", "0", "0"); e == nil {
		t.Fatal("public values manufactured original owner")
	}
	if _, e := g.CancelDailyPayoutRead(v, "1"); e == nil {
		t.Fatal("public cancellation manufactured original owner")
	}
	e := &nativeDailyWalletError{cause: errors.New("PUBLIC-SYNTHETIC-SECRET")}
	if strings.Contains(e.Error(), "PUBLIC-SYNTHETIC-SECRET") {
		t.Fatal("unbounded error disclosure")
	}
}
