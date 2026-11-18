//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymbout

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymseed"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBoutConcreteRunReplayOwnershipAndReopen(t *testing.T) {
	f := newBoutFixture(t)
	ctx := context.Background()
	calls := 0
	f.s.checkpoint = func(stage string) {
		if stage == "before-engine" {
			calls++
			if f.inLease.Load() != 0 {
				t.Fatal("engine inside account lease")
			}
			if _, e := f.seed.Info(ctx); e != nil {
				t.Fatal("seed gate held during preparation", e)
			}
			if _, e := f.event.Info(ctx); e != nil {
				t.Fatal("event gate held during preparation", e)
			}
			if _, e := f.entry.Status(ctx, f.session, f.date()); e != nil {
				t.Fatal("entry gate held during preparation", e)
			}
			if _, e := f.stable.Status(ctx, f.session); e != nil {
				t.Fatal("stable gate held during preparation", e)
			}
		}
	}
	got, e := f.run(ctx)
	if e != nil {
		t.Fatal(e)
	}
	if got.Replayed || calls != 1 || !got.Status.Saved.Usable {
		t.Fatal("new run", calls, got.Replayed)
	}
	if _, e = ValidateRunResult(ctx, f.p, got); e != nil {
		t.Fatal("result DTO", e)
	}
	var original Record
	originalRaw, _ := json.Marshal(got.Record)
	json.Unmarshal(originalRaw, &original)
	got.Record.Source.Corners[0].Member.Assignment.WorkHash = strings.Repeat("0", 64)
	info, e := f.s.Info(ctx)
	if e != nil {
		t.Fatal(e)
	}
	info.Pin.SourcePin.SourcePin.SourcePin.Participants[0].StablePin.AccountID = strings.Repeat("f", 64)
	again, e := f.run(ctx)
	if e != nil || !again.Replayed || calls != 1 || !equal(again.Record, original) {
		t.Fatal("owned replay", e, calls)
	}
	// A callback may mutate its owned Pin but never the source's authority.
	if e = f.card.WithBoutView(ctx, f.seed, f.event, f.entry, f.session, f.date(), func(v gymcard.BoutView, check func() error) error {
		v.Pin.SourcePin.SourcePin.Participants[0].StablePin.AccountID = strings.Repeat("f", 64)
		v.Bouts[0].SourceDigest = strings.Repeat("0", 64)
		return check()
	}); e != nil {
		t.Fatal(e)
	}
	if _, e = f.status(ctx); e != nil {
		t.Fatal("callback alias", e)
	}
	f.s.Close()
	f.s, e = open(ctx, filepath.Join(f.dir, "bout"), f.p, f.card, f.clock, false)
	if e != nil {
		t.Fatal(e)
	}
	read, e := f.s.Read(ctx, f.card, f.seed, f.event, f.entry, f.session, f.date(), f.id())
	if e != nil || !equal(read.Record, original) {
		t.Fatal("reopen exact bytes", e)
	}
	// Independently modeled conflict is retained; it changes only fresh overlay.
	r := f.seedRecord
	f.now.Add(1)
	r.Conflicts = append(r.Conflicts, gymseed.Conflict{Kind: "chain", ObservedAtMS: f.now.Load(), ObservedTip: &gymseed.Tip{Height: r.Target.Height, Hash: strings.Repeat("e", 64)}, ExpectedHeight: r.Target.Height, ExpectedHash: r.Target.Hash, ObservedHash: strings.Repeat("f", 64)})
	r.Observation.CurrentFinality = false
	f.replaceSeed(t, r)
	replay, e := f.run(ctx)
	if e != nil || !replay.Replayed || replay.Status.Saved.Usable || !equal(replay.Record, original) {
		t.Fatal("conflict replay", e)
	}
	other := f.cardRecord.Plan.Bouts[1].BoutID
	if _, e = f.s.Run(ctx, f.card, f.seed, f.event, f.entry, f.session, f.date(), other); !errors.Is(e, ErrSourceConflict) {
		t.Fatal("new run with conflict", e)
	}
	f.revoke.Store(true)
	if _, e = f.status(ctx); !errors.Is(e, ErrUnauthorized) {
		t.Fatal("original view refusal", e)
	}
}

func TestBoutPreparationRevocationAndLostSync(t *testing.T) {
	f := newBoutFixture(t)
	ctx := context.Background()
	calls := 0
	f.s.checkpoint = func(stage string) {
		if stage == "before-engine" {
			calls++
		}
		if stage == "run-prepared" {
			f.revoke.Store(true)
		}
	}
	if _, e := f.run(ctx); e == nil {
		t.Fatal("revoked preparation published")
	}
	if f.s.meta.BoutCount != 0 || calls != 1 {
		t.Fatal("revoked write or wrong engine count")
	}
	f.revoke.Store(false)
	original := f.s.writeBatch
	f.s.checkpoint = func(stage string) {
		if stage == "before-result-sync" {
			f.s.writeBatch = func(b *leveldb.Batch, o *opt.WriteOptions) error {
				if e := original(b, o); e != nil {
					return e
				}
				return errors.New("lost sync ACK")
			}
		}
	}
	if _, e := f.run(ctx); !errors.Is(e, ErrOutcomeUnknown) {
		t.Fatal("lost ACK", e)
	}
	if _, e := f.s.Info(ctx); !errors.Is(e, ErrPoisoned) {
		t.Fatal("poison", e)
	}
	if len(f.s.index) != 0 {
		t.Fatal("failed sync published index")
	}
	f.s.Close()
	var e error
	f.s, e = open(ctx, filepath.Join(f.dir, "bout"), f.p, f.card, f.clock, false)
	if e != nil {
		t.Fatal(e)
	}
	f.s.checkpoint = func(stage string) {
		if stage == "before-engine" {
			t.Fatal("lost ACK resimulated")
		}
	}
	got, e := f.run(ctx)
	if e != nil || !got.Replayed {
		t.Fatal("recover exact committed result", e)
	}
	f.now.Add(1)
	f.s.checkpoint = func(stage string) {
		if stage == "read-observed" {
			f.revoke.Store(true)
		}
	}
	if _, e = f.status(ctx); !errors.Is(e, ErrOutcomeUnknown) {
		t.Fatal("revoked after clock sync", e)
	}
}

func TestBoutSourceMissingCardCannotHideSavedRows(t *testing.T) {
	f := newBoutFixture(t)
	ctx := context.Background()
	if _, e := f.run(ctx); e != nil {
		t.Fatal(e)
	}
	// Same pin and creation instant, but a distinct empty card incarnation.
	empty, e := gymcard.CreateForTestClock(ctx, filepath.Join(f.dir, "empty-card"), f.p.SourcePin, f.seed, f.clock)
	if e != nil {
		t.Fatal(e)
	}
	defer empty.Close()
	for _, selected := range []bool{false, true} {
		var e error
		if selected {
			_, e = f.s.Status(ctx, empty, f.seed, f.event, f.entry, f.session, f.date(), f.id())
		} else {
			_, e = f.s.CardStatus(ctx, empty, f.seed, f.event, f.entry, f.session, f.date())
		}
		if !errors.Is(e, ErrSourceChanged) {
			t.Fatal("missing retained source hidden", selected, e)
		}
	}
}

func TestBoutCancellationAndCloseJoinPreparation(t *testing.T) {
	f := newBoutFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	entered, release := make(chan struct{}), make(chan struct{})
	f.s.checkpoint = func(stage string) {
		if stage == "before-engine" {
			close(entered)
			<-release
		}
	}
	result := make(chan error, 1)
	go func() { _, e := f.run(ctx); result <- e }()
	<-entered
	cancel()
	close(release)
	if e := <-result; !errors.Is(e, ErrCancelled) {
		t.Fatal("caller cancellation", e)
	}
	if f.s.meta.BoutCount != 0 {
		t.Fatal("cancelled write")
	}
	f.s.checkpoint = nil
	ctx = context.Background()
	entered, release = make(chan struct{}), make(chan struct{})
	f.s.checkpoint = func(stage string) {
		if stage == "run-prepared" {
			close(entered)
			<-release
		}
	}
	go func() { _, e := f.run(ctx); result <- e }()
	<-entered
	closed := make(chan error, 1)
	go func() { closed <- f.s.Close() }()
	select {
	case <-closed:
		t.Fatal("close failed to join preparation")
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	if e := <-result; e == nil {
		t.Fatal("closed source published")
	}
	if e := <-closed; e != nil {
		t.Fatal(e)
	}
}

func TestBoutConflictAndSourceClosureDuringEngineGap(t *testing.T) {
	for _, kind := range []string{"conflict", "source-close"} {
		t.Run(kind, func(t *testing.T) {
			f := newBoutFixture(t)
			ctx := context.Background()
			f.s.checkpoint = func(stage string) {
				if stage != "after-engine" {
					return
				}
				if f.inLease.Load() != 0 {
					t.Fatal("source lease spans engine")
				}
				if kind == "source-close" {
					if e := f.seed.Close(); e != nil {
						t.Fatal(e)
					}
					return
				}
				v := f.seedRecord.Declaration
				v.Declaration.SnapshotDigest = strings.Repeat("e", 64)
				raw, _ := json.Marshal(v.Declaration)
				seed := sha256.Sum256([]byte("SLUG candidate seed declaration fixture\x00v1\x00"))
				v.Signature = hex.EncodeToString(ed25519.Sign(ed25519.NewKeyFromSeed(seed[:]), append([]byte("SLUG candidate seed declaration\x00v1\x00"), lp(raw)...)))
				if _, e := f.seed.ObserveEquivocation(ctx, f.event, f.entry, f.session, f.date(), v); e != nil {
					t.Fatal("source mutation could not run outside gate", e)
				}
			}
			_, e := f.run(ctx)
			want := ErrSourceConflict
			if kind == "source-close" {
				want = ErrUnavailable
			}
			if !errors.Is(e, want) || f.s.meta.BoutCount != 0 {
				t.Fatal("source changed during preparation published", e, want)
			}
		})
	}
}

func TestBoutConcurrentSameBookingCommitsOneResult(t *testing.T) {
	f := newBoutFixture(t)
	ctx := context.Background()
	engine := 0
	f.s.checkpoint = func(stage string) {
		if stage == "before-engine" {
			engine++
		}
	}
	type answer struct {
		v RunResult
		e error
	}
	ch := make(chan answer, 5)
	for i := 0; i < 5; i++ {
		go func() { v, e := f.run(ctx); ch <- answer{v, e} }()
	}
	first := ""
	created := int64(0)
	newCount := 0
	for i := 0; i < 5; i++ {
		a := <-ch
		if a.e != nil {
			t.Fatal(a.e)
		}
		if !a.v.Replayed {
			newCount++
		}
		d := recordDigest(a.v.Record)
		if first == "" {
			first = d
			created = a.v.Record.CreatedAtMS
		}
		if first != d || created != a.v.Record.CreatedAtMS {
			t.Fatal("concurrent result changed")
		}
	}
	if newCount != 1 || engine != 1 || f.s.meta.BoutCount != 1 {
		t.Fatal("not exactly one insertion", newCount, engine, f.s.meta.BoutCount)
	}
}
