//go:build slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package registry

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

func TestManifestStagedCheapProfileRefusesBeforeStorage(t *testing.T) {
	f := readManifestCorpus(t)
	// A nonnil, unusable DB proves profile checks happen before any storage
	// access even for an internally fabricated handle. Nothing is created.
	s := &ManifestStore{db: &leveldb.DB{}, now: func() time.Time {
		t.Fatal("cheap profile consulted clock")
		return time.Time{}
	}, writeBatch: func(*leveldb.Batch, *opt.WriteOptions) error {
		t.Fatal("cheap profile wrote storage")
		return errors.New("unreachable")
	}}
	refused := func(name string, e error) {
		t.Helper()
		if e == nil || !strings.Contains(e.Error(), "slugtestpow") {
			t.Fatal(name, e)
		}
	}
	issue, e := s.PrepareManifestIssue(f.Jobs[0].Job.Context, f.Jobs[0].Job.PublicKey)
	refused("PrepareIssue", e)
	if issue != nil {
		t.Fatal("cheap preparation returned issue handle")
	}
	_, e = s.CommitManifestIssue(nil, func() error { t.Fatal("cheap commit checked session"); return nil })
	refused("CommitIssue", e)
	admission, e := s.PrepareManifestAdmission(f.Cases[0].Proof, f.Cases[0].Authorization)
	refused("PrepareAdmission", e)
	if admission != nil {
		t.Fatal("cheap preparation returned admission handle")
	}
	_, inserted, e := s.CommitManifestAdmission(nil, Eligibility{}, func() error { t.Fatal("cheap commit checked session"); return nil })
	refused("CommitAdmission", e)
	if inserted || s.meta != (manifestMetadata{}) || s.clockSeen != 0 || s.poisoned != nil {
		t.Fatal("cheap staged API changed state")
	}
}
