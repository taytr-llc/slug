//go:build !slugtestpow && (linux || (darwin && cgo))

// Copyright (c) 2026 The SLUG developers. ISC license.
package pooloperator

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRecoverableOperatorFilesRefuseOtherModes(t *testing.T) {
	base := loadDailyOperatorReference(t).Composition.Positives[0].Setup
	selected, e := ComposeRecoverableDailySetup(base)
	if e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(opSetupTestDir(t), "issue")
	f, e := openOperatorFilesSelection(path, base, true, 2, &selected)
	if f != nil {
		t.Cleanup(func() {
			if e := f.close(); e != nil {
				t.Error(e)
			}
		})
	}
	if e != nil {
		t.Fatal(e)
	}
	opSetupTestCompleteFilesystem(t, f)
	before, e := os.ReadFile(filepath.Join(path, "SLUG-POOL-OPERATOR-ISSUE1.json"))
	if e != nil {
		t.Fatal(e)
	}
	want, e := MarshalRecoverableDailySetup(selected)
	if e != nil || !bytes.Equal(before, append(want, '\n')) {
		t.Fatal("selected intent", e)
	}
	if e = f.close(); e != nil {
		t.Fatal(e)
	}
	other, e := openOperatorFilesVersion(path, base, false, 2)
	if other != nil {
		if ce := other.close(); ce != nil {
			t.Fatal(ce)
		}
	}
	if e == nil {
		t.Fatal("plain DAILY adopted request owner")
	}
	after, e := os.ReadFile(filepath.Join(path, "SLUG-POOL-OPERATOR-ISSUE1.json"))
	if e != nil || !bytes.Equal(before, after) {
		t.Fatal("refusal changed original intent", e)
	}
	reopened, e := openOperatorFilesSelection(path, base, false, 2, &selected)
	if reopened != nil {
		t.Cleanup(func() {
			if e := reopened.close(); e != nil {
				t.Error(e)
			}
		})
	}
	if e != nil {
		t.Fatal("selected original reopen", e)
	}
	if e = reopened.close(); e != nil {
		t.Fatal(e)
	}
	// Reverse boundary: the new explicit mode cannot adopt old DAILY markers.
	oldPath := filepath.Join(opSetupTestDir(t), "daily")
	old, e := openOperatorFilesVersion(oldPath, base, true, 2)
	if old != nil {
		t.Cleanup(func() {
			if e := old.close(); e != nil {
				t.Error(e)
			}
		})
	}
	if e != nil {
		t.Fatal(e)
	}
	opSetupTestCompleteFilesystem(t, old)
	if e = old.close(); e != nil {
		t.Fatal(e)
	}
	wrong, e := openOperatorFilesSelection(oldPath, base, false, 2, &selected)
	if wrong != nil {
		if ce := wrong.close(); ce != nil {
			t.Fatal(ce)
		}
	}
	if e == nil {
		t.Fatal("selected mode adopted plain DAILY")
	}
}

func TestRecoverableProvisionOriginalMaterialLostReady(t *testing.T) {
	f := newDailyProvisionTestSelection(t)
	f.args[len(f.args)-1] = "--daily-recoverable-issue"
	refs := loadIssueOperatorReference(t)
	values := map[string][]byte{}
	for _, r := range refs.Fixtures.Wrappers {
		values[r.Name] = provisionHex(t, r.Canonical)
	}
	const prefix = "one-enrollment-"
	expected := f.expected(t)
	delete(expected, provisionIntentFile(2))
	delete(expected, provisionReadyFile(2))
	delete(expected, "offline/"+provisionBackupFile(2))
	expected[provisionIntentFile(3)] = values[prefix+"provision-intent"]
	expected[provisionReadyFile(3)] = values[prefix+"provision-ready"]
	expected["offline/"+provisionBackupFile(3)] = values[prefix+"published-fixture-backup"]
	expected["public/setup.json"] = values[prefix+"operator-intent"]
	check := func() {
		t.Helper()
		for name, want := range expected {
			if len(want) == 0 {
				t.Fatal("missing independent expectation", name)
			}
			got, e := os.ReadFile(filepath.Join(f.root, name))
			if e != nil || !bytes.Equal(got, want) {
				t.Fatal("changed original output", name, e)
			}
		}
		for _, name := range []string{provisionIntentFile(2), provisionReadyFile(2), "offline/" + provisionBackupFile(2)} {
			if _, e := os.Lstat(filepath.Join(f.root, name)); !os.IsNotExist(e) {
				t.Fatal("old marker appeared", name, e)
			}
		}
	}
	if _, e := f.run(t, "create", bytes.NewReader(f.capsule)); e != nil {
		t.Fatal("selected published-fixture provisioning", e)
	}
	check()
	noEntropy := &provisionNoEntropy{}
	if _, e := f.run(t, "verify", noEntropy); e != nil {
		t.Fatal("original selected wallet reopen", e)
	}
	check()
	if e := os.Remove(filepath.Join(f.root, provisionReadyFile(3))); e != nil {
		t.Fatal(e)
	}
	if _, e := f.run(t, "resume", noEntropy); e != nil {
		t.Fatal("lost READY must retain original material", e)
	}
	if noEntropy.calls != 0 {
		t.Fatal("recovery sampled replacement entropy")
	}
	check()
	args := append([]string(nil), f.args...)
	args[0] = "verify"
	args[len(args)-1] = "--daily"
	if e := runProvisionCommand(context.Background(), args, provisionTestOutput(t), provisionTestOutput(t), noEntropy, f.now); e == nil {
		t.Fatal("old mode adopted selected provisioning")
	}
	check()
}

func TestRecoverableProvisionCapacityBeforeEffects(t *testing.T) {
	f := newDailyProvisionTestSelection(t)
	f.args[len(f.args)-1] = "--daily-recoverable-issue"
	// An out-of-profile job count must fail before root creation or entropy.
	p := f.v.Policy
	p.Limits.MaxWorks = 8193
	raw, e := provisionJSON(p, MaxProvisionPolicyBytes+1)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(f.inputs, "policy"), raw, 0600); e != nil {
		t.Fatal(e)
	}
	noEntropy := &provisionNoEntropy{}
	if _, e = f.run(t, "create", noEntropy); e == nil {
		t.Fatal("unsupported capacity admitted")
	}
	if noEntropy.calls != 0 {
		t.Fatal("entropy before capacity refusal")
	}
	if _, e = os.Lstat(f.root); !os.IsNotExist(e) {
		t.Fatal("root before capacity refusal", e)
	}
}
