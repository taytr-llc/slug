//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package registry

import (
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

type authorizedFixture struct {
	Context       prospect.Context      `json:"context"`
	Proof         prospect.Proof        `json:"proof"`
	Expected      prospect.Verified     `json:"expected_verified"`
	Authorization gymauth.Authorization `json:"authorization"`
}

func authFixture(t *testing.T) authorizedFixture {
	t.Helper()
	raw, err := os.ReadFile("../gymauth/testdata/authorizations_v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []authorizedFixture
	if err := json.Unmarshal(raw, &fixtures); err != nil || len(fixtures) == 0 {
		t.Fatalf("independent authorization fixtures: %v", err)
	}
	return fixtures[0]
}

func TestAuthenticatedSchemaAndImmutableReplay(t *testing.T) {
	f := authFixture(t)
	path := filepath.Join(t.TempDir(), "authenticated")
	s, err := CreateAuthenticated(path, f.Context.Network, f.Context.Genesis)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, _, err := s.Record(f.Context, f.Proof); err == nil {
		t.Fatal("evidence-only admission bypassed authenticated schema")
	}
	calls := 0
	guard := func() error { calls++; return nil }
	bad := f.Authorization
	bad.Signature = "00"
	if _, _, err := s.RecordAuthorizedChecked(f.Context, f.Proof, bad, guard); err == nil || calls != 0 {
		t.Fatal("invalid authorization reached final admission guard")
	}
	if _, _, err := s.RecordAuthorizedChecked(f.Context, f.Proof, f.Authorization, nil); err == nil {
		t.Fatal("nil final check accepted")
	}
	first, inserted, err := s.RecordAuthorizedChecked(f.Context, f.Proof, f.Authorization, guard)
	if err != nil || !inserted || calls != 1 || first.Version != Version2 || first.Sequence != 1 ||
		first.Verified != f.Expected || first.Authorization == nil || *first.Authorization != f.Authorization {
		t.Fatalf("authenticated admission: %+v %v %v calls=%d", first, inserted, err, calls)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path, false)
	if err != nil {
		t.Fatal(err)
	}
	// Equivalent time assertions and hex spelling do not replace first evidence.
	ctx, auth := f.Context, f.Authorization
	ctx.MinTimestamp--
	ctx.MaxTimestamp++
	auth.PublicKey = strings.ToUpper(auth.PublicKey)
	auth.Signature = strings.ToUpper(auth.Signature)
	got, inserted, err := s.RecordAuthorized(ctx, f.Proof, auth)
	if err != nil || inserted || !reflect.DeepEqual(first, got) {
		t.Fatalf("replay changed original authorization: %+v %v %v", got, inserted, err)
	}
	reorg := errors.New("node tip changed")
	if _, _, err := s.RecordAuthorizedChecked(ctx, f.Proof, auth, func() error { return reorg }); !errors.Is(err, reorg) {
		t.Fatal("authenticated replay bypassed final node check")
	}
	// A returned pointer is a value snapshot; mutating it cannot alter storage.
	got.Authorization.Signature = "00"
	got, err = s.Get(f.Expected.ProspectID)
	if err != nil || !reflect.DeepEqual(first, got) {
		t.Fatal("returned authorization alias changed storage")
	}
	if records, err := s.List(0, 10); err != nil || len(records) != 1 {
		t.Fatal("replay added a receipt")
	}

	legacy, err := Create(filepath.Join(t.TempDir(), "legacy"), f.Context.Network, f.Context.Genesis)
	if err != nil {
		t.Fatal(err)
	}
	defer legacy.Close()
	if _, _, err := legacy.RecordAuthorized(f.Context, f.Proof, f.Authorization); err == nil {
		t.Fatal("authentication silently upgraded evidence-only registry")
	}
}

func TestAuthenticatedWriteOutcomeAndCorruption(t *testing.T) {
	f := authFixture(t)
	for _, committed := range []bool{false, true} {
		t.Run(map[bool]string{false: "before-commit", true: "after-commit"}[committed], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "authenticated")
			s, err := CreateAuthenticated(path, f.Context.Network, f.Context.Genesis)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = s.Close() }()
			write := s.writeBatch
			s.writeBatch = func(batch *leveldb.Batch, options *opt.WriteOptions) error {
				if options == nil || !options.Sync {
					t.Fatal("authorization receipt was not synced")
				}
				if committed {
					if err := write(batch, options); err != nil {
						return err
					}
				}
				return errors.New("injected write error")
			}
			got, inserted, err := s.RecordAuthorized(f.Context, f.Proof, f.Authorization)
			if !errors.Is(err, ErrPoisoned) || inserted || !reflect.DeepEqual(got, Record{}) {
				t.Fatal("uncertain write acknowledged")
			}
			if _, err := s.Get(f.Expected.ProspectID); !errors.Is(err, ErrPoisoned) {
				t.Fatal("poisoned handle allowed reads")
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			s, err = Open(path, false)
			if err != nil {
				t.Fatal(err)
			}
			got, inserted, err = s.RecordAuthorized(f.Context, f.Proof, f.Authorization)
			if err != nil || inserted == committed || got.Sequence != 1 || got.Authorization == nil || *got.Authorization != f.Authorization {
				t.Fatalf("retry did not reconcile atomic authorization: %+v %v %v", got, inserted, err)
			}
			// Even a structurally valid rewrite cannot turn a bad signature into
			// an authenticated receipt; Open detects it without expensive Argon.
			got.Authorization.Signature = "00"
			raw, _ := json.Marshal(got)
			if err := s.db.Put(recordKey(f.Expected.ProspectID), raw, &opt.WriteOptions{Sync: true}); err != nil {
				t.Fatal(err)
			}
			if err := s.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err := Open(path, false); !errors.Is(err, ErrCorrupt) {
				t.Fatalf("corrupt authorization accepted: %v", err)
			}
		})
	}
}

func TestAuthenticatedFirstAdmissionGuardFailureWritesNothing(t *testing.T) {
	f := authFixture(t)
	s, err := CreateAuthenticated(filepath.Join(t.TempDir(), "authenticated"), f.Context.Network, f.Context.Genesis)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	writes := 0
	write := s.writeBatch
	s.writeBatch = func(batch *leveldb.Batch, options *opt.WriteOptions) error {
		writes++
		return write(batch, options)
	}
	changed := errors.New("node changed before first authorized admission")
	checks := 0
	reject := func() error { checks++; return changed }
	got, inserted, err := s.RecordAuthorizedChecked(f.Context, f.Proof, f.Authorization, reject)
	if !errors.Is(err, changed) || inserted || !reflect.DeepEqual(got, Record{}) || checks != 1 || writes != 0 {
		t.Fatalf("failed first guard returned %+v inserted=%v err=%v checks=%d writes=%d", got, inserted, err, checks, writes)
	}
	if _, err := s.Get(f.Expected.ProspectID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("failed guard reserved the prospect: %v", err)
	}
	if records, err := s.List(0, 1); err != nil || len(records) != 0 {
		t.Fatalf("failed guard changed the empty registry: %+v, %v", records, err)
	}
	got, inserted, err = s.RecordAuthorizedChecked(f.Context, f.Proof, f.Authorization, func() error { checks++; return nil })
	if err != nil || !inserted || got.Sequence != 1 || checks != 2 || writes != 1 {
		t.Fatalf("retry lost first sequence or did not write exactly once: %+v inserted=%v err=%v checks=%d writes=%d", got, inserted, err, checks, writes)
	}
}
