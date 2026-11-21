// External finite Set74 rehearsal. All private material below is generated from
// explicitly published synthetic fixtures. No public seed or real keys are used.
package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"reflect"
	"sync"
	"syscall"
	"time"

	"github.com/taytr-llc/slug/btcutil/v2/hdkeychain"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/desktop/pooloperator"
	"github.com/taytr-llc/slug/internal/operatorauthority"
	"github.com/taytr-llc/slug/poolingress"
	"github.com/taytr-llc/slug/poolminer"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/poolsettlefixture"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
	"github.com/taytr-llc/slugwallet/recovery"
	"github.com/taytr-llc/slugwallet/wallet"
)

const setupReferenceSHA = "97fa4879bf955f2f321202c34a95dfc335e684031152e100979e27f0d98da0b3"
const publicPass = "published pool wallet public passphrase"
const privatePass = "published pool wallet private passphrase"

type publicFixtureMinerConfig struct {
	Setup         pooloperator.Setup
	RPC           prospectnode.RPCConfig
	CACertificate string
	GymPublicKey  string
	PoolToken     string
}

const minerPayment int64 = 990000000 // 10 SLUG minus fixed operator 1%, one gym per round.

func privateWrite(path string, raw []byte) {
	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	check(e, "exclusive fixture input")
	n, we := f.Write(raw)
	se := f.Sync()
	ce := f.Close()
	require(n == len(raw), "complete fixture input")
	check(errors.Join(we, se, ce), "fixture input sync/close")
}
func readBound(path string, n int64) []byte {
	f, e := os.Open(path)
	check(e, "bounded input open")
	b, e := io.ReadAll(io.LimitReader(f, n+1))
	ce := f.Close()
	check(errors.Join(e, ce), "bounded input read/close")
	require(int64(len(b)) <= n, "bounded input cap")
	return b
}
func poolSetup(path string) pooloperator.Setup {
	raw := readBound(path, 1<<20)
	require(len(raw) == 28735 && hx(digest(raw)) == setupReferenceSHA, "frozen independent late-winner setup")
	var corpus struct {
		Setup     pooloperator.Setup
		Canonical string `json:"canonical_hex"`
		ID        string `json:"setup_id"`
	}
	check(json.Unmarshal(raw, &corpus), "independent late setup")
	selected, e := pooloperator.ParseSetup(dec(corpus.Canonical))
	check(e, "canonical late setup")
	require(reflect.DeepEqual(selected, corpus.Setup), "exact independent setup bytes")
	id, e := pooloperator.SetupID(selected)
	check(e, "setup ID")
	require(id == corpus.ID, "independent setup ID")
	reservation, e := pooloperator.CapacityEnvelope(selected)
	check(e, "actual derived envelope")
	require(reservation.Submissions == 4 && selected.Pool.MaxSubmissions == 8 && selected.Pool.MaxWorks == 4 && selected.Settlement.MaxBatches == 8, "real ordinary four-row envelope and physical headroom")
	return selected
}
func fixtureBinding(s pooloperator.Setup) wallet.PoolWalletBinding {
	return wallet.PoolWalletBinding{Version: 1, WalletOwnerID: s.WalletGuard.WalletOwnerID, OperatorProfileID: s.WalletGuard.OperatorProfileID, Source: s.Pool, Settlement: s.Settlement, Guard: s.WalletGuard}
}
func fixtureAuthorityBinding(s pooloperator.Setup) operatorauthority.Binding {
	p, e := registry.NewOperatorProfile(s.Authority)
	check(e, "operator profile")
	g, e := chainhash.NewHashFromStr(s.Authority.Source.Genesis)
	check(e, "operator genesis")
	return operatorauthority.Binding{Version: 2, PinID: p.ID(), Network: wire.BitcoinNet(s.Authority.Source.Network), Genesis: *g, Keys: operatorauthority.Keys{Admission: s.Authority.AdmissionKey, Publication: s.Authority.PublicationKey, Cutoff: s.Authority.CutoffKey, Checkpoint: s.Authority.CheckpointKey}}
}
func fixtureTLS(root string) (trustCA, key, spki string) {
	caPub, caPriv, e := ed25519.GenerateKey(rand.Reader)
	check(e, "synthetic TLS CA key")
	defer clear(caPriv)
	pub, priv, e := ed25519.GenerateKey(rand.Reader)
	check(e, "synthetic TLS leaf key")
	defer clear(priv)
	now := time.Now().UTC()
	caTemplate := &x509.Certificate{SerialNumber: big.NewInt(74), Subject: pkix.Name{CommonName: "Set74 isolated pool rehearsal CA"}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour), KeyUsage: x509.KeyUsageCertSign, BasicConstraintsValid: true, IsCA: true}
	caDER, e := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, caPub, caPriv)
	check(e, "synthetic TLS CA certificate")
	ca, e := x509.ParseCertificate(caDER)
	check(e, "synthetic TLS CA parse")
	template := &x509.Certificate{SerialNumber: big.NewInt(75), Subject: pkix.Name{CommonName: "Set74 isolated pool rehearsal server"}, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, BasicConstraintsValid: true, IsCA: false, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}}
	der, e := x509.CreateCertificate(rand.Reader, template, ca, pub, caPriv)
	check(e, "synthetic TLS server certificate")
	leaf, e := x509.ParseCertificate(der)
	check(e, "synthetic server certificate parse")
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	_, e = leaf.Verify(x509.VerifyOptions{Roots: roots, CurrentTime: now, DNSName: "127.0.0.1", KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}})
	check(e, "synthetic leaf chain/IP/server-auth preflight")
	require(!leaf.IsCA, "server certificate is not a CA")
	pk, e := x509.MarshalPKCS8PrivateKey(priv)
	check(e, "synthetic TLS private encoding")
	defer clear(pk)
	trustCA = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}))
	chain := append(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), []byte(trustCA)...)
	key = string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pk}))
	spki = hx(digest(leaf.RawSubjectPublicKeyInfo))
	privateWrite(filepath.Join(root, "tls-cert.pem"), chain)
	privateWrite(filepath.Join(root, "tls-key.pem"), []byte(key))
	return
}
func uniquePorts(n int) []string {
	out := make([]string, 0, n)
	seen := map[string]bool{}
	for len(out) < n {
		p := port()
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

// The controlled proxy forwards only to the original owned node. It loses
// exactly the first successful sendrawtransaction reply after acceptance; all
// retained bytes and returned txids are public signed transaction evidence.
type lostBroadcast struct {
	Raw  string `json:"raw"`
	TxID string `json:"txid"`
	AtMS int64  `json:"at_ms"`
}
type rpcRelay struct {
	hold bool
	held chan lostBroadcast

	url             string
	server          *http.Server
	listener        net.Listener
	done            chan error
	client          *http.Client
	mu              sync.Mutex
	calls           []lostBroadcast
	attempts        []lostBroadcast
	attemptOverflow bool
	lost            chan lostBroadcast
	drop            bool
	handlers        sync.WaitGroup
	closing         bool
	closeError      error
}

func startRPCRelay(ctx context.Context, upstream string) *rpcRelay {
	l, e := net.Listen("tcp4", "127.0.0.1:0")
	check(e, "RPC relay listen")
	r := &rpcRelay{url: "http://" + l.Addr().String(), listener: l, done: make(chan error, 1), lost: make(chan lostBroadcast, 1), drop: true, hold: true, held: make(chan lostBroadcast, 1), client: &http.Client{Transport: &http.Transport{Proxy: nil, DisableCompression: true, DisableKeepAlives: true, MaxResponseHeaderBytes: 65536}, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
	r.server = &http.Server{ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 20 * time.Second, MaxHeaderBytes: 8192, BaseContext: func(net.Listener) context.Context { return ctx }}
	r.server.Handler = http.HandlerFunc(func(w http.ResponseWriter, q *http.Request) {
		r.mu.Lock()
		if r.closing {
			r.mu.Unlock()
			http.Error(w, "closed", 503)
			return
		}
		r.handlers.Add(1)
		r.mu.Unlock()
		defer r.handlers.Done()
		body, e := io.ReadAll(io.LimitReader(q.Body, 1<<20))
		if e != nil || len(body) >= 1<<20 {
			http.Error(w, "bounded request", 400)
			return
		}
		var req struct {
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		if json.Unmarshal(body, &req) != nil {
			http.Error(w, "request", 400)
			return
		}
		if req.Method == "sendrawtransaction" {
			var rawHex string
			if len(req.Params) == 0 || json.Unmarshal(req.Params[0], &rawHex) != nil || len(rawHex) == 0 || len(rawHex) > 2*poolsettle.MaxTransactionBytes {
				r.mu.Lock()
				r.attemptOverflow = true
				r.mu.Unlock()
				http.Error(w, "invalid bounded send attempt", 400)
				return
			}
			r.mu.Lock()
			if len(r.attempts) >= 16 {
				r.attemptOverflow = true
				r.mu.Unlock()
				http.Error(w, "send attempt cap", 503)
				return
			}
			attempt := lostBroadcast{Raw: rawHex, AtMS: time.Now().UnixMilli()}
			r.attempts = append(r.attempts, attempt)
			held := r.hold
			r.mu.Unlock()
			if held {
				select {
				case r.held <- attempt:
				default:
				}
				http.Error(w, "synthetic pre-forward payout hold", 503)
				return
			}
		}
		up, e := http.NewRequestWithContext(q.Context(), http.MethodPost, upstream, bytes.NewReader(body))
		if e != nil {
			http.Error(w, "upstream", 502)
			return
		}
		up.SetBasicAuth("fixture", "public-pool-actual")
		up.Header.Set("Content-Type", "application/json")
		res, e := r.client.Do(up)
		if e != nil {
			http.Error(w, "upstream", 502)
			return
		}
		reply, e := io.ReadAll(io.LimitReader(res.Body, (9<<20)+1))
		ce := res.Body.Close()
		if e != nil || ce != nil || len(reply) > 9<<20 {
			http.Error(w, "reply", 502)
			return
		}
		if req.Method == "sendrawtransaction" && res.StatusCode == 200 {
			var envelope struct {
				Result string          `json:"result"`
				Error  json.RawMessage `json:"error"`
			}
			var rawHex string
			if json.Unmarshal(reply, &envelope) == nil && bytes.Equal(bytes.TrimSpace(envelope.Error), []byte("null")) && len(req.Params) > 0 && json.Unmarshal(req.Params[0], &rawHex) == nil {
				observed := lostBroadcast{Raw: rawHex, TxID: envelope.Result, AtMS: time.Now().UnixMilli()}
				r.mu.Lock()
				r.calls = append(r.calls, observed)
				drop := r.drop
				r.drop = false
				r.mu.Unlock()
				if drop {
					r.lost <- observed
					if h, ok := w.(http.Hijacker); ok {
						conn, _, he := h.Hijack()
						if he == nil {
							if closeErr := conn.Close(); closeErr != nil {
								r.mu.Lock()
								r.closeError = errors.Join(r.closeError, closeErr)
								r.mu.Unlock()
							}
							return
						}
					}
					http.Error(w, "accepted response intentionally lost", 502)
					return
				}
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(res.StatusCode)
		_, _ = w.Write(reply)
	})
	go func() { r.done <- r.server.Serve(l) }()
	return r
}
func (r *rpcRelay) sends() []lostBroadcast {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]lostBroadcast(nil), r.calls...)
}
func (r *rpcRelay) attempted() ([]lostBroadcast, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]lostBroadcast(nil), r.attempts...), r.attemptOverflow
}
func assertAttempts(r *rpcRelay, original string) {
	attempts, overflow := r.attempted()
	require(!overflow && len(attempts) > 0 && len(attempts) <= 16, "bounded attempted send inventory")
	for _, attempt := range attempts {
		require(attempt.Raw == original, "every attempted broadcast retains exact original signed bytes")
	}
}
func (r *rpcRelay) close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	r.closing = true
	r.mu.Unlock()
	e := r.server.Close()
	se := <-r.done
	r.handlers.Wait()
	r.client.CloseIdleConnections()
	if errors.Is(se, http.ErrServerClosed) {
		se = nil
	}
	r.mu.Lock()
	ce := r.closeError
	r.mu.Unlock()
	return errors.Join(e, se, ce)
}

type runningOperator struct {
	runtime                              *pooloperator.Runtime
	owner                                *operatorauthority.Owner
	wallet                               *wallet.PoolWalletOwner
	lifeCancel, custodyCancel, runCancel context.CancelFunc
	done                                 chan error
	joined, closed                       bool
}

func cancellationOnly(e error) bool {
	if e == nil || e == context.Canceled {
		return true
	}
	if many, ok := e.(interface{ Unwrap() []error }); ok {
		for _, x := range many.Unwrap() {
			if !cancellationOnly(x) {
				return false
			}
		}
		return true
	}
	if one, ok := e.(interface{ Unwrap() error }); ok {
		return cancellationOnly(one.Unwrap())
	}
	return false
}
func (r *runningOperator) stop() error {
	if r == nil || r.closed {
		return nil
	}
	if r.runCancel != nil {
		r.runCancel()
	}
	var e error
	if r.done != nil && !r.joined {
		select {
		case x := <-r.done:
			r.joined = true
			if !cancellationOnly(x) {
				e = errors.Join(e, x)
			}
		case <-time.After(90 * time.Second):
			return errors.New("runtime join timeout")
		}
	}
	if r.runtime != nil {
		if x := r.runtime.Close(); x != nil {
			return errors.Join(e, x)
		}
		r.runtime = nil
	}
	if r.wallet != nil {
		if x := r.wallet.Close(); x != nil {
			return errors.Join(e, x)
		}
		r.wallet = nil
	}
	if r.owner != nil {
		if x := r.owner.Close(); x != nil {
			return errors.Join(e, x)
		}
		r.owner = nil
	}
	if r.lifeCancel != nil {
		r.lifeCancel()
	}
	if r.custodyCancel != nil {
		r.custodyCancel()
	}
	r.closed = true
	return e
}
func constructRuntime(ctx context.Context, root, private string, s pooloperator.Setup, rpcConfig prospectnode.RPCConfig, create bool) (r *runningOperator) {
	r = &runningOperator{}
	ok := false
	defer func() {
		if !ok {
			check(r.stop(), "partial runtime cleanup")
		}
	}()
	custody, cc := context.WithCancel(context.Background())
	r.custodyCancel = cc
	var e error
	r.owner, e = operatorauthority.OpenOnline(custody, fixtureAuthorityBinding(s), operatorauthority.OnlineSeedFiles{Admission: filepath.Join(private, "admission.seed"), Publication: filepath.Join(private, "publication.seed"), Cutoff: filepath.Join(private, "cutoff.seed")})
	check(e, "online fixture custody")
	if create {
		seed := recovery.SeedFromEntropy([32]byte{})
		master, x := hdkeychain.NewMaster(seed[:], params)
		clear(seed[:])
		check(x, "published zero256 BIP39 root")
		r.wallet, e = wallet.CreatePoolSettlementWallet(custody, filepath.Join(root, "wallet16"), fixtureBinding(s), []byte(publicPass), []byte(privatePass), master, time.Now().Add(-24*time.Hour))
		master.Zero()
	} else {
		r.wallet, e = wallet.OpenPoolSettlementWallet(custody, filepath.Join(root, "wallet16"), fixtureBinding(s), []byte(publicPass))
	}
	check(e, "dedicated wallet16")
	life, lc := context.WithCancel(ctx)
	r.lifeCancel = lc
	selected := pooloperator.PrivateInputs{NodeRPC: rpcConfig, TLSCertificateFile: filepath.Join(private, "tls-cert.pem"), TLSPrivateKeyFile: filepath.Join(private, "tls-key.pem"), UnlockPassphraseFile: filepath.Join(private, "unlock.pass")}
	borrowed := pooloperator.BorrowedOwners{Authority: r.owner, Wallet: r.wallet}
	if create {
		r.runtime, e = pooloperator.Create(life, filepath.Join(root, "operator"), s, borrowed, selected)
	} else {
		r.runtime, e = pooloperator.Open(life, filepath.Join(root, "operator"), s, borrowed, selected)
	}
	check(e, "exact runtime construction")
	require(!r.owner.Capabilities().Checkpoint, "online custody has no checkpoint role")
	ok = true
	return
}
func (r *runningOperator) run(ctx context.Context, origin string) {
	work, cancel := context.WithCancel(ctx)
	r.runCancel = cancel
	r.done = make(chan error, 1)
	ready := make(chan string, 1)
	go func() { r.done <- r.runtime.Run(work, func(s string) { ready <- s }) }()
	select {
	case s := <-ready:
		require(s == origin, "exact ingress ready")
	case e := <-r.done:
		r.joined = true
		check(e, "runtime exited before ready")
		panic("runtime stopped before ready")
	case <-ctx.Done():
		panic("runtime ready deadline")
	case <-time.After(60 * time.Second):
		panic("bounded runtime readiness")
	}
}

// Every subprocess is an exact owned handle. Signals are never selected by name.
type childProcess struct {
	name              string
	cmd               *exec.Cmd
	done              chan error
	log               *os.File
	joined, forced    bool
	waitErr, closeErr string
}

func startChild(binary, name, logPath string, args ...string) *childProcess {
	f, e := os.OpenFile(logPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	check(e, "child log")
	c := &childProcess{name: name, cmd: exec.Command(binary, args...), done: make(chan error, 1), log: f}
	c.cmd.Stdout = &cappedLog{f: f}
	c.cmd.Stderr = c.cmd.Stdout
	if e = c.cmd.Start(); e != nil {
		ce := f.Close()
		check(errors.Join(e, ce), "child start/close")
	}
	go func() { c.done <- c.cmd.Wait() }()
	return c
}
func (c *childProcess) collect(e error) {
	c.joined = true
	if e != nil {
		c.waitErr = e.Error()
	}
	if e = c.log.Close(); e != nil {
		c.closeErr = e.Error()
	}
}
func (c *childProcess) wait(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case e := <-c.done:
		c.collect(e)
		check(e, "child completed")
	case <-ctx.Done():
		panic("child caller canceled")
	case <-timer.C:
		panic("child deadline")
	}
}
func (c *childProcess) stop() {
	if c == nil || c.joined {
		return
	}
	select {
	case e := <-c.done:
		c.collect(e)
		return
	default:
	}
	_ = c.cmd.Process.Signal(syscall.SIGTERM)
	select {
	case e := <-c.done:
		c.collect(e)
	case <-time.After(40 * time.Second):
		c.forced = true
		_ = c.cmd.Process.Kill()
		select {
		case e := <-c.done:
			c.collect(e)
		case <-time.After(5 * time.Second):
		}
	}
}
func (c *childProcess) clean() bool {
	return c.joined && !c.forced && c.waitErr == "" && c.closeErr == ""
}
func (c *childProcess) evidence() any {
	return map[string]any{"name": c.name, "pid": c.cmd.Process.Pid, "joined": c.joined, "forced": c.forced, "wait_error": c.waitErr, "log_close_error": c.closeErr, "clean": c.clean()}
}

func clientFor(s pooloperator.Setup, c publicFixtureMinerConfig) *poolservice.Client {
	client, e := poolservice.NewClient(poolservice.ClientConfig{Version: 1, URL: s.Ingress.Origin, Token: c.PoolToken, CACertificate: c.CACertificate, PublicKey: c.GymPublicKey, Source: s.Pool})
	check(e, "pinned TLS read client")
	return client
}
func pollBalances(ctx context.Context, clients []*poolservice.Client, want func(poolservice.BalanceResponse) bool, d time.Duration) []poolservice.BalanceResponse {
	out := make([]poolservice.BalanceResponse, len(clients))
	wait(ctx, d, func() bool {
		for i, c := range clients {
			v, e := c.Balance(ctx)
			if e != nil || !want(v) {
				return false
			}
			out[i] = v
		}
		return true
	})
	return out
}
func pollPayouts(ctx context.Context, clients []*poolservice.Client, txid string, d time.Duration) []poolservice.PayoutHistoryResponse {
	out := make([]poolservice.PayoutHistoryResponse, len(clients))
	wait(ctx, d, func() bool {
		for i, c := range clients {
			v, e := c.Payouts(ctx)
			if e != nil || len(v.History.Items) != 1 {
				return false
			}
			item := v.History.Items[0]
			if item.TxID != txid || item.Atoms != minerPayment || item.RecordedState != "confirmed" || v.History.Balance.PaidAtoms != minerPayment {
				return false
			}
			out[i] = v
		}
		return true
	})
	return out
}
func runtimeStatus(ctx context.Context, r *pooloperator.Runtime) pooloperator.Status {
	var s pooloperator.Status
	wait(ctx, 20*time.Second, func() bool {
		v, e := r.Status(ctx)
		if e == nil {
			s = v
			return true
		}
		return false
	})
	return s
}
func validatePayment(rawHex string, s pooloperator.Setup, inputs []wire.OutPoint) map[string]any {
	b := dec(rawHex)
	require(len(b) < poolsettle.MaxTransactionBytes, "signed payout cap")
	var tx wire.MsgTx
	reader := bytes.NewReader(b)
	check(tx.Deserialize(reader), "signed payout decode")
	require(reader.Len() == 0 && len(tx.TxIn) == 2 && len(tx.TxOut) == 3, "two original inputs/two payments+change")
	expected := map[wire.OutPoint]bool{}
	for _, op := range inputs {
		expected[op] = true
	}
	for _, in := range tx.TxIn {
		require(expected[in.PreviousOutPoint], "original pool coinbase input")
		delete(expected, in.PreviousOutPoint)
	}
	require(len(expected) == 0, "all original pool inputs")
	payees := map[string]bool{}
	for _, p := range s.Settlement.Destinations {
		payees[p.Script] = true
	}
	var outputs int64
	var change int64
	for _, o := range tx.TxOut {
		outputs += o.Value
		script := hx(o.PkScript)
		if script == s.Settlement.ChangeScript {
			require(change == 0, "one pool change")
			change = o.Value
		} else {
			require(payees[script] && o.Value == minerPayment, "fixed9.9SLUG miner principal")
			delete(payees, script)
		}
	}
	fee := int64(2*1000000000) - outputs
	require(len(payees) == 0 && fee > 0 && fee <= s.Node.Fee.MaxFeeAtoms && change == 20000000-fee && change > 0, "network fee only operator two1percent cuts")
	return map[string]any{"txid": tx.TxHash().String(), "fee_atoms": fee, "change_atoms": change, "miner_each_atoms": minerPayment, "raw_sha256": hx(digest(b)), "inputs": inputs}
}
func cliArgs(command, root, private string) []string {
	return []string{command, "--setup-file", filepath.Join(private, "selected-setup.json"), "--root", filepath.Join(root, "operator"), "--wallet-root", filepath.Join(root, "wallet16"), "--wallet-public-passphrase-file", filepath.Join(private, "public.pass"), "--wallet-unlock-passphrase-file", filepath.Join(private, "unlock.pass"), "--admission-seed-file", filepath.Join(private, "admission.seed"), "--publication-seed-file", filepath.Join(private, "publication.seed"), "--cutoff-seed-file", filepath.Join(private, "cutoff.seed"), "--rpc-config-file", filepath.Join(private, "rpc.json"), "--tls-certificate-file", filepath.Join(private, "tls-cert.pem"), "--tls-key-file", filepath.Join(private, "tls-key.pem")}
}
func cliStatusComplete(path, setupID string) bool {
	f, e := os.Open(path)
	if e != nil {
		return false
	}
	defer f.Close()
	scan := bufio.NewScanner(io.LimitReader(f, 1<<20))
	scan.Buffer(make([]byte, 4096), 65536)
	for scan.Scan() {
		var row struct {
			Version uint32              `json:"version"`
			Command string              `json:"command"`
			Code    string              `json:"code"`
			Result  pooloperator.Status `json:"result"`
		}
		if json.Unmarshal(scan.Bytes(), &row) == nil && row.Version == 1 && row.Command == "status" && row.Code == "complete" && row.Result.SetupID == setupID && !row.Result.Running {
			return true
		}
	}
	return false
}
func cliListening(path, origin string) bool {
	f, e := os.Open(path)
	if e != nil {
		return false
	}
	defer f.Close()
	scan := bufio.NewScanner(io.LimitReader(f, 1<<20))
	scan.Buffer(make([]byte, 4096), 65536)
	for scan.Scan() {
		var row struct {
			Code   string `json:"code"`
			Result struct {
				Origin string `json:"origin"`
			} `json:"result"`
		}
		if json.Unmarshal(scan.Bytes(), &row) == nil && row.Code == "listening" && row.Result.Origin == origin {
			return true
		}
	}
	return false
}

func runActual(ctx context.Context, nodeBinary, operatorBinary, root, reference string) (out map[string]any) {
	out = map[string]any{"version": 1, "pass": false, "scope": "isolated full-intake unseen D80 winner and immutable pending payout", "public_seed_mining": false}
	started := time.Now()
	var nodes []*node
	var children []*childProcess
	var running *runningOperator
	var relay *rpcRelay
	var clients []*poolservice.Client
	defer func() {
		if failure := recover(); failure != nil {
			out["failure"] = fmt.Sprintf("%.1024s", fmt.Sprint(failure))
			out["phase"] = phase
		}
		for _, c := range clients {
			c.Close()
		}
		for i := len(children) - 1; i >= 0; i-- {
			children[i].stop()
		}
		cleanup := []string{}
		if running != nil {
			if e := running.stop(); e != nil {
				cleanup = append(cleanup, fmt.Sprintf("runtime: %.512s", e))
			}
		}
		if relay != nil {
			if e := relay.close(); e != nil {
				cleanup = append(cleanup, fmt.Sprintf("proxy: %.256s", e))
			}
			out["broadcasts"] = relay.sends()
			attempts, overflow := relay.attempted()
			out["broadcast_attempts"] = attempts
			out["broadcast_attempt_overflow"] = overflow
		}
		rows := []any{}
		clean := len(cleanup) == 0
		for i := len(nodes) - 1; i >= 0; i-- {
			nodes[i].stop()
			rows = append(rows, nodes[i].cleanupReport())
			clean = clean && nodes[i].clean()
		}
		out["node_cleanup"] = rows
		childRows := []any{}
		for _, c := range children {
			childRows = append(childRows, c.evidence())
			clean = clean && c.clean()
		}
		out["child_cleanup"] = childRows
		out["cleanup_errors"] = cleanup
		out["clean_join"] = clean
		out["elapsed_seconds"] = time.Since(started).Seconds()
		out["pass"] = out["completed"] == true && clean
		write(filepath.Join(root, "actual-report.json"), out)
	}()
	require(!wire.TestPoW, "normal PoW only")
	require(filepath.IsAbs(root) && filepath.IsAbs(nodeBinary) && filepath.IsAbs(operatorBinary) && filepath.IsAbs(reference), "explicit absolute inputs")
	check(os.Mkdir(root, 0700), "fresh isolated artifact root")
	private := filepath.Join(root, "private-fixture")
	check(os.Mkdir(private, 0700), "private fixture directory")
	original := poolSetup(reference)
	s := original
	now := time.Now().UTC()
	require(now.Hour() != 23 || now.Minute() < 40, "15minute fixture must not cross UTC day")
	due := now.Truncate(time.Minute).Add(3 * time.Minute)
	s.Schedule.DailyUTCMinute = uint32(due.Hour()*60 + due.Minute())
	s.Schedule.RetryMS = 30000
	s.MaintenanceIntervalMS = 1000
	cert, _, spki := fixtureTLS(private)
	ports := uniquePorts(4)
	s.Ingress = poolingress.Config{Version: 1, Listen: ports[0], Origin: "https://" + ports[0], CertificateSPKI: spki, PoolURL: "http://" + ports[1], AdmissionURL: "http://" + ports[2], OwnershipURL: "http://" + ports[3]}
	check(pooloperator.ValidateSetup(s), "dynamic transport/schedule setup")
	require(reflect.DeepEqual(s.Authority, original.Authority) && reflect.DeepEqual(s.Pool, original.Pool) && reflect.DeepEqual(s.Settlement, original.Settlement) && reflect.DeepEqual(s.WalletGuard, original.WalletGuard), "unchanged independent authority/economics/wallet input")
	canonical, e := pooloperator.MarshalSetup(s)
	check(e, "selected setup")
	privateWrite(filepath.Join(private, "selected-setup.json"), canonical)
	write(filepath.Join(root, "selected-public-setup.json"), s)
	for _, role := range []string{"admission", "publication", "cutoff"} {
		seed := sha256.Sum256([]byte("SLUG set74 public synthetic operator " + role + "\x00v1\x00"))
		privateWrite(filepath.Join(private, role+".seed"), seed[:])
		clear(seed[:])
	}
	privateWrite(filepath.Join(private, "public.pass"), []byte(publicPass))
	privateWrite(filepath.Join(private, "unlock.pass"), []byte(privatePass))
	bootstrap := poolsettlefixture.PoolAddress()
	require(bootstrap != s.Pool.PayoutAddress, "bootstrap funds cannot enter operator wallet")
	phase = "fresh two normal loopback nodes"
	p2p := uniquePorts(2)
	a := startNode(ctx, nodeBinary, root, "node-primary", p2p[0], p2p[1], bootstrap)
	nodes = append(nodes, a)
	b := startNode(ctx, nodeBinary, root, "node-peer", p2p[1], p2p[0], bootstrap)
	nodes = append(nodes, b)
	ready(ctx, a)
	ready(ctx, b)
	wait(ctx, 30*time.Second, func() bool { var p []any; return a.rpc.call(ctx, "getpeerinfo", nil, &p) == nil && len(p) > 0 })
	peers(ctx, a)
	peers(ctx, b)
	var initialHeight uint32
	a.rpc.must(ctx, "getblockcount", nil, &initialHeight)
	require(initialHeight == 0, "fresh node genesis")
	bootHash := generate(ctx, a, b)
	out["bootstrap"] = map[string]any{"hash": bootHash, "height": 1, "payout_address": bootstrap, "credited": false}
	relay = startRPCRelay(ctx, a.rpc.endpoint)
	rpcConfig := prospectnode.RPCConfig{Version: 1, URL: relay.url, Username: "fixture", Password: "public-pool-actual"}
	rb, e := json.Marshal(rpcConfig)
	check(e, "fixture RPC")
	privateWrite(filepath.Join(private, "rpc.json"), rb)
	phase = "construct exact operator runtime"
	running = constructRuntime(ctx, root, private, s, rpcConfig, true)
	configs := make([]publicFixtureMinerConfig, 2)
	require(len(s.Miners.Enrollments) == 2, "two fixed enrollments")
	for i, en := range s.Miners.Enrollments {
		poolToken := hx(digest([]byte(fmt.Sprintf("Set74 public rehearsal pool token %d", i))))
		expiry := time.Now().Add(time.Hour).UnixMilli()
		_, e = running.runtime.RotateCredential(ctx, en.GymID, poolminer.PoolAudience, 0, poolToken, expiry)
		check(e, "pool audience credential")
		configs[i] = publicFixtureMinerConfig{Setup: s, RPC: prospectnode.RPCConfig{Version: 1, URL: a.rpc.endpoint, Username: "fixture", Password: "public-pool-actual"}, CACertificate: cert, GymPublicKey: en.PublicKey, PoolToken: poolToken}
	}
	running.run(ctx, s.Ingress.Origin)
	for _, c := range configs {
		clients = append(clients, clientFor(s, c))
	}
	write(filepath.Join(root, "01-runtime-listening.json"), runtimeStatus(ctx, running.runtime))
	phase = "two deterministic block-only original TLS submissions"
	poolInputs, reports := lateBootstrap(ctx, s, a, b, running, configs, clients)
	highest := uint32(3)
	write(filepath.Join(root, "02-two-blocks.json"), reports)
	phase = "sixty descendants then due immature deferral"
	for {
		var h uint32
		a.rpc.must(ctx, "getblockcount", nil, &h)
		if h >= highest+60 {
			break
		}
		generate(ctx, a, b)
	}
	lateWaitWall(ctx, due.UnixMilli(), "real UTC daily due")
	immature := pollBalances(ctx, clients, func(v poolservice.BalanceResponse) bool {
		return v.Balance.Confirmed.Numerator == "990000000" && v.Balance.Confirmed.Denominator == "1" && v.Balance.Available.Numerator == "0" && v.Balance.Reason == "immature" && v.Balance.PaidAtoms == 0 && v.Balance.ReservedAtoms == 0
	}, 3*time.Minute)
	dueObservedMS := time.Now().UnixMilli()
	require(dueObservedMS >= due.UnixMilli() && len(relay.sends()) == 0, fmt.Sprintf("due reached without premature broadcast: observed_wall_ms=%d target_wall_ms=%d", dueObservedMS, due.UnixMilli()))
	var st pooloperator.Status
	wait(ctx, 45*time.Second, func() bool {
		candidate, e := running.runtime.Status(ctx)
		if e != nil || candidate.Schedule == nil || candidate.Schedule.State.Attempt == nil {
			return false
		}
		attempt := candidate.Schedule.State.Attempt
		if attempt.Kind != "daily" || attempt.Result != "returned" || attempt.StartedAtMS < due.UnixMilli() || attempt.FinishedAtMS < attempt.StartedAtMS || attempt.ReportState != "deferred" {
			return false
		}
		st = candidate
		return true
	})

	write(filepath.Join(root, "03-due-immature.json"), map[string]any{"due": due, "status": st, "balances": immature, "broadcasts": 0})
	attempted, overflow := relay.attempted()
	require(len(attempted) == 0 && !overflow, "immature original funds never attempted broadcast")
	check(running.stop(), "stop scheduler while both pool coinbases remain immature")
	running = nil
	phase = "mature both original pool coinbases with scheduler stopped"
	for {
		var h uint32
		a.rpc.must(ctx, "getblockcount", nil, &h)
		if h >= highest+100 {
			break
		}
		generate(ctx, a, b)
	}
	running = constructRuntime(ctx, root, private, s, rpcConfig, false)
	running.run(ctx, s.Ingress.Origin)
	late := runLateWinner(ctx, root, private, operatorBinary, s, a, b, relay, &running, &children, configs, clients, poolInputs)
	for k, v := range late {
		out[k] = v
	}
	out["completed"] = true
	return out
}

func pauseActual(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
func main() {
	nodeBinary := flag.String("node", "", "exact normal node")
	operatorBinary := flag.String("operator-binary", "", "exact slugpoold")
	root := flag.String("artifacts", "", "fresh absolute owned root")
	reference := flag.String("setup-reference", "", "independent setup corpus")
	flag.Parse()
	signal.Ignore(syscall.SIGPIPE)
	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(signalCtx, 15*time.Minute)
	defer cancel()
	result := runActual(ctx, *nodeBinary, *operatorBinary, *root, *reference)
	fmt.Println("Set74 public pool rehearsal", result["pass"], phase)
	if result["pass"] != true {
		os.Exit(1)
	}
}
