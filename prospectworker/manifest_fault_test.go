//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package prospectworker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectwork"
	"io"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

func TestManifestOutboxUncertainReceipt(t *testing.T) {
	for _, committed := range []bool{false, true} {
		t.Run(fmt.Sprint(committed), func(t *testing.T) {
			f, c := mwFixture(t)
			b, path := mwBox(t, c)
			e := mwFound(t, b, f)
			e.BlockState = "submitted"
			if err := b.update(e); err != nil {
				t.Fatal(err)
			}
			e.Authorization = &f.Auth
			e.AdmissionState = "pending"
			if err := b.update(e); err != nil {
				t.Fatal(err)
			}
			write := b.writeBatch
			b.writeBatch = func(batch *leveldb.Batch, o *opt.WriteOptions) error {
				if o == nil || !o.Sync {
					t.Fatal("unsynced receipt")
				}
				if committed {
					if err := write(batch, o); err != nil {
						return err
					}
				}
				return io.ErrUnexpectedEOF
			}
			r := mwResult(f)
			e.Receipt = &r
			e.AdmissionState = "accepted"
			if err := b.update(e); !errors.Is(err, ErrPoisoned) {
				t.Fatal("write uncertainty not poisoned", err)
			}
			if rows, err := b.List(0, 1); !errors.Is(err, ErrPoisoned) || rows != nil {
				t.Fatal("uncertain handle disclosed receipt")
			}
			if err := b.Close(); err != nil {
				t.Fatal(err)
			}
			b, err := OpenManifestOutbox(path, c)
			if err != nil {
				t.Fatal(err)
			}
			defer b.Close()
			rows, err := b.List(0, 1)
			if err != nil || len(rows) != 1 {
				t.Fatal(err)
			}
			if (rows[0].Receipt != nil) != committed || rows[0].Authorization == nil || *rows[0].Authorization != f.Auth || rows[0].BlockState != "submitted" {
				t.Fatal("atomic receipt/auth/block state differs")
			}
			if committed && rows[0].Receipt.Record.Birth != f.Record.Birth {
				t.Fatal("first birth changed")
			}
		})
	}
}

func TestManifestOutboxIndexAndStateCorruption(t *testing.T) {
	for _, kind := range []string{"hash-index-long", "hash-index-zero", "pending-index-long", "pending-time-overflow", "missing-cursor", "cursor-overflow", "reserved", "stored-null", "mixed-version", "unknown-array"} {
		t.Run(kind, func(t *testing.T) {
			f, c := mwFixture(t)
			b, path := mwBox(t, c)
			e := mwFound(t, b, f)
			batch := new(leveldb.Batch)
			switch kind {
			case "hash-index-long":
				batch.Put(hashKey(e.WorkHash), bytes.Repeat([]byte("e"), 1<<20))
			case "hash-index-zero":
				batch.Put(hashKey(e.WorkHash), seqKey(0))
			case "pending-index-long":
				batch.Put(manifestPendingKey(e), bytes.Repeat([]byte("e"), 1<<20))
			case "pending-time-overflow":
				k := manifestPendingKey(e)
				k[1] = 255
				batch.Delete(manifestPendingKey(e))
				batch.Put(k, seqKey(e.Sequence))
			case "missing-cursor":
				batch.Delete([]byte("cursor"))
			case "cursor-overflow":
				batch.Put([]byte("cursor"), cursor(1<<32+1))
			case "reserved":
				m := b.meta
				m.Reserved++
				raw, _ := json.Marshal(m)
				batch.Put([]byte("meta"), raw)
			case "stored-null":
				raw, _ := b.db.Get([]byte("scan"), nil)
				raw = bytes.Replace(raw, []byte(`"depends":[]`), []byte(`"depends":null`), 1)
				batch.Put([]byte("scan"), raw)
			case "mixed-version":
				m := b.meta
				m.Version = 1
				raw, _ := json.Marshal(m)
				batch.Put([]byte("meta"), raw)
			case "unknown-array":
				raw, _ := b.db.Get([]byte("scan"), nil)
				raw = append([]byte(`{"future":[],`), raw[1:]...)
				batch.Put([]byte("scan"), raw)
			}
			if err := b.db.Write(batch, &opt.WriteOptions{Sync: true}); err != nil {
				t.Fatal(err)
			}
			if kind == "pending-index-long" || kind == "pending-time-overflow" {
				if _, ok, err := b.nextDue(0); err == nil || ok {
					t.Fatal("unbounded pending index treated as valid/future")
				}
			}
			if err := b.Close(); err != nil {
				t.Fatal(err)
			}
			if opened, err := OpenManifestOutbox(path, c); err == nil || opened != nil {
				t.Fatal("corruption opened")
			}
		})
	}
}

func TestManifestOutboxCloseRetainsFailure(t *testing.T) {
	_, c := mwFixture(t)
	b, _ := mwBox(t, c)
	if err := b.db.Close(); err != nil {
		t.Fatal(err)
	}
	first := b.Close()
	if first == nil {
		t.Fatal("expected injected already-closed failure")
	}
	if b.Close() != first {
		t.Fatal("repeated Close forgot first failure")
	}
}

func TestManifestBlockOnlyCapacitySaturation(t *testing.T) {
	f, c := mwFixture(t)
	f.Job.Context.ProspectTarget = strings.Repeat("0", 63) + "1"
	var err error
	f.Job, err = prospectjob.NewManifest(f.Job.Context, f.Job.AuthorityID, f.Job.DescriptorID, f.Job.PublicKey, f.Job.Challenge, f.Job.IssuedAtMS)
	if err != nil {
		t.Fatal(err)
	}
	f.Template.Context = f.Job.Context
	c.Policy.ProspectTarget = f.Job.Context.ProspectTarget
	f.Candidate, err = prospectwork.BuildManifest(f.Job, f.Template, c.PayoutAddress, 0)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := mwBox(t, c)
	s := ManifestScan{Version: 2, Job: f.Job, Template: f.Template, Candidate: f.Candidate}
	if err = b.saveScan(&s); err != nil {
		t.Fatal(err)
	}
	s.NextNonce = 1
	// Claimed digest2 is storage-only classification, not a PoW oracle.
	e, err := b.found(ManifestEntry{Version: 2, WorkHash: strings.Repeat("0", 63) + "2", Job: f.Job, Candidate: f.Candidate, BlockWin: true, BlockState: "pending", AdmissionState: "not-applicable"}, s)
	if err != nil {
		t.Fatal(err)
	}
	budget := b.meta.Bytes + b.meta.Reserved
	b.config.MaxBytes = budget
	e.BlockState = "submitted"
	e.LastError = "outcome_unknown"
	e.NextAttemptMS = math.MaxInt64
	e.RetryCount = 32
	if err = b.update(e); err != nil {
		t.Fatal("block-only reservation unusable", err)
	}
	if b.meta.Bytes+b.meta.Reserved != budget {
		t.Fatal("state growth did not consume reservation")
	}
	e.BlockState = "canonical"
	e.LastError = ""
	e.NextAttemptMS = 0
	if err = b.update(e); err != nil || b.meta.Reserved != 0 || b.meta.Pending != 0 {
		t.Fatal("block-only reservation not released", err)
	}
}

type mwCancelPrepareNode struct {
	*mwNode
	resolve func()
}

func (n *mwCancelPrepareNode) ResolveTemplate(ctx context.Context, p prospectnode.Policy) (prospectwork.Template, error) {
	n.resolve()
	return n.f.Template, nil
}
func TestManifestPrepareCancellationStopsLaterTransport(t *testing.T) {
	for _, stage := range []string{"issuance", "template"} {
		t.Run(stage, func(t *testing.T) {
			f, c := mwFixture(t)
			b, _ := mwBox(t, c)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			resolved, proposed := 0, 0
			a := &mwAuthority{c: c, f: f, issue: func() {
				if stage == "issuance" {
					cancel()
				}
			}}
			n := &mwCancelPrepareNode{mwNode: &mwNode{f: f, call: func(s string) {
				if s == "proposal" {
					proposed++
				}
			}}, resolve: func() {
				resolved++
				if stage == "template" {
					cancel()
				}
			}}
			w, err := NewManifest(c, a, n, func(context.Context, prospect.Context, prospect.Proof) (gymauth.Authorization, error) {
				return f.Auth, nil
			}, b)
			if err != nil {
				t.Fatal(err)
			}
			w.now = func() time.Time { return time.UnixMilli(f.Job.IssuedAtMS + 100) }
			if _, err = w.prepare(ctx); !errors.Is(err, context.Canceled) {
				t.Fatal("canceled prepare returned output", err)
			}
			if proposed != 0 || (stage == "issuance" && resolved != 0) || (stage == "template" && resolved != 1) {
				t.Fatal("transport after cancellation", resolved, proposed)
			}
		})
	}
}
