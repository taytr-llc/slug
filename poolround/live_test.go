// Copyright (c) 2026 The SLUG developers. ISC license.
package poolround

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/wire/v2"
)

func liveHarness(t *testing.T, version uint32) (*roundHarness, *Live) {
	t.Helper()
	h := newRoundProfileHarness(t, false, version)
	if _, err := h.store.Sync(context.Background()); err != nil {
		t.Fatal(err)
	}
	l, err := h.store.BeginLive(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return h, l
}
func liveReopen(t *testing.T, h *roundHarness) {
	t.Helper()
	if err := h.store.Close(); err != nil {
		t.Fatal(err)
	}
	var err error
	if h.limits.Version == Version2 {
		h.store, err = OpenDaily(context.Background(), h.path, h.limits, h.source, h.observer)
	} else {
		h.store, err = Open(h.path, h.limits, h.source, h.observer)
	}
	if err != nil {
		t.Fatal(err)
	}
}

func TestRoundLiveOriginalOwnerSyncAndRefresh(t *testing.T) {
	for _, version := range []uint32{Version1, Version2} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			h := newRoundProfileHarness(t, false, version)
			ctx, cancel := context.WithCancel(context.Background())
			l, err := h.store.BeginLive(ctx)
			if err != nil {
				t.Fatal(err)
			}
			cancel() // A successful operation's context is not owner lifetime.
			ctx = context.Background()
			before := walkTestRows(t, h.store)
			rows, err := l.List(ctx, 0)
			if err != nil || len(rows) != 0 || h.store.meta.Through != 0 || !reflect.DeepEqual(before, walkTestRows(t, h.store)) {
				t.Fatal("zero prefix imported newer source", err)
			}
			if same, err := h.store.BeginLive(ctx); err != nil || same != l {
				t.Fatal("healthy epoch replaced", err)
			}
			write := h.store.writeBatch
			writes := 0
			h.store.writeBatch = func(b *leveldb.Batch, o *opt.WriteOptions) error {
				writes++
				if o == nil || !o.Sync {
					t.Fatal("unsynchronized write")
				}
				return write(b, o)
			}
			if got, err := h.store.Sync(ctx); err != nil || got != (Summary{Through: 3, Rounds: 1}) || writes != 1 {
				t.Fatal("full Sync changed", got, writes, err)
			}
			rows, err = l.List(ctx, 0)
			full, fullErr := h.store.List(ctx, 0, MaxPage)
			if err != nil || fullErr != nil || !reflect.DeepEqual(rows, full) || len(rows) != 1 {
				t.Fatal("live/full mismatch", err, fullErr)
			}
			rows[0].Round.ShareKeys[0] = "changed"
			rows[0].Round.Winner.Proof.Header = "changed"
			again, err := l.List(ctx, 0)
			if err != nil || !reflect.DeepEqual(again, full) {
				t.Fatal("returned records aliased", err)
			}
			if _, err := h.store.Sync(ctx); err != nil || writes != 1 {
				t.Fatal("unchanged Sync wrote or revoked", writes, err)
			}
			first, err := l.Refresh(ctx, h.winner.Hash)
			if err != nil || first.Status() != "provisional" {
				t.Fatal(err)
			}
			h.observer.depth = 60
			locked, err := l.Refresh(ctx, h.winner.Hash)
			if err != nil || locked.Confirmation == nil {
				t.Fatal(err)
			}
			h.observer.canonical = false
			conflict, err := l.Refresh(ctx, h.winner.Hash)
			if err != nil || conflict.Status() != "funding-conflict" || !reflect.DeepEqual(conflict.Confirmation, locked.Confirmation) || !reflect.DeepEqual(conflict.Round, full[0].Round) {
				t.Fatal("lock/allocation changed", err)
			}
			walk := walkTestRead(t, h.store, h.sourceConfig)
			if len(walk) != 1 || !reflect.DeepEqual(walk[0], conflict) {
				t.Fatal("financial full audit differs")
			}
			h.observer.canonical = true
			h.observer.depth = 80
			legacy, err := h.store.Refresh(ctx, h.winner.Hash)
			got, readErr := l.List(ctx, 0)
			if err != nil || readErr != nil || len(got) != 1 || !reflect.DeepEqual(got[0], legacy) || !reflect.DeepEqual(legacy.Confirmation, locked.Confirmation) {
				t.Fatal("legacy Refresh did not maintain exact directory", err, readErr)
			}
		})
	}
}

func TestRoundLiveLateWinnerAndReopen(t *testing.T) {
	h := newRoundHarness(t, true)
	ctx := context.Background()
	if got, err := h.store.Sync(ctx); err != nil || got != (Summary{Through: 2, PendingShares: 2}) {
		t.Fatal(got, err)
	}
	old, err := h.store.BeginLive(ctx)
	if err != nil {
		t.Fatal(err)
	}
	liveReopen(t, h)
	if _, err := old.List(ctx, 0); !errors.Is(err, ErrClosed) {
		t.Fatal("old owner survived reopen", err)
	}
	l, err := h.store.BeginLive(ctx)
	if err != nil {
		t.Fatal(err)
	}
	h.poolNode.canonical = true
	late, err := h.source.Submit(ctx, roundToken, h.input.Job.ID, h.winner.Nonce)
	if err != nil || late.Sequence != 1 || late.AcceptedSequence != 3 || late.Acceptance != "canonical-late-winner" {
		t.Fatal(late, err)
	}
	if got, err := h.store.Sync(ctx); err != nil || got != (Summary{Through: 3, Rounds: 1}) {
		t.Fatal(got, err)
	}
	rows, err := l.List(ctx, 0)
	if err != nil || len(rows) != 1 || rows[0].Round.FirstAccepted != 1 || rows[0].Round.LastAccepted != 3 || rows[0].Round.Winner.Share.Sequence != 1 {
		t.Fatal(rows, err)
	}
	if duplicate, err := h.source.Submit(ctx, roundToken, h.input.Job.ID, h.winner.Nonce); err != nil || duplicate != late {
		t.Fatal("receipt changed", err)
	}
	if _, err := h.store.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if got := walkTestRead(t, h.store, h.sourceConfig); !reflect.DeepEqual(got, rows) {
		t.Fatal("late duplicate changed original round")
	}
}

func TestRoundLiveTouchedCorruptionRevokes(t *testing.T) {
	for _, mode := range []string{"row", "index", "meta", "well-formed-observation"} {
		t.Run(mode, func(t *testing.T) {
			h, l := liveHarness(t, Version1)
			ctx := context.Background()
			b := new(leveldb.Batch)
			switch mode {
			case "row":
				b.Delete(roundKey(h.winner.Hash))
			case "index":
				b.Put(indexKey(1), []byte(h.input.Job.ID))
			case "meta":
				b.Put([]byte("meta"), []byte("{}"))
			case "well-formed-observation":
				r, err := h.store.Get(ctx, h.winner.Hash)
				if err != nil {
					t.Fatal(err)
				}
				r.Observation = &prospectnode.PoolBlockObservation{BlockHash: r.Round.BlockHash, BlockHeight: r.Round.BlockHeight, TipHash: r.Round.BlockHash, TipHeight: r.Round.BlockHeight, Canonical: true}
				raw, err := recordBytes(r)
				if err != nil {
					t.Fatal(err)
				}
				b.Put(roundKey(h.winner.Hash), raw)
			}
			if err := h.store.db.Write(b, &opt.WriteOptions{Sync: true}); err != nil {
				t.Fatal(err)
			}
			before := walkTestRows(t, h.store)
			if _, err := l.Refresh(ctx, h.winner.Hash); !errors.Is(err, ErrCorrupt) || h.observer.calls != 0 {
				t.Fatal("corrupt touched row observed node", err)
			}
			if _, err := l.List(ctx, 0); !errors.Is(err, ErrLiveStale) || h.store.live != nil {
				t.Fatal("corrupt epoch survived", err)
			}
			if !reflect.DeepEqual(before, walkTestRows(t, h.store)) {
				t.Fatal("corruption was repaired")
			}
		})
	}
}

func TestRoundLiveDoesNotWeakenFullAudits(t *testing.T) {
	for _, api := range []string{"sync", "get", "list", "refresh", "walk"} {
		t.Run(api, func(t *testing.T) {
			h, l := liveHarness(t, Version1)
			ctx := context.Background()
			if err := h.store.db.Put([]byte("unrelated"), []byte("must remain visible to full audits"), &opt.WriteOptions{Sync: true}); err != nil {
				t.Fatal(err)
			}
			before := walkTestRows(t, h.store)
			// Deliberate selected contract: this unrelated key is not touched.
			if rows, err := l.List(ctx, 0); err != nil || len(rows) != 1 {
				t.Fatal("live scope unexpectedly became full audit", err)
			}
			var err error
			switch api {
			case "sync":
				_, err = h.store.Sync(ctx)
			case "get":
				_, err = h.store.Get(ctx, h.winner.Hash)
			case "list":
				_, err = h.store.List(ctx, 0, MaxPage)
			case "refresh":
				_, err = h.store.Refresh(ctx, h.winner.Hash)
			case "walk":
				called := false
				var cfg poolshare.Config
				cfg, err = h.store.WalkSettlement(ctx, func([]Record) error { called = true; return nil })
				if cfg != (poolshare.Config{}) || called {
					t.Fatal("financial audit disclosed corrupt source")
				}
			}
			if !errors.Is(err, ErrCorrupt) || h.observer.calls != 0 || h.store.live != nil {
				t.Fatal("full API weakened", api, err)
			}
			if _, err := l.List(ctx, 0); !errors.Is(err, ErrLiveStale) {
				t.Fatal("rejected full audit rescued by live", err)
			}
			if !reflect.DeepEqual(before, walkTestRows(t, h.store)) {
				t.Fatal("full audit altered evidence")
			}
		})
	}
}

func TestRoundLiveUncertainWriteAndExplicitReanchor(t *testing.T) {
	for _, after := range []bool{false, true} {
		t.Run(fmt.Sprint(after), func(t *testing.T) {
			h, l := liveHarness(t, Version1)
			ctx := context.Background()
			write := h.store.writeBatch
			h.store.writeBatch = func(b *leveldb.Batch, o *opt.WriteOptions) error {
				if o == nil || !o.Sync {
					t.Fatal("unsynchronized")
				}
				if after {
					if e := write(b, o); e != nil {
						return e
					}
				}
				return errors.New("independent lost sync acknowledgment")
			}
			h.observer.depth = 60
			if _, err := l.Refresh(ctx, h.winner.Hash); !errors.Is(err, ErrPoisoned) || h.store.live != nil {
				t.Fatal("uncertain write usable", err)
			}
			if _, err := l.List(ctx, 0); !errors.Is(err, ErrPoisoned) {
				t.Fatal(err)
			}
			liveReopen(t, h)
			l2, err := h.store.BeginLive(ctx)
			if err != nil {
				t.Fatal(err)
			}
			rows, err := l2.List(ctx, 0)
			if err != nil || len(rows) != 1 || (rows[0].Confirmation != nil) != after {
				t.Fatal("reopen did not audit exact persisted outcome", err)
			}
			locked, err := l2.Refresh(ctx, h.winner.Hash)
			if err != nil || locked.Confirmation == nil {
				t.Fatal(err)
			}
			if after && !reflect.DeepEqual(locked.Confirmation, rows[0].Confirmation) {
				t.Fatal("lost acknowledgement replaced first lock")
			}
		})
	}
	t.Run("unaware-private-mutator", func(t *testing.T) {
		h, l := liveHarness(t, Version1)
		ctx := context.Background()
		end, err := h.store.begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		err = h.store.commit(new(leveldb.Batch), h.store.meta)
		end()
		if err != nil {
			t.Fatal(err)
		}
		if _, err = l.List(ctx, 0); !errors.Is(err, ErrLiveStale) {
			t.Fatal("unaware commit retained epoch", err)
		}
		next, err := h.store.BeginLive(ctx)
		if err != nil || next == l {
			t.Fatal("explicit full reanchor failed", err)
		}
		if rows, err := next.List(ctx, 0); err != nil || len(rows) != 1 {
			t.Fatal(err)
		}
	})
}

func TestRoundLiveSourceFencesCancellationAndOwnership(t *testing.T) {
	for _, when := range []string{"before", "during", "cancel-during"} {
		t.Run(when, func(t *testing.T) {
			h, l := liveHarness(t, Version1)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			before := walkTestRows(t, h.store)
			if when == "before" {
				if err := h.source.Close(); err != nil {
					t.Fatal(err)
				}
			} else {
				h.observer.change = func(*prospectnode.PoolBlockObservation) {
					if when == "cancel-during" {
						cancel()
					} else if err := h.source.Close(); err != nil {
						t.Fatal(err)
					}
				}
			}
			_, err := l.Refresh(ctx, h.winner.Hash)
			want := ErrSourceUnavailable
			if when == "cancel-during" {
				want = context.Canceled
			}
			if !errors.Is(err, want) || (when == "before" && h.observer.calls != 0) {
				t.Fatal("source fence failed", err)
			}
			if !reflect.DeepEqual(before, walkTestRows(t, h.store)) {
				t.Fatal("failed final fence committed")
			}
			if when == "before" {
				if _, err := l.List(context.Background(), 0); !errors.Is(err, ErrSourceUnavailable) {
					t.Fatal("closed source disclosed list", err)
				}
			}
		})
	}
	t.Run("original-handle", func(t *testing.T) {
		h, l := liveHarness(t, Version1)
		ctx := context.Background()
		copied := *l
		for _, bad := range []*Live{nil, {}, &copied, {store: h.store, directory: &liveDirectory{}}} {
			if _, err := bad.List(ctx, 0); !errors.Is(err, ErrLiveStale) {
				t.Fatal("forged handle", err)
			}
		}
		before := walkTestRows(t, h.store)
		c, cancel := context.WithCancel(ctx)
		cancel()
		if _, err := h.store.BeginLive(c); !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		if _, err := l.List(c, 0); !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(before, walkTestRows(t, h.store)) {
			t.Fatal("canceled selection wrote")
		}
	})
}

func TestRoundLiveOriginalCloseJoins(t *testing.T) {
	h, l := liveHarness(t, Version1)
	ctx := context.Background()
	entered, release := make(chan struct{}), make(chan struct{})
	released := false
	h.observer.change = func(*prospectnode.PoolBlockObservation) { close(entered); <-release }
	done := make(chan error, 1)
	go func() { _, e := l.Refresh(ctx, h.winner.Hash); done <- e }()
	joined := false
	defer func() {
		if !released {
			close(release)
		}
		if !joined {
			select {
			case <-done:
			case <-time.After(10 * time.Second):
				t.Error("refresh did not join")
			}
		}
	}()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("observer not entered")
	}
	if _, e := l.List(ctx, 0); !errors.Is(e, ErrBusy) {
		t.Fatal("gate queued", e)
	}
	closed := make(chan error, 1)
	go func() { closed <- h.store.Close() }()
	closeJoined := false
	defer func() {
		if !released {
			close(release)
			released = true
		}
		if !closeJoined {
			select {
			case <-closed:
			case <-time.After(10 * time.Second):
				t.Error("Close did not join")
			}
		}
	}()
	deadline := time.Now().Add(10 * time.Second)
	for {
		h.store.lifecycle.Lock()
		closing := h.store.closing
		h.store.lifecycle.Unlock()
		if closing {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Close did not begin")
		}
		time.Sleep(time.Millisecond)
	}
	if _, e := l.List(ctx, 0); !errors.Is(e, ErrClosed) {
		t.Fatal("closing owner admitted read", e)
	}
	select {
	case e := <-closed:
		closeJoined = true
		t.Fatal("Close passed admitted operation", e)
	default:
	}
	close(release)
	released = true
	select {
	case e := <-done:
		joined = true
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("refresh did not join")
	}
	select {
	case e := <-closed:
		closeJoined = true
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Close did not join")
	}
	if _, e := l.Refresh(ctx, h.winner.Hash); !errors.Is(e, ErrClosed) {
		t.Fatal("closed epoch survived", e)
	}
}

func TestRoundLiveCheapProfileRefusal(t *testing.T) {
	if !wire.TestPoW {
		t.Skip("cheap-profile refusal control")
	}
	var s Store
	var l Live
	if _, e := s.BeginLive(context.Background()); !errors.Is(e, ErrProfile) {
		t.Fatal(e)
	}
	if _, e := l.List(context.Background(), 0); !errors.Is(e, ErrProfile) {
		t.Fatal(e)
	}
	if _, e := l.Refresh(context.Background(), ""); !errors.Is(e, ErrProfile) {
		t.Fatal(e)
	}
}

func TestRoundLiveReadOnlyRefusal(t *testing.T) {
	for _, version := range []uint32{Version1, Version2} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			h, old := liveHarness(t, version)
			ctx := context.Background()
			before := walkTestRows(t, h.store)
			if err := h.store.Close(); err != nil {
				t.Fatal(err)
			}
			var err error
			if version == Version2 {
				h.store, err = OpenDailyAudit(ctx, h.path, h.limits, h.source)
			} else {
				h.store, err = OpenAudit(h.path, h.limits, h.source)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := h.store.BeginLive(ctx); !errors.Is(err, ErrInvalid) || h.store.live != nil {
				t.Fatal("audit owner selected live mode", err)
			}
			if _, err := old.List(ctx, 0); !errors.Is(err, ErrClosed) {
				t.Fatal("old owner rebound to audit handle", err)
			}
			if rows := walkTestRead(t, h.store, h.sourceConfig); len(rows) != 1 || !reflect.DeepEqual(before, walkTestRows(t, h.store)) {
				t.Fatal("read-only refusal altered evidence")
			}
		})
	}
}

func TestRoundLiveFullSyncFailureIsAtomic(t *testing.T) {
	for _, mode := range []string{"late-namespace-corruption", "capacity", "corruption-before-capacity"} {
		t.Run(mode, func(t *testing.T) {
			h := newRoundHarness(t, false)
			ctx := context.Background()
			if mode != "late-namespace-corruption" {
				if err := h.store.Close(); err != nil {
					t.Fatal(err)
				}
				h.limits.MaxBytes = statusReserve // Exact valid empty journal, insufficient for its real round.
				h.path = filepath.Join(t.TempDir(), "bounded")
				var err error
				h.store, err = Create(h.path, h.limits, h.source, h.observer)
				if err != nil {
					t.Fatal(err)
				}
			}
			l, err := h.store.BeginLive(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if mode != "capacity" {
				if err := h.store.db.Put([]byte("unexpected-after-source-tail"), []byte("corrupt"), &opt.WriteOptions{Sync: true}); err != nil {
					t.Fatal(err)
				}
			}
			before, meta := walkTestRows(t, h.store), h.store.meta
			writes := 0
			h.store.writeBatch = func(*leveldb.Batch, *opt.WriteOptions) error { writes++; return errors.New("unexpected write") }
			want := ErrCorrupt
			if mode == "capacity" {
				want = ErrCapacity
			}
			if got, err := h.store.Sync(ctx); !errors.Is(err, want) || got != (Summary{}) || writes != 0 || h.store.meta != meta || !reflect.DeepEqual(before, walkTestRows(t, h.store)) {
				t.Fatal("failed full Sync exposed candidate tail", got, writes, err)
			}
			if mode == "capacity" {
				if rows, err := l.List(ctx, 0); err != nil || len(rows) != 0 {
					t.Fatal("definite capacity invalidated healthy original epoch", err)
				}
			} else if _, err := l.List(ctx, 0); !errors.Is(err, ErrLiveStale) {
				t.Fatal("corrupt full Sync retained epoch", err)
			}
		})
	}
}
