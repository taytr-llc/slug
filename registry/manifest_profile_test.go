//go:build slugtestpow

// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package registry

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
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

func TestManifestCheapProfileRefusesBeforeStorage(t *testing.T) {
	f := readManifestCorpus(t)
	refused := func(operation string, err error) {
		t.Helper()
		if err == nil || errors.Is(err, ErrCorrupt) || !strings.Contains(err.Error(), "slugtestpow") {
			t.Fatalf("%s did not refuse the profile before storage access: %v", operation, err)
		}
	}
	path := filepath.Join(t.TempDir(), "must-not-exist")
	created, err := CreateManifestAuthority(path, f.Pin.Network, f.Pin.Genesis, f.Pin.DescriptorID)
	refused("Create", err)
	if created != nil {
		t.Fatal("cheap profile returned an authority")
	}
	for _, readOnly := range []bool{false, true} {
		opened, err := OpenManifestAuthority(path, f.Pin, readOnly)
		refused("Open missing directory", err)
		if opened != nil {
			t.Fatal("cheap profile returned a missing authority")
		}
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("profile refusal created state: %v", err)
	}
}

func TestManifestCheapProfileCannotDisclosePopulatedEvidence(t *testing.T) {
	f := readManifestCorpus(t)
	// Seed exact independent production rows. No authority constructor,
	// receipt encoder, proof verifier, or cheap work produces these values.
	raw, err := os.ReadFile("testdata/manifest/reference/manifest_admission_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var input struct {
		Rows struct {
			Meta          string `json:"meta"`
			Jobs, Records []struct{ Key, Value string }
			Sequences     []struct {
				KeyHex string `json:"key_hex"`
				Value  string `json:"value"`
			}
		} `json:"database_rows"`
	}
	if err := json.Unmarshal(raw, &input); err != nil {
		t.Fatal(err)
	}
	if len(input.Rows.Jobs) != 2 || len(input.Rows.Records) != 5 || len(input.Rows.Sequences) != 5 || input.Rows.Meta != f.Metadata {
		t.Fatal("incomplete frozen database projection")
	}
	path := filepath.Join(t.TempDir(), "production-manifest")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, markerName), []byte(f.Marker), 0600); err != nil {
		t.Fatal(err)
	}
	db, err := leveldb.OpenFile(filepath.Join(path, databaseName), &opt.Options{Strict: opt.StrictAll, ErrorIfExist: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	wantRows := map[string]string{"meta": input.Rows.Meta}
	for _, row := range input.Rows.Jobs {
		wantRows[row.Key] = row.Value
	}
	for _, row := range input.Rows.Records {
		wantRows[row.Key] = row.Value
	}
	for _, row := range input.Rows.Sequences {
		key, err := hex.DecodeString(row.KeyHex)
		if err != nil {
			t.Fatal(err)
		}
		wantRows[string(key)] = row.Value
	}
	batch := new(leveldb.Batch)
	for key, value := range wantRows {
		batch.Put([]byte(key), []byte(value))
	}
	if err := db.Write(batch, &opt.WriteOptions{Sync: true}); err != nil {
		t.Fatal(err)
	}
	if len(wantRows) != 13 {
		t.Fatal("unexpected independent row cardinality")
	}

	// Inspection is unsigned public marker discovery and remains available.
	// The LevelDB is deliberately already locked: refusal must occur before
	// either writable or read-only Open attempts to acquire that lock.
	pin, err := InspectManifestAuthority(path)
	if err != nil || pin != f.Pin {
		t.Fatalf("public marker inspection: %+v %v", pin, err)
	}
	refused := func(operation string, err error) {
		t.Helper()
		if err == nil || errors.Is(err, ErrCorrupt) || !strings.Contains(err.Error(), "slugtestpow") {
			t.Fatalf("%s did not refuse the cheap profile: %v", operation, err)
		}
	}
	for _, readOnly := range []bool{false, true} {
		opened, err := OpenManifestAuthority(path, f.Pin, readOnly)
		refused("Open populated directory", err)
		if opened != nil {
			t.Fatal("cheap profile opened production evidence")
		}
	}
	created, err := CreateManifestAuthority(path, f.Pin.Network, f.Pin.Genesis, f.Pin.DescriptorID)
	refused("Create existing directory", err)
	if created != nil {
		t.Fatal("cheap profile recreated production evidence")
	}

	// A deliberately internal handle tests defenses on every public method
	// even when the public constructor's profile guard has been bypassed.
	var meta manifestMetadata
	if err := json.Unmarshal([]byte(f.Metadata), &meta); err != nil {
		t.Fatal(err)
	}
	s := &ManifestStore{db: db, meta: meta,
		now: func() time.Time { t.Fatal("profile refusal consulted clock"); return time.Time{} },
		writeBatch: func(*leveldb.Batch, *opt.WriteOptions) error {
			t.Fatal("profile refusal wrote state")
			return errors.New("unexpected write")
		},
	}
	t.Cleanup(func() { _ = s.Close() })
	guard := func(prospect.Context, prospect.Proof, prospect.Verified) (Eligibility, error) {
		t.Fatal("cheap profile called final eligibility guard")
		return Eligibility{}, errors.New("unexpected guard")
	}
	for _, tc := range f.Cases {
		got, err := s.GetManifestAdmission(tc.Verified.ProspectID)
		refused("Get populated evidence", err)
		if !reflect.DeepEqual(got, ManifestRecord{}) {
			t.Fatal("Get disclosed unverified production evidence")
		}
		got, inserted, err := s.RecordManifestJobChecked(tc.Proof, tc.Authorization, guard)
		refused("historical admission", err)
		if inserted || !reflect.DeepEqual(got, ManifestRecord{}) {
			t.Fatal("historical replay bypassed profile refusal")
		}
	}
	for _, limit := range []int{1, MaxManifestPageSize} {
		page, err := s.ListManifestAdmissions(0, limit)
		refused("List populated evidence", err)
		if page != nil {
			t.Fatal("List emitted a partial or empty trusted page")
		}
	}
	issued, err := s.IssueManifestJob(f.Jobs[0].Job.Context, f.Jobs[0].Job.PublicKey, func() error { t.Fatal("cheap profile called issuance guard"); return nil })
	refused("Issue", err)
	if issued != (prospectjob.ManifestJob{}) {
		t.Fatal("cheap profile issued work")
	}
	job, err := s.GetManifestJob(f.Jobs[0].Job.ID)
	refused("Get issued job through disallowed handle", err)
	if job != (prospectjob.ManifestJob{}) {
		t.Fatal("disallowed handle returned a job")
	}
	info, err := s.Info()
	refused("Info through disallowed handle", err)
	if info != (ManifestAuthorityInfo{}) {
		t.Fatal("disallowed handle returned authority info")
	}
	if s.meta != meta || s.clockSeen != 0 || s.poisoned != nil {
		t.Fatal("profile refusal changed in-memory authority state")
	}
	actualRows := make(map[string]string)
	it := db.NewIterator(nil, nil)
	for it.Next() {
		actualRows[string(it.Key())] = string(it.Value())
	}
	iterationErr := it.Error()
	it.Release()
	if iterationErr != nil || !reflect.DeepEqual(actualRows, wantRows) {
		t.Fatalf("profile refusal changed durable rows: %v", iterationErr)
	}
	marker, err := os.ReadFile(filepath.Join(path, markerName))
	if err != nil || !bytes.Equal(marker, []byte(f.Marker)) {
		t.Fatal("profile refusal changed marker", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
}
