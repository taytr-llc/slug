//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package registry

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
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

func TestManifestStagedIssueMatchesIndependentJobs(t *testing.T) {
	f := readManifestCorpus(t)
	random, e := hex.DecodeString(f.Random)
	if e != nil {
		t.Fatal(e)
	}
	reader := bytes.NewReader(random)
	s, e := createManifestAuthority(filepath.Join(t.TempDir(), "manifest"), f.Pin.Network, f.Pin.Genesis, f.Pin.DescriptorID, reader, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	s.random = reader
	for _, row := range f.Jobs {
		before := s.meta
		s.now = func() time.Time { t.Fatal("prepare sampled birth clock"); return time.Time{} }
		p, e := s.PrepareManifestIssue(row.Job.Context, row.Job.PublicKey)
		if e != nil || p == nil || s.meta != before || s.clockSeen != before.LastClockMS {
			t.Fatal("prepare changed durable issuance state", e)
		}
		calls := 0
		s.now = func() time.Time { return time.UnixMilli(f.Issued) }
		got, e := s.CommitManifestIssue(p, func() error { calls++; return nil })
		if e != nil || calls != 2 || got != row.Job {
			t.Fatal("staged issuance differs from independent job", got, e)
		}
		raw, e := s.db.Get(jobKey(got.ID), nil)
		if e != nil || string(raw) != row.Canonical {
			t.Fatal("staged issuance changed canonical bytes", e)
		}
		copy := *p
		if _, e = s.CommitManifestIssue(&copy, func() error { return nil }); !errors.Is(e, ErrConflict) {
			t.Fatal("copied preparation issued another job", e)
		}
	}
}

func TestManifestStagedIssueRejectsStaleHandlesAndGuard(t *testing.T) {
	s, path, pin, now := manifestTestStore(t)
	f := miningFixture(t)
	p, e := s.PrepareManifestIssue(f.Context, f.Job.PublicKey)
	if e != nil {
		t.Fatal(e)
	}
	before := s.meta
	denied := errors.New("original view revoked")
	if _, e = s.CommitManifestIssue(p, nil); e == nil {
		t.Fatal("nil guard accepted")
	}
	if _, e = s.CommitManifestIssue(p, func() error { return denied }); !errors.Is(e, denied) || s.meta != before {
		t.Fatal("denied issuance changed state", e)
	}
	other, _, _, _ := manifestTestStore(t)
	if _, e = other.CommitManifestIssue(p, func() error { return nil }); !errors.Is(e, ErrManifestAuthority) {
		t.Fatal("foreign store used preparation", e)
	}
	*now += 1234
	calls := 0
	if _, e = s.CommitManifestIssue(p, func() error {
		calls++
		if calls == 2 {
			return denied
		}
		return nil
	}); !errors.Is(e, denied) || s.meta != before {
		t.Fatal("last issuance fence did not prevent publication", e)
	}
	job, e := s.CommitManifestIssue(p, func() error { return nil })
	if e != nil || job.IssuedAtMS != *now || job.ExpiresAtMS != *now+prospectjob.LifetimeMS {
		t.Fatal("birth did not start at successful commit", job, e)
	}
	unused, e := s.PrepareManifestIssue(f.Context, f.Job.PublicKey)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	if _, e = s.CommitManifestIssue(unused, func() error { return nil }); !errors.Is(e, ErrClosed) {
		t.Fatal("closed store used handle", e)
	}
	reopened, e := OpenManifestAuthority(path, pin, false)
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.Close()
	if _, e = reopened.CommitManifestIssue(unused, func() error { return nil }); !errors.Is(e, ErrManifestAuthority) {
		t.Fatal("reopened store used prior handle", e)
	}
}

func TestManifestStagedCommitNeverWaitsForRegistryGate(t *testing.T) {
	s, _, _, _ := manifestTestStore(t)
	for _, admission := range []bool{false, true} {
		s.mu.Lock()
		done := make(chan error, 1)
		go func() {
			if admission {
				_, _, e := s.CommitManifestAdmission(nil, Eligibility{}, nil)
				done <- e
			} else {
				_, e := s.CommitManifestIssue(nil, nil)
				done <- e
			}
		}()
		select {
		case e := <-done:
			s.mu.Unlock()
			if !errors.Is(e, ErrBusy) {
				t.Fatal("locked registry commit was not busy", e)
			}
		case <-time.After(2 * time.Second):
			s.mu.Unlock()
			<-done
			t.Fatal("commit waited for registry gate")
		}
	}
}

func TestManifestStagedAdmissionsMatchIndependentEvidenceAndOwnSlices(t *testing.T) {
	f := readManifestCorpus(t)
	s, _, now := manifestCorpusStore(t, f)
	for _, tc := range f.Cases {
		before := s.meta
		clock := s.now
		s.now = func() time.Time { t.Fatal("prepare sampled admission clock"); return time.Time{} }
		proof := ownedManifestProof(tc.Proof)
		prepared, e := s.PrepareManifestAdmission(proof, tc.Authorization)
		if e != nil || prepared == nil || prepared.Historical() || s.meta != before {
			t.Fatal("prepare changed admission state", e)
		}
		if prepared.Context() != tc.Job.Context || prepared.Verified() != tc.Verified || !reflect.DeepEqual(prepared.Proof(), tc.Proof) {
			t.Fatal("prepared accessors changed independent evidence")
		}
		if len(proof.MerkleBranch) > 0 {
			proof.MerkleBranch[0] = strings.Repeat("f", 64)
		}
		s.now, *now = clock, tc.Accepted
		got, inserted, e := s.CommitManifestAdmission(prepared, tc.Eligibility, func() error {
			copy := prepared.Proof()
			if len(copy.MerkleBranch) > 0 {
				copy.MerkleBranch[0] = strings.Repeat("0", 64)
			}
			return nil
		})
		if e != nil || !inserted || !reflect.DeepEqual(got, tc.Record) {
			t.Fatal("staged record differs from independent evidence", tc.Name, e)
		}
		raw, e := s.db.Get(recordKey(tc.Verified.ProspectID), nil)
		if e != nil || string(raw) != tc.Canonical {
			t.Fatal("canonical record bytes changed", e)
		}
		if len(got.Proof.MerkleBranch) > 0 {
			got.Proof.MerkleBranch[0] = strings.Repeat("a", 64)
		}
		if _, _, e = s.CommitManifestAdmission(prepared, tc.Eligibility, func() error { return nil }); !errors.Is(e, ErrConflict) {
			t.Fatal("new preparation reused after insertion without reprepare", e)
		}
	}
	// Expiry/current node policy never changes an already verified receipt.
	s.now = func() time.Time { t.Fatal("historical recovery consulted clock"); return time.Time{} }
	for _, alternate := range f.Alternates {
		prepared, e := s.PrepareManifestAdmission(alternate.Proof, alternate.Authorization)
		if e != nil || !prepared.Historical() {
			t.Fatal("historical preparation", e)
		}
		called := false
		got, inserted, e := s.CommitManifestAdmission(prepared, Eligibility{}, func() error { called = true; return nil })
		want := f.Cases[alternate.Sequence-1].Record
		if e != nil || inserted || !called || !reflect.DeepEqual(got, want) {
			t.Fatal("historical recovery changed original receipt", e)
		}
	}
}

func TestManifestStagedAdmissionExpiryPersistsAcrossRestart(t *testing.T) {
	f := readManifestCorpus(t)
	tc := f.Cases[0]
	for _, final := range []bool{false, true} {
		t.Run(map[bool]string{false: "before-commit", true: "at-final-fence"}[final], func(t *testing.T) {
			s, path, now := manifestCorpusStore(t, f)
			p, e := s.PrepareManifestAdmission(tc.Proof, tc.Authorization)
			if e != nil {
				t.Fatal(e)
			}
			*now = tc.Job.ExpiresAtMS
			if final {
				*now--
			}
			checks := 0
			got, inserted, e := s.CommitManifestAdmission(p, tc.Eligibility, func() error {
				checks++
				if final && checks == 2 {
					*now = tc.Job.ExpiresAtMS
				}
				return nil
			})
			if !errors.Is(e, ErrJobExpired) || inserted || !reflect.DeepEqual(got, ManifestRecord{}) || s.meta.LastSequence != 0 || s.meta.LastClockMS != tc.Job.ExpiresAtMS {
				t.Fatal("expiry lost clock/sequence atomicity", e)
			}
			if e = s.Close(); e != nil {
				t.Fatal(e)
			}
			reopened, e := OpenManifestAuthority(path, f.Pin, false)
			if e != nil {
				t.Fatal(e)
			}
			defer reopened.Close()
			if _, _, e = reopened.CommitManifestAdmission(p, tc.Eligibility, func() error { return nil }); !errors.Is(e, ErrManifestAuthority) {
				t.Fatal("reopened store accepted old preparation", e)
			}
			reopened.now = func() time.Time { return time.UnixMilli(tc.Job.ExpiresAtMS - 1) }
			p, e = reopened.PrepareManifestAdmission(tc.Proof, tc.Authorization)
			if e != nil {
				t.Fatal(e)
			}
			if _, _, e = reopened.CommitManifestAdmission(p, tc.Eligibility, func() error { return nil }); !errors.Is(e, ErrClock) {
				t.Fatal("reopen forgot observed expiry", e)
			}
		})
	}
}

func TestManifestStagedAdmissionChangedDuplicateAndSourceRejected(t *testing.T) {
	f := readManifestCorpus(t)
	tc := f.Cases[0]
	s, _, now := manifestCorpusStore(t, f)
	p, e := s.PrepareManifestAdmission(tc.Proof, tc.Authorization)
	if e != nil {
		t.Fatal(e)
	}
	*now = tc.Accepted
	other, _, _ := manifestCorpusStore(t, f)
	if _, _, e = other.CommitManifestAdmission(p, tc.Eligibility, func() error { return nil }); !errors.Is(e, ErrManifestAuthority) {
		t.Fatal("foreign store accepted prepared proof", e)
	}
	admitManifestCase(t, s, now, tc)
	if _, _, e = s.CommitManifestAdmission(p, tc.Eligibility, func() error { t.Fatal("changed duplicate reached fence"); return nil }); !errors.Is(e, ErrConflict) {
		t.Fatal("duplicate race did not require reprepare", e)
	}
	p, e = s.PrepareManifestAdmission(tc.Proof, tc.Authorization)
	if e != nil || !p.Historical() {
		t.Fatal(e)
	}
	changed := tc.Record
	changed.Birth.AcceptedAtMS++
	raw, e := json.Marshal(changed)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.db.Put(recordKey(tc.Verified.ProspectID), raw, &opt.WriteOptions{Sync: true}); e != nil {
		t.Fatal(e)
	}
	if _, _, e = s.CommitManifestAdmission(p, Eligibility{}, func() error { t.Fatal("changed historical row reached fence"); return nil }); !errors.Is(e, ErrConflict) {
		t.Fatal("changed historical row returned cached receipt", e)
	}
}

func TestManifestStagedAdmissionFinalGuardAndUncertainty(t *testing.T) {
	f := readManifestCorpus(t)
	tc := f.Cases[0]
	for _, mode := range []string{"nil", "revoked", "final-revoked", "invalid-eligibility", "sequence-full", "lost-ack"} {
		t.Run(mode, func(t *testing.T) {
			s, path, now := manifestCorpusStore(t, f)
			p, e := s.PrepareManifestAdmission(tc.Proof, tc.Authorization)
			if e != nil {
				t.Fatal(e)
			}
			*now = tc.Accepted
			denied := errors.New("original session revoked")
			check, observation := func() error { return nil }, tc.Eligibility
			calls := 0
			switch mode {
			case "nil":
				check = nil
			case "revoked":
				check = func() error { return denied }
			case "final-revoked":
				check = func() error {
					calls++
					if calls == 2 {
						return denied
					}
					return nil
				}
			case "invalid-eligibility":
				observation = Eligibility{}
			case "sequence-full":
				s.meta.LastSequence = math.MaxUint64
			case "lost-ack":
				s.checkpoint = func(stage string) {
					if stage == "before-admission" {
						s.writeBatch = func(b *leveldb.Batch, o *opt.WriteOptions) error {
							if o == nil || !o.Sync {
								t.Fatal("unsynchronized commit")
							}
							if e := s.db.Write(b, o); e != nil {
								return e
							}
							return errors.New("admission ACK lost")
						}
					}
				}
			}
			got, inserted, e := s.CommitManifestAdmission(p, observation, check)
			if e == nil || inserted || !reflect.DeepEqual(got, ManifestRecord{}) {
				t.Fatal("failed admission disclosed success", mode, e)
			}
			if mode != "sequence-full" && s.meta.LastSequence != 0 {
				t.Fatal("failed admission advanced sequence")
			}
			if mode != "lost-ack" {
				return
			}
			if !errors.Is(e, ErrPoisoned) {
				t.Fatal("uncertain write did not poison", e)
			}
			if _, _, e = s.CommitManifestAdmission(p, observation, check); !errors.Is(e, ErrPoisoned) {
				t.Fatal("poisoned handle retried", e)
			}
			if e = s.Close(); e != nil {
				t.Fatal(e)
			}
			reopened, e := OpenManifestAuthority(path, f.Pin, false)
			if e != nil {
				t.Fatal(e)
			}
			defer reopened.Close()
			reopened.now = func() time.Time { t.Fatal("committed recovery sampled clock"); return time.Time{} }
			p, e = reopened.PrepareManifestAdmission(tc.Proof, tc.Authorization)
			if e != nil || !p.Historical() {
				t.Fatal(e)
			}
			got, inserted, e = reopened.CommitManifestAdmission(p, Eligibility{}, func() error { return nil })
			if e != nil || inserted || !reflect.DeepEqual(got, tc.Record) {
				t.Fatal("lost ACK changed original receipt", e)
			}
		})
	}
}

func TestManifestStagedNilHandleAccessors(t *testing.T) {
	var p *PreparedManifestAdmission
	if p.Historical() || p.Context() != (prospect.Context{}) || p.Verified() != (prospect.Verified{}) || !reflect.DeepEqual(p.Proof(), prospect.Proof{}) {
		t.Fatal("nil preparation disclosed evidence")
	}
}
