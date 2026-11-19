//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package prospectworker

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/prospectwork"
)

// The public Set52 corpus supplies the expected header, digest, duplicate key
// and classification. These storage tests do no Argon hashing and derive no
// expected proof bytes from the implementation under test.
func poFixture(t *testing.T, name string, nonce uint32) (PoolConfig, PoolScan, PoolEntry) {
	t.Helper()
	c, work, f := pwFixture(t, name, nonce, false)
	c.MaxRecords = 8
	w, err := poolservice.PrepareWork(c.Source, c.PublicKey, work)
	if err != nil {
		t.Fatal(err)
	}
	candidate := withNonce(w.Candidate(), f.Nonce)
	if candidate.Proof.Header != f.Header {
		t.Fatal("fixture header differs from independently frozen header")
	}
	s := PoolScan{Version: PoolVersion1, Work: work, NextNonce: uint64(nonce)}
	check := prospectwork.ShareCheck{JobID: work.Job.ID, GymID: work.Job.GymID, Nonce: f.Nonce, Header: f.Header, WorkHash: f.Hash, DuplicateKey: f.Key, Pool: true, Prospect: f.Prospect, Block: f.Block, Candidate: &candidate}
	return c, s, poolEntryForCheck(s, check)
}

func poFound(t *testing.T, b *PoolOutbox, s PoolScan, entry PoolEntry) PoolEntry {
	t.Helper()
	if err := b.saveScan(&s); err != nil {
		t.Fatal(err)
	}
	if err := b.reserve(s); err != nil {
		t.Fatal(err)
	}
	s.NextNonce++
	result, err := b.found(entry, s)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestPoolOutboxOwnedWorkAndAtomicCursor(t *testing.T) {
	c, work, _ := pwFixture(t, "witness-odd-two-level-dependent", 0, false)
	c.MaxRecords = 8
	scan := PoolScan{Version: PoolVersion1, Work: work}
	b, path := pwBox(t, c)
	// Nil GBT dependencies are legal and stay nil. Proof branches are validated
	// separately; the journal does not normalize the issuer's retained template.
	scan.Work.Template.Block.Transactions[0].Depends = nil
	if err := b.saveScan(&scan); err != nil {
		t.Fatal(err)
	}
	value := *scan.Work.Template.Block.CoinbaseValue
	*scan.Work.Template.Block.CoinbaseValue = value + 1
	scan.Work.Template.Block.Transactions[0].Data = "caller changed"
	stored, ok, err := b.scan()
	if err != nil || !ok || stored.NextNonce != 0 || *stored.Work.Template.Block.CoinbaseValue != value || stored.Work.Template.Block.Transactions[0].Data == "caller changed" || stored.Work.Template.Block.Transactions[0].Depends != nil {
		t.Fatal("scan retained caller-owned storage or normalized nil", err)
	}
	// The independent witness PoW fixture uses a different extraNonce. Only
	// its unmined template is used above. Durable proof expectations below use
	// the frozen coinbase-only candidate, whose extraNonce is exactly zero.
	c2, stored, entry := poFixture(t, "coinbase-only", 0)
	if c2 != c {
		t.Fatal("fixtures differ in pinned source")
	}
	if err = b.saveScan(&stored); err != nil {
		t.Fatal(err)
	}
	if err = b.reserve(stored); err != nil {
		t.Fatal(err)
	}
	stored.NextNonce++
	got, err := b.found(entry, stored)
	if err != nil || got.Sequence != 1 || got.Fighter == nil || got.Fighter.Sequence != 1 {
		t.Fatal("found did not assign both sequences", err)
	}
	*got.Work.Template.Block.CoinbaseValue = -1
	got.Fighter.Candidate.Proof.Coinbase = "return changed"
	rows, err := b.List(0, 8)
	if err != nil || len(rows) != 1 || *rows[0].Work.Template.Block.CoinbaseValue == -1 || rows[0].Fighter.Candidate.Proof.Coinbase == "return changed" {
		t.Fatal("found returned shared durable storage", err)
	}
	*rows[0].Work.Template.Block.CoinbaseValue = -2
	duplicate, err := b.found(entry, stored)
	if err != nil || duplicate.Sequence != 1 || *duplicate.Work.Template.Block.CoinbaseValue == -2 || b.meta.Records != 1 {
		t.Fatal("exact duplicate did not recover original row", err)
	}
	if err = b.Close(); err != nil {
		t.Fatal(err)
	}
	b, err = OpenPoolOutbox(path, c)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	resumed, ok, err := b.scan()
	if err != nil || !ok || resumed.NextNonce != 1 || !reflect.DeepEqual(resumed.Work, stored.Work) {
		t.Fatal("reopen lost exact durable cursor/template", err)
	}
	rows, err = b.List(0, 8)
	if err != nil || len(rows) != 1 || rows[0].WorkHash != entry.WorkHash || rows[0].DuplicateKey != entry.DuplicateKey || b.meta.Pending != 1 {
		t.Fatal("reopen lost found work", err)
	}
	if err = b.saveScan(nil); err != nil {
		t.Fatal(err)
	}
	if _, ok, err = b.scan(); err != nil || ok || b.meta.Records != 1 || b.meta.Pending != 1 {
		t.Fatal("clearing scan erased pending recovery", err)
	}
}

func TestPoolOutboxUncertainFoundAndReceipt(t *testing.T) {
	for _, stage := range []string{"found", "receipt"} {
		for _, committed := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/committed-%t", stage, committed), func(t *testing.T) {
				c, scan, entry := poFixture(t, "modeled-all-three-miss", 0)
				b, path := pwBox(t, c)
				if err := b.saveScan(&scan); err != nil {
					t.Fatal(err)
				}
				if err := b.reserve(scan); err != nil {
					t.Fatal(err)
				}
				scan.NextNonce++
				if stage == "receipt" {
					var err error
					entry, err = b.found(entry, scan)
					if err != nil {
						t.Fatal(err)
					}
				}
				write := b.writeBatch
				b.writeBatch = func(batch *leveldb.Batch, options *opt.WriteOptions) error {
					if options == nil || !options.Sync {
						t.Fatal("durable mutation was not synchronous")
					}
					if committed {
						if err := write(batch, options); err != nil {
							return err
						}
					}
					return io.ErrUnexpectedEOF
				}
				var err error
				if stage == "found" {
					_, err = b.found(entry, scan)
				} else {
					_, work, f := pwFixture(t, "modeled-all-three-miss", 0, false)
					receipt := pwReceipt(c, work, f)
					entry.PoolReceipt, entry.PoolState = &receipt, "accepted"
					err = b.update(entry)
				}
				if !errors.Is(err, ErrPoisoned) {
					t.Fatal("uncertain mutation did not poison handle", err)
				}
				if rows, err := b.List(0, 1); rows != nil || !errors.Is(err, ErrPoisoned) {
					t.Fatal("poisoned handle disclosed uncertain state")
				}
				if _, ok, err := b.scan(); ok || !errors.Is(err, ErrPoisoned) {
					t.Fatal("poisoned handle disclosed uncertain cursor")
				}
				if err = b.Close(); err != nil {
					t.Fatal(err)
				}
				b, err = OpenPoolOutbox(path, c)
				if err != nil {
					t.Fatal(err)
				}
				defer b.Close()
				rows, err := b.List(0, 8)
				if err != nil {
					t.Fatal(err)
				}
				s, ok, err := b.scan()
				if err != nil || !ok {
					t.Fatal(err)
				}
				if stage == "found" {
					want := uint64(0)
					if committed {
						want = 1
					}
					if uint64(len(rows)) != want || s.NextNonce != want || b.meta.Records != want || b.meta.Pending != want {
						t.Fatal("found row/cursor did not commit atomically")
					}
					if !committed {
						if _, err = b.found(entry, scan); err != nil {
							t.Fatal("uncommitted find could not resume", err)
						}
					}
				} else if len(rows) != 1 || s.NextNonce != 1 || (rows[0].PoolReceipt != nil) != committed || (rows[0].PoolState == "accepted") != committed || (b.meta.Pending == 0) != committed || (b.meta.Reserved == 0) != committed {
					t.Fatal("receipt/terminal state/reservation did not commit atomically")
				}
			})
		}
	}
}

func TestPoolOutboxRecoveryAtCapacityAndTerminalMonotonicity(t *testing.T) {
	c, scan, entry := poFixture(t, "coinbase-only", 0)
	b, _ := pwBox(t, c)
	entry = poFound(t, b, scan, entry)
	scan, _, _ = b.scan()
	// Tighten the in-memory operational budget to the exact existing promise;
	// production configuration remains at its documented minimum in metadata.
	budget := b.meta.Bytes + b.meta.Reserved
	b.config.MaxBytes = budget
	if err := b.reserve(scan); !errors.Is(err, ErrCapacity) {
		t.Fatal("new hashing spent existing recovery capacity", err)
	}
	entry.LastError = "signer_unavailable"
	entry.NextAttemptMS, entry.RetryCount = math.MaxInt64, 32
	if err := b.update(entry); err != nil || b.meta.Bytes+b.meta.Reserved != budget {
		t.Fatal("pending growth consumed unreserved space", err)
	}
	if _, ok, err := b.nextDue(math.MaxInt64 - 1); err != nil || ok {
		t.Fatal("future retry appeared due", err)
	}
	if due, ok, err := b.nextDue(math.MaxInt64); err != nil || !ok || due.Sequence != 1 {
		t.Fatal("due retry disappeared at capacity", err)
	}
	_, work, f := pwFixture(t, "coinbase-only", 0, false)
	r := pwReceipt(c, work, f)
	entry.PoolReceipt, entry.PoolState = &r, "accepted"
	if err := b.update(entry); err != nil || b.meta.Pending != 1 || b.meta.Reserved == 0 {
		t.Fatal("pool acknowledgement stranded independent fighter", err)
	}
	// An expired unsigned fighter closes without invoking a signer or erasing
	// the already accepted pool evidence.
	entry.Fighter.AdmissionState = "expired"
	if err := b.update(entry); err != nil || b.meta.Pending != 0 || b.meta.Reserved != 0 {
		t.Fatal("reserved terminal update did not release capacity", err)
	}
	entry.Fighter.AdmissionState = "needs-signature"
	if err := b.update(entry); !errors.Is(err, ErrCorrupt) {
		t.Fatal("terminal fighter was reopened", err)
	}
	rows, err := b.List(0, 1)
	if err != nil || len(rows) != 1 || rows[0].Fighter.AdmissionState != "expired" || rows[0].PoolState != "accepted" {
		t.Fatal("rejected rollback altered durable evidence", err)
	}
	if _, ok, err := b.nextDue(math.MaxInt64); err != nil || ok {
		t.Fatal("terminal row retained pending index", err)
	}
}

func TestPoolOutboxExhaustionAndIdentityIsolation(t *testing.T) {
	c, scan, entry := poFixture(t, "coinbase-only", math.MaxUint32)
	b, path := pwBox(t, c)
	entry = poFound(t, b, scan, entry)
	if entry.Nonce != math.MaxUint32 {
		t.Fatal("last nonce changed")
	}
	exhausted, ok, err := b.scan()
	if err != nil || !ok || exhausted.NextNonce != 1<<32 {
		t.Fatal("last nonce cursor wrapped", err)
	}
	if err = b.reserve(exhausted); !errors.Is(err, ErrCapacity) {
		t.Fatal("exhausted nonce space reserved another hash", err)
	}
	tooFar := exhausted
	tooFar.NextNonce++
	if err = b.advance(tooFar); !errors.Is(err, ErrCorrupt) {
		t.Fatal("out-of-range cursor advanced", err)
	}
	b.running = true
	if err = b.Close(); !errors.Is(err, ErrInUse) {
		t.Fatal("active worker's journal closed", err)
	}
	b.running = false
	if err = b.Close(); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*PoolConfig){
		func(c *PoolConfig) { c.AuthorityOrigin = "http://127.0.0.1:39003" },
		func(c *PoolConfig) { c.AdmissionOrigin = "http://127.0.0.1:39004" },
		func(c *PoolConfig) { c.Source.MaxWorks++ },
		func(c *PoolConfig) { c.Source.PoolTarget = strings.Repeat("e", 64) },
		func(c *PoolConfig) { c.Source.AuthorityPin.AuthorityID = strings.Repeat("1", 64) },
		func(c *PoolConfig) { c.TransportTrustID = strings.Repeat("2", 64) },
		func(c *PoolConfig) { c.AdmissionTrustID = strings.Repeat("3", 64) },
	} {
		changed := c
		mutate(&changed)
		if opened, err := OpenPoolOutbox(path, changed); err == nil || opened != nil {
			t.Fatal("pool source/identity change opened old journal")
		}
	}
	if opened, err := OpenManifestOutbox(path, poolManifestConfig(c)); err == nil || opened != nil {
		t.Fatal("pool journal opened as manifest journal")
	}
	if created, err := CreatePoolOutbox(path, c); err == nil || created != nil {
		t.Fatal("existing journal was silently replaced")
	}
	missing := filepath.Join(t.TempDir(), "missing")
	if opened, err := OpenPoolOutbox(missing, c); err == nil || opened != nil {
		t.Fatal("missing journal was initialized by Open")
	}
	if _, err = os.Stat(missing); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("Open created missing state")
	}
	larger := c
	larger.MaxRecords, larger.MaxBytes = larger.MaxRecords+1, 128<<20
	b, err = OpenPoolOutbox(path, larger)
	if err != nil {
		t.Fatal("explicit operational capacity increase rejected", err)
	}
	if b.meta.Config != larger {
		t.Fatal("capacity increase not durable")
	}
	if err = b.Close(); err != nil {
		t.Fatal(err)
	}
	if opened, err := OpenPoolOutbox(path, c); err == nil || opened != nil {
		t.Fatal("capacity silently decreased after restart")
	}
	b, err = OpenPoolOutbox(path, larger)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	resumed, _, _ := b.scan()
	if resumed.NextNonce != 1<<32 {
		t.Fatal("reopen reset exhausted nonce space")
	}
}

func TestPoolOutboxCanonicalAuditRejectsCorruption(t *testing.T) {
	for _, kind := range []string{"duplicate-index-long", "duplicate-index-zero", "pending-index-long", "pending-time-overflow", "missing-cursor", "cursor-overflow", "reserved", "mixed-namespace", "unknown-array", "duplicate-field", "outer-whitespace", "nullable-required", "sequence-gap"} {
		t.Run(kind, func(t *testing.T) {
			c, scan, entry := poFixture(t, "modeled-all-three-miss", 0)
			b, path := pwBox(t, c)
			entry = poFound(t, b, scan, entry)
			batch := new(leveldb.Batch)
			switch kind {
			case "duplicate-index-long":
				batch.Put(hashKey(entry.DuplicateKey), bytes.Repeat([]byte{'e'}, 1024))
			case "duplicate-index-zero":
				batch.Put(hashKey(entry.DuplicateKey), seqKey(0))
			case "pending-index-long":
				batch.Put(poolPendingKey(entry), bytes.Repeat([]byte{'e'}, 1024))
			case "pending-time-overflow":
				key := poolPendingKey(entry)
				key[1] = 255
				batch.Delete(poolPendingKey(entry))
				batch.Put(key, seqKey(entry.Sequence))
			case "missing-cursor":
				batch.Delete([]byte("cursor"))
			case "cursor-overflow":
				batch.Put([]byte("cursor"), cursor(1<<32+1))
			case "reserved", "mixed-namespace":
				m := b.meta
				if kind == "reserved" {
					m.Reserved++
				} else {
					m.Application = "slug-manifest-worker"
				}
				raw, _ := json.Marshal(m)
				batch.Put([]byte("meta"), raw)
			case "sequence-gap":
				batch.Delete(seqKey(1))
			default:
				raw, _ := b.db.Get([]byte("scan"), nil)
				switch kind {
				case "unknown-array":
					raw = append([]byte(`{"future":[],`), raw[1:]...)
				case "duplicate-field":
					raw = append([]byte(`{"version":1,`), raw[1:]...)
				case "outer-whitespace":
					raw = append(raw, '\n')
				case "nullable-required":
					raw = bytes.Replace(raw, []byte(`"next_nonce":0`), []byte(`"next_nonce":null`), 1)
				}
				batch.Put([]byte("scan"), raw)
			}
			if err := b.db.Write(batch, &opt.WriteOptions{Sync: true}); err != nil {
				t.Fatal(err)
			}
			if strings.HasPrefix(kind, "pending-") {
				if _, ok, err := b.nextDue(0); err == nil || ok {
					t.Fatal("malformed pending index reported as future work")
				}
			}
			if err := b.Close(); err != nil {
				t.Fatal(err)
			}
			if opened, err := OpenPoolOutbox(path, c); err == nil || opened != nil {
				t.Fatal("corrupt journal opened")
			}
		})
	}
}

func TestPoolOutboxTypedJSONBoundsAndNil(t *testing.T) {
	c, scan, entry := poFixture(t, "coinbase-only", 0)
	scan.Work.Template.Block.Transactions = nil
	b, path := pwBox(t, c)
	if err := b.saveScan(&scan); err != nil {
		t.Fatal("legal nil GBT transactions rejected", err)
	}
	got, _, err := b.scan()
	if err != nil || !reflect.DeepEqual(got, scan) {
		t.Fatal("canonical nil template did not round trip", err)
	}
	if err = b.Close(); err != nil {
		t.Fatal(err)
	}
	b, err = OpenPoolOutbox(path, c)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	got, _, _ = b.scan()
	if got.Work.Template.Block.Transactions != nil {
		t.Fatal("reopen normalized nil template")
	}
	entry.Work = scan.Work
	entry.Fighter.Candidate.Proof.MerkleBranch = make([]string, 21)
	scan.NextNonce++
	if _, err = b.found(entry, scan); !errors.Is(err, ErrCorrupt) {
		t.Fatal("oversized proof branch copied into journal", err)
	}
	for _, raw := range []string{
		`{"x":` + strings.Repeat(`{"x":`, 13) + `0` + strings.Repeat("}", 13) + `}`,
		`{"merkle_branch":[` + strings.Repeat(`"a",`, 20) + `"a"]}`,
		`{"transactions":[` + strings.Repeat(`{},`, 16666) + `{ }]}`,
		`{"version":1,"version":1}`,
		`{"version":1,"future":null}`,
		`null`,
	} {
		var decoded PoolScan
		if poolDecodeCanonical([]byte(raw), poolMaxScanSize, &decoded) == nil {
			t.Fatal("malformed/over-limit stored shape decoded")
		}
	}
}
