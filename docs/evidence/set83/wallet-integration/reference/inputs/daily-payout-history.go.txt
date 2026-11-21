// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettle

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"math/big"
	"time"

	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/internal/mininglease"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/wire/v2"
)

const MaxDailyPayoutSlots = 8
const MaxDailyPayoutBatches uint64 = 32767

var ErrStaleCapture = errors.New("pool payout history capture changed")

// DailyBalance uses decimal strings at the wire boundary for cumulative atom
// totals. Per-transaction amounts remain bounded numeric values in PayoutItem.
type DailyBalance struct {
	GymID         string   `json:"gym_id"`
	Confirmed     Fraction `json:"confirmed"`
	Available     Fraction `json:"available"`
	ReservedAtoms int64    `json:"reserved_atoms,string"`
	PaidAtoms     int64    `json:"paid_atoms,string"`
	Reason        string   `json:"reason"`
}

func DailyBalanceFrom(b Balance) DailyBalance {
	return DailyBalance{b.GymID, b.Confirmed, b.Available, b.ReservedAtoms, b.PaidAtoms, b.Reason}
}

func (b DailyBalance) Balance() Balance {
	return Balance{b.GymID, b.Confirmed, b.Available, b.ReservedAtoms, b.PaidAtoms, b.Reason}
}

// ValidateDailyBalance checks supplied shape, not authority or freshness.
func ValidateDailyBalance(b DailyBalance, expectedGym string, observedMS int64) error {
	if _, ok := hexBytes(expectedGym, 32); !ok || b.GymID != expectedGym || b.ReservedAtoms < 0 || b.PaidAtoms < 0 || !knownReason(b.Reason) || observedMS < 0 || observedMS > maxPayoutTimeMS {
		return ErrInvalid
	}
	confirmed, err := rat(b.Confirmed)
	if err != nil {
		return ErrInvalid
	}
	available, err := rat(b.Available)
	if err != nil {
		return ErrInvalid
	}
	unreserved := new(big.Rat).Sub(confirmed, new(big.Rat).SetInt64(b.ReservedAtoms))
	if unreserved.Sign() < 0 || available.Cmp(unreserved) > 0 || confirmed.Sign() == 0 && b.Reason != "" {
		return ErrInvalid
	}
	if observedMS == 0 && (confirmed.Sign() != 0 || available.Sign() != 0 || b.ReservedAtoms != 0 || b.PaidAtoms != 0 || b.Reason != "") {
		return ErrInvalid
	}
	raw, err := json.Marshal(b)
	if err != nil || len(raw) > MaxPayoutBalanceSize {
		return ErrInvalid
	}
	return nil
}

type DailyPayoutRequest struct {
	Revision    uint64 `json:"revision,string"`
	HighBatch   uint64 `json:"high_batch,string"`
	BeforeBatch uint64 `json:"before_batch,string"`
}

// ValidateDailyPayoutRequest distinguishes fresh capture from continuation.
// maxBatches is the selected store cap, or the global cap at a remote client.
func ValidateDailyPayoutRequest(q DailyPayoutRequest, maxBatches uint64) error {
	if maxBatches == 0 || maxBatches > MaxDailyPayoutBatches {
		return ErrInvalid
	}
	if q == (DailyPayoutRequest{}) {
		return nil
	}
	if q.Revision == 0 || q.HighBatch == 0 || q.HighBatch > maxBatches || q.BeforeBatch == 0 || q.BeforeBatch > q.HighBatch+1 {
		return ErrInvalid
	}
	return nil
}

type DailyPayoutItem struct {
	BatchSequence uint64     `json:"batch_sequence,string"`
	Payment       PayoutItem `json:"payment"`
}

// DailyPayoutPage is a bounded projection of one fully audited ledger capture.
// Its identifiers must also be compared to the client's independently pinned
// configuration. A page alone does not certify that earlier pages were read.
type DailyPayoutPage struct {
	Version            uint32            `json:"version"`
	BindingID          string            `json:"binding_id"`
	SettlementConfigID string            `json:"settlement_config_id"`
	Revision           uint64            `json:"revision,string"`
	HighBatch          uint64            `json:"high_batch,string"`
	BeforeBatch        uint64            `json:"before_batch,string"`
	NextBefore         uint64            `json:"next_before,string"`
	Done               bool              `json:"done"`
	RecipientScript    string            `json:"recipient_script"`
	Balance            DailyBalance      `json:"balance"`
	ObservedAtMS       int64             `json:"observed_at_ms"`
	Items              []DailyPayoutItem `json:"items"`
}

func dailyPayoutWindow(before uint64) (low uint64, done bool) {
	if before <= MaxDailyPayoutSlots+1 {
		return 1, true
	}
	return before - MaxDailyPayoutSlots, false
}

// ValidateDailyPayoutPage validates and owns supplied shape only. It performs
// no I/O and does not turn a response into ledger or session authority.
func ValidateDailyPayoutPage(p DailyPayoutPage, expectedGym string) (DailyPayoutPage, error) {
	bad := func() (DailyPayoutPage, error) { return DailyPayoutPage{}, ErrInvalid }
	if p.Version != Version2 || p.Revision == 0 || p.HighBatch > MaxDailyPayoutBatches || p.BeforeBatch == 0 || p.BeforeBatch > p.HighBatch+1 || p.Items == nil || len(p.Items) > MaxDailyPayoutSlots {
		return bad()
	}
	for _, id := range []string{p.BindingID, p.SettlementConfigID} {
		if _, ok := hexBytes(id, 32); !ok {
			return bad()
		}
	}
	if _, ok := nativeScript(p.RecipientScript); !ok || ValidateDailyBalance(p.Balance, expectedGym, p.ObservedAtMS) != nil {
		return bad()
	}
	low, done := dailyPayoutWindow(p.BeforeBatch)
	next := low
	if done {
		next = 0
	}
	if p.Done != done || p.NextBefore != next || p.ObservedAtMS == 0 && (p.HighBatch != 0 || len(p.Items) != 0) {
		return bad()
	}
	last := p.BeforeBatch
	seen := make(map[string]bool, len(p.Items))
	var paid, reserved int64
	for i, row := range p.Items {
		item := row.Payment
		if row.BatchSequence < low || row.BatchSequence >= last || item.CreatedAtMS < 1 || item.CreatedAtMS > p.ObservedAtMS || item.OutputIndex > 255 || item.Atoms < MinimumAtoms || item.Atoms > MaxMoney || len(item.Day) != 10 {
			return bad()
		}
		if i > 0 && (item.Day >= p.Items[i-1].Payment.Day || item.CreatedAtMS >= p.Items[i-1].Payment.CreatedAtMS) {
			return bad()
		}
		if _, ok := hexBytes(item.TxID, 32); !ok || seen[item.TxID] || time.UnixMilli(item.CreatedAtMS).UTC().Format("2006-01-02") != item.Day {
			return bad()
		}
		last, seen[item.TxID] = row.BatchSequence, true
		switch item.RecordedState {
		case "confirmed":
			if paid > math.MaxInt64-item.Atoms {
				return bad()
			}
			paid += item.Atoms
		case "prepared", "broadcast":
			if row.BatchSequence != p.HighBatch || reserved > math.MaxInt64-item.Atoms {
				return bad()
			}
			reserved += item.Atoms
		default:
			return bad()
		}
		raw, err := json.Marshal(item)
		if err != nil || len(raw) > MaxPayoutItemSize {
			return bad()
		}
	}
	if paid > p.Balance.PaidAtoms || reserved > p.Balance.ReservedAtoms {
		return bad()
	}
	raw, err := json.Marshal(p)
	if err != nil || len(raw)+1 > MaxPayoutHistorySize {
		return bad()
	}
	items := make([]DailyPayoutItem, len(p.Items))
	copy(items, p.Items)
	p.Items = items
	return p, nil
}

// MinerDailyPayoutPage uses one snapshot for the full integrity/source audit
// and the bounded projection. It never observes a node, advances the ledger
// clock, writes a revision, or holds an account lease over ledger/source work.
func (s *Store) MinerDailyPayoutPage(ctx context.Context, sessions prospectservice.SessionAuthorizer, token string, q DailyPayoutRequest) (DailyPayoutPage, error) {
	if wire.TestPoW {
		return DailyPayoutPage{}, ErrProfile
	}
	if s == nil || s.daily == nil {
		return DailyPayoutPage{}, ErrConfig
	}
	if !nonnil(sessions) || ValidateDailyPayoutRequest(q, uint64(s.state.Config.MaxBatches)) != nil {
		return DailyPayoutPage{}, ErrInvalid
	}
	end, err := s.begin(ctx)
	if err != nil {
		return DailyPayoutPage{}, err
	}
	defer end()
	if s.source == nil {
		return DailyPayoutPage{}, ErrSource
	}
	lease, err := mininglease.Capture(ctx, sessions, token)
	if err != nil {
		return DailyPayoutPage{}, err
	}
	policy := s.state.Source.Policy
	gym, err := gymauth.GymID(prospect.Context{Version: policy.Version, Network: policy.Network, Genesis: policy.Genesis, DNAVersion: policy.DNAVersion}, lease.PublicKey())
	if err != nil {
		return DailyPayoutPage{}, err
	}
	if err = s.checkDailyMarker(); err != nil {
		return DailyPayoutPage{}, err
	}
	snapshot, err := s.db.GetSnapshot()
	if err != nil {
		return DailyPayoutPage{}, ErrCorrupt
	}
	defer snapshot.Release()
	v, err := s.auditDailyRows(ctx, snapshot)
	if err != nil {
		return DailyPayoutPage{}, err
	}
	if _, err = s.dailySource(ctx, snapshot, v.head, false); err != nil {
		return DailyPayoutPage{}, err
	}
	check := func() error {
		if e := lease.Check(); e != nil {
			return e
		}
		return ctx.Err()
	}
	if q.Revision != 0 && (q.Revision != v.head.Revision || q.HighBatch != v.head.Batches) {
		if err = check(); err != nil {
			return DailyPayoutPage{}, err
		}
		return DailyPayoutPage{}, ErrStaleCapture
	}
	p := DailyPayoutPage{Version: Version2, BindingID: s.daily.marker.SourceID, SettlementConfigID: s.daily.marker.ConfigID, Revision: v.head.Revision, HighBatch: v.head.Batches, BeforeBatch: q.BeforeBatch, ObservedAtMS: v.report.ObservedAtMS, Items: make([]DailyPayoutItem, 0), Balance: DailyBalance{GymID: gym, Confirmed: Fraction{"0", "1"}, Available: Fraction{"0", "1"}}}
	if q.Revision == 0 {
		p.BeforeBatch = p.HighBatch + 1
	}
	for _, d := range s.state.Config.Destinations {
		if d.GymID == gym {
			p.RecipientScript = d.Script
			break
		}
	}
	if p.RecipientScript == "" {
		return DailyPayoutPage{}, ErrInvalid
	}
	for _, b := range v.report.Balances {
		if b.GymID == gym {
			p.Balance = DailyBalanceFrom(b)
			break
		}
	}
	low, done := dailyPayoutWindow(p.BeforeBatch)
	p.Done, p.NextBefore = done, low
	if done {
		p.NextBefore = 0
	}
	reader := dailyReader{ctx: ctx, db: snapshot, keys: make(map[string]bool)}
	for sequence := p.BeforeBatch - 1; sequence >= low; sequence-- {
		var body dailyBatchBody
		var delivery dailyDelivery
		if err = reader.get(dailyNumber("b/", sequence), dailyBatchLimit, &body); err != nil {
			return DailyPayoutPage{}, err
		}
		if err = reader.get(dailyNumber("bs/", sequence), dailyReferenceLimit, &delivery); err != nil {
			return DailyPayoutPage{}, err
		}
		for output, payment := range body.Payments {
			if payment.GymID != gym {
				continue
			}
			if payment.Script != p.RecipientScript {
				return DailyPayoutPage{}, ErrCorrupt
			}
			p.Items = append(p.Items, DailyPayoutItem{sequence, PayoutItem{body.Day, body.CreatedAtMS, body.TxID, uint32(output), payment.Atoms, delivery.State}})
		}
	}
	p, err = ValidateDailyPayoutPage(p, gym)
	if err != nil {
		return DailyPayoutPage{}, ErrCorrupt
	}
	if err = check(); err != nil {
		return DailyPayoutPage{}, err
	}
	return p, nil
}
