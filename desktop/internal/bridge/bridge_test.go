// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.
package bridge

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"github.com/taytr-llc/slug/desktop/pilot"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestBridgeHelper(t *testing.T) {
	scenario := os.Getenv("SLUG_BRIDGE_FIXTURE")
	if scenario == "" {
		return
	}
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var request pilot.Request
		_ = json.Unmarshal(scanner.Bytes(), &request)
		switch scenario {
		case "malformed":
			fmt.Println(`{"id":1,"id":1,"ok":false,"error":"invalid_request"}`)
		case "oversized":
			fmt.Println(strings.Repeat("x", pilot.MaxResponseSize+1))
		case "wrong-id":
			fmt.Println(`{"id":999,"ok":false,"error":"invalid_request"}`)
		case "eof":
			os.Exit(2)
		case "stall":
			_, _ = io.Copy(io.Discard, os.Stdin)
			os.Exit(0)
		default:
			response := pilot.Response{ID: request.ID, OK: true, Snapshot: &pilot.Snapshot{State: "stopped", Mode: pilot.Mode, WalletState: "closed", BackupState: "idle", Prospects: []pilot.Prospect{}}}
			raw, _ := json.Marshal(response)
			fmt.Println(string(raw))
		}
	}
	if path := os.Getenv("SLUG_BRIDGE_DRAIN_MARKER"); path != "" {
		_ = os.WriteFile(path, []byte("drained"), 0600)
	}
	os.Exit(0)
}
func fixture(t *testing.T, scenario string) *Bridge {
	t.Helper()
	executable, _ := os.Executable()
	cmd := exec.Command(executable, "-test.run=^TestBridgeHelper$")
	cmd.Env = append(os.Environ(), "SLUG_BRIDGE_FIXTURE="+scenario)
	b, err := start(cmd)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	return b
}
func TestRoundTripAndCloseDrain(t *testing.T) {
	marker := t.TempDir() + "/drained"
	t.Setenv("SLUG_BRIDGE_DRAIN_MARKER", marker)
	b := fixture(t, "valid")
	response, err := b.Request(context.Background(), "snapshot", 0)
	if err != nil || response.Snapshot.State != "stopped" {
		t.Fatalf("response %v: %v", response, err)
	}
	if err = b.Close(); err != nil {
		t.Fatal(err)
	}
	if raw, err := os.ReadFile(marker); err != nil || string(raw) != "drained" {
		t.Fatal("returned before helper drained")
	}
	if _, err = b.Request(context.Background(), "start", 0); err == nil {
		t.Fatal("request after close")
	}
}
func TestMalformedResponsesFailClosed(t *testing.T) {
	for _, scenario := range []string{"malformed", "oversized", "wrong-id", "eof"} {
		t.Run(scenario, func(t *testing.T) {
			b := fixture(t, scenario)
			if _, err := b.Request(context.Background(), "snapshot", 0); err == nil {
				t.Fatal("accepted invalid response")
			}
		})
	}
}
func TestTimeoutAndCloseInterruptIdleHelper(t *testing.T) {
	b := fixture(t, "stall")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := b.Request(ctx, "snapshot", 0); err == nil {
		t.Fatal("stalled request succeeded")
	}
	done := make(chan struct{})
	go func() { _ = b.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("helper orphaned after timeout")
	}
}
func TestResponseCanonicalAndBounds(t *testing.T) {
	valid := pilot.Response{ID: 1, OK: true, Snapshot: &pilot.Snapshot{State: "stopped", Mode: pilot.Mode, WalletState: "closed", BackupState: "idle", Prospects: []pilot.Prospect{}}}
	raw, _ := json.Marshal(valid)
	if _, err := decode(raw); err != nil {
		t.Fatal(err)
	}
	for _, mutated := range []string{strings.Replace(string(raw), `"id"`, `"ID"`, 1), strings.Replace(string(raw), `"mode":"local-wallet-pilot"`, `"mode":"local-wallet-pilot","mode":"local-wallet-pilot"`, 1), strings.Replace(string(raw), `"prospects":[]`, `"prospects":null`, 1), string(raw) + `{}`, `{"id":1,"ok":true}`, `{"id":1,"ok":false,"error":"sensitive path or key"}`} {
		if _, err := decode([]byte(mutated)); err == nil {
			t.Fatalf("accepted %s", mutated)
		}
	}
}

func TestRecoveryNativePathAndPublicResponse(t *testing.T) {
	b := fixture(t, "valid")
	for _, command := range []string{"export_recovery", "verify_backup"} {
		if _, err := b.Request(context.Background(), command, 0); err == nil {
			t.Fatal("public command accepted path operation")
		}
		response, err := b.RequestPath(context.Background(), command, "/private/tmp/test wallet.json")
		if err != nil || response.Snapshot == nil || response.Receipt != nil {
			t.Fatalf("native request: %v", err)
		}
	}
	for _, command := range []string{"snapshot", "receipt", "dumpprivkey", "stop"} {
		if _, err := b.RequestPath(context.Background(), command, "/tmp/a"); err == nil {
			t.Fatal("unrestricted path command")
		}
	}
	if _, err := b.RequestPath(context.Background(), "verify_backup", "relative"); err == nil {
		t.Fatal("relative path accepted")
	}
	base := pilot.Response{ID: 1, OK: true, Snapshot: &pilot.Snapshot{State: "stopped", Mode: pilot.Mode, WalletState: "closed", BackupState: "idle", Prospects: []pilot.Prospect{}}}
	raw, _ := json.Marshal(base)
	for _, secret := range []string{"mnemonic", "seed", "private_key", "rpc_password"} {
		mutated := strings.Replace(string(raw), `"wallet_state":"closed"`, `"wallet_state":"closed","`+secret+`":"forbidden"`, 1)
		if _, err := decode([]byte(mutated)); err == nil {
			t.Fatal("accepted private response field")
		}
	}
	for _, mutate := range []func(*pilot.Snapshot){
		func(s *pilot.Snapshot) { s.SpendableAtoms = -1 },
		func(s *pilot.Snapshot) { s.ImmatureAtoms = 1 << 53 },
		func(s *pilot.Snapshot) { s.GymPublicKey = "not a public key" },
		func(s *pilot.Snapshot) { s.BackupError = "/private/path" },
		func(s *pilot.Snapshot) { s.BackupPhase = "unbounded private content" },
	} {
		copy := *base.Snapshot
		mutate(&copy)
		response := base
		response.Snapshot = &copy
		raw, _ := json.Marshal(response)
		if _, err := decode(raw); err == nil {
			t.Fatal("accepted unsafe wallet state")
		}
	}
}
