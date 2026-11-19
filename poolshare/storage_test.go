// Copyright (c) 2026 The SLUG developers. ISC license.
package poolshare

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/wire/v2"
)

type submissionBatchDetector struct{ found, accepted bool }

func (d *submissionBatchDetector) Put(k, v []byte) {
	if len(k) == 65 && k[0] == 's' {
		d.found = true
		var s Submission
		if json.Unmarshal(v, &s) == nil && s.AcceptedSequence != 0 {
			d.accepted = true
		}
	}
}
func (*submissionBatchDetector) Delete([]byte) {}

func TestPoolUncertainIntentNeverBroadcastsUntilReopen(t *testing.T) {
	for _, afterSync := range []bool{false, true} {
		name := "before-sync"
		if afterSync {
			name = "after-sync"
		}
		t.Run(name, func(t *testing.T) {
			h := setupPool(t, "", 0)
			h.now = h.source.Job.ExpiresAtMS
			write := h.pool.writeBatch
			fired := false
			h.pool.writeBatch = func(batch *leveldb.Batch, options *opt.WriteOptions) error {
				d := new(submissionBatchDetector)
				if err := batch.Replay(d); err != nil {
					t.Fatal(err)
				}
				if d.found && !fired {
					fired = true
					if options == nil || !options.Sync {
						t.Fatal("intent not synchronously written")
					}
					if afterSync {
						if err := write(batch, options); err != nil {
							t.Fatal(err)
						}
					}
					return errors.New("simulated uncertain disk write")
				}
				return write(batch, options)
			}
			if _, err := h.pool.Submit(context.Background(), poolTestToken, h.source.Job.ID, h.winner.Nonce); !errors.Is(err, ErrPoisoned) {
				t.Fatal("write uncertainty did not poison", err)
			}
			if !fired || h.node.broadcasts != 0 {
				t.Fatal("broadcast despite uncertain intent")
			}
			if _, err := h.pool.Lookup(context.Background(), poolTestToken, h.source.Job.ID, h.winner.Nonce); !errors.Is(err, ErrPoisoned) {
				t.Fatal("poisoned handle served evidence", err)
			}
			h.reopen(t)
			saved, err := h.pool.Lookup(context.Background(), poolTestToken, h.source.Job.ID, h.winner.Nonce)
			if afterSync {
				if err != nil || saved.Sequence != 1 || saved.AcceptedSequence != 0 || saved.BlockState != "pending" {
					t.Fatal("synced intent lost", saved, err)
				}
			} else if !errors.Is(err, ErrNotFound) {
				t.Fatal("failed pre-sync intent appeared", err)
			}
			h.node.canonicalErr = nil
			got, err := h.pool.Submit(context.Background(), poolTestToken, h.source.Job.ID, h.winner.Nonce)
			if err != nil || got.Sequence != 1 || got.AcceptedSequence != 1 || got.Acceptance != "canonical-late-winner" {
				t.Fatal("exact recovery duplicated or lost winner", got, err)
			}
		})
	}
}

func TestPoolUncertainCanonicalAcceptanceReconcilesOnce(t *testing.T) {
	h := setupPool(t, "", 0)
	h.now = h.source.Job.ExpiresAtMS
	first, err := h.pool.Submit(context.Background(), poolTestToken, h.source.Job.ID, h.winner.Nonce)
	if err != nil || first.AcceptedSequence != 0 {
		t.Fatal("prepare pending winner", err)
	}
	write := h.pool.writeBatch
	fired := false
	h.pool.writeBatch = func(batch *leveldb.Batch, options *opt.WriteOptions) error {
		d := new(submissionBatchDetector)
		if err := batch.Replay(d); err != nil {
			t.Fatal(err)
		}
		if err := write(batch, options); err != nil {
			return err
		}
		if d.accepted && !fired {
			fired = true
			return errors.New("acknowledgment lost after synced canonical credit")
		}
		return nil
	}
	h.node.canonicalErr = nil
	h.now++
	if _, err := h.pool.Submit(context.Background(), poolTestToken, h.source.Job.ID, h.winner.Nonce); !errors.Is(err, ErrPoisoned) || !fired {
		t.Fatal("canonical acceptance uncertainty not latched", err)
	}
	h.reopen(t)
	saved, err := h.pool.Lookup(context.Background(), poolTestToken, h.source.Job.ID, h.winner.Nonce)
	if err != nil || saved.Sequence != first.Sequence || saved.AcceptedSequence != 1 || saved.Acceptance != "canonical-late-winner" {
		t.Fatal("synced late acceptance lost", saved, err)
	}
	h.now++
	replay, err := h.pool.Submit(context.Background(), poolTestToken, h.source.Job.ID, h.winner.Nonce)
	if err != nil || replay.AcceptedSequence != saved.AcceptedSequence || replay.AcceptedAtMS != saved.AcceptedAtMS || h.pool.meta.Accepted != 1 {
		t.Fatal("uncertain credit granted twice", replay, err)
	}
}

func TestPoolOpenRejectsTamperedEvidenceAndConfig(t *testing.T) {
	for _, change := range []string{"classification", "hash", "nonce-index", "unexpected-key", "config"} {
		t.Run(change, func(t *testing.T) {
			h := setupPool(t, "", 0)
			saved, err := h.pool.Submit(context.Background(), poolTestToken, h.source.Job.ID, h.ordinary.Nonce)
			if err != nil {
				t.Fatal(err)
			}
			if err := h.pool.Close(); err != nil {
				t.Fatal(err)
			}
			if change == "config" {
				h.config.PoolTarget = strings.Repeat("e", 64)
			} else {
				db, err := leveldb.OpenFile(filepath.Join(h.path, "db"), &opt.Options{ErrorIfMissing: true})
				if err != nil {
					t.Fatal(err)
				}
				batch := new(leveldb.Batch)
				switch change {
				case "classification":
					saved.Pool = false
				case "hash":
					saved.WorkHash = strings.Repeat("1", 64)
				case "nonce-index":
					batch.Put(nonceKey(h.source.Job.ID, h.ordinary.Nonce), []byte(strings.Repeat("1", 64)))
				case "unexpected-key":
					batch.Put([]byte("unrecognized"), []byte("entry"))
				}
				if change == "classification" || change == "hash" {
					raw, err := json.Marshal(saved)
					if err != nil {
						t.Fatal(err)
					}
					batch.Put(recordKey(saved.DuplicateKey), raw)
				}
				if err := db.Write(batch, &opt.WriteOptions{Sync: true}); err != nil {
					t.Fatal(err)
				}
				if err := db.Close(); err != nil {
					t.Fatal(err)
				}
			}
			opened, err := Open(h.path, h.config, h.authority, h.node, h.sessions)
			if err == nil {
				_ = opened.Close()
				t.Fatal("tampered journal reopened")
			}
		})
	}
}

func TestPoolCapacityStillAllowsRetainedWinnerRecovery(t *testing.T) {
	h := setupPool(t, "", 1)
	// The reservation is fixed, so the complete future state must fit even
	// when the configured logical byte cap is exactly consumed.
	if err := h.pool.Close(); err != nil {
		t.Fatal(err)
	}
	h.config.MaxBytes = h.pool.meta.Bytes + maxSubmissionSize
	// Capacity is pinned: recreate the owned empty test pool with the intended
	// cap, retaining the original authoritative job, never modifying a marker.
	h.path = filepath.Join(t.TempDir(), "exact-capacity")
	var err error
	h.pool, err = Create(h.path, h.config, h.authority, h.node, h.sessions)
	if err != nil {
		t.Fatal(err)
	}
	h.pool.now = func() time.Time { return time.UnixMilli(h.now) }
	if err := h.pool.saveWork(savedWork{Job: h.source.Job, Template: h.source.Template}, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	h.now = h.source.Job.ExpiresAtMS
	first, err := h.pool.Submit(context.Background(), poolTestToken, h.source.Job.ID, h.winner.Nonce)
	if err != nil {
		t.Fatal(err)
	}
	if first.AcceptedSequence != 0 || h.pool.meta.Bytes != h.config.MaxBytes {
		t.Fatal("test did not fill reserved capacity")
	}
	h.reopen(t)
	h.node.canonicalErr = nil
	h.now++
	got, err := h.pool.Submit(context.Background(), poolTestToken, h.source.Job.ID, h.winner.Nonce)
	if err != nil || got.AcceptedSequence != 1 {
		t.Fatal("full journal blocked retained intent recovery", err)
	}
}

func TestPoolBusyAndCloseDrain(t *testing.T) {
	h := setupPool(t, "", 0)
	entered := make(chan struct{})
	release := make(chan struct{})
	h.node.onCurrent = func() { close(entered); <-release }
	result := make(chan error, 1)
	go func() {
		_, err := h.pool.Submit(context.Background(), poolTestToken, h.source.Job.ID, h.ordinary.Nonce)
		result <- err
	}()
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("submission did not reach guard")
	}
	if _, err := h.pool.Lookup(context.Background(), poolTestToken, h.source.Job.ID, h.ordinary.Nonce); !errors.Is(err, ErrBusy) {
		close(release)
		t.Fatal("concurrent operation queued instead of refusing", err)
	}
	closed := make(chan error, 1)
	go func() { closed <- h.pool.Close() }()
	select {
	case err := <-closed:
		close(release)
		t.Fatal("close escaped active operation", err)
	default:
	}
	close(release)
	select {
	case err := <-result:
		if err != nil {
			t.Fatal("draining accepted submission", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("submission failed to drain")
	}
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("close failed to drain")
	}
	if _, err := h.pool.Lookup(context.Background(), poolTestToken, h.source.Job.ID, h.ordinary.Nonce); !errors.Is(err, ErrClosed) {
		t.Fatal("closed pool remained usable", err)
	}
}

func TestPoolCheapProfileRefusesBeforeStorage(t *testing.T) {
	if !wire.TestPoW {
		t.Skip("explicit cheap-profile refusal")
	}
	path := filepath.Join(t.TempDir(), "not-created")
	if _, err := Create(path, Config{}, nil, nil, nil); !errors.Is(err, ErrProfile) {
		t.Fatal("cheap Create", err)
	}
	if _, err := Open(path, Config{}, nil, nil, nil); !errors.Is(err, ErrProfile) {
		t.Fatal("cheap Open", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("cheap profile touched storage", err)
	}
	p := new(Pool)
	if _, err := p.Issue(context.Background(), poolTestToken); !errors.Is(err, ErrProfile) {
		t.Fatal("cheap Issue", err)
	}
	if got, err := p.Submit(context.Background(), poolTestToken, strings.Repeat("0", 64), 0); !errors.Is(err, ErrProfile) || !reflect.DeepEqual(got, Submission{}) {
		t.Fatal("cheap Submit", err)
	}
	if _, err := p.Lookup(context.Background(), poolTestToken, strings.Repeat("0", 64), 0); !errors.Is(err, ErrProfile) {
		t.Fatal("cheap Lookup", err)
	}
	if _, err := p.Work(context.Background(), poolTestToken, strings.Repeat("0", 64)); !errors.Is(err, ErrProfile) {
		t.Fatal("cheap Work", err)
	}
}
