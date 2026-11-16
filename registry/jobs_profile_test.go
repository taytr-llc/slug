//go:build slugtestpow

// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package registry

import (
	"bytes"
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
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
)

func TestCheapProfileRefusesBirthButAllowsPublicJobInspection(t *testing.T) {
	raw, err := os.ReadFile("../prospectjob/testdata/jobs_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Job           prospectjob.Job       `json:"job"`
		Context       prospect.Context      `json:"context"`
		Proof         prospect.Proof        `json:"proof"`
		Authorization gymauth.Authorization `json:"authorization"`
		Verified      prospect.Verified     `json:"expected_verified"`
	}
	if err := json.Unmarshal(raw, &fixtures); err != nil || len(fixtures) == 0 {
		t.Fatalf("read independent production fixture: %v", err)
	}
	f := fixtures[0]
	refusedPath := filepath.Join(t.TempDir(), "refused-birth")
	if store, err := CreateBirth(refusedPath, f.Context.Network, f.Context.Genesis); err == nil || store != nil || !strings.Contains(err.Error(), "slugtestpow") {
		t.Fatal("cheap profile initialized a birth authority")
	}
	if _, err := os.Stat(refusedPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("refused birth initialization created a directory")
	}

	// Install independently derived public production evidence directly in a
	// disposable test store. This models opening an existing normal-profile
	// authority without treating cheap hashes as valid work or issuing a job.
	path := filepath.Join(t.TempDir(), "production-birth")
	store, err := create(path, f.Context.Network, f.Context.Genesis, Version3)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	accepted := f.Job.IssuedAtMS + 1
	record := Record{Version: Version3, Sequence: 1, Context: f.Context, Proof: f.Proof, Verified: f.Verified,
		Authorization: &f.Authorization, Job: &f.Job,
		Birth: &Birth{RuleVersion: f.Job.RuleVersion, BornAtMS: f.Job.IssuedAtMS, AcceptedAtMS: accepted}}
	meta := metadata{Version: Version3, Network: f.Context.Network, Genesis: f.Context.Genesis, LastSequence: 1, JobCount: 1, LastClockMS: accepted}
	encode := func(value any) []byte {
		t.Helper()
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	batch := new(leveldb.Batch)
	batch.Put(jobKey(f.Job.ID), encode(f.Job))
	batch.Put(recordKey(f.Verified.ProspectID), encode(record))
	batch.Put(sequenceKey(1), []byte(f.Verified.ProspectID))
	batch.Put([]byte(metadataKey), encode(meta))
	if err := store.db.Write(batch, &opt.WriteOptions{Sync: true}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(path, false)
	if err != nil {
		t.Fatalf("structural open misclassified valid schema-3 evidence: %v", err)
	}
	store.now = func() time.Time { t.Fatal("cheap profile consulted issuance clock"); return time.Time{} }
	job, err := store.GetJob(strings.ToUpper(f.Job.ID))
	if err != nil || job != f.Job {
		t.Fatalf("public issued-job inspection failed: %v", err)
	}
	refused := func(operation string, err error) {
		t.Helper()
		if err == nil || errors.Is(err, ErrCorrupt) || !strings.Contains(err.Error(), "slugtestpow") {
			t.Fatalf("%s must refuse profile without mislabeling evidence: %v", operation, err)
		}
	}
	got, err := store.Get(f.Verified.ProspectID)
	refused("Get", err)
	if !reflect.DeepEqual(got, Record{}) {
		t.Fatal("cheap profile emitted a birth receipt")
	}
	page, err := store.List(0, MaxPageSize)
	refused("List", err)
	if len(page) != 0 {
		t.Fatal("cheap profile listed birth receipts")
	}
	var output bytes.Buffer
	refused("Export", store.Export(&output))
	if output.Len() != 0 {
		t.Fatal("cheap profile exported birth evidence")
	}
	guardCalls := 0
	got, inserted, err := store.RecordJob(f.Proof, f.Authorization, func(prospect.Context, prospect.Proof) error { guardCalls++; return nil })
	refused("duplicate RecordJob", err)
	if inserted || !reflect.DeepEqual(got, Record{}) || guardCalls != 0 {
		t.Fatal("cheap profile acknowledged birth replay or contacted a node")
	}
	issued, err := store.IssueJob(f.Context, f.Authorization.PublicKey, func() error { guardCalls++; return nil })
	refused("IssueJob", err)
	if issued != (prospectjob.Job{}) || guardCalls != 0 {
		t.Fatal("cheap profile issued work or contacted a node")
	}
	if store.meta != meta {
		t.Fatal("profile refusal changed durable receipt/clock metadata")
	}
}
