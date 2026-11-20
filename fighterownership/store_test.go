//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterownership

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"path/filepath"
	"strings"
	"testing"
)

func TestOwnershipStoreClosedAuditAndUncertainWrite(t *testing.T) {
	c, pins := coreReference(t)
	var admission AdmissionStatement
	var pin Pin
	for _, v := range c.Positives {
		if v.Family == "admission" && v.Name == "v1/zero256-unawarded" {
			json.Unmarshal(v.Value, &admission)
			pin = pins[admission.PinID]
		}
	}
	id := StoreIdentity{strings.Repeat("74", 32)}
	path := filepath.Join(t.TempDir(), "ownership")
	s, e := Create(path, pin, id)
	if e != nil {
		t.Fatal(e)
	}
	initial, e := InitialState(pin, admission)
	if e != nil {
		t.Fatal(e)
	}
	batch := new(leveldb.Batch)
	raw, _ := json.Marshal(admission)
	batch.Put(countKey("a/", 1), raw)
	raw, _ = json.Marshal(initial)
	batch.Put(idKey("s/", initial.StateID), raw)
	original := s.writeBatch
	s.writeBatch = func(b *leveldb.Batch, o *opt.WriteOptions) error {
		if e := original(b, o); e != nil {
			return e
		}
		return errors.New("injected post-sync uncertainty")
	}
	if e = s.commit(batch, s.meta); !errors.Is(e, ErrOutcomeUnknown) {
		t.Fatal(e)
	}
	if _, e = s.Head(context.Background()); !errors.Is(e, ErrOutcomeUnknown) {
		t.Fatal("poisoned head", e)
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	s, e = Open(path, pin, id)
	if e != nil {
		t.Fatal(e)
	}
	if s.meta.Counts.Admissions != 1 || s.meta.Counts.States != 1 {
		t.Fatal("lost exact committed rows")
	}
	if _, e = s.Head(context.Background()); !errors.Is(e, ErrUnavailable) {
		t.Fatal("cached receipt minted a ready head", e)
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	wrong := id
	wrong.StoreID = strings.Repeat("75", 32)
	if owner, e := Open(path, pin, wrong); e == nil {
		owner.Close()
		t.Fatal("marker selected its own trust")
	}
	db, e := leveldb.OpenFile(filepath.Join(path, "db"), nil)
	if e != nil {
		t.Fatal(e)
	}
	db.Put([]byte("foreign"), []byte("{}"), &opt.WriteOptions{Sync: true})
	db.Close()
	if owner, e := OpenAudit(path, pin, id); e == nil {
		owner.Close()
		t.Fatal("unknown namespace accepted")
	}
}
func TestOwnershipRawBlockFramingBounds(t *testing.T) {
	// No header work is needed to refuse claimed counts before allocation.
	ctx := context.Background()
	for _, raw := range [][]byte{make([]byte, 80), append(make([]byte, 80), 255, 255, 255, 255, 255, 255, 255, 255, 255)} {
		h := HeaderRow{1, strings.Repeat("1", 64), 0, strings.Repeat("2", 64), hex.EncodeToString(make([]byte, 80))}
		if _, e := parseBlock(ctx, raw, h); e == nil {
			t.Fatal("unbounded/truncated block accepted")
		}
	}
}
