//go:build slugdailyactual && !slugtestpow && (linux || (darwin && cgo))

package poolsettle_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/poolsettlefixture"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectwork"
	"github.com/taytr-llc/slug/wire/v2"
	"github.com/taytr-llc/slugwallet/waddrmgr"
	"github.com/taytr-llc/slugwallet/wallet"
)

const dailyAdmissionReferenceSHA = "1d2b1462296bd022c53df9dc583db3c38cd13a8e13e84f9983f2505b3aefb08f"

type dailyAdmissionExpectedBalance struct {
	Gym       string              `json:"gym"`
	Confirmed poolsettle.Fraction `json:"confirmed"`
	Available poolsettle.Fraction `json:"available"`
	Reserved  int64               `json:"reserved_atoms"`
	Paid      int64               `json:"paid_atoms"`
	Reason    string              `json:"reason"`
}
type dailyAdmissionExpectedReport struct {
	State    string                          `json:"state"`
	Reason   string                          `json:"reason"`
	Balances []dailyAdmissionExpectedBalance `json:"balances"`
}
type dailyAdmissionEconomics struct {
	RewardA     int64                                   `json:"a_reward_atoms"`
	RewardB     int64                                   `json:"b_reward_atoms"`
	Fee         int64                                   `json:"a_fee_atoms"`
	Payment     int64                                   `json:"a_payment_atoms"`
	Change      int64                                   `json:"a_change_atoms"`
	RoundBNet   []poolsettle.Fraction                   `json:"round_b_net_by_gym"`
	RoundBGross []poolsettle.Fraction                   `json:"round_b_gross_by_gym"`
	OperatorA   poolsettle.Fraction                     `json:"operator_a_earned"`
	OperatorB   poolsettle.Fraction                     `json:"operator_b_earned"`
	Reports     map[string]dailyAdmissionExpectedReport `json:"reports"`
}
type dailyAdmissionReference struct {
	Original dailyActualReference `json:"-"`
	Capacity struct {
		Maximum uint64 `json:"maximum"`
	} `json:"capacity"`
	Economics dailyAdmissionEconomics `json:"actual_economics"`
}

func dailyAdmissionReadReference(path string) dailyAdmissionReference {
	raw := dailyActualPublicBytes(path, 1<<20)
	require(len(raw) == 29980 && hx(digest(raw)) == dailyAdmissionReferenceSHA, "independent Set78 reference")
	var r dailyAdmissionReference
	check(json.Unmarshal(raw, &r), "actual expected projections")
	check(json.Unmarshal(raw, &r.Original), "actual original binding")
	b := r.Original
	require(len(b.Accounts) == 2 && len(b.Binding.Settlement.Destinations) == 2 && r.Capacity.Maximum == 2, "two gyms and independently derived ceiling")
	check(wallet.ValidateDailyPoolWalletBinding(b.Binding), "original selected wallet binding")
	sid, e := poolshare.DailyConfigID(b.Binding.Source)
	check(e, "source ID")
	cid, e := poolsettle.DailyConfigBindingID(b.Binding.Settlement)
	check(e, "config ID")
	require(sid == b.SourceID && cid == b.ConfigID, "independent source/config identities")
	guard, e := waddrmgr.MarshalPoolSettlementGuard(b.Binding.Guard)
	check(e, "guard bytes")
	gid, e := waddrmgr.PoolSettlementGuardDigest(b.Binding.Guard)
	check(e, "guard identity")
	require(hx(guard) == b.GuardHex && gid == b.GuardDigest, "independent guard bytes and identity")
	for i, account := range b.Accounts {
		require(account.Index == i && account.Scalar == i+1, "published scalar1/2 identities")
	}
	return r
}

type dailyAdmissionOwners struct {
	base       *dailyActualOwners
	intake     *poolshare.OperatorAdmission
	closeError error
}

func (o *dailyAdmissionOwners) close() error {
	if o.closeError != nil {
		return o.closeError
	}
	if o.intake != nil {
		if e := o.intake.Close(); e != nil {
			o.closeError = e
			return e
		}
		o.intake = nil
	}
	o.closeError = o.base.close()
	return o.closeError
}
func dailyAdmissionOpen(ctx context.Context, root string, r dailyAdmissionReference, rpc prospectnode.RPCConfig, create bool, o *dailyAdmissionOwners) {
	dailyActualOpen(ctx, root, r.Original, rpc, create, o.base)
	limits, source, e := o.base.rounds.DailyBinding(ctx)
	check(e, "original round binding before intake")
	config, ledgerSource, e := o.base.ledger.DailySchedulerBinding(ctx)
	check(e, "original ledger binding before intake")
	require(limits == r.Original.Limits && source == r.Original.Binding.Source && ledgerSource == source && reflect.DeepEqual(config, r.Original.Binding.Settlement), "same original full configuration")
	gyms := []string{}
	for _, destination := range config.Destinations {
		gyms = append(gyms, destination.GymID)
	}
	for _, account := range r.Original.Accounts {
		i := sort.SearchStrings(gyms, account.GymID)
		require(i < len(gyms) && gyms[i] == account.GymID, "all original sessions are selected destinations")
	}
	o.intake, e = o.base.source.AttachDailyOperatorAdmission(ctx, source, poolshare.DailyRecoveryBudget{Rounds: limits.MaxRounds, RoundBytes: limits.MaxBytes, CreditRounds: uint64(config.MaxRounds), Batches: uint64(config.MaxBatches), GymIDs: gyms})
	check(e, "original daily intake attachment")
	status, e := o.intake.Status(ctx)
	check(e, "original attached census")
	require(status.Maximum == r.Capacity.Maximum && status.CanonicalRecovery && !status.OverBound, "whole-history daily reservation")
	if create {
		require(status.Mode == "paused", "fresh intake initially paused")
		check(o.intake.Enable(ctx), "explicit original intake enable")
	} else {
		require(status.Mode == "draining", "original above-ordinary reopen remains draining")
	}
}

func dailyAdmissionStatus(ctx context.Context, o *dailyAdmissionOwners, jobs, rows, blocks uint64) poolshare.OperatorAdmissionStatus {
	status, e := o.intake.Status(ctx)
	check(e, "original complete physical census")
	info, e := o.base.issuer.Info()
	check(e, "original issuer counters")
	require(status.Maximum == 2 && status.Works == jobs && status.Submissions == rows && status.Accepted == rows && status.RecoveryBlocks == blocks && !status.OverBound && status.CanonicalRecovery, "exact original retained counts and reservation")
	require(info.Jobs == jobs && info.Records == 0, "exact issuer jobs and no fighter births")
	if rows >= 2 {
		require(status.Mode == "draining", "full ordinary intake remains drained")
	}
	return status
}
func dailyAdmissionEvidence(ctx context.Context, o *dailyAdmissionOwners) poolsettle.DailyAdmissionActualEvidence {
	op, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	e, err := poolsettle.DailyAdmissionActualRead(op, o.base.ledger)
	check(err, "full original ledger/body evidence")
	return e
}
func dailyAdmissionDay(day string) {
	require(time.Now().UTC().Format("2006-01-02") == day, "same real UTC day without clock substitution")
}
func dailyAdmissionReport(got poolsettle.Report, r dailyAdmissionReference, expected, day string) {
	dailyAdmissionDay(day)
	want, ok := r.Economics.Reports[expected]
	require(ok && got.Day == day && got.State == want.State && got.Reason == want.Reason && len(got.Balances) == 2, "independent report phase")
	for _, w := range want.Balances {
		index := 0
		if w.Gym == "gym2" {
			index = 1
		} else {
			require(w.Gym == "gym1", "known independent gym label")
		}
		gym := r.Original.Accounts[index].GymID
		found := false
		for _, b := range got.Balances {
			if b.GymID == gym {
				require(!found && b.Confirmed == w.Confirmed && b.Available == w.Available && b.ReservedAtoms == w.Reserved && b.PaidAtoms == w.Paid && b.Reason == w.Reason, "exact independent outstanding/reserved/paid balance")
				found = true
			}
		}
		require(found, "both independent report gyms")
	}
}
func dailyAdmissionLookup(ctx context.Context, o *dailyAdmissionOwners, account dailyActualAccount, w poolshare.MiningWork, nonce uint32) (poolshare.Submission, error) {
	op, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	for {
		v, e := o.base.source.Lookup(op, account.Token, w.Job.ID, nonce)
		if e != poolshare.ErrBusy {
			return v, e
		}
		dailyActualPause(op)
	}
}
func dailyAdmissionSearch(ctx context.Context, r dailyAdmissionReference, w poolshare.MiningWork, block bool, count int) []prospectwork.ShareCheck {
	p, e := prospectwork.PrepareManifestWork(w.Job, w.Template, r.Original.Binding.Source.PayoutAddress, 0, r.Original.Binding.Source.PoolTarget)
	check(e, "original candidate preparation")
	op, cancel := context.WithTimeout(ctx, 4*time.Minute)
	defer cancel()
	out := []prospectwork.ShareCheck{}
	for nonce := uint32(0); nonce < 64 && len(out) < count; nonce++ {
		var v prospectwork.ShareCheck
		for {
			v, e = p.CheckNonce(op, nonce)
			if e != prospectwork.ErrShareBusy {
				break
			}
			dailyActualPause(op)
		}
		check(e, "normal independent nonce classification")
		if v.Block == block && v.Pool {
			out = append(out, v)
		}
	}
	require(len(out) == count, "bounded nonce search without retaining skipped candidates")
	return out
}
func dailyAdmissionWaitWall(ctx context.Context, target int64) {
	for {
		require(ctx.Err() == nil, "original expiry deadline")
		remaining := target - time.Now().UnixMilli()
		if remaining <= 0 {
			return
		}
		timer := time.NewTimer(time.Duration(remaining) * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			panic("original expiry deadline")
		case <-timer.C:
		}
	}
}
func dailyAdmissionBatchA(batch poolsettle.Batch, reward dailyActualReward, r dailyAdmissionReference) {
	e := r.Economics
	account := r.Original.Accounts[0]
	require(len(batch.Inputs) == 1 && len(batch.Payments) == 1 && batch.Change != nil && batch.FeeAtoms == e.Fee, "single original signed native payout")
	c := batch.Inputs[0]
	require(c.TxID == reward.Coin.Hash.String() && c.Index == reward.Coin.Index && c.Coinbase && c.Atoms == e.RewardA && c.Script == r.Original.Binding.Guard.FundingScript && c.BlockHash == reward.Receipt.WorkHash && c.BlockHeight == 2 && len(c.Claims) == 2, "A spends only its original credited reward")
	claims := map[string]poolsettle.Fraction{account.GymID: {Numerator: fmt.Sprint(e.Payment), Denominator: "1"}, "": e.OperatorA}
	for _, claim := range c.Claims {
		want, ok := claims[claim.GymID]
		require(ok && claim.RoundHash == reward.Receipt.WorkHash && claim.RoundHeight == 2 && claim.Amount == want, "exact original A claims")
		delete(claims, claim.GymID)
	}
	require(len(claims) == 0 && batch.Payments[0] == (poolsettle.Payment{GymID: account.GymID, Script: account.Script, Atoms: e.Payment}), "exact independent A payment")
	change := batch.Change
	require(change.TxID == batch.TxID && change.Index == 1 && change.Atoms == e.Change && change.Script == c.Script && len(change.Claims) == 1 && change.Claims[0].GymID == "" && change.Claims[0].RoundHash == reward.Receipt.WorkHash && change.Claims[0].Amount == (poolsettle.Fraction{Numerator: fmt.Sprint(e.Change), Denominator: "1"}), "only original operator cut remains")
	tx := wire.NewMsgTx(2)
	check(tx.Deserialize(bytes.NewReader(dec(batch.Raw))), "signed A bytes")
	require(tx.TxHash().String() == batch.TxID && len(tx.TxIn) == 1 && len(tx.TxOut) == 2 && tx.TxIn[0].PreviousOutPoint == reward.Coin && tx.TxOut[0].Value == e.Payment && hx(tx.TxOut[0].PkScript) == account.Script && tx.TxOut[1].Value == e.Change && hx(tx.TxOut[1].PkScript) == c.Script && c.Atoms-tx.TxOut[0].Value-tx.TxOut[1].Value == e.Fee, "original signed A inputs outputs and fee")
}

type dailyAdmissionPhaseA struct {
	Batch    poolsettle.Batch                        `json:"batch"`
	Reward   dailyActualReward                       `json:"reward"`
	Snapshot poolsettle.DailyActualState             `json:"snapshot"`
	Evidence poolsettle.DailyAdmissionActualEvidence `json:"evidence"`
	Status   poolshare.OperatorAdmissionStatus       `json:"status"`
	Day      string                                  `json:"day"`
}
type dailyAdmissionPhaseB struct {
	Work              poolshare.MiningWork                    `json:"work"`
	Receipt           poolshare.Submission                    `json:"receipt"`
	Ordinary          poolshare.Submission                    `json:"ordinary"`
	Status            poolshare.OperatorAdmissionStatus       `json:"status"`
	BeforeStatus      poolshare.OperatorAdmissionStatus       `json:"before_status"`
	Tail              poolshare.AccountingPage                `json:"tail"`
	Snapshot          poolsettle.DailyActualState             `json:"snapshot"`
	Evidence          poolsettle.DailyAdmissionActualEvidence `json:"evidence"`
	ExpiryObserved    int64                                   `json:"expiry_observed_at_ms"`
	CanonicalObserved int64                                   `json:"canonical_observed_at_ms"`
	AbsentBefore      bool                                    `json:"absent_before"`
}

func TestDailyAdmissionActualLateWinnerFreshProcess(t *testing.T) {
	if os.Getenv("SLUG_DAILY_ACTUAL") != "1" || os.Getenv("SLUG_DAILY_REOPEN_CHILD") != "" {
		t.Skip("explicit isolated daily-admission actual opt-in")
	}
	day := time.Now().UTC()
	midnight := time.Date(day.Year(), day.Month(), day.Day()+1, 0, 0, 0, 0, time.UTC)
	if time.Until(midnight) <= 900*time.Second {
		t.Fatal("insufficient same-day real UTC budget")
	}
	root := os.Getenv("SLUG_DAILY_ROOT")
	if !filepath.IsAbs(root) || filepath.Clean(root) != root || root == string(os.PathSeparator) {
		t.Fatal("explicit fresh canonical fixture root")
	}
	parent, e := filepath.EvalSymlinks(filepath.Dir(root))
	if e != nil || parent != filepath.Dir(root) {
		t.Fatal("canonical existing fixture parent")
	}
	if e = os.Mkdir(root, 0700); e != nil {
		t.Fatal("fresh fixture root required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 900*time.Second)
	defer cancel()
	out := dailyAdmissionRun(ctx, root, os.Getenv("SLUG_DAILY_NODE"), os.Getenv("SLUG_DAILY_REFERENCE"), os.Getenv("SLUG_DAILY_OUTPUT"), day.Format("2006-01-02"))
	if out["pass"] != true {
		t.Fatalf("daily admission actual failed: phase=%v failure=%v cleanup=%v", out["phase"], out["failure"], out["cleanup_errors"])
	}
}

func dailyAdmissionRun(ctx context.Context, root, binary, reference, output, day string) (out map[string]any) {
	out = map[string]any{"version": 1, "pass": false, "scope": "finite derived daily intake2 to canonical receipt3; real normal nodes/schema16 and fresh-process original recovery; fixture sessions; no test clock, public TLS or scheduler claim"}
	start := time.Now()
	owners := &dailyAdmissionOwners{base: &dailyActualOwners{}}
	var nodes []*node
	var relay *rpcRelay
	var child *dailyActualRecoveryProcess
	defer func() {
		if v := recover(); v != nil {
			out["failure"] = fmt.Sprintf("%.1024s", fmt.Sprint(v))
			out["phase"] = phase
		}
		cleanup := []string{}
		joined := true
		if child != nil {
			child.stop()
			out["recovery_process"] = child.metadata()
			joined = !child.started || child.joined
			if !child.clean() {
				cleanup = append(cleanup, "recovery child cleanup")
			}
		}
		if e := owners.close(); e != nil {
			cleanup = append(cleanup, fmt.Sprintf("owners: %.256s", e))
		}
		if relay != nil && joined {
			if e := relay.close(); e != nil {
				cleanup = append(cleanup, fmt.Sprintf("relay: %.256s", e))
			}
			out["broadcasts"] = relay.sends()
			attempts, overflow := relay.attempted()
			out["broadcast_attempts"], out["broadcast_overflow"] = attempts, overflow
			if overflow {
				cleanup = append(cleanup, "send attempt overflow")
			}
		}
		rows := []any{}
		for i := len(nodes) - 1; i >= 0; i-- {
			if joined {
				nodes[i].stop()
			}
			rows = append(rows, nodes[i].cleanupReport())
			if !nodes[i].clean() {
				cleanup = append(cleanup, "node join")
			}
		}
		out["node_cleanup"], out["cleanup_errors"], out["elapsed_seconds"] = rows, cleanup, time.Since(start).Seconds()
		out["pass"] = out["completed"] == true && len(cleanup) == 0
		if filepath.IsAbs(output) {
			write(filepath.Join(output, "actual-report.json"), out)
		}
	}()
	require(filepath.IsAbs(binary) && filepath.IsAbs(output), "explicit existing node/output paths")
	r := dailyAdmissionReadReference(reference)
	require(!wire.TestPoW, "normal PoW")
	bootstrap := poolsettlefixture.PoolAddress()
	require(bootstrap != r.Original.Binding.Source.PayoutAddress, "bootstrap rewards excluded from credited source")
	p1, p2 := port(), port()
	require(p1 != p2, "distinct isolated peers")
	phase = "two original normal nodes and uncredited bootstrap"
	a := startNode(ctx, binary, root, "node-a", p1, p2, bootstrap)
	nodes = append(nodes, a)
	b := startNode(ctx, binary, root, "node-b", p2, p1, bootstrap)
	nodes = append(nodes, b)
	ready(ctx, a)
	ready(ctx, b)
	wait(ctx, 30*time.Second, func() bool {
		var peers []any
		return a.rpc.call(ctx, "getpeerinfo", nil, &peers) == nil && len(peers) > 0
	})
	peers(ctx, a)
	peers(ctx, b)
	var height uint32
	a.rpc.must(ctx, "getblockcount", nil, &height)
	require(height == 0, "fresh genesis")
	generate(ctx, a, b)
	relay = startRPCRelay(ctx, a.rpc.endpoint)
	rpcConfig := prospectnode.RPCConfig{Version: 1, URL: relay.url, Username: "fixture", Password: "public-pool-actual"}
	owned := filepath.Join(root, "owners")
	check(os.Mkdir(owned, 0700), "fresh original owner root")
	dailyAdmissionOpen(ctx, owned, r, rpcConfig, true, owners)
	phase = "original A reward and pending reserved payment"
	reward := dailyActualWinner(ctx, owners.base, r.Original, a, b, 0, 2)
	dailyActualMature(ctx, a, b, 102)
	dailyActualRounds(ctx, owners.base, []dailyActualReward{reward})
	dailyAdmissionDay(day)
	relay.mu.Lock()
	relay.drop = true
	relay.mu.Unlock()
	report := dailyActualRun(ctx, owners.base, false)
	dailyAdmissionReport(report, r, "a_lost_reply", day)
	var lost lostBroadcast
	select {
	case lost = <-relay.lost:
	case <-ctx.Done():
		panic("original accepted A lost response")
	}
	snap := dailyActualSnapshot(ctx, owners.base)
	require(snap.Credits == 1 && len(snap.Batches) == 1 && snap.Pending == 1, "original A reserved")
	originalA := snap.Batches[0]
	dailyAdmissionBatchA(originalA, reward, r)
	require(lost.Raw == originalA.Raw && lost.TxID == originalA.TxID && originalA.State == "prepared", "accepted response lost after immutable reservation")
	var mempoolRaw string
	a.rpc.must(ctx, "getrawtransaction", []any{originalA.TxID, 0}, &mempoolRaw)
	require(mempoolRaw == originalA.Raw, "A exact bytes in original mempool")
	ae := dailyAdmissionEvidence(ctx, owners)
	priorA := dailyAdmissionPhaseA{Batch: originalA, Reward: reward, Snapshot: snap, Evidence: ae, Status: dailyAdmissionStatus(ctx, owners, 1, 1, 1), Day: day}
	write(filepath.Join(output, "01-A.json"), priorA)
	phase = "B and C original jobs fill the derived ordinary boundary"
	workB := dailyActualIssue(ctx, owners.base, r.Original.Accounts[1])
	workC := dailyActualIssue(ctx, owners.base, r.Original.Accounts[0])
	require(workB.Job.Context.ParentHeight == 102 && workC.Job.Context.ParentHeight == 102 && workB.Job.GymID == r.Original.Accounts[1].GymID && workC.Job.GymID == r.Original.Accounts[0].GymID && workB.Job.ExpiresAtMS-workB.Job.IssuedAtMS == 300000, "distinct gym-bound original jobs and real lifetime")
	winner := dailyAdmissionSearch(ctx, r, workB, true, 1)[0]
	require(winner.Candidate != nil && winner.Candidate.CoinbaseValue == r.Economics.RewardB && winner.Candidate.TransactionCount == 2, "B original reward includes only A fee")
	var candidate wire.MsgBlock
	check(candidate.Deserialize(bytes.NewReader(dec(winner.Candidate.BlockHex))), "original B block bytes")
	require(len(candidate.Transactions) == 2 && hx(raw(candidate.Transactions[1])) == originalA.Raw, "B immutable template includes exact reserved A")
	nonblocks := dailyAdmissionSearch(ctx, r, workC, false, 2)
	ordinary := dailyActualSubmit(ctx, owners.base, r.Original.Accounts[0], workC, nonblocks[0].Nonce)
	require(ordinary.Sequence == 2 && ordinary.AcceptedSequence == 2 && !ordinary.Block && ordinary.Pool && ordinary.Acceptance == "current-parent", "exact original non-block ordinary second row")
	before := dailyAdmissionStatus(ctx, owners, 3, 2, 1)
	_, e := owners.base.source.IssueMiningWork(ctx, r.Original.Accounts[0].Token)
	require(e == poolshare.ErrCapacity, "ordinary issuance definitely closed")
	_, e = owners.base.source.Submit(ctx, r.Original.Accounts[0].Token, workC.Job.ID, nonblocks[1].Nonce)
	require(e == poolshare.ErrCapacity, "new ordinary retention definitely closed")
	_, e = dailyAdmissionLookup(ctx, owners, r.Original.Accounts[1], workB, winner.Nonce)
	require(e == poolshare.ErrNotFound, "winning nonce physically absent before canonical submission")
	dailyAdmissionStatus(ctx, owners, 3, 2, 1)
	phase = "original B expires on real wall clock then becomes canonical"
	dailyAdmissionWaitWall(ctx, workB.Job.ExpiresAtMS)
	expiryAt := time.Now().UnixMilli()
	require(expiryAt >= workB.Job.ExpiresAtMS, "actual expiry reached")
	dailyAdmissionDay(day)
	var reply any
	a.rpc.must(ctx, "submitblock", []any{winner.Candidate.BlockHex}, &reply)
	require(reply == nil && sameTip(ctx, a, b) == winner.WorkHash, "exact B canonical on original nodes")
	require(hx(raw(minedTx(ctx, a, winner.WorkHash, originalA.TxID))) == originalA.Raw, "A mined unchanged in B")
	canonicalAt := time.Now().UnixMilli()
	receipt := dailyActualSubmit(ctx, owners.base, r.Original.Accounts[1], workB, winner.Nonce)
	require(receipt.Sequence == 3 && receipt.AcceptedSequence == 3 && receipt.Block && receipt.BlockHeight == 103 && receipt.Acceptance == "canonical-late-winner" && receipt.BlockState == "canonical-observed" && receipt.WorkHash == winner.WorkHash && receipt.ReceivedAtMS >= workB.Job.ExpiresAtMS, "one original canonical late-winner receipt")
	_, e = owners.base.rounds.Sync(ctx)
	check(e, "full original atomic round catchup")
	tail, e := owners.base.source.AccountingPage(ctx, 0, 0, 8)
	check(e, "complete original accepted tail")
	require(tail.Config == r.Original.Binding.Source && tail.Through == 3 && len(tail.Entries) == 3, "all original accepted rows")
	snap = dailyActualSnapshot(ctx, owners.base)
	dailyAdmissionOriginalA(snap, priorA, 1, 1, originalA.State)
	be := dailyAdmissionEvidence(ctx, owners)
	require(be.BodySHA256 == ae.BodySHA256 && be.BodyBytes == ae.BodyBytes, "original immutable b1 before child")
	priorB := dailyAdmissionPhaseB{Work: workB, Receipt: receipt, Ordinary: ordinary, Status: dailyAdmissionStatus(ctx, owners, 3, 3, 2), BeforeStatus: before, Tail: tail, Snapshot: snap, Evidence: be, ExpiryObserved: expiryAt, CanonicalObserved: canonicalAt, AbsentBefore: true}
	write(filepath.Join(output, "02-B-pending.json"), priorB)
	check(owners.close(), "all original parent owners closed before fresh child")
	phase = "fresh process exact receipt and pending A recovery"
	child = dailyAdmissionLaunch(ctx, root, reference, output, relay.url, a.rpc.endpoint, b.rpc.endpoint, day)
	check(child.startError, "fresh original recovery process start")
	child.wait(ctx)
	require(child.clean(), "fresh recovery process clean join")
	result := dailyActualReadRecoveryResult(child, output)
	var complete dailyAdmissionComplete
	dailyActualDecodePublic(filepath.Join(output, "03-complete.json"), result.CompleteSHA, dailyActualProcessLimit, &complete)
	require(complete.Height == 163 && complete.SourceID == r.Original.SourceID && complete.ConfigID == r.Original.ConfigID && complete.Evidence.BodySHA256 == ae.BodySHA256 && reflect.DeepEqual(complete.Receipt, receipt), "same selected child completion evidence")
	dailyAdmissionReport(complete.Report, r, "b60_same_day", day)
	attempts, overflow := relay.attempted()
	sends := relay.sends()
	require(!overflow && len(attempts) == 1 && len(sends) == 1 && attempts[0].Raw == originalA.Raw && sends[0].Raw == originalA.Raw && sends[0].TxID == originalA.TxID, "one original A send and zero recovery sends")
	require(ctx.Err() == nil, "original overall deadline")
	out["complete"] = complete
	out["day"] = day
	out["completed"] = true
	return out
}
