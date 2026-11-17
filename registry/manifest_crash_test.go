//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package registry

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/taytr-llc/slug/prospect"
)

const manifestCrashMode = "SLUG_MANIFEST_TEST_CRASH_MODE"
const manifestCrashPath = "SLUG_MANIFEST_TEST_CRASH_PATH"
const manifestCrashACK = "SLUG_MANIFEST_CHECKPOINT:"

func TestManifestSubprocessCrashHelper(t *testing.T) {
	mode := os.Getenv(manifestCrashMode)
	if mode == "" {
		return
	}
	f := readManifestCorpus(t)
	random, err := hex.DecodeString(f.Random)
	if err != nil {
		t.Fatal(err)
	}
	reader := bytes.NewReader(random)
	stage := mode
	if mode == "expiry-clock" {
		stage = "clock"
	}
	checkpoint := func(at string) {
		if at == stage {
			fmt.Println(manifestCrashACK + mode)
			select {}
		}
	}
	s, err := createManifestAuthority(os.Getenv(manifestCrashPath), f.Pin.Network, f.Pin.Genesis, f.Pin.DescriptorID, reader, checkpoint)
	if err != nil {
		t.Fatal(err)
	}
	// Deliberately no deferred Close: parent kills at the durable/lost-ACK
	// boundary so cleanup cannot manufacture the tested persistence.
	now := f.Issued
	s.now = func() time.Time { return time.UnixMilli(now) }
	s.random = reader
	for _, j := range f.Jobs {
		if _, err := s.IssueManifestJob(j.Job.Context, j.Job.PublicKey, func() error { return nil }); err != nil {
			t.Fatal(err)
		}
	}
	tc := f.Cases[0]
	now = tc.Accepted
	if mode == "expiry-clock" {
		now = tc.Job.ExpiresAtMS
	}
	if _, _, err := s.RecordManifestJobChecked(tc.Proof, tc.Authorization, manifestGuard(tc)); err != nil {
		t.Fatal(err)
	}
	t.Fatal("requested checkpoint was not reached", mode)
}

func killManifestChild(t *testing.T, mode, path string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestManifestSubprocessCrashHelper$", "-test.v")
	cmd.Env = append(os.Environ(), manifestCrashMode+"="+mode, manifestCrashPath+"="+path)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if cmd.ProcessState == nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()
	scanner := bufio.NewScanner(stdout)
	ack := false
	for scanner.Scan() {
		if scanner.Text() == manifestCrashACK+mode {
			ack = true
			break
		}
	}
	if !ack {
		_ = cmd.Wait()
		t.Fatalf("missing crash checkpoint %s: %v %s", mode, scanner.Err(), stderr.String())
	}
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err == nil {
		t.Fatal("crash child exited cleanly")
	}
	if ctx.Err() != nil {
		t.Fatal("crash child exceeded bounded deadline", ctx.Err())
	}
}

func TestManifestRealProcessKillDurability(t *testing.T) {
	f := readManifestCorpus(t)
	for _, mode := range []string{"marker", "initialized", "before-issue", "issued", "clock", "bound", "before-admission", "admitted", "expiry-clock"} {
		t.Run(mode, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "authority")
			killManifestChild(t, mode, path)
			pin, err := InspectManifestAuthority(path)
			if err != nil || pin != f.Pin {
				t.Fatal("lost create ACK inspection", pin, err)
			}
			if mode == "marker" {
				before, _ := os.ReadDir(path)
				s, err := OpenManifestAuthority(path, pin, false)
				if err == nil || s != nil {
					t.Fatal("incomplete initialization opened")
				}
				after, _ := os.ReadDir(path)
				if len(before) != len(after) {
					t.Fatal("Open repaired incomplete directory")
				}
				return
			}
			s, err := OpenManifestAuthority(path, pin, false)
			if err != nil {
				t.Fatal("reopen after abrupt kill", err)
			}
			defer s.Close()
			info, err := s.Info()
			if err != nil {
				t.Fatal(err)
			}
			expectedJobs := uint64(2)
			if mode == "initialized" || mode == "before-issue" {
				expectedJobs = 0
			}
			if mode == "issued" {
				expectedJobs = 1
			}
			if info.Jobs != expectedJobs {
				t.Fatal("lost synchronized jobs", mode, info)
			}
			for i := uint64(0); i < expectedJobs; i++ {
				got, err := s.GetManifestJob(f.Jobs[i].Job.ID)
				if err != nil || got != f.Jobs[i].Job {
					t.Fatal("job changed after kill", err)
				}
			}
			tc := f.Cases[0]
			if mode == "admitted" {
				if info.Records != 1 {
					t.Fatal("lost atomic admission", info)
				}
				r, err := s.GetManifestAdmission(tc.Verified.ProspectID)
				if err != nil || !reflect.DeepEqual(r, tc.Record) {
					t.Fatal("partial receipt/binding after kill", err)
				}
				s.now = func() time.Time { t.Fatal("lost ACK recovery consulted clock"); return time.Time{} }
				r, inserted, err := s.RecordManifestJobChecked(tc.Proof, tc.Authorization, func(prospect.Context, prospect.Proof, prospect.Verified) (Eligibility, error) {
					t.Fatal("lost ACK recovery consulted node")
					return Eligibility{}, nil
				})
				if err != nil || inserted || !reflect.DeepEqual(r, tc.Record) {
					t.Fatal("lost ACK recovery changed receipt", err)
				}
			} else {
				if info.Records != 0 {
					t.Fatal("uncommitted receipt became visible", mode, info)
				}
				if _, err := s.GetManifestAdmission(tc.Verified.ProspectID); !errors.Is(err, ErrNotFound) {
					t.Fatal("unexpected receipt", err)
				}
				if mode == "expiry-clock" {
					if s.meta.LastClockMS != tc.Job.ExpiresAtMS {
						t.Fatal("lost rejected clock")
					}
					s.now = func() time.Time { return time.UnixMilli(tc.Job.ExpiresAtMS - 1) }
					if _, _, err := s.RecordManifestJobChecked(tc.Proof, tc.Authorization, manifestGuard(tc)); !errors.Is(err, ErrClock) {
						t.Fatal("crash reopened expired job", err)
					}
				}
			}
		})
	}
}
