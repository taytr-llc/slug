package accountpilot

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/wire/v2"
	"os"
	"path/filepath"
	"testing"
)

func TestStableRootFreshUpgradeAndOldRefusal(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal authority")
	}
	for _, upgrade := range []bool{false, true} {
		t.Run(map[bool]string{false: "fresh", true: "upgrade"}[upgrade], func(t *testing.T) {
			cfg := pilot.Config{DataDir: filepath.Join(t.TempDir(), "accounts"), NodeBinary: "/public-fixture/node"}
			var authority string
			if upgrade {
				old, err := NewManifest(cfg)
				if err != nil {
					t.Fatal(err)
				}
				authority = old.store.AuthorityID()
				if err = old.Close(); err != nil {
					t.Fatal(err)
				}
			}
			c, err := NewStable(cfg)
			if err != nil {
				t.Fatal(err)
			}
			marker, err := readStableMarker(filepath.Join(cfg.DataDir, "SLUG-MANIFEST-ACCOUNTS.json"))
			if err != nil || marker.Version != 2 || marker.StableInstalls == nil || len(marker.StableInstalls) != 0 {
				t.Fatal("root2", err)
			}
			if upgrade && authority != c.store.AuthorityID() {
				t.Fatal("upgrade replaced account authority")
			}
			if c.StableSnapshot().View != nil {
				t.Fatal("signed out disclosed view")
			}
			if err = c.Close(); err != nil {
				t.Fatal(err)
			}
			if old, err := NewManifest(cfg); err == nil || old != nil {
				t.Fatal("old route accepted root2")
			}
			again, err := NewStable(cfg)
			if err != nil {
				t.Fatal(err)
			}
			defer again.Close()
			if again.store.AuthorityID() != marker.AccountAuthorityID {
				t.Fatal("reopen replaced identity")
			}
		})
	}
}
func TestStableRootMalformedMarkerNoLockMutation(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal authority")
	}
	cfg := pilot.Config{DataDir: filepath.Join(t.TempDir(), "accounts"), NodeBinary: "/public-fixture/node"}
	if err := os.Mkdir(cfg.DataDir, 0700); err != nil {
		t.Fatal(err)
	}
	raw := []byte("{\"version\":2}\n")
	path := filepath.Join(cfg.DataDir, "SLUG-MANIFEST-ACCOUNTS.json")
	os.WriteFile(path, raw, 0600)
	if c, err := NewStable(cfg); err == nil || c != nil {
		t.Fatal("malformed root accepted")
	}
	after, _ := os.ReadFile(path)
	entries, _ := os.ReadDir(cfg.DataDir)
	if !bytes.Equal(raw, after) || len(entries) != 1 {
		t.Fatal("rejected root mutated")
	}
}
func TestStableRootCheapProfileBeforeUpgrade(t *testing.T) {
	if !wire.TestPoW {
		t.Skip("cheap-only")
	}
	cfg := pilot.Config{DataDir: filepath.Join(t.TempDir(), "root"), NodeBinary: "/public-fixture/node"}
	os.Mkdir(cfg.DataDir, 0700)
	raw, _ := json.Marshal(manifestAccountMarker{Version: 1, Mode: pilot.ManifestMode})
	path := filepath.Join(cfg.DataDir, "SLUG-MANIFEST-ACCOUNTS.json")
	os.WriteFile(path, raw, 0600)
	if c, err := NewStable(cfg); err == nil || c != nil {
		t.Fatal("cheap profile")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(raw, after) {
		t.Fatal("cheap refusal changed root")
	}
}

func TestStableUpgradePreservesExistingAuthoritySession(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal authority")
	}
	cfg := pilot.Config{DataDir: filepath.Join(t.TempDir(), "accounts"), NodeBinary: "/public-fixture/node"}
	old, err := NewManifest(cfg)
	if err != nil {
		t.Fatal(err)
	}
	principal, err := verifyFixture(context.Background(), "zero256", false)
	if err != nil {
		t.Fatal(err)
	}
	session, err := old.store.Login(context.Background(), principal)
	if err != nil {
		t.Fatal(err)
	}
	authority := old.store.AuthorityID()
	if err = old.Close(); err != nil {
		t.Fatal(err)
	}
	next, err := NewStable(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer next.Close()
	if next.store.AuthorityID() != authority {
		t.Fatal("authority changed")
	}
	called := false
	err = next.store.WithSession(context.Background(), session.Token, gymaccount.View, func(a gymaccount.Account, check func() error) error {
		called = true
		if a.ID != session.Account.ID || a.Generation != session.Account.Generation {
			t.Fatal("account changed")
		}
		return check()
	})
	if err != nil || !called {
		t.Fatal("existing token revoked by installation", err)
	}
}
