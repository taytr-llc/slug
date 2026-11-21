// Extracted from frozen Path B; no observation/broadcast fault is included.
package main

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"github.com/taytr-llc/slug/btcutil/v2/hdkeychain"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/desktop/pooloperator"
	"github.com/taytr-llc/slug/internal/operatorauthority"
	"github.com/taytr-llc/slug/poolround"
	"github.com/taytr-llc/slug/poolschedule"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
	"github.com/taytr-llc/slugwallet/recovery"
	"github.com/taytr-llc/slugwallet/wallet"
	_ "github.com/taytr-llc/slugwallet/walletdb/bdb"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"time"
)

const publicPass = "published pool wallet public passphrase"
const privatePass = "published pool wallet private passphrase"

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

func runtimeStatus(ctx context.Context, r *pooloperator.Runtime) pooloperator.Status {
	var s pooloperator.Status
	wait(ctx, 20*time.Second, func() bool {
		v, e := r.Status(ctx)
		if e == nil {
			s = v
			return true
		}
		if e != poolschedule.ErrBusy {
			check(e, "original runtime status")
		}
		return false
	})
	return s
}
