//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package registry

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
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
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
)

func manifestTestStore(t *testing.T) (*ManifestStore, string, ManifestAuthorityPin, *int64) {
	t.Helper()
	f := miningFixture(t)
	id, err := fighterdescriptor.ID(fighterdescriptor.CandidateDescriptor())
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "manifest")
	s, err := createManifestAuthority(path, f.Context.Network, f.Context.Genesis, id, bytes.NewReader(bytes.Repeat([]byte{0x31}, 32)), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	now := f.Job.IssuedAtMS
	s.now = func() time.Time { return time.UnixMilli(now) }
	s.random = bytes.NewReader(bytes.Repeat([]byte{0x42}, 320))
	info, err := s.Info()
	if err != nil {
		t.Fatal(err)
	}
	return s, path, info.Pin, &now
}

func TestManifestCreationPinsAndLegacyIsolation(t *testing.T) {
	s, path, pin, _ := manifestTestStore(t)
	info, err := s.Info()
	if err != nil || info.Pin != pin || info.Records != 0 || info.Jobs != 0 || info.Descriptor != fighterdescriptor.CandidateDescriptor() {
		t.Fatal(info, err)
	}
	if _, err := CreateManifestAuthority(path, pin.Network, pin.Genesis, pin.DescriptorID); err == nil {
		t.Fatal("recreated authority")
	}
	if _, err := Open(path, false); err == nil {
		t.Fatal("legacy reader accepted schema5")
	}
	if _, err := OpenManifestAuthority(path, pin, false); err == nil {
		t.Fatal("cross-handle lock missing")
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if legacy, err := Open(path, false); err == nil || legacy != nil {
		if legacy != nil {
			_ = legacy.Close()
		}
		t.Fatal("unlocked legacy reader accepted schema5")
	}
	discovered, err := InspectManifestAuthority(path)
	if err != nil || discovered != pin {
		t.Fatal(discovered, err)
	}
	for _, mutate := range []func(*ManifestAuthorityPin){
		func(p *ManifestAuthorityPin) { p.Version = 4 }, func(p *ManifestAuthorityPin) { p.Network++ }, func(p *ManifestAuthorityPin) { p.Genesis = strings.Repeat("1", 64) },
		func(p *ManifestAuthorityPin) { p.AuthorityID = strings.Repeat("2", 64) }, func(p *ManifestAuthorityPin) { p.DescriptorID = strings.Repeat("3", 64) }, func(p *ManifestAuthorityPin) { p.DescriptorID = strings.ToUpper(p.DescriptorID) },
	} {
		p := pin
		mutate(&p)
		if got, err := OpenManifestAuthority(path, p, false); err == nil || got != nil {
			t.Fatal("accepted wrong pin", p, err)
		}
	}
	reopened, err := OpenManifestAuthority(path, discovered, true)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if got, err := reopened.Info(); err != nil || !got.ReadOnly || got.Pin != pin {
		t.Fatal(got, err)
	}
	f := miningFixture(t)
	if job, err := reopened.IssueManifestJob(f.Context, f.Job.PublicKey, func() error { return nil }); !errors.Is(err, ErrReadOnly) || job != (prospectjob.ManifestJob{}) {
		t.Fatal("read-only issuance", job, err)
	}
	for _, version := range []uint32{1, 2, 3, 4} {
		oldPath := filepath.Join(t.TempDir(), "old")
		old, err := create(oldPath, pin.Network, pin.Genesis, version)
		if err != nil {
			t.Fatal(err)
		}
		_ = old.Close()
		if got, err := OpenManifestAuthority(oldPath, pin, false); err == nil || got != nil {
			t.Fatal("accepted old store", version)
		}
	}
}

func TestManifestCreateRefusesBeforeFilesystemAndRetainsIncomplete(t *testing.T) {
	f := miningFixture(t)
	id, _ := fighterdescriptor.ID(fighterdescriptor.CandidateDescriptor())
	for _, tc := range []struct {
		name        string
		network     uint32
		genesis, id string
		entropy     io.Reader
	}{
		{"network", 0, f.Context.Genesis, id, bytes.NewReader(make([]byte, 32))},
		{"genesis", f.Context.Network, strings.Repeat("0", 64), id, bytes.NewReader(make([]byte, 32))},
		{"descriptor", f.Context.Network, f.Context.Genesis, strings.Repeat("f", 64), bytes.NewReader(make([]byte, 32))},
		{"entropy", f.Context.Network, f.Context.Genesis, id, bytes.NewReader(nil)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "absent")
			s, err := createManifestAuthority(path, tc.network, tc.genesis, tc.id, tc.entropy, nil)
			if err == nil || s != nil {
				t.Fatal("invalid creation")
			}
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("invalid creation touched filesystem", err)
			}
		})
	}
	path := filepath.Join(t.TempDir(), "incomplete")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	pin := ManifestAuthorityPin{Version5, f.Context.Network, f.Context.Genesis, strings.Repeat("0", 64), id}
	if s, err := OpenManifestAuthority(path, pin, false); err == nil || s != nil {
		t.Fatal("initialized incomplete directory")
	}
	entries, err := os.ReadDir(path)
	if err != nil || len(entries) != 0 {
		t.Fatal("failed Open changed incomplete path", err)
	}
	if s, err := CreateManifestAuthority(path, pin.Network, pin.Genesis, pin.DescriptorID); err == nil || s != nil {
		t.Fatal("recreated incomplete directory")
	}
}

func TestManifestIssuanceClockAndUncertainOutcome(t *testing.T) {
	f := miningFixture(t)
	for _, afterCommit := range []bool{false, true} {
		t.Run(map[bool]string{false: "before", true: "after"}[afterCommit], func(t *testing.T) {
			s, path, pin, now := manifestTestStore(t)
			s.writeBatch = func(b *leveldb.Batch, o *opt.WriteOptions) error {
				if o == nil || !o.Sync {
					t.Fatal("unsynchronized write")
				}
				if afterCommit {
					if err := s.db.Write(b, o); err != nil {
						return err
					}
				}
				return errors.New("injected uncertain write")
			}
			job, err := s.IssueManifestJob(f.Context, f.Job.PublicKey, func() error { return nil })
			if !errors.Is(err, ErrPoisoned) || job != (prospectjob.ManifestJob{}) {
				t.Fatal(job, err)
			}
			if _, err := s.Info(); !errors.Is(err, ErrPoisoned) {
				t.Fatal("poisoned metadata exposed", err)
			}
			if _, err := s.GetManifestJob(strings.Repeat("0", 64)); !errors.Is(err, ErrPoisoned) {
				t.Fatal("poisoned job read", err)
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			s, err = OpenManifestAuthority(path, pin, false)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			info, err := s.Info()
			if err != nil || info.Jobs != map[bool]uint64{false: 0, true: 1}[afterCommit] {
				t.Fatal(info, err)
			}
			if afterCommit {
				want, err := prospectjob.NewManifest(f.Context, pin.AuthorityID, pin.DescriptorID, f.Job.PublicKey, strings.Repeat("42", 32), *now)
				if err != nil {
					t.Fatal(err)
				}
				got, err := s.GetManifestJob(strings.ToUpper(want.ID))
				if err != nil || got != want {
					t.Fatal("lost synchronized job", got, err)
				}
			}
		})
	}
	s, path, pin, now := manifestTestStore(t)
	*now = math.MaxInt64
	if job, err := s.IssueManifestJob(f.Context, f.Job.PublicKey, func() error { return nil }); !errors.Is(err, ErrClock) || job != (prospectjob.ManifestJob{}) {
		t.Fatal(job, err)
	}
	if s.meta.LastClockMS != math.MaxInt64 || s.meta.JobCount != 0 {
		t.Fatal("time rejection forgot observed clock")
	}
	_ = s.Close()
	s, err := OpenManifestAuthority(path, pin, false)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.now = func() time.Time { return time.UnixMilli(f.Job.IssuedAtMS) }
	if _, err := s.IssueManifestJob(f.Context, f.Job.PublicKey, func() error { return nil }); !errors.Is(err, ErrClock) {
		t.Fatal("rollback reopened invalid issuance", err)
	}
}

func TestManifestMetadataCanonicalBoundsAndCorruption(t *testing.T) {
	mutations := []struct {
		name string
		fn   func([]byte) []byte
	}{
		{"unknown", func(b []byte) []byte { return append([]byte(`{"unknown":1,`), b[1:]...) }},
		{"alias", func(b []byte) []byte { return bytes.Replace(b, []byte(`"version"`), []byte(`"VERSION"`), 1) }},
		{"duplicate", func(b []byte) []byte { return append([]byte(`{"version":5,`), b[1:]...) }},
		{"decoded-duplicate", func(b []byte) []byte { return append([]byte(`{"ver\u0073ion":5,`), b[1:]...) }},
		{"null", func(b []byte) []byte { return bytes.Replace(b, []byte(`"job_count":0`), []byte(`"job_count":null`), 1) }},
		{"missing", func(b []byte) []byte { return bytes.Replace(b, []byte(`,"job_count":0`), nil, 1) }},
		{"number", func(b []byte) []byte { return bytes.Replace(b, []byte(`"version":5`), []byte(`"version":5.0`), 1) }},
		{"trailing", func(b []byte) []byte { return append(b, '\n') }},
		{"array", func(b []byte) []byte { return bytes.Replace(b, []byte(`"job_count":0`), []byte(`"job_count":[0]`), 1) }},
		{"deep", func(b []byte) []byte {
			return bytes.Replace(b, []byte(`"job_count":0`), []byte(`"job_count":{"a":{"b":0}}`), 1)
		}},
		{"oversized", func(b []byte) []byte { return append(b, bytes.Repeat([]byte(" "), MaxManifestMetadataSize)...) }},
		{"count", func(b []byte) []byte { return bytes.Replace(b, []byte(`"job_count":0`), []byte(`"job_count":1`), 1) }},
		{"clock", func(b []byte) []byte {
			return bytes.Replace(b, []byte(`"last_clock_ms":0`), []byte(`"last_clock_ms":-1`), 1)
		}},
		{"descriptor", func(b []byte) []byte {
			return bytes.Replace(b, []byte(`"renderer_version":1`), []byte(`"renderer_version":2`), 1)
		}},
	}
	for _, tc := range mutations {
		t.Run(tc.name, func(t *testing.T) {
			s, path, pin, _ := manifestTestStore(t)
			raw, err := s.db.Get([]byte(metadataKey), nil)
			if err != nil {
				t.Fatal(err)
			}
			_ = s.Close()
			db, err := leveldb.OpenFile(filepath.Join(path, databaseName), nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := db.Put([]byte(metadataKey), tc.fn(raw), &opt.WriteOptions{Sync: true}); err != nil {
				t.Fatal(err)
			}
			_ = db.Close()
			got, err := OpenManifestAuthority(path, pin, false)
			if err == nil || got != nil {
				if got != nil {
					got.Close()
				}
				t.Fatal("corruption accepted")
			}
		})
	}
	// Null/alias/missing required marker fields likewise fail unsigned inspection.
	for _, mutation := range []func([]byte) []byte{func(b []byte) []byte { return bytes.Replace(b, []byte(`"version":5`), []byte(`"version":null`), 1) }, func(b []byte) []byte { return append(b, '\n') }} {
		s, path, _, _ := manifestTestStore(t)
		_ = s.Close()
		p := filepath.Join(path, markerName)
		b, _ := os.ReadFile(p)
		if err := os.WriteFile(p, mutation(b), 0600); err != nil {
			t.Fatal(err)
		}
		if pin, err := InspectManifestAuthority(path); err == nil || pin != (ManifestAuthorityPin{}) {
			t.Fatal("invalid marker inspected", pin, err)
		}
	}
}

func TestManifestCloseDrainsAndRetainsFailure(t *testing.T) {
	s, _, _, _ := manifestTestStore(t)
	f := miningFixture(t)
	entered, release, issued, closed := make(chan struct{}), make(chan struct{}), make(chan error, 1), make(chan error, 1)
	go func() {
		_, err := s.IssueManifestJob(f.Context, f.Job.PublicKey, func() error { close(entered); <-release; return nil })
		issued <- err
	}()
	<-entered
	go func() { closed <- s.Close() }()
	select {
	case err := <-closed:
		t.Fatal("Close did not drain", err)
	case <-time.After(25 * time.Millisecond):
	}
	close(release)
	if err := <-issued; err != nil {
		t.Fatal(err)
	}
	if err := <-closed; err != nil {
		t.Fatal(err)
	}
	if info, err := s.Info(); !errors.Is(err, ErrClosed) || !reflect.DeepEqual(info, ManifestAuthorityInfo{}) {
		t.Fatal(info, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, _, _, _ = manifestTestStore(t)
	if err := s.db.Close(); err != nil {
		t.Fatal(err)
	}
	first := s.Close()
	second := s.Close()
	if first == nil || second != first {
		t.Fatal("close error not retained", first, second)
	}
}

func TestManifestCodecLimits(t *testing.T) {
	var value ManifestRecord
	// Establish a complete canonical DTO control before mutating the array
	// count. Semantic zero values are irrelevant at the encoding-only layer.
	control := ManifestRecord{}
	control.Proof.MerkleBranch = make([]string, 20)
	for i := range control.Proof.MerkleBranch {
		control.Proof.MerkleBranch[i] = strings.Repeat("0", 64)
	}
	rawControl, err := json.Marshal(control)
	if err != nil || decodeManifestJSON(rawControl, MaxRecordSize, MaxManifestRecordDepth, &value) != nil {
		t.Fatal("20-sibling complete encoding control", err)
	}
	control.Proof.MerkleBranch = append(control.Proof.MerkleBranch, strings.Repeat("0", 64))
	rawControl, err = json.Marshal(control)
	if err != nil || decodeManifestJSON(rawControl, MaxRecordSize, MaxManifestRecordDepth, &value) == nil {
		t.Fatal("21-sibling encoding accepted", err)
	}
	for _, raw := range []string{`{"proof":{"merkle_branch":[` + strings.Repeat(`"`+strings.Repeat("0", 64)+`",`, 20) + `"` + strings.Repeat("0", 64) + `"]}}`, `{"proof":{"merkle_branch":[null]}}`, `{"proof":{"merkle_branch":[{}]}}`, `{"x":{"a":{"b":{"c":{}}}}}`} {
		if err := decodeManifestJSON([]byte(raw), MaxRecordSize, MaxManifestRecordDepth, &value); err == nil {
			t.Fatal("unbounded shape accepted")
		}
	}
	s, _, _, _ := manifestTestStore(t)
	for _, limit := range []int{-1, 0, 9, math.MaxInt} {
		if page, err := s.ListManifestAdmissions(0, limit); err == nil || page != nil {
			t.Fatal("invalid page", limit)
		}
	}
	page, err := s.ListManifestAdmissions(math.MaxUint64, 8)
	if err != nil || page == nil || len(page) != 0 {
		t.Fatal(page, err)
	}
	if _, err := s.GetManifestAdmission(strings.Repeat("0", 64)); !errors.Is(err, ErrNotFound) {
		t.Fatal("zero digest lookup", err)
	}
	var meta map[string]json.RawMessage
	raw, _ := json.Marshal(s.meta)
	if err := json.Unmarshal(raw, &meta); err != nil || len(meta) != 9 {
		t.Fatal("metadata schema", err)
	}
}

func FuzzManifestStorageCodec(f *testing.F) {
	raw, _ := json.Marshal(ManifestRecord{Proof: prospect.Proof{MerkleBranch: []string{}}})
	f.Add(raw)
	f.Add([]byte(`{}`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		before := bytes.Clone(raw)
		var value ManifestRecord
		if err := decodeManifestJSON(raw, MaxRecordSize, MaxManifestRecordDepth, &value); err == nil {
			canonical, err := json.Marshal(value)
			if err != nil || !bytes.Equal(canonical, raw) {
				t.Fatal("accepted noncanonical stored bytes")
			}
			if len(value.Proof.MerkleBranch) > 20 {
				t.Fatal("accepted unbounded branch")
			}
		}
		if !bytes.Equal(before, raw) {
			t.Fatal("decoder mutated caller data")
		}
	})
}
