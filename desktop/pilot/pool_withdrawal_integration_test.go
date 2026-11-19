//go:build desktopintegration && !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/taytr-llc/slug/btcjson"
	"github.com/taytr-llc/slug/btcutil/v2"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/poolsettlefixture"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/walletwithdraw"
	"github.com/taytr-llc/slugwallet/recovery"
)

type withdrawalActualCase struct {
	Name     string                 `json:"name"`
	Fixture  string                 `json:"fixture"`
	Request  walletwithdraw.Request `json:"request"`
	Draft    walletwithdraw.Draft   `json:"draft"`
	Expected struct {
		TxID string `json:"txid"`
		Raw  string `json:"raw_hex"`
	} `json:"expected_signed"`
	SignedSHA256 string `json:"signed_sha256"`
}
type withdrawalActualSend struct {
	Raw     string `json:"raw"`
	Reply   string `json:"reply"`
	Dropped bool   `json:"dropped"`
}
type withdrawalActualRecorder struct {
	mu    sync.Mutex
	drop  bool
	sends []withdrawalActualSend
}
type withdrawalActualTransport struct {
	base   http.RoundTripper
	record *withdrawalActualRecorder
}

func (x *withdrawalActualTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	raw, e := io.ReadAll(io.LimitReader(r.Body, 16385))
	if e != nil || len(raw) > 16384 {
		return nil, errors.New("actual bounded request")
	}
	r.Body = io.NopCloser(bytes.NewReader(raw))
	var request struct {
		Method string            `json:"method"`
		Params []json.RawMessage `json:"params"`
	}
	if json.Unmarshal(raw, &request) != nil {
		return nil, errors.New("actual malformed request")
	}
	response, e := x.base.RoundTrip(r)
	if request.Method != "sendrawtransaction" || e != nil {
		return response, e
	}
	var signed string
	if len(request.Params) != 2 || json.Unmarshal(request.Params[0], &signed) != nil {
		response.Body.Close()
		return nil, errors.New("actual malformed send")
	}
	body, e := io.ReadAll(io.LimitReader(response.Body, 65537))
	response.Body.Close()
	if e != nil || len(body) > 65536 {
		return nil, errors.New("actual bounded reply")
	}
	x.record.mu.Lock()
	drop := x.record.drop
	x.record.drop = false
	x.record.sends = append(x.record.sends, withdrawalActualSend{signed, string(body), drop})
	x.record.mu.Unlock()
	if drop {
		return nil, errors.New("fixture lost response after owned node processed request")
	}
	response.Body = io.NopCloser(bytes.NewReader(body))
	return response, nil
}
func (x *withdrawalActualTransport) CloseIdleConnections() {
	if b, ok := x.base.(interface{ CloseIdleConnections() }); ok {
		b.CloseIdleConnections()
	}
}
func (x *withdrawalActualRecorder) snapshot() []withdrawalActualSend {
	x.mu.Lock()
	defer x.mu.Unlock()
	return append([]withdrawalActualSend(nil), x.sends...)
}

type withdrawalActualWallet struct {
	fixture, subject, path string
	login                  gymaccount.Session
	account                gymaccount.Account
	binding                PoolRecipientBinding
	scope                  poolWithdrawalScope
	wallet                 *walletRuntime
	resource               *poolWithdrawalResources
	want                   withdrawalActualCase
	record                 *withdrawalActualRecorder
}

func (x *withdrawalActualWallet) close(t *testing.T) {
	t.Helper()
	if x.resource != nil {
		if e := x.resource.Close(); e != nil {
			t.Error(e)
		}
		x.resource = nil
	}
	if x.wallet != nil {
		if e := x.wallet.Close(); e != nil {
			t.Error(e)
		}
		x.wallet = nil
	}
}
func withdrawalActualID(label string) string {
	sum := sha256.Sum256([]byte("Set68 public actual action: " + label))
	return hex.EncodeToString(sum[:])
}
func withdrawalActualFence(ctx context.Context, accounts *gymaccount.Store, login gymaccount.Session, expected gymaccount.Account, cap gymaccount.Capability) walletwithdraw.Fence {
	return func(fn func(func() error) error) error {
		return accounts.WithSession(ctx, login.Token, cap, func(a gymaccount.Account, check func() error) error {
			if a.ID != expected.ID || a.Generation != expected.Generation || a.Enrollment == nil || expected.Enrollment == nil || *a.Enrollment != *expected.Enrollment {
				return gymaccount.ErrUnauthorized
			}
			return fn(check)
		})
	}
}
func withdrawalActualPublish(t *testing.T, p *walletwithdraw.PreparedCommit, e error, fence walletwithdraw.Fence) walletwithdraw.Operation {
	t.Helper()
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		if e := p.Close(); e != nil {
			t.Error(e)
		}
	}()
	op, e := p.Publish(fence)
	if e != nil {
		t.Fatal(e)
	}
	return op
}
func withdrawalActualLogin(t *testing.T, ctx context.Context, store *gymaccount.Store, x *withdrawalActualWallet, enroll bool) {
	t.Helper()
	login, e := store.Login(ctx, gymaccount.Principal{Issuer: "fixture:local", Subject: x.subject, AuthenticatedAtMS: time.Now().UnixMilli()})
	if e != nil {
		t.Fatal(e)
	}
	if enroll {
		challenge, e := store.BeginChallenge(ctx, login.Token, "enroll")
		if e != nil {
			t.Fatal(e)
		}
		auth, e := x.wallet.SignAccountChallenge(ctx, store.AuthorityID(), challenge)
		if e != nil {
			t.Fatal(e)
		}
		if _, _, e = store.CompleteChallenge(ctx, login.Token, challenge.ChallengeID, auth); e != nil {
			t.Fatal(e)
		}
	}
	x.login = login
	if e = store.WithSession(ctx, login.Token, gymaccount.View, func(a gymaccount.Account, check func() error) error { x.account = a; return check() }); e != nil {
		t.Fatal(e)
	}
}
func withdrawalActualAttach(t *testing.T, ctx context.Context, x *withdrawalActualWallet, rpc prospectnode.RPCConfig, height uint32, create bool) {
	t.Helper()
	var e error
	if create {
		var entropy [32]byte
		if x.fixture == "incrementing256" {
			for i := range entropy {
				entropy[i] = byte(i)
			}
		}
		accts := recovery.DefaultAccounts()
		accts[2].NextExternal = 1
		bundle, e := recovery.NewExport(entropy, accts)
		clear(entropy[:])
		if e != nil {
			t.Fatal(e)
		}
		x.wallet, e = restoreAccountFixtureWallet(x.path, x.fixture, bundle)
		bundle.Mnemonic = ""
		if e != nil {
			t.Fatal(e)
		}
	} else {
		x.wallet, e = openAccountFixtureWallet(x.path, x.fixture, false)
		if e != nil {
			t.Fatal(e)
		}
	}
	if e = x.wallet.Attach(ctx, rpc); e != nil {
		t.Fatal(e)
	}
	recipientIntegrationWaitWallet(t, ctx, x.wallet, height)
}
func withdrawalActualOpen(t *testing.T, ctx context.Context, x *withdrawalActualWallet, rpc prospectnode.RPCConfig, source poolshare.Config) {
	t.Helper()
	var e error
	x.resource, e = openPoolWithdrawal(ctx, poolChildResources{Source: source, RPC: rpc, Recipient: x.binding, Withdrawal: &x.scope, Life: ctx}, x.wallet)
	if e != nil {
		t.Fatal(e)
	}
	x.resource.node.http.Transport = &withdrawalActualTransport{base: x.resource.node.http.Transport, record: x.record}
}
func withdrawalActualLegacy(t *testing.T, ctx context.Context, x *withdrawalActualWallet, root, label string, confirmed bool) btcjson.GetTransactionResult {
	t.Helper()
	limit := time.Now().Add(30 * time.Second)
	var got btcjson.GetTransactionResult
	for {
		e := x.wallet.rpc.call(ctx, "gettransaction", []any{x.want.Expected.TxID}, &got)
		var utxos []btcjson.ListUnspentResult
		u := x.wallet.rpc.call(ctx, "listunspent", []any{0, 9999999}, &utxos)
		change := false
		for _, v := range utxos {
			a, err := btcutil.NewAmount(v.Amount)
			if err == nil && v.TxID == x.want.Expected.TxID && v.Vout == uint32(x.want.Draft.Change.OutputIndex) && v.ScriptPubKey == x.want.Draft.Change.Script && int64(a) == x.want.Draft.Change.Atoms {
				change = true
			}
		}
		if e == nil && u == nil && got.TxID == x.want.Expected.TxID && got.Hex == x.want.Expected.Raw && change && (!confirmed || got.Confirmations == 1) {
			break
		}
		if ctx.Err() != nil || time.Now().After(limit) {
			t.Fatal("legacy wallet did not learn exact direct send/change", x.fixture, label)
		}
		time.Sleep(50 * time.Millisecond)
	}
	payoutActualWrite(t, filepath.Join(root, "legacy-"+label+"-"+x.fixture+".json"), got)
	return got
}

// This composition test uses real fixture account leases and concrete resources,
// not a fake UI review or a current PoolRuntime startup over an old source.
func TestPoolWithdrawalRetainedActual(t *testing.T) {
	root, nodeBinary := os.Getenv("SLUG_WITHDRAWAL_ACTUAL_ROOT"), os.Getenv("SLUG_WITHDRAWAL_ACTUAL_NODE")
	if root == "" || nodeBinary == "" {
		t.Skip("explicit copied source and normal node required")
	}
	if !filepath.IsAbs(root) || !filepath.IsAbs(nodeBinary) {
		t.Fatal("absolute owned inputs required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	var reference struct {
		Cases []withdrawalActualCase `json:"cases"`
	}
	corpus, e := os.ReadFile("../../../slugwallet/wallet/testdata/candidate-withdrawals/reference-v1.json")
	if e != nil || fmt.Sprintf("%x", sha256.Sum256(corpus)) != "d7c7b1e4c94e02f734a162b5974749853c91828ea1a0a4c4446c379bddc841c3" || json.Unmarshal(corpus, &reference) != nil {
		t.Fatal("frozen independent wallet corpus unavailable", e)
	}
	var source struct {
		Source poolshare.Config `json:"source"`
	}
	payoutActualRead(t, "../../poolsettle/testdata/payout-history/inputs/original-state-public.json", &source)
	journalBefore := payoutActualJournalSnapshot(t, filepath.Join(root, "shared"))
	wallets := []*withdrawalActualWallet{{fixture: "zero256", subject: "alice", record: &withdrawalActualRecorder{drop: true}}, {fixture: "incrementing256", subject: "bob", record: &withdrawalActualRecorder{}}}
	for _, x := range wallets {
		x.path = filepath.Join(root, "fresh-wallet-"+x.fixture)
		for _, c := range reference.Cases {
			if c.Name == x.fixture+"-original-to-84" {
				x.want = c
			}
		}
		if x.want.Name == "" {
			t.Fatal("missing exact oracle")
		}
		defer x.close(t)
	}
	accounts, e := gymaccount.Create(filepath.Join(root, "fresh-accounts"), gymaccount.Config{Version: 1, Principals: []gymaccount.FixturePrincipal{{Issuer: "fixture:local", Subject: "alice", Fixture: "zero256"}, {Issuer: "fixture:local", Subject: "bob", Fixture: "incrementing256"}}, MaxSessions: 32, MaxChallenges: 8, MaxEvents: 64})
	if e != nil {
		t.Fatal(e)
	}
	defer func() {
		if e := accounts.Close(); e != nil {
			t.Error(e)
		}
	}()
	node := withdrawalActualNodes(t, ctx, nodeBinary, root, 0, 109)
	defer node.close(t)
	rpc := prospectnode.RPCConfig{Version: 1, URL: node.rpc.url, Username: "fixture", Password: integrationFixturePassword}
	parentDir := filepath.Join(root, "withdrawal-parent")
	if e = os.Mkdir(parentDir, 0700); e != nil {
		t.Fatal(e)
	}
	parent := &PoolRuntime{config: Config{DataDir: parentDir}, source: source.Source, marker: poolRuntimeMarker{AccountAuthorityID: accounts.AuthorityID()}, gymAccounts: make(map[string]string)}
	for _, x := range wallets {
		withdrawalActualAttach(t, ctx, x, rpc, 109, true)
		withdrawalActualLogin(t, ctx, accounts, x, true)
		parent.gymAccounts[x.fixture] = x.account.ID
		identity, e := gymaccount.FixtureIdentity(x.fixture)
		if e != nil {
			t.Fatal(e)
		}
		x.binding, _, e = poolRecipientExpected(PoolRecipientConfig{Version: 1, AccountAuthorityID: accounts.AuthorityID(), AccountID: x.account.ID, Fixture: x.fixture, Identity: identity, Source: source.Source})
		if e != nil {
			t.Fatal(e)
		}
		var baseline btcjson.GetTransactionResult
		if e = x.wallet.rpc.call(ctx, "gettransaction", []any{x.want.Draft.Inputs[0].TxID}, &baseline); e != nil {
			t.Fatal(e)
		}
		if baseline.TxID != x.want.Draft.Inputs[0].TxID || baseline.BlockHash != x.want.Draft.TipHash || baseline.Confirmations != 1 {
			t.Fatal("original legacy baseline differs")
		}
		payoutActualWrite(t, filepath.Join(root, "legacy-original-"+x.fixture+".json"), baseline)
		snap, e := x.wallet.Snapshot(ctx)
		if e != nil || snap.Spendable != x.want.Draft.TotalInputAtoms || snap.Height != 109 {
			t.Fatal("original balance differs", e)
		}
		x.scope, e = parent.ensureWithdrawalScope(ctx, x.binding, x.wallet)
		if e != nil {
			t.Fatal(e)
		}
		withdrawalActualOpen(t, ctx, x, rpc, source.Source)
	}
	for _, x := range wallets {
		owner, e := x.resource.Journal.TryAcquire(ctx)
		if e != nil {
			t.Fatal(e)
		}
		func() {
			defer func() {
				if e := owner.Close(); e != nil {
					t.Error(e)
				}
			}()
			fence := withdrawalActualFence(ctx, accounts, x.login, x.account, gymaccount.Withdrawal)
			p, e := owner.StageIntent(x.want.Request)
			op := withdrawalActualPublish(t, p, e, fence)
			d, e := x.resource.Prepare(ctx, x.want.Request)
			if e != nil {
				t.Fatal(e)
			}
			want := x.want.Draft
			want.BindingID = x.scope.BindingID
			if !reflect.DeepEqual(d, want) {
				payoutActualWrite(t, filepath.Join(root, "draft-mismatch-"+x.fixture+".json"), struct{ Got, Want walletwithdraw.Draft }{d, want})
				t.Fatal("actual draft differs from independent transaction oracle", x.fixture)
			}
			p, e = owner.StagePrepared(op.OperationID, d)
			op = withdrawalActualPublish(t, p, e, fence)
			if e = x.resource.Inspect(ctx, d); e != nil {
				t.Fatal(e)
			}
			raw, e := x.resource.Sign(ctx, d)
			if e != nil {
				t.Fatal(e)
			}
			if hex.EncodeToString(raw) != x.want.Expected.Raw || fmt.Sprintf("%x", sha256.Sum256(raw)) != x.want.SignedSHA256 {
				clear(raw)
				t.Fatal("actual signature/bytes differ from independent oracle", x.fixture)
			}
			auth := walletwithdraw.AuthorizationInput{ActionID: withdrawalActualID(x.fixture + "-first-action"), ReviewID: withdrawalActualID(x.fixture + "-first-review"), ViewID: withdrawalActualID(x.fixture + "-first-view"), Generation: x.account.Generation}
			p, e = owner.StageSigned(op.OperationID, raw, auth)
			clear(raw)
			op = withdrawalActualPublish(t, p, e, fence)
			reconcile, e := owner.Reconciliation(op.OperationID)
			if e != nil {
				t.Fatal(e)
			}
			if e = x.resource.Reconcile(ctx, reconcile); e != nil {
				t.Fatal(e)
			}
			p, e = owner.StageDispatch(op.OperationID, auth)
			op = withdrawalActualPublish(t, p, e, fence)
			ticket, e := owner.TakeDelivery(op.OperationID)
			if e != nil {
				t.Fatal(e)
			}
			result, code, e := x.resource.Deliver(ctx, ticket)
			if e != nil {
				t.Fatal(e)
			}
			if x.fixture == "zero256" && (result != walletwithdraw.DeliveryUnknown || code != "node_unavailable") || x.fixture != "zero256" && (result != walletwithdraw.DeliveryAcknowledged || code != "") {
				t.Fatalf("unexpected actual delivery %s %s/%s", x.fixture, result, code)
			}
			p, e = owner.StageDeliveryResult(op.OperationID, op.Dispatch.Attempt, result, code)
			op = withdrawalActualPublish(t, p, e, withdrawalActualFence(ctx, accounts, x.login, x.account, gymaccount.View))
			if x.fixture != "zero256" {
				observation, e := x.resource.Observe(ctx, owner, op.OperationID)
				if e != nil || observation.State != walletwithdraw.ObservationMempool {
					t.Fatal("exact mempool observation", e, observation.State)
				}
				p, e = owner.StageObservation(op.OperationID, observation)
				op = withdrawalActualPublish(t, p, e, withdrawalActualFence(ctx, accounts, x.login, x.account, gymaccount.View))
			}
			payoutActualWrite(t, filepath.Join(root, "operation-first-"+x.fixture+".json"), op)
		}()
		withdrawalActualLegacy(t, ctx, x, root, "live", false)
		t.Logf("%s exact frozen transaction sent; fee %d atoms", x.fixture, x.want.Draft.FeeAtoms)
	}
	// Resolve the deliberately lost acknowledgment only through the old operation.
	// This new authenticated action never calls Prepare or Sign.
	first := wallets[0]
	oldLogin := first.login
	first.close(t)
	if e = accounts.Logout(ctx, oldLogin.Token); e != nil {
		t.Fatal(e)
	}
	if e = accounts.WithSession(ctx, oldLogin.Token, gymaccount.Withdrawal, func(gymaccount.Account, func() error) error { return nil }); e == nil {
		t.Fatal("revoked original session survived")
	}
	withdrawalActualAttach(t, ctx, first, rpc, 109, false)
	withdrawalActualLogin(t, ctx, accounts, first, false)
	withdrawalActualOpen(t, ctx, first, rpc, source.Source)
	owner, e := first.resource.Journal.TryAcquire(ctx)
	if e != nil {
		t.Fatal(e)
	}
	func() {
		defer func() {
			if e := owner.Close(); e != nil {
				t.Error(e)
			}
		}()
		op, e := owner.Lookup(first.want.Request.OperationID)
		if e != nil || op.Stage != walletwithdraw.StageDispatching || op.Signed == nil || op.Signed.Raw != first.want.Expected.Raw {
			t.Fatal("lost acknowledgment not retained", e)
		}
		auth := walletwithdraw.AuthorizationInput{ActionID: withdrawalActualID("zero256-explicit-retry-action"), ReviewID: withdrawalActualID("zero256-explicit-retry-review"), ViewID: withdrawalActualID("zero256-new-view"), Generation: first.account.Generation}
		p, e := owner.StageDispatch(op.OperationID, auth)
		op = withdrawalActualPublish(t, p, e, withdrawalActualFence(ctx, accounts, first.login, first.account, gymaccount.Withdrawal))
		if op.Dispatch.Attempt != 2 {
			t.Fatal("retry attempt did not advance once")
		}
		ticket, e := owner.TakeDelivery(op.OperationID)
		if e != nil {
			t.Fatal(e)
		}
		result, code, e := first.resource.Deliver(ctx, ticket)
		if e != nil {
			t.Fatal(e)
		}
		p, e = owner.StageDeliveryResult(op.OperationID, 2, result, code)
		op = withdrawalActualPublish(t, p, e, withdrawalActualFence(ctx, accounts, first.login, first.account, gymaccount.View))
		observation, e := first.resource.Observe(ctx, owner, op.OperationID)
		if e != nil || observation.State != walletwithdraw.ObservationMempool {
			t.Fatal("retry exact observation unavailable", e, observation.State)
		}
		p, e = owner.StageObservation(op.OperationID, observation)
		op = withdrawalActualPublish(t, p, e, withdrawalActualFence(ctx, accounts, first.login, first.account, gymaccount.View))
		if op.Stage != walletwithdraw.StageSubmitted {
			t.Fatal("retry not resolved by exact observation")
		}
		payoutActualWrite(t, filepath.Join(root, "operation-retry-zero256.json"), op)
	}()
	var mempool []string
	node.rpc.must(t, "getrawmempool", nil, &mempool)
	if len(mempool) != 2 {
		t.Fatal("expected exactly two real reciprocal withdrawals")
	}
	for _, x := range wallets {
		found := false
		for _, txid := range mempool {
			found = found || txid == x.want.Expected.TxID
		}
		if !found {
			t.Fatal("wrong mempool transaction")
		}
	}
	var mined []string
	node.rpc.client.Timeout = 45 * time.Second
	node.rpc.must(t, "generate", []any{1}, &mined)
	node.rpc.client.Timeout = 5 * time.Second
	if len(mined) != 1 {
		t.Fatal("expected one real block")
	}
	withdrawalActualWaitTip(t, ctx, node, 110, mined[0])
	type result struct {
		Fixture     string                   `json:"fixture"`
		Operation   walletwithdraw.Operation `json:"operation"`
		Sends       []withdrawalActualSend   `json:"sends"`
		Balance     int64                    `json:"balance_atoms"`
		ExpectedFee int64                    `json:"expected_fee_atoms"`
	}
	results := make([]result, 0, 2)
	for _, x := range wallets {
		recipientIntegrationWaitWallet(t, ctx, x.wallet, 110)
		legacy := withdrawalActualLegacy(t, ctx, x, root, "confirmed", true)
		if legacy.BlockHash != mined[0] {
			t.Fatal("withdrawal not in exact new block")
		}
		owner, e := x.resource.Journal.TryAcquire(ctx)
		if e != nil {
			t.Fatal(e)
		}
		var final walletwithdraw.Operation
		func() {
			defer func() {
				if e := owner.Close(); e != nil {
					t.Error(e)
				}
			}()
			observation, e := x.resource.Observe(ctx, owner, x.want.Request.OperationID)
			if e != nil || observation.State != walletwithdraw.ObservationConfirmed || observation.BlockHash != mined[0] || observation.Confirmations != 1 {
				t.Fatal("exact canonical observation", e)
			}
			p, e := owner.StageObservation(x.want.Request.OperationID, observation)
			final = withdrawalActualPublish(t, p, e, withdrawalActualFence(ctx, accounts, x.login, x.account, gymaccount.View))
			snap, e := owner.Snapshot()
			if e != nil || snap.Count != 1 || snap.UnresolvedOperationID != "" {
				t.Fatal("unexpected operation count/unresolved state", e)
			}
		}()
		snap, e := x.wallet.Snapshot(ctx)
		if e != nil || snap.Spendable != x.want.Draft.TotalInputAtoms-x.want.Draft.FeeAtoms || snap.Unconfirmed != 0 {
			t.Fatal("reciprocal balance is not original minus one fee", x.fixture, snap, e)
		}
		var spent any
		node.rpc.must(t, "gettxout", []any{x.want.Draft.Inputs[0].TxID, x.want.Draft.Inputs[0].OutputIndex, true}, &spent)
		if spent != nil {
			t.Fatal("original payout outpoint not spent")
		}
		sends := x.record.snapshot()
		wantCount := 1
		if x.fixture == "zero256" {
			wantCount = 2
		}
		if len(sends) != wantCount {
			t.Fatal("extra/missing network attempt")
		}
		for _, sent := range sends {
			if sent.Raw != x.want.Expected.Raw {
				t.Fatal("retry changed raw transaction")
			}
		}
		results = append(results, result{x.fixture, final, sends, snap.Spendable, x.want.Draft.FeeAtoms})
		x.close(t)
		if e = accounts.Logout(ctx, x.login.Token); e != nil {
			t.Fatal(e)
		}
	}
	node.close(t)
	// Reopen both chain copies and each established wallet/journal, with no new
	// allocation, signing, authority, dispatch or mining.
	node = withdrawalActualNodes(t, ctx, nodeBinary, root, 1, 110)
	defer node.close(t)
	rpc.URL = node.rpc.url
	for _, x := range wallets {
		withdrawalActualAttach(t, ctx, x, rpc, 110, false)
		withdrawalActualLogin(t, ctx, accounts, x, false)
		withdrawalActualOpen(t, ctx, x, rpc, source.Source)
		owner, e := x.resource.Journal.TryAcquire(ctx)
		if e != nil {
			t.Fatal(e)
		}
		func() {
			defer func() {
				if e := owner.Close(); e != nil {
					t.Error(e)
				}
			}()
			op, e := owner.Lookup(x.want.Request.OperationID)
			if e != nil || op.Signed == nil || op.Signed.Raw != x.want.Expected.Raw || op.Stage != walletwithdraw.StageSubmitted {
				t.Fatal("established exact operation not retained", e)
			}
			ids, e := x.resource.seam.OperationIDs(ctx)
			if e != nil || !reflect.DeepEqual(ids, []string{x.want.Request.OperationID}) {
				t.Fatal("wallet exclusion operation changed", e)
			}
			row, e := x.resource.seam.Lookup(ctx, x.want.Request.OperationID)
			if e != nil || row.State != "retained" || row.RetainedTxID != x.want.Expected.TxID {
				t.Fatal("permanent wallet retention missing", e)
			}
		}()
		withdrawalActualLegacy(t, ctx, x, root, "reopened", true)
		snap, e := x.wallet.Snapshot(ctx)
		if e != nil || snap.Spendable != x.want.Draft.TotalInputAtoms-x.want.Draft.FeeAtoms {
			t.Fatal("reopened balance charged twice", e)
		}
		x.close(t)
	}
	node.close(t)
	journalAfter := payoutActualJournalSnapshot(t, filepath.Join(root, "shared"))
	if !reflect.DeepEqual(journalBefore, journalAfter) {
		t.Fatal("original source accounting logical records changed")
	}
	payoutActualWrite(t, filepath.Join(root, "actual-withdrawal-result.json"), struct {
		Height   uint32            `json:"height"`
		Block    string            `json:"block"`
		Journals map[string]string `json:"journal_logical_sha256"`
		Results  []result          `json:"results"`
		Scope    string            `json:"scope"`
	}{110, mined[0], journalBefore, results, "Actual fixture account/journal/wallet/node composition; native review qualified separately. One explicitly mined normal block, no source/scheduler mutation."})
}

func withdrawalActualWaitTip(t *testing.T, ctx context.Context, n *integrationNode, height uint32, hash string) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		var left, right string
		var lh, rh uint32
		var lp, rp []btcjson.GetPeerInfoResult
		if n.rpc.call("getbestblockhash", nil, &left) == nil && n.peer.rpc.call("getbestblockhash", nil, &right) == nil && n.rpc.call("getblockcount", nil, &lh) == nil && n.peer.rpc.call("getblockcount", nil, &rh) == nil && n.rpc.call("getpeerinfo", nil, &lp) == nil && n.peer.rpc.call("getpeerinfo", nil, &rp) == nil && len(lp) == 1 && len(rp) == 1 && onlyPilotPeers(lp) && onlyPilotPeers(rp) && left == right && lh == height && rh == height && (hash == "" || left == hash) {
			return
		}
		if ctx.Err() != nil || time.Now().After(deadline) {
			t.Fatal("owned loopback nodes failed exact tip synchronization")
		}
		time.Sleep(50 * time.Millisecond)
	}
}
func withdrawalActualNodes(t *testing.T, ctx context.Context, binaryPath, root string, epoch int, height uint32) *integrationNode {
	t.Helper()
	peerAddress := recipientIntegrationPort(t)
	peer := withdrawalActualOneNode(t, ctx, binaryPath, root, epoch, "peer", "peer-chain", "127.0.0.1:1", peerAddress)
	node := withdrawalActualOneNode(t, ctx, binaryPath, root, epoch, "primary", "primary-chain", peerAddress, "")
	node.peer = peer
	withdrawalActualWaitTip(t, ctx, node, height, "")
	return node
}
func withdrawalActualOneNode(t *testing.T, ctx context.Context, binaryPath, root string, epoch int, name, chainDirectory, connect, listen string) *integrationNode {
	t.Helper()
	home := filepath.Join(root, fmt.Sprintf("withdrawal-%s-epoch%d", name, epoch))
	if e := os.Mkdir(home, 0700); e != nil {
		t.Fatal(e)
	}
	rpc := recipientIntegrationPort(t)
	conf := fmt.Sprintf("rpcuser=fixture\nrpcpass=%s\nnotls=1\nrpclisten=%s\nnodnsseed=1\nconnect=%s\nlocalwalletpilot=1\ntxindex=1\nminingaddr=%s\ndatadir=%s\nlogdir=%s\n", integrationFixturePassword, rpc, connect, poolsettlefixture.PoolAddress(), filepath.Join(root, "shared", chainDirectory), filepath.Join(home, "logs"))
	if listen == "" {
		conf += "nolisten=1\n"
	} else {
		conf += "listen=" + listen + "\n"
	}
	config := filepath.Join(home, "fixture.conf")
	if e := os.WriteFile(config, []byte(conf), 0600); e != nil {
		t.Fatal(e)
	}
	log, e := os.OpenFile(filepath.Join(home, "node.log"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		t.Fatal(e)
	}
	n := &integrationNode{rpc: newIntegrationRPC("http://" + rpc), cmd: exec.Command(binaryPath, "--configfile="+config), done: make(chan error, 1)}
	n.rpc.client.Timeout = 5 * time.Second
	n.cmd.Stdout, n.cmd.Stderr = log, log
	if e = n.cmd.Start(); e != nil {
		log.Close()
		t.Fatal(e)
	}
	go func() { e := n.cmd.Wait(); _ = log.Close(); n.done <- e }()
	t.Cleanup(func() { n.close(t) })
	deadline := time.Now().Add(30 * time.Second)
	for {
		var magic uint32
		if n.rpc.call("getcurrentnet", nil, &magic) == nil {
			var genesis string
			n.rpc.must(t, "getblockhash", []any{0}, &genesis)
			if magic != uint32(chaincfg.MainNetParams.Net) || genesis != chaincfg.MainNetParams.GenesisHash.String() {
				t.Fatal("wrong owned chain identity")
			}
			return n
		}
		if ctx.Err() != nil || time.Now().After(deadline) {
			t.Fatal("owned node startup deadline")
		}
		time.Sleep(50 * time.Millisecond)
	}
}
