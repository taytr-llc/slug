//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package prospectworker

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/blockchain"
	"github.com/taytr-llc/slug/btcjson"
	"github.com/taytr-llc/slug/btcutil/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/prospectwork"
	"github.com/taytr-llc/slug/wire/v2"
)

func obFound(t *testing.T, b *Outbox, f wkFixture, s Scan) Entry {
	t.Helper()
	if e := b.saveScan(&s); e != nil {
		t.Fatal(e)
	}
	if e := b.reserve(s); e != nil {
		t.Fatal(e)
	}
	entry := Entry{Version: 1, WorkHash: f.Verified.WorkHash, Job: s.Job, Candidate: s.Candidate, BlockWin: true, ProspectWin: true, BlockState: "pending", AdmissionState: "needs-signature"}
	s.NextNonce++
	got, e := b.found(entry, s)
	if e != nil {
		t.Fatal(e)
	}
	return got
}

func TestOutboxCursorWritesAreSmallAndAtomic(t *testing.T) {
	f, c, b, path := wkSetup(t)
	s := wkScan(f)
	if e := b.saveScan(&s); e != nil {
		t.Fatal(e)
	}
	before, e := b.db.Get([]byte("scan"), nil)
	if e != nil {
		t.Fatal(e)
	}
	write := b.writeBatch
	b.writeBatch = func(batch *leveldb.Batch, options *opt.WriteOptions) error {
		if options == nil || !options.Sync || len(batch.Dump()) > 8192 || bytes.Contains(batch.Dump(), []byte(s.Candidate.BlockHex)) {
			t.Fatal("nonce progress rewrote full scan or was not synchronized")
		}
		return write(batch, options)
	}
	s.NextNonce++
	if e := b.advance(s); e != nil {
		t.Fatal(e)
	}
	after, _ := b.db.Get([]byte("scan"), nil)
	if !bytes.Equal(before, after) {
		t.Fatal("cursor update changed immutable payload")
	}
	if e := b.Close(); e != nil {
		t.Fatal(e)
	}
	b, e = OpenOutbox(path, c)
	if e != nil {
		t.Fatal(e)
	}
	defer b.Close()
	loaded, ok, e := b.scan()
	if e != nil || !ok || loaded.NextNonce != 1 {
		t.Fatal("durable cursor lost on reopen")
	}
	*loaded.Template.Block.CoinbaseValue = 0
	loaded.Template.Block.Mutable[0] = "changed"
	again, _, _ := b.scan()
	if *again.Template.Block.CoinbaseValue == 0 || again.Template.Block.Mutable[0] == "changed" {
		t.Fatal("scan copy aliases cache")
	}
}

func TestOutboxMarkerMissingForeignAndCapacityRules(t *testing.T) {
	f := wkLoad(t)
	c := wkConfig(f)
	missing := filepath.Join(t.TempDir(), "missing")
	if b, e := OpenOutbox(missing, c); e == nil {
		_ = b.Close()
		t.Fatal("missing outbox opened")
	}
	if _, e := os.Stat(missing); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("Open created missing directories")
	}
	_, _, b, path := wkSetup(t)
	if e := b.Close(); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(path, markerName), []byte(`{"application":"foreign"}`), 0600); e != nil {
		t.Fatal(e)
	}
	logPath := filepath.Join(path, "db", "LOG")
	before, _ := os.ReadFile(logPath)
	if opened, e := OpenOutbox(path, c); e == nil {
		_ = opened.Close()
		t.Fatal("foreign marker opened")
	}
	after, _ := os.ReadFile(logPath)
	if !bytes.Equal(before, after) {
		t.Fatal("foreign DB touched before marker rejection")
	}
	_, c, b, path = wkSetup(t)
	_ = b.Close()
	larger := c
	larger.MaxRecords++
	larger.MaxBytes += 1 << 20
	b, e := OpenOutbox(path, larger)
	if e != nil {
		t.Fatal(e)
	}
	_ = b.Close()
	if old, e := OpenOutbox(path, c); e == nil {
		_ = old.Close()
		t.Fatal("capacity silently decreased after reopen")
	}
	wrong := larger
	wrong.Policy.ProspectTarget = strings.Repeat("e", 64)
	if old, e := OpenOutbox(path, wrong); e == nil {
		_ = old.Close()
		t.Fatal("policy identity changed on reopen")
	}
}

func TestOutboxDueIndexReservationsAndReceiptRelease(t *testing.T) {
	f, c, b, path := wkSetup(t)
	entry := obFound(t, b, f, wkScan(f))
	reserved := b.meta.Reserved
	if reserved == 0 || b.pendingCount() != 1 {
		t.Fatal("found evidence has no future receipt reservation")
	}
	entry.NextAttemptMS = 1000
	entry.RetryCount = 1
	if e := b.update(entry); e != nil {
		t.Fatal(e)
	}
	if _, ok, e := b.nextDue(999); e != nil || ok {
		t.Fatal("retry became due early")
	}
	if due, ok, e := b.nextDue(1000); e != nil || !ok || due.Sequence != entry.Sequence {
		t.Fatal("retry index lost evidence")
	}
	if e := b.Close(); e != nil {
		t.Fatal(e)
	}
	b, e := OpenOutbox(path, c)
	if e != nil {
		t.Fatal(e)
	}
	defer b.Close()
	if b.meta.Reserved != reserved || b.pendingCount() != 1 {
		t.Fatal("reopen forgot reservation/index")
	}
	result := wkReceipt(f, f.Authorization)
	entry.Authorization = &f.Authorization
	entry.Receipt = &result
	entry.AdmissionState = "accepted"
	entry.BlockState = "canonical"
	if e := b.update(entry); e != nil {
		t.Fatal(e)
	}
	if b.meta.Reserved != 0 || b.pendingCount() != 0 {
		t.Fatal("terminal receipt did not release future growth budget")
	}
	if _, ok, e := b.nextDue(100000); e != nil || ok {
		t.Fatal("terminal receipt remained scheduled")
	}
	page, e := b.List(0, 100)
	if e != nil || len(page) != 1 || page[0].Receipt == nil {
		t.Fatal("terminal evidence was pruned")
	}
}

func TestOutboxFutureReservationsBlockNewHashesButPermitRecovery(t *testing.T) {
	f, _, b, _ := wkSetup(t)
	entry := obFound(t, b, f, wkScan(f))
	scan, _, _ := b.scan()
	// Model an operational budget exactly sufficient for existing evidence and
	// its future authenticated receipt. No additional hash may consume it.
	b.config.MaxBytes = b.meta.Bytes + b.meta.Reserved
	if e := b.reserve(scan); !errors.Is(e, ErrCapacity) {
		t.Fatal("new hash consumed pending receipt reservation")
	}
	result := wkReceipt(f, f.Authorization)
	entry.Authorization = &f.Authorization
	entry.Receipt = &result
	entry.AdmissionState = "accepted"
	entry.BlockState = "canonical"
	if e := b.update(entry); e != nil {
		t.Fatalf("reserved receipt could not commit: %v", e)
	}
}

func TestOutboxUncertainFoundIncludesCursorAndPendingIndex(t *testing.T) {
	for _, committed := range []bool{false, true} {
		t.Run(fmt.Sprint(committed), func(t *testing.T) {
			f, c, b, path := wkSetup(t)
			s := wkScan(f)
			if e := b.saveScan(&s); e != nil {
				t.Fatal(e)
			}
			write := b.writeBatch
			b.writeBatch = func(batch *leveldb.Batch, options *opt.WriteOptions) error {
				if committed {
					if e := write(batch, options); e != nil {
						return e
					}
				}
				return io.ErrUnexpectedEOF
			}
			entry := Entry{Version: 1, WorkHash: f.Verified.WorkHash, Job: f.Job, Candidate: f.Candidate, BlockWin: true, ProspectWin: true, BlockState: "pending", AdmissionState: "needs-signature"}
			s.NextNonce = 1
			if _, e := b.found(entry, s); !errors.Is(e, ErrPoisoned) {
				t.Fatal("uncertain found not poisoned")
			}
			_ = b.Close()
			b, e := OpenOutbox(path, c)
			if e != nil {
				t.Fatal(e)
			}
			defer b.Close()
			scan, _, e := b.scan()
			if e != nil || scan.NextNonce != map[bool]uint64{false: 0, true: 1}[committed] || b.meta.Pending != map[bool]uint64{false: 0, true: 1}[committed] {
				t.Fatal("found and cursor/index diverged")
			}
		})
	}
}

func TestOutboxCorruptionFailsClosed(t *testing.T) {
	for _, kind := range []string{"cursor-missing", "cursor-range", "pending-missing", "pending-wrong", "reservation", "sequence-gap", "receipt-birth", "receipt-dna"} {
		t.Run(kind, func(t *testing.T) {
			f, c, b, path := wkSetup(t)
			entry := obFound(t, b, f, wkScan(f))
			batch := new(leveldb.Batch)
			switch kind {
			case "cursor-missing":
				batch.Delete([]byte("cursor"))
			case "cursor-range":
				batch.Put([]byte("cursor"), cursor(1<<32+1))
			case "pending-missing":
				batch.Delete(pendingKey(entry))
			case "pending-wrong":
				batch.Put(pendingKey(entry), seqKey(2))
			case "reservation":
				m := b.meta
				m.Reserved--
				raw, _ := json.Marshal(m)
				batch.Put([]byte("meta"), raw)
			case "sequence-gap":
				m := b.meta
				m.LastSequence++
				raw, _ := json.Marshal(m)
				batch.Put([]byte("meta"), raw)
			case "receipt-birth", "receipt-dna":
				r := wkReceipt(f, f.Authorization)
				entry.Authorization = &f.Authorization
				entry.Receipt = &r
				entry.AdmissionState = "accepted"
				entry.BlockState = "canonical"
				if kind == "receipt-birth" {
					r.Record.Birth.RuleVersion = 2
				} else {
					r.Record.Verified.DNA.Attributes.Power++
				}
				raw, _ := json.Marshal(entry)
				batch.Put(seqKey(entry.Sequence), raw)
			}
			if e := b.db.Write(batch, &opt.WriteOptions{Sync: true}); e != nil {
				t.Fatal(e)
			}
			_ = b.Close()
			if opened, e := OpenOutbox(path, c); e == nil {
				_ = opened.Close()
				t.Fatal("corrupt journal opened")
			} else if !errors.Is(e, ErrCorrupt) {
				t.Fatalf("wrong corruption error: %v", e)
			}
		})
	}
}

func TestOutboxPolicyAndReceiptFieldsCannotBeRebound(t *testing.T) {
	f := wkLoad(t)
	c := wkConfig(f)
	other := c
	other.Policy.ProspectTarget = strings.Repeat("e", 64)
	if validateScan(wkScan(f), other) == nil {
		t.Fatal("scan accepted different configured target")
	}
	if _, e := workBlock(f.Job, f.Candidate, other); e == nil {
		t.Fatal("entry accepted different configured target")
	}
	entry := Entry{Version: 1, Sequence: 1, WorkHash: f.Verified.WorkHash, Job: f.Job, Candidate: f.Candidate, BlockWin: true, ProspectWin: true, BlockState: "canonical", AdmissionState: "accepted", Authorization: &f.Authorization}
	result := wkReceipt(f, f.Authorization)
	entry.Receipt = &result
	expected, e := expectedVerified(entry)
	if e != nil || expected != f.Verified {
		t.Fatal("cheap receipt fields differ from independent reference")
	}
	for _, kind := range []string{"zero-sequence", "source", "rule", "dna", "policyid"} {
		copy := entry
		r := wkReceipt(f, f.Authorization)
		copy.Receipt = &r
		switch kind {
		case "zero-sequence":
			r.Record.Sequence = 0
		case "source":
			r.ContextSource = "unverified"
		case "rule":
			r.Record.Birth.RuleVersion = 2
		case "dna":
			r.Record.Verified.DNA.Attributes.Power++
		case "policyid":
			r.Record.Verified.PolicyID = strings.Repeat("0", 64)
		}
		if validateEntry(copy, c) == nil {
			t.Fatalf("invalid receipt %s accepted", kind)
		}
	}
	upper := entry
	r := wkReceipt(f, f.Authorization)
	upper.Receipt = &r
	r.Record.Proof.Header = strings.ToUpper(r.Record.Proof.Header)
	r.Record.Proof.Coinbase = strings.ToUpper(r.Record.Proof.Coinbase)
	if validateEntry(upper, c) != nil {
		t.Fatal("valid equivalent hex spelling rejected")
	}
}

func obLargeScan(t *testing.T, f wkFixture, size int) Scan {
	t.Helper()
	s := wkScan(f)
	tx := wire.NewMsgTx(2)
	tx.AddTxIn(wire.NewTxIn(&wire.OutPoint{Hash: chainhash.DoubleHashH([]byte(fmt.Sprint(size)))}, nil, wire.TxWitness{bytes.Repeat([]byte{1}, size)}))
	tx.AddTxOut(wire.NewTxOut(1, []byte{0x51}))
	var raw bytes.Buffer
	_ = tx.Serialize(&raw)
	s.Template.Block.Transactions = []btcjson.GetBlockTemplateResultTx{{Data: hex.EncodeToString(raw.Bytes()), TxID: tx.TxHash().String(), Hash: tx.WitnessHash().String(), Depends: []int64{}, Fee: 1, Weight: blockchain.GetTransactionWeight(btcutil.NewTx(tx))}}
	value := int64(1000000001)
	s.Template.Block.CoinbaseValue = &value
	var pair [64]byte
	wtxid := tx.WitnessHash()
	copy(pair[32:], wtxid[:])
	root := chainhash.DoubleHashH(pair[:])
	var preimage [64]byte
	copy(preimage[:32], root[:])
	s.Template.Block.DefaultWitnessCommitment = hex.EncodeToString(chainhash.DoubleHashB(preimage[:]))
	var e error
	s.Candidate, e = prospectwork.Build(s.Job, s.Template, f.Candidate.PayoutAddress, 0)
	if e != nil {
		t.Fatal(e)
	}
	return s
}

func TestOutboxListBoundsAggregateLargePayload(t *testing.T) {
	f, _, b, _ := wkSetup(t)
	b.config.MaxBytes = 128 << 20
	sizes := []int{3000000, 3500000, 3200000, 3400000, 3300000, 3100000}
	var total int
	for i, size := range sizes {
		s := obLargeScan(t, f, size)
		if e := b.saveScan(&s); e != nil {
			t.Fatal(e)
		}
		entry := Entry{Version: 1, WorkHash: fmt.Sprintf("%064x", i+1), Job: s.Job, Candidate: s.Candidate, BlockWin: true, ProspectWin: true, BlockState: "pending", AdmissionState: "needs-signature"}
		s.NextNonce = 1
		if _, e := b.found(entry, s); e != nil {
			t.Fatal(e)
		}
	}
	var after uint64
	for {
		page, e := b.List(after, 100)
		if e != nil {
			t.Fatal(e)
		}
		if len(page) == 0 {
			break
		}
		encoded := 0
		for _, entry := range page {
			raw, _ := json.Marshal(entry)
			encoded += len(raw)
			after = entry.Sequence
			total++
		}
		if encoded > maxPageBytes {
			t.Fatal("aggregate page budget exceeded")
		}
		if total < len(sizes) && len(page) == 100 {
			t.Fatal("large page unexpectedly used count limit")
		}
	}
	if total != len(sizes) {
		t.Fatal("byte-limited pagination omitted retained entries")
	}
	// Reserve uses the cached fixed-size estimate, not full-block JSON encoding.
	scan, _, _ := b.scan()
	if allocations := testing.AllocsPerRun(3, func() { _ = b.reserve(scan) }); allocations > 10 {
		t.Fatalf("per-hash reservation allocates full payload: %v allocations", allocations)
	}
}
