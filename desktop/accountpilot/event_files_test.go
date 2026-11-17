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

func TestEventRootFreshUpgradeAndClosedOldRoutes(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal fixture store")
	}
	for _, version := range []int{0, 1, 2, 3} {
		t.Run(string(rune('0'+version)), func(t *testing.T) {
			cfg := pilot.Config{DataDir: filepath.Join(t.TempDir(), "accounts"), NodeBinary: "/public-fixture/node"}
			var authority string
			if version > 0 {
				var c *ManifestController
				var e error
				switch version {
				case 1:
					c, e = NewManifest(cfg)
				case 2:
					var s *StableController
					s, e = NewStable(cfg)
					if s != nil {
						c = s.ManifestController
					}
				case 3:
					var s *StableController
					s, e = NewEntries(cfg)
					if s != nil {
						c = s.ManifestController
					}
				}
				if e != nil {
					t.Fatal(e)
				}
				authority = c.store.AuthorityID()
				if e = c.Close(); e != nil {
					t.Fatal(e)
				}
			}
			c, e := NewEvents(cfg)
			if e != nil {
				t.Fatal(e)
			}
			if authority != "" && authority != c.store.AuthorityID() {
				t.Fatal("identity replaced")
			}
			m, e := readEventMarker(filepath.Join(cfg.DataDir, "SLUG-MANIFEST-ACCOUNTS.json"))
			if e != nil || m.Version != 4 || m.EventPhase != "waiting_participants" || m.EventStoreID != "" {
				t.Fatalf("marker %+v %v", m, e)
			}
			if c.EventSnapshot().State != "unavailable" || c.EventSnapshot().View != nil {
				t.Fatal("signed out disclosure")
			}
			if e = c.Close(); e != nil {
				t.Fatal(e)
			}
			for _, old := range []func() error{func() error {
				x, e := NewManifest(cfg)
				if x != nil {
					_ = x.Close()
				}
				return e
			}, func() error {
				x, e := NewStable(cfg)
				if x != nil {
					_ = x.Close()
				}
				return e
			}, func() error {
				x, e := NewEntries(cfg)
				if x != nil {
					_ = x.Close()
				}
				return e
			}} {
				if old() == nil {
					t.Fatal("old route adopted root4")
				}
			}
			reopen, e := NewEvents(cfg)
			if e != nil {
				t.Fatal(e)
			}
			if reopen.store.AuthorityID() != m.AccountAuthorityID {
				t.Fatal("reopen replaced authority")
			}
			_ = reopen.Close()
		})
	}
}
func TestEventRootMalformedBeforeMutationAndExactFields(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal fixture store")
	}
	cfg := pilot.Config{DataDir: filepath.Join(t.TempDir(), "accounts"), NodeBinary: "/public-fixture/node"}
	c, e := NewEvents(cfg)
	if e != nil {
		t.Fatal(e)
	}
	_ = c.Close()
	path := filepath.Join(cfg.DataDir, "SLUG-MANIFEST-ACCOUNTS.json")
	raw, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	for _, k := range []string{"entry_schema", "entry_calendar_id", "entry_rules_id", "entry_installs", "event_schema", "event_phase", "event_store_id"} {
		if _, ok := fields[k]; !ok {
			t.Fatal("missing", k)
		}
	}
	bad := bytes.Replace(raw, []byte(`"event_schema":1`), []byte(`"event_schema":2`), 1)
	_ = os.WriteFile(path, bad, 0600)
	_ = os.RemoveAll(filepath.Join(cfg.DataDir, "instance-lock"))
	if x, e := NewEvents(cfg); e == nil || x != nil {
		t.Fatal("invalid marker accepted")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(after, bad) {
		t.Fatal("marker changed")
	}
	if _, e := os.Lstat(filepath.Join(cfg.DataDir, "instance-lock")); !os.IsNotExist(e) {
		t.Fatal("refusal created lock")
	}
}
func TestEventRootCheapRefusalBeforeFilesystem(t *testing.T) {
	if !wire.TestPoW {
		t.Skip("cheap-only")
	}
	cfg := pilot.Config{DataDir: filepath.Join(t.TempDir(), "new"), NodeBinary: "/public-fixture/node"}
	if x, e := NewEvents(cfg); e == nil || x != nil {
		t.Fatal("cheap accepted")
	}
	if _, e := os.Lstat(cfg.DataDir); !os.IsNotExist(e) {
		t.Fatal("cheap mutated filesystem")
	}
}
func TestEventUpgradePreservesAccountSession(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal fixture")
	}
	cfg := pilot.Config{DataDir: filepath.Join(t.TempDir(), "accounts"), NodeBinary: "/public-fixture/node"}
	old, e := NewEntries(cfg)
	if e != nil {
		t.Fatal(e)
	}
	p, e := verifyFixture(context.Background(), "zero256", false)
	if e != nil {
		t.Fatal(e)
	}
	session, e := old.store.Login(context.Background(), p)
	if e != nil {
		t.Fatal(e)
	}
	_ = old.Close()
	c, e := NewEvents(cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer c.Close()
	called := false
	e = c.store.WithSession(context.Background(), session.Token, gymaccount.View, func(a gymaccount.Account, check func() error) error {
		called = true
		if a.ID != session.Account.ID || a.Generation != session.Account.Generation {
			t.Fatal("account replaced")
		}
		return check()
	})
	if e != nil || !called {
		t.Fatal("upgrade revoked session", e)
	}
}
