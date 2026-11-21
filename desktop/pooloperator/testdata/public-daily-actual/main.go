// External finite Set79 DAILY rehearsal. All private material below is generated from
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
	"github.com/taytr-llc/slug/poolround"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/poolsettlefixture"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
	"github.com/taytr-llc/slugwallet/recovery"
	"github.com/taytr-llc/slugwallet/wallet"
)

const setupReferenceSHA = "a5c65673442846249b500d0db05c80652b5350ed1c032a586d275265698d25e3"
const economicsReferenceSHA = "1d2b1462296bd022c53df9dc583db3c38cd13a8e13e84f9983f2505b3aefb08f"
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

// Published public fixtures only. The mnemonic is never emitted in evidence.
func poolSetup(path string) (pooloperator.Setup, string) {
	raw := readBound(path, 4<<20)
	require(len(raw) == 3176583 && hx(digest(raw)) == setupReferenceSHA, "qualified public DAILY corpus")
	var corpus struct {
		Composition struct {
			Positives []struct {
				Name      string
				Setup     pooloperator.Setup
				Canonical string `json:"setup_hex"`
				ID        string `json:"setup_id"`
				Backup    string `json:"published_fixture_backup_hex"`
			}
		}
	}
	check(json.Unmarshal(raw, &corpus), "public DAILY composition")
	for _, v := range corpus.Composition.Positives {
		if v.Name != "two-enrollments" {
			continue
		}
		selected, e := pooloperator.ParseDailySetup(dec(v.Canonical))
		check(e, "canonical daily Setup")
		require(reflect.DeepEqual(selected, v.Setup), "exact independent Setup bytes")
		id, e := pooloperator.DailySetupID(selected)
		check(e, "daily Setup ID")
		require(id == v.ID, "independent daily Setup ID")
		reservation, e := pooloperator.DailyCapacityEnvelope(selected)
		check(e, "derived daily envelope")
		require(reservation.OrdinaryMaximum == 2 && selected.Pool.MaxSubmissions == 8 && selected.Pool.MaxWorks == 8 && selected.Settlement.MaxBatches == 6, "true ordinary two-row daily envelope")
		require(len(selected.Miners.Enrollments) == 2 && selected.Miners.Enrollments[0].GymID == "28452fc8a478d28f284dfe671cfc57e7cabeca08ea953aef371f5674d3f4c510" && selected.Miners.Enrollments[1].GymID == "a227587d837507dbcb5f9d781967339d816070fb7bd574931c690ba6937b88e5", "independent Gym1/Gym2 label map")
		var backup struct {
			Version  uint32
			Mnemonic string
			SetupID  string `json:"setup_id"`
		}
		check(json.Unmarshal(dec(v.Backup), &backup), "published synthetic wallet backup")
		require(backup.Version == 2 && backup.SetupID == v.ID && len(backup.Mnemonic) > 0, "backup original selection")
		return selected, backup.Mnemonic
	}
	panic("missing two-enrollments fixture")
}
func fixtureBinding(s pooloperator.Setup) wallet.PoolWalletBinding {
	return wallet.PoolWalletBinding{Version: 2, WalletOwnerID: s.WalletGuard.WalletOwnerID, OperatorProfileID: s.WalletGuard.OperatorProfileID, Source: s.Pool, Settlement: s.Settlement, Guard: s.WalletGuard}
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
	observeHold         bool
	observedTxID        string
	observationRefusals uint64

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
	r := &rpcRelay{url: "http://" + l.Addr().String(), listener: l, done: make(chan error, 1), lost: make(chan lostBroadcast, 1), drop: true, observeHold: true, client: &http.Client{Transport: &http.Transport{Proxy: nil, DisableCompression: true, DisableKeepAlives: true, MaxResponseHeaderBytes: 65536}, Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}
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
			r.mu.Unlock()
		}
		// Exact original A observation only. Forward every other method/raw.
		if req.Method == "getrawtransaction" && len(req.Params) == 2 {
			var txid string
			var verbose int
			if json.Unmarshal(req.Params[0], &txid) == nil && json.Unmarshal(req.Params[1], &verbose) == nil {
				r.mu.Lock()
				held := r.observeHold && r.observedTxID != "" && txid == r.observedTxID && verbose == 1
				if held {
					r.observationRefusals++
				}
				r.mu.Unlock()
				if held {
					http.Error(w, "published fixture A observation unavailable", 503)
					return
				}
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
				if drop {
					r.observedTxID = observed.TxID
				}
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
	runErr, stopErr                      error
	runObservedMS                        int64
	prematureExit                        bool
	lastStatus                           *pooloperator.Status
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

// Only the harness's serial scenario/cleanup path receives done. Retain the
// original joined result even when a wait detects completion before cleanup.
func (r *runningOperator) recordCompletion(e error, premature bool) {
	require(r != nil && !r.joined, "original runtime completion received once")
	r.joined, r.runErr, r.prematureExit, r.runObservedMS = true, e, premature, time.Now().UnixMilli()
}
func (r *runningOperator) checkRunning() {
	require(r != nil && r.done != nil, "original runtime was started")
	if !r.joined {
		select {
		case e := <-r.done:
			r.recordCompletion(e, true)
		default:
		}
	}
	if r.joined {
		check(r.runErr, "original runtime exited before requested stop")
		panic("original runtime exited nil before requested stop")
	}
}
func (r *runningOperator) status(ctx context.Context) pooloperator.Status {
	r.checkRunning()
	v := runtimeStatus(ctx, r.runtime)
	r.lastStatus = &v
	r.checkRunning()
	return v
}
func (r *runningOperator) completionEvidence() map[string]any {
	if r == nil {
		return nil
	}
	return map[string]any{"joined": r.joined, "owners_closed": r.closed, "premature": r.prematureExit,
		"observed_at_ms": r.runObservedMS, "error_present": r.runErr != nil, "error_type": fmt.Sprintf("%T", r.runErr),
		"cancellation_only": cancellationOnly(r.runErr), "round_busy": errors.Is(r.runErr, poolround.ErrBusy), "last_status": r.lastStatus}
}

// Same original wall target/context as lateWaitWall, with the already-owned
// completion channel added to the wait. No new timer or replacement deadline.
func (r *runningOperator) waitWall(ctx context.Context, targetMS int64, label string) int64 {
	for {
		r.checkRunning()
		observed := time.Now().UnixMilli()
		require(ctx.Err() == nil, fmt.Sprintf("%s canceled: observed_wall_ms=%d target_wall_ms=%d", label, observed, targetMS))
		if observed >= targetMS {
			return observed
		}
		d := time.Duration(targetMS-observed) * time.Millisecond
		if d < time.Millisecond {
			d = time.Millisecond
		}
		timer := time.NewTimer(d)
		select {
		case e := <-r.done:
			timer.Stop()
			r.recordCompletion(e, true)
			r.checkRunning()
		case <-ctx.Done():
			timer.Stop()
			panic(fmt.Sprintf("%s canceled: observed_wall_ms=%d target_wall_ms=%d", label, time.Now().UnixMilli(), targetMS))
		case <-timer.C:
		}
	}
}
func (r *runningOperator) stop() error {
	if r == nil {
		return nil
	}
	if r.closed {
		return r.stopErr
	}
	// Observe an already-published completion before requesting shutdown.
	// In particular, a premature nil return must not become a clean stop.
	if r.done != nil && !r.joined {
		select {
		case x := <-r.done:
			r.recordCompletion(x, true)
		default:
		}
	}
	if r.runCancel != nil {
		r.runCancel()
	}
	var e error
	if r.done != nil && !r.joined {
		select {
		case x := <-r.done:
			r.recordCompletion(x, false)
		case <-time.After(90 * time.Second):
			return errors.New("runtime join timeout")
		}
	}
	if !cancellationOnly(r.runErr) {
		e = r.runErr
	}
	if r.prematureExit && cancellationOnly(r.runErr) {
		e = errors.Join(e, errors.New("runtime exited before requested stop"))
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
	r.stopErr = e
	return e
}
func constructRuntime(ctx context.Context, root, private string, s pooloperator.Setup, rpcConfig prospectnode.RPCConfig, create bool, mnemonic string) (r *runningOperator) {
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
		seed, seedErr := recovery.Seed(mnemonic)
		check(seedErr, "published synthetic BIP39 seed")
		master, x := hdkeychain.NewMaster(seed[:], params)
		clear(seed[:])
		check(x, "published selected BIP39 root")
		r.wallet, e = wallet.CreateDailyPoolSettlementWallet(custody, filepath.Join(root, "wallet16"), fixtureBinding(s), []byte(publicPass), []byte(privatePass), master, time.Now().Add(-24*time.Hour))
		master.Zero()
	} else {
		r.wallet, e = wallet.OpenDailyPoolSettlementWallet(custody, filepath.Join(root, "wallet16"), fixtureBinding(s), []byte(publicPass))
	}
	check(e, "dedicated wallet16")
	life, lc := context.WithCancel(ctx)
	r.lifeCancel = lc
	selected := pooloperator.PrivateInputs{NodeRPC: rpcConfig, TLSCertificateFile: filepath.Join(private, "tls-cert.pem"), TLSPrivateKeyFile: filepath.Join(private, "tls-key.pem"), UnlockPassphraseFile: filepath.Join(private, "unlock.pass")}
	borrowed := pooloperator.BorrowedOwners{Authority: r.owner, Wallet: r.wallet}
	if create {
		r.runtime, e = pooloperator.CreateDaily(life, filepath.Join(root, "operator"), s, borrowed, selected)
	} else {
		r.runtime, e = pooloperator.OpenDaily(life, filepath.Join(root, "operator"), s, borrowed, selected)
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
		r.checkRunning()
	case e := <-r.done:
		r.recordCompletion(e, true)
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
	recipient := ""
	for _, e := range s.Miners.Enrollments {
		if e.PublicKey == c.GymPublicKey {
			recipient = e.PayeeScript
		}
	}
	client, e := poolservice.NewDailyClient(poolservice.DailyClientConfig{Version: 2, URL: s.Ingress.Origin, Token: c.PoolToken, CACertificate: c.CACertificate, PublicKey: c.GymPublicKey, Source: s.Pool, SettlementConfigID: s.WalletGuard.SettlementConfigID, RecipientScript: recipient})
	check(e, "original DAILY TLS client")
	require(client.ProtocolVersion() == 2, "daily client mode")
	return client
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
func validatePayment(rawHex string, s pooloperator.Setup, input wire.OutPoint, economics actualEconomics) map[string]any {
	b := dec(rawHex)
	require(len(b) < poolsettle.MaxTransactionBytes, "one-input signed A cap")
	var tx wire.MsgTx
	reader := bytes.NewReader(b)
	check(tx.Deserialize(reader), "signed A decode")
	require(reader.Len() == 0 && len(tx.TxIn) == 1 && len(tx.TxOut) == 2 && tx.TxIn[0].PreviousOutPoint == input, "A only original pool coinbase input")
	require(hx(tx.TxOut[0].PkScript) == s.Miners.Enrollments[0].PayeeScript && tx.TxOut[0].Value == economics.APayment && hx(tx.TxOut[1].PkScript) == s.Settlement.ChangeScript && tx.TxOut[1].Value == economics.AChange, "A independent exact miner payment and change")
	require(economics.AReward-tx.TxOut[0].Value-tx.TxOut[1].Value == economics.AFee, "A independently derived network fee")
	return map[string]any{"txid": tx.TxHash().String(), "fee_atoms": economics.AFee, "change_atoms": economics.AChange, "miner_atoms": economics.APayment, "raw_sha256": hx(digest(b)), "input": input}
}
func cliArgs(command, root, private string) []string {
	return []string{command, "--daily", "--setup-file", filepath.Join(private, "selected-setup.json"), "--root", filepath.Join(root, "operator"), "--wallet-root", filepath.Join(root, "wallet16"), "--wallet-public-passphrase-file", filepath.Join(private, "public.pass"), "--wallet-unlock-passphrase-file", filepath.Join(private, "unlock.pass"), "--admission-seed-file", filepath.Join(private, "admission.seed"), "--publication-seed-file", filepath.Join(private, "publication.seed"), "--cutoff-seed-file", filepath.Join(private, "cutoff.seed"), "--rpc-config-file", filepath.Join(private, "rpc.json"), "--tls-certificate-file", filepath.Join(private, "tls-cert.pem"), "--tls-key-file", filepath.Join(private, "tls-key.pem")}
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

// The existing qualified arithmetic model supplies expectations. Gym labels map
// to the selected, sorted two-enrollment Setup; no source IDs or payees change.
type expectedBalance struct {
	Gym       string              `json:"gym"`
	Confirmed poolsettle.Fraction `json:"confirmed"`
	Available poolsettle.Fraction `json:"available"`
	Reserved  int64               `json:"reserved_atoms"`
	Paid      int64               `json:"paid_atoms"`
	Reason    string              `json:"reason"`
}
type expectedReport struct {
	State    string            `json:"state"`
	Reason   string            `json:"reason"`
	Balances []expectedBalance `json:"balances"`
}
type actualEconomics struct {
	AReward   int64                     `json:"a_reward_atoms"`
	BReward   int64                     `json:"b_reward_atoms"`
	AFee      int64                     `json:"a_fee_atoms"`
	APayment  int64                     `json:"a_payment_atoms"`
	AChange   int64                     `json:"a_change_atoms"`
	BNet      []poolsettle.Fraction     `json:"round_b_net_by_gym"`
	BGross    []poolsettle.Fraction     `json:"round_b_gross_by_gym"`
	BOperator poolsettle.Fraction       `json:"operator_b_earned"`
	Reports   map[string]expectedReport `json:"reports"`
}

func readEconomics(path string) actualEconomics {
	raw := readBound(path, 1<<20)
	require(len(raw) == 29980 && hx(digest(raw)) == economicsReferenceSHA, "qualified independent Set78 economic recipe")
	var v struct {
		Economics actualEconomics `json:"actual_economics"`
	}
	check(json.Unmarshal(raw, &v), "independent actual arithmetic")
	require(v.Economics.AReward == 1000000000 && v.Economics.BReward == 1000000141 && v.Economics.AFee == 141 && len(v.Economics.BNet) == 2 && len(v.Economics.Reports) == 4, "exact one-input actual scenario, not kernel economics")
	return v.Economics
}
func sameDay(day string) {
	require(time.Now().UTC().Format("2006-01-02") == day, "original actual UTC day unchanged")
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

func runActual(ctx context.Context, nodeBinary, operatorBinary, root, reference, economicsPath string) (out map[string]any) {
	out = map[string]any{"version": 2, "pass": false, "scope": "finite public DAILY original TLS/operator/UTC/schema16 late winner and immutable pending payout", "public_seed_mining": false, "setup_reference_sha256": setupReferenceSHA, "economics_reference_sha256": economicsReferenceSHA}
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
			out["runtime_completion"] = running.completionEvidence()
		}
		if relay != nil {
			if e := relay.close(); e != nil {
				cleanup = append(cleanup, fmt.Sprintf("proxy: %.256s", e))
			}
			out["broadcasts"] = relay.sends()
			attempts, overflow := relay.attempted()
			out["broadcast_attempts"] = attempts
			out["broadcast_attempt_overflow"] = overflow
			relay.mu.Lock()
			out["observation_fault"] = map[string]any{"method": "getrawtransaction", "verbose": 1, "txid": relay.observedTxID, "refusals": relay.observationRefusals, "released": !relay.observeHold}
			relay.mu.Unlock()
		}
		clean := len(cleanup) == 0
		rows := []any{}
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
	for _, p := range []string{root, nodeBinary, operatorBinary, reference, economicsPath} {
		require(filepath.IsAbs(p), "explicit absolute selected inputs")
	}
	check(os.Mkdir(root, 0700), "fresh actual root")
	balanceWaits := newDailyBalanceWaitEvidence(root)
	private := filepath.Join(root, "private-fixture")
	check(os.Mkdir(private, 0700), "private fixture directory")
	original, mnemonic := poolSetup(reference)
	economics := readEconomics(economicsPath)
	s := original
	now := time.Now().UTC()
	require(now.Hour() != 23 || now.Minute() < 40, "900-second fixture cannot start near UTC midnight")
	day := now.Format("2006-01-02")
	due := now.Truncate(time.Minute).Add(3 * time.Minute)
	s.Schedule.DailyUTCMinute = uint32(due.Hour()*60 + due.Minute())
	cert, _, spki := fixtureTLS(private)
	ports := uniquePorts(4)
	s.Ingress = poolingress.Config{Version: 2, Listen: ports[0], Origin: "https://" + ports[0], CertificateSPKI: spki, PoolURL: "http://" + ports[1], AdmissionURL: "http://" + ports[2], OwnershipURL: "http://" + ports[3]}
	check(pooloperator.ValidateDailySetup(s), "selected dynamic DAILY transport/schedule")
	require(reflect.DeepEqual(s.Authority, original.Authority) && reflect.DeepEqual(s.Pool, original.Pool) && reflect.DeepEqual(s.Settlement, original.Settlement) && reflect.DeepEqual(s.WalletGuard, original.WalletGuard) && reflect.DeepEqual(s.RoundLimits, original.RoundLimits) && reflect.DeepEqual(s.Miners, original.Miners) && s.MaintenanceIntervalMS == original.MaintenanceIntervalMS && s.Schedule.RetryMS == original.Schedule.RetryMS, "fixed original source/economics/keys/caps/intervals")
	canonical, e := pooloperator.MarshalDailySetup(s)
	check(e, "selected daily Setup bytes")
	privateWrite(filepath.Join(private, "selected-setup.json"), canonical)
	write(filepath.Join(root, "selected-public-setup.json"), s)
	out["selected_day"] = day
	out["due_ms"] = due.UnixMilli()
	for _, role := range []string{"admission", "publication", "cutoff"} {
		seed := sha256.Sum256([]byte("SLUG set74 public synthetic operator " + role + "\x00v1\x00"))
		privateWrite(filepath.Join(private, role+".seed"), seed[:])
		clear(seed[:])
	}
	privateWrite(filepath.Join(private, "public.pass"), []byte(publicPass))
	privateWrite(filepath.Join(private, "unlock.pass"), []byte(privatePass))
	bootstrap := poolsettlefixture.PoolAddress()
	require(bootstrap != s.Pool.PayoutAddress, "fixture bootstrap funds excluded")
	phase = "fresh two isolated normal nodes"
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
	var height uint32
	a.rpc.must(ctx, "getblockcount", nil, &height)
	require(height == 0, "fresh genesis")
	boot := generate(ctx, a, b)
	out["bootstrap"] = map[string]any{"hash": boot, "height": 1, "payout_address": bootstrap, "credited": false}
	relay = startRPCRelay(ctx, a.rpc.endpoint)
	rpcConfig := prospectnode.RPCConfig{Version: 1, URL: relay.url, Username: "fixture", Password: "public-pool-actual"}
	rpcRaw, e := json.Marshal(rpcConfig)
	check(e, "RPC config")
	privateWrite(filepath.Join(private, "rpc.json"), rpcRaw)
	phase = "original DAILY owner construction"
	running = constructRuntime(ctx, root, private, s, rpcConfig, true, mnemonic)
	mnemonic = ""
	configs := make([]publicFixtureMinerConfig, 2)
	for i, en := range s.Miners.Enrollments {
		token := hx(digest([]byte(fmt.Sprintf("Set79 public DAILY fixture pool token %d", i))))
		_, e = running.runtime.RotateCredential(ctx, en.GymID, poolminer.PoolAudience, 0, token, time.Now().Add(time.Hour).UnixMilli())
		check(e, "daily pool credential")
		configs[i] = publicFixtureMinerConfig{Setup: s, RPC: prospectnode.RPCConfig{Version: 1, URL: a.rpc.endpoint, Username: "fixture", Password: "public-pool-actual"}, CACertificate: cert, GymPublicKey: en.PublicKey, PoolToken: token}
	}
	running.run(ctx, s.Ingress.Origin)
	for _, c := range configs {
		clients = append(clients, clientFor(s, c))
	}
	write(filepath.Join(root, "01-runtime-listening.json"), running.status(ctx))
	phase = "A original TLS job and one canonical reward"
	workA := lateIssue(ctx, clients[0], running)
	require(workA.Job.Context.ParentHeight == 1, "A original parent height")
	winnerA, _ := dailyFind(ctx, s, configs[0], workA, true, false)
	coinA := dailyPublish(ctx, s, a, b, winnerA, 2, economics.AReward, "")
	receiptA, e := lateSubmit(ctx, clients[0], workA, winnerA.Nonce)
	check(e, "A canonical TLS submission")
	require(receiptA.Sequence == 1 && receiptA.AcceptedSequence == 1 && receiptA.Block && receiptA.WorkHash == winnerA.WorkHash, "A unique first accepted row")
	write(filepath.Join(root, "02-A-block.json"), map[string]any{"work": workA, "receipt": receiptA, "coin": coinA, "height": 2})
	phase = "mature A then original real UTC due"
	for height = 2; height < 102; height++ {
		running.checkRunning()
		generate(ctx, a, b)
	}
	dueObserved := running.waitWall(ctx, due.UnixMilli(), "original real UTC due")
	sameDay(day)
	var lost lostBroadcast
	select {
	case lost = <-relay.lost:
		running.checkRunning()
	case e := <-running.done:
		running.recordCompletion(e, true)
		running.checkRunning()
	case <-ctx.Done():
		panic("A payout canceled")
	case <-time.After(90 * time.Second):
		panic("A scheduler did not broadcast after actual due")
	}
	payment := validatePayment(lost.Raw, s, coinA, economics)
	require(payment["txid"] == lost.TxID, "accepted original A txid")
	status := running.status(ctx)
	pendingBalances := dailyWaitBalances(ctx, balanceWaits, clients, economics, "a_lost_reply", 0, 45*time.Second, false)
	require(status.Schedule != nil && status.Schedule.State.Attempt != nil && status.Schedule.State.Attempt.Kind == "daily" && status.Schedule.State.Attempt.StartedAtMS >= due.UnixMilli(), "actual scheduler daily attempt, never manual RunDaily")
	check(running.stop(), "join original pending A owner")
	out["original_runtime_completion"] = running.completionEvidence()
	running = nil
	first := lateClosedAudit(ctx, root, s)
	require(first.Submissions == 1 && first.Accepted == 1 && first.Works == 1 && first.AuthorityJobs == 1 && first.Births == 0 && len(first.Rounds) == 1 && len(first.Credits) == 1 && first.Header.PendingBatch == 1 && first.Delivery == "prepared" && first.Body.Raw == lost.Raw && first.Body.TxID == lost.TxID, "original synchronized pending A")
	require(first.Report.Balances[0].ReservedAtoms == economics.APayment && first.Report.Balances[0].PaidAtoms == 0, "original A remains reserved")
	assertAttempts(relay, lost.Raw)
	require(len(relay.sends()) == 1, "one accepted A send")
	write(filepath.Join(root, "03-A-pending.json"), map[string]any{"audit": first, "payment": payment, "balances": pendingBalances, "status": status, "due_ms": due.UnixMilli(), "due_observed_ms": dueObserved})
	result := runDailyLate(ctx, root, private, operatorBinary, s, a, b, relay, &running, &children, configs, clients, rpcConfig, first, economics, day, balanceWaits)
	for k, v := range result {
		out[k] = v
	}
	sameDay(day)
	out["completed"] = true
	return
}
func main() {
	nodeBinary := flag.String("node", "", "exact normal node")
	operatorBinary := flag.String("operator-binary", "", "exact production daily slugpoold")
	root := flag.String("artifacts", "", "fresh absolute owned root")
	reference := flag.String("setup-reference", "", "qualified public DAILY corpus")
	economics := flag.String("economics-reference", "", "qualified actual economic recipe")
	flag.Parse()
	signal.Ignore(syscall.SIGPIPE)
	life, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(life, 900*time.Second)
	defer cancel()
	result := runActual(ctx, *nodeBinary, *operatorBinary, *root, *reference, *economics)
	fmt.Println("Set79 public DAILY rehearsal", result["pass"], phase)
	if result["pass"] != true {
		os.Exit(1)
	}
}
