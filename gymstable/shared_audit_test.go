// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/btcec/v2"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/registry"
)

func TestSharedStableAuditRequiresCompleteAssignmentOrSkipPrefix(t *testing.T) {
	x := newSharedFixture(t, "BAB")
	a := x.gyms[0]
	mustSync(t, a)
	if e := a.s.Close(); e != nil {
		t.Fatal(e)
	}
	a.s = nil
	path := filepath.Join(a.path, "db")
	db, e := leveldb.OpenFile(path, nil)
	if e != nil {
		t.Fatal(e)
	}
	rows := map[string][]byte{}
	it := db.NewIterator(nil, nil)
	for it.Next() {
		rows[string(it.Key())] = append([]byte{}, it.Value()...)
	}
	if e = it.Error(); e != nil {
		t.Fatal(e)
	}
	it.Release()
	if e = db.Close(); e != nil {
		t.Fatal(e)
	}
	// Mutations below are structural corruption, not replacement trusted
	// receipts. Opening audits the complete prefix without rehashing history.
	tests := []struct {
		name string
		edit func(*leveldb.Batch)
	}{
		{"missing-skip", func(b *leveldb.Batch) { b.Delete(sharedSkipKey(1)) }},
		{"missing-assignment", func(b *leveldb.Batch) { b.Delete(seqKey(2)) }},
		{"missing-local-index", func(b *leveldb.Batch) { b.Delete(idKey('i', a.records[0].Verified.ProspectID)) }},
		{"wrong-count", func(b *leveldb.Batch) { b.Put([]byte("shared"), []byte(`{"version":1,"assignment_count":2}`)) }},
		{"count-over-capacity", func(b *leveldb.Batch) { b.Put([]byte("shared"), []byte(`{"version":1,"assignment_count":1001}`)) }},
		{"unknown-skip-field", func(b *leveldb.Batch) { b.Put(sharedSkipKey(1), []byte(`{"version":1,"unexpected":true}`)) }},
		{"wrong-index", func(b *leveldb.Batch) {
			var n [8]byte
			binary.BigEndian.PutUint64(n[:], 1)
			b.Put(idKey('i', a.records[0].Verified.ProspectID), n[:])
		}},
	}
	for _, mutation := range []string{"local-gym", "bad-key", "bad-digest", "wrong-sequence", "duplicate-prospect", "overlap-assignment"} {
		mutation := mutation
		tests = append(tests, struct {
			name string
			edit func(*leveldb.Batch)
		}{mutation, func(b *leveldb.Batch) {
			var s sharedSkip
			if e := json.Unmarshal(rows[string(sharedSkipKey(1))], &s); e != nil {
				t.Fatal(e)
			}
			key := sharedSkipKey(1)
			switch mutation {
			case "local-gym":
				s.GymID, s.PublicKey = a.pin.Identity.GymID, a.pin.Identity.GymPublicKey
			case "bad-key":
				s.PublicKey = "02"
			case "bad-digest":
				s.SourceRecordDigest = "not-a-digest"
			case "wrong-sequence":
				s.Sequence = 3
			case "duplicate-prospect":
				s.ProspectID = a.records[0].Verified.ProspectID
			case "overlap-assignment":
				s.Sequence, key = 2, sharedSkipKey(2)
			}
			raw, e := json.Marshal(s)
			if e != nil {
				t.Fatal(e)
			}
			b.Put(key, raw)
		}})
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db, e := leveldb.OpenFile(path, nil)
			if e != nil {
				t.Fatal(e)
			}
			b := new(leveldb.Batch)
			it := db.NewIterator(nil, nil)
			for it.Next() {
				b.Delete(append([]byte{}, it.Key()...))
			}
			if e = it.Error(); e != nil {
				t.Fatal(e)
			}
			it.Release()
			for k, v := range rows {
				b.Put([]byte(k), v)
			}
			tc.edit(b)
			if e = db.Write(b, &opt.WriteOptions{Sync: true}); e != nil {
				t.Fatal(e)
			}
			if e = db.Close(); e != nil {
				t.Fatal(e)
			}
			if s, e := OpenShared(a.path, a.pin, a.source); !errors.Is(e, ErrCorrupt) {
				if s != nil {
					s.Close()
				}
				t.Fatal("incomplete/corrupt sparse audit accepted", e)
			}
		})
	}
}

func TestSharedStableVerifiesForeignProofBeforeSkipping(t *testing.T) {
	x := newSharedFixture(t, "BA")
	a := x.gyms[0]
	rows := append([]registry.ManifestRecord{}, x.all...)
	// Retain a well-formed issued foreign receipt and its original claimed
	// result, but re-sign a different header. Metadata/authentication alone can
	// accept it; full normal-PoW verification must discover the mismatched result.
	header, e := hex.DecodeString(rows[0].Proof.Header)
	if e != nil {
		t.Fatal(e)
	}
	header[76] ^= 1
	rows[0].Proof.Header = hex.EncodeToString(header)
	scalar, _ := hex.DecodeString("948996ecaa9fa9707693267d6d4abefe0b5fb28b6ebae34f542e106244d254e1")
	key, _ := btcec.PrivKeyFromBytes(scalar)
	defer key.Zero()
	rows[0].Authorization, e = gymauth.SignAdmission(rows[0].Context, rows[0].Proof, key)
	if e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(t.TempDir(), "bad-source")
	sourceRows(t, path, a.pin, rows)
	source, e := registry.OpenManifestAuthority(path, a.pin.Source, false)
	if e != nil {
		t.Fatal("fixture must pass structural source audit", e)
	}
	defer source.Close()
	s, e := createShared(filepath.Join(t.TempDir(), "stable"), a.pin, source, func() time.Time { return time.UnixMilli(a.now) }, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	reads := 0
	s.proofRead = func() { reads++ }
	result, e := s.Sync(context.Background(), a.session)
	if !errors.Is(e, ErrCorrupt) || result.Version != 0 || result.Items != nil || reads != 1 || s.meta.Checkpoint != 0 || s.meta.assignmentCount != 0 {
		t.Fatal("foreign proof bypassed verification/filter boundary", result, e, reads)
	}
	if skipped, e := s.db.Has(sharedSkipKey(1), nil); e != nil || skipped {
		t.Fatal("unverified foreign row became durable skip", e)
	}
}

func TestSharedStableEmptyForeignInventoryRetainsCursor(t *testing.T) {
	x := newSharedFixture(t, "BB")
	a := x.gyms[0]
	mustSync(t, a)
	reopenSharedFixture(t, a)
	ctx := context.Background()
	for _, read := range []func() (Page, error){
		func() (Page, error) { return a.s.Inventory(ctx, a.session, 0, 8) },
		func() (Page, error) { return a.s.SharedRecent(ctx, a.session, 8) },
	} {
		p, e := read()
		if e != nil || p.Items == nil || len(p.Items) != 0 || p.NextAfter != 2 || p.After != 0 || p.HasMore || !p.Status.CaughtUp {
			t.Fatal("foreign-only stable fabricated local rows or lost global cursor", p, e)
		}
	}
}
