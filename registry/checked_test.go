//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package registry

import (
	"errors"
	"reflect"
	"testing"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

func TestCheckedAdmissionGuardsWritesAndReplay(t *testing.T) {
	fixture := registryFixtures(t)[0]
	store, path := createTestStore(t, fixture)
	changed := errors.New("trusted node changed its tip")
	calls := 0
	reject := func() error { calls++; return changed }

	// Invalid evidence must not call the external check, and a failed check
	// after valid work must not reserve an ID or consume a sequence.
	bad := fixture.Proof
	bad.Header = "00"
	if _, _, err := store.RecordChecked(fixture.Context, bad, reject); err == nil || calls != 0 {
		t.Fatalf("invalid evidence reached guard: calls=%d err=%v", calls, err)
	}
	if _, _, err := store.RecordChecked(fixture.Context, fixture.Proof, nil); err == nil {
		t.Fatal("nil guard accepted")
	}
	got, inserted, err := store.RecordChecked(fixture.Context, fixture.Proof, reject)
	if !errors.Is(err, changed) || inserted || !reflect.DeepEqual(got, Record{}) || calls != 1 {
		t.Fatalf("failed guard acknowledged receipt: %+v %v %v calls=%d", got, inserted, err, calls)
	}
	if page, err := store.List(0, 1); err != nil || len(page) != 0 {
		t.Fatalf("failed guard wrote a receipt: %+v %v", page, err)
	}

	// The accepted path must perform the check before the synchronized batch.
	checked := false
	writes := 0
	write := store.writeBatch
	store.writeBatch = func(batch *leveldb.Batch, options *opt.WriteOptions) error {
		writes++
		if !checked || options == nil || !options.Sync {
			t.Fatal("write happened before the guard or without synchronization")
		}
		return write(batch, options)
	}
	accept := func() error { checked = true; calls++; return nil }
	first, inserted, err := store.RecordChecked(fixture.Context, fixture.Proof, accept)
	if err != nil || !inserted || first.Sequence != 1 || writes != 1 || calls != 2 {
		t.Fatalf("checked admission failed: %+v %v %v writes=%d calls=%d", first, inserted, err, writes, calls)
	}

	got, inserted, err = store.RecordChecked(fixture.Context, fixture.Proof, reject)
	if !errors.Is(err, changed) || inserted || !reflect.DeepEqual(got, Record{}) || writes != 1 || calls != 3 {
		t.Fatal("duplicate bypassed guard or modified storage")
	}
	got, inserted, err = store.RecordChecked(fixture.Context, fixture.Proof, accept)
	if err != nil || inserted || !reflect.DeepEqual(got, first) || writes != 1 || calls != 4 {
		t.Fatalf("checked replay changed first evidence: %+v %v %v", got, inserted, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(path, true)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	got, err = store.Get(first.Verified.ProspectID)
	if err != nil || !reflect.DeepEqual(got, first) {
		t.Fatalf("guarded receipt changed after reopen: %+v %v", got, err)
	}
	if _, _, err := store.RecordChecked(fixture.Context, fixture.Proof, accept); !errors.Is(err, ErrReadOnly) || calls != 4 {
		t.Fatal("read-only admission called guard")
	}
}
