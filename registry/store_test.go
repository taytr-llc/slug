//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package registry

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/prospect"
)

type registryFixture struct {
	Name     string            `json:"name"`
	Context  prospect.Context  `json:"context"`
	Proof    prospect.Proof    `json:"proof"`
	Expected prospect.Verified `json:"expected"`
}

func registryFixtures(t *testing.T) []registryFixture {
	t.Helper()
	data, err := os.ReadFile("../prospect/testdata/proofs_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []registryFixture
	if err := json.Unmarshal(data, &fixtures); err != nil || len(fixtures) < 2 {
		t.Fatalf("load independent prospect fixtures: %v (count %d)", err, len(fixtures))
	}
	return fixtures
}

func createTestStore(t *testing.T, fixture registryFixture) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "registry")
	store, err := Create(path, fixture.Context.Network, fixture.Context.Genesis)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store, path
}

func requireAdmission(t *testing.T, store *Store, fixture registryFixture, sequence uint64) Record {
	t.Helper()
	record, inserted, err := store.Record(fixture.Context, fixture.Proof)
	if err != nil || !inserted {
		t.Fatalf("admit %s: inserted=%v err=%v", fixture.Name, inserted, err)
	}
	if record.Version != Version1 || record.Sequence != sequence || record.Verified != fixture.Expected {
		t.Fatalf("admission differs from independent fixture: %+v", record)
	}
	return record
}

func TestRecordReopenAndDuplicatePreserveFirstEvidence(t *testing.T) {
	fixture := registryFixtures(t)[1]
	store, path := createTestStore(t, fixture)
	first := requireAdmission(t, store, fixture, 1)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := Open(path, false)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	got, err := store.Get(strings.ToUpper(fixture.Expected.ProspectID))
	if err != nil || !reflect.DeepEqual(got, first) {
		t.Fatalf("reopen changed evidence: got %+v, err %v", got, err)
	}

	// Both representations verify under the same committed policy. A later
	// receipt with wider time assertions must not replace the original context.
	variantContext, variantProof := fixture.Context, fixture.Proof
	variantContext.Genesis = strings.ToUpper(variantContext.Genesis)
	variantContext.ParentHash = strings.ToUpper(variantContext.ParentHash)
	variantContext.ProspectTarget = strings.ToUpper(variantContext.ProspectTarget)
	variantContext.MinTimestamp--
	variantContext.MaxTimestamp++
	variantProof.GymID = strings.ToUpper(variantProof.GymID)
	variantProof.Header = strings.ToUpper(variantProof.Header)
	variantProof.Coinbase = strings.ToUpper(variantProof.Coinbase)
	variantProof.MerkleBranch = append([]string(nil), fixture.Proof.MerkleBranch...)
	for i := range variantProof.MerkleBranch {
		variantProof.MerkleBranch[i] = strings.ToUpper(variantProof.MerkleBranch[i])
	}
	got, inserted, err := store.Record(variantContext, variantProof)
	if err != nil || inserted || !reflect.DeepEqual(got, first) {
		t.Fatalf("duplicate replaced first evidence: inserted=%v got=%+v err=%v", inserted, got, err)
	}
	page, err := store.List(0, MaxPageSize)
	if err != nil || len(page) != 1 || !reflect.DeepEqual(page[0], first) {
		t.Fatalf("duplicate appended a journal entry: %+v, %v", page, err)
	}
}

func TestConcurrentDuplicateAdmission(t *testing.T) {
	fixture := registryFixtures(t)[0]
	store, _ := createTestStore(t, fixture)
	const workers = 8
	type outcome struct {
		record   Record
		inserted bool
		err      error
	}
	start := make(chan struct{})
	results := make(chan outcome, workers)
	var group sync.WaitGroup
	for i := 0; i < workers; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			record, inserted, err := store.Record(fixture.Context, fixture.Proof)
			results <- outcome{record, inserted, err}
		}()
	}
	close(start)
	group.Wait()
	close(results)
	insertions := 0
	for result := range results {
		if result.err != nil || result.record.Sequence != 1 || result.record.Verified != fixture.Expected {
			t.Fatalf("concurrent admission: %+v", result)
		}
		if result.inserted {
			insertions++
		}
	}
	if insertions != 1 {
		t.Fatalf("inserted %d copies, want one", insertions)
	}
	page, err := store.List(0, MaxPageSize)
	if err != nil || len(page) != 1 {
		t.Fatalf("duplicate journal entries: %+v, %v", page, err)
	}
}

func TestRejectedEvidenceDoesNotConsumeSequence(t *testing.T) {
	fixture := registryFixtures(t)[0]
	store, _ := createTestStore(t, fixture)
	badProof := fixture.Proof
	badProof.GymID = strings.Repeat("11", 32)
	wrongNetwork := fixture.Context
	wrongNetwork.Network++
	wrongGenesis := fixture.Context
	wrongGenesis.Genesis = strings.Repeat("22", 32)
	for _, test := range []struct {
		name    string
		context prospect.Context
		proof   prospect.Proof
	}{
		{"uncommitted gym", fixture.Context, badProof},
		{"wrong network", wrongNetwork, fixture.Proof},
		{"wrong genesis", wrongGenesis, fixture.Proof},
	} {
		t.Run(test.name, func(t *testing.T) {
			record, inserted, err := store.Record(test.context, test.proof)
			if err == nil || inserted || !reflect.DeepEqual(record, Record{}) {
				t.Fatalf("rejected evidence produced admission: %+v, %v, %v", record, inserted, err)
			}
		})
	}
	if _, err := store.Get(fixture.Expected.ProspectID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("invalid submission left a record: %v", err)
	}
	page, err := store.List(0, MaxPageSize)
	if err != nil || len(page) != 0 {
		t.Fatalf("invalid submission changed journal: %+v, %v", page, err)
	}
	requireAdmission(t, store, fixture, 1)
}

func TestRecordDefensiveCopiesAndBoundedPages(t *testing.T) {
	fixtures := registryFixtures(t)
	store, _ := createTestStore(t, fixtures[0])
	first := requireAdmission(t, store, fixtures[0], 1)
	second := requireAdmission(t, store, fixtures[1], 2)
	// The input and every returned branch slice must be independent of storage.
	fixtures[1].Proof.MerkleBranch[0] = strings.Repeat("00", 32)
	second.Proof.MerkleBranch[0] = strings.Repeat("ff", 32)
	second.Verified.DNA.Attributes.Power = -1
	got, err := store.Get(fixtures[1].Expected.ProspectID)
	if err != nil || got.Proof.MerkleBranch[0] == second.Proof.MerkleBranch[0] || got.Verified != fixtures[1].Expected {
		t.Fatalf("caller mutation reached stored evidence: %+v, %v", got, err)
	}
	for _, test := range []struct {
		after uint64
		limit int
		want  []uint64
	}{
		{0, 1, []uint64{1}}, {1, 1, []uint64{2}},
		{0, MaxPageSize, []uint64{1, 2}}, {2, 1, nil}, {^uint64(0), 1, nil},
	} {
		page, err := store.List(test.after, test.limit)
		if err != nil || len(page) != len(test.want) {
			t.Fatalf("page after %d: count %d err %v", test.after, len(page), err)
		}
		for i := range page {
			if page[i].Sequence != test.want[i] {
				t.Fatal("page is not in admission sequence order")
			}
		}
		if len(page) > 0 && len(page[len(page)-1].Proof.MerkleBranch) > 0 {
			page[len(page)-1].Proof.MerkleBranch[0] = strings.Repeat("33", 32)
		}
	}
	for _, limit := range []int{-1, 0, MaxPageSize + 1} {
		if _, err := store.List(0, limit); err == nil {
			t.Fatalf("accepted page limit %d", limit)
		}
	}
	for _, id := range []string{"", "ab", "0x" + first.Verified.ProspectID, strings.Repeat("z", 64)} {
		if _, err := store.Get(id); err == nil {
			t.Fatalf("accepted malformed ID %q", id)
		}
	}
	got, err = store.Get(fixtures[1].Expected.ProspectID)
	if err != nil || got.Proof.MerkleBranch[0] == strings.Repeat("33", 32) {
		t.Fatal("mutating a page changed stored evidence")
	}
}

type registryErrorWriter struct{ err error }

func (writer registryErrorWriter) Write([]byte) (int, error) { return 0, writer.err }

func TestReadOnlyExportAndClosedHandle(t *testing.T) {
	fixtures := registryFixtures(t)
	store, path := createTestStore(t, fixtures[0])
	want := []Record{requireAdmission(t, store, fixtures[0], 1), requireAdmission(t, store, fixtures[1], 2)}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err := Open(path, true)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if _, _, err := store.Record(fixtures[0].Context, fixtures[0].Proof); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("read-only handle accepted admission: %v", err)
	}
	var output bytes.Buffer
	if err := store.Export(&output); err != nil {
		t.Fatal(err)
	}
	if len(strings.Split(strings.TrimSuffix(output.String(), "\n"), "\n")) != len(want) {
		t.Fatal("export is not one record per line")
	}
	decoder := json.NewDecoder(&output)
	for _, expected := range want {
		var record Record
		if err := decoder.Decode(&record); err != nil || !reflect.DeepEqual(record, expected) {
			t.Fatalf("export changed evidence: %+v, %v", record, err)
		}
	}
	if err := decoder.Decode(new(Record)); err != io.EOF {
		t.Fatalf("unexpected export tail: %v", err)
	}
	writeErr := errors.New("export destination unavailable")
	if err := store.Export(registryErrorWriter{writeErr}); !errors.Is(err, writeErr) {
		t.Fatalf("export swallowed writer error: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close is not idempotent: %v", err)
	}
	if _, err := store.Get(want[0].Verified.ProspectID); !errors.Is(err, ErrClosed) {
		t.Fatalf("Get after Close: %v", err)
	}
	if _, err := store.List(0, 1); !errors.Is(err, ErrClosed) {
		t.Fatalf("List after Close: %v", err)
	}
	if err := store.Export(io.Discard); !errors.Is(err, ErrClosed) {
		t.Fatalf("Export after Close: %v", err)
	}
	if _, _, err := store.Record(fixtures[0].Context, fixtures[0].Proof); !errors.Is(err, ErrClosed) {
		t.Fatalf("Record after Close: %v", err)
	}
}

func TestOpenDoesNotCreateOrInitializeForeignDirectories(t *testing.T) {
	fixture := registryFixtures(t)[0]
	missing := filepath.Join(t.TempDir(), "missing")
	for _, readOnly := range []bool{false, true} {
		if store, err := Open(missing, readOnly); err == nil {
			store.Close()
			t.Fatal("opened a missing registry")
		}
		if _, err := os.Stat(missing); !os.IsNotExist(err) {
			t.Fatalf("Open created a missing registry directory: %v", err)
		}
	}
	foreign := t.TempDir()
	untouched := filepath.Join(foreign, "foreign.txt")
	if err := os.WriteFile(untouched, []byte("keep me"), 0600); err != nil {
		t.Fatal(err)
	}
	if store, err := Create(foreign, fixture.Context.Network, fixture.Context.Genesis); err == nil {
		store.Close()
		t.Fatal("Create accepted an existing foreign directory")
	}
	if store, err := Open(foreign, false); err == nil {
		store.Close()
		t.Fatal("Open initialized a foreign directory")
	}
	entries, err := os.ReadDir(foreign)
	if err != nil || len(entries) != 1 || entries[0].Name() != "foreign.txt" {
		t.Fatalf("registry operation changed foreign contents: %+v, %v", entries, err)
	}
	data, err := os.ReadFile(untouched)
	if err != nil || string(data) != "keep me" {
		t.Fatal("registry operation changed foreign data")
	}
}

func TestCreateValidatesChainIdentity(t *testing.T) {
	fixture := registryFixtures(t)[0]
	for _, test := range []struct {
		network uint32
		genesis string
	}{
		{0, fixture.Context.Genesis}, {fixture.Context.Network, "00"},
		{fixture.Context.Network, strings.Repeat("00", 32)},
		{fixture.Context.Network, strings.Repeat("z", 64)},
	} {
		path := filepath.Join(t.TempDir(), "registry")
		if store, err := Create(path, test.network, test.genesis); err == nil {
			store.Close()
			t.Fatal("accepted invalid registry chain identity")
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("invalid identity created registry files: %v", err)
		}
	}
}

func TestZeroDigestIsAValidAbsentIdentifier(t *testing.T) {
	fixture := registryFixtures(t)[0]
	store, _ := createTestStore(t, fixture)
	zero := strings.Repeat("0", 64)
	if got, err := normalizeHash(zero); err != nil || got != zero {
		t.Fatalf("generic digest normalization rejected zero: %q, %v", got, err)
	}
	if _, err := chainIdentity(fixture.Context.Network, zero); err == nil {
		t.Fatal("chain identity accepted a zero genesis")
	}
	if _, err := store.Get(zero); !errors.Is(err, ErrNotFound) {
		t.Fatalf("zero digest lookup should be absent, not malformed: %v", err)
	}
}

func TestLiveSequenceGapFailsClosed(t *testing.T) {
	fixtures := registryFixtures(t)
	for _, missing := range []uint64{1, 2} {
		t.Run(fmt.Sprintf("missing sequence %d", missing), func(t *testing.T) {
			store, _ := createTestStore(t, fixtures[0])
			requireAdmission(t, store, fixtures[0], 1)
			requireAdmission(t, store, fixtures[1], 2)
			// Deliberately bypass the API to model an index inconsistency
			// discovered after Open's initial structural scan.
			if err := store.db.Delete(sequenceKey(missing), &opt.WriteOptions{Sync: true}); err != nil {
				t.Fatal(err)
			}
			if page, err := store.List(0, MaxPageSize); !errors.Is(err, ErrCorrupt) || len(page) != 0 {
				t.Fatalf("List silently omitted a receipt: %+v, %v", page, err)
			}
			if err := store.Export(io.Discard); !errors.Is(err, ErrCorrupt) {
				t.Fatalf("Export silently omitted a receipt: %v", err)
			}
		})
	}
}

func modifyRegistryDB(t *testing.T, path string, modify func(*leveldb.DB) error) {
	t.Helper()
	db, err := leveldb.OpenFile(filepath.Join(path, "db"), &opt.Options{ErrorIfMissing: true})
	if err != nil {
		t.Fatal(err)
	}
	err = modify(db)
	closeErr := db.Close()
	if err != nil || closeErr != nil {
		t.Fatalf("inject registry corruption: %v, close %v", err, closeErr)
	}
}

func TestRegistryCorruptionFailsClosed(t *testing.T) {
	fixture := registryFixtures(t)[0]
	sequenceKey := append([]byte("sequence:"), make([]byte, 8)...)
	binary.BigEndian.PutUint64(sequenceKey[len("sequence:"):], 1)
	for _, test := range []struct {
		name   string
		mutate func(*leveldb.DB) error
	}{
		{"missing metadata", func(db *leveldb.DB) error { return db.Delete([]byte("meta"), &opt.WriteOptions{Sync: true}) }},
		{"missing sequence", func(db *leveldb.DB) error { return db.Delete(sequenceKey, &opt.WriteOptions{Sync: true}) }},
		{"wrong sequence target", func(db *leveldb.DB) error {
			return db.Put(sequenceKey, []byte(strings.Repeat("f", 64)), &opt.WriteOptions{Sync: true})
		}},
		{"unknown key", func(db *leveldb.DB) error {
			return db.Put([]byte("foreign-key"), []byte("foreign-value"), &opt.WriteOptions{Sync: true})
		}},
		{"invalid record JSON", func(db *leveldb.DB) error {
			return db.Put([]byte("record:"+fixture.Expected.ProspectID), []byte("{"), &opt.WriteOptions{Sync: true})
		}},
		{"oversized record", func(db *leveldb.DB) error {
			return db.Put([]byte("record:"+fixture.Expected.ProspectID), bytes.Repeat([]byte{' '}, MaxRecordSize+1), &opt.WriteOptions{Sync: true})
		}},
		{"altered stored DNA", func(db *leveldb.DB) error {
			key := []byte("record:" + fixture.Expected.ProspectID)
			data, err := db.Get(key, nil)
			if err != nil {
				return err
			}
			var record Record
			if err := json.Unmarshal(data, &record); err != nil {
				return err
			}
			record.Verified.DNA.Attributes.Power++
			data, err = json.Marshal(record)
			if err != nil {
				return err
			}
			return db.Put(key, data, &opt.WriteOptions{Sync: true})
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, path := createTestStore(t, fixture)
			requireAdmission(t, store, fixture, 1)
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			modifyRegistryDB(t, path, test.mutate)
			store, err := Open(path, false)
			if err != nil {
				if !errors.Is(err, ErrCorrupt) {
					t.Fatalf("corruption did not return ErrCorrupt: %v", err)
				}
				// Validation failures must release their LevelDB file lock.
				modifyRegistryDB(t, path, func(*leveldb.DB) error { return nil })
				return
			}
			defer store.Close()
			if record, err := store.Get(fixture.Expected.ProspectID); !errors.Is(err, ErrCorrupt) || !reflect.DeepEqual(record, Record{}) {
				t.Fatalf("exposed corrupted record: %+v, %v", record, err)
			}
		})
	}
}

func TestUnknownSchemaAndMarkerFailClosed(t *testing.T) {
	fixture := registryFixtures(t)[0]
	for _, change := range []string{"missing", "future version", "foreign application"} {
		t.Run(change, func(t *testing.T) {
			store, path := createTestStore(t, fixture)
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			markerPath := filepath.Join(path, "SLUG-REGISTRY.json")
			original, err := os.ReadFile(markerPath)
			if err != nil {
				t.Fatal(err)
			}
			switch change {
			case "missing":
				err = os.Remove(markerPath)
			case "future version":
				err = os.WriteFile(markerPath, bytes.Replace(original, []byte(`"version":1`), []byte(`"version":99`), 1), 0600)
			case "foreign application":
				err = os.WriteFile(markerPath, bytes.Replace(original, []byte("slug-prospect-registry"), []byte("unrelated-application"), 1), 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			if opened, err := Open(path, false); err == nil {
				opened.Close()
				t.Fatal("opened unknown or missing registry schema")
			}
			if err := os.WriteFile(markerPath, original, 0600); err != nil {
				t.Fatal(err)
			}
			opened, err := Open(path, false)
			if err != nil {
				t.Fatalf("failed Open retained a lock or changed the database: %v", err)
			}
			opened.Close()
		})
	}
}

func TestWriteFailurePoisonsHandleAndReconcilesAfterReopen(t *testing.T) {
	fixtures := registryFixtures(t)
	for _, commitFirst := range []bool{false, true} {
		t.Run(fmt.Sprintf("error after commit=%v", commitFirst), func(t *testing.T) {
			store, path := createTestStore(t, fixtures[0])
			originalWrite := store.writeBatch
			injected := errors.New("injected uncertain storage failure")
			store.writeBatch = func(batch *leveldb.Batch, options *opt.WriteOptions) error {
				if options == nil || !options.Sync {
					t.Error("admission was not a synchronous batch")
				}
				if commitFirst {
					if err := originalWrite(batch, options); err != nil {
						return err
					}
				}
				return injected
			}
			if _, inserted, err := store.Record(fixtures[0].Context, fixtures[0].Proof); !errors.Is(err, ErrPoisoned) || !errors.Is(err, injected) || inserted {
				t.Fatalf("uncertain write acknowledged success: %v, %v", inserted, err)
			}
			if _, err := store.Get(fixtures[0].Expected.ProspectID); !errors.Is(err, ErrPoisoned) {
				t.Fatalf("Get on poisoned handle: %v", err)
			}
			if _, err := store.List(0, 1); !errors.Is(err, ErrPoisoned) {
				t.Fatalf("List on poisoned handle: %v", err)
			}
			if err := store.Export(io.Discard); !errors.Is(err, ErrPoisoned) {
				t.Fatalf("Export on poisoned handle: %v", err)
			}
			if _, _, err := store.Record(fixtures[1].Context, fixtures[1].Proof); !errors.Is(err, ErrPoisoned) {
				t.Fatalf("Record on poisoned handle: %v", err)
			}
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			store, err := Open(path, false)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			record, inserted, err := store.Record(fixtures[0].Context, fixtures[0].Proof)
			if err != nil || inserted == commitFirst || record.Sequence != 1 || record.Verified != fixtures[0].Expected {
				t.Fatalf("failed to reconcile uncertain admission: %+v, inserted=%v err=%v", record, inserted, err)
			}
			requireAdmission(t, store, fixtures[1], 2)
		})
	}
}

const registryChildMode = "SLUG_REGISTRY_TEST_CHILD"
const registryChildPath = "SLUG_REGISTRY_TEST_PATH"

// TestRegistrySubprocessHelper runs only in subprocesses created below. It
// deliberately avoids Close after acknowledging a synchronous admission.
func TestRegistrySubprocessHelper(t *testing.T) {
	mode := os.Getenv(registryChildMode)
	if mode == "" {
		return
	}
	path := os.Getenv(registryChildPath)
	if mode == "try-open-writer" || mode == "try-open-reader" {
		store, err := Open(path, mode == "try-open-reader")
		if err == nil {
			store.Close()
			t.Fatal("second process acquired a locked registry")
		}
		fmt.Println("REGISTRY_LOCK_REJECTED")
		return
	}
	if mode != "record-and-wait" {
		t.Fatalf("unknown registry child mode %q", mode)
	}
	fixture := registryFixtures(t)[0]
	store, err := Create(path, fixture.Context.Network, fixture.Context.Genesis)
	if err != nil {
		t.Fatal(err)
	}
	requireAdmission(t, store, fixture, 1)
	fmt.Println("REGISTRY_SYNC_ACKNOWLEDGED")
	// Wait for the parent to kill this process. No deferred database close
	// can turn this into an ordinary clean-shutdown test.
	for {
		time.Sleep(time.Second)
		if store == nil {
			t.Fatal("registry disappeared")
		}
	}
}

func registryChildCommand(ctx context.Context, mode, path string) *exec.Cmd {
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRegistrySubprocessHelper$", "-test.v")
	command.Env = append(os.Environ(), registryChildMode+"="+mode, registryChildPath+"="+path)
	return command
}

func TestRegistryCrossProcessLock(t *testing.T) {
	fixture := registryFixtures(t)[0]
	_, path := createTestStore(t, fixture)
	for _, mode := range []string{"try-open-writer", "try-open-reader"} {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		output, err := registryChildCommand(ctx, mode, path).CombinedOutput()
		cancel()
		if err != nil || !bytes.Contains(output, []byte("REGISTRY_LOCK_REJECTED")) {
			t.Fatalf("cross-process exclusion failed: %v\n%s", err, output)
		}
	}
}

func TestAcknowledgedAdmissionSurvivesAbruptProcessExit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "registry")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	command := registryChildCommand(ctx, "record-and-wait", path)
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
		if scanner.Text() == "REGISTRY_SYNC_ACKNOWLEDGED" {
			acknowledged = true
			break
		}
	}
	if !acknowledged {
		_ = command.Wait()
		t.Fatalf("child did not acknowledge durable admission: %v\n%s", scanner.Err(), &stderr)
	}
	if err := command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := command.Wait(); err == nil {
		t.Fatal("child unexpectedly exited cleanly")
	}
	fixture := registryFixtures(t)[0]
	store, err := Open(path, false)
	if err != nil {
		t.Fatalf("recover after acknowledged write and process kill: %v", err)
	}
	defer store.Close()
	record, inserted, err := store.Record(fixture.Context, fixture.Proof)
	if err != nil || inserted || record.Sequence != 1 || record.Verified != fixture.Expected {
		t.Fatalf("lost acknowledged admission: %+v, inserted=%v err=%v", record, inserted, err)
	}
}
