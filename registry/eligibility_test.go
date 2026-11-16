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
	"strings"
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/prospect"
)

func miningFixture(t *testing.T) jobFixture {
	t.Helper()
	raw, err := os.ReadFile("testdata/mining_winner_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var value jobFixture
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func newMiningStore(t *testing.T, f jobFixture) (*Store, string, *int64) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mining")
	store, err := CreateMining(path, f.Context.Network, f.Context.Genesis)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	now := f.Job.IssuedAtMS
	store.now = func() time.Time { return time.UnixMilli(now) }
	challenge, _ := hex.DecodeString(f.Job.Challenge)
	store.random = bytes.NewReader(challenge)
	return store, path, &now
}

func currentEligibility(context prospect.Context, _ prospect.Proof, _ prospect.Verified) (Eligibility, error) {
	return Eligibility{EligibilityRuleVersion2, "current-parent", context.ParentHash, context.ParentHeight}, nil
}

func winnerEligibility(context prospect.Context, _ prospect.Proof, verified prospect.Verified) (Eligibility, error) {
	return Eligibility{EligibilityRuleVersion2, "canonical-winning-block", verified.WorkHash, context.ParentHeight + 1}, nil
}

func TestMiningSchemaRequiresExplicitOptIn(t *testing.T) {
	f := miningFixture(t)
	store, path, _ := newMiningStore(t, f)
	issueFixtureJob(t, store, f)
	if _, err := CreateMining(path, f.Context.Network, f.Context.Genesis); err == nil {
		t.Fatal("reinitialized existing registry")
	}
	if _, _, err := store.RecordJob(f.Proof, f.Authorization, acceptJob); err == nil {
		t.Fatal("schema3 callback bypassed schema4 eligibility")
	}
	if _, _, err := store.RecordAuthorized(f.Context, f.Proof, f.Authorization); err == nil {
		t.Fatal("untimed callback bypassed schema4")
	}
	if _, _, err := store.RecordJobChecked(f.Proof, f.Authorization, nil); err == nil {
		t.Fatal("nil eligibility guard accepted")
	}
	legacy, legacyPath, _ := newBirthStore(t, f)
	issueFixtureJob(t, legacy, f)
	if _, _, err := legacy.RecordJobChecked(f.Proof, f.Authorization, winnerEligibility); err == nil {
		t.Fatal("schema3 silently enabled D69")
	}
	first, inserted, err := legacy.RecordJob(f.Proof, f.Authorization, acceptJob)
	if err != nil || !inserted || first.Version != Version3 || first.Eligibility != nil {
		t.Fatalf("legacy birth path changed: %+v %v", first, err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	legacy, err = Open(legacyPath, false)
	if err != nil {
		t.Fatal(err)
	}
	defer legacy.Close()
	if info, err := legacy.Info(); err != nil || info.Version != Version3 {
		t.Fatal("open migrated schema3")
	}
}

func TestMiningEligibilityPersistsAndHistoricalReplayDoesNotReclassify(t *testing.T) {
	f := miningFixture(t)
	for _, source := range []string{"current-parent", "canonical-winning-block"} {
		t.Run(source, func(t *testing.T) {
			store, path, now := newMiningStore(t, f)
			issueFixtureJob(t, store, f)
			*now++
			calls := 0
			first, inserted, err := store.RecordJobChecked(f.Proof, f.Authorization, func(context prospect.Context, proof prospect.Proof, verified prospect.Verified) (Eligibility, error) {
				calls++
				if context != f.Context || !reflect.DeepEqual(proof, f.Proof) || verified != f.Expected {
					t.Fatal("guard did not receive independently verified immutable evidence")
				}
				if source == "canonical-winning-block" {
					return winnerEligibility(context, proof, verified)
				}
				return currentEligibility(context, proof, verified)
			})
			if err != nil || !inserted || calls != 1 || first.Version != Version4 || first.Eligibility == nil || first.Eligibility.Source != source || first.Birth.RuleVersion != 1 {
				t.Fatalf("new schema4 receipt: %+v inserted=%v err=%v", first, inserted, err)
			}
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			store, err = Open(path, false)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			store.now = func() time.Time { t.Fatal("historical replay consulted clock"); return time.Time{} }
			replay, inserted, err := store.RecordJobChecked(f.Proof, f.Authorization, func(prospect.Context, prospect.Proof, prospect.Verified) (Eligibility, error) {
				t.Fatal("historical replay consulted reorged/unavailable node")
				return Eligibility{}, errors.New("unavailable")
			})
			if err != nil || inserted || !reflect.DeepEqual(first, replay) {
				t.Fatalf("historical observation changed: %+v %v", replay, err)
			}
			replay.Eligibility.Hash = strings.Repeat("0", 64)
			read, err := store.Get(first.Verified.ProspectID)
			if err != nil || !reflect.DeepEqual(first, read) {
				t.Fatal("returned eligibility pointer mutated durable first receipt")
			}
			page, err := store.List(0, 1)
			if err != nil || len(page) != 1 || !reflect.DeepEqual(first, page[0]) {
				t.Fatal("schema4 listing lost eligibility")
			}
			var export bytes.Buffer
			if err := store.Export(&export); err != nil || !bytes.Contains(export.Bytes(), []byte(`"eligibility":`)) {
				t.Fatal("schema4 export lost eligibility")
			}
		})
	}
}

func TestMiningRejectsUnboundObservationsAndNonwinningWork(t *testing.T) {
	f := miningFixture(t)
	for _, mutate := range []struct {
		name string
		fn   func(*Eligibility)
	}{
		{"rule", func(e *Eligibility) { e.RuleVersion = 1 }},
		{"source", func(e *Eligibility) { e.Source = "submitted-block" }},
		{"hash", func(e *Eligibility) { e.Hash = f.Context.ParentHash }},
		{"height", func(e *Eligibility) { e.Height-- }},
		{"case", func(e *Eligibility) { e.Hash = strings.ToUpper(e.Hash) }},
		{"parent-hash", func(e *Eligibility) { e.Source = "current-parent"; e.Height-- }},
		{"parent-height", func(e *Eligibility) { e.Source = "current-parent"; e.Hash = f.Context.ParentHash }},
	} {
		t.Run(mutate.name, func(t *testing.T) {
			store, _, _ := newMiningStore(t, f)
			issueFixtureJob(t, store, f)
			got, inserted, err := store.RecordJobChecked(f.Proof, f.Authorization, func(context prospect.Context, proof prospect.Proof, verified prospect.Verified) (Eligibility, error) {
				observation, _ := winnerEligibility(context, proof, verified)
				mutate.fn(&observation)
				return observation, nil
			})
			if err == nil || inserted || !reflect.DeepEqual(got, Record{}) || store.meta.LastSequence != 0 {
				t.Fatal("malformed guard observation created receipt")
			}
		})
	}
	// This independent older vector passes the prospect target but deliberately
	// misses its block target of one. A trusted callback cannot waive D69's
	// local requirement that the claimed canonical block actually won.
	nonwinner := birthFixture(t)
	store, _, _ := newMiningStore(t, nonwinner)
	issueFixtureJob(t, store, nonwinner)
	if got, inserted, err := store.RecordJobChecked(nonwinner.Proof, nonwinner.Authorization, winnerEligibility); err == nil || inserted || !reflect.DeepEqual(got, Record{}) {
		t.Fatal("prospect-only work received canonical-winning eligibility")
	}
}

func TestMiningFinalExpiryAfterEligibilityAndPersistentClock(t *testing.T) {
	f := miningFixture(t)
	store, path, now := newMiningStore(t, f)
	issueFixtureJob(t, store, f)
	*now = f.Job.ExpiresAtMS - 1
	got, inserted, err := store.RecordJobChecked(f.Proof, f.Authorization, func(context prospect.Context, proof prospect.Proof, verified prospect.Verified) (Eligibility, error) {
		*now = f.Job.ExpiresAtMS
		return winnerEligibility(context, proof, verified)
	})
	if !errors.Is(err, ErrJobExpired) || inserted || !reflect.DeepEqual(got, Record{}) || store.meta.LastSequence != 0 || store.meta.LastClockMS != f.Job.ExpiresAtMS {
		t.Fatalf("canonical exception bypassed final D68 expiry: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(path, false)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	store.now = func() time.Time { return time.UnixMilli(f.Job.ExpiresAtMS - 1) }
	if _, _, err := store.RecordJobChecked(f.Proof, f.Authorization, winnerEligibility); !errors.Is(err, ErrClock) {
		t.Fatal("reopen forgot expiry high-water observation")
	}
}

func TestMiningEligibilityTargetEqualityAndZeroDigest(t *testing.T) {
	f := birthFixture(t) // Compact target exactly one.
	for _, value := range []string{strings.Repeat("0", 64), strings.Repeat("0", 63) + "1", strings.Repeat("0", 63) + "2"} {
		verified := f.Expected
		verified.WorkHash = value
		observation, _ := winnerEligibility(f.Context, f.Proof, verified)
		err := ValidateEligibility(f.Context, verified, observation)
		if (err == nil) != (value[63] != '2') {
			t.Fatalf("block target equality boundary %s: %v", value, err)
		}
	}
	for _, bits := range []uint32{0, 0x01810000, 0x03000001, 0x22010000} {
		context := f.Context
		context.ExpectedBits = bits
		verified := f.Expected
		verified.WorkHash = strings.Repeat("0", 64)
		observation, _ := winnerEligibility(context, f.Proof, verified)
		if err := ValidateEligibility(context, verified, observation); err == nil {
			t.Fatalf("invalid/noncanonical compact target %08x accepted", bits)
		}
	}
}

func TestMiningEligibilityCorruptionFailsClosedOnOpen(t *testing.T) {
	f := miningFixture(t)
	for _, test := range []struct {
		name string
		edit func(*Record)
	}{
		{"missing", func(r *Record) { r.Eligibility = nil }},
		{"version", func(r *Record) { r.Eligibility.RuleVersion++ }},
		{"source", func(r *Record) { r.Eligibility.Source = "mempool" }},
		{"hash", func(r *Record) { r.Eligibility.Hash = r.Context.ParentHash }},
		{"height", func(r *Record) { r.Eligibility.Height++ }},
		{"job-missing", func(r *Record) { r.Job = nil }},
		{"birth-expired", func(r *Record) { r.Birth.AcceptedAtMS = r.Job.ExpiresAtMS }},
	} {
		t.Run(test.name, func(t *testing.T) {
			store, path, _ := newMiningStore(t, f)
			issueFixtureJob(t, store, f)
			record, _, err := store.RecordJobChecked(f.Proof, f.Authorization, winnerEligibility)
			if err != nil {
				t.Fatal(err)
			}
			test.edit(&record)
			raw, _ := json.Marshal(record)
			if err := store.db.Put(recordKey(f.Expected.ProspectID), raw, &opt.WriteOptions{Sync: true}); err != nil {
				t.Fatal(err)
			}
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			if reopened, err := Open(path, false); err == nil {
				_ = reopened.Close()
				t.Fatal("corrupted schema4 receipt opened")
			} else if !errors.Is(err, ErrCorrupt) {
				t.Fatalf("wrong corruption error: %v", err)
			}
		})
	}
}

func TestMiningEligibilityUncertainAtomicReceiptReconciles(t *testing.T) {
	f := miningFixture(t)
	for _, committed := range []bool{false, true} {
		t.Run(map[bool]string{false: "before", true: "after"}[committed], func(t *testing.T) {
			store, path, _ := newMiningStore(t, f)
			issueFixtureJob(t, store, f)
			write := store.writeBatch
			store.writeBatch = func(batch *leveldb.Batch, options *opt.WriteOptions) error {
				if options == nil || !options.Sync {
					t.Fatal("schema4 receipt was not synchronized")
				}
				if committed {
					if err := write(batch, options); err != nil {
						return err
					}
				}
				return io.ErrUnexpectedEOF
			}
			got, inserted, err := store.RecordJobChecked(f.Proof, f.Authorization, winnerEligibility)
			if !errors.Is(err, ErrPoisoned) || inserted || !reflect.DeepEqual(got, Record{}) {
				t.Fatal("uncertain receipt acknowledged or handle not poisoned")
			}
			if err := store.Close(); err != nil {
				t.Fatal(err)
			}
			store, err = Open(path, false)
			if err != nil {
				t.Fatal(err)
			}
			defer store.Close()
			store.now = func() time.Time { return time.UnixMilli(f.Job.IssuedAtMS) }
			got, inserted, err = store.RecordJobChecked(f.Proof, f.Authorization, winnerEligibility)
			if err != nil || inserted == committed || got.Sequence != 1 || got.Eligibility == nil || got.Eligibility.Source != "canonical-winning-block" || got.Birth.BornAtMS != f.Job.IssuedAtMS {
				t.Fatalf("uncertain schema4 receipt did not reconcile: %+v %v", got, err)
			}
		})
	}
}
