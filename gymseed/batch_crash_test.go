//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymseed

import (
	"context"
	"errors"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymevent"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// These are actual process kills across real Sync batches, with genuine source
// receipts but explicitly synthetic transaction/chain observations. They do not
// replace the separately owned actual-node dispatch/confirmation acceptance.
func TestSeedBatchCrashChild(t *testing.T) {
	stage := os.Getenv("SLUG_SEED_BATCH_STAGE")
	if stage == "" {
		t.Skip("owned subprocess only")
	}
	x, s, _ := seedFixture(t)
	ctx := context.Background()
	r, e := s.Prepare(ctx, x.s, x.entry[0], x.session[0], "2026-11-21")
	if e != nil {
		t.Fatal(e)
	}
	real := s.writeBatch
	checkpoint := func() {
		if e := os.WriteFile(filepath.Join(filepath.Dir(x.path), "barrier"), []byte(stage), 0600); e != nil {
			t.Fatal(e)
		}
		for {
			time.Sleep(time.Second)
		}
	}
	save := func(name string, b *leveldb.Batch) {
		s.writeBatch = func(batch *leveldb.Batch, options *opt.WriteOptions) error {
			if strings.HasPrefix(stage, name+"-") {
				if strings.HasSuffix(stage, "-before") {
					checkpoint()
				}
				if e := real(batch, options); e != nil {
					return e
				}
				checkpoint()
			}
			return real(batch, options)
		}
		work, done, e := s.begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		r, e = s.saveAction(work, x.s, x.entry[0], x.session[0], gymaccount.View, r, s.meta, b, nil)
		done()
		if e != nil {
			t.Fatal(e)
		}
	}
	x.now += 10
	pub := syntheticPublication(t, s.meta.Pin, r)
	pub.BuiltAtMS = x.now
	r.Publication = &pub
	reservation := new(leveldb.Batch)
	reservation.Put(coinKey('u', pub.Coin.Outpoint), rawID(r.EventKey))
	save("publication", reservation)
	x.now += 10
	r.Publication.DispatchCount = 1
	r.Publication.State = "unknown"
	r.Publication.FirstDispatchAtMS = x.now
	r.Publication.LastDispatchAtMS = x.now
	save("dispatch", nil)
	if strings.HasPrefix(stage, "finality-") {
		x.now += 10
		r = syntheticFinality(t, s.meta.Pin, r, x.now)
		save("finality", nil)
	}
	if strings.HasPrefix(stage, "incident-") {
		x.now = r.SourceReceipt.Input.Event.IncidentDeadlineMS
		save("incident", nil)
	}
	t.Fatal("missed batch kill stage")
}
func TestSeedAtomicBatchProcessKills(t *testing.T) {
	requireNormal(t)
	for _, stage := range []string{"publication-before", "publication-after", "dispatch-before", "dispatch-after", "finality-before", "finality-after", "incident-before", "incident-after"} {
		t.Run(stage, func(t *testing.T) {
			root := realTemp(t)
			cmd := exec.Command(os.Args[0], "-test.run=^TestSeedBatchCrashChild$", "-test.count=1", "-test.timeout=50s")
			cmd.Env = append(os.Environ(), "SLUG_SEED_TEST_ROOT="+root, "SLUG_SEED_BATCH_STAGE="+stage)
			log, e := os.Create(filepath.Join(root, "child.log"))
			if e != nil {
				t.Fatal(e)
			}
			defer log.Close()
			cmd.Stdout = log
			cmd.Stderr = log
			if e = cmd.Start(); e != nil {
				t.Fatal(e)
			}
			joined := false
			t.Cleanup(func() {
				if !joined {
					_ = cmd.Process.Kill()
					_ = cmd.Wait()
				}
			})
			deadline := time.Now().Add(40 * time.Second)
			for {
				raw, e := os.ReadFile(filepath.Join(root, "barrier"))
				if e == nil && string(raw) == stage {
					break
				}
				if time.Now().After(deadline) {
					raw, _ := os.ReadFile(log.Name())
					t.Fatalf("batch barrier timeout: %s", raw)
				}
				time.Sleep(10 * time.Millisecond)
			}
			if e = cmd.Process.Kill(); e != nil {
				t.Fatal(e)
			}
			e = cmd.Wait()
			joined = true
			var exit *exec.ExitError
			if !errors.As(e, &exit) {
				t.Fatal(e)
			}
			status, ok := exit.Sys().(syscall.WaitStatus)
			if !ok || !status.Signaled() || status.Signal() != syscall.SIGKILL {
				t.Fatal("not SIGKILL")
			}
			ep := eventPin(t)
			ev, _ := gymevent.EventForDate(ep, "2026-11-21")
			clock := func() time.Time { return time.UnixMilli(ev.IncidentDeadlineMS + 1000) }
			events, e := gymevent.OpenForTestClock(context.Background(), filepath.Join(root, "events"), ep, clock)
			if e != nil {
				t.Fatal(e)
			}
			defer events.Close()
			p, _ := CandidatePin(ep)
			s, e := OpenForTestClock(context.Background(), filepath.Join(root, "seeds"), p, events, clock)
			if e != nil {
				t.Fatal("atomic graph reopen", e)
			}
			defer s.Close()
			r, ok, e := s.record(eventKey(p, ev))
			if e != nil || !ok {
				t.Fatal(e)
			}
			wantPub := stage != "publication-before"
			if (r.Publication != nil) != wantPub {
				t.Fatal("publication batch split")
			}
			if wantPub {
				reserved, e := s.db.Has(coinKey('u', r.Publication.Coin.Outpoint), nil)
				if e != nil || !reserved {
					t.Fatal("reservation batch split", e)
				}
				wantDispatch := stage != "publication-after" && stage != "dispatch-before"
				if (r.Publication.DispatchCount == 1) != wantDispatch {
					t.Fatal("dispatch intent batch split")
				}
			}
			if (r.Finality != nil) != (stage == "finality-after") {
				t.Fatal("checkpoint/finality batch split")
			}
			if (r.IncidentAtMS != 0) != (stage == "incident-after") {
				t.Fatal("incident batch split")
			}
			t.Log("real SIGKILL with synthetic chain-state injection", stage)
		})
	}
}
