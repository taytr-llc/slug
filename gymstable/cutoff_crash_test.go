// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"bytes"
	"context"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func cutoffCrashPin(t *testing.T) Pin {
	t.Helper()
	f := readCoreCorpus(t)
	for _, p := range f.Pins {
		if p.Name == "zero256" {
			return p.Pin
		}
	}
	t.Fatal("pin")
	return Pin{}
}
func TestStableCutoffFenceCrashHelper(t *testing.T) {
	dir := os.Getenv("SLUG_STABLE_CUTOFF_CRASH")
	if dir == "" {
		return
	}
	if wire.TestPoW {
		t.Fatal("normal profile required")
	}
	p := cutoffCrashPin(t)
	sourceRows(t, filepath.Join(dir, "source"), p, nil)
	src, e := registry.OpenManifestAuthority(filepath.Join(dir, "source"), p.Source, false)
	if e != nil {
		t.Fatal(e)
	}
	defer src.Close()
	now := int64(1795000000000)
	s, e := create(filepath.Join(dir, "stable"), p, src, func() time.Time { return time.UnixMilli(now) }, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	now += 100
	c := now
	session := fixtureSession(p, 1, strings.Repeat("a", 64))
	e = s.WithCutoffSelection(context.Background(), session, c, nil, func(CutoffSelection) (func(CutoffFence, func() error) error, error) {
		now++
		return func(f CutoffFence, check func() error) error {
			if f.ObservedAtMS != now || s.meta.ClockMS != now {
				t.Fatal("foreign callback before durable fence")
			}
			if e := os.WriteFile(filepath.Join(dir, "barrier"), []byte("stable-fence-synced"), 0600); e != nil {
				t.Fatal(e)
			}
			<-time.After(time.Hour)
			return check()
		}, nil
	})
	t.Fatal("barrier returned", e)
}
func TestStableCutoffActualKillAfterFenceSync(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal profile")
	}
	dir := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestStableCutoffFenceCrashHelper$", "-test.count=1", "-test.timeout=30s")
	cmd.Env = append(os.Environ(), "SLUG_STABLE_CUTOFF_CRASH="+dir)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if e := cmd.Start(); e != nil {
		t.Fatal(e)
	}
	joined := make(chan error, 1)
	go func() { joined <- cmd.Wait() }()
	stopped := false
	defer func() {
		if !stopped {
			cmd.Process.Kill()
			<-joined
		}
	}()
	deadline := time.NewTimer(10 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
wait:
	for {
		select {
		case e := <-joined:
			stopped = true
			t.Fatal(e, out.String())
		case <-deadline.C:
			t.Fatal("fence barrier timeout")
		case <-tick.C:
			if _, e := os.Stat(filepath.Join(dir, "barrier")); e == nil {
				break wait
			}
		}
	}
	if e := cmd.Process.Kill(); e != nil {
		t.Fatal(e)
	}
	if e := <-joined; e == nil {
		t.Fatal("expected SIGKILL")
	}
	stopped = true
	p := cutoffCrashPin(t)
	src, e := registry.OpenManifestAuthority(filepath.Join(dir, "source"), p.Source, false)
	if e != nil {
		t.Fatal(e)
	}
	defer src.Close()
	s, e := Open(filepath.Join(dir, "stable"), p, src)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	if s.meta.ClockMS != 1795000000101 || s.meta.OperationCount != 0 || s.meta.RosterRevision != 0 {
		t.Fatal("fence recovery", s.meta.ClockMS)
	}
	s.now = func() time.Time { return time.UnixMilli(1795000000100) }
	if _, e = s.Status(context.Background(), fixtureSession(p, 1, strings.Repeat("b", 64))); e != ErrClock {
		t.Fatal("backdating permitted", e)
	}
}
