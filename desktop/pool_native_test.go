package main

import (
	"github.com/taytr-llc/slug/desktop/internal/bridge"
	"github.com/taytr-llc/slug/desktop/pilot"
	"os"
	"path/filepath"
	"testing"
)

func TestPoolNativeModeRefusesSoloRecoveryBeforeDialogs(t *testing.T) {
	g := &Gym{poolHelper: &bridge.PoolBridge{}}
	if g.Mode() != pilot.PoolMode {
		t.Fatal("pool mode not explicit")
	}
	if _, e := g.ExportRecovery(); e == nil {
		t.Fatal("pool exported old solo recovery")
	}
	if _, e := g.VerifyBackup(); e == nil {
		t.Fatal("pool opened old restore flow")
	}
}
func TestPoolPathSeparateFromCandidateNamespace(t *testing.T) {
	parent := candidateTestRoot(t)
	pool := filepath.Join(parent, "LocalPoolPilot")
	if e := preparePoolDataParent(pool, parent, false); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(pool); !os.IsNotExist(e) {
		t.Fatal("native preflight created helper root")
	}
	if e := os.Mkdir(pool, 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(pool, "SLUG-POOL-ACCOUNTS.json"), []byte("backend owns this marker"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := preparePoolDataParent(pool, parent, false); e != nil {
		t.Fatal("own marker refused before backend validation", e)
	}
	if e := prepareCandidateDataParent(pool, parent, false); e == nil {
		t.Fatal("old candidate mode adopted pool root")
	}
	old := filepath.Join(parent, "LocalManifestPilot")
	if e := os.Mkdir(old, 0700); e != nil {
		t.Fatal(e)
	}
	if e := preparePoolDataParent(old, parent, false); e == nil {
		t.Fatal("pool mode adopted old namespace")
	}
	link := filepath.Join(parent, "alias")
	if e := os.Symlink(pool, link); e != nil {
		t.Fatal(e)
	}
	if e := preparePoolDataParent(link, parent, false); e == nil {
		t.Fatal("pool symlink accepted")
	}
}

func TestHostedPoolNativeNamespaceNeverAdoptsLegacy(t *testing.T) {
	parent := candidateTestRoot(t)
	root := filepath.Join(parent, "LocalHostedPoolPilot")
	if err := prepareHostedPoolDataParent(root, parent); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatal("native preflight created helper root")
	}
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"SLUG-HOSTED-POOL-INIT.json", "SLUG-HOSTED-POOL-READY.json"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("helper validates exact contents"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := prepareHostedPoolDataParent(root, parent); err != nil {
		t.Fatal("own hosted markers refused before helper audit", err)
	}
	if err := preparePoolDataParent(root, parent, false); err == nil {
		t.Fatal("old pool selected hosted root")
	}
	if err := prepareCandidateDataParent(root, parent, false); err == nil {
		t.Fatal("old candidate selected hosted root")
	}
	custom := filepath.Join(parent, "custom-old")
	if err := os.Mkdir(custom, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(custom, "SLUG-POOL-ACCOUNTS.json"), []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{custom, filepath.Join(custom, "nested"), filepath.Join(root, "nested")} {
		if err := prepareHostedPoolDataParent(path, parent); err == nil {
			t.Fatal("hosted preflight accepted managed overlap", path)
		}
	}
}
