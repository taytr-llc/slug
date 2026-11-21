// External normal-PoW actual. Only public synthetic fixture owners are used.
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"time"

	"github.com/taytr-llc/slug/desktop/pooloperator"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospectwork"
	"github.com/taytr-llc/slug/wire/v2"
)

type lateBootstrapReport struct {
	Work                poolshare.MiningWork `json:"work"`
	Receipt             poolshare.Submission `json:"receipt"`
	SearchedNonces      uint32               `json:"searched_nonces"`
	OrdinarySubmissions uint32               `json:"ordinary_submissions"`
	FighterSubmissions  uint32               `json:"fighter_submissions"`
	Coin                wire.OutPoint        `json:"coin"`
}

// Each gym contributes exactly one block row. Ordinary candidates are hashed
// but never submitted; no worker or fighter signer runs in this accounting
// bootstrap. A lucky first nonce is not a prerequisite for the fixed row count.
func lateBootstrap(ctx context.Context, s pooloperator.Setup, a, b *node, r *runningOperator, configs []publicFixtureMinerConfig, clients []*poolservice.Client) ([]wire.OutPoint, []lateBootstrapReport) {
	var inputs []wire.OutPoint
	var reports []lateBootstrapReport
	for i, client := range clients {
		work := lateIssue(ctx, client, r)
		require(work.Job.Context.ParentHeight == uint32(i+1) && work.Job.PublicKey == configs[i].GymPublicKey, "original per-gym issued bootstrap work")
		prepared, e := poolservice.PrepareWork(s.Pool, configs[i].GymPublicKey, work)
		check(e, "bootstrap fixed original candidate")
		hashing, cancel := context.WithTimeout(ctx, 4*time.Minute)
		var winner prospectwork.ShareCheck
		var searched uint32
		for nonce := uint32(0); nonce < 64; nonce++ {
			candidate := lateCheck(hashing, prepared, nonce)
			searched++
			if candidate.Block {
				winner = candidate
				break
			}
		}
		cancel()
		require(winner.Block && winner.Candidate != nil, "bounded normal bootstrap block search")
		var reply any
		a.rpc.must(ctx, "submitblock", []any{winner.Candidate.BlockHex}, &reply)
		require(reply == nil && sameTip(ctx, a, b) == winner.WorkHash, "exact bootstrap block accepted and canonical")
		var hash string
		a.rpc.must(ctx, "getblockhash", []any{i + 2}, &hash)
		require(hash == winner.WorkHash, "original bootstrap height membership")
		block := block(ctx, a, hash)
		var encoded bytes.Buffer
		check(block.Serialize(&encoded), "bootstrap canonical block serialization")
		require(hx(encoded.Bytes()) == winner.Candidate.BlockHex, "canonical bootstrap bytes unchanged")
		var coin wire.OutPoint
		found := false
		for index, output := range block.Transactions[0].TxOut {
			if hx(output.PkScript) == s.WalletGuard.FundingScript {
				require(!found && output.Value == 1000000000, "exact bootstrap pool reward")
				coin = wire.OutPoint{Hash: block.Transactions[0].TxHash(), Index: uint32(index)}
				found = true
			}
		}
		require(found, "bootstrap pays original pool script")
		// Publication changed the parent; canonical admission is valid even
		// though the timely current-parent branch is now false.
		receipt, e := lateSubmit(ctx, client, work, winner.Nonce)
		check(e, "original TLS block-only submission")
		require(receipt.Sequence == uint64(i+1) && receipt.AcceptedSequence == uint64(i+1) && receipt.Block && receipt.BlockState == "canonical-observed" && receipt.WorkHash == winner.WorkHash && receipt.BlockHeight == uint32(i+2), "exact one bootstrap accepted block row")
		inputs = append(inputs, coin)
		reports = append(reports, lateBootstrapReport{Work: work, Receipt: receipt, SearchedNonces: searched, Coin: coin})
	}
	return inputs, reports
}

func lateIssue(ctx context.Context, c *poolservice.Client, r *runningOperator) poolshare.MiningWork {
	work, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	for {
		// Intake and public replay readiness consume this same original20s
		// budget. They never renew it or authorize a replacement runtime.
		status := runtimeStatus(work, r.runtime)
		if status.Capacity.Mode != "enabled" || !status.Public.Ready {
			check(pauseActual(work, time.Second), "original Issue readiness deadline")
			continue
		}
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

func lateWaitRounds(ctx context.Context, r *runningOperator, after int64, rows uint64) pooloperator.Status {
	var out pooloperator.Status
	wait(ctx, 90*time.Second, func() bool {
		v, e := r.runtime.Status(ctx)
		if e != nil {
			return false
		}
		if v.RoundCode != "" || v.RoundAtMS <= after || v.Rounds.Through != rows || v.Rounds.Rounds != 3 {
			return false
		}
		require(v.Capacity.Reservation.Submissions == 4 && v.Capacity.Mode == "draining" && !v.Capacity.OverBound && v.Capacity.CanonicalRecovery && v.Capacity.RecoveryBlocks == 3, "original derived drain remains exact")
		out = v
		return true
	})
	return out
}

func lateSameBatch(original poolsettle.Batch, a lateAudit) {
	require(len(a.Batches) == 1, "no replacement/additional batch")
	got := a.Batches[0]
	// Only recorded delivery state may change. This includes all original input
	// claims, exact scripts/amounts, fee, change, raw witness bytes, day and time.
	got.State = ""
	original.State = ""
	require(reflect.DeepEqual(original, got), "original signed batch economic bytes/claims changed")
}

func lateStartCLI(ctx context.Context, binary, root, private, name string, s pooloperator.Setup, children *[]*childProcess) *childProcess {
	path := filepath.Join(root, name+".log")
	c := startChild(binary, name, path, cliArgs("serve", root, private)...)
	*children = append(*children, c)
	wait(ctx, 60*time.Second, func() bool { return cliListening(path, s.Ingress.Origin) })
	return c
}

func runLateWinner(ctx context.Context, root, private, binary string, s pooloperator.Setup, a, b *node, relay *rpcRelay, running **runningOperator, children *[]*childProcess, configs []publicFixtureMinerConfig, clients []*poolservice.Client, inputs []wire.OutPoint) map[string]any {
	phase = "save original signed payout A before node forwarding"
	var held lostBroadcast
	select {
	case held = <-relay.held:
	case <-ctx.Done():
		panic("held payout canceled")
	case <-time.After(90 * time.Second):
		panic("signed payout did not reach hold")
	}
	wait(ctx, 30*time.Second, func() bool {
		for _, c := range clients {
			v, e := c.Payouts(ctx)
			if e != nil || len(v.History.Items) != 1 || v.History.Items[0].RecordedState != "prepared" || v.History.Balance.ReservedAtoms != minerPayment {
				return false
			}
		}
		return true
	})
	check((*running).stop(), "join original signed payout owner")
	*running = nil
	require(len(relay.sends()) == 0, "held payout never reached node")
	payment := validatePayment(held.Raw, s, inputs)
	first := lateClosedAudit(ctx, root, s)
	require(first.Submissions == 2 && first.Accepted == 2 && first.Works == 2 && first.AuthorityJobs == 2 && len(first.Batches) == 1 && len(first.Credits) == 2 && len(first.Entries) == 2 && len(first.Rounds) == 2 && first.Births == 0, "original two-reward fixture")
	original := first.Batches[0]
	require(original.Raw == held.Raw && original.TxID == payment["txid"] && original.State == "prepared", "original synchronized signed reservation")
	require(first.Report.Balances[0].ReservedAtoms == minerPayment && first.Report.Balances[1].ReservedAtoms == minerPayment, "both original9.9SLUG reserved")
	write(filepath.Join(root, "04-original-pending.json"), map[string]any{"payment": payment, "audit": first, "node_forwarded": false})

	phase = "issue distinct original B then fill real ordinary intake"
	rpcConfig := configs[0].RPC
	rpcConfig.URL = relay.url
	*running = constructRuntime(ctx, root, private, s, rpcConfig, false)
	(*running).run(ctx, s.Ingress.Origin)
	work := lateIssue(ctx, clients[0], *running)
	require(work.Job.Context.ParentHeight == 103, "original late job parent")
	for _, entry := range first.Entries {
		require(entry.Job.ID != work.Job.ID, "B must be distinct original job")
	}
	prepared, e := poolservice.PrepareWork(s.Pool, configs[0].GymPublicKey, work)
	check(e, "original B candidate")
	hashCtx, cancelHash := context.WithTimeout(ctx, 4*time.Minute)
	var filler []poolshare.Submission
	var nonce uint32
	for ; nonce < 64 && len(filler) < 2; nonce++ {
		v := lateCheck(hashCtx, prepared, nonce)
		if !v.Pool || v.Block {
			continue
		}
		row, e := lateSubmit(hashCtx, clients[0], work, nonce)
		check(e, "timely nonblock filler")
		require(row.Acceptance == "current-parent" && row.AcceptedSequence == uint64(3+len(filler)) && !row.Block, "exact ordinary accepted filler")
		filler = append(filler, row)
	}
	cancelHash()
	require(len(filler) == 2, "bounded two ordinary nonblock shares")
	var full pooloperator.Status
	wait(ctx, 30*time.Second, func() bool {
		v, e := (*running).runtime.Status(ctx)
		if e != nil {
			return false
		}
		full = v
		return v.Capacity.Mode == "draining" && v.Capacity.Submissions == 4
	})
	require(full.Capacity.Reservation.Submissions == 4 && !full.Capacity.OverBound, "real CapacityEnvelope four, never manual attachment")
	issueDenial := lateIssueClosed(ctx, clients[0])
	check((*running).stop(), "join before real expiry")
	*running = nil
	filled := lateClosedAudit(ctx, root, s)
	lateSameBatch(original, filled)
	require(filled.Submissions == 4 && filled.Accepted == 4 && filled.Works == 3 && filled.AuthorityJobs == 3 && len(filled.Entries) == 4 && len(filled.Rounds) == 2 && len(filled.Credits) == 2 && filled.Births == first.Births, "full intake before unseen winner")
	for _, v := range filled.Entries {
		require(v.Job.ID != work.Job.ID || !v.Share.Block, "no B winner row before expiry")
	}
	write(filepath.Join(root, "05-full-intake.json"), map[string]any{"status": full, "work": work, "filler": filler, "audit": filled, "issue_denial": issueDenial})

	phase = "real five-minute job expiry and actual normal winner search"
	expiryObservedMS := lateWaitWall(ctx, work.Job.ExpiresAtMS, "original real expiry")
	hashCtx, cancelHash = context.WithTimeout(ctx, 4*time.Minute)
	var winner prospectwork.ShareCheck
	var ordinary prospectwork.ShareCheck
	for checked := 0; checked < 64 && (!winner.Block || ordinary.Candidate == nil); checked++ {
		v := lateCheck(hashCtx, prepared, nonce)
		nonce++
		if v.Block && !winner.Block {
			winner = v
		}
		if v.Pool && !v.Block && ordinary.Candidate == nil {
			ordinary = v
		}
	}
	cancelHash()
	require(winner.Block && winner.Candidate != nil && ordinary.Candidate != nil, "bounded actual winner plus late nonblock")
	winnerObservedMS := time.Now().UnixMilli()
	require(winnerObservedMS >= work.Job.ExpiresAtMS, fmt.Sprintf("winner found only after expiry: observed_wall_ms=%d expires_at_ms=%d", winnerObservedMS, work.Job.ExpiresAtMS))
	*running = constructRuntime(ctx, root, private, s, rpcConfig, false)
	(*running).run(ctx, s.Ingress.Origin)
	// The original full4/4 attachment is draining before listeners start.
	// Recovery does not depend on public-replay readiness. Each exact original
	// identity retries only definite pre-admission contention, never uncertainty.
	_, ordinaryErr := lateSubmit(ctx, clients[0], work, ordinary.Nonce)
	ordinaryFailure := lateRequireFailure(ordinaryErr, "capacity", false, "ordinary late row remains refused at drain")
	_, winnerErr := lateSubmit(ctx, clients[0], work, winner.Nonce)
	winnerFailure := lateRequireFailure(winnerErr, "outcome_unknown", true, "conservative mutating node-proof failure")
	// The HTTP boundary conservatively labels ErrNode unknown. Do not retry it:
	// close and audit the exact original row/chain before later canonical recovery.

	var h uint32
	a.rpc.must(ctx, "getblockcount", nil, &h)
	require(h == 103, "preproof recovery cannot broadcast B")
	check((*running).stop(), "join canonical precondition refusal")
	*running = nil
	absent := lateClosedAudit(ctx, root, s)
	lateSameBatch(original, absent)
	require(absent.Submissions == 4 && absent.Accepted == 4 && absent.Works == 3 && absent.AuthorityJobs == 3 && reflect.DeepEqual(absent.Entries, filled.Entries) && len(absent.Credits) == 2 && absent.Births == first.Births, "refused late candidates changed no accepted rows/births")

	phase = "publish exact original B block after expiry"
	var reply any
	a.rpc.must(ctx, "submitblock", []any{winner.Candidate.BlockHex}, &reply)
	require(reply == nil, "node accepts exact B block")
	require(sameTip(ctx, a, b) == winner.WorkHash, "B actual canonical membership")
	var hash string
	a.rpc.must(ctx, "getblockhash", []any{104}, &hash)
	require(hash == winner.WorkHash, "exact B height")
	blk := block(ctx, a, hash)
	var blockBytes bytes.Buffer
	check(blk.Serialize(&blockBytes), "canonical B bytes")
	require(hx(blockBytes.Bytes()) == winner.Candidate.BlockHex, "exact original candidate became canonical")
	require(len(blk.Transactions) == 1, "held A cannot enter B template")
	var lateCoin wire.OutPoint
	found := false
	for i, o := range blk.Transactions[0].TxOut {
		if hx(o.PkScript) == s.WalletGuard.FundingScript {
			require(!found && o.Value == 1000000000, "exact late reward")
			lateCoin = wire.OutPoint{Hash: blk.Transactions[0].TxHash(), Index: uint32(i)}
			found = true
		}
	}
	require(found, "late original pool payee")
	for _, v := range inputs {
		require(v != lateCoin, "late funding distinct from reserved A inputs")
	}
	write(filepath.Join(root, "06-canonical-late.json"), map[string]any{"job": work.Job, "proof": winner, "block_hash": hash, "height": 104, "coin": lateCoin, "observed_at_ms": time.Now().UnixMilli(), "expiry_observed_at_ms": expiryObservedMS, "winner_observed_at_ms": winnerObservedMS, "ordinary_refusal": ordinaryFailure, "noncanonical_refusal": winnerFailure, "absent_audit": absent})

	phase = "reopen original operator and accept unseen canonical B at drain"
	started := time.Now().UnixMilli()
	*running = constructRuntime(ctx, root, private, s, rpcConfig, false)
	(*running).run(ctx, s.Ingress.Origin)
	receipt, e := lateSubmit(ctx, clients[0], work, winner.Nonce)
	check(e, "D80 unseen winner at full ordinary intake")
	require(receipt.Sequence == 5 && receipt.AcceptedSequence == 5 && receipt.Acceptance == "canonical-late-winner" && receipt.AcceptedAtMS >= work.Job.ExpiresAtMS && receipt.BlockState == "canonical-observed" && receipt.WorkHash == winner.WorkHash, "one late canonical acceptance")
	lateWaitRounds(ctx, *running, started, 5)
	check((*running).stop(), "join accepted B owner")
	*running = nil
	accepted := lateClosedAudit(ctx, root, s)
	lateSameBatch(original, accepted)
	lateAssertWinner(accepted, work, receipt, first.Births, false)
	require(len(accepted.Credits) == 2, "no immature/unconfirmed B credit import")

	phase = "separate production operator process retries exact B"
	cli := lateStartCLI(ctx, binary, root, private, "slugpoold-late-retry", s, children)
	duplicate, e := lateSubmit(ctx, clients[0], work, winner.Nonce)
	check(e, "same original B retry after process restart")
	require(reflect.DeepEqual(receipt, duplicate), "byte-equivalent saved receipt after process restart")
	looked, e := lateLookup(ctx, clients[0], work, winner.Nonce)
	check(e, "original B lookup")
	require(reflect.DeepEqual(receipt, looked), "lookup must not substitute receipt")
	cli.stop()
	require(cli.clean(), "production retry process joined")
	retried := lateClosedAudit(ctx, root, s)
	lateSameBatch(original, retried)
	lateAssertWinner(retried, work, receipt, first.Births, false)
	require(reflect.DeepEqual(accepted.Entries, retried.Entries) && len(retried.Credits) == 2, "retry creates neither share nor credit")
	write(filepath.Join(root, "07-late-restart.json"), map[string]any{"receipt": receipt, "duplicate": duplicate, "audit": retried, "process": cli.evidence()})

	phase = "B59 then B60 while original A remains reserved"
	for height := uint32(104); height < 163; height++ {
		generate(ctx, a, b)
	}
	started = time.Now().UnixMilli()
	*running = constructRuntime(ctx, root, private, s, rpcConfig, false)
	(*running).run(ctx, s.Ingress.Origin)
	lateWaitRounds(ctx, *running, started, 5)
	check((*running).stop(), "join B59 observation")
	*running = nil
	before60 := lateClosedAudit(ctx, root, s)
	lateSameBatch(original, before60)
	require(before60.Rounds[2].Confirmation == nil && before60.Rounds[2].Observation != nil && before60.Rounds[2].Observation.Descendants == 59 && len(before60.Credits) == 2, "exact B59 no credit/confirmation")
	generate(ctx, a, b)
	started = time.Now().UnixMilli()
	*running = constructRuntime(ctx, root, private, s, rpcConfig, false)
	(*running).run(ctx, s.Ingress.Origin)
	lateWaitRounds(ctx, *running, started, 5)
	check((*running).stop(), "join B60 before releasing A")
	*running = nil
	locked := lateClosedAudit(ctx, root, s)
	lateSameBatch(original, locked)
	lateAssertWinner(locked, work, receipt, first.Births, true)
	require(locked.Rounds[2].Confirmation.Descendants == 60 && len(locked.Credits) == 2, "RecoverPending must not import B credit")
	write(filepath.Join(root, "08-confirmed-reservation.json"), map[string]any{"before60": before60, "at60": locked, "original_batch_unchanged": true})

	phase = "release exact A, lose accepted reply, reopen and resolve original reservation"
	relay.mu.Lock()
	relay.hold = false
	relay.mu.Unlock()
	*running = constructRuntime(ctx, root, private, s, rpcConfig, false)
	(*running).run(ctx, s.Ingress.Origin)
	var lost lostBroadcast
	select {
	case lost = <-relay.lost:
		(*running).runCancel()
	case <-ctx.Done():
		panic("original payout release canceled")
	case <-time.After(90 * time.Second):
		panic("original payout not released")
	}
	check((*running).stop(), "join accepted A lost reply")
	*running = nil
	require(lost.Raw == original.Raw && lost.TxID == original.TxID, "only retained signed A sent")
	assertAttempts(relay, original.Raw)
	*running = constructRuntime(ctx, root, private, s, rpcConfig, false)
	(*running).run(ctx, s.Ingress.Origin)
	wait(ctx, 90*time.Second, func() bool {
		for _, c := range clients {
			v, e := c.Payouts(ctx)
			if e != nil || len(v.History.Items) != 1 || v.History.Items[0].TxID != original.TxID || v.History.Items[0].RecordedState != "broadcast" {
				return false
			}
		}
		return true
	})
	mined := generate(ctx, a, b)
	require(hx(raw(minedTx(ctx, a, mined, original.TxID))) == original.Raw, "canonical original payout bytes")
	wait(ctx, 90*time.Second, func() bool {
		for i, c := range clients {
			v, e := c.Payouts(ctx)
			if e != nil || len(v.History.Items) != 1 || v.History.Items[0].TxID != original.TxID || v.History.Items[0].RecordedState != "confirmed" || v.History.Balance.PaidAtoms != minerPayment || v.History.Balance.ReservedAtoms != 0 {
				return false
			}
			want := "0"
			if i == 0 {
				want = "990000000"
			}
			if v.History.Balance.Confirmed.Numerator != want || v.History.Balance.Confirmed.Denominator != "1" || v.History.Balance.Available.Numerator != "0" {
				return false
			}
		}
		return true
	})
	check(pauseActual(ctx, 65*time.Second), "two original scheduler intervals without replacement")
	check((*running).stop(), "join final original scheduler")
	*running = nil
	final := lateClosedAudit(ctx, root, s)
	lateSameBatch(original, final)
	lateAssertWinner(final, work, receipt, first.Births, true)
	require(final.Batches[0].State == "confirmed" && len(final.Credits) == 3, "one new B credit and original A resolved")
	count := 0
	for _, credit := range final.Credits {
		if credit.RoundHash == winner.WorkHash {
			count++
			require(credit.Coin.TxID == lateCoin.Hash.String() && credit.Coin.Index == lateCoin.Index && credit.Coin.Atoms == 1000000000, "exact distinct B funding credit")
		}
	}
	require(count == 1, "exactly one B credit")
	for i := 0; i < 2; i++ {
		require(reflect.DeepEqual(first.Rounds[i].Round, final.Rounds[i].Round) && reflect.DeepEqual(first.Entries[i], final.Entries[i]), "original D81 round membership and accepted identity immutable")
	}
	for _, credit := range final.Credits {
		if credit.RoundHash != winner.WorkHash {
			continue
		}
		require(len(credit.Coin.Claims) == 2, "one B miner claim plus operator claim")
		miner, operator := 0, 0
		for _, claim := range credit.Coin.Claims {
			require(claim.RoundHash == winner.WorkHash && claim.RoundHeight == 104, "exact B claim source")
			if claim.GymID == work.Job.GymID {
				miner++
				require(claim.Amount == (poolsettle.Fraction{Numerator: "990000000", Denominator: "1"}), "independent B99percent claim")
			} else {
				operator++
				require(claim.GymID == "" && claim.Amount == (poolsettle.Fraction{Numerator: "10000000", Denominator: "1"}), "independent B1percent claim")
			}
		}
		require(miner == 1 && operator == 1, "exact B claim split")
	}
	assertAttempts(relay, original.Raw)
	for _, send := range relay.sends() {
		require(send.Raw == original.Raw && send.TxID == original.TxID, "no replacement journaled/broadcast transaction")
	}
	a.rpc.must(ctx, "getblockcount", nil, &h)
	require(h == 165 && h <= maxBlocks, "original normal block budget")
	write(filepath.Join(root, "09-combined-final.json"), map[string]any{"audit": final, "payment": payment, "winner_receipt": receipt, "late_credit_count": count, "original_batch_unchanged": true, "attempts": relay.attemptedPublic(), "final_height": h})
	return map[string]any{"final_height": h, "normal_blocks_max": maxBlocks, "late_credit_count": count, "ordinary_intake": 4, "late_rows": 5, "late_block_rows": 3, "pending_batch_unchanged": true, "exact_restart_receipt": true, "late_births": 0, "real_expiry": true}
}

func (r *rpcRelay) attemptedPublic() []lostBroadcast {
	rows, overflow := r.attempted()
	require(!overflow, "attempt inventory")
	return rows
}
