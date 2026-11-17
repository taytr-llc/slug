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

func TestEntryRootFreshUpgradeAndOldRefusal(t *testing.T) {
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
			c, err := NewEntries(cfg)
			if err != nil {
				t.Fatal(err)
			}
			marker, err := readEntryMarker(filepath.Join(cfg.DataDir, "SLUG-MANIFEST-ACCOUNTS.json"))
			if err != nil || marker.Version != 3 || marker.StableInstalls == nil || len(marker.StableInstalls) != 0 {
				t.Fatal("root3", err)
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
				t.Fatal("old route accepted root3")
			}
			again, err := NewEntries(cfg)
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
func TestEntryRootMalformedMarkerNoLockMutation(t *testing.T) {
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
	if c, err := NewEntries(cfg); err == nil || c != nil {
		t.Fatal("malformed root accepted")
	}
	after, _ := os.ReadFile(path)
	entries, _ := os.ReadDir(cfg.DataDir)
	if !bytes.Equal(raw, after) || len(entries) != 1 {
		t.Fatal("rejected root mutated")
	}
}
func TestEntryRootCheapProfileBeforeUpgrade(t *testing.T) {
	if !wire.TestPoW {
		t.Skip("cheap-only")
	}
	cfg := pilot.Config{DataDir: filepath.Join(t.TempDir(), "root"), NodeBinary: "/public-fixture/node"}
	os.Mkdir(cfg.DataDir, 0700)
	raw, _ := json.Marshal(manifestAccountMarker{Version: 1, Mode: pilot.ManifestMode})
	path := filepath.Join(cfg.DataDir, "SLUG-MANIFEST-ACCOUNTS.json")
	os.WriteFile(path, raw, 0600)
	if c, err := NewEntries(cfg); err == nil || c != nil {
		t.Fatal("cheap profile")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(raw, after) {
		t.Fatal("cheap refusal changed root")
	}
}

func TestEntryUpgradePreservesExistingAuthoritySession(t *testing.T) {
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
	next, err := NewEntries(cfg)
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

func TestEntryParentMarkerExactFieldsAndOldV2Refusal(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal authority")
	}
	cfg := pilot.Config{DataDir: filepath.Join(t.TempDir(), "accounts"), NodeBinary: "/public-fixture/node"}
	old, e := NewStable(cfg)
	if e != nil {
		t.Fatal(e)
	}
	authority := old.store.AuthorityID()
	if e = old.Close(); e != nil {
		t.Fatal(e)
	}
	c, e := NewEntries(cfg)
	if e != nil {
		t.Fatal(e)
	}
	if c.store.AuthorityID() != authority {
		t.Fatal("v2 authority replaced")
	}
	if e = c.Close(); e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(cfg.DataDir, "SLUG-MANIFEST-ACCOUNTS.json")
	raw, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || len(fields) != 10 || fields["entry_calendar_id"] == nil || fields["entry_rules_id"] == nil || fields["calendar_id"] != nil || fields["rules_id"] != nil {
		t.Fatal("root field names", string(raw))
	}
	if old, e = NewStable(cfg); e == nil || old != nil {
		t.Fatal("old v2 route accepted root3")
	}
	bad := bytes.Replace(raw, []byte(`"entry_calendar_id"`), []byte(`"calendar_id"`), 1)
	if e = os.WriteFile(path, bad, 0600); e != nil {
		t.Fatal(e)
	}
	if c, e = NewEntries(cfg); e == nil || c != nil {
		t.Fatal("wrong calendar field accepted")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(after, bad) {
		t.Fatal("refusal mutated marker")
	}
}
