// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/taytr-llc/slug/registry"
)

type crashInput struct {
	Path, Source, Stage, Signal string
	Pin                         Pin
	Now                         int64
}

func TestStableProcessCrash(t *testing.T) {
	if path := os.Getenv("SLUG_STABLE_CRASH_INPUT"); path != "" {
		runStableCrashChild(t, path)
		return
	}
	stages := []string{"before_initialize", "initialized", "before_sync", "after_sync", "before_prepare", "after_prepare", "before_apply", "after_apply", "before_abandon", "after_abandon"}
	for _, stage := range stages {
		t.Run(stage, func(t *testing.T) {
			x := newCoreFixture(t, "zero256", 3)
			if e := x.s.Close(); e != nil {
				t.Fatal(e)
			}
			x.s = nil
			if e := x.source.Close(); e != nil {
				t.Fatal(e)
			}
			x.source = nil
			path := x.path
			if stage == "before_initialize" || stage == "initialized" {
				path = x.path + "-initial"
			}
			input := crashInput{Path: path, Source: x.sourcePath, Stage: stage, Signal: filepath.Join(filepath.Dir(path), "checkpoint"), Pin: x.pin, Now: x.now}
			raw, _ := json.Marshal(input)
			inputPath := filepath.Join(filepath.Dir(path), "child.json")
			if e := os.WriteFile(inputPath, raw, 0600); e != nil {
				t.Fatal(e)
			}
			cmd := exec.Command(os.Args[0], "-test.run=^TestStableProcessCrash$", "-test.timeout=45s")
			cmd.Env = append(os.Environ(), "SLUG_STABLE_CRASH_INPUT="+inputPath)
			output := filepath.Join(filepath.Dir(path), "child-output")
			f, e := os.Create(output)
			if e != nil {
				t.Fatal(e)
			}
			defer f.Close()
			cmd.Stdout = f
			cmd.Stderr = f
			if e = cmd.Start(); e != nil {
				t.Fatal(e)
			}
			exited := make(chan error, 1)
			go func() { exited <- cmd.Wait() }()
			deadline := time.Now().Add(30 * time.Second)
			for {
				if _, e = os.Stat(input.Signal); e == nil {
					break
				}
				select {
				case e := <-exited:
					raw, _ := os.ReadFile(output)
					t.Fatalf("child exited before %s: %v %s", stage, e, raw)
				default:
				}
				if time.Now().After(deadline) {
					_ = cmd.Process.Kill()
					<-exited
					t.Fatal("child checkpoint timeout")
				}
				time.Sleep(10 * time.Millisecond)
			}
			if e = cmd.Process.Kill(); e != nil {
				t.Fatal(e)
			}
			if e = <-exited; e == nil {
				t.Fatal("child was not killed")
			}
			source, e := registry.OpenManifestAuthority(x.sourcePath, x.pin.Source, false)
			if e != nil {
				t.Fatal(e)
			}
			defer source.Close()
			s, e := Open(path, x.pin, source)
			if stage == "before_initialize" {
				if !errors.Is(e, ErrCorrupt) {
					if s != nil {
						s.Close()
					}
					t.Fatal("partial creation adopted", e)
				}
				return
			}
			if e != nil {
				t.Fatal(e)
			}
			defer s.Close()
			s.now = func() time.Time { return time.UnixMilli(x.now) }
			status, e := s.Status(context.Background(), x.session)
			if e != nil {
				t.Fatal(e)
			}
			wantCheckpoint := uint64(3)
			if stage == "initialized" || stage == "before_sync" {
				wantCheckpoint = 0
			}
			if status.Checkpoint != wantCheckpoint {
				t.Fatal(stage, status)
			}
			if stage == "before_prepare" || stage == "after_sync" || stage == "before_sync" || stage == "initialized" {
				if status.OperationCount != 0 {
					t.Fatal(status)
				}
				return
			}
			if status.OperationCount != 1 {
				t.Fatal(status)
			}
			operation, e := s.Operation(context.Background(), x.session, status.LastOperationID)
			if e != nil {
				t.Fatal(e)
			}
			want := "prepared"
			if stage == "after_apply" {
				want = "committed"
			}
			if stage == "after_abandon" {
				want = "abandoned"
			}
			if operation.State != want {
				t.Fatal(stage, operation.State)
			}
			if want == "committed" && (operation.Receipt.Revision != 1 || operation.Receipt.Slots[0] != x.records[0].Verified.ProspectID) {
				t.Fatal(operation)
			}
			fresh := fixtureSession(x.pin, 2, strings.Repeat("b", 64))
			if _, e = s.Operation(context.Background(), fresh, operation.Intent.OperationID); e != nil {
				t.Fatal(e)
			}
			if _, e = s.ApplySlot(context.Background(), fresh, operation.Intent.OperationID); !errors.Is(e, ErrStaleView) {
				t.Fatal("old view executed after crash", e)
			}
		})
	}
}
func runStableCrashChild(t *testing.T, path string) {
	raw, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	var in crashInput
	if e = json.Unmarshal(raw, &in); e != nil {
		t.Fatal(e)
	}
	source, e := registry.OpenManifestAuthority(in.Source, in.Pin.Source, false)
	if e != nil {
		t.Fatal(e)
	}
	defer source.Close()
	hook := func(stage string) {
		if stage == in.Stage {
			if e := os.WriteFile(in.Signal, []byte(stage), 0600); e != nil {
				panic(e)
			}
			select {}
		}
	}
	clock := func() time.Time { return time.UnixMilli(in.Now) }
	if in.Stage == "before_initialize" || in.Stage == "initialized" {
		s, e := create(in.Path, in.Pin, source, clock, hook)
		if e != nil {
			t.Fatal(e)
		}
		s.Close()
		t.Fatal("creation checkpoint missed")
	}
	s, e := Open(in.Path, in.Pin, source)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	s.now = clock
	session := fixtureSession(in.Pin, 1, strings.Repeat("a", 64))
	ctx := context.Background()
	if in.Stage == "before_sync" || in.Stage == "after_sync" {
		s.checkpoint = hook
	}
	result, e := s.Sync(ctx, session)
	if e != nil {
		t.Fatal(e)
	}
	if in.Stage == "before_prepare" || in.Stage == "after_prepare" {
		s.checkpoint = hook
	}
	intent, e := s.PrepareSlot(ctx, session, SlotRequest{Slot: 1, ProspectID: result.Items[0].Assignment.ProspectID})
	if e != nil {
		t.Fatal(e)
	}
	s.checkpoint = hook
	if in.Stage == "before_abandon" || in.Stage == "after_abandon" {
		_, e = s.Abandon(ctx, session, intent.OperationID)
	} else {
		_, e = s.ApplySlot(ctx, session, intent.OperationID)
	}
	if e != nil {
		t.Fatal(e)
	}
	t.Fatal("checkpoint missed")
}
