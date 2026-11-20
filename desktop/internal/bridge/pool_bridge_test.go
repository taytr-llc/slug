// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/wire/v2"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"testing"
	"time"
)

func poolBridgeSnapshot(t *testing.T) pilot.PoolSnapshot {
	old := manifestBridgeSnapshot(t)
	s := pilot.BlankPoolSnapshot()
	s.Account = old.Account
	s.Account.CanExport = false
	s.View = old.View
	s.GymID = old.GymID
	s.GymPublicKey = old.GymPublicKey
	s.PayoutAddress = "slug1qzmtrqsfuaf6l6kkcsseumq26ukaphfj9s9ptme"
	s.SourceBindingID = strings.Repeat("f", 64)
	s.Balance = pilot.PoolBalanceSnapshot{State: "ready", Value: &poolservice.BalanceResponse{Version: 1, BindingID: s.SourceBindingID, ObservedAtMS: 1795000000000, Balance: poolsettle.Balance{GymID: s.GymID, Confirmed: poolsettle.Fraction{Numerator: "300000001", Denominator: "3"}, Available: poolsettle.Fraction{Numerator: "300000001", Denominator: "3"}, Reason: "fee-budget"}}}
	return s
}
func TestPoolSnapshotOwnScopeAndExactFractions(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal identity")
	}
	for _, s := range []pilot.PoolSnapshot{pilot.BlankPoolSnapshot(), poolBridgeSnapshot(t)} {
		raw, _ := json.Marshal(pilot.PoolResponse{ID: 1, OK: true, Snapshot: &s})
		if _, e := pilot.ParsePoolResponse(raw); e != nil {
			t.Fatal("valid pool shape", e)
		}
		for _, bad := range [][]byte{bytes.Replace(raw, []byte(`"id":1`), []byte(`"id":1,"id":1`), 1), bytes.Replace(raw, []byte(`"view":`), []byte(`"token":"private","view":`), 1), bytes.Replace(raw, []byte(`"assignment_count":0`), []byte(`"accepted_count":0`), 1)} {
			if _, e := pilot.ParsePoolResponse(bad); e == nil {
				t.Fatal("malformed pool response accepted")
			}
		}
	}
	good := poolBridgeSnapshot(t)
	for name, change := range map[string]func(*pilot.PoolSnapshot){
		"foreign-gym": func(s *pilot.PoolSnapshot) { s.Balance.Value.Balance.GymID = strings.Repeat("a", 64) },
		"source":      func(s *pilot.PoolSnapshot) { s.Balance.Value.BindingID = strings.Repeat("b", 64) },
		"unreduced": func(s *pilot.PoolSnapshot) {
			s.Balance.Value.Balance.Confirmed = poolsettle.Fraction{Numerator: "600000002", Denominator: "6"}
		},
		"negative":         func(s *pilot.PoolSnapshot) { s.Balance.Value.Balance.Available.Numerator = "-1" },
		"lost-view":        func(s *pilot.PoolSnapshot) { s.Account.State = "signed_out" },
		"unavailable-zero": func(s *pilot.PoolSnapshot) { s.Balance.State = "unavailable" },
		"unknown-reason":   func(s *pilot.PoolSnapshot) { s.Balance.Value.Balance.Reason = "operator secret text" },
	} {
		t.Run(name, func(t *testing.T) {
			s := good
			v := *s.Balance.Value
			s.Balance.Value = &v
			change(&s)
			raw, _ := json.Marshal(pilot.PoolResponse{ID: 1, OK: true, Snapshot: &s})
			if _, e := pilot.ParsePoolResponse(raw); e == nil {
				t.Fatal("invalid scope/arithmetic accepted")
			}
		})
	}
	for _, command := range []string{"export_recovery", "verify_backup", "refresh_manifest_prospects"} {
		raw, _ := json.Marshal(pilot.PoolRequest{ID: 1, Command: command})
		if _, e := pilot.ParsePoolRequest(raw); e == nil {
			t.Fatal("unsupported solo operation accepted")
		}
	}
}
func TestPoolBridgeProcess(t *testing.T) {
	mode := os.Getenv("SLUG_POOL_BRIDGE_PROCESS")
	if mode == "" {
		return
	}
	if mode == "startup" {
		ready := os.Getenv("SLUG_POOL_STARTUP_READY")
		ch := make(chan os.Signal, 1)
		signal.Notify(ch, os.Interrupt)
		_ = os.WriteFile(ready, []byte("ready"), 0600)
		<-ch
		os.Exit(0)
	}
	scanner := bufio.NewScanner(os.Stdin)
	calls := 0
	for scanner.Scan() {
		var req pilot.PoolRequest
		if json.Unmarshal(scanner.Bytes(), &req) != nil {
			os.Exit(2)
		}
		calls++
		if req.Command == "stable_snapshot" {
			s := stableBridgeSnapshot(t)
			if mode != "pin1" {
				s.Status.Pin.Version = gymstable.SharedPinVersion2
				s.Status.StoreID, _ = gymstable.ID(s.Status.Pin)
			}
			raw, _ := json.Marshal(pilot.StableResponse{ID: req.ID, OK: true, Snapshot: &s})
			fmt.Println(string(raw))
			continue
		}
		s := poolBridgeSnapshot(t)
		if mode == "hosted" || mode == "hosted-then-legacy" && calls == 1 {
			s.Mode = pilot.HostedPoolMode
		}
		if mode == "hosted-signed-out" {
			s = pilot.BlankPoolSnapshot()
			s.Mode = pilot.HostedPoolMode
		}
		if mode == "backup-uncertain" && req.Command == "export_recovery" {
			raw, _ := json.Marshal(pilot.PoolResponse{ID: req.ID, Error: "export_uncertain"})
			fmt.Println(string(raw))
			continue
		}
		if mode == "backup-wrong-view" && req.Command == "export_recovery" {
			s.View.ViewID = strings.Repeat("e", 64)
		}
		if mode == "changed-source" && calls > 1 {
			s.SourceBindingID = strings.Repeat("e", 64)
			s.Balance.Value.BindingID = s.SourceBindingID
		}
		if mode == "wrong-family" {
			old := manifestBridgeSnapshot(t)
			raw, _ := json.Marshal(pilot.ManifestResponse{ID: req.ID, OK: true, Snapshot: &old})
			fmt.Println(string(raw))
		} else {
			raw, _ := json.Marshal(pilot.PoolResponse{ID: req.ID, OK: true, Snapshot: &s})
			fmt.Println(string(raw))
		}
	}
	os.Exit(0)
}

func TestHostedPoolBridgePinsModeBeforeAnyAccount(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal identity")
	}
	for _, tc := range []struct {
		process, selected string
		accepted          bool
	}{
		{"hosted", pilot.HostedPoolMode, true},
		{"hosted-signed-out", pilot.HostedPoolMode, true},
		{"hosted-signed-out", pilot.PoolMode, false},
		{"valid", pilot.HostedPoolMode, false},
		{"hosted", pilot.PoolMode, false},
		{"hosted-then-legacy", pilot.HostedPoolMode, true},
	} {
		t.Run(tc.process+tc.selected, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestPoolBridgeProcess$", "-test.count=1")
			cmd.Env = append(os.Environ(), "SLUG_POOL_BRIDGE_PROCESS="+tc.process)
			base, err := startStable(cmd)
			if err != nil {
				t.Fatal(err)
			}
			base.poolMode = true
			base.poolExpectedMode = tc.selected
			b := &PoolBridge{base}
			defer b.Close()
			if b.Mode() != tc.selected {
				t.Fatal("mode not bound to launch selection")
			}
			_, err = b.PoolRequest(context.Background(), pilot.PoolRequest{Command: "snapshot"})
			if (err == nil) != tc.accepted {
				t.Fatal("wrong mode acceptance", err)
			}
			if tc.process == "hosted-then-legacy" {
				if _, err = b.PoolRequest(context.Background(), pilot.PoolRequest{Command: "snapshot"}); err == nil {
					t.Fatal("later mode substitution accepted")
				}
			}
		})
	}
}
func TestPoolBridgePinsFamilySourceAndSharedStable(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal identity")
	}
	for _, mode := range []string{"valid", "wrong-family", "changed-source", "pin1"} {
		t.Run(mode, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestPoolBridgeProcess$", "-test.count=1")
			cmd.Env = append(os.Environ(), "SLUG_POOL_BRIDGE_PROCESS="+mode)
			base, e := startStable(cmd)
			if e != nil {
				t.Fatal(e)
			}
			base.poolMode = true
			b := &PoolBridge{base}
			defer b.Close()
			_, e = b.PoolRequest(context.Background(), pilot.PoolRequest{Command: "snapshot"})
			if mode == "wrong-family" {
				if e == nil {
					t.Fatal("legacy family accepted")
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			if mode == "changed-source" {
				if _, e = b.PoolRequest(context.Background(), pilot.PoolRequest{Command: "snapshot"}); e == nil {
					t.Fatal("changed source accepted")
				}
				return
			}
			stable := b.StableRequest(context.Background(), pilot.StableRequest{Command: "stable_snapshot"})
			if (mode == "valid") != stable.OK {
				t.Fatalf("shared scope %s: %+v", mode, stable)
			}
		})
	}
}

func TestPoolBridgeQuitCancelsPendingStartup(t *testing.T) {
	ready := t.TempDir() + "/ready"
	cmd := exec.Command(os.Args[0], "-test.run=^TestPoolBridgeProcess$", "-test.count=1")
	cmd.Env = append(os.Environ(), "SLUG_POOL_BRIDGE_PROCESS=startup", "SLUG_POOL_STARTUP_READY="+ready)
	base, e := startStable(cmd)
	if e != nil {
		t.Fatal(e)
	}
	base.poolMode = true
	b := &PoolBridge{base}
	done := make(chan error, 1)
	go func() { _, e := b.PoolRequest(context.Background(), pilot.PoolRequest{Command: "snapshot"}); done <- e }()
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, e = os.Stat(ready)
		base.mu.Lock()
		pending := base.poolHandshakePending
		base.mu.Unlock()
		if e == nil && pending {
			break
		}
		if time.Now().After(deadline) {
			_ = cmd.Process.Kill()
			t.Fatal("startup handshake did not begin")
		}
		time.Sleep(time.Millisecond)
	}
	closed := make(chan error, 1)
	go func() { closed <- b.Close() }()
	select {
	case e := <-closed:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("Quit waited for constructor timeout")
	}
	if e = <-done; e == nil {
		t.Fatal("closed startup returned a snapshot")
	}
}

func TestPoolSnapshotResponseBudget(t *testing.T) {
	s := pilot.BlankPoolSnapshot()
	raw, e := json.Marshal(pilot.PoolResponse{ID: 1, OK: true, Snapshot: &s})
	if e != nil || len(raw) >= pilot.MaxResponseSize {
		t.Fatal("plain snapshot exceeds bounded pipe", e)
	}
	if _, e = pilot.ParsePoolResponse(raw); e != nil {
		t.Fatal(e)
	}
	if _, e = pilot.ParsePoolResponse(append(raw, bytes.Repeat([]byte(" "), pilot.MaxResponseSize)...)); e == nil {
		t.Fatal("oversized response reached typed decode")
	}
	s.Account.LastError = strings.Repeat("x", pilot.MaxResponseSize)
	raw, _ = json.Marshal(pilot.PoolResponse{ID: 1, OK: true, Snapshot: &s})
	if _, e = pilot.ParsePoolResponse(raw); e == nil {
		t.Fatal("oversized private error accepted")
	}
}

func TestPoolBridgeQuitBeforeFirstRequest(t *testing.T) {
	ready := t.TempDir() + "/ready"
	cmd := exec.Command(os.Args[0], "-test.run=^TestPoolBridgeProcess$", "-test.count=1")
	cmd.Env = append(os.Environ(), "SLUG_POOL_BRIDGE_PROCESS=startup", "SLUG_POOL_STARTUP_READY="+ready)
	base, e := startStable(cmd)
	if e != nil {
		t.Fatal(e)
	}
	base.poolMode = true
	b := &PoolBridge{base}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, e = os.Stat(ready); e == nil {
			break
		}
		if time.Now().After(deadline) {
			_ = cmd.Process.Kill()
			t.Fatal("startup did not begin")
		}
		time.Sleep(time.Millisecond)
	}
	done := make(chan error, 1)
	go func() { done <- b.Close() }()
	select {
	case e := <-done:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("Quit waited for first request")
	}
}
