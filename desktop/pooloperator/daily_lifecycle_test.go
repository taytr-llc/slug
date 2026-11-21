//go:build !slugtestpow && (linux || (darwin && cgo))

// Copyright (c) 2026 The SLUG developers. ISC license.
package pooloperator

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func newDailyProvisionTestSelection(t *testing.T) *provisionTestSelection {
	t.Helper()
	refs := provisionReference(t)
	f := newProvisionTestSelectionForVector(t, refs, dailyProvisionMaterial(t, refs))
	v := loadDailyOperatorReference(t).Composition.Positives[0]
	f.v.Policy, f.v.PolicyID, f.v.Setup, f.v.SetupID = v.Policy, v.PolicyID, v.Setup, v.SetupID
	f.v.PolicyHex, f.v.SetupHex = v.PolicyHex, v.SetupHex
	f.v.PolicyFile = hexFile(v.PolicyHex)
	f.v.SetupFile = hexFile(v.SetupHex)
	f.v.Intent, f.v.Ready, f.v.BackupFile = v.ProvisionIntent, v.ProvisionReady, v.Backup
	if provisionJSONFile(provisionHex(t, v.Backup), 8192, &f.v.Backup) != nil {
		t.Fatal("backup fixture")
	}
	if e := os.WriteFile(filepath.Join(f.inputs, "policy"), provisionHex(t, f.v.PolicyFile), 0600); e != nil {
		t.Fatal(e)
	}
	f.args = append(f.args, "--daily")
	return f
}
func hexFile(canonical string) string { return canonical + "0a" }
func TestDailyProvisionOriginalWalletAndRuntimeReopen(t *testing.T) {
	testProvisionCreateVerifyLostReadyAndRuntimeInit(t, newDailyProvisionTestSelection(t))
}
func TestDailyProvisionResumesOriginalMaterialAndRefusesOtherMode(t *testing.T) {
	for _, prefix := range []int{2, 7} {
		t.Run(string(rune('0'+prefix)), func(t *testing.T) {
			f := newDailyProvisionTestSelection(t)
			f.prefix(t, prefix)
			noEntropy := &provisionNoEntropy{}
			if _, e := f.run(t, "resume", noEntropy); e != nil || noEntropy.calls != 0 {
				t.Fatal("retained daily material", e)
			}
			f.assertFiles(t)
			args := append([]string(nil), f.args[:len(f.args)-1]...)
			args[0] = "verify"
			before, e := os.ReadFile(filepath.Join(f.root, provisionReadyFile(2)))
			if e != nil {
				t.Fatal(e)
			}
			if e := runProvisionCommand(context.Background(), args, provisionTestOutput(t), provisionTestOutput(t), noEntropy, f.now); e == nil {
				t.Fatal("V1 adopted V2")
			}
			after, e := os.ReadFile(filepath.Join(f.root, provisionReadyFile(2)))
			if e != nil || !bytes.Equal(before, after) {
				t.Fatal("cross-mode changed READY")
			}
			f.assertFiles(t)
		})
	}
}
func TestDailyOperatorFilesSelectedMarkers(t *testing.T) {
	s := loadDailyOperatorReference(t).Composition.Positives[0].Setup
	path := filepath.Join(opSetupTestDir(t), "daily")
	f, e := openOperatorFilesVersion(path, s, true, 2)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := f.close(); e != nil {
			t.Error(e)
		}
	})
	opSetupTestCompleteFilesystem(t, f)
	raw, e := os.ReadFile(filepath.Join(path, operatorIntentName(2)))
	if e != nil {
		t.Fatal(e)
	}
	want, _ := json.Marshal(s)
	if !bytes.Equal(raw, append(want, '\n')) {
		t.Fatal("intent changed")
	}
	if e = f.close(); e != nil {
		t.Fatal(e)
	}
	other, e := openOperatorFiles(path, s, false)
	if other != nil {
		if ce := other.close(); ce != nil {
			t.Fatal(ce)
		}
	}
	if e == nil {
		t.Fatal("legacy filesystem accepted daily")
	}
	reopened, e := openOperatorFilesVersion(path, s, false, 2)
	if e != nil {
		t.Fatal(e)
	}
	if e = reopened.close(); e != nil {
		t.Fatal(e)
	}
}

func TestDailyProvisionCapacityBeforeCreation(t *testing.T) {
	f := newDailyProvisionTestSelection(t)
	p := f.v.Policy
	p.Limits.PoolMaxBytes = 4096
	raw, e := json.Marshal(p)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(f.inputs, "policy"), raw, 0600); e != nil {
		t.Fatal(e)
	}
	noEntropy := &provisionNoEntropy{}
	if _, e = f.run(t, "create", noEntropy); e == nil {
		t.Fatal("insufficient reservation accepted")
	}
	if noEntropy.calls != 0 {
		t.Fatal("entropy before reservation")
	}
	if _, e = os.Stat(f.root); !os.IsNotExist(e) {
		t.Fatal("root created before reservation", e)
	}
}
