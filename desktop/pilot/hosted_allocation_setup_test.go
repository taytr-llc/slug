// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/wire/v2"
	"github.com/taytr-llc/slugwallet/wallet"
)

type hostedAllocationSetupFixture struct {
	root     string
	accounts *gymaccount.Store
	guard    string
	pins     []HostedInventoryPin
	bindings []wallet.AllocationBinding
}

func newHostedAllocationSetupFixture(t *testing.T) hostedAllocationSetupFixture {
	t.Helper()
	if wire.TestPoW {
		t.Skip("normal profile required for concrete inventory setup")
	}
	raw, err := os.ReadFile("../../../slugwallet/wallet/testdata/hosted-allocation/reference.json")
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	if hex.EncodeToString(digest[:]) != "221fc42f6848fed27912a43454a1fa9ebe2d4e16b383aad9e294c31316424e6b" {
		t.Fatal("independent public reference changed")
	}
	var reference struct {
		Bindings []struct {
			Binding wallet.AllocationBinding `json:"binding"`
		} `json:"bindings"`
	}
	if err = json.Unmarshal(raw, &reference); err != nil || len(reference.Bindings) != 2 {
		t.Fatal("independent bindings unavailable", err)
	}
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	f := hostedAllocationSetupFixture{root: filepath.Join(parent, "allocation-recovery"), guard: strings.Repeat("c", 64)}
	principals := make([]gymaccount.FixturePrincipal, 0, 2)
	for i, name := range []string{"zero256", "incrementing256"} {
		binding := reference.Bindings[i].Binding
		identity, err := gymaccount.FixtureIdentity(name)
		if err != nil || binding.InitialManifest.GymID != identity.GymID {
			t.Fatal("wrong independent fixture order", err)
		}
		f.bindings = append(f.bindings, binding)
		f.pins = append(f.pins, HostedInventoryPin{name, binding.InventoryID})
		principals = append(principals, gymaccount.FixturePrincipal{Issuer: "fixture:hosted-allocation-setup", Subject: name, Fixture: name})
	}
	// This is a fresh test authority with an explicitly pinned public identity.
	// No login, enrollment, wallet, node or source receipt is fabricated here.
	cfg := gymaccount.IdentityConfig{Version: 2, VerifierConfigHash: strings.Repeat("a", 64), MaxLoginAttempts: 8, Accounts: gymaccount.Config{Version: 1, Principals: principals, MaxSessions: 8, MaxChallenges: 8, MaxEvents: 8}}
	f.accounts, err = gymaccount.CreateIdentityPinned(filepath.Join(parent, "accounts"), cfg, f.bindings[0].AuthorityID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := f.accounts.Close(); err != nil {
			t.Error(err)
		}
	})
	return f
}

func (f hostedAllocationSetupFixture) open(t *testing.T) *HostedAllocationSetup {
	t.Helper()
	s, err := OpenHostedAllocationSetup(context.Background(), f.root, f.accounts, f.guard, f.pins, false, nil)
	if err != nil {
		t.Fatal("fresh or exact pre-READY setup", err)
	}
	t.Cleanup(func() {
		if err := s.Close(); err != nil {
			t.Error(err)
		}
	})
	return s
}

func TestHostedAllocationSetupFreshAndResumeDirectories(t *testing.T) {
	f := newHostedAllocationSetupFixture(t)
	s := f.open(t)
	if s.Ready() || !reflect.DeepEqual(s.init, hostedAllocationInit{1, f.guard, f.pins}) {
		t.Fatal("fresh setup changed the expected intent")
	}
	initRaw, err := os.ReadFile(filepath.Join(f.root, allocationInitName))
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s = f.open(t) // Resume immediately after INIT, before any fixture intent.
	ids := make([]string, 2)
	intents := make([][]byte, 2)
	for i, pin := range f.pins {
		scope, err := hostedScopeForBinding(f.bindings[i])
		if err != nil {
			t.Fatal(err)
		}
		inv, id, err := s.ensureInventory(context.Background(), pin.Fixture, scope.CurrentBinding, true)
		if err != nil || inv == nil || !payoutNonzeroID(id) || !reflect.DeepEqual(inv.Binding(), f.bindings[i]) {
			t.Fatal("first intent did not create exact inventory", err)
		}
		ids[i] = id
		intents[i], err = os.ReadFile(filepath.Join(f.root, "setup", pin.Fixture+".json"))
		if err != nil {
			t.Fatal(err)
		}
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	s = f.open(t) // Resume real setup/ directories and exact empty inventories.
	for i, pin := range f.pins {
		scope, err := hostedScopeForBinding(f.bindings[i])
		if err != nil {
			t.Fatal(err)
		}
		inv, id, err := s.ensureInventory(context.Background(), pin.Fixture, scope.CurrentBinding, true)
		if err != nil || inv == nil || id != ids[i] || !reflect.DeepEqual(inv.Binding(), f.bindings[i]) {
			t.Fatal("resume replaced an exact intent or inventory", err)
		}
		raw, err := os.ReadFile(filepath.Join(f.root, "setup", pin.Fixture+".json"))
		if err != nil || !bytes.Equal(raw, intents[i]) {
			t.Fatal("resume rewrote fixture intent", err)
		}
	}
	got, err := os.ReadFile(filepath.Join(f.root, allocationInitName))
	if err != nil || !bytes.Equal(got, initRaw) || s.Ready() {
		t.Fatal("pre-READY resume changed initial identity or fabricated READY", err)
	}
}

func TestHostedAllocationSetupRejectsFileAndSymlinkDirectories(t *testing.T) {
	for _, boundary := range []string{"root", "resume-setup", "first-intent-setup"} {
		for _, kind := range []string{"file", "symlink"} {
			t.Run(boundary+"/"+kind, func(t *testing.T) {
				f := newHostedAllocationSetupFixture(t)
				path := f.root
				var s *HostedAllocationSetup
				if boundary != "root" {
					s = f.open(t)
					path = filepath.Join(f.root, "setup")
					if boundary == "resume-setup" {
						if err := s.Close(); err != nil {
							t.Fatal(err)
						}
					}
				}
				target := filepath.Join(filepath.Dir(f.root), "foreign-target")
				if kind == "file" {
					if err := os.WriteFile(path, []byte("not a directory"), 0600); err != nil {
						t.Fatal(err)
					}
				} else {
					if err := os.Mkdir(target, 0700); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink(target, path); err != nil {
						t.Fatal(err)
					}
				}
				if boundary == "first-intent-setup" {
					scope, err := hostedScopeForBinding(f.bindings[0])
					if err != nil {
						t.Fatal(err)
					}
					inv, id, err := s.ensureInventory(context.Background(), "zero256", scope.CurrentBinding, true)
					if err == nil || inv != nil || id != "" || len(s.intents) != 0 || len(s.inventories) != 0 {
						t.Fatal("invalid directory advanced fixture setup", err)
					}
				} else {
					got, err := OpenHostedAllocationSetup(context.Background(), f.root, f.accounts, f.guard, f.pins, false, nil)
					if got != nil {
						_ = got.Close()
					}
					if err == nil || got != nil {
						t.Fatal("accepted non-directory setup boundary", err)
					}
				}
				if kind == "symlink" {
					entries, err := os.ReadDir(target)
					if err != nil || len(entries) != 0 {
						t.Fatal("followed setup alias and wrote its target", err)
					}
				} else {
					raw, err := os.ReadFile(path)
					if err != nil || string(raw) != "not a directory" {
						t.Fatal("replaced invalid path", err)
					}
				}
			})
		}
	}
}
