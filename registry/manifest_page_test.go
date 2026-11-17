//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package registry

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/btcec/v2"
	"github.com/taytr-llc/slug/gymauth"
)

func TestManifestPageEightThenOneRealProductionProofs(t *testing.T) {
	f := readManifestCorpus(t)
	s, path, now := manifestCorpusStore(t, f)
	want := make([]ManifestRecord, 0, 9)
	for _, tc := range f.Cases {
		want = append(want, admitManifestCase(t, s, now, tc))
	}
	// These extra inputs exercise the page boundary with actual production
	// work. They are behavioral cases, not new independent expected values:
	// all immutable golden records above remain byte-for-byte corpus values.
	key, _ := btcec.PrivKeyFromBytes([]byte{1}) // Published test scalar only.
	defer key.Zero()
	for nonce := uint32(100); nonce < 104; nonce++ {
		tc := f.Cases[0]
		proof := tc.Proof
		header, err := hex.DecodeString(proof.Header)
		if err != nil || len(header) != 80 {
			t.Fatal("independent header shape", err)
		}
		binary.LittleEndian.PutUint32(header[76:], nonce)
		proof.Header = hex.EncodeToString(header)
		auth, err := gymauth.SignAdmission(tc.Job.Context, proof, key)
		if err != nil {
			t.Fatal(err)
		}
		*now++
		r, inserted, err := s.RecordManifestJobChecked(proof, auth, manifestGuard(tc))
		if err != nil || !inserted || r.Sequence != uint64(len(want)+1) || r.Proof.Header != proof.Header {
			t.Fatalf("extra production input %d: inserted=%v err=%v", nonce, inserted, err)
		}
		for _, old := range want {
			if old.Verified.ProspectID == r.Verified.ProspectID {
				t.Fatal("different headers collided")
			}
		}
		want = append(want, r)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err := OpenManifestAuthority(path, f.Pin, false)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.now = func() time.Time { t.Fatal("page consulted admission clock"); return time.Time{} }
	for _, limit := range []int{0, 9} {
		if page, err := s.ListManifestAdmissions(0, limit); err == nil || page != nil {
			t.Fatalf("invalid limit %d returned trusted data: %v", limit, err)
		}
	}
	page, err := s.ListManifestAdmissions(0, 8)
	if err != nil || !reflect.DeepEqual(page, want[:8]) {
		t.Fatal("first exact eight-record page", err)
	}
	encoded, err := json.Marshal(page)
	if err != nil || len(encoded) > MaxManifestPageBytes {
		t.Fatal("page byte bound", err)
	}
	last, err := s.ListManifestAdmissions(8, 8)
	if err != nil || !reflect.DeepEqual(last, want[8:]) {
		t.Fatal("ninth record continuation", err)
	}
	if end, err := s.ListManifestAdmissions(9, 8); err != nil || end == nil || len(end) != 0 {
		t.Fatal("terminal cursor", err)
	}

	// Corrupt only the ninth receipt after Open. The first page must neither
	// read nor verify beyond its eight-record work budget. A page containing
	// that ninth row must fail without leaking its already verified prefix.
	if err := s.db.Put(recordKey(want[8].Verified.ProspectID), []byte("{}"), &opt.WriteOptions{Sync: true}); err != nil {
		t.Fatal(err)
	}
	page, err = s.ListManifestAdmissions(0, 8)
	if err != nil || !reflect.DeepEqual(page, want[:8]) {
		t.Fatal("first page touched ninth receipt", err)
	}
	if partial, err := s.ListManifestAdmissions(7, 2); !errors.Is(err, ErrCorrupt) || partial != nil {
		t.Fatal("corrupt continuation leaked verified prefix", err)
	}
}
