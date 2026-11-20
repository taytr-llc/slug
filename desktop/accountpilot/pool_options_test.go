// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slugwallet/recovery"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func poolOptionTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	if e := filepath.WalkDir(root, func(p string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		rel, e := filepath.Rel(root, p)
		if e != nil {
			return e
		}
		if d.IsDir() {
			out[rel] = "directory"
			return nil
		}
		raw, e := os.ReadFile(p)
		if e != nil {
			return e
		}
		out[rel] = fmt.Sprintf("%x", sha256.Sum256(raw))
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	return out
}

func poolOptionRoot(t *testing.T) string {
	t.Helper()
	parent, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(parent, "account-pool")
}

func TestPoolAccountSelectionBeforeAnyAuthorityLock(t *testing.T) {
	v1, e := pilot.ResolvePoolOptions(pilot.PoolOptions{})
	if e != nil {
		t.Fatal(e)
	}
	v2, e := fighterdescriptor.ID(fighterdescriptor.CandidateNameV2Descriptor())
	if e != nil {
		t.Fatal(e)
	}
	for _, id := range []string{v1.CandidateDescriptorID, v2} {
		t.Run(id[:8], func(t *testing.T) {
			root := poolOptionRoot(t)
			options := pilot.PoolOptions{CandidateDescriptorID: id}
			store, lock, e := openPoolAccountStoreWithOptions(root, options)
			if e != nil {
				t.Fatal(e)
			}
			authority := store.AuthorityID()
			if e = store.Close(); e != nil {
				t.Fatal(e)
			}
			if e = lock.Close(); e != nil {
				t.Fatal(e)
			}
			before := poolOptionTree(t, root)
			other := v2
			if id == v2 {
				other = v1.CandidateDescriptorID
			}
			called := false
			_, e = newPoolWithOptions(context.Background(), pilot.Config{DataDir: root, NodeBinary: "/not-a-node"}, pilot.PoolOptions{CandidateDescriptorID: other}, func(context.Context, pilot.Config, *gymaccount.Store) (*pilot.PoolRuntime, error) {
				called = true
				return nil, nil
			})
			if e == nil || called {
				t.Fatal("mismatch reached runtime")
			}
			if !reflect.DeepEqual(before, poolOptionTree(t, root)) {
				t.Fatal("mismatch opened account authority or lock")
			}
			if id == v2 {
				if _, _, e = openPoolAccountStore(root); e == nil {
					t.Fatal("default silently adopted v2")
				}
				if !reflect.DeepEqual(before, poolOptionTree(t, root)) {
					t.Fatal("default mismatch mutated v2 root")
				}
			}
			store, lock, e = openPoolAccountStoreWithOptions(root, options)
			if e != nil {
				t.Fatal("matching explicit reopen", e)
			}
			if store.AuthorityID() != authority {
				t.Fatal("authority changed")
			}
			if e = store.Close(); e != nil {
				t.Fatal(e)
			}
			if e = lock.Close(); e != nil {
				t.Fatal(e)
			}
		})
	}
	root := filepath.Join(t.TempDir(), "absent")
	if _, e := NewPoolWithOptions(context.Background(), pilot.Config{DataDir: root, NodeBinary: "/not-a-node"}, pilot.PoolOptions{CandidateDescriptorID: "unknown"}); e == nil {
		t.Fatal("unknown selection")
	}
	if _, e := os.Lstat(root); !os.IsNotExist(e) {
		t.Fatal("unknown selection created account root")
	}
}
func TestPoolAccountPreflightsInnerSelectionBeforeOuterLock(t *testing.T) {
	root := poolOptionRoot(t)
	store, lock, e := openPoolAccountStore(root)
	if e != nil {
		t.Fatal(e)
	}
	authority := store.AuthorityID()
	if e = store.Close(); e != nil {
		t.Fatal(e)
	}
	if e = lock.Close(); e != nil {
		t.Fatal(e)
	}
	v2, e := fighterdescriptor.ID(fighterdescriptor.CandidateNameV2Descriptor())
	if e != nil {
		t.Fatal(e)
	}
	// Exact public runtime marker shape; no issuer, wallet or process is fabricated.
	marker := struct {
		Application   string `json:"application"`
		Version       uint32 `json:"version"`
		Authority     string `json:"account_authority_id"`
		Network       uint32 `json:"network"`
		Genesis       string `json:"genesis"`
		Activation    uint32 `json:"activation_height"`
		Recovery      string `json:"recovery_profile"`
		Descriptor    string `json:"descriptor_id"`
		TXIndex       bool   `json:"primary_txindex"`
		PoolPort      uint16 `json:"pool_port"`
		AdmissionPort uint16 `json:"admission_port"`
	}{"slug-shared-pool-runtime", 1, authority, recovery.Network, "7893676099cf9985bb198dfb6a347d9422a19f2422cf3326a999a7230ce246cd", chaincfg.PhaseAActivationHeight, recovery.ProfileID, v2, true, 19061, 19062}
	raw, e := json.Marshal(marker)
	if e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(root, "shared", "SLUG-POOL-RUNTIME.json"), append(raw, '\n'), 0600); e != nil {
		t.Fatal(e)
	}
	before := poolOptionTree(t, root)
	if _, _, e = openPoolAccountStore(root); e == nil {
		t.Fatal("outer v1 adopted inner v2")
	}
	if !reflect.DeepEqual(before, poolOptionTree(t, root)) {
		t.Fatal("inner mismatch touched outer LevelDB files")
	}
}
