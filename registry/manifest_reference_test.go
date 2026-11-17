//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package registry

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
)

func manifestCorpusStore(t *testing.T, f manifestFixtureCorpus) (*ManifestStore, string, *int64) {
	t.Helper()
	random, err := hex.DecodeString(f.Random)
	if err != nil {
		t.Fatal(err)
	}
	reader := bytes.NewReader(random)
	path := filepath.Join(t.TempDir(), "manifest")
	s, err := createManifestAuthority(path, f.Pin.Network, f.Pin.Genesis, f.Pin.DescriptorID, reader, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	now := f.Issued
	s.now = func() time.Time { return time.UnixMilli(now) }
	s.random = reader
	for _, j := range f.Jobs {
		got, err := s.IssueManifestJob(j.Job.Context, j.Job.PublicKey, func() error { return nil })
		if err != nil || got != j.Job {
			t.Fatal("independent issued job", got, err)
		}
		raw, err := s.db.Get(jobKey(got.ID), nil)
		if err != nil || string(raw) != j.Canonical {
			t.Fatal("canonical issued bytes", err)
		}
	}
	return s, path, &now
}

func admitManifestCase(t *testing.T, s *ManifestStore, now *int64, tc manifestFixtureCase) ManifestRecord {
	t.Helper()
	*now = tc.Accepted
	r, inserted, err := s.RecordManifestJobChecked(tc.Proof, tc.Authorization, func(c prospect.Context, p prospect.Proof, v prospect.Verified) (Eligibility, error) {
		if c != tc.Job.Context || !manifestSameProof(p, tc.Proof) || v != tc.Verified {
			t.Fatal("guard did not receive independent normal-PoW result")
		}
		return tc.Eligibility, nil
	})
	if err != nil || !inserted || !reflect.DeepEqual(r, tc.Record) {
		t.Fatalf("independent complete receipt %s: inserted=%v err=%v got=%+v", tc.Name, inserted, err, r)
	}
	encoded, err := json.Marshal(r)
	if err != nil || string(encoded) != tc.Canonical {
		t.Fatal("independent canonical receipt bytes", tc.Name, err)
	}
	return r
}

func TestManifestIndependentAdmissionAndHistoricalRecovery(t *testing.T) {
	f := readManifestCorpus(t)
	s, path, now := manifestCorpusStore(t, f)
	marker, err := os.ReadFile(filepath.Join(path, markerName))
	if err != nil || string(marker) != f.Marker {
		t.Fatal("independent marker bytes", err)
	}
	for _, tc := range f.Cases {
		admitManifestCase(t, s, now, tc)
	}
	meta, err := s.db.Get([]byte(metadataKey), nil)
	if err != nil || string(meta) != f.Metadata {
		t.Fatal("independent metadata bytes", err)
	}
	info, err := s.Info()
	if err != nil || info != f.Info {
		t.Fatal("independent info", info, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenManifestAuthority(path, f.Pin, false)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.now = func() time.Time { t.Fatal("historical operation consulted clock"); return time.Time{} }
	noGuard := func(prospect.Context, prospect.Proof, prospect.Verified) (Eligibility, error) {
		t.Fatal("historical recovery consulted node/policy")
		return Eligibility{}, errors.New("offline")
	}
	for _, tc := range f.Cases {
		r, err := s.GetManifestAdmission(strings.ToUpper(tc.Verified.ProspectID))
		if err != nil || !reflect.DeepEqual(r, tc.Record) {
			t.Fatal("verified historical Get", tc.Name, err)
		}
		replay, inserted, err := s.RecordManifestJobChecked(tc.Proof, tc.Authorization, noGuard)
		if err != nil || inserted || !reflect.DeepEqual(replay, tc.Record) {
			t.Fatal("historical duplicate changed", tc.Name, err)
		}
	}
	for _, tc := range f.Alternates {
		r, inserted, err := s.RecordManifestJobChecked(tc.Proof, tc.Authorization, noGuard)
		want := f.Cases[tc.Sequence-1].Record
		if err != nil || inserted || !reflect.DeepEqual(r, want) {
			t.Fatal("valid alternate signature overwrote first evidence", tc.Name, err)
		}
	}
	page, err := s.ListManifestAdmissions(0, 8)
	if err != nil || len(page) != len(f.Cases) {
		t.Fatal("verified page", err)
	}
	for i := range page {
		if !reflect.DeepEqual(page[i], f.Cases[i].Record) {
			t.Fatal("page differs", i)
		}
	}
	if raw, err := json.Marshal(page); err != nil || len(raw) > MaxManifestPageBytes {
		t.Fatal("page exceeds byte bound", err)
	}
	if page, err := s.ListManifestAdmissions(math.MaxUint64, 8); err != nil || page == nil || len(page) != 0 {
		t.Fatal("cursor overflow", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenManifestAuthority(path, f.Pin, true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.GetManifestAdmission(f.Cases[0].Verified.ProspectID); err != nil {
		t.Fatal("read-only verified read", err)
	}
	if r, inserted, err := s.RecordManifestJobChecked(f.Cases[0].Proof, f.Cases[0].Authorization, noGuard); !errors.Is(err, ErrReadOnly) || inserted || !reflect.DeepEqual(r, ManifestRecord{}) {
		t.Fatal("read-only duplicate accepted", err)
	}
}

// This installs independently encoded rows, not runtime-generated expected
// receipts. It exercises external storage corruption and read audit directly.
func manifestRawFixtureStore(t *testing.T, f manifestFixtureCorpus) (*ManifestStore, string) {
	t.Helper()
	random, _ := hex.DecodeString(f.Random)
	path := filepath.Join(t.TempDir(), "manifest")
	s, err := createManifestAuthority(path, f.Pin.Network, f.Pin.Genesis, f.Pin.DescriptorID, bytes.NewReader(random), nil)
	if err != nil {
		t.Fatal(err)
	}
	batch := new(leveldb.Batch)
	batch.Put([]byte(metadataKey), []byte(f.Metadata))
	for _, j := range f.Jobs {
		batch.Put(jobKey(j.Job.ID), []byte(j.Canonical))
	}
	for _, tc := range f.Cases {
		batch.Put(recordKey(tc.Verified.ProspectID), []byte(tc.Canonical))
		batch.Put(sequenceKey(tc.Record.Sequence), []byte(tc.Verified.ProspectID))
	}
	if err := s.db.Write(batch, &opt.WriteOptions{Sync: true}); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenManifestAuthority(path, f.Pin, false)
	if err != nil {
		t.Fatal("independent row audit", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s, path
}

func TestManifestIndependentStorageBoundaries(t *testing.T) {
	f := readManifestCorpus(t)
	s, path := manifestRawFixtureStore(t, f)
	for kind, cases := range f.Inputs {
		for _, tc := range cases {
			t.Run(kind+"/"+tc.Name, func(t *testing.T) {
				raw, err := hex.DecodeString(tc.Input)
				if err != nil {
					t.Fatal(err)
				}
				var gotErr error
				switch kind {
				case "marker":
					if err := os.WriteFile(filepath.Join(path, markerName), raw, 0600); err != nil {
						t.Fatal(err)
					}
					_, gotErr = InspectManifestAuthority(path)
				case "metadata":
					var value manifestMetadata
					gotErr = decodeManifestJSON(raw, MaxManifestMetadataSize, 2, &value)
				case "record":
					var value ManifestRecord
					gotErr = decodeManifestJSON(raw, MaxRecordSize, MaxManifestRecordDepth, &value)
				default:
					t.Fatal("unknown fixture kind", kind)
				}
				if (gotErr == nil) != tc.Accepted {
					t.Fatalf("encoding acceptance=%v want=%v err=%v", gotErr == nil, tc.Accepted, gotErr)
				}
			})
		}
	}
	if err := os.WriteFile(filepath.Join(path, markerName), []byte(f.Marker), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range f.Mutations {
		t.Run("record-mutation/"+tc.Name, func(t *testing.T) {
			raw, err := json.Marshal(tc.Record)
			if err != nil {
				t.Fatal(err)
			}
			got, err := s.decodeRecord(raw, f.Cases[0].Verified.ProspectID)
			if (err == nil) != tc.Accepted {
				t.Fatal("mutation acceptance", tc.Name, err)
			}
			if err != nil && !reflect.DeepEqual(got, ManifestRecord{}) {
				t.Fatal("partial invalid record")
			}
		})
	}
	for _, tc := range f.Pins {
		_, supportErr := fighterdescriptor.ResolveCandidate(tc.Pin.DescriptorID)
		_, err := validateManifestPin(tc.Pin)
		if (err == nil) != (tc.Shape && supportErr == nil) {
			t.Fatal("pin shape and compiled support", tc.Name, err)
		}
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range f.Pins {
		opened, err := OpenManifestAuthority(path, tc.Pin, true)
		if (err == nil) != tc.Matches {
			t.Fatal("expected pin matching", tc.Name, err)
		}
		if opened != nil {
			if err := opened.Close(); err != nil {
				t.Fatal(err)
			}
		}
	}
	// Reopen the exact pinned source for the independent trust-limit case.
	s, err := OpenManifestAuthority(path, f.Pin, false)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	// This explicit trust-limit fixture must remain distinguishable from a
	// corruption guarantee: an internally consistent unsigned timestamp is not
	// authenticated by the work, miner signature or static binding.
	raw, _ := json.Marshal(f.TrustLimit.Record)
	if !f.TrustLimit.Consistent || f.TrustLimit.Authenticated {
		t.Fatal("trust-limit fixture mislabeled")
	}
	if _, err := s.decodeRecord(raw, f.TrustLimit.Record.Verified.ProspectID); err != nil {
		t.Fatal("claimed detection outside trusted storage model", err)
	}
}

func TestManifestReadsDoNotTrustStoredWorkSummaries(t *testing.T) {
	f := readManifestCorpus(t)
	s, path := manifestRawFixtureStore(t, f)
	// Same job/key/context, but move another genuine fixture's summary/binding
	// onto the first signed proof. Cheap consistency cannot establish PoW.
	r := f.Cases[0].Record
	r.Verified = f.Cases[1].Verified
	r.StaticBinding = f.Cases[1].Binding
	batch := new(leveldb.Batch)
	for _, tc := range f.Cases {
		batch.Delete(recordKey(tc.Verified.ProspectID))
		batch.Delete(sequenceKey(tc.Record.Sequence))
	}
	raw, _ := json.Marshal(r)
	batch.Put(recordKey(r.Verified.ProspectID), raw)
	batch.Put(sequenceKey(1), []byte(r.Verified.ProspectID))
	next := s.meta
	next.LastSequence = 1
	if err := s.commit(batch, next); err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	s, err := OpenManifestAuthority(path, f.Pin, false)
	if err != nil {
		t.Fatal("cheap Open unexpectedly asserted work", err)
	}
	defer s.Close()
	if got, err := s.GetManifestAdmission(r.Verified.ProspectID); !errors.Is(err, ErrCorrupt) || !reflect.DeepEqual(got, ManifestRecord{}) {
		t.Fatal("stored Verified bypassed production work", err)
	}
	if page, err := s.ListManifestAdmissions(0, 8); !errors.Is(err, ErrCorrupt) || page != nil {
		t.Fatal("partial unverified page", err)
	}
}

func uppercaseManifestEvidence(p prospect.Proof, a gymauth.Authorization) (prospect.Proof, gymauth.Authorization) {
	p.GymID = strings.ToUpper(p.GymID)
	p.JobID = strings.ToUpper(p.JobID)
	p.Header = strings.ToUpper(p.Header)
	p.Coinbase = strings.ToUpper(p.Coinbase)
	p.MerkleBranch = append([]string{}, p.MerkleBranch...)
	for i := range p.MerkleBranch {
		p.MerkleBranch[i] = strings.ToUpper(p.MerkleBranch[i])
	}
	a.PublicKey = strings.ToUpper(a.PublicKey)
	a.Signature = strings.ToUpper(a.Signature)
	return p, a
}
