// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/desktop/pilot"
)

func hostedControlFixtures(t *testing.T) (HostedInit, HostedReady, []SetupReceipt, map[string]string) {
	t.Helper()
	var init HostedInit
	var ready HostedReady
	var stages []SetupReceipt
	raws := map[string]string{}
	for _, r := range hostedReference(t).Records {
		var dst any
		switch r.Kind {
		case "init":
			dst = &init
		case "ready":
			dst = &ready
		case "stage_receipt":
			dst = new(SetupReceipt)
		default:
			continue
		}
		if err := json.Unmarshal(r.Value, dst); err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal(dst)
		if err != nil || !bytes.Equal(append(raw, '\n'), []byte(r.Canonical)) {
			t.Fatal("independent marker bytes differ", r.Name, err)
		}
		if p, ok := dst.(*SetupReceipt); ok {
			stages = append(stages, *p)
			raws[p.Stage] = r.Canonical
		} else {
			raws[r.Kind] = r.Canonical
		}
	}
	if len(stages) != 8 || !hostedReadyMatches(init, ready) {
		t.Fatal("incomplete independent marker fixtures")
	}
	return init, ready, stages, raws
}
func hostedControlRoot(t *testing.T) string {
	t.Helper()
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(parent, "hosted")
	lock, err := openHostedRoot(root, pilot.PoolOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := lock.Close(); err != nil {
			t.Error(err)
		}
	})
	return root
}
func hostedControlInitialize(t *testing.T, root string, through int) (HostedInit, HostedReady, []SetupReceipt) {
	t.Helper()
	init, ready, stages, _ := hostedControlFixtures(t)
	if err := writeHostedInit(root, init); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < through; i++ {
		if dir := hostedStageDirectories[i]; dir != "" {
			if err := os.Mkdir(filepath.Join(root, dir), 0700); err != nil {
				t.Fatal(err)
			}
		}
		if err := writeHostedSetupReceipt(root, stages[i]); err != nil {
			t.Fatal(err)
		}
	}
	return init, ready, stages
}

func TestHostedControlIndependentMarkersAndOrderedPublication(t *testing.T) {
	root := hostedControlRoot(t)
	init, ready, stages, raws := hostedControlFixtures(t)
	if _, found, err := readHostedInit(root); found || err != nil {
		t.Fatal("new root acquired identity", err)
	}
	if err := writeHostedInit(root, init); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(root, hostedInitName))
	if err != nil || string(raw) != raws["init"] {
		t.Fatal("INIT differs from independent bytes", err)
	}
	if err := writeHostedReady(root, ready); err == nil {
		t.Fatal("READY preceded concrete stages")
	}
	for i, receipt := range stages {
		if got := hostedReadyStageHash(init, ready, receipt.Stage); got != receipt.IdentitySHA256 {
			t.Fatal("stage preimage mismatch", receipt.Stage)
		}
		if dir := hostedStageDirectories[i]; dir != "" {
			if err := writeHostedSetupReceipt(root, receipt); err == nil {
				t.Fatal("stage accepted without component directory")
			}
			if err := os.Mkdir(filepath.Join(root, dir), 0700); err != nil {
				t.Fatal(err)
			}
		}
		prefix, err := readHostedSetupPrefix(root, init)
		if err != nil || len(prefix) != i {
			t.Fatal("next unfinished initializer refused", i, err)
		}
		if err = writeHostedSetupReceipt(root, receipt); err != nil {
			t.Fatal(err)
		}
		raw, err = os.ReadFile(filepath.Join(root, "setup", receipt.Stage+".json"))
		if err != nil || string(raw) != raws[receipt.Stage] {
			t.Fatal("receipt bytes differ", receipt.Stage, err)
		}
		if err = writeHostedSetupReceipt(root, receipt); err == nil {
			t.Fatal("receipt overwrite accepted")
		}
	}
	if err := writeHostedReady(root, ready); err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile(filepath.Join(root, hostedReadyName))
	if err != nil || string(raw) != raws["ready"] {
		t.Fatal("READY bytes differ", err)
	}
	if prefix, err := readHostedSetupPrefix(root, init); err != nil || len(prefix) != 8 {
		t.Fatal("complete exact marker set refused", err)
	}
	wrong := init
	wrong.AuthorityID = strings.Repeat("e", 64)
	if err := writeHostedInit(root, wrong); err == nil {
		t.Fatal("INIT overwritten")
	}
	raw, _ = os.ReadFile(filepath.Join(root, hostedInitName))
	if string(raw) != raws["init"] {
		t.Fatal("failed replacement changed original pin")
	}
}

func TestHostedControlPrefixRefusesMissingUnknownAndSkippedStores(t *testing.T) {
	for _, mode := range []string{"unknown-root", "skipped-component", "missing-component", "gap", "unknown-stage", "symlink-stage", "ready-before-stages", "altered-stage-hash"} {
		t.Run(mode, func(t *testing.T) {
			root := hostedControlRoot(t)
			init, ready, stages := hostedControlInitialize(t, root, 2)
			var err error
			switch mode {
			case "unknown-root":
				err = os.WriteFile(filepath.Join(root, "foreign"), []byte("x"), 0600)
			case "skipped-component":
				err = os.Mkdir(filepath.Join(root, "shared"), 0700)
			case "missing-component":
				err = os.Remove(filepath.Join(root, "authority"))
			case "gap":
				err = os.Remove(filepath.Join(root, "setup", "authority.json"))
			case "unknown-stage":
				err = os.WriteFile(filepath.Join(root, "setup", "other.json"), []byte("{}\n"), 0600)
			case "symlink-stage":
				if err = os.Remove(filepath.Join(root, "setup", "authority.json")); err == nil {
					err = os.Symlink(filepath.Join(root, "setup", "coordinator.json"), filepath.Join(root, "setup", "authority.json"))
				}
			case "ready-before-stages":
				err = writeHostedControl(filepath.Join(root, hostedReadyName), ready, hostedMarkerMax)
			case "altered-stage-hash":
				stages[0].IdentitySHA256 = strings.Repeat("f", 64)
				raw, _ := json.Marshal(stages[0])
				err = os.WriteFile(filepath.Join(root, "setup", "authority.json"), append(raw, '\n'), 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := readHostedSetupPrefix(root, init); err == nil {
				t.Fatal("corrupt prefix accepted")
			}
		})
	}
}

func TestHostedControlStrictRawFramingAndLimits(t *testing.T) {
	_, _, _, raws := hostedControlFixtures(t)
	valid := raws["init"]
	cases := [][]byte{[]byte(strings.TrimSuffix(valid, "\n")), []byte(valid + "\n"), []byte(strings.Replace(valid, "\"version\":1", "\"Version\":1", 1)), []byte(strings.Replace(valid, "\"version\":1", "\"version\":1,\"version\":1", 1)), []byte(strings.Replace(valid, "\"version\":1", "\"version\":1.0", 1)), []byte(strings.Repeat("x", hostedMarkerMax+1))}
	for _, r := range hostedReference(t).Raw {
		if r.Kind == "init" && !r.Valid {
			raw, err := hex.DecodeString(r.Hex)
			if err != nil {
				t.Fatal(err)
			}
			cases = append(cases, raw)
		}
	}
	for i, raw := range cases {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, hostedInitName), raw, 0600); err != nil {
			t.Fatal(err)
		}
		if _, _, err := readHostedInit(root); err == nil {
			t.Fatal("invalid raw marker accepted", i)
		}
	}
}

func TestHostedControlRootCannotAdoptLegacyOrAliases(t *testing.T) {
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(parent, "old-custom")
	if err = os.Mkdir(legacy, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(legacy, "SLUG-POOL-ACCOUNTS.json"), []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, root := range []string{legacy, filepath.Join(legacy, "nested"), parent} {
		if lock, err := openHostedRoot(root, pilot.PoolOptions{}); err == nil {
			_ = lock.Close()
			t.Fatal("managed overlap adopted", root)
		}
	}
	alias := filepath.Join(parent, "alias")
	if err = os.Symlink(legacy, alias); err != nil {
		t.Fatal(err)
	}
	if lock, err := openHostedRoot(alias, pilot.PoolOptions{}); err == nil {
		_ = lock.Close()
		t.Fatal("root alias adopted")
	}
	if _, err = os.Lstat(filepath.Join(legacy, "nested")); !os.IsNotExist(err) {
		t.Fatal("failed overlap mutated namespace")
	}
}
