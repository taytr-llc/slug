//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package registry

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/btcec/v2"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
)

func manifestGuard(tc manifestFixtureCase) func(prospect.Context, prospect.Proof, prospect.Verified) (Eligibility, error) {
	return func(prospect.Context, prospect.Proof, prospect.Verified) (Eligibility, error) {
		return tc.Eligibility, nil
	}
}

func TestManifestCanonicalOwnershipAndConcurrentDuplicates(t *testing.T) {
	f := readManifestCorpus(t)
	s, _, now := manifestCorpusStore(t, f)
	tc := f.Cases[0]
	*now = tc.Accepted
	p, a := uppercaseManifestEvidence(tc.Proof, tc.Authorization)
	r, inserted, err := s.RecordManifestJobChecked(p, a, manifestGuard(tc))
	if err != nil || !inserted || !reflect.DeepEqual(r, tc.Record) {
		t.Fatal("first evidence not canonical", err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, inserted, err := s.RecordManifestJobChecked(p, a, func(prospect.Context, prospect.Proof, prospect.Verified) (Eligibility, error) {
				return Eligibility{}, errors.New("historical guard invoked")
			})
			if err != nil || inserted || !reflect.DeepEqual(got, tc.Record) {
				t.Error("concurrent duplicate changed", err)
			}
		}()
	}
	wg.Wait()
	if s.meta.LastSequence != 1 {
		t.Fatal("duplicate consumed sequence")
	}
	tc = f.Cases[1]
	*now = tc.Accepted
	p = tc.Proof
	p.MerkleBranch = append([]string{}, tc.Proof.MerkleBranch...)
	if len(p.MerkleBranch) == 0 {
		t.Fatal("missing branch mutation fixture")
	}
	r, inserted, err = s.RecordManifestJobChecked(p, tc.Authorization, func(_ prospect.Context, guard prospect.Proof, _ prospect.Verified) (Eligibility, error) {
		guard.MerkleBranch[0] = strings.Repeat("0", 64)
		p.MerkleBranch[0] = strings.Repeat("f", 64)
		return tc.Eligibility, nil
	})
	if err != nil || !inserted || !reflect.DeepEqual(r, tc.Record) {
		t.Fatal("guard/caller slice changed admitted evidence", err)
	}
	r.Proof.MerkleBranch[0] = strings.Repeat("0", 64)
	read, err := s.GetManifestAdmission(tc.Verified.ProspectID)
	if err != nil || !reflect.DeepEqual(read, tc.Record) {
		t.Fatal("returned branch aliased store", err)
	}
	page, err := s.ListManifestAdmissions(0, 8)
	if err != nil {
		t.Fatal(err)
	}
	page[1].Proof.MerkleBranch[0] = strings.Repeat("a", 64)
	read, err = s.GetManifestAdmission(tc.Verified.ProspectID)
	if err != nil || !reflect.DeepEqual(read, tc.Record) {
		t.Fatal("page branch aliased store", err)
	}
}

func TestManifestFinalExpiryAndClockRecovery(t *testing.T) {
	f := readManifestCorpus(t)
	tc := f.Cases[2] // Genuine independent block-target winner.
	for _, stage := range []string{"entry", "bound", "guard"} {
		t.Run(stage, func(t *testing.T) {
			s, path, now := manifestCorpusStore(t, f)
			*now = tc.Job.ExpiresAtMS - 1
			if stage == "entry" {
				*now = tc.Job.ExpiresAtMS
			}
			s.checkpoint = func(at string) {
				if stage == "bound" && at == "bound" {
					*now = tc.Job.ExpiresAtMS
				}
			}
			got, inserted, err := s.RecordManifestJobChecked(tc.Proof, tc.Authorization, func(prospect.Context, prospect.Proof, prospect.Verified) (Eligibility, error) {
				if stage == "guard" {
					*now = tc.Job.ExpiresAtMS
				}
				return tc.Eligibility, nil
			})
			if !errors.Is(err, ErrJobExpired) || inserted || !reflect.DeepEqual(got, ManifestRecord{}) || s.meta.LastSequence != 0 || s.meta.LastClockMS != tc.Job.ExpiresAtMS {
				t.Fatal("expiry lost atomicity/highwater", err)
			}
			_ = s.Close()
			s, err = OpenManifestAuthority(path, f.Pin, false)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			s.now = func() time.Time { return time.UnixMilli(tc.Job.ExpiresAtMS - 1) }
			if _, _, err := s.RecordManifestJobChecked(tc.Proof, tc.Authorization, manifestGuard(tc)); !errors.Is(err, ErrClock) {
				t.Fatal("reopen forgot rejected expiry", err)
			}
		})
	}
}

func TestManifestRejectedEvidenceNeverAllocates(t *testing.T) {
	f := readManifestCorpus(t)
	tc := f.Cases[0]
	for _, name := range []string{"nil-guard", "unknown-job", "foreign-gym", "wrong-key", "bad-signature", "oversized-coinbase", "too-many-siblings", "version1", "bad-renderer", "guard-reject", "false-winner", "binding-dna"} {
		t.Run(name, func(t *testing.T) {
			s, _, now := manifestCorpusStore(t, f)
			*now = tc.Accepted
			p := tc.Proof
			p.MerkleBranch = append([]string{}, p.MerkleBranch...)
			a := tc.Authorization
			guard := manifestGuard(tc)
			switch name {
			case "nil-guard":
				guard = nil
			case "unknown-job":
				p.JobID = strings.Repeat("0", 64)
			case "foreign-gym":
				p.GymID = f.Cases[3].Proof.GymID
			case "wrong-key":
				a.PublicKey = f.Cases[3].Authorization.PublicKey
			case "bad-signature":
				a.Signature = strings.Repeat("00", 64)
			case "oversized-coinbase":
				p.Coinbase = strings.Repeat("00", 16385)
			case "too-many-siblings":
				p.MerkleBranch = make([]string, 21)
			case "version1":
				p.Version = 1
				a.Version = 1
			case "bad-renderer":
				s.meta.Descriptor.RendererVersion++
			case "guard-reject":
				guard = func(prospect.Context, prospect.Proof, prospect.Verified) (Eligibility, error) {
					return Eligibility{}, errors.New("unavailable")
				}
			case "false-winner":
				guard = func(c prospect.Context, _ prospect.Proof, v prospect.Verified) (Eligibility, error) {
					return Eligibility{EligibilityRuleVersion2, "canonical-winning-block", v.WorkHash, c.ParentHeight + 1}, nil
				}
			case "binding-dna":
				v := tc.Verified
				v.DNA.Attributes.Power++
				if b, err := s.binding(v); err == nil || b != (StaticBinding{}) {
					t.Fatal("binding trusted altered DNA")
				}
				return
			}
			got, inserted, err := s.RecordManifestJobChecked(p, a, guard)
			if err == nil || inserted || !reflect.DeepEqual(got, ManifestRecord{}) || s.meta.LastSequence != 0 {
				t.Fatal("rejected evidence created receipt", name, err)
			}
		})
	}
	// An otherwise exact job from a different authority cannot be imported or
	// silently selected by proof-supplied JobID.
	s, _, _, _ := manifestTestStore(t)
	if r, inserted, err := s.RecordManifestJobChecked(tc.Proof, tc.Authorization, manifestGuard(tc)); !errors.Is(err, ErrJobNotFound) || inserted || !reflect.DeepEqual(r, ManifestRecord{}) {
		t.Fatal("foreign issued job accepted", err)
	}
}

func TestManifestAdmissionWriteUncertaintyReconciles(t *testing.T) {
	f := readManifestCorpus(t)
	tc := f.Cases[0]
	for _, after := range []bool{false, true} {
		t.Run(map[bool]string{false: "definite-before", true: "committed-then-error"}[after], func(t *testing.T) {
			s, path, now := manifestCorpusStore(t, f)
			*now = tc.Accepted
			s.checkpoint = func(stage string) {
				if stage == "bound" {
					s.writeBatch = func(b *leveldb.Batch, o *opt.WriteOptions) error {
						if o == nil || !o.Sync {
							t.Fatal("receipt not Sync")
						}
						if after {
							if err := s.db.Write(b, o); err != nil {
								return err
							}
						}
						return errors.New("injected receipt acknowledgement failure")
					}
				}
			}
			r, inserted, err := s.RecordManifestJobChecked(tc.Proof, tc.Authorization, manifestGuard(tc))
			if !errors.Is(err, ErrPoisoned) || inserted || !reflect.DeepEqual(r, ManifestRecord{}) {
				t.Fatal("uncertain write emitted receipt", err)
			}
			if _, err := s.GetManifestAdmission(tc.Verified.ProspectID); !errors.Is(err, ErrPoisoned) {
				t.Fatal("poisoned read", err)
			}
			if page, err := s.ListManifestAdmissions(0, 1); !errors.Is(err, ErrPoisoned) || page != nil {
				t.Fatal("poisoned list", err)
			}
			_ = s.Close()
			s, err = OpenManifestAuthority(path, f.Pin, false)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			if after {
				s.now = func() time.Time { t.Fatal("committed retry consulted clock"); return time.Time{} }
				r, inserted, err = s.RecordManifestJobChecked(tc.Proof, tc.Authorization, func(prospect.Context, prospect.Proof, prospect.Verified) (Eligibility, error) {
					t.Fatal("committed retry consulted node")
					return Eligibility{}, nil
				})
				if err != nil || inserted || !reflect.DeepEqual(r, tc.Record) {
					t.Fatal("commit reconciliation changed", err)
				}
			} else {
				if _, err := s.GetManifestAdmission(tc.Verified.ProspectID); !errors.Is(err, ErrNotFound) {
					t.Fatal("uncommitted receipt survived", err)
				}
				s.now = func() time.Time { return time.UnixMilli(tc.Accepted) }
				r, inserted, err = s.RecordManifestJobChecked(tc.Proof, tc.Authorization, manifestGuard(tc))
				if err != nil || !inserted || !reflect.DeepEqual(r, tc.Record) {
					t.Fatal("uncommitted retry could not safely commit", err)
				}
			}
		})
	}
	// A synchronized rejection observation followed by a lost write ACK also
	// poisons; reopen must not permit the time to move behind the observed expiry.
	s, path, now := manifestCorpusStore(t, f)
	*now = tc.Job.ExpiresAtMS
	s.writeBatch = func(b *leveldb.Batch, o *opt.WriteOptions) error {
		if err := s.db.Write(b, o); err != nil {
			return err
		}
		return errors.New("clock ACK lost")
	}
	if _, _, err := s.RecordManifestJobChecked(tc.Proof, tc.Authorization, manifestGuard(tc)); !errors.Is(err, ErrPoisoned) {
		t.Fatal(err)
	}
	_ = s.Close()
	s, err := OpenManifestAuthority(path, f.Pin, false)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.now = func() time.Time { return time.UnixMilli(tc.Job.ExpiresAtMS - 1) }
	if _, _, err := s.RecordManifestJobChecked(tc.Proof, tc.Authorization, manifestGuard(tc)); !errors.Is(err, ErrClock) {
		t.Fatal("uncertain clock write forgotten", err)
	}
}

func TestManifestMalformedIndexAndLivePageFailure(t *testing.T) {
	f := readManifestCorpus(t)
	for _, name := range []string{"long-job-key", "short-record-key", "long-index-key", "oversized-index-value", "missing-job", "missing-record", "gap", "unknown-key"} {
		t.Run(name, func(t *testing.T) {
			s, path := manifestRawFixtureStore(t, f)
			batch := new(leveldb.Batch)
			switch name {
			case "long-job-key":
				batch.Put(append(jobKey(f.Jobs[0].Job.ID), '0'), []byte(f.Jobs[0].Canonical))
			case "short-record-key":
				batch.Put([]byte(recordPrefix+"0"), []byte(f.Cases[0].Canonical))
			case "long-index-key":
				batch.Put(append(sequenceKey(1), '0'), []byte(f.Cases[0].Verified.ProspectID))
			case "oversized-index-value":
				batch.Put(sequenceKey(1), bytes.Repeat([]byte("a"), 8192))
			case "missing-job":
				batch.Delete(jobKey(f.Jobs[0].Job.ID))
			case "missing-record":
				batch.Delete(recordKey(f.Cases[0].Verified.ProspectID))
			case "gap":
				batch.Delete(sequenceKey(2))
			case "unknown-key":
				batch.Put([]byte("foreign"), []byte("x"))
			}
			if err := s.db.Write(batch, &opt.WriteOptions{Sync: true}); err != nil {
				t.Fatal(err)
			}
			if name == "gap" || name == "oversized-index-value" || name == "missing-record" {
				page, err := s.ListManifestAdmissions(0, 8)
				if err == nil || page != nil {
					t.Fatal("live corrupt page emitted prefix", err)
				}
			}
			_ = s.Close()
			got, err := OpenManifestAuthority(path, f.Pin, false)
			if err == nil || got != nil {
				if got != nil {
					got.Close()
				}
				t.Fatal("corrupt storage opened", name, err)
			}
		})
	}
}

func TestManifestDuplicateReuseRequiresCompleteProofEquality(t *testing.T) {
	f := readManifestCorpus(t)
	s, _ := manifestRawFixtureStore(t, f)
	tc := f.Cases[1]
	if tc.Scalar != 1 || len(tc.Proof.MerkleBranch) == 0 {
		t.Fatal("missing public scalar1 branch fixture")
	}
	// Keep header, job, context, Verified and binding exactly equal, but alter
	// another committed proof field and legitimately reauthorize that action.
	// A header-only/Verified-only duplicate cache would incorrectly emit it.
	changed := tc.Record
	changed.Proof.MerkleBranch = append([]string{}, changed.Proof.MerkleBranch...)
	changed.Proof.MerkleBranch[0] = strings.Repeat("0", 64)
	var scalar [32]byte
	scalar[31] = 1
	private, _ := btcec.PrivKeyFromBytes(scalar[:])
	auth, err := gymauth.SignAdmission(changed.Context, changed.Proof, private)
	private.Zero()
	if err != nil {
		t.Fatal(err)
	}
	changed.Authorization = auth
	raw, err := json.Marshal(changed)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.db.Put(recordKey(tc.Verified.ProspectID), raw, &opt.WriteOptions{Sync: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.decodeRecord(raw, tc.Verified.ProspectID); err != nil {
		t.Fatal("negative control failed before production proof boundary", err)
	}
	r, inserted, err := s.RecordManifestJobChecked(tc.Proof, tc.Authorization, func(prospect.Context, prospect.Proof, prospect.Verified) (Eligibility, error) {
		t.Fatal("corrupt historical evidence reached new admission")
		return Eligibility{}, nil
	})
	if !errors.Is(err, ErrCorrupt) || inserted || !reflect.DeepEqual(r, ManifestRecord{}) {
		t.Fatal("duplicate reuse trusted incomplete evidence equality", err)
	}
}
