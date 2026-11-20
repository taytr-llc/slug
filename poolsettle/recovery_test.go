// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettle

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/wire/v2"
)

// This observer wraps the existing real-source fixture. It supplies no new
// expected transaction bytes, credits or source authority.
type recoveryBackend struct {
	Backend
	inspects, quotes, signs, observes, broadcasts int
	inspect                                       func(context.Context, []Coin) ([]Funding, error)
	observe                                       func(context.Context, string) (TransactionObservation, error)
}

func (b *recoveryBackend) Inspect(ctx context.Context, coins []Coin) ([]Funding, error) {
	b.inspects++
	if b.inspect != nil {
		return b.inspect(ctx, coins)
	}
	return b.Backend.Inspect(ctx, coins)
}
func (b *recoveryBackend) EstimateFee(ctx context.Context, coins []Coin, payments []Payment, change string) (int64, error) {
	b.quotes++
	return b.Backend.EstimateFee(ctx, coins, payments, change)
}
func (b *recoveryBackend) Sign(ctx context.Context, coins []Coin, raw []byte) ([]byte, error) {
	b.signs++
	return b.Backend.Sign(ctx, coins, raw)
}
func (b *recoveryBackend) ObserveTransaction(ctx context.Context, id string) (TransactionObservation, error) {
	b.observes++
	if b.observe != nil {
		return b.observe(ctx, id)
	}
	return b.Backend.ObserveTransaction(ctx, id)
}
func (b *recoveryBackend) Broadcast(ctx context.Context, raw []byte) (string, error) {
	b.broadcasts++
	return b.Backend.Broadcast(ctx, raw)
}

func recoveryWatch(h *settlementHarness) *recoveryBackend {
	b := &recoveryBackend{Backend: h.backend}
	h.store.backend = b
	return b
}
func recoveryDisk(t *testing.T, s *Store) []byte {
	t.Helper()
	raw, err := s.db.Get(stateKey, nil)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
func recoveryNoNewPayment(t *testing.T, b *recoveryBackend) {
	t.Helper()
	if b.quotes != 0 || b.signs != 0 {
		t.Fatal("recovery quoted a fee or signed a transaction")
	}
}
func recoveryComplete(t *testing.T, h *settlementHarness, r Report) {
	t.Helper()
	if r.State != "complete" || r.Reason != "" || r.Day != h.clock.UTC().Format("2006-01-02") || r.ObservedAtMS != h.clock.UnixMilli() {
		t.Fatalf("recovery completion claimed daily payment or lost its date: %+v", r)
	}
	for _, b := range r.Balances {
		if b.Available != (Fraction{"0", "1"}) || b.Reason == "below-minimum" || b.Reason == "daily-batch-complete" {
			t.Fatal("recovery invented availability or payout eligibility", b)
		}
	}
	if err := validateReport(h.store.state); err != nil {
		t.Fatal("recovery report changed persisted grammar", err)
	}
}

func TestSettlementRecoverPendingBeforeDueDoesNotPrepare(t *testing.T) {
	h := newSettlementHarness(t)
	ctx := context.Background()
	m := recoveryWatch(h)
	r, err := h.store.RecoverPending(ctx)
	if err != nil {
		t.Fatal(err)
	}
	recoveryComplete(t, h, r)
	if len(h.store.state.Credits) != 0 || len(h.store.state.Batches) != 0 || m.inspects+m.quotes+m.signs+m.observes+m.broadcasts != 0 {
		t.Fatal("empty recovery imported an existing mature source credit or called the backend")
	}
	h.reopen(t)
	if r, err = h.store.View(ctx); err != nil {
		t.Fatal(err)
	}
	recoveryComplete(t, h, r)

	// An ordinary fee-budget deferral has imported credit but no payment. A
	// recovery-only scan must not erase its earnings or call the fee estimator.
	h.backend.fee = 10000001
	if _, err = h.store.RunDaily(ctx); err != nil {
		t.Fatal(err)
	}
	if len(h.store.state.Credits) != 1 || len(h.store.state.Batches) != 0 {
		t.Fatal("fixture did not establish the inherited fee-budget deferral")
	}
	h.backend.fee = 1000
	m = recoveryWatch(h)
	r, err = h.store.RecoverPending(ctx)
	if err != nil {
		t.Fatal(err)
	}
	recoveryComplete(t, h, r)
	b := settlementBalance(t, r, h.gym)
	if b.Confirmed != (Fraction{"990000000", "1"}) || b.PaidAtoms != 0 || b.ReservedAtoms != 0 || b.Reason != "" || m.inspects+m.quotes+m.signs+m.observes+m.broadcasts != 0 {
		t.Fatal("recovery changed credit or made an unrequested readiness/payment decision")
	}
	// Existing history consumers accept the conservative balance without
	// inferring a paid item or the below-minimum reason.
	key, _, _ := settlementFixtureKey(1)
	defer key.Zero()
	history, err := h.store.MinerPayoutHistory(ctx, settlementSessions{hex.EncodeToString(key.PubKey().SerializeCompressed())}, settlementTestToken)
	if err != nil || len(history.Items) != 0 || history.Balance != b {
		t.Fatal("existing authenticated history rejects recovery report", err)
	}
	if _, err = ValidatePayoutHistory(history, h.gym); err != nil {
		t.Fatal(err)
	}
	// RecoverPending did not occupy today's slot: the original daily API can
	// still prepare its exact ordinary payment at the scheduled time.
	if _, err = h.store.RunDaily(ctx); err != nil || len(h.store.state.Batches) != 1 || h.backend.signCalls != 1 || h.backend.broadcasts != 1 {
		t.Fatal("recovery occupied the daily slot", err)
	}
}

func TestSettlementRecoverPendingExactRetryAndNextDay(t *testing.T) {
	h := newSettlementHarness(t)
	ctx := context.Background()
	h.backend.loseAck = true
	if r, err := h.store.RunDaily(ctx); err != nil || r.Reason != "broadcast-unknown" {
		t.Fatal("fixture did not retain uncertain send", err)
	}
	first := clone(h.store.state.Batches[0])
	h.addConfirmedRound(t)
	h.clock = h.clock.Add(24 * time.Hour)
	h.reopen(t)
	m := recoveryWatch(h)
	// A found transaction after lost acknowledgment needs no retransmission.
	if r, err := h.store.RecoverPending(ctx); err != nil || r.State != "pending" || r.Reason != "payout-pending" || m.broadcasts != 0 {
		t.Fatal("known lost-ack transaction was not recovered", err)
	}
	// Definite absence permits only the persisted original bytes. New mature
	// funding exists but must not be imported, quoted, signed or selected.
	delete(h.backend.txs, first.TxID)
	m.inspect = func(ctx context.Context, coins []Coin) ([]Funding, error) {
		if !reflect.DeepEqual(coins, first.Inputs) {
			t.Fatal("recovery selected different inputs")
		}
		return h.backend.Inspect(ctx, coins)
	}
	if r, err := h.store.RecoverPending(ctx); err != nil || r.State != "pending" || r.TxID != first.TxID || m.broadcasts != 1 {
		t.Fatal("absent transaction not retried exactly", err)
	}
	if len(h.store.state.Credits) != 1 || len(h.store.state.Batches) != 1 || !bytes.Equal(h.backend.broadcastBytes[0], h.backend.broadcastBytes[1]) {
		t.Fatal("retry imported credit, duplicated debit or changed signed bytes")
	}
	h.backend.confirmations = 1
	r, err := h.store.RecoverPending(ctx)
	if err != nil {
		t.Fatal(err)
	}
	recoveryComplete(t, h, r)
	recoveryNoNewPayment(t, m)
	got := clone(h.store.state.Batches[0])
	got.State = first.State
	if !reflect.DeepEqual(got, first) || len(h.store.state.Credits) != 1 || len(h.store.state.Batches) != 1 || settlementBalance(t, r, h.gym).PaidAtoms != 990000000 {
		t.Fatal("confirmation changed the saved payment or paid today's new credit")
	}
	h.reopen(t)
	if len(h.store.state.Credits) != 1 || h.store.state.Batches[0].State != "confirmed" {
		t.Fatal("confirmation observation was not durable")
	}
	if _, err = h.store.RunDaily(ctx); err != nil || len(h.store.state.Credits) != 2 || len(h.store.state.Batches) != 2 || h.backend.signCalls != 2 || h.backend.broadcasts != 3 {
		t.Fatal("ordinary next-day preparation changed", err)
	}
	if h.store.state.Batches[1].Payments[0].Atoms != 990000000 {
		t.Fatal("second independent round was debited incorrectly")
	}
}

func TestSettlementRecoverPendingHoldsAndCancellation(t *testing.T) {
	h := newSettlementHarness(t)
	ctx := context.Background()
	if _, err := h.store.RunDaily(ctx); err != nil {
		t.Fatal(err)
	}
	first := clone(h.store.state.Batches[0])
	m := recoveryWatch(h)
	for _, tc := range []struct {
		name, phase, reason string
		observe             func(context.Context, string) (TransactionObservation, error)
		inspect             func(context.Context, []Coin) ([]Funding, error)
		wantErr             error
	}{
		{"lookup-unavailable", "deferred", "transaction-unavailable", func(context.Context, string) (TransactionObservation, error) {
			return TransactionObservation{}, errors.New("controlled unavailable")
		}, nil, nil},
		{"raw-conflict", "deferred", "payout-conflict", func(context.Context, string) (TransactionObservation, error) {
			return TransactionObservation{Raw: []byte{1}}, nil
		}, nil, nil},
		{"missing-raw-confirmed", "", "", func(context.Context, string) (TransactionObservation, error) {
			return TransactionObservation{Confirmations: 1}, nil
		}, nil, ErrBackend},
		{"funding-unavailable", "pending", "funding-unavailable", func(context.Context, string) (TransactionObservation, error) { return TransactionObservation{}, nil }, func(context.Context, []Coin) ([]Funding, error) { return nil, errors.New("controlled unavailable") }, nil},
		{"funding-not-ready", "pending", "funding-conflict", func(context.Context, string) (TransactionObservation, error) { return TransactionObservation{}, nil }, func(_ context.Context, coins []Coin) ([]Funding, error) {
			return []Funding{{TxID: coins[0].TxID, Index: coins[0].Index, State: "immature"}}, nil
		}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m.observe, m.inspect = tc.observe, tc.inspect
			before := recoveryDisk(t, h.store)
			r, err := h.store.RecoverPending(ctx)
			if !errors.Is(err, tc.wantErr) || err == nil && (r.State != tc.phase || r.Reason != tc.reason) {
				t.Fatalf("recovery hold: %+v %v", r, err)
			}
			if err != nil && !bytes.Equal(before, recoveryDisk(t, h.store)) {
				t.Fatal("definite refusal wrote state")
			}
			if len(h.store.state.Credits) != 1 || len(h.store.state.Batches) != 1 || !reflect.DeepEqual(first, h.store.state.Batches[0]) || m.broadcasts != 0 {
				t.Fatal("hold changed saved financial intent or broadcast")
			}
		})
	}
	recoveryNoNewPayment(t, m)
	work, cancel := context.WithCancel(ctx)
	defer cancel()
	m.observe = func(context.Context, string) (TransactionObservation, error) {
		cancel()
		return TransactionObservation{}, context.Canceled
	}
	before := recoveryDisk(t, h.store)
	if _, err := h.store.RecoverPending(work); !errors.Is(err, context.Canceled) || !bytes.Equal(before, recoveryDisk(t, h.store)) || m.broadcasts != 0 {
		t.Fatal("cancellation mutated state or broadcast", err)
	}
	m.observe, m.inspect = nil, nil
	h.backend.confirmations = 1
	if _, err := h.store.RecoverPending(ctx); err != nil {
		t.Fatal(err)
	}
	delete(h.backend.txs, first.TxID)
	h.backend.confirmations = 0
	if r, err := h.store.RecoverPending(ctx); err != nil || r.Reason != "payout-conflict" || h.store.state.Batches[0].State != "confirmed" || m.broadcasts != 0 {
		t.Fatal("lost confirmed payment was released or retransmitted", err)
	}
}

func TestSettlementRecoverPendingCapacityAndLifecycle(t *testing.T) {
	h := newSettlementHarnessWithLimit(t, 1)
	ctx := context.Background()
	if _, err := h.store.RunDaily(ctx); err != nil {
		t.Fatal(err)
	}
	h.addConfirmedRound(t)
	h.backend.confirmations = 1
	m := recoveryWatch(h)
	r, err := h.store.RecoverPending(ctx)
	if err != nil {
		t.Fatal("new credit capacity blocked old payment recovery", err)
	}
	recoveryComplete(t, h, r)
	recoveryNoNewPayment(t, m)
	if len(h.store.state.Credits) != 1 || len(h.store.state.Batches) != 1 || m.inspects != 0 || m.broadcasts != 0 {
		t.Fatal("capacity recovery imported or inspected new funding")
	}
	if r, err = h.store.RunDaily(ctx); err != nil || r.Reason != "capacity" {
		t.Fatal("existing RunDaily capacity semantics changed", err)
	}
	before := recoveryDisk(t, h.store)
	h.store.gate <- struct{}{}
	_, err = h.store.RecoverPending(ctx)
	<-h.store.gate
	if !errors.Is(err, ErrBusy) || !bytes.Equal(before, recoveryDisk(t, h.store)) {
		t.Fatal("recovery bypassed original gate", err)
	}
	h.clock = h.clock.Add(-time.Minute)
	if _, err = h.store.RecoverPending(ctx); !errors.Is(err, ErrClock) || !bytes.Equal(before, recoveryDisk(t, h.store)) {
		t.Fatal("recovery accepted clock rollback", err)
	}
	h.clock = h.clock.Add(time.Minute)
	if err = h.store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = h.store.RecoverPending(ctx); !errors.Is(err, ErrClosed) {
		t.Fatal("closed store accepted recovery", err)
	}
	audit, err := OpenAudit(h.path, h.config, h.source)
	if err != nil {
		t.Fatal(err)
	}
	defer audit.Close()
	if _, err = audit.RecoverPending(ctx); !errors.Is(err, ErrInvalid) || !bytes.Equal(before, recoveryDisk(t, audit)) {
		t.Fatal("read-only audit owner accepted recovery", err)
	}
}

func TestSettlementRecoverPendingUnknownWriteNeverSends(t *testing.T) {
	h := newSettlementHarness(t)
	ctx := context.Background()
	if _, err := h.store.RunDaily(ctx); err != nil {
		t.Fatal(err)
	}
	first := clone(h.store.state.Batches[0])
	delete(h.backend.txs, first.TxID)
	m := recoveryWatch(h)
	write := h.store.writeBatch
	h.store.writeBatch = func(batch *leveldb.Batch, options *opt.WriteOptions) error {
		if options == nil || !options.Sync {
			t.Fatal("recovery write was not synchronous")
		}
		if err := write(batch, options); err != nil {
			return err
		}
		return errors.New("controlled unknown write outcome")
	}
	if _, err := h.store.RecoverPending(ctx); !errors.Is(err, ErrPoisoned) || m.broadcasts != 0 {
		t.Fatal("unknown recovery write broadcast or lost poison", err)
	}
	if _, err := h.store.RecoverPending(ctx); !errors.Is(err, ErrPoisoned) {
		t.Fatal("same poisoned handle allowed recovery", err)
	}
	recoveryNoNewPayment(t, m)
	h.reopen(t)
	m = recoveryWatch(h)
	if r, err := h.store.RecoverPending(ctx); err != nil || r.TxID != first.TxID || m.broadcasts != 1 {
		t.Fatal("reopen did not recover exact transaction", err)
	}
	recoveryNoNewPayment(t, m)
	if !bytes.Equal(h.backend.broadcastBytes[0], h.backend.broadcastBytes[1]) || len(h.store.state.Batches) != 1 {
		t.Fatal("recovery after unknown write changed or duplicated intent")
	}
	if err := h.source.Close(); err != nil {
		t.Fatal(err)
	}
	before := recoveryDisk(t, h.store)
	observed := m.observes
	if _, err := h.store.RecoverPending(ctx); !errors.Is(err, ErrSource) || m.observes != observed || !bytes.Equal(before, recoveryDisk(t, h.store)) {
		t.Fatal("missing original source was bypassed", err)
	}
}

func TestSettlementRecoverPendingRejectsCheapProfile(t *testing.T) {
	if !wire.TestPoW {
		t.Skip("cheap-profile refusal")
	}
	var s *Store
	if _, err := s.RecoverPending(context.Background()); !errors.Is(err, ErrProfile) {
		t.Fatal("recovery touched state before profile refusal", err)
	}
}
