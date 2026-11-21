//go:build slugdailyactual && !slugtestpow && (linux || (darwin && cgo))

package poolsettle_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/taytr-llc/slug/btcutil/v2/hdkeychain"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/internal/operatorauthority"
	"github.com/taytr-llc/slug/poolround"
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/poolsettlefixture"
	"github.com/taytr-llc/slug/poolsettlenode"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectwork"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
	"github.com/taytr-llc/slugwallet/recovery"
	"github.com/taytr-llc/slugwallet/waddrmgr"
	"github.com/taytr-llc/slugwallet/wallet"
	_ "github.com/taytr-llc/slugwallet/walletdb/bdb"
)

const dailyActualReferenceSHA = "9bcb4b64505e38de193411dcf51fb19f4d1332cbb2af53d21904663848cb4955"
const dailyActualPublicPass = "published pool wallet public passphrase"
const dailyActualPrivatePass = "published pool wallet private passphrase"

type dailyActualAccount struct {
	Index     int    `json:"index"`
	Scalar    int    `json:"published_scalar"`
	PublicKey string `json:"public_key"`
	GymID     string `json:"gym_id"`
	Script    string `json:"payee_script"`
	Token     string `json:"public_fixture_token"`
}
type dailyActualPayment struct {
	GymID  string `json:"gym_id"`
	Script string `json:"script"`
	First  int64  `json:"first_atoms"`
	Second int64  `json:"second_atoms"`
	Total  int64  `json:"total_atoms"`
}
type dailyActualReference struct {
	Authority   registry.PublicManifestPin `json:"authority"`
	Binding     wallet.PoolWalletBinding   `json:"binding"`
	Limits      poolround.Limits           `json:"round_limits"`
	Node        poolsettlenode.Config      `json:"node"`
	SourceID    string                     `json:"source_id"`
	ConfigID    string                     `json:"config_id"`
	GuardHex    string                     `json:"guard_hex"`
	GuardDigest string                     `json:"guard_digest"`
	Accounts    []dailyActualAccount       `json:"accounts"`
	Payments    []dailyActualPayment       `json:"payments"`
}

func dailyActualReadReference(path string) dailyActualReference {
	require(filepath.IsAbs(path), "absolute public reference")
	f, e := os.Open(path)
	check(e, "public reference open")
	st, e := f.Stat()
	check(e, "public reference stat")
	require(st.Mode().IsRegular() && st.Size() == 85070, "bounded reference size")
	b := make([]byte, st.Size())
	_, e = f.ReadAt(b, 0)
	ce := f.Close()
	check(errors.Join(e, ce), "public reference read/close")
	require(hx(digest(b)) == dailyActualReferenceSHA, "independent reference identity")
	var r dailyActualReference
	check(json.Unmarshal(b, &r), "reference decode")
	require(len(r.Accounts) == 64 && len(r.Payments) == 64, "64 independently modeled accounts")
	check(wallet.ValidateDailyPoolWalletBinding(r.Binding), "original independent wallet binding")
	sid, e := poolshare.DailyConfigID(r.Binding.Source)
	check(e, "daily source ID")
	require(sid == r.SourceID, "independent source ID")
	cid, e := poolsettle.DailyConfigBindingID(r.Binding.Settlement)
	check(e, "daily config ID")
	require(cid == r.ConfigID, "independent config ID")
	guard, e := waddrmgr.MarshalPoolSettlementGuard(r.Binding.Guard)
	check(e, "guard bytes")
	require(hx(guard) == r.GuardHex, "independent unchanged guard bytes")
	gid, e := waddrmgr.PoolSettlementGuardDigest(r.Binding.Guard)
	check(e, "guard ID")
	require(gid == r.GuardDigest, "independent guard ID")
	for i, a := range r.Accounts {
		require(a.Index == i && a.Scalar == i+1 && len(a.PublicKey) == 66 && len(a.Token) == 64, "published fixture account")
	}
	return r
}

// Only authentication is a fixture. All work, receipts, rounds, coins, signatures
// and node evidence below are produced by their original concrete owners.
type dailyActualSessions struct {
	life   context.Context
	cancel context.CancelFunc
	mu     sync.Mutex
	closed atomic.Bool
	calls  sync.WaitGroup
	keys   map[string]string
}

func dailyActualNewSessions(life context.Context, accounts []dailyActualAccount) *dailyActualSessions {
	life, cancel := context.WithCancel(life)
	s := &dailyActualSessions{life: life, cancel: cancel, keys: make(map[string]string)}
	for _, a := range accounts {
		require(s.keys[a.Token] == "", "unique frozen tokens")
		s.keys[a.Token] = a.PublicKey
	}
	return s
}
func (s *dailyActualSessions) WithSession(ctx context.Context, token string, fn func(string, func() error) error) error {
	if ctx == nil || fn == nil {
		return errors.New("fixture session invalid")
	}
	s.mu.Lock()
	if s.closed.Load() {
		s.mu.Unlock()
		return context.Canceled
	}
	key, ok := s.keys[token]
	if !ok {
		s.mu.Unlock()
		return errors.New("fixture session refused")
	}
	s.calls.Add(1)
	s.mu.Unlock()
	defer s.calls.Done()
	check := func() error {
		if e := ctx.Err(); e != nil {
			return e
		}
		if s.closed.Load() {
			return context.Canceled
		}
		return s.life.Err()
	}
	if e := check(); e != nil {
		return e
	}
	e := fn(key, check)
	if e != nil {
		return e
	}
	return check()
}
func (s *dailyActualSessions) close() {
	s.mu.Lock()
	s.closed.Store(true)
	s.cancel()
	s.mu.Unlock()
	s.calls.Wait()
}

type dailyActualOwners struct {
	authority   *operatorauthority.Owner
	issuer      *registry.ManifestStore
	node        *prospectnode.Client
	source      *poolshare.Pool
	rounds      *poolround.Store
	custody     *wallet.PoolWalletOwner
	evidence    *poolsettlenode.Client
	backend     *wallet.PoolSettlementBackend
	ledger      *poolsettle.Store
	sessions    *dailyActualSessions
	coldSeconds float64
}

func (o *dailyActualOwners) close() error {
	// On a failed close, retain that original owner and its ancestors.
	if o.ledger != nil {
		if e := o.ledger.Close(); e != nil {
			return e
		}
		o.ledger = nil
	}
	if o.backend != nil {
		if e := o.backend.Close(); e != nil {
			return e
		}
		o.backend = nil
	}
	if o.evidence != nil {
		if e := o.evidence.Close(); e != nil {
			return e
		}
		o.evidence = nil
	}
	if o.custody != nil {
		if e := o.custody.Close(); e != nil {
			return e
		}
		o.custody = nil
	}
	if o.rounds != nil {
		if e := o.rounds.Close(); e != nil {
			return e
		}
		o.rounds = nil
	}
	if o.source != nil {
		if e := o.source.Close(); e != nil {
			return e
		}
		o.source = nil
	}
	if o.node != nil {
		o.node.Close()
		o.node = nil
	}
	if o.sessions != nil {
		o.sessions.close()
		o.sessions = nil
	}
	if o.issuer != nil {
		if e := o.issuer.Close(); e != nil {
			return e
		}
		o.issuer = nil
	}
	if o.authority != nil {
		if e := o.authority.Close(); e != nil {
			return e
		}
		o.authority = nil
	}
	return nil
}
func dailyActualOpen(ctx context.Context, root string, r dailyActualReference, rpc prospectnode.RPCConfig, create bool, o *dailyActualOwners) {
	profile, e := registry.NewOperatorProfile(r.Authority)
	check(e, "operator profile")
	require(profile.ID() == r.Binding.OperatorProfileID, "independent profile identity")
	genesis, e := chainhash.NewHashFromStr(r.Authority.Source.Genesis)
	check(e, "genesis")
	binding := operatorauthority.Binding{Version: 2, PinID: profile.ID(), Network: wire.BitcoinNet(r.Authority.Source.Network), Genesis: *genesis, Keys: operatorauthority.Keys{Admission: r.Authority.AdmissionKey, Publication: r.Authority.PublicationKey, Cutoff: r.Authority.CutoffKey, Checkpoint: r.Authority.CheckpointKey}}
	seeds := operatorauthority.OnlineSeedFiles{Admission: filepath.Join(root, "admission.seed"), Publication: filepath.Join(root, "publication.seed"), Cutoff: filepath.Join(root, "cutoff.seed")}
	if create {
		for _, role := range []string{"admission", "publication", "cutoff"} {
			seed := sha256.Sum256([]byte("SLUG set74 public synthetic operator " + role + "\x00v1\x00"))
			check(os.WriteFile(filepath.Join(root, role+".seed"), seed[:], 0600), "public synthetic role fixture")
			clear(seed[:])
		}
	}
	o.authority, e = operatorauthority.OpenOnline(ctx, binding, seeds)
	check(e, "online original authority")
	require(!o.authority.Capabilities().Checkpoint, "no online checkpoint custody")
	if create {
		d, x := fighterdescriptor.ResolveCandidate(r.Authority.Source.DescriptorID)
		check(x, "original descriptor")
		o.issuer, e = registry.CreateOperatorManifestAuthority(filepath.Join(root, "issuer"), d, profile, o.authority)
	} else {
		o.issuer, e = registry.OpenOperatorManifestAuthority(filepath.Join(root, "issuer"), profile, o.authority)
	}
	check(e, "original issuer")
	o.node, e = prospectnode.NewClient(rpc)
	check(e, "original normal node adapter")
	o.sessions = dailyActualNewSessions(ctx, r.Accounts)
	if create {
		o.source, e = poolshare.CreateDaily(ctx, filepath.Join(root, "source"), r.Binding.Source, o.issuer, o.node, o.sessions)
	} else {
		cold, cancel := context.WithTimeout(ctx, 5*time.Minute)
		start := time.Now()
		o.source, e = poolshare.OpenDaily(cold, filepath.Join(root, "source"), r.Binding.Source, o.issuer, o.node, o.sessions)
		o.coldSeconds = time.Since(start).Seconds()
		cancel()
	}
	check(e, "original daily source")
	if create {
		o.rounds, e = poolround.CreateDaily(ctx, filepath.Join(root, "rounds"), r.Limits, o.source, o.node)
	} else {
		o.rounds, e = poolround.OpenDaily(ctx, filepath.Join(root, "rounds"), r.Limits, o.source, o.node)
	}
	check(e, "original daily rounds")
	if create {
		seed := recovery.SeedFromEntropy([32]byte{})
		master, x := hdkeychain.NewMaster(seed[:], params)
		clear(seed[:])
		check(x, "published zero256 root")
		o.custody, e = wallet.CreateDailyPoolSettlementWallet(ctx, filepath.Join(root, "wallet16"), r.Binding, []byte(dailyActualPublicPass), []byte(dailyActualPrivatePass), master, time.Now().Add(-24*time.Hour))
		master.Zero()
	} else {
		o.custody, e = wallet.OpenDailyPoolSettlementWallet(ctx, filepath.Join(root, "wallet16"), r.Binding, []byte(dailyActualPublicPass))
	}
	check(e, "original schema16 daily custody")
	o.evidence, e = poolsettlenode.NewDaily(ctx, r.Node, rpc)
	check(e, "original daily node evidence")
	o.backend, e = wallet.NewDailyPoolSettlementBackend(ctx, o.custody, o.evidence, r.Binding)
	check(e, "original daily backend")
	if create {
		o.ledger, e = poolsettle.CreateDaily(ctx, filepath.Join(root, "settlement"), r.Binding.Settlement, o.rounds, o.backend)
	} else {
		o.ledger, e = poolsettle.OpenDaily(ctx, filepath.Join(root, "settlement"), r.Binding.Settlement, o.rounds, o.backend)
	}
	check(e, "original daily ledger")
	check(o.backend.BindDailySettlement(ctx, o.ledger), "one original backend binding")
	// This explicit fixture startup unlock is outside Sign and below the24h cap.
	check(o.custody.Unlock(ctx, []byte(dailyActualPrivatePass), time.Hour), "explicit original wallet unlock")
}
func dailyActualPause(ctx context.Context) {
	select {
	case <-ctx.Done():
		panic("original operation deadline")
	case <-time.After(100 * time.Millisecond):
	}
}
func dailyActualIssue(ctx context.Context, o *dailyActualOwners, a dailyActualAccount) poolshare.MiningWork {
	op, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	for {
		w, e := o.source.IssueMiningWork(op, a.Token)
		if e == nil {
			return w
		}
		if e != poolshare.ErrBusy {
			check(e, "original Issue")
		}
		dailyActualPause(op)
	}
}
func dailyActualSubmit(ctx context.Context, o *dailyActualOwners, a dailyActualAccount, w poolshare.MiningWork, nonce uint32) poolshare.Submission {
	op, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	for {
		v, e := o.source.Submit(op, a.Token, w.Job.ID, nonce)
		if e == nil {
			return v
		}
		if e != poolshare.ErrBusy {
			check(e, "original Submit")
		}
		dailyActualPause(op)
	}
}

type dailyActualReward struct {
	Receipt  poolshare.Submission `json:"receipt"`
	Coin     wire.OutPoint        `json:"coin"`
	GymID    string               `json:"gym_id"`
	Searched uint32               `json:"searched"`
}

func dailyActualWinner(ctx context.Context, o *dailyActualOwners, r dailyActualReference, a, b *node, index int, height uint32) dailyActualReward {
	account := r.Accounts[0]
	if index > 0 {
		account = r.Accounts[(index-1)%64]
	}
	work := dailyActualIssue(ctx, o, account)
	require(work.Job.PublicKey == account.PublicKey && work.Job.GymID == account.GymID && work.Job.Context.ParentHeight+1 == height, "original issued work correlation")
	prepared, e := prospectwork.PrepareManifestWork(work.Job, work.Template, r.Binding.Source.PayoutAddress, 0, r.Binding.Source.PoolTarget)
	check(e, "original candidate")
	hashing, cancel := context.WithTimeout(ctx, 4*time.Minute)
	defer cancel()
	var winner prospectwork.ShareCheck
	var searched uint32
	for nonce := uint32(0); nonce < 64; nonce++ {
		var c prospectwork.ShareCheck
		for {
			c, e = prepared.CheckNonce(hashing, nonce)
			if e == nil {
				break
			}
			if e != prospectwork.ErrShareBusy {
				check(e, "normal CheckNonce")
			}
			dailyActualPause(hashing)
		}
		searched++
		if c.Block {
			winner = c
			break
		}
	}
	require(winner.Block && winner.Candidate != nil, "bounded normal-PoW winner")
	var reply any
	a.rpc.must(ctx, "submitblock", []any{winner.Candidate.BlockHex}, &reply)
	require(reply == nil && sameTip(ctx, a, b) == winner.WorkHash, "exact canonical winner")
	var hash string
	a.rpc.must(ctx, "getblockhash", []any{height}, &hash)
	require(hash == winner.WorkHash, "exact original height")
	canonical := block(ctx, a, hash)
	var serialized bytes.Buffer
	check(canonical.Serialize(&serialized), "canonical serialization")
	require(hx(serialized.Bytes()) == winner.Candidate.BlockHex, "canonical bytes unchanged")
	var coin wire.OutPoint
	found := false
	for i, out := range canonical.Transactions[0].TxOut {
		if hx(out.PkScript) == r.Binding.Guard.FundingScript {
			require(!found && out.Value == 1000000000, "independent pool reward")
			coin = wire.OutPoint{Hash: canonical.Transactions[0].TxHash(), Index: uint32(i)}
			found = true
		}
	}
	require(found, "original payout script")
	receipt := dailyActualSubmit(ctx, o, account, work, winner.Nonce)
	require(receipt.Sequence == uint64(index+1) && receipt.AcceptedSequence == uint64(index+1) && receipt.Block && receipt.BlockState == "canonical-observed" && receipt.WorkHash == hash && receipt.BlockHeight == height, "exact retained winner row")
	return dailyActualReward{Receipt: receipt, Coin: coin, GymID: account.GymID, Searched: searched}
}
func dailyActualMature(ctx context.Context, a, b *node, height uint32) {
	for {
		var got uint32
		a.rpc.must(ctx, "getblockcount", nil, &got)
		require(got <= height, "no extra block")
		if got == height {
			return
		}
		generate(ctx, a, b)
	}
}
func dailyActualRounds(ctx context.Context, o *dailyActualOwners, rewards []dailyActualReward) {
	op, cancel := context.WithTimeout(ctx, 30*time.Second)
	_, e := o.rounds.Sync(op)
	cancel()
	check(e, "original streaming Sync")
	for i, w := range rewards {
		op, cancel := context.WithTimeout(ctx, 30*time.Second)
		record, e := o.rounds.Refresh(op, w.Receipt.WorkHash)
		cancel()
		check(e, "canonical round confirmation")
		require(record.Confirmation != nil && record.Round.Sequence == uint64(i+1) && record.Round.FirstAccepted == uint64(i+1) && record.Round.LastAccepted == uint64(i+1) && record.Round.RewardAtoms == 1000000000 && len(record.Round.ShareKeys) == 1 && len(record.Round.Allocations) == 1 && record.Round.Allocations[0].GymID == w.GymID && record.Round.Allocations[0].Numerator == "1000000000" && record.Round.Allocations[0].Denominator == "1", "single original winner round")
	}
}
func dailyActualSnapshot(ctx context.Context, o *dailyActualOwners) poolsettle.DailyActualState {
	op, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	s, e := poolsettle.DailyActualSnapshot(op, o.ledger)
	check(e, "original complete daily audit")
	return s
}
func dailyActualRun(ctx context.Context, o *dailyActualOwners, recoveryOnly bool) poolsettle.Report {
	op, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	var r poolsettle.Report
	var e error
	if recoveryOnly {
		r, e = o.ledger.RecoverPending(op)
	} else {
		r, e = o.ledger.RunDaily(op)
	}
	check(e, "original daily operation")
	return r
}
func dailyActualSame(got, want poolsettle.Batch) {
	got.State = want.State
	require(reflect.DeepEqual(got, want), "original complete economic batch unchanged")
}
func dailyActualCheckBatch(batch poolsettle.Batch, r dailyActualReference, rewards []dailyActualReward, first bool) {
	tx := wire.NewMsgTx(2)
	check(tx.Deserialize(bytes.NewReader(dec(batch.Raw))), "signed transaction parse")
	require(tx.TxHash().String() == batch.TxID, "exact signed txid")
	var wanted []poolsettle.Payment
	var fee, change int64
	var inputs int
	if first {
		wanted = []poolsettle.Payment{{GymID: r.Accounts[0].GymID, Script: r.Accounts[0].Script, Atoms: 990000000}}
		fee, change, inputs = 141, 9999859, 1
	} else {
		for _, p := range r.Payments {
			wanted = append(wanted, poolsettle.Payment{GymID: p.GymID, Script: p.Script, Atoms: p.Second})
		}
		fee, change, inputs = 51236, 7209948623, 721
	}
	require(reflect.DeepEqual(batch.Payments, wanted) && batch.FeeAtoms == fee && len(batch.Inputs) == inputs && len(tx.TxIn) == inputs && batch.Change != nil && batch.Change.Atoms == change && batch.Change.Script == r.Binding.Guard.FundingScript, "independent full batch shape/economics")
	require(len(tx.TxOut) == len(wanted)+1, "no unsolicited outputs")
	for i, p := range wanted {
		require(tx.TxOut[i].Value == p.Atoms && hx(tx.TxOut[i].PkScript) == p.Script, "all immutable payee outputs")
	}
	require(tx.TxOut[len(wanted)].Value == change && hx(tx.TxOut[len(wanted)].PkScript) == r.Binding.Guard.FundingScript, "exact operator change")
	var total int64
	for i, c := range batch.Inputs {
		if first || i > 0 {
			reward := rewards[i]
			require(c.Coinbase && c.TxID == reward.Coin.Hash.String() && c.Index == reward.Coin.Index && c.Atoms == 1000000000 && c.Script == r.Binding.Guard.FundingScript && c.BlockHash == reward.Receipt.WorkHash && c.BlockHeight == reward.Receipt.BlockHeight, "exact independently observed reward input")
			claims := map[string]poolsettle.Fraction{"": {Numerator: "10000000", Denominator: "1"}, reward.GymID: {Numerator: "990000000", Denominator: "1"}}
			require(len(c.Claims) == 2, "reward original claim count")
			for _, claim := range c.Claims {
				want, ok := claims[claim.GymID]
				require(ok && claim.RoundHash == reward.Receipt.WorkHash && claim.RoundHeight == reward.Receipt.BlockHeight && claim.Amount == want, "independent original reward allocation")
				delete(claims, claim.GymID)
			}
			require(len(claims) == 0, "no omitted original claim")
		}
		total += c.Atoms
		require(tx.TxIn[i].PreviousOutPoint.Hash.String() == c.TxID && tx.TxIn[i].PreviousOutPoint.Index == c.Index, "exact input outpoint")
	}
	for _, o := range tx.TxOut {
		total -= o.Value
	}
	require(total == fee, "actual fee equals independent quote")
	count := 1
	if !first {
		count = 721
	}
	require(len(batch.Change.Claims) == count, "all carried origins")
	for i, c := range batch.Change.Claims {
		want := int64(10000000)
		if i == 0 {
			want = 9999859
			if !first {
				want -= 51236
			}
		}
		require(c.GymID == "" && c.RoundHash == rewards[i].Receipt.WorkHash && c.RoundHeight == rewards[i].Receipt.BlockHeight && c.Amount == (poolsettle.Fraction{Numerator: strconv.FormatInt(want, 10), Denominator: "1"}), "independent operator-only original claim")
	}
}
func dailyActualFinalBalances(report poolsettle.Report, r dailyActualReference) {
	require(len(report.Balances) == 64, "all64 balances")
	for i, b := range report.Balances {
		p := r.Payments[i]
		require(b.GymID == p.GymID && b.PaidAtoms == p.Total && b.ReservedAtoms == 0 && b.Confirmed == (poolsettle.Fraction{Numerator: "0", Denominator: "1"}), "exact all-gym paid once")
	}
}

func TestDailyActual721Inputs64Gyms(t *testing.T) {
	if os.Getenv("SLUG_DAILY_ACTUAL") != "1" || os.Getenv("SLUG_DAILY_REOPEN_CHILD") != "" {
		t.Skip("explicit isolated actual opt-in")
	}
	// The outer process owner retains this exact fresh tree on every result.
	// t.TempDir would remove it even after uncertain custody/node cleanup.
	root := os.Getenv("SLUG_DAILY_ROOT")
	if !filepath.IsAbs(root) || filepath.Clean(root) != root || root == string(os.PathSeparator) {
		t.Fatal("explicit fresh absolute fixture root required")
	}
	parent, e := filepath.EvalSymlinks(filepath.Dir(root))
	if e != nil || parent != filepath.Dir(root) {
		t.Fatal("canonical existing fixture parent required")
	}
	if e = os.Mkdir(root, 0700); e != nil {
		t.Fatal("fixture root must not exist")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	out := dailyActualRunAll(ctx, root, os.Getenv("SLUG_DAILY_NODE"), os.Getenv("SLUG_DAILY_REFERENCE"), os.Getenv("SLUG_DAILY_OUTPUT"))
	if out["pass"] != true {
		t.Fatalf("actual failed: phase=%v failure=%v cleanup=%v", out["phase"], out["failure"], out["cleanup_errors"])
	}
}
func dailyActualRunAll(ctx context.Context, root, binary, reference, output string) (out map[string]any) {
	out = map[string]any{"version": 1, "pass": false, "scope": "fresh source2/round2/daily-ledger/schema16/normal-node with separate-process original-owner reopen; fixture authentication; settlement-only test clock; no public service or scheduler claim"}
	start := time.Now()
	var nodes []*node
	var relay *rpcRelay
	var recoveryProcess *dailyActualRecoveryProcess
	owners := &dailyActualOwners{}
	defer func() {
		if v := recover(); v != nil {
			out["failure"] = fmt.Sprintf("%.1024s", fmt.Sprint(v))
			out["phase"] = phase
		}
		cleanup := []string{}
		childJoined := true
		if recoveryProcess != nil {
			recoveryProcess.stop()
			out["recovery_process"] = recoveryProcess.metadata()
			childJoined = !recoveryProcess.started || recoveryProcess.joined
			if !recoveryProcess.clean() {
				cleanup = append(cleanup, "recovery child cleanup")
			}
		}
		if e := owners.close(); e != nil {
			cleanup = append(cleanup, fmt.Sprintf("owners: %.256s", e))
		}
		if relay != nil && childJoined {
			if e := relay.close(); e != nil {
				cleanup = append(cleanup, fmt.Sprintf("relay: %.256s", e))
			}
			out["broadcasts"] = relay.sends()
			at, overflow := relay.attempted()
			out["broadcast_attempts"] = at
			out["broadcast_overflow"] = overflow
			if overflow {
				cleanup = append(cleanup, "relay attempt bound")
			}
		}
		rows := []any{}
		for i := len(nodes) - 1; i >= 0; i-- {
			if childJoined {
				nodes[i].stop()
			}
			rows = append(rows, nodes[i].cleanupReport())
			if !nodes[i].clean() {
				cleanup = append(cleanup, "node join")
			}
		}
		out["node_cleanup"] = rows
		out["cleanup_errors"] = cleanup
		out["elapsed_seconds"] = time.Since(start).Seconds()
		out["pass"] = out["completed"] == true && len(cleanup) == 0
		if filepath.IsAbs(output) {
			write(filepath.Join(output, "actual-report.json"), out)
		}
	}()
	require(filepath.IsAbs(binary) && filepath.IsAbs(output), "explicit node/output paths")
	st, e := os.Stat(output)
	check(e, "existing output directory")
	require(st.IsDir(), "output directory")
	r := dailyActualReadReference(reference)
	require(!wire.TestPoW, "normal PoW")
	bootstrap := poolsettlefixture.PoolAddress()
	require(bootstrap != r.Binding.Source.PayoutAddress, "uncredited bootstrap script")
	p1, p2 := port(), port()
	require(p1 != p2, "distinct peer ports")
	phase = "two original normal nodes"
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
	rpc := prospectnode.RPCConfig{Version: 1, URL: relay.url, Username: "fixture", Password: "public-pool-actual"}
	owned := filepath.Join(root, "owners")
	check(os.Mkdir(owned, 0700), "fresh owner root")
	dailyActualOpen(ctx, owned, r, rpc, true, owners)
	dayA := time.Now().UTC()
	check(poolsettle.DailyActualClock(ctx, owners.ledger, dayA), "test-only day A")
	phase = "first original winner and mature A"
	rewards := []dailyActualReward{dailyActualWinner(ctx, owners, r, a, b, 0, 2)}
	dailyActualMature(ctx, a, b, 102)
	dailyActualRounds(ctx, owners, rewards)
	report := dailyActualRun(ctx, owners, false)
	require(report.State == "pending" && report.Reason == "payout-pending", "A broadcast")
	snap := dailyActualSnapshot(ctx, owners)
	require(snap.Credits == 1 && len(snap.Batches) == 1 && snap.Pending == 1, "A exact retained ledger")
	originalA := snap.Batches[0]
	dailyActualCheckBatch(originalA, r, rewards, true)
	ahash := generate(ctx, a, b)
	require(hx(raw(minedTx(ctx, a, ahash, originalA.TxID))) == originalA.Raw, "A canonical exact bytes")
	require(dailyActualRun(ctx, owners, true).State == "complete", "A confirmed recovery")
	write(filepath.Join(output, "01-A.json"), map[string]any{"batch": originalA, "reward": rewards[0], "canonical_block": ahash})
	phase = "720 additional canonical original winners"
	for i := 1; i <= 720; i++ {
		rewards = append(rewards, dailyActualWinner(ctx, owners, r, a, b, i, uint32(103+i)))
		if i%32 == 0 {
			fmt.Printf("daily actual: %d/720 additional canonical winners\n", i)
		}
	}
	dailyActualMature(ctx, a, b, 923)
	info, e := owners.issuer.Info()
	check(e, "original issuer counts")
	require(info.Jobs == 721 && info.Records == 0, "721 original issuer jobs and no births")
	tail, e := owners.source.AccountingPage(ctx, 0, 0, 1)
	check(e, "exact source tail")
	require(tail.Through == 721 && tail.Config == r.Binding.Source, "721 actual accepted rows")
	before := time.Now()
	dailyActualRounds(ctx, owners, rewards)
	out["round_sync_confirm_seconds"] = time.Since(before).Seconds()
	phase = "721-input64-payee B and post-forward response loss"
	dayB := dayA.Add(24 * time.Hour)
	check(poolsettle.DailyActualClock(ctx, owners.ledger, dayB), "test-only day B")
	relay.mu.Lock()
	require(!relay.drop && !relay.hold, "relay original unarmed state")
	relay.drop = true
	relay.mu.Unlock()
	before = time.Now()
	report = dailyActualRun(ctx, owners, false)
	out["B_run_seconds"] = time.Since(before).Seconds()
	require(report.State == "pending" && report.Reason == "broadcast-unknown", "B actual post-forward loss")
	snap = dailyActualSnapshot(ctx, owners)
	require(snap.Credits == 721 && len(snap.Batches) == 2 && snap.Pending == 2, "B exact original ledger")
	originalB := snap.Batches[1]
	dailyActualSame(snap.Batches[0], originalA)
	dailyActualCheckBatch(originalB, r, rewards, false)
	require(reflect.DeepEqual(originalB.Inputs[0], *originalA.Change), "B spends exact original A carry")
	require(len(report.Balances) == 64, "all original reserved payees")
	for i, balance := range report.Balances {
		p := r.Payments[i]
		require(balance.GymID == p.GymID && balance.PaidAtoms == p.First && balance.ReservedAtoms == p.Second && balance.Confirmed == (poolsettle.Fraction{Numerator: strconv.FormatInt(p.Second, 10), Denominator: "1"}), "full B reserved once while original A remains paid")
	}
	select {
	case lost := <-relay.lost:
		require(lost.Raw == originalB.Raw && lost.TxID == originalB.TxID, "real forwarded B exact bytes")
	default:
		panic("lost reply not observed")
	}
	sends := relay.sends()
	require(len(sends) == 2 && sends[0].Raw == originalA.Raw && sends[1].Raw == originalB.Raw, "exactly original A/B sends")
	write(filepath.Join(output, "02-B-pending.json"), map[string]any{"batch": originalB, "rewards": rewards, "report": report})
	phase = "separate-process reopen and recovery of all original owners"
	check(owners.close(), "full original owner close before child")
	var child dailyActualRecoveryResult
	recoveryProcess = dailyActualLaunchRecovery(ctx, root, reference, output, rpc.URL, a.rpc.endpoint, b.rpc.endpoint, dayB)
	check(recoveryProcess.startError, "recovery child start")
	recoveryProcess.wait(ctx)
	require(recoveryProcess.clean(), "recovery child exited and joined cleanly")
	child = dailyActualReadRecoveryResult(recoveryProcess, output)
	complete := dailyActualReadComplete(output, child.CompleteSHA)
	dailyActualSame(complete.Snapshot.Batches[0], originalA)
	dailyActualSame(complete.Snapshot.Batches[1], originalB)
	dailyActualFinalBalances(complete.Report, r)
	require(complete.SourceID == r.SourceID && complete.ConfigID == r.ConfigID && complete.GuardDigest == r.GuardDigest, "child original public identities")
	sends = relay.sends()
	require(len(sends) == 2 && sends[0].Raw == originalA.Raw && sends[1].Raw == originalB.Raw, "child recovered without another broadcast")
	attempts, overflow := relay.attempted()
	require(!overflow && len(attempts) == 2 && attempts[0].Raw == originalA.Raw && attempts[1].Raw == originalB.Raw, "exactly original two attempted sends")
	a.rpc.must(ctx, "getblockcount", nil, &height)
	require(height == 924 && complete.Height == height, "final exact block budget")
	sameTip(ctx, a, b)
	peers(ctx, a)
	peers(ctx, b)
	out["cold_source_seconds"] = child.ColdSourceSeconds
	out["final_change_inspect_seconds"] = child.FinalInspectSeconds
	out["height"] = height
	out["inputs"] = 721
	out["payees"] = 64
	out["change_origins"] = len(complete.Snapshot.Frontier[0].Claims)
	out["batches"] = 2
	out["reopens"] = 1
	out["completed"] = true
	return out
}
