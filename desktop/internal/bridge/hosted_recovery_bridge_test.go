// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/wire/v2"
	"github.com/taytr-llc/slugwallet/recoveryvault"
	"github.com/taytr-llc/slugwallet/wallet"
)

// Transport-shape examples reuse the independently frozen public binding.
// They make no claim about actual wallet, account or node readiness.
func hostedBridgeSnapshot(t *testing.T, missing bool) pilot.HostedRecoverySnapshot {
	t.Helper()
	raw, err := os.ReadFile("../../../../slugwallet/wallet/testdata/hosted-allocation/reference.json")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != "221fc42f6848fed27912a43454a1fa9ebe2d4e16b383aad9e294c31316424e6b" {
		t.Fatal("independent corpus changed")
	}
	var corpus struct {
		Bindings []struct {
			Binding wallet.AllocationBinding `json:"binding"`
			ID      string                   `json:"binding_id"`
		} `json:"bindings"`
	}
	if err := json.Unmarshal(raw, &corpus); err != nil {
		t.Fatal(err)
	}
	b := corpus.Bindings[0].Binding
	m := b.InitialManifest
	scope := pilot.HostedAllocationScope{Version: 1, InventoryID: b.InventoryID, InventoryBindingID: corpus.Bindings[0].ID,
		CurrentBinding: recoveryvault.CurrentBinding{Version: 1, HostedVaultID: b.HostedVaultID, RetentionID: b.RetentionID, WalletOwnerID: b.WalletOwnerID, SourceBindingID: b.SourceBindingID,
			VaultBinding: recoveryvault.Binding{Version: 1, AuthorityID: b.AuthorityID, AccountID: b.AccountID, PrincipalID: b.PrincipalID, ProvisionID: b.ProvisionID, Network: m.Network, Genesis: m.Genesis, Profile: m.Profile, RootPublicKey: m.RootPublicKey, GymPublicKey: m.GymPublicKey, GymID: m.GymID}}}
	p := poolBridgeSnapshot(t)
	p.Mode = pilot.HostedRecoveryPoolMode
	p.Account.ID, p.View.AccountID, p.View.AccountAuthorityID = b.AccountID, b.AccountID, b.AuthorityID
	p.SourceBindingID, p.Balance.Value.BindingID = b.SourceBindingID, b.SourceBindingID
	p.Account.CanExport = true
	s := pilot.HostedRecoverySnapshot{Version: 1, Mode: pilot.HostedRecoveryPoolMode, Kind: "healthy", Account: p.Account,
		Healthy: &pilot.HostedHealthySnapshot{Pool: p, Allocation: scope}}
	if missing {
		s.Kind, s.Healthy = "recovery_only", nil
		s.Recovery = &pilot.HostedMissingWalletSnapshot{View: pilot.HostedRecoveryView{Version: 1, ViewID: p.View.ViewID, AccountAuthorityID: b.AuthorityID, AccountID: b.AccountID, Generation: p.Account.Generation, Fixture: p.Account.Fixture, RootPublicKey: m.RootPublicKey, GymPublicKey: m.GymPublicKey, GymID: m.GymID}, Allocation: scope, LocalWallet: "missing", InventoryRevision: 2, ObservedAtMS: 1795000000000, Backup: pilot.HostedBackupSnapshot{State: "idle"}}
	}
	return s
}

func TestHostedRecoveryBridgeProcess(t *testing.T) {
	mode := os.Getenv("SLUG_HOSTED_RECOVERY_BRIDGE_PROCESS")
	if mode == "" {
		return
	}
	s := hostedBridgeSnapshot(t, mode == "missing")
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		var req pilot.HostedRecoveryRequest
		if json.Unmarshal(scanner.Bytes(), &req) != nil {
			os.Exit(2)
		}
		if mode == "wrong-family" {
			old := poolBridgeSnapshot(t)
			raw, _ := json.Marshal(pilot.PoolResponse{ID: req.ID, OK: true, Snapshot: &old})
			fmt.Println(string(raw))
			continue
		}
		r := pilot.HostedRecoveryResponse{ID: req.ID, Version: 1, OK: true, Snapshot: &s}
		if strings.HasPrefix(req.Command, "hosted_recovery_receive_") {
			r.Snapshot = nil
			code := ""
			if strings.HasSuffix(req.Command, "receive_begin") && strings.HasPrefix(mode, "unknown-") {
				code = "outcome_unknown"
			}
			if strings.HasSuffix(req.Command, "receive_lookup") {
				switch mode {
				case "unknown-absent":
					code = "allocation_not_found"
				case "unknown-durable":
					code = "allocation_resume_required"
				case "unknown-busy":
					code = "busy"
				}
			}
			if code != "" {
				r.OK, r.Error = false, code
			} else {
				j := pilot.HostedReceiveJob{Version: 1, ActionID: req.ActionID, OperationID: req.OperationID, ViewID: req.ViewID, Allocation: s.Healthy.Allocation, State: "running"}
				if mode == "wrong-action" {
					j.ActionID = strings.Repeat("e", 64)
				}
				if mode == "wrong-operation" {
					j.OperationID = strings.Repeat("e", 64)
				}
				if mode == "wrong-view" {
					j.ViewID = strings.Repeat("e", 64)
				}
				r.Receive = &j
			}
		}
		if req.Command == "hosted_recovery_logout" {
			blank := pilot.BlankHostedRecoverySnapshot()
			r.Snapshot = &blank
		}
		raw, _ := json.Marshal(r)
		fmt.Println(string(raw))
	}
	os.Exit(0)
}

func newHostedBridgeTest(t *testing.T, mode string) *PoolBridge {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestHostedRecoveryBridgeProcess$", "-test.count=1")
	cmd.Env = append(os.Environ(), "SLUG_HOSTED_RECOVERY_BRIDGE_PROCESS="+mode)
	base, err := startStable(cmd)
	if err != nil {
		t.Fatal(err)
	}
	base.poolMode, base.poolExpectedMode = true, pilot.HostedRecoveryPoolMode
	b := &PoolBridge{base}
	t.Cleanup(func() { _ = b.Close() })
	return b
}
func hostedTestRoot(t *testing.T, b *PoolBridge) HostedRecoveryContext {
	t.Helper()
	r, err := b.HostedRecoveryRequest(context.Background(), pilot.HostedRecoveryRequest{Command: "hosted_recovery_snapshot"})
	if err != nil || r.Snapshot == nil {
		t.Fatal("root", err)
	}
	owner, err := HostedContextForSnapshot(*r.Snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return owner
}
func hostedTestReceive(owner HostedRecoveryContext) pilot.HostedRecoveryRequest {
	return pilot.HostedRecoveryRequest{Command: "hosted_recovery_receive_begin", ViewID: owner.ViewID, ActionID: strings.Repeat("a", 64), OperationID: strings.Repeat("b", 64)}
}

func TestHostedRecoveryBridgeOwnerAndFamilyBoundaries(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal identity")
	}
	b := newHostedBridgeTest(t, "healthy")
	owner := hostedTestRoot(t, b)
	if owner.Kind != pilot.HostedHealthyOwner || !b.HostedContextMatches(owner) {
		t.Fatal("healthy owner not retained")
	}
	changed := owner
	changed.Allocation.CurrentBinding.WalletOwnerID = strings.Repeat("e", 64)
	if b.HostedContextMatches(changed) {
		t.Fatal("owner scope substitution accepted")
	}
	if _, err := b.HostedRecoveryRequest(context.Background(), pilot.HostedRecoveryRequest{Command: "hosted_recovery_logout"}); err != nil {
		t.Fatal(err)
	}
	if b.HostedContextMatches(owner) {
		t.Fatal("logout preserved child authority")
	}
	if _, err := b.HostedRecoveryRequest(context.Background(), hostedTestReceive(owner)); err == nil {
		t.Fatal("old receive owner accepted after logout")
	}
	missing := newHostedBridgeTest(t, "missing")
	recoveryOwner := hostedTestRoot(t, missing)
	if recoveryOwner.Kind != pilot.HostedMissingOwner || missing.withdrawalPool != nil || missing.manifestView != nil {
		t.Fatal("missing wallet acquired child authority")
	}
	if _, err := missing.HostedRecoveryRequest(context.Background(), hostedTestReceive(recoveryOwner)); err == nil {
		t.Fatal("missing wallet admitted receive")
	}
	wrong := newHostedBridgeTest(t, "wrong-family")
	if _, err := wrong.HostedRecoveryRequest(context.Background(), pilot.HostedRecoveryRequest{Command: "hosted_recovery_snapshot"}); err == nil {
		t.Fatal("legacy frame adopted by new mode")
	}
}

func TestHostedRecoveryBridgeExactReceiveRecovery(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal identity")
	}
	for _, mode := range []string{"healthy", "wrong-action", "wrong-operation", "wrong-view", "unknown-absent", "unknown-durable", "unknown-busy"} {
		t.Run(mode, func(t *testing.T) {
			b := newHostedBridgeTest(t, mode)
			owner := hostedTestRoot(t, b)
			req := hostedTestReceive(owner)
			_, err := b.HostedRecoveryRequest(context.Background(), req)
			if strings.HasPrefix(mode, "wrong-") {
				if err == nil {
					t.Fatal("foreign reply resolved action")
				}
				return
			}
			if mode == "healthy" {
				if err != nil {
					t.Fatal(err)
				}
			} else if !IsHostedRecoveryUnknown(err) {
				t.Fatal("unknown admission lost", err)
			}
			newReq := req
			newReq.ActionID = strings.Repeat("c", 64)
			newReq.OperationID = strings.Repeat("d", 64)
			if _, err := b.HostedRecoveryRequest(context.Background(), newReq); !errors.Is(err, ErrBusy) {
				t.Fatal("second invocation replaced pending action", err)
			}
			wrong := req
			wrong.Command = "hosted_recovery_receive_lookup"
			wrong.OperationID = newReq.OperationID
			if _, err := b.HostedRecoveryRequest(context.Background(), wrong); err == nil {
				t.Fatal("lookup changed operation")
			}
			req.Command = "hosted_recovery_receive_lookup"
			_, err = b.HostedRecoveryRequest(context.Background(), req)
			if mode == "healthy" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			var refusal *HostedRecoveryFailure
			if !errors.As(err, &refusal) {
				t.Fatal("closed refusal missing", err)
			}
			b.mu.Lock()
			state := b.hostedReceive.state
			b.mu.Unlock()
			want := map[string]string{"unknown-absent": "failed", "unknown-durable": "needs_resume", "unknown-busy": "unknown"}[mode]
			if state != want {
				t.Fatalf("lookup state %s want %s", state, want)
			}
			if mode == "unknown-busy" {
				if _, err := b.HostedRecoveryRequest(context.Background(), newReq); !errors.Is(err, ErrBusy) {
					t.Fatal("refused lookup erased uncertainty", err)
				}
			}
		})
	}
}
