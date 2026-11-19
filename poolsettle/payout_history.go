// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettle

import (
	"context"
	"encoding/json"
	"math/big"
	"time"

	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/internal/mininglease"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/wire/v2"
)

const MaxPayoutItems = 366
const MaxPayoutHistorySize = 131072
const MaxPayoutItemSize = 256
const MaxPayoutBalanceSize = 17408
const maxPayoutTimeMS int64 = 253402300799999

// PayoutItem preserves an original payment output and its recorded pool state.
// CreatedAtMS is preparation time; RecordedState is not fresh chain evidence.
type PayoutItem struct {
	Day           string `json:"day"`
	CreatedAtMS   int64  `json:"created_at_ms"`
	TxID          string `json:"txid"`
	OutputIndex   uint32 `json:"output_index"`
	Atoms         int64  `json:"atoms"`
	RecordedState string `json:"recorded_state"`
}

// PayoutHistory contains only one gym's payments and balance from the same
// journal state. The timestamp dates the report, not each item's confirmation.
type PayoutHistory struct {
	Version                uint32       `json:"version"`
	RecipientScript        string       `json:"recipient_script"`
	Balance                Balance      `json:"balance"`
	SettlementObservedAtMS int64        `json:"settlement_observed_at_ms"`
	Items                  []PayoutItem `json:"items"`
}

// ValidatePayoutHistory checks bounded supplied shape and consistency only. It
// authenticates neither a source nor a caller and returns an owned Items slice.
func ValidatePayoutHistory(h PayoutHistory, expectedGym string) (PayoutHistory, error) {
	bad := func() (PayoutHistory, error) { return PayoutHistory{}, ErrInvalid }
	if _, ok := hexBytes(expectedGym, 32); !ok || h.Version != Version1 || h.Balance.GymID != expectedGym || h.Items == nil || len(h.Items) > MaxPayoutItems || h.SettlementObservedAtMS < 0 || h.SettlementObservedAtMS > maxPayoutTimeMS {
		return bad()
	}
	if h.RecipientScript != "" {
		if _, ok := nativeScript(h.RecipientScript); !ok {
			return bad()
		}
	}
	b := h.Balance
	if b.PaidAtoms < 0 || b.PaidAtoms > MaxMoney || b.ReservedAtoms < 0 || b.ReservedAtoms > MaxMoney-b.PaidAtoms || !knownReason(b.Reason) {
		return bad()
	}
	confirmed, err := rat(b.Confirmed)
	if err != nil || confirmed.Cmp(new(big.Rat).SetInt64(MaxMoney)) > 0 {
		return bad()
	}
	available, err := rat(b.Available)
	if err != nil {
		return bad()
	}
	unreserved := new(big.Rat).Sub(confirmed, new(big.Rat).SetInt64(b.ReservedAtoms))
	if unreserved.Sign() < 0 || available.Cmp(unreserved) > 0 || confirmed.Sign() == 0 && b.Reason != "" {
		return bad()
	}
	if h.RecipientScript == "" && (len(h.Items) != 0 || b.PaidAtoms != 0 || b.ReservedAtoms != 0 || confirmed.Sign() > 0 && b.Reason != "destination-unavailable") {
		return bad()
	}
	if h.SettlementObservedAtMS == 0 && (len(h.Items) != 0 || confirmed.Sign() != 0 || available.Sign() != 0 || b.PaidAtoms != 0 || b.ReservedAtoms != 0 || b.Reason != "") {
		return bad()
	}
	encoded, err := json.Marshal(b)
	if err != nil || len(encoded) > MaxPayoutBalanceSize {
		return bad()
	}
	seen := make(map[string]bool, len(h.Items))
	var paid, reserved int64
	for i, item := range h.Items {
		if len(item.Day) != 10 || item.CreatedAtMS < 1 || item.CreatedAtMS > h.SettlementObservedAtMS || item.OutputIndex > 255 || item.Atoms < MinimumAtoms || item.Atoms > MaxMoney || i > 0 && (item.Day >= h.Items[i-1].Day || item.CreatedAtMS >= h.Items[i-1].CreatedAtMS) {
			return bad()
		}
		if _, ok := hexBytes(item.TxID, 32); !ok || seen[item.TxID] || time.UnixMilli(item.CreatedAtMS).UTC().Format("2006-01-02") != item.Day {
			return bad()
		}
		seen[item.TxID] = true
		switch item.RecordedState {
		case "confirmed":
			if paid > MaxMoney-item.Atoms {
				return bad()
			}
			paid += item.Atoms
		case "prepared", "broadcast":
			if i != 0 {
				return bad()
			}
			reserved = item.Atoms
		default:
			return bad()
		}
		encoded, err = json.Marshal(item)
		if err != nil || len(encoded) > MaxPayoutItemSize {
			return bad()
		}
	}
	if paid != b.PaidAtoms || reserved != b.ReservedAtoms || paid > MaxMoney-reserved {
		return bad()
	}
	encoded, err = json.Marshal(h)
	if err != nil || len(encoded) > MaxPayoutHistorySize {
		return bad()
	}
	owned := make([]PayoutItem, len(h.Items))
	copy(owned, h.Items)
	h.Items = owned
	return h, nil
}

// MinerPayoutHistory verifies the concrete journal once outside account leases.
// It observes no node/backend and never advances settlement or its clock.
func (s *Store) MinerPayoutHistory(ctx context.Context, sessions prospectservice.SessionAuthorizer, token string) (PayoutHistory, error) {
	if wire.TestPoW {
		return PayoutHistory{}, ErrProfile
	}
	if !nonnil(sessions) {
		return PayoutHistory{}, ErrInvalid
	}
	end, err := s.begin(ctx)
	if err != nil {
		return PayoutHistory{}, err
	}
	defer end()
	if s.source == nil {
		return PayoutHistory{}, ErrSource
	}
	lease, err := mininglease.Capture(ctx, sessions, token)
	if err != nil {
		return PayoutHistory{}, err
	}
	p := s.state.Source.Policy
	gym, err := gymauth.GymID(prospect.Context{Version: p.Version, Network: p.Network, Genesis: p.Genesis, DNAVersion: p.DNAVersion}, lease.PublicKey())
	if err != nil {
		return PayoutHistory{}, err
	}
	if err = s.verify(ctx); err != nil {
		return PayoutHistory{}, err
	}
	h := PayoutHistory{Version: Version1, Balance: Balance{GymID: gym, Confirmed: Fraction{"0", "1"}, Available: Fraction{"0", "1"}}, SettlementObservedAtMS: s.state.Report.ObservedAtMS, Items: make([]PayoutItem, 0)}
	for _, d := range s.state.Config.Destinations {
		if d.GymID == gym {
			h.RecipientScript = d.Script
			break
		}
	}
	for _, b := range s.state.Report.Balances {
		if b.GymID == gym {
			h.Balance = b
			break
		}
	}
	for i := len(s.state.Batches) - 1; i >= 0; i-- {
		batch := s.state.Batches[i]
		for output, payment := range batch.Payments {
			if payment.GymID == gym {
				if payment.Script != h.RecipientScript || len(h.Items) >= MaxPayoutItems {
					return PayoutHistory{}, ErrCorrupt
				}
				h.Items = append(h.Items, PayoutItem{batch.Day, batch.CreatedAtMS, batch.TxID, uint32(output), payment.Atoms, batch.State})
			}
		}
	}
	h, err = ValidatePayoutHistory(h, gym)
	if err != nil {
		return PayoutHistory{}, ErrCorrupt
	}
	if err = lease.Check(); err != nil {
		return PayoutHistory{}, err
	}
	if err = ctx.Err(); err != nil {
		return PayoutHistory{}, err
	}
	return h, nil
}
