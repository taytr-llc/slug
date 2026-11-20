// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"encoding/json"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/poolround"
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/poolsettlefixture"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/registry"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// These sentinels panic if construction/audit starts live backend work.
type auditNoNode struct {
	poolshare.Node
	poolround.Observer
}
type auditNoSessions struct {
	prospectservice.SessionAuthorizer
}
type auditNoBackend struct{ poolsettle.Backend }

func hostedAuditFiles(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	e := filepath.WalkDir(root, func(p string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			return nil
		}
		raw, e := os.ReadFile(p)
		if e != nil {
			return e
		}
		out[p] = string(raw)
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	return out
}
func TestHostedRecoveryAuditAccountingNoBackendNoWrites(t *testing.T) {
	root := t.TempDir()
	issuer, e := registry.CreateManifestAuthority(filepath.Join(root, "issuer"), policy().Network, genesis, manifestDescriptorID)
	if e != nil {
		t.Fatal(e)
	}
	defer issuer.Close()
	info, e := issuer.Info()
	if e != nil {
		t.Fatal(e)
	}
	marker := poolRuntimeMarker{}
	source := marker.source(info.Pin)
	sharesPath, roundsPath, settlePath := filepath.Join(root, "shares"), filepath.Join(root, "rounds"), filepath.Join(root, "settlement")
	shares, e := poolshare.Create(sharesPath, source, issuer, auditNoNode{}, auditNoSessions{})
	if e != nil {
		t.Fatal(e)
	}
	limits := poolround.Limits{Version: 1, MaxRounds: 256, MaxBytes: 256 << 20}
	rounds, e := poolround.Create(roundsPath, limits, shares, auditNoNode{})
	if e != nil {
		t.Fatal(e)
	}
	identity, e := gymaccount.FixtureIdentity("zero256")
	if e != nil {
		t.Fatal(e)
	}
	config := poolsettle.Config{Version: 1, Destinations: []poolsettle.Destination{{GymID: identity.GymID, Script: "0014" + strings.Repeat("a", 40)}}, ChangeScript: poolsettlefixture.PoolScript(), MaxRounds: 256, MaxBatches: 366}
	settlement, e := poolsettle.Create(settlePath, config, rounds, auditNoBackend{})
	if e != nil {
		t.Fatal(e)
	}
	for _, close := range []func() error{settlement.Close, rounds.Close, shares.Close} {
		if e = close(); e != nil {
			t.Fatal(e)
		}
	}
	before := []map[string]string{hostedAuditFiles(t, sharesPath), hostedAuditFiles(t, roundsPath), hostedAuditFiles(t, settlePath)}
	shares, e = poolshare.OpenAudit(sharesPath, source, issuer)
	if e != nil {
		t.Fatal(e)
	}
	rounds, e = poolround.OpenAudit(roundsPath, limits, shares)
	if e != nil {
		t.Fatal(e)
	}
	settlement, e = poolsettle.OpenAudit(settlePath, config, rounds)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = shares.Issue(context.Background(), "never-authorized"); e == nil {
		t.Fatal("audit issued work")
	}
	if _, e = rounds.Sync(t.Context()); e == nil {
		t.Fatal("audit advanced rounds")
	}
	if _, e = settlement.RunDaily(t.Context()); e == nil {
		t.Fatal("audit ran settlement")
	}
	for _, close := range []func() error{settlement.Close, rounds.Close, shares.Close} {
		if e = close(); e != nil {
			t.Fatal(e)
		}
	}
	for i, p := range []string{sharesPath, roundsPath, settlePath} {
		if !reflect.DeepEqual(before[i], hostedAuditFiles(t, p)) {
			t.Fatal("read-only audit changed journal", p)
		}
	}
	db, e := leveldb.OpenFile(filepath.Join(sharesPath, "db"), &opt.Options{ErrorIfMissing: true})
	if e != nil {
		t.Fatal(e)
	}
	raw, _ := json.Marshal("foreign-row")
	if e = db.Put([]byte("foreign"), raw, &opt.WriteOptions{Sync: true}); e != nil {
		t.Fatal(e)
	}
	if e = db.Close(); e != nil {
		t.Fatal(e)
	}
	if s, e := poolshare.OpenAudit(sharesPath, source, issuer); e == nil {
		s.Close()
		t.Fatal("full audit ignored foreign row")
	}
}
