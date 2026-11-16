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

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
)

func TestTestPoWRefusesAdmissionAndStoredEvidenceReads(t *testing.T) {
	data, err := os.ReadFile("../prospect/testdata/proofs_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Context  prospect.Context  `json:"context"`
		Proof    prospect.Proof    `json:"proof"`
		Expected prospect.Verified `json:"expected"`
	}
	if err := json.Unmarshal(data, &fixtures); err != nil || len(fixtures) == 0 {
		t.Fatalf("load independent production fixture: %v", err)
	}
	fixture := fixtures[0]
	path := filepath.Join(t.TempDir(), "registry")
	store, err := Create(path, fixture.Context.Network, fixture.Context.Genesis)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if record, inserted, err := store.Record(fixture.Context, fixture.Proof); err == nil || inserted || !reflect.DeepEqual(record, Record{}) {
		t.Fatalf("cheap build admitted evidence: %+v, %v, %v", record, inserted, err)
	}
	if page, err := store.List(0, 1); err != nil || len(page) != 0 {
		t.Fatalf("rejected admission changed the empty registry: %+v, %v", page, err)
	}

	// Install an independently derived production receipt directly in this
	// disposable test database. This models a legitimate production registry
	// being opened by a tagged binary; it never treats cheap hashing as proof.
	record := Record{Version: Version1, Sequence: 1, Context: fixture.Context,
		Proof: fixture.Proof, Verified: fixture.Expected}
	encodedRecord, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	meta := metadata{Version: Version1, Network: fixture.Context.Network,
		Genesis: fixture.Context.Genesis, LastSequence: 1}
	encodedMeta, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	batch := new(leveldb.Batch)
	batch.Put([]byte(metadataKey), encodedMeta)
	batch.Put(recordKey(fixture.Expected.ProspectID), encodedRecord)
	batch.Put(sequenceKey(1), []byte(fixture.Expected.ProspectID))
	if err := store.db.Write(batch, &opt.WriteOptions{Sync: true}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(path, false)
	if err != nil {
		t.Fatalf("tagged build could not structurally open production evidence: %v", err)
	}
	checkRefusal := func(operation string, err error) {
		t.Helper()
		if err == nil || errors.Is(err, ErrCorrupt) || !strings.Contains(err.Error(), "slugtestpow") {
			t.Fatalf("%s must refuse the profile without mislabeling valid evidence corrupt: %v", operation, err)
		}
	}
	got, err := store.Get(fixture.Expected.ProspectID)
	checkRefusal("Get", err)
	if !reflect.DeepEqual(got, Record{}) {
		t.Fatal("Get emitted a production receipt without production verification")
	}
	page, err := store.List(0, MaxPageSize)
	checkRefusal("List", err)
	if len(page) != 0 {
		t.Fatal("List emitted production receipts in the cheap profile")
	}
	var output bytes.Buffer
	checkRefusal("Export", store.Export(&output))
	if output.Len() != 0 {
		t.Fatal("Export emitted a production receipt in the cheap profile")
	}
	got, inserted, err := store.Record(fixture.Context, fixture.Proof)
	checkRefusal("duplicate Record", err)
	if inserted || !reflect.DeepEqual(got, Record{}) {
		t.Fatal("duplicate admission bypassed the production profile requirement")
	}
}

func TestTestPoWRefusesAuthenticatedAdmissionAndStoredEvidenceReads(t *testing.T) {
	raw, err := os.ReadFile("../gymauth/testdata/authorizations_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Context       prospect.Context      `json:"context"`
		Proof         prospect.Proof        `json:"proof"`
		Expected      prospect.Verified     `json:"expected_verified"`
		Authorization gymauth.Authorization `json:"authorization"`
	}
	if err := json.Unmarshal(raw, &fixtures); err != nil || len(fixtures) == 0 {
		t.Fatalf("load independent production authorization: %v", err)
	}
	f := fixtures[0]
	path := filepath.Join(t.TempDir(), "authenticated")
	store, err := CreateAuthenticated(path, f.Context.Network, f.Context.Genesis)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()
	checkRefusal := func(operation string, err error) {
		t.Helper()
		if err == nil || errors.Is(err, ErrCorrupt) || !strings.Contains(err.Error(), "slugtestpow") {
			t.Fatalf("%s must refuse production profile without mislabelling valid authorization corrupt: %v", operation, err)
		}
	}
	checks := 0
	guard := func() error { checks++; return nil }
	got, inserted, err := store.RecordAuthorizedChecked(f.Context, f.Proof, f.Authorization, guard)
	checkRefusal("first authorized admission", err)
	if inserted || !reflect.DeepEqual(got, Record{}) || checks != 0 {
		t.Fatalf("cheap profile emitted a receipt or reached final guard: %+v inserted=%v checks=%d", got, inserted, checks)
	}
	if records, err := store.List(0, 1); err != nil || len(records) != 0 {
		t.Fatal("refused authorized admission modified empty storage")
	}

	// Install an independently derived, legitimate schema-2 production receipt
	// to model opening an existing authenticated registry with a tagged binary.
	record := Record{Version: Version2, Sequence: 1, Context: f.Context,
		Proof: f.Proof, Verified: f.Expected, Authorization: &f.Authorization}
	encodedRecord, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	meta := metadata{Version: Version2, Network: f.Context.Network, Genesis: f.Context.Genesis, LastSequence: 1}
	encodedMeta, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	batch := new(leveldb.Batch)
	batch.Put([]byte(metadataKey), encodedMeta)
	batch.Put(recordKey(f.Expected.ProspectID), encodedRecord)
	batch.Put(sequenceKey(1), []byte(f.Expected.ProspectID))
	if err := store.db.Write(batch, &opt.WriteOptions{Sync: true}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(path, false)
	if err != nil {
		t.Fatalf("valid production authorization failed structural open: %v", err)
	}
	got, err = store.Get(f.Expected.ProspectID)
	checkRefusal("authenticated Get", err)
	if !reflect.DeepEqual(got, Record{}) {
		t.Fatal("Get emitted unverified production evidence")
	}
	page, err := store.List(0, 100)
	checkRefusal("authenticated List", err)
	if len(page) != 0 {
		t.Fatal("List emitted unverified production evidence")
	}
	var exported bytes.Buffer
	checkRefusal("authenticated Export", store.Export(&exported))
	if exported.Len() != 0 {
		t.Fatal("Export emitted unverified production evidence")
	}
	got, inserted, err = store.RecordAuthorized(f.Context, f.Proof, f.Authorization)
	checkRefusal("authorized replay", err)
	if inserted || !reflect.DeepEqual(got, Record{}) {
		t.Fatal("replay bypassed production profile requirement")
	}
	got, inserted, err = store.RecordAuthorizedChecked(f.Context, f.Proof, f.Authorization, guard)
	checkRefusal("checked authorized replay", err)
	if inserted || !reflect.DeepEqual(got, Record{}) || checks != 0 {
		t.Fatal("checked replay bypassed production profile requirement")
	}
}
