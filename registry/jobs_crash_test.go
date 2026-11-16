//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package registry

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
)

const (
	birthChildMode = "SLUG_BIRTH_TEST_MODE"
	birthChildPath = "SLUG_BIRTH_TEST_PATH"
	birthChildACK  = "SLUG_BIRTH_SYNCHRONIZED:"
)

// TestBirthSubprocessCrashHelper deliberately omits all cleanup after its ACK.
// The parent kills the process (SIGKILL on Unix), so a deferred database close
// cannot supply the durability being tested. Only public fixture data is used.
func TestBirthSubprocessCrashHelper(t *testing.T) {
	mode := os.Getenv(birthChildMode)
	if mode == "" {
		return
	}
	if mode != "issue" && mode != "birth" && mode != "expired-observation" {
		t.Fatalf("unknown birth child mode %q", mode)
	}
	f := birthFixture(t)
	store, err := CreateBirth(os.Getenv(birthChildPath), f.Context.Network, f.Context.Genesis)
	if err != nil {
		t.Fatal(err)
	}
	now := f.Job.IssuedAtMS
	store.now = func() time.Time { return time.UnixMilli(now) }
	challenge, err := hex.DecodeString(f.Job.Challenge)
	if err != nil {
		t.Fatal(err)
	}
	store.random = bytes.NewReader(challenge)
	issueFixtureJob(t, store, f)
	switch mode {
	case "birth":
		now += 25
		record, inserted, err := store.RecordJob(f.Proof, f.Authorization, acceptJob)
		if err != nil || !inserted || record.Birth == nil || record.Birth.AcceptedAtMS != now {
			t.Fatalf("child birth admission: %+v, %v, %v", record, inserted, err)
		}
	case "expired-observation":
		now = f.Job.ExpiresAtMS
		record, inserted, err := store.RecordJob(f.Proof, f.Authorization, acceptJob)
		if !errors.Is(err, ErrJobExpired) || inserted || !reflect.DeepEqual(record, Record{}) || store.meta.LastClockMS != now {
			t.Fatalf("child expired observation: %+v, %v, %v", record, inserted, err)
		}
	}
	fmt.Println(birthChildACK + mode)
	for {
		time.Sleep(time.Second)
		if store == nil {
			t.Fatal("store disappeared")
		}
	}
}

func killBirthChildAfterAcknowledgment(t *testing.T, mode, path string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestBirthSubprocessCrashHelper$", "-test.v")
	command.Env = append(os.Environ(), birthChildMode+"="+mode, birthChildPath+"="+path)
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if command.ProcessState == nil {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
	}()
	acknowledged := false
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		if scanner.Text() == birthChildACK+mode {
			acknowledged = true
			break
		}
	}
	if !acknowledged {
		_ = command.Wait()
		t.Fatalf("child did not acknowledge synchronized %s: %v\n%s", mode, scanner.Err(), &stderr)
	}
	if err := command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := command.Wait(); err == nil {
		t.Fatal("crash child exited cleanly instead of being killed")
	}
	if ctx.Err() != nil {
		t.Fatalf("crash test exceeded deadline: %v", ctx.Err())
	}
}

func TestSynchronizedJobAndBirthSurviveProcessKill(t *testing.T) {
	f := birthFixture(t)
	for _, mode := range []string{"issue", "birth", "expired-observation"} {
		t.Run(mode, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "birth")
			killBirthChildAfterAcknowledgment(t, mode, path)
			store, err := Open(path, false)
			if err != nil {
				t.Fatalf("recover synchronized %s after process kill: %v", mode, err)
			}
			defer store.Close()
			job, err := store.GetJob(f.Job.ID)
			if err != nil || job != f.Job || store.meta.JobCount != 1 {
				t.Fatalf("lost or changed synchronized issuance: %+v, %v", job, err)
			}
			switch mode {
			case "issue":
				if store.meta.LastClockMS != f.Job.IssuedAtMS || store.meta.LastSequence != 0 {
					t.Fatal("issuance clock or receipt sequence did not survive")
				}
				if _, err := store.Get(f.Expected.ProspectID); !errors.Is(err, ErrNotFound) {
					t.Fatal("issuance alone created a birth receipt")
				}
				store.now = func() time.Time { return time.UnixMilli(f.Job.IssuedAtMS + 50) }
				record, inserted, err := store.RecordJob(f.Proof, f.Authorization, acceptJob)
				if err != nil || !inserted || record.Sequence != 1 || record.Birth.BornAtMS != f.Job.IssuedAtMS {
					t.Fatalf("recovered issuance could not admit original work: %+v, %v, %v", record, inserted, err)
				}
			case "birth":
				record, err := store.Get(f.Expected.ProspectID)
				if err != nil || record.Birth == nil || record.Sequence != 1 || record.Verified != f.Expected ||
					record.Birth.BornAtMS != f.Job.IssuedAtMS || record.Birth.AcceptedAtMS != f.Job.IssuedAtMS+25 ||
					store.meta.LastClockMS != f.Job.IssuedAtMS+25 {
					t.Fatalf("lost synchronized birth: %+v, %v", record, err)
				}
				// Replays recover historical evidence even after expiry and an
				// unavailable node. Neither callback is permitted to run here.
				store.now = func() time.Time {
					t.Fatal("historical replay consulted the clock")
					return time.UnixMilli(f.Job.ExpiresAtMS + 1)
				}
				replayed, inserted, err := store.RecordJob(f.Proof, f.Authorization, func(prospect.Context, prospect.Proof) error {
					t.Fatal("historical replay consulted the node")
					return errors.New("node unavailable")
				})
				if err != nil || inserted || !reflect.DeepEqual(replayed, record) {
					t.Fatalf("crash recovery changed original birth: %+v, %v, %v", replayed, inserted, err)
				}
			case "expired-observation":
				if store.meta.LastClockMS != f.Job.ExpiresAtMS || store.meta.LastSequence != 0 {
					t.Fatal("rejected expiry observation was lost or allocated a birth")
				}
				store.now = func() time.Time { return time.UnixMilli(f.Job.ExpiresAtMS - 1) }
				record, inserted, err := store.RecordJob(f.Proof, f.Authorization, acceptJob)
				if !errors.Is(err, ErrClock) || inserted || !reflect.DeepEqual(record, Record{}) {
					t.Fatalf("process restart reopened an expired job after rollback: %+v, %v, %v", record, inserted, err)
				}
			}
		})
	}
}

func TestBirthStoredJobAndClockCorruptionFailsClosed(t *testing.T) {
	f := birthFixture(t)
	tests := []struct {
		name       string
		editRecord func(*Record)
		editJob    func(*prospectjob.Job)
		editMeta   func(*metadata)
		deleteJob  bool
	}{
		{name: "missing issued job", deleteJob: true, editMeta: func(m *metadata) { m.JobCount = 0 }},
		{name: "mismatched stored job ID", editJob: func(j *prospectjob.Job) { j.ID = strings.Repeat("0", 64) }},
		{name: "changed stored challenge", editJob: func(j *prospectjob.Job) { j.Challenge = strings.Repeat("f", 64) }},
		{name: "changed stored issuance", editJob: func(j *prospectjob.Job) { j.IssuedAtMS++ }},
		{name: "missing receipt job", editRecord: func(r *Record) { r.Job = nil }},
		{name: "missing receipt birth", editRecord: func(r *Record) { r.Birth = nil }},
		{name: "mismatched receipt job", editRecord: func(r *Record) { r.Job.ExpiresAtMS++ }},
		{name: "changed birth anchor", editRecord: func(r *Record) { r.Birth.BornAtMS++ }},
		{name: "changed birth rule", editRecord: func(r *Record) { r.Birth.RuleVersion++ }},
		{name: "accepted at expiry", editRecord: func(r *Record) { r.Birth.AcceptedAtMS = f.Job.ExpiresAtMS }},
		{name: "accepted before issuance", editRecord: func(r *Record) { r.Birth.AcceptedAtMS = f.Job.IssuedAtMS - 1 }},
		{name: "context differs from job", editRecord: func(r *Record) { r.Context.MaxTimestamp++ }},
		{name: "proof references other job", editRecord: func(r *Record) { r.Proof.JobID = strings.Repeat("0", 64) }},
		{name: "clock below accepted birth", editMeta: func(m *metadata) { m.LastClockMS = f.Job.IssuedAtMS + 24 }},
		{name: "clock below issued job", editMeta: func(m *metadata) { m.LastClockMS = f.Job.IssuedAtMS - 1 }},
		{name: "negative clock metadata", editMeta: func(m *metadata) { m.LastClockMS = -1 }},
		{name: "job count mismatch", editMeta: func(m *metadata) { m.JobCount = 0 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store, path, now := newBirthStore(t, f)
			issueFixtureJob(t, store, f)
			*now += 25
			record, inserted, err := store.RecordJob(f.Proof, f.Authorization, acceptJob)
			if err != nil || !inserted {
				t.Fatalf("seed production birth: %+v, %v", record, err)
			}
			meta := store.meta
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			// Alter the closed database as an external corruption scenario;
			// these writes are deliberately outside the immutable registry API.
			db, err := leveldb.OpenFile(filepath.Join(path, databaseName), &opt.Options{ErrorIfMissing: true, Strict: opt.StrictAll})
			if err != nil {
				t.Fatal(err)
			}
			batch := new(leveldb.Batch)
			if test.deleteJob {
				batch.Delete(jobKey(f.Job.ID))
			}
			if test.editJob != nil {
				job := f.Job
				test.editJob(&job)
				raw, err := json.Marshal(job)
				if err != nil {
					t.Fatal(err)
				}
				batch.Put(jobKey(f.Job.ID), raw)
			}
			if test.editRecord != nil {
				test.editRecord(&record)
				raw, err := json.Marshal(record)
				if err != nil {
					t.Fatal(err)
				}
				batch.Put(recordKey(f.Expected.ProspectID), raw)
			}
			if test.editMeta != nil {
				test.editMeta(&meta)
				raw, err := json.Marshal(meta)
				if err != nil {
					t.Fatal(err)
				}
				batch.Put([]byte(metadataKey), raw)
			}
			writeErr := db.Write(batch, &opt.WriteOptions{Sync: true})
			closeErr := db.Close()
			if writeErr != nil || closeErr != nil {
				t.Fatalf("install corruption: write=%v close=%v", writeErr, closeErr)
			}
			reopened, err := Open(path, false)
			if reopened != nil {
				_ = reopened.Close()
			}
			if !errors.Is(err, ErrCorrupt) || reopened != nil {
				t.Fatalf("corrupt birth registry opened: store=%v err=%v", reopened, err)
			}
		})
	}
}

func TestBirthFinalGuardCannotMutateVerifiedProof(t *testing.T) {
	raw, err := os.ReadFile("../prospectjob/testdata/jobs_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []jobFixture
	if err := json.Unmarshal(raw, &fixtures); err != nil || len(fixtures) < 2 || len(fixtures[1].Proof.MerkleBranch) == 0 {
		t.Fatalf("independent nonempty-branch fixture: %v", err)
	}
	f := fixtures[1]
	store, _, now := newBirthStore(t, f)
	issueFixtureJob(t, store, f)
	*now += 25
	guard := func(_ prospect.Context, proof prospect.Proof) error {
		// The callback is an independent consumer of a proof snapshot. It must
		// not be able to alter evidence already checked before admission.
		proof.MerkleBranch[0] = strings.Repeat("0", 64)
		return nil
	}
	record, inserted, err := store.RecordJob(f.Proof, f.Authorization, guard)
	if err != nil || !inserted || !reflect.DeepEqual(record.Proof, f.Proof) {
		t.Fatalf("final guard mutated acknowledged verified evidence: %+v, %v, %v", record, inserted, err)
	}
	read, err := store.Get(f.Expected.ProspectID)
	if err != nil || !reflect.DeepEqual(read, record) {
		t.Fatalf("acknowledged receipt failed re-verification after callback: %+v, %v", read, err)
	}
}
