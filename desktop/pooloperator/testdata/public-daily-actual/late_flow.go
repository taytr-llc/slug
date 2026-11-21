package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/taytr-llc/slug/desktop/pooloperator"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/poolsettlenode"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectwork"
	"github.com/taytr-llc/slug/wire/v2"
	"path/filepath"
	"reflect"
	"time"
)

// Startup readiness is read-only and belongs to the same original runtime.
// Its selected maintenance interval is 30s, so readiness must allow a retry
// before the distinct 20s Issue operation begins. Both use the original 900s ctx.
func lateWaitIssueReady(ctx context.Context, r *runningOperator) {
	ready, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	for {
		status := r.status(ready)
		if status.Capacity.Mode == "enabled" && status.Public.Ready {
			return
		}
		check(pauseActual(ready, time.Second), "original read-only startup readiness deadline")
	}
}

func lateIssue(ctx context.Context, c *poolservice.Client, r *runningOperator) poolshare.MiningWork {
	lateWaitIssueReady(ctx, r)
	work, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	for {
		r.checkRunning()
		v, e := c.Issue(work)
		if e == nil {
			return v
		}
		failure := lateFailure(e)
		if failure.OutcomeUnknown || (failure.Code != "busy" && failure.Code != "rate_limited") {
			panic(fmt.Sprintf("Issue refused code=%s outcome_unknown=%t", failure.Code, failure.OutcomeUnknown))
		}
		check(pauseActual(work, time.Second), "definite Issue retry deadline")
	}
}

type lateServiceFailure struct {
	Code           string `json:"code"`
	OutcomeUnknown bool   `json:"outcome_unknown"`
}

// Closed diagnostic vocabulary only; never echo arbitrary error text or inputs.
func lateFailure(err error) lateServiceFailure {
	if err == nil {
		return lateServiceFailure{Code: "success"}
	}
	var ce *poolservice.ClientError
	if !errors.As(err, &ce) {
		return lateServiceFailure{Code: "non_client_error", OutcomeUnknown: true}
	}
	code := "unrecognized"
	switch ce.Code {
	case "invalid_request", "unauthorized", "forbidden", "not_found", "expired", "rejected", "too_large", "rate_limited", "busy", "capacity", "unavailable", "outcome_unknown":
		code = ce.Code
	}
	return lateServiceFailure{Code: code, OutcomeUnknown: ce.OutcomeUnknown}
}

func lateRequireFailure(err error, code string, unknown bool, label string) lateServiceFailure {
	got := lateFailure(err)
	require(got.Code == code && got.OutcomeUnknown == unknown,
		fmt.Sprintf("%s: code=%s outcome_unknown=%t expected_code=%s expected_unknown=%t", label, got.Code, got.OutcomeUnknown, code, unknown))
	return got
}

// Retry only a definite pre-admission refusal of this exact original identity.
// A successful receipt or any uncertainty/other error is terminal for this call.
func lateSubmit(ctx context.Context, c *poolservice.Client, original poolshare.MiningWork, nonce uint32) (poolshare.Submission, error) {
	work, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	for {
		receipt, err := c.Submit(work, original, nonce)
		failure := lateFailure(err)
		if err == nil || failure.OutcomeUnknown || (failure.Code != "busy" && failure.Code != "rate_limited") {
			return receipt, err
		}
		if err = pauseActual(work, time.Second); err != nil {
			return poolshare.Submission{}, err
		}
	}
}

// Lookup uses the same bounded pre-admission policy and original identity;
// callers still compare the complete saved receipt, never infer a replacement.
func lateLookup(ctx context.Context, c *poolservice.Client, original poolshare.MiningWork, nonce uint32) (poolshare.Submission, error) {
	work, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	for {
		receipt, err := c.Lookup(work, original, nonce)
		failure := lateFailure(err)
		if err == nil || failure.OutcomeUnknown || (failure.Code != "busy" && failure.Code != "rate_limited") {
			return receipt, err
		}
		if err = pauseActual(work, time.Second); err != nil {
			return poolshare.Submission{}, err
		}
	}
}

func lateIssueClosed(ctx context.Context, c *poolservice.Client) lateServiceFailure {
	work, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	last := lateServiceFailure{Code: "not_called"}
	for {
		// Pace before the first request too: the last original Issue may have
		// consumed its allowance. Only definite pre-admission failures retry.
		if pauseActual(work, time.Second) != nil {
			panic(fmt.Sprintf("closed Issue deadline: code=%s outcome_unknown=%t", last.Code, last.OutcomeUnknown))
		}
		_, err := c.Issue(work)
		last = lateFailure(err)
		if !last.OutcomeUnknown && (last.Code == "busy" || last.Code == "rate_limited") {
			continue
		}
		// Issue conservatively maps every underlying non-Busy error (including
		// capacity) to unknown. Never retry that reply: close and audit exact
		// original issuer Jobs and pool Works/rows before claiming no issuance.
		return lateRequireFailure(err, "outcome_unknown", true, "ordinary issuance is closed")
	}
}

// A timer wake is not proof that a wall-clock target has passed. The original
// scenario context bounds every wait; neither target nor deadline is replaced.
func lateWaitWall(ctx context.Context, targetMS int64, label string) int64 {
	for {
		observedMS := time.Now().UnixMilli()
		require(ctx.Err() == nil, fmt.Sprintf("%s canceled: observed_wall_ms=%d target_wall_ms=%d", label, observedMS, targetMS))
		if observedMS >= targetMS {
			return observedMS
		}
		remaining := time.Duration(targetMS-observedMS) * time.Millisecond
		if remaining < time.Millisecond {
			remaining = time.Millisecond
		}
		if pauseActual(ctx, remaining) != nil {
			panic(fmt.Sprintf("%s canceled: observed_wall_ms=%d target_wall_ms=%d", label, time.Now().UnixMilli(), targetMS))
		}
	}
}

func lateCheck(ctx context.Context, p *prospectwork.ManifestWork, nonce uint32) prospectwork.ShareCheck {
	for {
		v, e := p.CheckNonce(ctx, nonce)
		if e == nil {
			return v
		}
		require(e == prospectwork.ErrShareBusy, "normal share check refused")
		check(pauseActual(ctx, 10*time.Millisecond), "hash admission deadline")
	}
}

func dailyFind(ctx context.Context, s pooloperator.Setup, c publicFixtureMinerConfig, w poolshare.MiningWork, wantWinner, wantOrdinary bool) (winner, ordinary prospectwork.ShareCheck) {
	prepared, e := poolservice.PrepareDailyWork(s.Pool, c.GymPublicKey, w)
	check(e, "exact original daily candidate")
	hashing, cancel := context.WithTimeout(ctx, 4*time.Minute)
	defer cancel()
	for nonce := uint32(0); nonce < 64; nonce++ {
		v := lateCheck(hashing, prepared, nonce)
		if v.Block && !winner.Block {
			winner = v
		}
		if v.Pool && !v.Block && ordinary.Candidate == nil {
			ordinary = v
		}
		if (!wantWinner || winner.Block) && (!wantOrdinary || ordinary.Candidate != nil) {
			return
		}
	}
	panic("bounded normal fixed-job nonce search exhausted")
}
func dailyPublish(ctx context.Context, s pooloperator.Setup, a, b *node, w prospectwork.ShareCheck, height uint32, reward int64, expectedA string) wire.OutPoint {
	require(w.Block && w.Candidate != nil, "actual winner candidate")
	var reply any
	a.rpc.must(ctx, "submitblock", []any{w.Candidate.BlockHex}, &reply)
	require(reply == nil && sameTip(ctx, a, b) == w.WorkHash, "actual canonical original block")
	var h string
	a.rpc.must(ctx, "getblockhash", []any{height}, &h)
	require(h == w.WorkHash, "canonical original height")
	blk := block(ctx, a, h)
	var encoded bytes.Buffer
	check(blk.Serialize(&encoded), "canonical block bytes")
	require(hx(encoded.Bytes()) == w.Candidate.BlockHex, "node kept exact candidate")
	if expectedA == "" {
		require(len(blk.Transactions) == 1, "A includes no unrelated transaction")
	} else {
		require(len(blk.Transactions) == 2 && hx(raw(blk.Transactions[1])) == expectedA, "B contains exact original A signed transaction")
	}
	found := false
	var out wire.OutPoint
	for i, o := range blk.Transactions[0].TxOut {
		if hx(o.PkScript) == s.WalletGuard.FundingScript {
			require(!found && o.Value == reward, "independent exact original pool reward")
			out = wire.OutPoint{Hash: blk.Transactions[0].TxHash(), Index: uint32(i)}
			found = true
		}
	}
	require(found, "pool original funding output")
	return out
}
func dailyBalanceMatches(got poolsettle.DailyBalance, want expectedBalance, reason bool) bool {
	return got.Confirmed == want.Confirmed && got.Available == want.Available && got.ReservedAtoms == want.Reserved && got.PaidAtoms == want.Paid && (!reason || got.Reason == want.Reason)
}
func dailyWaitBalances(ctx context.Context, evidence *dailyBalanceWaitEvidence, clients []*poolservice.Client, e actualEconomics, name string, after int64, d time.Duration, reasons bool) []poolservice.DailyBalanceResponse {
	record, save := evidence.begin(ctx, name, after, d, reasons)
	defer save()
	want, ok := e.Reports[name]
	require(ok && len(want.Balances) == 2, "independent report expectation")
	out := make([]poolservice.DailyBalanceResponse, 2)
	wait(ctx, d, func() bool {
		for i, c := range clients {
			v, err := c.DailyBalance(ctx)
			record.sample(i, v, err)
			if err != nil || v.ObservedAtMS < after || !dailyBalanceMatches(v.Balance, want.Balances[i], reasons) {
				return false
			}
			out[i] = v
		}
		return true
	})
	record.Terminal = "success"
	return out
}
func dailyPages(ctx context.Context, clients []*poolservice.Client, s pooloperator.Setup, txid string, e actualEconomics, name string) []poolsettle.DailyPayoutPage {
	out := make([]poolsettle.DailyPayoutPage, 2)
	wait(ctx, 30*time.Second, func() bool {
		for i, c := range clients {
			v, err := c.PayoutsPage(ctx, poolsettle.DailyPayoutRequest{})
			if err != nil {
				return false
			}
			if v.Version != 2 || v.BindingID != s.WalletGuard.SourceBindingID || v.SettlementConfigID != s.WalletGuard.SettlementConfigID || v.RecipientScript != s.Miners.Enrollments[i].PayeeScript || v.Balance.GymID != s.Miners.Enrollments[i].GymID || v.HighBatch != 1 || v.BeforeBatch != 2 || !v.Done || v.NextBefore != 0 || v.Revision == 0 || !dailyBalanceMatches(v.Balance, e.Reports[name].Balances[i], true) {
				return false
			}
			if i == 0 {
				if len(v.Items) != 1 || v.Items[0].BatchSequence != 1 || v.Items[0].Payment.TxID != txid || v.Items[0].Payment.Atoms != e.APayment || v.Items[0].Payment.RecordedState != "confirmed" {
					return false
				}
			} else if len(v.Items) != 0 {
				return false
			}
			out[i] = v
		}
		return true
	})
	return out
}
func dailyWaitRounds(ctx context.Context, r *runningOperator, through, rounds uint64) pooloperator.Status {
	var out pooloperator.Status
	wait(ctx, 90*time.Second, func() bool {
		v, e := r.runtime.Status(ctx)
		if e != nil || v.RoundCode != "" || v.Rounds.Through != through || v.Rounds.Rounds != rounds {
			return false
		}
		out = v
		return true
	})
	require(out.Capacity.DailyReservation != nil && out.Capacity.DailyReservation.OrdinaryMaximum == 2, "original derived daily reservation")
	return out
}
func dailyAssertWinner(a lateAudit, s pooloperator.Setup, w poolshare.MiningWork, receipt poolshare.Submission, e actualEconomics, locked bool) {
	require(a.Submissions == 3 && a.Accepted == 3 && a.Works == 3 && a.AuthorityJobs == 3 && a.Births == 0 && len(a.Entries) == 3 && len(a.Rounds) == 2, "exact retained source/round/issuer counts, no fighter birth")
	// Accounting excludes mutable block observation fields by its original contract.
	// The full saved receipt remains compared separately across Submit and Lookup.
	projected := receipt
	projected.BlockState = ""
	projected.BlockObservedAtMS = 0
	require(reflect.DeepEqual(a.Entries[2].Share, projected) && a.Entries[2].Job.ID == w.Job.ID, "exact original accounting receipt projection and job")
	r := a.Rounds[1]
	require(r.Round.FirstAccepted == 2 && r.Round.LastAccepted == 3 && len(r.Round.ShareKeys) == 2 && r.Round.ShareKeys[1] == receipt.DuplicateKey && r.Round.BlockHash == receipt.WorkHash && r.Round.BlockHeight == 103 && r.Round.RewardAtoms == e.BReward, "unchanged round window2..3")
	require(len(r.Round.Allocations) == 2, "two equal-share gyms")
	for i, allocation := range r.Round.Allocations {
		require(allocation.GymID == s.Miners.Enrollments[i].GymID && allocation.Shares == 1 && allocation.Numerator == e.BGross[i].Numerator && allocation.Denominator == e.BGross[i].Denominator, "independent B gross allocation")
	}
	require((r.Confirmation != nil) == locked, "exact B confirmation lock boundary")
}
func lateStartCLI(ctx context.Context, binary, root, private, name string, s pooloperator.Setup, children *[]*childProcess) *childProcess {
	path := filepath.Join(root, name+".log")
	c := startChild(binary, name, path, cliArgs("serve", root, private)...)
	*children = append(*children, c)
	wait(ctx, 60*time.Second, func() bool { return cliListening(path, s.Ingress.Origin) })
	return c
}

func runDailyLate(ctx context.Context, root, private, binary string, s pooloperator.Setup, a, b *node, relay *rpcRelay, running **runningOperator, children *[]*childProcess, configs []publicFixtureMinerConfig, clients []*poolservice.Client, rpcConfig prospectnode.RPCConfig, first lateAudit, e actualEconomics, day string, balanceWaits *dailyBalanceWaitEvidence) map[string]any {
	phase = "original B/C jobs and true ordinary two-row intake"
	*running = constructRuntime(ctx, root, private, s, rpcConfig, false, "")
	(*running).run(ctx, s.Ingress.Origin)
	workB := lateIssue(ctx, clients[1], *running)
	workC := lateIssue(ctx, clients[0], *running)
	require(workB.Job.ID != workC.Job.ID && workB.Job.ID != first.Entries[0].Job.ID && workC.Job.ID != first.Entries[0].Job.ID && workB.Job.Context.ParentHeight == 102 && workC.Job.Context.ParentHeight == 102, "three distinct original jobs A/B/C")
	require(workB.Job.ExpiresAtMS-workB.Job.IssuedAtMS == 300000, "original real300s B expiry")
	_, ordinaryC := dailyFind(ctx, s, configs[0], workC, false, true)
	filler, err := lateSubmit(ctx, clients[0], workC, ordinaryC.Nonce)
	check(err, "C timely original ordinary row")
	require(filler.Sequence == 2 && filler.AcceptedSequence == 2 && !filler.Block && filler.Acceptance == "current-parent", "C is only ordinary filler")
	full := dailyWaitRounds(ctx, *running, 2, 1)
	wait(ctx, 35*time.Second, func() bool {
		full = runtimeStatus(ctx, (*running).runtime)
		return full.Capacity.Mode == "draining" && full.Capacity.Submissions == 2
	})
	require(full.Capacity.DailyReservation != nil && full.Capacity.DailyReservation.OrdinaryMaximum == 2 && !full.Capacity.OverBound, "actual derived intake exhaustion")
	issueDenial := lateIssueClosed(ctx, clients[0])
	check((*running).stop(), "join full original DAILY source")
	*running = nil
	filled := lateClosedAudit(ctx, root, s)
	lateSameBatch(first, filled)
	require(filled.Submissions == 2 && filled.Accepted == 2 && filled.Works == 3 && filled.AuthorityJobs == 3 && filled.Births == 0 && len(filled.Rounds) == 1 && len(filled.Credits) == 1 && filled.Header.PendingBatch == 1, "full intake with B wholly unretained")
	for _, entry := range filled.Entries {
		require(entry.Job.ID != workB.Job.ID, "no B receipt before expiry")
	}
	write(filepath.Join(root, "04-full-intake.json"), map[string]any{"audit": filled, "status": full, "work_b": workB, "work_c": workC, "filler": filler, "issue_denial": issueDenial})
	phase = "actual B expiry and noncanonical refusal"
	expiryObserved := lateWaitWall(ctx, workB.Job.ExpiresAtMS, "original B real wall expiry")
	sameDay(day)
	winner, ordinary := dailyFind(ctx, s, configs[1], workB, true, true)
	winnerObserved := time.Now().UnixMilli()
	require(winnerObserved >= workB.Job.ExpiresAtMS, "winner searched after actual expiry")
	*running = constructRuntime(ctx, root, private, s, rpcConfig, false, "")
	(*running).run(ctx, s.Ingress.Origin)
	_, err = lateSubmit(ctx, clients[1], workB, ordinary.Nonce)
	ordinaryDenial := lateRequireFailure(err, "capacity", false, "ordinary expired submission remains refused")
	_, err = lateSubmit(ctx, clients[1], workB, winner.Nonce)
	noncanonicalDenial := lateRequireFailure(err, "outcome_unknown", true, "unseen noncanonical winner is terminal unknown")
	check((*running).stop(), "join noncanonical refusal owner")
	*running = nil
	absent := lateClosedAudit(ctx, root, s)
	lateSameBatch(first, absent)
	require(absent.Submissions == 2 && absent.Accepted == 2 && absent.Works == 3 && absent.AuthorityJobs == 3 && reflect.DeepEqual(absent.Entries, filled.Entries) && len(absent.Credits) == 1 && absent.Header.PendingBatch == 1, "terminal unknown audited physically absent")
	var h uint32
	a.rpc.must(ctx, "getblockcount", nil, &h)
	require(h == 102, "refused candidates did not publish")
	write(filepath.Join(root, "05-expired-absent.json"), map[string]any{"audit": absent, "job": workB.Job, "expiry_observed_at_ms": expiryObserved, "winner_observed_at_ms": winnerObserved, "ordinary_refusal": ordinaryDenial, "noncanonical_refusal": noncanonicalDenial})
	phase = "actual canonical expired B and reserved-slot acceptance"
	coinB := dailyPublish(ctx, s, a, b, winner, 103, e.BReward, first.Body.Raw)
	require(coinB.Hash.String() != first.Credits[0].Coin.TxID, "B new original funding family")
	*running = constructRuntime(ctx, root, private, s, rpcConfig, false, "")
	(*running).run(ctx, s.Ingress.Origin)
	receipt, err := lateSubmit(ctx, clients[1], workB, winner.Nonce)
	check(err, "unseen canonical expired B accepted in reserved slot")
	require(receipt.Sequence == 3 && receipt.AcceptedSequence == 3 && receipt.Acceptance == "canonical-late-winner" && receipt.AcceptedAtMS >= workB.Job.ExpiresAtMS && receipt.WorkHash == winner.WorkHash && receipt.BlockState == "canonical-observed", "exact late third row")
	dailyWaitRounds(ctx, *running, 3, 2)
	check((*running).stop(), "join original parent with pending A and late B")
	*running = nil
	accepted := lateClosedAudit(ctx, root, s)
	lateSameBatch(first, accepted)
	dailyAssertWinner(accepted, s, workB, receipt, e, false)
	require(accepted.Header.PendingBatch == 1 && accepted.Delivery == "prepared" && len(accepted.Credits) == 1 && accepted.Report.Balances[0].ReservedAtoms == e.APayment, "A still reserved before real process restart")
	relay.mu.Lock()
	refusals := relay.observationRefusals
	relay.observeHold = false
	relay.mu.Unlock()
	require(refusals > 0, "explicit original A observation-only fault exercised")
	write(filepath.Join(root, "06-B-pending.json"), map[string]any{"audit": accepted, "receipt": receipt, "work": workB, "proof": winner, "coin": coinB, "height": 103, "expiry_observed_at_ms": expiryObserved, "winner_observed_at_ms": winnerObserved, "observation_fault_refusals": refusals, "observation_fault_released_after_parent_join": true})
	phase = "fresh production DAILY process exact retry and A recovery"
	cli := lateStartCLI(ctx, binary, root, private, "slugpoold-daily-b59", s, children)
	duplicate, err := lateSubmit(ctx, clients[1], workB, winner.Nonce)
	check(err, "fresh production CLI exact B retry")
	require(reflect.DeepEqual(receipt, duplicate), "entire original receipt across process restart")
	lookup, err := lateLookup(ctx, clients[1], workB, winner.Nonce)
	check(err, "fresh process B lookup")
	require(reflect.DeepEqual(receipt, lookup), "exact original lookup")
	// Work is read-only; known pre-admission contention can be retried, never Issue.
	var reopenedWork poolshare.MiningWork
	wait(ctx, 20*time.Second, func() bool {
		v, x := clients[1].Work(ctx, workB.Job.ID)
		if x != nil {
			return false
		}
		reopenedWork = v
		return true
	})
	require(reflect.DeepEqual(reopenedWork, workB), "fresh process retained original work/template")
	recovered := dailyWaitBalances(ctx, balanceWaits, clients, e, "a_recovered_before_b_credit", 0, 90*time.Second, true)
	require(len(relay.sends()) == 1, "original A recovered by observation, never resent")
	phase = "B59 original process observation then closed audit"
	for height := uint32(103); height < 162; height++ {
		generate(ctx, a, b)
	}
	observed59 := time.Now().UnixMilli()
	check(pauseActual(ctx, 65*time.Second), "two original30s maintenance intervals at B59")
	balances59 := dailyWaitBalances(ctx, balanceWaits, clients, e, "b59_same_day", observed59, 45*time.Second, true)
	pages59 := dailyPages(ctx, clients, s, first.Body.TxID, e, "b59_same_day")
	cli.stop()
	require(cli.clean(), "first production DAILY process joined")
	before60 := lateClosedAudit(ctx, root, s)
	lateSameBatch(first, before60)
	dailyAssertWinner(before60, s, workB, receipt, e, false)
	require(before60.Header.PendingBatch == 0 && before60.Delivery == "confirmed" && len(before60.Credits) == 1 && before60.Rounds[1].Observation != nil && before60.Rounds[1].Observation.Descendants == 59 && before60.Rounds[1].Observation.Canonical, "exact B59 no lock/no credit")
	write(filepath.Join(root, "07-B59-restart.json"), map[string]any{"audit": before60, "receipt": receipt, "duplicate": duplicate, "lookup": lookup, "recovered_balances": recovered, "balances": balances59, "pages": pages59, "process": cli.evidence(), "height": 162})
	phase = "one descendant then second exact production DAILY process"
	generate(ctx, a, b)
	at60 := time.Now().UnixMilli()
	sameDay(day)
	cli = lateStartCLI(ctx, binary, root, private, "slugpoold-daily-b60", s, children)
	balances60 := dailyWaitBalances(ctx, balanceWaits, clients, e, "b60_same_day", at60, 150*time.Second, true)
	check(pauseActual(ctx, 35*time.Second), "repeat original scheduler interval without second batch")
	pages60 := dailyPages(ctx, clients, s, first.Body.TxID, e, "b60_same_day")
	cli.stop()
	require(cli.clean(), "second production DAILY process joined")
	final := lateClosedAudit(ctx, root, s)
	lateSameBatch(first, final)
	dailyAssertWinner(final, s, workB, receipt, e, true)
	require(final.Header.PendingBatch == 0 && final.Delivery == "confirmed" && final.Header.Credits == 2 && len(final.Credits) == 2 && final.Rounds[1].Confirmation.Descendants == 60, "B60 exactly one new credit")
	count := 0
	for _, credit := range final.Credits {
		if credit.RoundHash != winner.WorkHash {
			continue
		}
		count++
		require(credit.Coin.TxID == coinB.Hash.String() && credit.Coin.Index == coinB.Index && credit.Coin.Atoms == e.BReward && credit.Coin.Script == s.WalletGuard.FundingScript && credit.Coin.Coinbase && credit.Coin.BlockHash == winner.WorkHash && credit.Coin.BlockHeight == 103 && len(credit.Coin.Claims) == 3, "exact B funding provenance")
		seen := map[string]bool{}
		for _, claim := range credit.Coin.Claims {
			require(!seen[claim.GymID] && claim.RoundHash == winner.WorkHash && claim.RoundHeight == 103, "unique original B claim")
			seen[claim.GymID] = true
			want := e.BOperator
			if claim.GymID != "" {
				matched := false
				for i, en := range s.Miners.Enrollments {
					if claim.GymID == en.GymID {
						want = e.BNet[i]
						matched = true
					}
				}
				require(matched, "configured B claim gym")
			}
			require(claim.Amount == want, "independent exact B99/1 claim")
		}
	}
	require(count == 1, "B credited exactly once")
	require(reflect.DeepEqual(first.Rounds[0].Round, final.Rounds[0].Round) && reflect.DeepEqual(first.Entries[0], final.Entries[0]), "original A accepted identity/round immutable")
	dailyRequireReport(final.Report, e.Reports["b60_same_day"], s, day)
	evidenceNode, err := poolsettlenode.NewDaily(ctx, s.Node, configs[0].RPC)
	check(err, "original normal node funding evidence")
	funding, inspectErr := evidenceNode.Inspect(ctx, []poolsettle.Coin{final.Credits[1].Coin})
	closeErr := evidenceNode.Close()
	check(errors.Join(inspectErr, closeErr), "joined B funding inspection")
	require(len(funding) == 1 && funding[0].TxID == coinB.Hash.String() && funding[0].Index == coinB.Index && funding[0].State == "immature", "B actual node funding still immature")
	assertAttempts(relay, first.Body.Raw)
	attempts, overflow := relay.attempted()
	require(!overflow && len(attempts) == 1 && len(relay.sends()) == 1 && relay.sends()[0].TxID == first.Body.TxID, "only one original A attempted and accepted send")
	a.rpc.must(ctx, "getblockcount", nil, &h)
	require(h == 163 && h <= maxBlocks, "exact final normal block budget")
	write(filepath.Join(root, "08-B60-final.json"), map[string]any{"audit": final, "balances": balances60, "pages": pages60, "funding": funding, "receipt": receipt, "original_batch_unchanged": true, "late_credit_count": count, "process": cli.evidence(), "height": h})
	return map[string]any{"final_height": h, "normal_blocks_max": maxBlocks, "ordinary_intake": 2, "late_rows": 3, "late_block_rows": 2, "late_credit_count": count, "pending_batch_unchanged": true, "exact_restart_receipt": true, "late_births": 0, "real_expiry": true, "signed_batches": 1, "fresh_production_cli_lifetimes": 2, "daily_tls_money_binding": true, "original_a_recovery_without_resend": true}
}
func dailyRequireReport(r poolsettle.Report, w expectedReport, s pooloperator.Setup, day string) {
	require(r.Day == day && r.State == w.State && r.Reason == w.Reason && len(r.Balances) == 2, "independent actual final report")
	for i, b := range r.Balances {
		require(b.GymID == s.Miners.Enrollments[i].GymID && dailyBalanceMatches(poolsettle.DailyBalanceFrom(b), w.Balances[i], true), "independent final outstanding/paid balance")
	}
}
