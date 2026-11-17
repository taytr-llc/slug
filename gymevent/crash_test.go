//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymevent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestEventCrashHelper(t *testing.T) {
	stage := os.Getenv("SLUG_EVENT_KILL_STAGE")
	if stage == "" {
		return
	}
	requireNormal(t)
	path := os.Getenv("SLUG_EVENT_TEST_ROOT")
	if path == "" {
		t.Fatal("missing owned directory")
	}
	hook := func(s string) {
		if s == stage {
			if e := os.WriteFile(filepath.Join(path, "barrier"), []byte(s), 0600); e != nil {
				t.Fatal(e)
			}
			<-time.After(time.Hour)
		}
	}
	switch stage {
	case "marker-synced", "init-synced", "initialized":
		_, e := create(context.Background(), filepath.Join(path, "events"), fixturePin(t), func() time.Time { return time.UnixMilli(1795000000000) }, hook)
		if e != nil {
			t.Fatal(e)
		}
	default:
		x := newEventFixture(t, false)
		x.now = x.cutoff(t)
		participant := 0
		if stage == "second-capture-committed" || stage == "inside-second-capture-write" {
			if _, e := x.s.Capture(context.Background(), x.entry[0], x.session[0], "2026-11-21"); e != nil {
				t.Fatal(e)
			}
			participant = 1
		}
		x.s.checkpoint = func(s string) {
			if stage == "second-capture-committed" && s == "capture-committed" {
				hook(stage)
				return
			}
			if (stage == "inside-capture-write" || stage == "inside-second-capture-write") && s == "before-capture-commit" {
				x.s.writeBatch = func(*leveldb.Batch, *opt.WriteOptions) error {
					hook(stage)
					return errors.New("unexpected barrier return")
				}
				return
			}
			hook(s)
		}
		if _, e := x.s.Capture(context.Background(), x.entry[participant], x.session[participant], "2026-11-21"); e != nil {
			t.Fatal(e)
		}
	}
	t.Fatal("selected kill stage was not reached")
}
func TestEventActualProcessKillRecovery(t *testing.T) {
	requireNormal(t)
	for _, stage := range []string{"marker-synced", "init-synced", "initialized", "reserved", "capture-prepared", "before-capture-commit", "capture-committed", "source-fences-synced", "inside-capture-write", "inside-second-capture-write", "second-capture-committed"} {
		t.Run(stage, func(t *testing.T) {
			dir := realTemp(t)
			cmd := exec.Command(os.Args[0], "-test.run=^TestEventCrashHelper$", "-test.count=1", "-test.timeout=45s")
			cmd.Env = append(os.Environ(), "SLUG_EVENT_KILL_STAGE="+stage, "SLUG_EVENT_TEST_ROOT="+dir)
			var output bytes.Buffer
			cmd.Stdout = &output
			cmd.Stderr = &output
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
			deadline := time.NewTimer(20 * time.Second)
			defer deadline.Stop()
			tick := time.NewTicker(10 * time.Millisecond)
			defer tick.Stop()
		wait:
			for {
				select {
				case e := <-joined:
					stopped = true
					t.Fatalf("child exited before barrier: %v %s", e, output.String())
				case <-deadline.C:
					t.Fatal("barrier timeout")
				case <-tick.C:
					if b, e := os.ReadFile(filepath.Join(dir, "barrier")); e == nil && string(b) == stage {
						break wait
					}
				}
			}
			if e := cmd.Process.Kill(); e != nil {
				t.Fatal(e)
			}
			if e := <-joined; e == nil {
				t.Fatal("kill exited successfully")
			}
			stopped = true
			p := fixturePin(t)
			path := filepath.Join(dir, "events")
			clock := func() time.Time { return time.UnixMilli(1795291200000) }
			partial := stage == "marker-synced" || stage == "init-synced"
			if partial {
				if s, e := OpenForTestClock(context.Background(), path, p, clock); e == nil {
					s.Close()
					t.Fatal("ordinary Open accepted partial")
				}
			}
			var s *Store
			var e error
			if partial {
				s, e = ResumePreparedForTestClock(context.Background(), path, p, clock)
			} else {
				s, e = OpenForTestClock(context.Background(), path, p, clock)
			}
			if e != nil {
				t.Fatal("recovery", e)
			}
			defer s.Close()
			info, e := s.Info(context.Background())
			if e != nil {
				t.Fatal(e)
			}
			switch stage {
			case "marker-synced", "init-synced":
				if info.CreatedAtMS != clock().UnixMilli() || info.EventCount != 0 {
					t.Fatal(info)
				}
			case "initialized":
				if info.CreatedAtMS != 1795000000000 || info.EventCount != 0 {
					t.Fatal(info)
				}
			default:
				if info.EventCount != 1 {
					t.Fatal(info)
				}
				ev, _ := EventForDate(p, "2026-11-21")
				cap, exists, e := s.capture(ev, p.Participants[0].StablePin.Identity.GymID)
				if e != nil {
					t.Fatal(e)
				}
				if exists != (stage == "capture-committed" || stage == "inside-second-capture-write" || stage == "second-capture-committed") {
					t.Fatal("atomic capture recovery", exists)
				}
				if exists && validateParticipantCapture(p, cap) != nil {
					t.Fatal("recovered invalid capture")
				}
				_, second, e := s.capture(ev, p.Participants[1].StablePin.Identity.GymID)
				if e != nil || second != (stage == "second-capture-committed") {
					t.Fatal("second capture atomicity", second, e)
				}
				_, complete, e := s.receipt(ev)
				if e != nil || complete != (stage == "second-capture-committed") {
					t.Fatal("complete receipt atomicity", complete, e)
				}
				if stage == "source-fences-synced" {
					for _, name := range []string{"stable0", "entry0"} {
						db, e := leveldb.OpenFile(filepath.Join(dir, name, "db"), &opt.Options{ErrorIfMissing: true})
						if e != nil {
							t.Fatal(e)
						}
						raw, e := db.Get([]byte("meta"), nil)
						db.Close()
						var meta struct {
							ClockMS int64 `json:"clock_ms"`
						}
						if e != nil || json.Unmarshal(raw, &meta) != nil || meta.ClockMS < ev.CutoffMS {
							t.Fatal("source fence not durable", name, meta, e)
						}
					}
				}
				if other, e := ResumePreparedForTestClock(context.Background(), path, p, clock); e == nil {
					other.Close()
					t.Fatal("active DB unexpectedly reopened")
				}
			}
			if e = s.Close(); e != nil {
				t.Fatal(e)
			}
			if !partial && info.EventCount > 0 {
				if other, e := ResumePreparedForTestClock(context.Background(), path, p, clock); !errors.Is(e, ErrCorrupt) {
					if other != nil {
						other.Close()
					}
					t.Fatal("prepared adoption of event rows", e)
				}
			}
		})
	}
}
