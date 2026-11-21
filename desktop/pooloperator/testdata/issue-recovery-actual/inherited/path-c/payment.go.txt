package main

import (
	"bytes"
	"context"
	"github.com/taytr-llc/slug/desktop/pooloperator"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/wire/v2"
	"reflect"
	"time"
)

func selectedEconomics(ref actualReference, n int) economicBranch {
	for _, e := range ref.Economics {
		if e.AcceptedRewards == n {
			require(e.InputCount == n && e.Credits == n && e.Batches == 1 && e.OutputCount == 2, "independent branch counts")
			return e
		}
	}
	panic("unmodeled accepted count")
}
func funding(ctx context.Context, n *node, s pooloperator.Setup, receipts []poolshare.Submission) map[wire.OutPoint]bool {
	out := map[wire.OutPoint]bool{}
	for _, r := range receipts {
		b := block(ctx, n, r.WorkHash)
		require(b.Header.Bits == 0x207fffff && len(b.Transactions) > 0, "original actual target/coinbase")
		matches := 0
		for i, v := range b.Transactions[0].TxOut {
			if hx(v.PkScript) == s.Settlement.ChangeScript {
				require(v.Value == 1000000000, "exact fee-free reward")
				o := wire.OutPoint{Hash: b.Transactions[0].TxHash(), Index: uint32(i)}
				require(!out[o], "distinct original funding")
				out[o] = true
				matches++
			}
		}
		require(matches == 1, "one exact operator coinbase output")
	}
	return out
}
func validatePayment(send observedSend, s pooloperator.Setup, inputs map[wire.OutPoint]bool, e economicBranch) map[string]any {
	b := dec(send.Raw)
	require(len(b) > 0 && len(b) <= 400000, "signed raw bound")
	reader := bytes.NewReader(b)
	var tx wire.MsgTx
	check(tx.Deserialize(reader), "actual signed transaction")
	require(reader.Len() == 0 && tx.TxHash().String() == send.TxID && len(tx.TxIn) == e.InputCount && len(tx.TxOut) == e.OutputCount, "exact actual signed shape")
	seen := map[wire.OutPoint]bool{}
	for _, in := range tx.TxIn {
		require(inputs[in.PreviousOutPoint] && !seen[in.PreviousOutPoint] && len(in.Witness) > 0, "only original accepted funding")
		seen[in.PreviousOutPoint] = true
	}
	require(len(seen) == len(inputs) && hx(tx.TxOut[0].PkScript) == s.Settlement.Destinations[0].Script && tx.TxOut[0].Value == e.MinerAtoms && hx(tx.TxOut[1].PkScript) == s.Settlement.ChangeScript && tx.TxOut[1].Value == e.ChangeAtoms && e.GrossAtoms-e.MinerAtoms-e.ChangeAtoms == e.FeeAtoms, "independent exact recipient/change/fee")
	return map[string]any{"txid": send.TxID, "raw": send.Raw, "raw_sha256": hx(digest(b)), "input_count": len(tx.TxIn), "miner_atoms": e.MinerAtoms, "fee_atoms": e.FeeAtoms, "change_atoms": e.ChangeAtoms, "broadcast_at_ms": send.AtMS}
}

type moneyReadRefusal struct {
	Read           string `json:"read"`
	GymIndex       int    `json:"gym_index"`
	AtMS           int64  `json:"at_ms"`
	Code           string `json:"code"`
	OutcomeUnknown bool   `json:"outcome_unknown"`
}
type moneyReadEvidence struct {
	Complete     bool               `json:"complete"`
	DeadlineMS   int64              `json:"deadline_ms"`
	RefusalCount uint64             `json:"refusal_count"`
	Refusals     []moneyReadRefusal `json:"refusals"`
	LastRefusal  *moneyReadRefusal  `json:"last_refusal"`
}

// Only these pure money reads retry definite unavailable. A successful complete
// authenticated response remains mandatory; no mutation or Lookup policy changes.
func (v *moneyReadEvidence) refused(read string, gym int, err error) bool {
	if err == nil {
		return false
	}
	r := moneyReadRefusal{Read: read, GymIndex: gym, AtMS: time.Now().UnixMilli(), Code: "other"}
	retry := false
	if f, ok := err.(*poolservice.ClientError); ok && f != nil {
		r.OutcomeUnknown = f.OutcomeUnknown
		switch f.Code {
		case "busy", "rate_limited", "unavailable":
			r.Code = f.Code
			retry = !f.OutcomeUnknown
		case "outcome_unknown", "unauthorized", "invalid_request", "forbidden", "not_found", "stale_capture", "capacity":
			r.Code = f.Code
		}
	}
	v.RefusalCount++
	v.LastRefusal = &r
	if len(v.Refusals) < 32 {
		v.Refusals = append(v.Refusals, r)
	}
	return retry
}
func waitMoney(ctx context.Context, clients []*poolservice.Client, s pooloperator.Setup, e economicBranch, txid string, after int64, evidencePath string) ([]poolservice.DailyBalanceResponse, []poolsettle.DailyPayoutPage) {
	work, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	deadline, _ := work.Deadline()
	reads := moneyReadEvidence{DeadlineMS: deadline.UnixMilli(), Refusals: []moneyReadRefusal{}}
	defer func() { write(evidencePath, reads) }()
	var balances []poolservice.DailyBalanceResponse
	var pages []poolsettle.DailyPayoutPage
	wait(work, 120*time.Second, func() bool {
		balances = []poolservice.DailyBalanceResponse{}
		pages = []poolsettle.DailyPayoutPage{}
		for i, c := range clients {
			v, err := c.DailyBalance(work)
			if reads.refused("balance", i, err) {
				return false
			}
			check(err, "authenticated daily balance")
			p, err := c.PayoutsPage(work, poolsettle.DailyPayoutRequest{})
			if reads.refused("payouts", i, err) {
				return false
			}
			check(err, "authenticated original page")
			require(v.Version == 2 && v.BindingID == s.Miners.SourceID && v.SettlementConfigID == s.Miners.SettlementConfigID && v.RecipientScript == s.Miners.Enrollments[i].PayeeScript && v.Balance.GymID == s.Miners.Enrollments[i].GymID, "bound whole balance")
			require(p.Version == 2 && p.BindingID == v.BindingID && p.SettlementConfigID == v.SettlementConfigID && p.RecipientScript == v.RecipientScript && p.Balance.GymID == v.Balance.GymID, "bound page money")
			if v.ObservedAtMS < after || p.ObservedAtMS < after || p.HighBatch != 1 || p.BeforeBatch != 2 || !p.Done || p.NextBefore != 0 {
				return false
			}
			want := int64(0)
			if i == 0 {
				want = e.MinerAtoms
			}
			if v.Balance.PaidAtoms != want || p.Balance.PaidAtoms != want || v.Balance.ReservedAtoms != 0 || p.Balance.ReservedAtoms != 0 {
				return false
			}
			zero := poolsettle.Fraction{Numerator: "0", Denominator: "1"}
			require(v.Balance.Confirmed == zero && v.Balance.Available == zero && p.Balance.Confirmed == zero && p.Balance.Available == zero, "complete confirmed payout leaves no miner carry")
			if i == 0 {
				if len(p.Items) != 1 || p.Items[0].Payment.RecordedState != "confirmed" {
					return false
				}
				v := p.Items[0]
				require(v.BatchSequence == 1 && v.Payment.TxID == txid && v.Payment.OutputIndex == 0 && v.Payment.Atoms == e.MinerAtoms, "same original payment page")
			} else {
				require(len(p.Items) == 0, "other gym whole empty page")
			}
			balances = append(balances, v)
			pages = append(pages, p)
		}
		return true
	})
	require(work.Err() == nil, "money reads within original bound")
	reads.Complete = true
	return balances, pages
}
func verifyFinal(a attachmentOperatorAudit, s pooloperator.Setup, ev rowEvidence, e economicBranch, send observedSend) {
	require(a.Accepted == uint64(len(ev.Accepted)) && a.Submissions >= a.Accepted && a.Submissions <= 2 && a.Works > 0 && a.Works <= s.Pool.MaxWorks && a.AuthorityJobs >= a.Works, "all physical retained rows/jobs")
	require(a.Births == uint64(len(a.Admissions)), "all original births audited")
	acknowledged := 0
	for _, saved := range a.Admissions {
		found := false
		for _, row := range ev.Rows {
			f := row.Fighter
			if f == nil || f.Authorization == nil || saved.Job != f.Job || saved.Authorization != *f.Authorization || !reflect.DeepEqual(saved.Proof, f.Candidate.Proof) {
				continue
			}
			require(!found, "unique original admission evidence")
			found = true
			if f.Receipt != nil {
				require(reflect.DeepEqual(saved, f.Receipt.Record), "exact original acknowledged birth")
				acknowledged++
			} else {
				require(f.AdmissionState == "pending", "lost acknowledgement retains original pending evidence")
			}
		}
		require(found, "no unexplained issuer admission")
	}
	require(acknowledged == ev.AcceptedFighters, "every acknowledged fighter retained by original issuer")
	require(len(a.Entries) == len(ev.Accepted) && len(a.Rounds) == len(ev.Accepted) && len(a.Credits) == len(ev.Accepted), "exact original source rounds credits")
	for i, r := range ev.Accepted {
		require(sourceReceipt(a.Entries[i].Share) == sourceReceipt(r), "complete exact accepted prefix")
	}
	for _, r := range a.Rounds {
		require(r.Confirmation != nil && r.Round.RewardAtoms == 1000000000 && len(r.Round.ShareKeys) == 1 && len(r.Round.Allocations) == 1 && r.Round.Allocations[0].GymID == s.Miners.Enrollments[0].GymID, "one immutable single-gym winner per round")
	}
	require(a.Header.Batches == 1 && a.Header.Credits == uint64(e.Credits) && a.Header.PendingBatch == 0 && a.Delivery == "confirmed" && a.Body.Raw == send.Raw && a.Body.TxID == send.TxID && a.Body.FeeAtoms == e.FeeAtoms && len(a.Body.Payments) == 1 && a.Body.Payments[0].Atoms == e.MinerAtoms && a.Body.Payments[0].Script == s.Miners.Enrollments[0].PayeeScript && a.Body.Change != nil && a.Body.Change.Atoms == e.ChangeAtoms && a.Body.Change.ClaimCount == uint64(e.InputCount), "one exact confirmed signed body and carried origins")
	require(len(a.Report.Balances) == 2 && a.ReadOnlyUnchanged, "complete original report and readonly audit")
	for i, b := range a.Report.Balances {
		require(b.GymID == s.Settlement.Destinations[i].GymID && b.ReservedAtoms == 0, "configured full balances")
		want := int64(0)
		if i == 0 {
			want = e.MinerAtoms
		}
		require(b.PaidAtoms == want && b.Confirmed == (poolsettle.Fraction{Numerator: "0", Denominator: "1"}), "exact original paid amount")
	}
	for _, c := range a.Credits {
		require(c.Coin.Coinbase && c.Coin.Atoms == 1000000000 && c.Coin.Script == s.Settlement.ChangeScript && len(c.Coin.Claims) == 2, "original credit provenance")
		expected := map[string]string{s.Miners.Enrollments[0].GymID: "990000000", "": "10000000"}
		for _, claim := range c.Coin.Claims {
			require(claim.RoundHash == c.RoundHash && claim.Amount.Denominator == "1" && expected[claim.GymID] == claim.Amount.Numerator, "independent99/1 claim")
			delete(expected, claim.GymID)
		}
		require(len(expected) == 0, "complete99/1 claims")
	}
}
