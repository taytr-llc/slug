// Copyright (c) 2026 The SLUG developers. ISC license.
package gymseed

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/taytr-llc/slug/gymevent"
	"path/filepath"
	"testing"
)

func TestSeedAuditReservationGraphAndRawBounds(t *testing.T) {
	requireNormal(t)
	ctx := context.Background()
	p, r := structuralRecord(t) // Modeled public receipt; no admitted-chain claim.
	events, e := gymevent.Create(ctx, filepath.Join(realTemp(t), "events"), p.SourcePin)
	if e != nil {
		t.Fatal(e)
	}
	defer events.Close()
	s, e := Create(ctx, filepath.Join(realTemp(t), "seed"), p, events)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	pub := syntheticPublication(t, p, r)
	r.Publication = &pub
	meta := s.meta
	meta.EventCount = 1
	meta.ClockMS = recordMaxTime(r)
	encoded, _ := json.Marshal(r)
	base := new(leveldb.Batch)
	if e = put(base, []byte("meta"), meta, MaxMetadataSize); e != nil {
		t.Fatal(e)
	}
	base.Put(key('r', r.EventKey), encoded)
	reservation := coinKey('u', pub.Coin.Outpoint)
	base.Put(reservation, rawID(r.EventKey))
	if e = s.db.Write(base, nil); e != nil {
		t.Fatal(e)
	}
	marker := marker{"slug-candidate-seeds", 1, meta.StoreID, p}
	if e = s.audit(ctx, marker); e != nil {
		t.Fatal("synthetic graph positive control", e)
	}
	cases := []struct {
		name       string
		key, value []byte
		remove     bool
	}{
		{"missing reservation", reservation, nil, true},
		{"wrong reservation event", reservation, rawID(hash("absent event")), false},
		{"reservation short value", reservation, []byte{1}, false},
		{"oversized record", key('r', r.EventKey), make([]byte, MaxRecordSize+1), false},
		{"unknown physical key", []byte("unexpected"), []byte("1"), false},
		{"init alongside meta", []byte("init"), []byte("{}"), false},
		{"malformed funding key", []byte{'f', 0}, []byte("{}"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			previous, e := s.db.Get(tc.key, nil)
			existed := e == nil
			if tc.remove {
				e = s.db.Delete(tc.key, nil)
			} else {
				e = s.db.Put(tc.key, tc.value, nil)
			}
			if e != nil {
				t.Fatal(e)
			}
			if e = s.audit(ctx, marker); !errors.Is(e, ErrCorrupt) {
				t.Fatal("bad physical graph accepted", e)
			}
			if existed {
				e = s.db.Put(tc.key, previous, nil)
			} else {
				e = s.db.Delete(tc.key, nil)
			}
			if e != nil {
				t.Fatal(e)
			}
			if e = s.audit(ctx, marker); e != nil {
				t.Fatal("restore positive control", e)
			}
		})
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if e = s.audit(cancelled, marker); !errors.Is(e, ErrCancelled) {
		t.Fatal("audit cancellation misclassified", e)
	}
}
