//go:build slugtestpow

// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

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

func TestCheapProfileRefusesMiningButInspectsIssuedJobs(t *testing.T) {
	raw, err := os.ReadFile("testdata/mining_winner_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Job           prospectjob.Job       `json:"job"`
		Context       prospect.Context      `json:"context"`
		Proof         prospect.Proof        `json:"proof"`
		Authorization gymauth.Authorization `json:"authorization"`
		Verified      prospect.Verified     `json:"expected_verified"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	refusedPath := filepath.Join(t.TempDir(), "refused")
	if store, err := CreateMining(refusedPath, f.Context.Network, f.Context.Genesis); err == nil || store != nil || !strings.Contains(err.Error(), "slugtestpow") {
		t.Fatal("cheap build created a mining authority")
	}
	if _, err := os.Stat(refusedPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("refused creation modified disk")
	}
	path := filepath.Join(t.TempDir(), "production-mining")
	store, err := create(path, f.Context.Network, f.Context.Genesis, Version4)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	record := Record{Version: Version4, Sequence: 1, Context: f.Context, Proof: f.Proof, Verified: f.Verified,
		Authorization: &f.Authorization, Job: &f.Job,
		Birth:       &Birth{RuleVersion: 1, BornAtMS: f.Job.IssuedAtMS, AcceptedAtMS: f.Job.IssuedAtMS},
		Eligibility: &Eligibility{EligibilityRuleVersion2, "canonical-winning-block", f.Verified.WorkHash, f.Context.ParentHeight + 1}}
	meta := metadata{Version: Version4, Network: f.Context.Network, Genesis: f.Context.Genesis, LastSequence: 1, JobCount: 1, LastClockMS: f.Job.IssuedAtMS}
	encode := func(value any) []byte { data, _ := json.Marshal(value); return data }
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
		t.Fatalf("structural open rejected independent production eligibility: %v", err)
	}
	store.now = func() time.Time { t.Fatal("cheap build consulted clock"); return time.Time{} }
	if job, err := store.GetJob(f.Job.ID); err != nil || job != f.Job {
		t.Fatal("cheap build could not inspect public issued job")
	}
	refused := func(err error) {
		t.Helper()
		if err == nil || errors.Is(err, ErrCorrupt) || !strings.Contains(err.Error(), "slugtestpow") {
			t.Fatalf("wrong profile refusal: %v", err)
		}
	}
	got, err := store.Get(f.Verified.ProspectID)
	refused(err)
	if !reflect.DeepEqual(got, Record{}) {
		t.Fatal("cheap Get emitted receipt")
	}
	page, err := store.List(0, 1)
	refused(err)
	if len(page) != 0 {
		t.Fatal("cheap List emitted receipt")
	}
	var output bytes.Buffer
	refused(store.Export(&output))
	if output.Len() != 0 {
		t.Fatal("cheap Export emitted receipt")
	}
	got, inserted, err := store.RecordJobChecked(f.Proof, f.Authorization, func(prospect.Context, prospect.Proof, prospect.Verified) (Eligibility, error) {
		t.Fatal("cheap replay reached node callback")
		return Eligibility{}, nil
	})
	refused(err)
	if inserted || !reflect.DeepEqual(got, Record{}) {
		t.Fatal("cheap replay acknowledged receipt")
	}
	job, err := store.IssueJob(f.Context, f.Authorization.PublicKey, func() error { t.Fatal("cheap issuance reached node callback"); return nil })
	refused(err)
	if job != (prospectjob.Job{}) || store.meta != meta {
		t.Fatal("cheap issuance modified authority")
	}
}
