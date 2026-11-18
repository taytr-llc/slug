//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymbout

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymentry"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/registry"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func boutCrashHook() func(string) {
	stage := os.Getenv("SLUG_BOUT_CRASH_STAGE")
	if stage == "" {
		return nil
	}
	return func(s string) {
		if s == stage {
			if e := os.WriteFile(filepath.Join(os.Getenv("SLUG_BOUT_CRASH_ROOT"), "checkpoint"), []byte(s), 0600); e != nil {
				panic(e)
			}
			select {}
		}
	}
}
func TestBoutCrashChild(t *testing.T) {
	if os.Getenv("SLUG_BOUT_CRASH_STAGE") == "" {
		t.Skip("owned subprocess only")
	}
	f := newBoutFixture(t)
	f.s.checkpoint = boutCrashHook()
	if _, e := f.run(context.Background()); e != nil {
		t.Fatal(e)
	}
	t.Fatal("checkpoint not reached")
}
func reopenCrashSources(t *testing.T, dir string) *boutFixture {
	t.Helper()
	ctx := context.Background()
	f := &boutFixture{dir: dir}
	var cp struct{ Value gymcard.Pin }
	raw, e := os.ReadFile("testdata/reference/inputs/card46/pin.json")
	if e != nil {
		t.Fatal(e)
	}
	json.Unmarshal(raw, &cp)
	f.p, e = CandidatePin(cp.Value)
	if e != nil {
		t.Fatal(e)
	}
	raw, e = os.ReadFile("testdata/reference/inputs/card46/modeled-five-class-seed.json")
	if e != nil {
		t.Fatal(e)
	}
	json.Unmarshal(raw, &f.seedRecord)
	f.now.Store(f.seedRecord.SourceReceipt.Input.Event.CutoffMS + 1000000)
	ep := cp.Value.SourcePin.SourcePin
	part := ep.Participants[0]
	sp := part.StablePin
	f.source, e = registry.OpenManifestAuthority(filepath.Join(dir, "source"), sp.Source, false)
	if e != nil {
		t.Fatal(e)
	}
	f.stable, e = gymstable.OpenForTestClock(filepath.Join(dir, "stable"), sp, f.source, f.clock)
	if e != nil {
		t.Fatal(e)
	}
	f.entry, e = gymentry.OpenForTestClock(ctx, filepath.Join(dir, "entry"), part, f.stable, f.clock)
	if e != nil {
		t.Fatal(e)
	}
	f.event, e = gymevent.OpenForTestClock(ctx, filepath.Join(dir, "event"), ep, f.clock)
	if e != nil {
		t.Fatal(e)
	}
	f.seed, e = gymseed.OpenForTestClock(ctx, filepath.Join(dir, "seed"), cp.Value.SourcePin, f.event, f.clock)
	if e != nil {
		t.Fatal(e)
	}
	f.card, e = gymcard.OpenForTestClock(ctx, filepath.Join(dir, "card"), cp.Value, f.seed, f.clock)
	if e != nil {
		t.Fatal(e)
	}
	f.session = gymstable.Session{Generation: 1, ViewID: strings.Repeat("a", 64), Lease: func(ctx context.Context, cap gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
		if cap != gymaccount.View {
			return gymaccount.ErrForbidden
		}
		identity := sp.Identity
		e := fn(gymaccount.Account{ID: sp.AccountID, Generation: 1, Enrollment: &identity}, ctx.Err)
		if e != nil {
			return e
		}
		return ctx.Err()
	}}
	card, e := f.card.Read(ctx, f.seed, f.event, f.entry, f.session, f.date())
	if e != nil {
		t.Fatal(e)
	}
	f.cardRecord = card.Record
	t.Cleanup(func() {
		for _, s := range []interface{ Close() error }{f.s, f.card, f.seed, f.event, f.entry, f.stable, f.source} {
			if s != nil {
				s.Close()
			}
		}
	})
	return f
}
func TestBoutActualSIGKILLRecovery(t *testing.T) {
	requireNormal(t)
	for _, stage := range []string{"marker-synced", "init-synced", "initialized", "before-result-sync", "result-synced"} {
		t.Run(stage, func(t *testing.T) {
			dir := realTemp(t)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestBoutCrashChild$", "-test.timeout=18s")
			cmd.Env = append(os.Environ(), "SLUG_BOUT_CRASH_ROOT="+dir, "SLUG_BOUT_CRASH_STAGE="+stage)
			log, e := os.Create(filepath.Join(dir, "child.log"))
			if e != nil {
				t.Fatal(e)
			}
			defer log.Close()
			cmd.Stdout = log
			cmd.Stderr = log
			if e = cmd.Start(); e != nil {
				t.Fatal(e)
			}
			wait := make(chan error, 1)
			go func() { wait <- cmd.Wait() }()
			for {
				if raw, e := os.ReadFile(filepath.Join(dir, "checkpoint")); e == nil && string(raw) == stage {
					break
				}
				select {
				case e := <-wait:
					raw, _ := os.ReadFile(filepath.Join(dir, "child.log"))
					t.Fatalf("child exited %v: %s", e, raw)
				case <-ctx.Done():
					t.Fatal("checkpoint deadline")
				case <-time.After(10 * time.Millisecond):
				}
			}
			if e = cmd.Process.Kill(); e != nil {
				t.Fatal(e)
			}
			if e = <-wait; e == nil {
				t.Fatal("expected actual killed subprocess")
			}
			f := reopenCrashSources(t, dir)
			path := filepath.Join(dir, "bout")
			before := stage == "marker-synced" || stage == "init-synced"
			if before {
				if _, e = open(context.Background(), path, f.p, f.card, f.clock, false); !errors.Is(e, ErrCorrupt) {
					t.Fatal("ready accepted partial", e)
				}
				f.s, e = open(context.Background(), path, f.p, f.card, f.clock, true)
			} else {
				f.s, e = open(context.Background(), path, f.p, f.card, f.clock, false)
			}
			if e != nil {
				t.Fatal("recovery", e)
			}
			info, e := f.s.Info(context.Background())
			if e != nil {
				t.Fatal(e)
			}
			want := uint32(0)
			if stage == "result-synced" {
				want = 1
			}
			if info.BoutCount != want {
				t.Fatal("atomic result count", info.BoutCount, want)
			}
			if before && info.CreatedAtMS != f.now.Load() {
				t.Fatal("resumed actual creation backdated")
			}
			if !before && info.CreatedAtMS >= f.now.Load() {
				t.Fatal("original creation replaced")
			}
			engine := 0
			f.s.checkpoint = func(s string) {
				if s == "before-engine" {
					engine++
				}
			}
			r, e := f.run(context.Background())
			if e != nil {
				t.Fatal(e)
			}
			if r.Replayed != (want == 1) || engine != int(1-want) {
				t.Fatal("exact replay versus uncommitted preparation", r.Replayed, engine)
			}
			t.Logf("SIGKILL joined at %s; recovered count=%d; replay=%v", stage, want, r.Replayed)
		})
	}
}
