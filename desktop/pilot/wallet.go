package pilot

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/address/v2"
	"github.com/taytr-llc/slug/btcjson"
	"github.com/taytr-llc/slug/btcutil/v2"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/prospectworker"
	"github.com/taytr-llc/slug/wire/v2"
	"github.com/taytr-llc/slugwallet/chain"
	"github.com/taytr-llc/slugwallet/recovery"
	"github.com/taytr-llc/slugwallet/rpc/legacyrpc"
	"github.com/taytr-llc/slugwallet/waddrmgr"
	"github.com/taytr-llc/slugwallet/wallet"
	"github.com/taytr-llc/slugwallet/walletdb"
	_ "github.com/taytr-llc/slugwallet/walletdb/bdb"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Public demonstration passwords, not custody credentials. This adapter can
// create only published fixtures. The original local-wallet mode remains zero256.
const fixtureWalletPublicPass = "slug-local-wallet-pilot-public"
const fixtureWalletPrivatePass = "slug-local-wallet-pilot-public-fixture-only"
const maxWalletRPCResponse = 4 << 20

// Public identity from slugwallet/recovery/testdata/recovery_v1.json.
var publishedWalletIdentity = walletIdentity{
	RootPublicKey: "JgduaQiYjRcnjcD9t1MfTP1qo8eWaAWPQtS9nChXwtAeyTsoEGVZ8SjVqa5qxxvvKqX8tqC77wMSjaMHXNY8JSYEASdzX6PT31ktXqKPJvkhyy31",
	GymPublicKey:  "03ec7488667fd544b4a1de8fb0ae03730c9a2dc346c652cbaa59a077dbeddca519",
	GymID:         "a227587d837507dbcb5f9d781967339d816070fb7bd574931c690ba6937b88e5",
}

type walletIdentity struct {
	RootPublicKey string `json:"root_public_key"`
	GymPublicKey  string `json:"gym_public_key"`
	GymID         string `json:"gym_id"`
}
type walletPayout struct {
	Address string `json:"address"`
	Purpose uint32 `json:"purpose"`
	Coin    uint32 `json:"coin"`
	Account uint32 `json:"account"`
	Branch  uint32 `json:"branch"`
	Index   uint32 `json:"index"`
}
type walletView struct {
	Synced      bool  `json:"synced"`
	Height      int32 `json:"height"`
	Spendable   int64 `json:"spendable"`
	Immature    int64 `json:"immature"`
	Unconfirmed int64 `json:"unconfirmed"`
}
type walletVerificationState struct {
	Accounts     []recovery.Account
	View         walletView
	UTXOs        []btcjson.ListUnspentResult
	Transactions []btcjson.ListTransactionsResult
}

// The controller drains mining before Close. This lock prevents concurrent
// private operations from racing database closure.
type walletRuntime struct {
	allocation2           *wallet.HostedAllocator2
	allocationGuard2      *waddrmgr.HostedAllocationGuard2
	allocation            *wallet.HostedAllocator
	allocationGuard       *waddrmgr.HostedAllocationGuard
	allocationRecipientID string
	mu                    sync.Mutex
	loader                *wallet.Loader
	wallet                *wallet.Wallet
	backend               *chain.RPCClient
	server                *legacyrpc.Server
	rpc                   *walletHTTPClient
	identity              walletIdentity
	closed                bool
	closeErr              error

	poolRecipientUncertain bool
}

func prepareWalletDirectory(dir string, create bool) error {
	if !filepath.IsAbs(dir) || filepath.Clean(dir) != dir {
		return errors.New("invalid wallet directory")
	}
	info, err := os.Lstat(dir)
	if os.IsNotExist(err) && create {
		if err = os.Mkdir(dir, 0700); err != nil {
			return errors.New("wallet directory unavailable")
		}
		info, err = os.Lstat(dir)
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("wallet directory unavailable")
	}
	if create {
		entries, err := os.ReadDir(dir)
		if err != nil || len(entries) != 0 {
			return errors.New("wallet creation requires an empty directory")
		}
	} else {
		file, err := os.Lstat(filepath.Join(dir, wallet.WalletDBName))
		if err != nil || !file.Mode().IsRegular() || file.Mode()&os.ModeSymlink != 0 {
			return errors.New("established wallet is unavailable")
		}
	}
	return nil
}
func openFixtureWallet(dir string, create bool) (*walletRuntime, error) {
	if wire.TestPoW {
		return nil, errors.New("wallet pilot requires production PoW")
	}
	if err := prepareWalletDirectory(dir, create); err != nil {
		return nil, err
	}
	loader := wallet.NewLoader(&chaincfg.MainNetParams, dir, true, 5*time.Second, 20)
	var loaded *wallet.Wallet
	var err error
	if create {
		loaded, err = loader.CreateNewRecoveryWallet([]byte(fixtureWalletPublicPass), []byte(fixtureWalletPrivatePass), [32]byte{}, chaincfg.MainNetParams.GenesisBlock.Header.Timestamp)
	} else {
		loaded, err = loader.OpenExistingWallet([]byte(fixtureWalletPublicPass), false)
	}
	if err != nil {
		return nil, errors.New("fixture wallet could not be opened")
	}
	return finishWalletOpen(loader, loaded)
}

// openAccountFixtureWallet is separate from the original zero256 pilot mode.
// Selection is an exact public fixture name supplied by trusted account config.
func openAccountFixtureWallet(dir, fixture string, create bool) (*walletRuntime, error) {
	identity, err := gymaccount.FixtureIdentity(fixture)
	if err != nil || wire.TestPoW {
		return nil, errors.New("account wallet fixture unavailable")
	}
	if err := prepareWalletDirectory(dir, create); err != nil {
		return nil, err
	}
	loader := wallet.NewLoader(&chaincfg.MainNetParams, dir, true, 5*time.Second, 20)
	var loaded *wallet.Wallet
	if create {
		var entropy [32]byte
		if fixture == "incrementing256" {
			for i := range entropy {
				entropy[i] = byte(i)
			}
		}
		loaded, err = loader.CreateNewRecoveryWallet([]byte(fixtureWalletPublicPass), []byte(fixtureWalletPrivatePass), entropy, chaincfg.MainNetParams.GenesisBlock.Header.Timestamp)
		clear(entropy[:])
	} else {
		loaded, err = loader.OpenExistingWallet([]byte(fixtureWalletPublicPass), false)
	}
	if err != nil {
		return nil, errors.New("account fixture wallet could not be opened")
	}
	return finishWalletOpenExpected(loader, loaded, walletIdentity{RootPublicKey: identity.RootPublicKey, GymPublicKey: identity.GymPublicKey, GymID: identity.GymID})
}
func restoreAccountFixtureWallet(dir, fixture string, bundle recovery.Export) (*walletRuntime, error) {
	identity, err := gymaccount.FixtureIdentity(fixture)
	if err != nil || wire.TestPoW {
		return nil, errors.New("account wallet fixture unavailable")
	}
	expected := walletIdentity{RootPublicKey: identity.RootPublicKey, GymPublicKey: identity.GymPublicKey, GymID: identity.GymID}
	if recovery.ValidateExport(bundle) != nil || bundle.Manifest.RootPublicKey != expected.RootPublicKey || bundle.Manifest.GymPublicKey != expected.GymPublicKey || bundle.Manifest.GymID != expected.GymID {
		return nil, errors.New("backup does not match the selected account fixture")
	}
	if err := prepareWalletDirectory(dir, true); err != nil {
		return nil, err
	}
	loader := wallet.NewLoader(&chaincfg.MainNetParams, dir, true, 5*time.Second, 20)
	loaded, err := loader.RestoreRecoveryWallet([]byte(fixtureWalletPublicPass), []byte(fixtureWalletPrivatePass), bundle)
	if err != nil {
		return nil, errors.New("account fixture recovery could not be restored")
	}
	return finishWalletOpenExpected(loader, loaded, expected)
}
func restoreFixtureWallet(dir string, bundle recovery.Export) (*walletRuntime, error) {
	if wire.TestPoW {
		return nil, errors.New("wallet pilot requires production PoW")
	}
	if recovery.ValidateExport(bundle) != nil || bundle.Manifest.RootPublicKey != publishedWalletIdentity.RootPublicKey || bundle.Manifest.GymPublicKey != publishedWalletIdentity.GymPublicKey || bundle.Manifest.GymID != publishedWalletIdentity.GymID {
		return nil, errors.New("backup is not the published pilot fixture")
	}
	if err := prepareWalletDirectory(dir, true); err != nil {
		return nil, err
	}
	loader := wallet.NewLoader(&chaincfg.MainNetParams, dir, true, 5*time.Second, 20)
	loaded, err := loader.RestoreRecoveryWallet([]byte(fixtureWalletPublicPass), []byte(fixtureWalletPrivatePass), bundle)
	if err != nil {
		return nil, errors.New("fixture recovery could not be restored")
	}
	return finishWalletOpen(loader, loaded)
}
func finishWalletOpen(loader *wallet.Loader, loaded *wallet.Wallet) (*walletRuntime, error) {
	return finishWalletOpenExpected(loader, loaded, publishedWalletIdentity)
}
func finishWalletOpenExpected(loader *wallet.Loader, loaded *wallet.Wallet, expected walletIdentity) (*walletRuntime, error) {
	r := &walletRuntime{loader: loader, wallet: loaded}
	gym, err := loaded.GymIdentity(context.Background())
	if err == nil {
		r.identity.GymID = gym.GymID
		r.identity.GymPublicKey = gym.PublicKey
		err = walletdb.View(loaded.Database(), func(tx walletdb.ReadTx) error {
			var e error
			r.identity.RootPublicKey, e = loaded.Manager.RecoveryRootPublicKey(tx.ReadBucket([]byte("waddrmgr")))
			return e
		})
	}
	if err != nil || r.identity != expected {
		closeErr := r.Close()
		return nil, errors.Join(errors.New("wallet does not match the published pilot identity"), closeErr)
	}
	return r, nil
}
func (r *walletRuntime) Identity() walletIdentity { return r.identity }
func (r *walletRuntime) Attach(ctx context.Context, config prospectnode.RPCConfig) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || r.backend != nil {
		return errors.New("wallet attachment unavailable")
	}
	if ctx.Err() != nil {
		return errors.New("wallet attachment canceled")
	}
	endpoint, err := url.Parse(config.URL)
	if err != nil || config.Version != 1 || endpoint.Scheme != "http" || endpoint.User != nil || endpoint.Hostname() != "127.0.0.1" || endpoint.Port() == "" || (endpoint.Path != "" && endpoint.Path != "/") || endpoint.RawQuery != "" || endpoint.Fragment != "" || config.CACertificate != "" {
		return errors.New("wallet requires its owned loopback node")
	}
	backend, err := chain.NewRPCClient(&chaincfg.MainNetParams, endpoint.Host, config.Username, config.Password, nil, true, 0)
	if err != nil {
		return errors.New("wallet node unavailable")
	}
	if err = backend.Start(ctx); err != nil {
		backend.Stop()
		backend.WaitForShutdown()
		return errors.New("wallet node unavailable")
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		backend.Stop()
		backend.WaitForShutdown()
		return errors.New("wallet RPC unavailable")
	}
	credential := make([]byte, 32)
	if _, err = rand.Read(credential); err != nil {
		listener.Close()
		backend.Stop()
		backend.WaitForShutdown()
		return errors.New("wallet RPC unavailable")
	}
	password := hex.EncodeToString(credential)
	clear(credential)
	server := legacyrpc.NewServer(&legacyrpc.Options{Username: "local-wallet-pilot", Password: password, MaxPOSTClients: 2, MaxWebsocketClients: 0, AllowedMethods: walletRPCMethods()}, r.loader, []net.Listener{listener})
	server.RegisterWallet(r.wallet)
	server.SetChainServer(backend)
	r.backend = backend
	r.server = server
	r.rpc = newWalletHTTPClient("http://"+listener.Addr().String(), "local-wallet-pilot", password)
	// Keep the concrete verified RPC client on the restricted HTTP server.
	// Only the wallet rescan readiness heuristic receives the local wrapper.
	r.wallet.SynchronizeRPC(&ownedLocalChain{RPCClient: backend})
	return nil
}
func walletRPCMethods() []string {
	return []string{"getnewaddress", "validateaddress", "getbalance", "getunconfirmedbalance", "gettransaction", "listtransactions", "listunspent"}
}
func (r *walletRuntime) EnsurePayout(ctx context.Context, expected *walletPayout) (walletPayout, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var payout walletPayout
	if r.closed || r.rpc == nil {
		return payout, errors.New("wallet RPC unavailable")
	}
	if expected == nil {
		if err := r.rpc.call(ctx, "getnewaddress", []any{"default", "legacy"}, &payout.Address); err != nil {
			return payout, err
		}
	} else {
		payout = *expected
	}
	var info btcjson.ValidateAddressWalletResult
	if err := r.rpc.call(ctx, "validateaddress", []any{payout.Address}, &info); err != nil {
		return walletPayout{}, err
	}
	if !info.IsValid || !info.IsMine || info.IsScript || info.Address != payout.Address {
		return walletPayout{}, errors.New("payout is not owned by this wallet")
	}
	decoded, err := address.DecodeAddress(payout.Address, &chaincfg.MainNetParams)
	if err != nil {
		return walletPayout{}, errors.New("invalid wallet payout")
	}
	managed, err := r.wallet.AddressInfo(decoded)
	if err != nil {
		return walletPayout{}, errors.New("wallet payout path unavailable")
	}
	key, ok := managed.(waddrmgr.ManagedPubKeyAddress)
	if !ok {
		return walletPayout{}, errors.New("unsupported wallet payout")
	}
	scope, path, ok := key.DerivationInfo()
	if !ok || scope != waddrmgr.KeyScopeBIP0044 || path.InternalAccount != 0 || path.Branch != 0 || path.Index >= recovery.MaxBranchIndex || managed.Internal() {
		return walletPayout{}, errors.New("unsupported wallet payout path")
	}
	actual := walletPayout{Address: payout.Address, Purpose: scope.Purpose, Coin: scope.Coin, Account: path.InternalAccount, Branch: path.Branch, Index: path.Index}
	if expected != nil && actual != *expected {
		return walletPayout{}, errors.New("persisted wallet payout changed")
	}
	return actual, nil
}
func (r *walletRuntime) Snapshot(ctx context.Context) (walletView, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.snapshot(ctx)
}
func (r *walletRuntime) snapshot(ctx context.Context) (walletView, error) {
	var result walletView
	if r.closed || r.rpc == nil {
		return result, errors.New("wallet RPC unavailable")
	}
	height := r.wallet.Manager.SyncedTo().Height
	var confirmed, unconfirmed float64
	if err := r.rpc.call(ctx, "getbalance", []any{"default", 1}, &confirmed); err != nil {
		return result, err
	}
	if err := r.rpc.call(ctx, "getunconfirmedbalance", []any{"default"}, &unconfirmed); err != nil {
		return result, err
	}
	spendable, err := btcutil.NewAmount(confirmed)
	if err != nil || spendable < 0 {
		return result, errors.New("invalid wallet balance")
	}
	pending, err := btcutil.NewAmount(unconfirmed)
	if err != nil || pending < 0 {
		return result, errors.New("invalid wallet balance")
	}
	// RPC omits immature reward totals; supplement this public field with the
	// wallet balance API, never a truncated transaction-history estimate.
	balances, err := r.wallet.CalculateAccountBalances(0, 1)
	if err != nil {
		return result, errors.New("wallet balances unavailable")
	}
	// Legacy getunconfirmedbalance means total minus spendable, including
	// immature coinbases. Report the latter separately without double counting.
	if r.wallet.Manager.SyncedTo().Height != height || pending < balances.ImmatureReward {
		return result, errors.New("wallet balance snapshot changed")
	}
	pending -= balances.ImmatureReward
	return walletView{Synced: r.wallet.ChainSynced(), Height: height, Spendable: int64(spendable), Immature: int64(balances.ImmatureReward), Unconfirmed: int64(pending)}, nil
}
func (r *walletRuntime) VerificationState(ctx context.Context) (walletVerificationState, error) {
	var allocationAccounts []recovery.Account
	if r.allocation != nil {
		m, e := r.allocation.CapturePublicRecovery(ctx)
		if e != nil {
			return walletVerificationState{}, e
		}
		allocationAccounts = m.Accounts
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	var result walletVerificationState
	var err error
	result.View, err = r.snapshot(ctx)
	if err != nil {
		return result, err
	}
	result.Accounts = recovery.DefaultAccounts()
	if allocationAccounts != nil {
		result.Accounts = allocationAccounts
	} else {
		err = walletdb.View(r.wallet.Database(), func(tx walletdb.ReadTx) error {
			states, err := waddrmgr.RecoveryAccounts(tx.ReadBucket([]byte("waddrmgr")))
			if err != nil {
				return err
			}
			for i := range result.Accounts {
				found := false
				for _, state := range states {
					if state.Scope.Purpose == result.Accounts[i].Purpose && state.Scope.Coin == result.Accounts[i].Coin {
						result.Accounts[i].NextExternal = state.NextExternal
						result.Accounts[i].NextInternal = state.NextInternal
						found = true
					}
				}
				if !found {
					return errors.New("wallet inventory unavailable")
				}
			}
			return nil
		})
	}
	if err != nil {
		return walletVerificationState{}, errors.New("wallet inventory unavailable")
	}
	if err = r.rpc.call(ctx, "listunspent", []any{0, 99999999}, &result.UTXOs); err != nil {
		return walletVerificationState{}, err
	}
	if len(result.UTXOs) > 4096 {
		return walletVerificationState{}, errors.New("wallet inventory exceeds pilot limit")
	}
	if err = r.rpc.call(ctx, "listtransactions", []any{"*", 1001, 0}, &result.Transactions); err != nil {
		return walletVerificationState{}, err
	}
	if len(result.Transactions) > 1000 {
		return walletVerificationState{}, errors.New("wallet history exceeds pilot limit")
	}
	// These two fields are local discovery times, not chain data. A fresh
	// scan legitimately observes a transaction later than the original wallet.
	// Retain block time, membership, amounts and all other history fields.
	for i := range result.Transactions {
		result.Transactions[i].Time = 0
		result.Transactions[i].TimeReceived = 0
	}
	sort.Slice(result.UTXOs, func(i, j int) bool {
		a, b := result.UTXOs[i], result.UTXOs[j]
		if a.TxID == b.TxID {
			return a.Vout < b.Vout
		}
		return a.TxID < b.TxID
	})
	sort.Slice(result.Transactions, func(i, j int) bool {
		a, _ := json.Marshal(result.Transactions[i])
		b, _ := json.Marshal(result.Transactions[j])
		return bytes.Compare(a, b) < 0
	})
	return result, nil
}
func (r *walletRuntime) Signer(ctx context.Context, policy prospectnode.Policy) (prospectworker.Signer, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || ctx.Err() != nil {
		return nil, errors.New("wallet signer unavailable")
	}
	signer, err := r.wallet.ProspectSigner(ctx, policy)
	if err != nil {
		return nil, errors.New("wallet signer unavailable")
	}
	if err = r.wallet.Unlock([]byte(fixtureWalletPrivatePass), nil); err != nil {
		return nil, errors.New("wallet unlock failed")
	}
	if ctx.Err() != nil {
		r.wallet.Lock()
		return nil, errors.New("wallet signer canceled")
	}
	return func(ctx context.Context, c prospect.Context, p prospect.Proof) (gymauth.Authorization, error) {
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.closed {
			return gymauth.Authorization{}, errors.New("wallet signer unavailable")
		}
		return signer(ctx, c, p)
	}, nil
}
func (r *walletRuntime) ExportRecovery(ctx context.Context) (recovery.Export, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || ctx.Err() != nil {
		return recovery.Export{}, errors.New("wallet export unavailable")
	}
	if r.wallet.Locked() {
		if err := r.wallet.Unlock([]byte(fixtureWalletPrivatePass), nil); err != nil {
			return recovery.Export{}, errors.New("wallet unlock failed")
		}
		defer r.wallet.Lock()
	}
	result, err := r.wallet.ExportRecovery(ctx)
	if err != nil {
		return recovery.Export{}, errors.New("wallet recovery export failed")
	}
	return result, nil
}

// SignAccountChallenge uses a short unlock window and the wallet's bounded
// enrollment signer. Session generation/hold checks belong to the authority.
func (r *walletRuntime) SignAccountChallenge(ctx context.Context, pinnedAuthority string, challenge gymaccount.EnrollmentChallenge) (gymaccount.Authorization, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed || ctx.Err() != nil {
		return gymaccount.Authorization{}, errors.New("account wallet signer unavailable")
	}
	if r.wallet.Locked() {
		if err := r.wallet.Unlock([]byte(fixtureWalletPrivatePass), nil); err != nil {
			return gymaccount.Authorization{}, errors.New("account wallet unlock failed")
		}
		defer r.wallet.Lock()
	}
	result, err := r.wallet.SignAccountChallenge(ctx, pinnedAuthority, challenge)
	if err != nil {
		return gymaccount.Authorization{}, errors.New("account wallet authorization failed")
	}
	return result, nil
}
func (r *walletRuntime) Close() error {
	if r.allocation2 != nil {
		if e := r.allocation2.Close(); e != nil {
			return e
		}
	}
	// The concrete owner drains operations before closing this retained session.
	// No wallet mutex spans the allocator's account fence or lifecycle join.
	if r.allocation != nil {
		if e := r.allocation.Close(); e != nil {
			return e
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return r.closeErr
	}
	r.closed = true
	if r.rpc != nil {
		r.rpc.client.CloseIdleConnections()
	}
	if r.server != nil {
		r.server.Stop()
	} else if r.backend != nil {
		r.backend.Stop()
		r.backend.WaitForShutdown()
	}
	// Server joins registered wallet/backend; loader then closes its owned DB.
	if err := r.loader.UnloadWallet(); err != nil {
		r.closeErr = errors.New("wallet close failed")
	}
	// Loader joins goroutines and closes the file, but does not erase the
	// retained manager's in-memory key caches. Explicitly end this session.
	r.wallet.Manager.Close()
	return r.closeErr
}

type walletHTTPClient struct {
	url, user, password string
	client              *http.Client
	mu                  sync.Mutex
	nextID              uint64
	calls               map[string]uint64
}

func newWalletHTTPClient(endpoint, user, password string) *walletHTTPClient {
	tr := &http.Transport{Proxy: nil, DisableKeepAlives: true, DisableCompression: true, ResponseHeaderTimeout: 30 * time.Second, MaxResponseHeaderBytes: 8192}
	return &walletHTTPClient{url: endpoint, user: user, password: password, client: &http.Client{Transport: tr, Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, calls: make(map[string]uint64)}
}
func (r *walletHTTPClient) callCounts() map[string]uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make(map[string]uint64, len(r.calls))
	for k, v := range r.calls {
		result[k] = v
	}
	return result
}
func (r *walletHTTPClient) call(ctx context.Context, method string, params []any, result any) error {
	allowed := false
	for _, m := range walletRPCMethods() {
		if m == method {
			allowed = true
		}
	}
	if !allowed {
		return errors.New("wallet method unavailable")
	}
	r.mu.Lock()
	r.nextID++
	id := r.nextID
	r.calls[method]++
	r.mu.Unlock()
	body, err := json.Marshal(map[string]any{"jsonrpc": "1.0", "id": id, "method": method, "params": params})
	if err != nil || len(body) > 64<<10 {
		return errors.New("invalid wallet request")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.url, bytes.NewReader(body))
	if err != nil {
		return errors.New("wallet RPC unavailable")
	}
	req.SetBasicAuth(r.user, r.password)
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.client.Do(req)
	if err != nil {
		return errors.New("wallet RPC unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.New("wallet RPC unavailable")
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxWalletRPCResponse+1))
	if err != nil || len(data) > maxWalletRPCResponse {
		return errors.New("invalid wallet response")
	}
	var reply struct {
		ID     uint64          `json:"id"`
		Result json.RawMessage `json:"result"`
		Error  json.RawMessage `json:"error"`
	}
	if json.Unmarshal(data, &reply) != nil || reply.ID != id || string(reply.Error) != "null" || len(reply.Result) == 0 || string(reply.Result) == "null" {
		return errors.New("wallet request failed")
	}
	if result != nil && json.Unmarshal(reply.Result, result) != nil {
		return errors.New("invalid wallet response")
	}
	return nil
}
