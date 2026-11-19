// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/desktop/internal/recoveryfile"
	"github.com/taytr-llc/slug/desktop/pilot"
)

func poolBackupProtocolPin(mode string) recoveryfile.Pin {
	p := recoveryfile.Pin{Version: 1, Mode: mode, Path: "/private/tmp/slug-public-fixture-recovery.json", Parent: recoveryfile.Identity{Device: "0000000000000001", Inode: "0000000000000002"}}
	if mode == recoveryfile.ReadMode {
		p.File = &recoveryfile.FileIdentity{Device: p.Parent.Device, Inode: "0000000000000003", Size: 64, ModifiedNS: "1795000000000000000"}
	}
	return p
}
func TestPoolBackupProtocolRequiresBoundSelectionAndView(t *testing.T) {
	for _, command := range []string{"export_recovery", "verify_backup"} {
		mode := recoveryfile.CreateMode
		if command == "verify_backup" {
			mode = recoveryfile.ReadMode
		}
		pin := poolBackupProtocolPin(mode)
		req := pilot.PoolRequest{ID: 1, Command: command, ViewID: strings.Repeat("a", 64), Selection: &pin}
		raw, _ := json.Marshal(req)
		if _, err := pilot.ParsePoolRequest(raw); err != nil {
			t.Fatal("valid selection", err)
		}
		bad := [][]byte{
			bytes.Replace(raw, []byte(`"parent":{`), []byte(`"parent":{"device":"0000000000000001",`), 1),
			bytes.Replace(raw, []byte(`"parent":{`), []byte(`"parent":{"token":"private",`), 1),
			bytes.Replace(raw, []byte(`"view_id":"`+req.ViewID+`",`), nil, 1),
			bytes.Replace(raw, []byte(`"selection":`), []byte(`"path":"/private/tmp/other","selection":`), 1),
			bytes.Replace(raw, []byte(`"selection":`), []byte(`"mnemonic":"private","selection":`), 1),
		}
		for _, value := range bad {
			if _, err := pilot.ParsePoolRequest(value); err == nil {
				t.Fatal("ambiguous/private selection accepted")
			}
		}
		req.Selection = nil
		raw, _ = json.Marshal(req)
		if _, err := pilot.ParsePoolRequest(raw); err == nil {
			t.Fatal("pathless selection accepted")
		}
	}
	for _, view := range []string{"", strings.Repeat("a", 64)} {
		raw, _ := json.Marshal(pilot.PoolRequest{ID: 1, Command: "cancel_pool_backup", ViewID: view})
		_, err := pilot.ParsePoolRequest(raw)
		if (err == nil) != (view != "") {
			t.Fatal("cancel view boundary", err)
		}
	}
}
func TestPoolBackupSnapshotStateAndCapabilityMatrix(t *testing.T) {
	base := poolBridgeSnapshot(t)
	base.State = "stopped"
	base.Account.CanExport = true
	for _, row := range []struct {
		state, phase, code string
		valid              bool
	}{
		{"idle", "", "", true}, {"running", "restoring", "", true}, {"succeeded", "complete", "", true}, {"cancelled", "", "", true}, {"failed", "verifying", "backup_mismatch", true}, {"failed", "opening", "backup_export_uncertain", true},
		{"idle", "complete", "", false}, {"running", "complete", "", false}, {"running", "checking", "backup_invalid", false}, {"succeeded", "verifying", "", false}, {"failed", "", "", false}, {"failed", "complete", "backup_invalid", false}, {"cancelled", "", "backup_storage_failed", false},
	} {
		s := base
		s.BackupState, s.BackupPhase, s.BackupError = row.state, row.phase, row.code
		if (pilot.ValidatePoolSnapshot(s) == nil) != row.valid {
			t.Fatalf("state matrix %+v", row)
		}
	}
	s := base
	s.BackupState = "idle"
	s.Account.Held = true
	s.Account.HoldUntilMS = 1795000000000
	if pilot.ValidatePoolSnapshot(s) == nil {
		t.Fatal("held export capability accepted")
	}
	s.Account.CanExport = false
	if err := pilot.ValidatePoolSnapshot(s); err != nil {
		t.Fatal("held view unavailable", err)
	}
	s = pilot.BlankPoolSnapshot()
	s.BackupState = "succeeded"
	s.BackupPhase = "complete"
	if pilot.ValidatePoolSnapshot(s) == nil {
		t.Fatal("signed-out backup result leaked")
	}
	s = base
	s.BackupState = "running"
	s.BackupPhase = "opening"
	s.State = "running"
	if pilot.ValidatePoolSnapshot(s) == nil {
		t.Fatal("mining and backup active together")
	}
}
func TestPoolBackupBridgeRejectsReplacedViewAndReportsUncertainty(t *testing.T) {
	for _, mode := range []string{"valid", "backup-wrong-view", "backup-uncertain"} {
		t.Run(mode, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestPoolBridgeProcess$", "-test.count=1")
			cmd.Env = append(os.Environ(), "SLUG_POOL_BRIDGE_PROCESS="+mode)
			base, err := startStable(cmd)
			if err != nil {
				t.Fatal(err)
			}
			base.poolMode = true
			b := &PoolBridge{base}
			defer b.Close()
			first, err := b.PoolRequest(context.Background(), pilot.PoolRequest{Command: "snapshot"})
			if err != nil {
				t.Fatal(err)
			}
			pin := poolBackupProtocolPin(recoveryfile.CreateMode)
			if _, err = b.PoolRequest(context.Background(), pilot.PoolRequest{Command: "export_recovery", ViewID: strings.Repeat("0", 64), Selection: &pin}); err == nil {
				t.Fatal("stale dialog sent")
			}
			_, err = b.PoolRequest(context.Background(), pilot.PoolRequest{Command: "export_recovery", ViewID: first.Snapshot.View.ViewID, Selection: &pin})
			if mode == "valid" && err != nil {
				t.Fatal(err)
			}
			if mode == "backup-wrong-view" && err == nil {
				t.Fatal("replacement response accepted")
			}
			if mode == "backup-uncertain" && !errors.Is(err, ErrRecoveryOutcomeUnknown) {
				t.Fatal("uncertainty flattened", err)
			}
		})
	}
}
