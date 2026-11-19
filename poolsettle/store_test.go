// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettle

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/wire/v2"
)

type settlementRevokedSession struct{ key string }

func (s settlementRevokedSession) WithSession(_ context.Context, _ string, fn func(string, func() error) error) error {
	return fn(s.key, func() error { return errors.New("fixture session revoked during view") })
}

type settlementSwallowedRevocation struct{ key string }

func (s settlementSwallowedRevocation) WithSession(_ context.Context, _ string, fn func(string, func() error) error) error {
	_ = fn(s.key, func() error { return errors.New("fixture swallowed revocation") })
	return nil
}

func TestSettlementPayoutDurableBeforeBroadcastAndRetry(t *testing.T) {
	h := newSettlementHarness(t)
	ctx := context.Background()
	h.backend.beforeBroadcast = func() {
		if len(h.store.state.Batches) != 1 {
			t.Fatal("broadcast without retained reservation")
		}
		it := h.store.db.NewIterator(nil, nil)
		defer it.Release()
		found := false
		for it.Next() {
			var stored state
			if json.Unmarshal(it.Value(), &stored) == nil && len(stored.Batches) == 1 && stored.Batches[0].Raw == h.store.state.Batches[0].Raw {
				found = true
			}
		}
		if it.Error() != nil || !found {
			t.Fatal("broadcast before durable exact transaction")
		}
	}
	r, err := h.store.RunDaily(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(h.store.state.Batches) != 1 || h.backend.broadcasts != 1 || h.backend.signCalls != 1 || len(h.backend.txs) != 1 {
		t.Fatalf("first payment: %+v backend %+v", r, h.backend)
	}
	batch := h.store.state.Batches[0]
	if len(batch.Payments) != 1 || batch.Payments[0].Atoms != 990000000 || batch.FeeAtoms != 1000 || batch.Change == nil || batch.Change.Atoms != 9999000 {
		t.Fatalf("fee charged beyond 1%% or value lost: %+v", batch)
	}
	if len(batch.Inputs) != 1 || !batch.Inputs[0].Coinbase || batch.Inputs[0].Atoms != 1000000000 {
		t.Fatal("payment did not use exact original reward")
	}
	raw, err := hex.DecodeString(batch.Raw)
	if err != nil {
		t.Fatal(err)
	}
	tx := wire.NewMsgTx(2)
	if err = tx.Deserialize(bytes.NewReader(raw)); err != nil {
		t.Fatal(err)
	}
	if len(tx.TxOut) != 2 || tx.TxOut[0].Value != 990000000 || tx.TxOut[1].Value != 9999000 {
		t.Fatal("actual signed transaction differs from exact hand-derived payment")
	}
	b := settlementBalance(t, r, h.gym)
	if b.ReservedAtoms != 990000000 || b.PaidAtoms != 0 || b.Confirmed != (Fraction{"990000000", "1"}) {
		t.Fatalf("mempool money shown as paid: %+v", b)
	}
	firstID := batch.TxID
	if _, err = h.store.RunDaily(ctx); err != nil {
		t.Fatal(err)
	}
	if h.backend.broadcasts != 1 || h.backend.signCalls != 1 || len(h.store.state.Batches) != 1 {
		t.Fatal("same-day retry created another payment")
	}
	h.backend.confirmations = 1
	r, err = h.store.RunDaily(ctx)
	if err != nil {
		t.Fatal(err)
	}
	b = settlementBalance(t, r, h.gym)
	if b.PaidAtoms != 990000000 || b.ReservedAtoms != 0 || b.Confirmed != (Fraction{"0", "1"}) {
		t.Fatalf("confirmed debit not applied exactly once: %+v", b)
	}
	h.reopen(t)
	if _, err = h.store.RunDaily(ctx); err != nil {
		t.Fatal(err)
	}
	if h.store.state.Batches[0].TxID != firstID || h.backend.broadcasts != 1 || h.backend.signCalls != 1 || len(h.store.state.Batches) != 1 {
		t.Fatal("reopened duplicate changed payout identity")
	}
	h.clock = h.clock.Add(24 * time.Hour)
	if _, err = h.store.RunDaily(ctx); err != nil {
		t.Fatal(err)
	}
	if len(h.store.state.Batches) != 1 || h.backend.broadcasts != 1 {
		t.Fatal("fee-only or duplicate next-day payment")
	}
}

func TestSettlementDeferralsKeepExactEarningsVisible(t *testing.T) {
	for _, tc := range []struct {
		name, funding, reason string
		fee                   int64
	}{
		{"immature", "immature", "immature", 1000},
		{"fee-spike", "ready", "fee-budget", 10000001},
		{"funding-conflict", "funding-conflict", "funding-conflict", 1000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newSettlementHarness(t)
			h.backend.funding, h.backend.fee = tc.funding, tc.fee
			r, err := h.store.RunDaily(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			b := settlementBalance(t, r, h.gym)
			if b.Reason != tc.reason || b.Confirmed != (Fraction{"990000000", "1"}) || b.PaidAtoms != 0 || b.ReservedAtoms != 0 {
				t.Fatalf("deferral hidden or reduced earnings: %+v", r)
			}
			if h.backend.signCalls != 0 || h.backend.broadcasts != 0 || len(h.store.state.Batches) != 0 {
				t.Fatal("deferred payout signed or transmitted")
			}
			view, err := h.store.View(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if settlementBalance(t, view, h.gym).Reason != tc.reason {
				t.Fatal("balance view lost deferral reason")
			}
			h.reopen(t)
			view, err = h.store.View(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if settlementBalance(t, view, h.gym).Reason != tc.reason {
				t.Fatal("restart lost deferred balance reason")
			}
			h.backend.funding, h.backend.fee = "ready", 1000
			if _, err = h.store.RunDaily(context.Background()); err != nil {
				t.Fatal(err)
			}
			if h.backend.broadcasts != 1 || h.store.state.Batches[0].Payments[0].Atoms != 990000000 {
				t.Fatal("deferral charged an extra fee or prevented same-day recovery")
			}
		})
	}
}

func TestSettlementUnknownBroadcastRecoversSameTransaction(t *testing.T) {
	h := newSettlementHarness(t)
	h.backend.loseAck = true
	r, err := h.store.RunDaily(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if h.backend.broadcasts != 1 || len(h.store.state.Batches) != 1 {
		t.Fatal("missing uncertain transaction")
	}
	b := settlementBalance(t, r, h.gym)
	if b.Reason != "broadcast-unknown" || b.ReservedAtoms != 990000000 || b.PaidAtoms != 0 {
		t.Fatalf("unknown outcome released or paid reservation: %+v", r)
	}
	id, raw := h.store.state.Batches[0].TxID, h.store.state.Batches[0].Raw
	h.reopen(t)
	if _, err = h.store.RunDaily(context.Background()); err != nil {
		t.Fatal(err)
	}
	if h.backend.broadcasts != 1 || h.backend.signCalls != 1 || len(h.backend.txs) != 1 || h.store.state.Batches[0].TxID != id || h.store.state.Batches[0].Raw != raw {
		t.Fatal("lost acknowledgment produced a replacement payment")
	}
}

func TestSettlementCreditCapacityDoesNotBlockPendingRecovery(t *testing.T) {
	h := newSettlementHarnessWithLimit(t, 1)
	h.backend.loseAck = true
	if _, err := h.store.RunDaily(context.Background()); err != nil {
		t.Fatal(err)
	}
	id, raw := h.store.state.Batches[0].TxID, h.store.state.Batches[0].Raw
	h.addConfirmedRound(t)
	h.reopen(t)
	if _, err := h.store.RunDaily(context.Background()); err != nil {
		t.Fatal("new source credits blocked retained pending recovery", err)
	}
	if h.store.state.Batches[0].TxID != id || h.store.state.Batches[0].Raw != raw || h.backend.broadcasts != 1 || h.backend.signCalls != 1 || len(h.store.state.Credits) != 1 {
		t.Fatal("capacity recovery replaced payout or partially imported excess credits")
	}
	h.backend.confirmations = 1
	r, err := h.store.RunDaily(context.Background())
	if err != nil {
		t.Fatal("new source credit capacity blocked confirmation observation", err)
	}
	if r.State != "deferred" || r.Reason != "capacity" || h.store.state.Batches[0].State != "confirmed" || settlementBalance(t, r, h.gym).PaidAtoms != 990000000 {
		t.Fatalf("capacity discarded existing payment confirmation: %+v", r)
	}
	h.reopen(t)
	if h.store.state.Batches[0].State != "confirmed" || len(h.store.state.Credits) != 1 || h.backend.broadcasts != 1 {
		t.Fatal("capacity confirmation was not durable")
	}
}

func TestSettlementSigningAcrossMidnightUsesPreparationDay(t *testing.T) {
	h := newSettlementHarness(t)
	h.clock = time.Date(2030, 1, 1, 23, 59, 59, 999000000, time.UTC)
	h.backend.afterSign = func() { h.clock = h.clock.Add(2 * time.Millisecond) }
	r, err := h.store.RunDaily(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(h.store.state.Batches) != 1 || h.store.state.Batches[0].Day != "2030-01-02" || h.store.state.Batches[0].CreatedAtMS != h.clock.UnixMilli() || r.Day != "2030-01-02" {
		t.Fatalf("slow signing retained yesterday's daily slot: %+v", r)
	}
	id := h.store.state.Batches[0].TxID
	h.backend.afterSign = nil
	h.addConfirmedRound(t)
	h.backend.confirmations = 1
	h.reopen(t)
	if _, err = h.store.RunDaily(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(h.store.state.Credits) != 2 || len(h.store.state.Batches) != 1 || h.store.state.Batches[0].TxID != id || h.backend.broadcasts != 1 || h.backend.signCalls != 1 {
		t.Fatal("midnight boundary allowed a second payment that calendar day")
	}
}

func TestSettlementRejectsCorruptPersistedBalanceReport(t *testing.T) {
	h := newSettlementHarness(t)
	h.backend.fee = 10000001
	if _, err := h.store.RunDaily(context.Background()); err != nil {
		t.Fatal(err)
	}
	original := clone(h.store.state)
	if err := h.store.Close(); err != nil {
		t.Fatal(err)
	}
	writeState := func(st state) {
		t.Helper()
		db, err := leveldb.OpenFile(filepath.Join(h.path, "db"), &opt.Options{ErrorIfMissing: true, Strict: opt.StrictAll})
		if err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal(st)
		if err == nil {
			err = db.Put(stateKey, raw, &opt.WriteOptions{Sync: true})
		}
		closeErr := db.Close()
		if err != nil || closeErr != nil {
			t.Fatalf("write corrupt fixture report: %v %v", err, closeErr)
		}
	}
	for _, tc := range []struct {
		name string
		edit func(*state)
	}{
		{"invented-confirmed-earnings", func(st *state) { st.Report.Balances[0].Confirmed.Numerator = "990000001" }},
		{"invented-available-balance", func(st *state) { st.Report.Balances[0].Available.Numerator = "990000001" }},
		{"different-miner", func(st *state) { st.Report.Balances[0].GymID = strings.Repeat("b", 64) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			corrupt := clone(original)
			tc.edit(&corrupt)
			writeState(corrupt)
			opened, err := Open(h.path, h.config, h.source, h.backend)
			if opened != nil {
				_ = opened.Close()
			}
			if !errors.Is(err, ErrCorrupt) {
				t.Fatalf("untrusted persisted report accepted: %v", err)
			}
		})
	}
	writeState(original)
	var err error
	h.store, err = Open(h.path, h.config, h.source, h.backend)
	if err != nil {
		t.Fatal(err)
	}
	h.store.now = func() time.Time { return h.clock }
}

func TestSettlementUncertainIntentWriteNeverSendsBeforeRecovery(t *testing.T) {
	for _, after := range []bool{false, true} {
		name := "before-commit"
		if after {
			name = "after-commit"
		}
		t.Run(name, func(t *testing.T) {
			h := newSettlementHarness(t)
			write := h.store.writeBatch
			injected := false
			h.store.writeBatch = func(batch *leveldb.Batch, options *opt.WriteOptions) error {
				if !injected && strings.Contains(string(batch.Dump()), `"state":"prepared"`) {
					injected = true
					if options == nil || !options.Sync {
						t.Fatal("payout intent was not synchronously written")
					}
					if after {
						if err := write(batch, options); err != nil {
							return err
						}
					}
					return errors.New("fixture unknown durable outcome")
				}
				return write(batch, options)
			}
			if _, err := h.store.RunDaily(context.Background()); !errors.Is(err, ErrPoisoned) {
				t.Fatalf("uncertain intent result %v", err)
			}
			if !injected || h.backend.broadcasts != 0 {
				t.Fatal("sent before uncertain intent recovery")
			}
			if _, err := h.store.RunDaily(context.Background()); !errors.Is(err, ErrPoisoned) {
				t.Fatal("poisoned journal permitted retry")
			}
			h.reopen(t)
			if _, err := h.store.RunDaily(context.Background()); err != nil {
				t.Fatal(err)
			}
			if h.backend.broadcasts != 1 || len(h.store.state.Batches) != 1 || len(h.backend.txs) != 1 {
				t.Fatal("recovery lost or duplicated retained intent")
			}
			if after && h.backend.signCalls != 1 {
				t.Fatal("recovery signed a replacement for committed transaction")
			}
		})
	}
}

func TestSettlementRejectsSignerMutationAndBackwardClock(t *testing.T) {
	h := newSettlementHarness(t)
	h.backend.badSigner = true
	if _, err := h.store.RunDaily(context.Background()); err == nil {
		t.Fatal("accepted signer destination substitution")
	}
	if h.backend.broadcasts != 0 || len(h.store.state.Batches) != 0 {
		t.Fatal("bad signed payment reserved or submitted")
	}
	h.backend.badSigner = false
	if _, err := h.store.RunDaily(context.Background()); err != nil {
		t.Fatal(err)
	}
	h.clock = h.clock.Add(-time.Minute)
	if _, err := h.store.RunDaily(context.Background()); !errors.Is(err, ErrClock) {
		t.Fatalf("backward clock allowed: %v", err)
	}
	if h.backend.broadcasts != 1 || len(h.store.state.Batches) != 1 {
		t.Fatal("backward clock duplicated payment")
	}
}

func TestSettlementMinerBalanceKeepsSessionAndDeferral(t *testing.T) {
	h := newSettlementHarness(t)
	h.backend.fee = 10000001
	if _, err := h.store.RunDaily(context.Background()); err != nil {
		t.Fatal(err)
	}
	key, _, _ := settlementFixtureKey(1)
	public := hex.EncodeToString(key.PubKey().SerializeCompressed())
	b, observed, err := h.store.MinerBalance(context.Background(), settlementSessions{public}, settlementTestToken)
	if err != nil || b.GymID != h.gym || b.Reason != "fee-budget" || b.Confirmed != (Fraction{"990000000", "1"}) || !observed.Equal(h.clock) {
		t.Fatalf("miner did not receive own deferred balance: %+v %v %v", b, observed, err)
	}
	if _, _, err = h.store.MinerBalance(context.Background(), settlementSessions{public}, "another-session"); err == nil {
		t.Fatal("unauthenticated balance exposed")
	}
	if b, _, err = h.store.MinerBalance(context.Background(), settlementRevokedSession{public}, settlementTestToken); err == nil || b.GymID != "" {
		t.Fatal("revoked original session exposed retained balance")
	}
	if b, _, err = h.store.MinerBalance(context.Background(), settlementSwallowedRevocation{public}, settlementTestToken); err == nil || b.GymID != "" {
		t.Fatal("authorizer swallowing callback error exposed revoked balance")
	}
}

func TestSettlementRejectsCheapProfile(t *testing.T) {
	if !wire.TestPoW {
		t.Skip("cheap-profile refusal")
	}
	if _, err := Create(t.TempDir()+"/settlement", Config{}, nil, nil); !errors.Is(err, ErrProfile) {
		t.Fatalf("cheap profile create: %v", err)
	}
	if _, err := Open(t.TempDir()+"/settlement", Config{}, nil, nil); !errors.Is(err, ErrProfile) {
		t.Fatalf("cheap profile open: %v", err)
	}
}
