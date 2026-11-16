//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package registry

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/btcec/v2"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
)

type jobFixture struct {
	Job           prospectjob.Job       `json:"job"`
	Context       prospect.Context      `json:"context"`
	Proof         prospect.Proof        `json:"proof"`
	Authorization gymauth.Authorization `json:"authorization"`
	Expected      prospect.Verified     `json:"expected_verified"`
}

func birthFixture(t *testing.T) jobFixture {
	t.Helper()
	raw, err := os.ReadFile("../prospectjob/testdata/jobs_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var values []jobFixture
	if err := json.Unmarshal(raw, &values); err != nil || len(values) == 0 {
		t.Fatalf("independent jobs: %v", err)
	}
	return values[0]
}

func newBirthStore(t *testing.T, f jobFixture) (*Store, string, *int64) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "birth")
	s, err := CreateBirth(path, f.Context.Network, f.Context.Genesis)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	now := f.Job.IssuedAtMS
	s.now = func() time.Time { return time.UnixMilli(now) }
	challenge, _ := hex.DecodeString(f.Job.Challenge)
	s.random = bytes.NewReader(challenge)
	return s, path, &now
}

func issueFixtureJob(t *testing.T, s *Store, f jobFixture) {
	t.Helper()
	job, err := s.IssueJob(f.Context, f.Authorization.PublicKey, func() error { return nil })
	if err != nil || job != f.Job {
		t.Fatalf("issuance differs from independent fixture: %+v %v", job, err)
	}
}

func acceptJob(prospect.Context, prospect.Proof) error { return nil }

func TestJobIssuanceDurabilityAndBoundaries(t *testing.T) {
	f := birthFixture(t)
	s, path, now := newBirthStore(t, f)
	refused := errors.New("node moved")
	if _, err := s.IssueJob(f.Context, f.Authorization.PublicKey, nil); err == nil {
		t.Fatal("nil guard")
	}
	if _, err := s.IssueJob(f.Context, f.Authorization.PublicKey, func() error { return refused }); !errors.Is(err, refused) {
		t.Fatal("guard bypassed")
	}
	if s.meta.JobCount != 0 {
		t.Fatal("failed guard issued job")
	}
	issueFixtureJob(t, s, f)
	if s.meta.JobCount != 1 || s.meta.LastClockMS != *now {
		t.Fatal("job and clock not synchronized")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path, false)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	job, err := s.GetJob(f.Job.ID)
	if err != nil || job != f.Job {
		t.Fatalf("issuance lost on reopen: %+v %v", job, err)
	}
	s.now = func() time.Time { return time.UnixMilli(*now) }
	*now--
	if _, _, err := s.RecordJob(f.Proof, f.Authorization, acceptJob); !errors.Is(err, ErrClock) {
		t.Fatalf("rollback after reopen accepted: %v", err)
	}
	if records, err := s.List(0, 1); err != nil || len(records) != 0 {
		t.Fatal("rollback created birth")
	}
	*now = f.Job.IssuedAtMS
	// Issuance does not consume the job: multiple different headers can qualify.
	s.random = bytes.NewReader(make([]byte, 32))
	second, err := s.IssueJob(f.Context, f.Authorization.PublicKey, func() error { return nil })
	if err != nil || second.ID == f.Job.ID || second.ExpiresAtMS-second.IssuedAtMS != 300000 {
		t.Fatal("fresh challenge did not create fixed lifetime")
	}
	s.random = bytes.NewReader(nil)
	if _, err := s.IssueJob(f.Context, f.Authorization.PublicKey, func() error { return nil }); err == nil {
		t.Fatal("failed random source issued job")
	}
	if s.meta.JobCount != 2 {
		t.Fatal("failed entropy consumed job count")
	}
}

func TestBirthExpiryFinalCheckAndPersistentClock(t *testing.T) {
	f := birthFixture(t)
	for _, test := range []struct {
		name          string
		start, finish int64
		want          error
	}{
		{"last-millisecond", f.Job.ExpiresAtMS - 1, f.Job.ExpiresAtMS - 1, nil},
		{"exact-expiry", f.Job.ExpiresAtMS, f.Job.ExpiresAtMS, ErrJobExpired},
		{"expired-during-check", f.Job.ExpiresAtMS - 1, f.Job.ExpiresAtMS, ErrJobExpired},
		{"clock-rollback-during-check", f.Job.IssuedAtMS + 2, f.Job.IssuedAtMS + 1, ErrClock},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, path, now := newBirthStore(t, f)
			issueFixtureJob(t, s, f)
			*now = test.start
			calls := 0
			guard := func(ctx prospect.Context, proof prospect.Proof) error {
				calls++
				if ctx != f.Context || !reflect.DeepEqual(proof, f.Proof) {
					t.Fatal("guard received claimant context")
				}
				*now = test.finish
				return nil
			}
			got, inserted, err := s.RecordJob(f.Proof, f.Authorization, guard)
			if !errors.Is(err, test.want) {
				t.Fatalf("boundary result: %v, want %v", err, test.want)
			}
			if test.want == nil {
				if !inserted || got.Birth == nil || got.Birth.BornAtMS != f.Job.IssuedAtMS || got.Birth.AcceptedAtMS != test.finish || got.Verified != f.Expected {
					t.Fatalf("birth boundary: %+v", got)
				}
			} else if inserted || !reflect.DeepEqual(got, Record{}) || s.meta.LastSequence != 0 {
				t.Fatal("failed final check created receipt")
			}
			if test.start >= f.Job.ExpiresAtMS && calls != 0 {
				t.Fatal("expired new proof reached node guard")
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			s, err = Open(path, false)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			// A persisted later observation survives reopening, even when the
			// corresponding expired new admission created no birth receipt.
			s.now = func() time.Time { return time.UnixMilli(test.start - 1) }
			s.random = bytes.NewReader(make([]byte, 32))
			if _, err := s.IssueJob(f.Context, f.Authorization.PublicKey, func() error { return nil }); !errors.Is(err, ErrClock) {
				t.Fatalf("reopen forgot high-water time: %v", err)
			}
		})
	}
}

func TestBirthHistoricalReplayAndMultipleProspects(t *testing.T) {
	f := birthFixture(t)
	s, path, now := newBirthStore(t, f)
	issueFixtureJob(t, s, f)
	*now += 50
	first, inserted, err := s.RecordJob(f.Proof, f.Authorization, acceptJob)
	if err != nil || !inserted || first.Version != Version3 {
		t.Fatalf("first birth: %+v %v", first, err)
	}
	// Public fixture scalar 1 only. This mutation tests multiple admissions,
	// not a new golden hash; independent golden proof bytes are tested above.
	proof := f.Proof
	header, _ := hex.DecodeString(proof.Header)
	header[76]++
	proof.Header = hex.EncodeToString(header)
	keyBytes := make([]byte, 32)
	keyBytes[31] = 1
	key, _ := btcec.PrivKeyFromBytes(keyBytes)
	auth, err := gymauth.SignAdmission(f.Context, proof, key)
	if err != nil {
		t.Fatal(err)
	}
	second, inserted, err := s.RecordJob(proof, auth, acceptJob)
	if err != nil || !inserted || second.Sequence != 2 || second.Verified.ProspectID == first.Verified.ProspectID || second.Birth.BornAtMS != first.Birth.BornAtMS {
		t.Fatalf("job incorrectly restricted to one prospect: %+v %v", second, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path, false)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.now = func() time.Time { t.Fatal("historical replay consulted current clock"); return time.Time{} }
	replay, inserted, err := s.RecordJob(f.Proof, f.Authorization, func(prospect.Context, prospect.Proof) error {
		t.Fatal("historical replay consulted reorganized/unavailable node")
		return errors.New("unavailable")
	})
	if err != nil || inserted || !reflect.DeepEqual(first, replay) {
		t.Fatalf("historical birth changed: %+v %v", replay, err)
	}
	replay.Birth.BornAtMS++
	replay.Job.ExpiresAtMS++
	read, err := s.Get(first.Verified.ProspectID)
	if err != nil || !reflect.DeepEqual(first, read) {
		t.Fatal("returned pointer altered persisted birth")
	}
	var exported bytes.Buffer
	if err := s.Export(&exported); err != nil || exported.Len() == 0 {
		t.Fatal("birth export failed")
	}
}

func TestBirthRejectsUnknownJobsRebindingAndLegacyBypass(t *testing.T) {
	f := birthFixture(t)
	s, _, now := newBirthStore(t, f)
	if _, _, err := s.RecordJob(f.Proof, f.Authorization, acceptJob); !errors.Is(err, ErrJobNotFound) {
		t.Fatal("unissued claimant job accepted")
	}
	issueFixtureJob(t, s, f)
	if _, _, err := s.RecordAuthorized(f.Context, f.Proof, f.Authorization); err == nil {
		t.Fatal("untimed method bypassed schema3")
	}
	*now += 1
	s.random = bytes.NewReader(bytes.Repeat([]byte{7}, 32))
	newJob, err := s.IssueJob(f.Context, f.Authorization.PublicKey, func() error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	changed := f.Proof
	changed.JobID = newJob.ID
	keyBytes := make([]byte, 32)
	keyBytes[31] = 1
	key, _ := btcec.PrivKeyFromBytes(keyBytes)
	auth, err := gymauth.SignAdmission(f.Context, changed, key)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.RecordJob(changed, auth, acceptJob); err == nil {
		t.Fatal("old work rebound to newer job despite changed commitment")
	}
	if s.meta.LastSequence != 0 {
		t.Fatal("failed rebinding allocated birth")
	}
	for _, version := range []uint32{Version1, Version2} {
		legacy, err := create(filepath.Join(t.TempDir(), "legacy"), f.Context.Network, f.Context.Genesis, version)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := legacy.RecordAuthorized(f.Context, f.Proof, f.Authorization); err == nil {
			t.Fatal("v2 proof entered old schema")
		}
		if _, _, err := legacy.RecordJob(f.Proof, f.Authorization, acceptJob); err == nil {
			t.Fatal("birth silently upgraded old schema")
		}
		_ = legacy.Close()
	}
}

func TestJobAndBirthUncertainWritesReconcile(t *testing.T) {
	f := birthFixture(t)
	for _, operation := range []string{"issue", "birth"} {
		for _, committed := range []bool{false, true} {
			t.Run(operation+map[bool]string{false: "-before", true: "-after"}[committed], func(t *testing.T) {
				s, path, _ := newBirthStore(t, f)
				if operation == "birth" {
					issueFixtureJob(t, s, f)
				}
				write := s.writeBatch
				s.writeBatch = func(batch *leveldb.Batch, options *opt.WriteOptions) error {
					if options == nil || !options.Sync {
						t.Fatal("unsynchronized job/birth write")
					}
					if committed {
						if err := write(batch, options); err != nil {
							return err
						}
					}
					return io.ErrUnexpectedEOF
				}
				var err error
				if operation == "issue" {
					var job prospectjob.Job
					job, err = s.IssueJob(f.Context, f.Authorization.PublicKey, func() error { return nil })
					if job != (prospectjob.Job{}) {
						t.Fatal("uncertain issuance exposed nonce/job")
					}
				} else {
					got, inserted, e := s.RecordJob(f.Proof, f.Authorization, acceptJob)
					err = e
					if inserted || !reflect.DeepEqual(got, Record{}) {
						t.Fatal("uncertain birth acknowledged")
					}
				}
				if !errors.Is(err, ErrPoisoned) {
					t.Fatalf("not poisoned: %v", err)
				}
				if _, err := s.GetJob(f.Job.ID); !errors.Is(err, ErrPoisoned) {
					t.Fatal("uncertain handle read")
				}
				if err := s.Close(); err != nil {
					t.Fatal(err)
				}
				s, err = Open(path, false)
				if err != nil {
					t.Fatal(err)
				}
				defer s.Close()
				if operation == "issue" {
					job, err := s.GetJob(f.Job.ID)
					if committed && (err != nil || job != f.Job) {
						t.Fatal("synced job lost")
					}
					if !committed && !errors.Is(err, ErrJobNotFound) {
						t.Fatal("uncommitted job appeared")
					}
				} else {
					s.now = func() time.Time { return time.UnixMilli(f.Job.IssuedAtMS) }
					got, inserted, err := s.RecordJob(f.Proof, f.Authorization, acceptJob)
					if err != nil || inserted == committed || got.Sequence != 1 || got.Birth.BornAtMS != f.Job.IssuedAtMS {
						t.Fatalf("uncertain birth retry: %+v %v %v", got, inserted, err)
					}
				}
			})
		}
	}
}
