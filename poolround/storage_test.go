// Copyright (c) 2026 The SLUG developers. ISC license.
package poolround

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/wire/v2"
)

func TestRoundUncertainWritesRecoverWithoutDuplicateOrLostLock(t *testing.T) {
	for _, phase := range []string{"round", "confirmation"} {
		for _, afterSync := range []bool{false, true} {
			name := phase + "-before-sync"
			if afterSync {
				name = phase + "-after-sync"
			}
			t.Run(name, func(t *testing.T) {
				h := newRoundHarness(t, false)
				ctx := context.Background()
				if phase == "confirmation" {
					if _, err := h.store.Sync(ctx); err != nil {
						t.Fatal(err)
					}
				}
				write := h.store.writeBatch
				fired := false
				h.store.writeBatch = func(batch *leveldb.Batch, options *opt.WriteOptions) error {
					if options == nil || !options.Sync {
						t.Fatal("round mutation was not synchronized")
					}
					if !fired {
						fired = true
						if afterSync {
							if err := write(batch, options); err != nil {
								return err
							}
						}
						return errors.New("uncertain fixture write")
					}
					return write(batch, options)
				}
				var err error
				if phase == "round" {
					_, err = h.store.Sync(ctx)
				} else {
					h.observer.depth = 60
					_, err = h.store.Refresh(ctx, h.winner.Hash)
				}
				if !fired || !errors.Is(err, ErrPoisoned) {
					t.Fatal("uncertain mutation did not poison handle", err)
				}
				if _, err := h.store.Get(ctx, h.winner.Hash); !errors.Is(err, ErrPoisoned) {
					t.Fatal("poisoned journal served evidence", err)
				}
				h.reopen(t)
				saved, err := h.store.Get(ctx, h.winner.Hash)
				if phase == "round" && !afterSync {
					if !errors.Is(err, ErrNotFound) {
						t.Fatal("unwritten round appeared", err)
					}
				} else if err != nil {
					t.Fatal("synced evidence lost", err)
				}
				if phase == "confirmation" && ((saved.Confirmation != nil) != afterSync) {
					t.Fatal("reopen misclassified uncertain confirmation")
				}
				var original *prospectnode.PoolBlockObservation
				if saved.Confirmation != nil {
					v := *saved.Confirmation
					original = &v
				}
				if _, err := h.store.Sync(ctx); err != nil {
					t.Fatal(err)
				}
				h.observer.depth = 80
				final, err := h.store.Refresh(ctx, h.winner.Hash)
				if err != nil || final.Round.Sequence != 1 || final.Confirmation == nil {
					t.Fatal("exact mutation recovery failed", err)
				}
				if original != nil && *final.Confirmation != *original {
					t.Fatal("acknowledgment loss replaced first confirmation")
				}
				rows, err := h.store.List(ctx, 0, 8)
				if err != nil || len(rows) != 1 {
					t.Fatal("recovery duplicated round", err)
				}
			})
		}
	}
}

func TestRoundFullCapacityStillUpdatesConfirmation(t *testing.T) {
	h := newRoundHarness(t, false)
	ctx := context.Background()
	if _, err := h.store.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	budget := h.store.meta.Bytes
	if err := h.store.Close(); err != nil {
		t.Fatal(err)
	}
	h.path = filepath.Join(t.TempDir(), "full-rounds")
	h.limits = Limits{Version: 1, MaxRounds: 1, MaxBytes: budget}
	var err error
	h.store, err = Create(h.path, h.limits, h.source, h.observer)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.store.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if h.store.meta.Bytes != h.limits.MaxBytes || h.store.meta.Rounds != h.limits.MaxRounds {
		t.Fatal("test did not exhaust both round capacities")
	}
	h.reopen(t)
	h.observer.depth = 60
	locked, err := h.store.Refresh(ctx, h.winner.Hash)
	if err != nil || locked.Confirmation == nil {
		t.Fatal("full journal prevented confirmation", err)
	}
	h.observer.canonical = false
	conflict, err := h.store.Refresh(ctx, h.winner.Hash)
	if err != nil || conflict.Status() != "funding-conflict" || !reflect.DeepEqual(conflict.Round, locked.Round) {
		t.Fatal("full journal prevented conflict report", err)
	}
}

func TestRoundOpenRejectsCorruptionAndConfigurationChange(t *testing.T) {
	for _, mode := range []string{"member", "allocation", "confirmation", "index", "extra-key", "limits"} {
		t.Run(mode, func(t *testing.T) {
			h := newRoundHarness(t, false)
			ctx := context.Background()
			if _, err := h.store.Sync(ctx); err != nil {
				t.Fatal(err)
			}
			r, err := h.store.Get(ctx, h.winner.Hash)
			if err != nil {
				t.Fatal(err)
			}
			if err := h.store.Close(); err != nil {
				t.Fatal(err)
			}
			if mode == "limits" {
				h.limits.MaxRounds--
			} else {
				db, err := leveldb.OpenFile(filepath.Join(h.path, "db"), &opt.Options{ErrorIfMissing: true})
				if err != nil {
					t.Fatal(err)
				}
				batch := new(leveldb.Batch)
				switch mode {
				case "member":
					r.Round.ShareKeys[0] = r.Round.ShareKeys[1]
				case "allocation":
					r.Round.Allocations[0].Numerator = "999999999"
				case "confirmation":
					r.Confirmation = &prospectnode.PoolBlockObservation{BlockHash: r.Round.BlockHash, BlockHeight: r.Round.BlockHeight, TipHash: r.Round.BlockHash, TipHeight: r.Round.BlockHeight, Canonical: true}
				case "index":
					batch.Put(indexKey(1), []byte(h.input.Job.ID))
				case "extra-key":
					batch.Put([]byte("unexpected"), []byte("entry"))
				}
				if mode == "member" || mode == "allocation" || mode == "confirmation" {
					raw, e := json.Marshal(r)
					if e != nil {
						t.Fatal(e)
					}
					batch.Put(roundKey(r.Round.BlockHash), raw)
				}
				if db.Write(batch, &opt.WriteOptions{Sync: true}) != nil {
					t.Fatal("write corruption fixture")
				}
				if db.Close() != nil {
					t.Fatal("close corruption fixture")
				}
			}
			opened, err := Open(h.path, h.limits, h.source, h.observer)
			if err == nil {
				_ = opened.Close()
				t.Fatal("changed journal/configuration reopened")
			}
		})
	}
}

func TestRoundFinalSourceGuardAndCloseDrain(t *testing.T) {
	t.Run("source-closes-during-observation", func(t *testing.T) {
		h := newRoundHarness(t, false)
		ctx := context.Background()
		if _, err := h.store.Sync(ctx); err != nil {
			t.Fatal(err)
		}
		h.observer.depth = 60
		h.observer.change = func(*prospectnode.PoolBlockObservation) {
			if err := h.source.Close(); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := h.store.Refresh(ctx, h.winner.Hash); !errors.Is(err, ErrSourceUnavailable) {
			t.Fatal("closed source granted new confirmation", err)
		}
		raw, err := h.store.db.Get(roundKey(h.winner.Hash), nil)
		if err != nil {
			t.Fatal(err)
		}
		var saved Record
		if json.Unmarshal(raw, &saved) != nil || saved.Confirmation != nil || saved.Observation != nil {
			t.Fatal("failed final source guard altered stored round")
		}
	})
	t.Run("busy-and-drain", func(t *testing.T) {
		h := newRoundHarness(t, false)
		ctx := context.Background()
		if _, err := h.store.Sync(ctx); err != nil {
			t.Fatal(err)
		}
		entered, release := make(chan struct{}), make(chan struct{})
		h.observer.change = func(*prospectnode.PoolBlockObservation) { close(entered); <-release }
		done := make(chan error, 1)
		go func() { _, err := h.store.Refresh(ctx, h.winner.Hash); done <- err }()
		select {
		case <-entered:
		case <-time.After(10 * time.Second):
			t.Fatal("refresh did not reach observer")
		}
		if _, err := h.store.Get(ctx, h.winner.Hash); !errors.Is(err, ErrBusy) {
			close(release)
			t.Fatal("concurrent operation queued", err)
		}
		closed := make(chan error, 1)
		go func() { closed <- h.store.Close() }()
		select {
		case err := <-closed:
			close(release)
			t.Fatal("close did not drain active operation", err)
		default:
		}
		close(release)
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("refresh failed to drain")
		}
		select {
		case err := <-closed:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(10 * time.Second):
			t.Fatal("close failed to finish")
		}
		if _, err := h.store.Get(ctx, h.winner.Hash); !errors.Is(err, ErrClosed) {
			t.Fatal("closed store served round", err)
		}
	})
}

func TestRoundCheapProfileRefusesBeforeStorage(t *testing.T) {
	if !wire.TestPoW {
		t.Skip("explicit cheap profile")
	}
	path := filepath.Join(t.TempDir(), "not-created")
	if _, err := Create(path, Limits{}, nil, nil); !errors.Is(err, ErrProfile) {
		t.Fatal("cheap create", err)
	}
	if _, err := Open(path, Limits{}, nil, nil); !errors.Is(err, ErrProfile) {
		t.Fatal("cheap open", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("cheap profile touched storage", err)
	}
	s := new(Store)
	ctx := context.Background()
	if _, err := s.Sync(ctx); !errors.Is(err, ErrProfile) {
		t.Fatal("cheap sync", err)
	}
	if _, err := s.Refresh(ctx, ""); !errors.Is(err, ErrProfile) {
		t.Fatal("cheap refresh", err)
	}
	if _, err := s.Get(ctx, ""); !errors.Is(err, ErrProfile) {
		t.Fatal("cheap get", err)
	}
	if _, err := s.List(ctx, 0, 1); !errors.Is(err, ErrProfile) {
		t.Fatal("cheap list", err)
	}
	called := false
	if source, err := s.WalkSettlement(ctx, func([]Record) error { called = true; return nil }); !errors.Is(err, ErrProfile) || source.Version != 0 || called {
		t.Fatal("cheap walk disclosed source or called visitor", err)
	}
}
