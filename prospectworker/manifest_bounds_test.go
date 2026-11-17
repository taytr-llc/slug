//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package prospectworker

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/taytr-llc/slug/blockchain"
	"github.com/taytr-llc/slug/btcjson"
	"github.com/taytr-llc/slug/btcutil/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/prospectwork"
	"github.com/taytr-llc/slug/wire/v2"
)

func TestManifestWorkerProductionBlockOnly(t *testing.T) {
	f, c := mwFixture(t)
	// Changing the target changes Job2 and the coinbase/header. Hash this actual
	// candidate under normal production PoW; these are behavioral, not golden bytes.
	f.Job.Context.ProspectTarget = strings.Repeat("0", 63) + "1"
	var err error
	f.Job, err = prospectjob.NewManifest(f.Job.Context, f.Job.AuthorityID, f.Job.DescriptorID, f.Job.PublicKey, f.Job.Challenge, f.Job.IssuedAtMS)
	if err != nil {
		t.Fatal(err)
	}
	f.Template.Context = f.Job.Context
	c.Policy.ProspectTarget = f.Job.Context.ProspectTarget
	b, _ := mwBox(t, c)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	events := []string{}
	a := &mwAuthority{c: c, f: f, submit: func() (prospectservice.ManifestAdmissionResult, error) {
		t.Error("block-only admission")
		return prospectservice.ManifestAdmissionResult{}, errors.New("unexpected")
	}}
	n := &mwNode{f: f, call: func(s string) {
		events = append(events, s)
		if s == "canonical" {
			cancel()
		}
	}}
	w, err := NewManifest(c, a, n, func(context.Context, prospect.Context, prospect.Proof) (gymauth.Authorization, error) {
		t.Error("block-only signer")
		return gymauth.Authorization{}, errors.New("unexpected")
	}, b)
	if err != nil {
		t.Fatal(err)
	}
	w.now = func() time.Time { return time.UnixMilli(f.Job.IssuedAtMS + 100) }
	if err = w.Run(ctx); err != nil {
		t.Fatal(err)
	}
	entries, err := b.List(0, 100)
	if err != nil || len(entries) != 1 {
		t.Fatal("expected actual winning block", err, len(entries))
	}
	e := entries[0]
	if !e.BlockWin || e.ProspectWin || e.BlockState != "canonical" || e.AdmissionState != "not-applicable" || e.Authorization != nil || e.Receipt != nil || b.meta.Pending != 0 || b.meta.Reserved != 0 {
		t.Fatal("independent target state differs")
	}
	if strings.Join(events, ",") != "proposal,broadcast,canonical" {
		t.Fatal(events)
	}
}

func mwLargeScan(t *testing.T, f mwCase, size int) ManifestScan {
	t.Helper()
	s := mwScan(f)
	s.ExtraNonce = 0
	tx := wire.NewMsgTx(2)
	tx.AddTxIn(wire.NewTxIn(&wire.OutPoint{Hash: chainhash.DoubleHashH([]byte(fmt.Sprint(size)))}, nil, wire.TxWitness{bytes.Repeat([]byte{1}, size)}))
	tx.AddTxOut(wire.NewTxOut(1, []byte{0x51}))
	var raw bytes.Buffer
	_ = tx.Serialize(&raw)
	s.Template.Block.Transactions = []btcjson.GetBlockTemplateResultTx{{Data: hex.EncodeToString(raw.Bytes()), TxID: tx.TxHash().String(), Hash: tx.WitnessHash().String(), Depends: []int64{}, Fee: 1, Weight: blockchain.GetTransactionWeight(btcutil.NewTx(tx))}}
	value := int64(1000000001)
	s.Template.Block.CoinbaseValue = &value
	var pair, preimage [64]byte
	hash := tx.WitnessHash()
	copy(pair[32:], hash[:])
	root := chainhash.DoubleHashH(pair[:])
	copy(preimage[:32], root[:])
	s.Template.Block.DefaultWitnessCommitment = hex.EncodeToString(chainhash.DoubleHashB(preimage[:]))
	var err error
	s.Candidate, err = prospectwork.BuildManifest(s.Job, s.Template, f.Candidate.PayoutAddress, 0)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestManifestOutboxListFullEncodedByteLimit(t *testing.T) {
	f, c := mwFixture(t)
	c.MaxBytes = 128 << 20
	b, _ := mwBox(t, c)
	// Public journal validation is deliberately hash-free. These claimed hashes
	// test storage/page behavior only; they are never sent as verified work.
	for i, size := range []int{3000000, 3500000, 3200000, 3400000, 3300000, 3100000} {
		s := mwLargeScan(t, f, size)
		if err := b.saveScan(&s); err != nil {
			t.Fatal(err)
		}
		if err := b.reserve(s); err != nil {
			t.Fatal(err)
		}
		s.NextNonce = 1
		if _, err := b.found(ManifestEntry{Version: 2, WorkHash: fmt.Sprintf("%064x", i+1), Job: s.Job, Candidate: s.Candidate, BlockWin: true, ProspectWin: true, BlockState: "pending", AdmissionState: "needs-signature"}, s); err != nil {
			t.Fatal(err)
		}
	}
	after, total := uint64(0), 0
	firstCount := 0
	for {
		page, err := b.List(after, 100)
		if err != nil {
			t.Fatal(err)
		}
		if page == nil {
			t.Fatal("nil page")
		}
		if len(page) == 0 {
			break
		}
		encoded, err := json.Marshal(page)
		if err != nil || len(encoded) > maxPageBytes {
			t.Fatal("full array exceeded byte budget", len(encoded), err)
		}
		if firstCount == 0 {
			firstCount = len(page)
		}
		total += len(page)
		after = page[len(page)-1].Sequence
	}
	if total != 6 || firstCount >= 6 {
		t.Fatal("byte pagination failed", total, firstCount)
	}
	// A corruption beyond the first page's byte budget is not examined early,
	// while its own page returns no trusted prefix.
	if err := b.db.Put(seqKey(6), bytes.Repeat([]byte("!"), 6200000), nil); err != nil {
		t.Fatal(err)
	}
	if page, err := b.List(0, 100); err != nil || len(page) != firstCount {
		t.Fatal("read beyond page budget", err)
	}
	if page, err := b.List(uint64(firstCount), 100); err == nil || page != nil {
		t.Fatal("corrupt page leaked prefix")
	}
	scan, _, _ := b.scan()
	if allocations := testing.AllocsPerRun(3, func() { _ = b.reserve(scan) }); allocations > 10 {
		t.Fatal("reservation copied large payload", allocations)
	}
}

func TestManifestOutboxFutureReservationAndWidths(t *testing.T) {
	f, c := mwFixture(t)
	b, _ := mwBox(t, c)
	e := mwFound(t, b, f)
	scan, _, _ := b.scan()
	originalBudget := b.config.MaxBytes
	b.config.MaxBytes = b.meta.Bytes + b.meta.Reserved
	if err := b.reserve(scan); !errors.Is(err, ErrCapacity) {
		t.Fatal("hash consumed future receipt budget", err)
	}
	e.BlockState = "submitted"
	if err := b.update(e); err != nil {
		t.Fatal(err)
	}
	e.Authorization = &f.Auth
	e.AdmissionState = "pending"
	if err := b.update(e); err != nil {
		t.Fatal(err)
	}
	r := mwResult(f)
	e.Receipt = &r
	e.AdmissionState = "accepted"
	e.NextAttemptMS = math.MaxInt64
	e.RetryCount = 32
	e.LastError = "signer_unavailable"
	if err := b.update(e); err != nil {
		t.Fatal("reserved future result could not commit", err)
	}
	if b.meta.Reserved == 0 || b.meta.Pending != 1 {
		t.Fatal("accepted receipt lost unresolved block reservation")
	}
	e.BlockState = "canonical"
	if err := b.update(e); err != nil {
		t.Fatal("reserved block reconciliation failed", err)
	}
	if b.meta.Reserved != 0 || b.meta.Pending != 0 {
		t.Fatal("terminal reservations not released")
	}
	b.config.MaxBytes = originalBudget
	// Independently encode the maximum extra JSON wrappers and state/error text.
	// Auth/response raw bodies are already separately bounded, including escaping.
	base := ManifestEntry{Version: 2, Sequence: math.MaxUint64, WorkHash: strings.Repeat("f", 64), Job: f.Job, Candidate: f.Candidate, BlockWin: true, ProspectWin: true, BlockState: "pending", AdmissionState: "needs-signature", NextAttemptMS: math.MaxInt64, RetryCount: 32}
	raw, _ := json.Marshal(base)
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		t.Fatal("shape")
	}
	fields["authorization"] = json.RawMessage(`"` + strings.Repeat("a", 4094) + `"`)
	fields["receipt"] = json.RawMessage(`"` + strings.Repeat("r", 73726) + `"`)
	fields["last_error"] = json.RawMessage(`"signer_unavailable"`)
	fields["block_state"] = json.RawMessage(`"not-applicable"`)
	fields["admission_state"] = json.RawMessage(`"not-applicable"`)
	expanded, _ := json.Marshal(fields)
	if uint64(len(expanded)-len(raw)) > manifestFutureReserve(base) {
		t.Fatal("future reservation undercounts JSON growth")
	}
	if manifestReservationEntryBytes(mwScan(f)) != uint64(len(raw)) {
		t.Fatal("initial width reservation differs")
	}
}

func TestManifestTypedBoundsAndStoredNull(t *testing.T) {
	f, c := mwFixture(t)
	b, _ := mwBox(t, c)
	s := mwScan(f)
	s.Candidate.Proof.MerkleBranch = make([]string, 21)
	if err := b.saveScan(&s); err == nil {
		t.Fatal("oversized live branch copied")
	}
	raw, _ := json.Marshal(mwScan(f))
	raw = bytes.Replace(raw, []byte(`"merkle_branch":[`), []byte(`"merkle_branch":null,"extra":[`), 1)
	var scan ManifestScan
	if manifestDecodeCanonical(raw, maxScanSize, &scan) == nil {
		t.Fatal("stored null accepted")
	}
	raw = []byte(`{"x":` + strings.Repeat("[", 9) + "0" + strings.Repeat("]", 9) + "}")
	if manifestJSONShape(raw, maxScanSize) == nil {
		t.Fatal("depth accepted")
	}
	if manifestJSONShape([]byte(`{"merkle_branch":[`+strings.Repeat(`"a",`, 20)+`"a"]}`), maxEntrySize) == nil {
		t.Fatal("oversized branch decoded")
	}
	s = mwScan(f)
	s.Template.Block.Transactions[0].Depends = nil
	if err := b.saveScan(&s); err != nil {
		t.Fatal(err)
	}
	stored, _, err := b.scan()
	if err != nil || stored.Template.Block.Transactions[0].Depends == nil {
		t.Fatal("live empty dependencies not owned/normalized")
	}
}
