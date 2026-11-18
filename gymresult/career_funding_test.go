//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymresult

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

func TestCareerFundingPreservesExactActivationIntent(t *testing.T) {
	f := newResultFixture(t)
	activateFixture(t, f)
	if f.s.careerOpening || f.s.meta.Version != 2 || f.s.meta.Career == nil {
		t.Fatal("fixture is not a published active store")
	}
	intent, e := f.s.db.Get([]byte("career_init"), nil)
	if e != nil {
		t.Fatal(e)
	}
	coins, e := f.s.funding(context.Background())
	if e != nil || len(coins) != 0 {
		t.Fatal("activated empty funding", e, len(coins))
	}
	c, done := fixtureCall(t, f)
	prepared, e := f.s.prepare(c)
	done()
	if e != nil {
		t.Fatal(e)
	}
	// This installs modeled verified-network funding facts only. The source
	// authorization, activation, journal and projection are concrete; this is
	// not a fresh node ownership, admission or PoW claim.
	p := fundedPublisher(t, f, prepared.Record)
	coins, e = f.s.funding(context.Background())
	if e != nil || len(coins) != 1 || !equal(coins[0], p.publication.Coin) {
		t.Fatal("activated coin enumeration", e)
	}
	state, e := f.s.projectFunding(context.Background(), f.s.meta, new(leveldb.Batch))
	if e != nil || state != "available" {
		t.Fatal("activated funding projection", state, e)
	}
	batch := new(leveldb.Batch)
	if e = put(batch, coinKey('u', coins[0].Outpoint), reservation{1, prepared.Record.ResultKey, p.publication.TxID}, 256); e != nil {
		t.Fatal(e)
	}
	state, e = f.s.projectFunding(context.Background(), f.s.meta, batch)
	if e != nil || state != "waiting_for_funds" {
		t.Fatal("proposed reservation was ignored", state, e)
	}
	if e = f.s.Close(); e != nil {
		t.Fatal(e)
	}
	f.s, e = OpenCareerForTestClock(context.Background(), filepath.Join(f.dir, "result"), f.pin, f.bout, f.clock)
	if e != nil {
		t.Fatal("reopen", e)
	}
	state, e = f.s.fundingState(context.Background())
	if e != nil || state != "available" {
		t.Fatal("reopened funding", state, e)
	}
	after, e := f.s.db.Get([]byte("career_init"), nil)
	if e != nil || !bytes.Equal(intent, after) {
		t.Fatal("funding changed activation intent", e)
	}
}

func TestCareerFundingRefusesInvalidIntentAndCoinKeys(t *testing.T) {
	f := newResultFixture(t)
	activateFixture(t, f)
	valid := encoded(intentFor(f.s.meta))
	wrong := intentFor(f.s.meta)
	wrong.ResultCreatedAtMS++
	cases := []struct {
		name string
		key  []byte
		data []byte
	}{
		{"malformed-intent", []byte("career_init"), []byte("{}")},
		{"wrong-intent-identity", []byte("career_init"), encoded(wrong)},
		{"noncanonical-intent", []byte("career_init"), append(append([]byte{}, valid...), '\n')},
		{"oversized-intent", []byte("career_init"), bytes.Repeat([]byte{' '}, 2049)},
		{"similar-key", []byte("career_init_extra"), valid},
		{"short-coin-key", []byte("c"), []byte("{}")},
		{"malformed-exact-length-coin", append([]byte{'c'}, bytes.Repeat([]byte{'x'}, 36)...), []byte("{}")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if e := f.s.db.Put(tc.key, tc.data, &opt.WriteOptions{Sync: true}); e != nil {
				t.Fatal(e)
			}
			if got, e := f.s.funding(context.Background()); !errors.Is(e, ErrCorrupt) || got != nil {
				t.Fatal("accepted invalid metadata or coin", e)
			}
			if bytes.Equal(tc.key, []byte("career_init")) {
				if e := f.s.db.Put(tc.key, valid, &opt.WriteOptions{Sync: true}); e != nil {
					t.Fatal(e)
				}
			} else if e := f.s.db.Delete(tc.key, &opt.WriteOptions{Sync: true}); e != nil {
				t.Fatal(e)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if got, e := f.s.funding(ctx); !errors.Is(e, ErrCancelled) || got != nil {
		t.Fatal("ignored caller cancellation", e)
	}
}

func TestCareerFundingRefusesIntentInLegacyStore(t *testing.T) {
	f := newResultFixture(t)
	if e := f.s.db.Put([]byte("career_init"), encoded(intentFor(f.s.meta)), &opt.WriteOptions{Sync: true}); e != nil {
		t.Fatal(e)
	}
	if got, e := f.s.funding(context.Background()); !errors.Is(e, ErrCorrupt) || got != nil {
		t.Fatal("legacy funding accepted activation intent", e)
	}
}
