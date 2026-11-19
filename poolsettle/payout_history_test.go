// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettle

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/taytr-llc/slug/wire/v2"
)

// These are explicitly modeled shape cases, not asserted chain transactions.
func payoutShapeFixture(count int) PayoutHistory {
	h := PayoutHistory{Version: 1, RecipientScript: "0014" + strings.Repeat("1", 40), Balance: Balance{GymID: strings.Repeat("a", 64), Confirmed: Fraction{"0", "1"}, Available: Fraction{"0", "1"}, PaidAtoms: int64(count) * MinimumAtoms}, Items: make([]PayoutItem, count)}
	for i := range h.Items {
		date := time.Date(2026, 1, 1+count-i, 12, 0, 0, 0, time.UTC)
		h.Items[i] = PayoutItem{date.Format("2006-01-02"), date.UnixMilli(), fmt.Sprintf("%064x", i+1), 1, MinimumAtoms, "confirmed"}
		if i == 0 {
			h.SettlementObservedAtMS = date.UnixMilli()
		}
	}
	return h
}

func TestPayoutHistoryShapeOwnershipAndCapacity(t *testing.T) {
	for _, count := range []int{0, 1, 8, 9, 256, 366} {
		h := payoutShapeFixture(count)
		got, err := ValidatePayoutHistory(h, h.Balance.GymID)
		if err != nil || !reflect.DeepEqual(got, h) || got.Items == nil {
			t.Fatal("bounded modeled capture", count, err)
		}
		if count > 0 {
			got.Items[0].Atoms++
			if h.Items[0].Atoms != MinimumAtoms {
				t.Fatal("validator aliased caller items")
			}
		}
	}
	upper := payoutShapeFixture(1)
	upper.SettlementObservedAtMS = maxPayoutTimeMS
	upper.Items[0].CreatedAtMS, upper.Items[0].Day = maxPayoutTimeMS, "9999-12-31"
	if _, err := ValidatePayoutHistory(upper, upper.Balance.GymID); err != nil {
		t.Fatal("last representable candidate millisecond", err)
	}
}

func TestPayoutHistoryShapeRefusals(t *testing.T) {
	for name, mutate := range map[string]func(*PayoutHistory){
		"nil items":          func(h *PayoutHistory) { h.Items = nil },
		"capacity":           func(h *PayoutHistory) { *h = payoutShapeFixture(367) },
		"wrong gym":          func(h *PayoutHistory) { h.Balance.GymID = strings.Repeat("b", 64) },
		"upper hash":         func(h *PayoutHistory) { h.Items[0].TxID = strings.Repeat("A", 64) },
		"duplicate tx":       func(h *PayoutHistory) { h.Items[1].TxID = h.Items[0].TxID },
		"day mismatch":       func(h *PayoutHistory) { h.Items[0].Day = "2026-02-30" },
		"equal times":        func(h *PayoutHistory) { h.Items[1].CreatedAtMS = h.Items[0].CreatedAtMS },
		"future preparation": func(h *PayoutHistory) { h.Items[0].CreatedAtMS++ },
		"time ceiling":       func(h *PayoutHistory) { h.SettlementObservedAtMS = maxPayoutTimeMS + 1 },
		"unknown state":      func(h *PayoutHistory) { h.Items[0].RecordedState = "final" },
		"older pending":      func(h *PayoutHistory) { h.Items[1].RecordedState = "broadcast" },
		"output bound":       func(h *PayoutHistory) { h.Items[0].OutputIndex = 256 },
		"minimum":            func(h *PayoutHistory) { h.Items[0].Atoms = MinimumAtoms - 1 },
		"paid mismatch":      func(h *PayoutHistory) { h.Balance.PaidAtoms++ },
		"reserved mismatch":  func(h *PayoutHistory) { h.Balance.ReservedAtoms++ },
		"recipient missing":  func(h *PayoutHistory) { h.RecipientScript = "" },
		"script wrong":       func(h *PayoutHistory) { h.RecipientScript = "0015" + strings.Repeat("1", 40) },
		"fraction too long":  func(h *PayoutHistory) { h.Balance.Confirmed.Numerator = strings.Repeat("1", 4097) },
		"fraction unreduced": func(h *PayoutHistory) { h.Balance.Confirmed = Fraction{"0", "2"} },
		"fraction ceiling":   func(h *PayoutHistory) { h.Balance.Confirmed = Fraction{"2100000000000001", "1"} },
		"available excess":   func(h *PayoutHistory) { h.Balance.Available = Fraction{"1", "1"} },
		"unexpected reason":  func(h *PayoutHistory) { h.Balance.Reason = "backend detail" },
		"missing report":     func(h *PayoutHistory) { h.SettlementObservedAtMS = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			h := payoutShapeFixture(2)
			expectedGym := h.Balance.GymID
			mutate(&h)
			got, err := ValidatePayoutHistory(h, expectedGym)
			if !errors.Is(err, ErrInvalid) || !reflect.DeepEqual(got, PayoutHistory{}) {
				t.Fatal("invalid supplied history disclosed", err)
			}
		})
	}
}

func TestPayoutHistoryPendingAndUnconfiguredGrammar(t *testing.T) {
	h := payoutShapeFixture(2)
	h.Items[0].RecordedState = "prepared"
	h.Balance.PaidAtoms = MinimumAtoms
	h.Balance.ReservedAtoms = MinimumAtoms
	h.Balance.Confirmed = Fraction{"100000000", "1"}
	h.Balance.Reason = "payout-pending"
	if _, err := ValidatePayoutHistory(h, h.Balance.GymID); err != nil {
		t.Fatal("prepared reservation", err)
	}
	h.Items[0].RecordedState = "broadcast"
	if _, err := ValidatePayoutHistory(h, h.Balance.GymID); err != nil {
		t.Fatal("broadcast reservation", err)
	}
	h = payoutShapeFixture(0)
	h.RecipientScript = ""
	h.Balance.Confirmed = Fraction{"1", "3"}
	h.Balance.Reason = "destination-unavailable"
	h.SettlementObservedAtMS = 1
	if _, err := ValidatePayoutHistory(h, h.Balance.GymID); err != nil {
		t.Fatal("unconfigured unpaid gym", err)
	}
	h.Balance.Reason = "below-minimum"
	if _, err := ValidatePayoutHistory(h, h.Balance.GymID); err == nil {
		t.Fatal("missing destination was hidden")
	}
}

type payoutForbiddenBackend struct{ Backend }

func (payoutForbiddenBackend) ObserveTransaction(context.Context, string) (TransactionObservation, error) {
	panic("history performed fresh transaction observation")
}
func (payoutForbiddenBackend) Inspect(context.Context, []Coin) ([]Funding, error) {
	panic("history inspected funding")
}
func (payoutForbiddenBackend) EstimateFee(context.Context, []Coin, []Payment, string) (int64, error) {
	panic("history estimated fees")
}
func (payoutForbiddenBackend) Sign(context.Context, []Coin, []byte) ([]byte, error) {
	panic("history signed")
}
func (payoutForbiddenBackend) Broadcast(context.Context, []byte) (string, error) {
	panic("history broadcast")
}

func TestPayoutHistoryConcreteReadOnlyCaptureAndReopen(t *testing.T) {
	h := newSettlementHarness(t)
	ctx := context.Background()
	key, _, _ := settlementFixtureKey(1)
	sessions := settlementSessions{hex.EncodeToString(key.PubKey().SerializeCompressed())}
	key.Zero()
	read := func() PayoutHistory {
		t.Helper()
		before := clone(h.store.state)
		h.store.backend = payoutForbiddenBackend{}
		got, err := h.store.MinerPayoutHistory(ctx, sessions, settlementTestToken)
		h.store.backend = h.backend
		if err != nil || !same(before, h.store.state) {
			t.Fatal("history changed ledger or failed", err)
		}
		return got
	}
	initial := read()
	if initial.RecipientScript != h.config.Destinations[0].Script || len(initial.Items) != 0 || initial.SettlementObservedAtMS != 0 {
		t.Fatal("initial configured recipient lost")
	}
	if _, err := h.store.RunDaily(ctx); err != nil {
		t.Fatal(err)
	}
	pending := read()
	if len(pending.Items) != 1 || pending.Items[0].RecordedState != "broadcast" || pending.Items[0].OutputIndex != 0 || pending.Items[0].Atoms != 990000000 || pending.Balance.ReservedAtoms != 990000000 || pending.Balance.PaidAtoms != 0 {
		t.Fatal("original hand-derived reservation not preserved")
	}
	h.backend.confirmations = 1
	if _, err := h.store.RunDaily(ctx); err != nil {
		t.Fatal(err)
	}
	confirmed := read()
	if confirmed.Items[0].RecordedState != "confirmed" || confirmed.Balance.PaidAtoms != 990000000 || confirmed.Balance.ReservedAtoms != 0 || confirmed.Items[0].TxID != pending.Items[0].TxID || pending.Items[0].RecordedState != "broadcast" {
		t.Fatal("scheduler rebound captured history")
	}
	h.reopen(t)
	if got := read(); !reflect.DeepEqual(got, confirmed) || h.backend.broadcasts != 1 {
		t.Fatal("reopen changed original payment")
	}
	foreign, _, _ := settlementFixtureKey(2)
	other := settlementSessions{hex.EncodeToString(foreign.PubKey().SerializeCompressed())}
	foreign.Zero()
	empty, err := h.store.MinerPayoutHistory(ctx, other, settlementTestToken)
	if err != nil || len(empty.Items) != 0 || empty.RecipientScript != "" || empty.Balance.GymID == h.gym || empty.Balance.PaidAtoms != 0 {
		t.Fatal("foreign gym received own history", err)
	}
}

func TestPayoutHistoryOriginalSessionAndCloseFence(t *testing.T) {
	for _, reset := range []bool{true, false} {
		t.Run(fmt.Sprint("reset=", reset), func(t *testing.T) {
			h := newSettlementHarness(t)
			key, _, _ := settlementFixtureKey(1)
			public := hex.EncodeToString(key.PubKey().SerializeCompressed())
			key.Zero()
			entered, release, sourceDone := make(chan struct{}), make(chan struct{}), make(chan error, 1)
			go func() {
				_, err := h.issuer.IssueManifestJob(h.node.template.Context, public, func() error {
					close(entered)
					<-release
					return errors.New("controlled source released")
				})
				sourceDone <- err
			}()
			<-entered
			var once sync.Once
			defer func() { once.Do(func() { close(release) }); <-sourceDone }()
			a := &balanceFenceSessions{gate: make(chan struct{}, 1), captured: make(chan struct{}), key: public}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			type answer struct {
				h   PayoutHistory
				err error
			}
			done := make(chan answer, 1)
			go func() {
				value, err := h.store.MinerPayoutHistory(ctx, a, settlementTestToken)
				done <- answer{value, err}
			}()
			select {
			case <-a.captured:
			case <-ctx.Done():
				t.Fatal("history failed to capture original token")
			}
			select {
			case a.gate <- struct{}{}:
				if reset {
					a.revoked = true
				}
				<-a.gate
			case <-time.After(time.Second):
				t.Fatal("source work held account lease")
			}
			var closed chan error
			if !reset {
				closed = make(chan error, 1)
				go func() { closed <- h.store.Close() }()
				select {
				case <-closed:
					t.Fatal("Close detached active source read")
				case <-time.After(20 * time.Millisecond):
				}
				// Cancel only after proving that Close is joining this live
				// operation. Cancellation before source entry may legitimately
				// end the read without waiting for the held registry.
				cancel()
			}
			once.Do(func() { close(release) })
			select {
			case got := <-done:
				if got.err == nil || !reflect.DeepEqual(got.h, PayoutHistory{}) {
					t.Fatal("stale history disclosed", got.err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("history failed to join")
			}
			if closed != nil {
				select {
				case err := <-closed:
					if err != nil {
						t.Fatal(err)
					}
				case <-time.After(time.Second):
					t.Fatal("Close failed to join")
				}
			}
		})
	}
}

func TestPayoutHistoryProfileRefusalBeforeState(t *testing.T) {
	if !wire.TestPoW {
		t.Skip("cheap profile only")
	}
	var store *Store
	if _, err := store.MinerPayoutHistory(nil, nil, ""); !errors.Is(err, ErrProfile) {
		t.Fatal("cheap profile did not refuse before state", err)
	}
}
