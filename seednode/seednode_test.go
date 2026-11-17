// Copyright (c) 2026 The SLUG developers. ISC license.
package seednode

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/txscript/v2"
	"github.com/taytr-llc/slug/wire/v2"
)

func TestExactRPCAmounts(t *testing.T) {
	for _, c := range []struct {
		raw  string
		want int64
	}{{"0", 0}, {"0e32", 0}, {"1e-8", 1}, {"0.00000001", 1}, {"1.000000000", 100000000}, {"2.1e7", 2100000000000000}, {"1E+1", 1000000000}} {
		got, e := amountSatoshis([]byte(c.raw))
		if e != nil || got != c.want {
			t.Fatalf("%s: %d %v", c.raw, got, e)
		}
	}
	for _, raw := range []string{"-0", "-1", "1e-9", "0.000000001", "21000000.00000001", "1e33", "1e-33", "true", "null", "01", "1.", ".1", "1e", strings.Repeat("1", 129)} {
		if _, e := amountSatoshis([]byte(raw)); e == nil {
			t.Fatalf("accepted %q", raw)
		}
	}
}

func TestPublicationPurposeAndWitness(t *testing.T) {
	var carrier [73]byte
	copy(carrier[:], "SLUGSEED")
	carrier[8] = 1
	carrier[9] = 1
	carrier[41] = 2
	_, script, _ := fundingKey()
	coin := FundingCoin{Outpoint: Outpoint{TxID: strings.Repeat("01", 32), Index: 3}, ValueSatoshis: 100000, Script: hex.EncodeToString(script), BlockHeight: 100, BlockHash: strings.Repeat("02", 32), Coinbase: true}
	tx, raw, e := publication(carrier, coin)
	if e != nil {
		t.Fatal(e)
	}
	if tx.Version != 2 || tx.LockTime != 0 || len(tx.TxIn) != 1 || len(tx.TxOut) != 2 || tx.TxIn[0].Sequence != wire.MaxTxInSequenceNum || len(tx.TxIn[0].SignatureScript) != 0 || tx.TxOut[0].Value != 0 || tx.TxOut[1].Value != 99000 || !bytes.Equal(tx.TxOut[0].PkScript, carrierScript(carrier)) || !bytes.Equal(tx.TxOut[1].PkScript, script) {
		t.Fatal("unexpected transaction shape")
	}
	_, again, e := publication(carrier, coin)
	if e != nil || !bytes.Equal(raw, again) {
		t.Fatal("non-deterministic publication")
	}
	// Verify the returned bytes through the independent script VM, then prove
	// that changing the committed payload invalidates the witness signature.
	decoded := new(wire.MsgTx)
	if decoded.Deserialize(bytes.NewReader(raw)) != nil {
		t.Fatal("wire decode")
	}
	check := func() error {
		fetch := txscript.NewCannedPrevOutputFetcher(script, coin.ValueSatoshis)
		hashes := txscript.NewTxSigHashes(decoded, fetch)
		engine, e := txscript.NewEngine(script, decoded, 0, txscript.StandardVerifyFlags, nil, hashes, coin.ValueSatoshis, fetch)
		if e != nil {
			return e
		}
		return engine.Execute()
	}
	if e := check(); e != nil {
		t.Fatal(e)
	}
	decoded.TxOut[0].PkScript[len(decoded.TxOut[0].PkScript)-1] ^= 1
	if check() == nil {
		t.Fatal("signature did not bind payload")
	}
	for _, mutate := range []func(*FundingCoin){func(c *FundingCoin) { c.ValueSatoshis = 1293 }, func(c *FundingCoin) { c.Script = "0014" + strings.Repeat("00", 20) }, func(c *FundingCoin) { c.BlockHash = "" }, func(c *FundingCoin) { c.Outpoint.TxID = "bad" }} {
		c := coin
		mutate(&c)
		if _, _, e := publication(carrier, c); e == nil {
			t.Fatal("accepted invalid funding input")
		}
	}
	carrier[8] = 2
	if _, _, e := publication(carrier, coin); e == nil {
		t.Fatal("accepted foreign carrier version")
	}
}

func TestRPCClosedEnvelopeAndTransport(t *testing.T) {
	for _, c := range []struct {
		name, body string
		ok         bool
	}{{"valid", `{"jsonrpc":"1.0","id":1,"result":"ok","error":null}`, true}, {"duplicate", `{"jsonrpc":"1.0","id":1,"id":1,"result":"ok","error":null}`, false}, {"escaped_duplicate", `{"jsonrpc":"1.0","id":1,"result":{"a":1,"\u0061":2},"error":null}`, false}, {"extra", `{"jsonrpc":"1.0","id":1,"result":"ok","error":null,"x":1}`, false}, {"wrong_id", `{"jsonrpc":"1.0","id":2,"result":"ok","error":null}`, false}, {"rpc_error", `{"jsonrpc":"1.0","id":1,"result":null,"error":{"code":-1,"message":"private"}}`, false}, {"oversized", strings.Repeat(" ", smallReply+1), false}} {
		t.Run(c.name, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				u, p, ok := r.BasicAuth()
				if !ok || u != "fixture" || p != "public" {
					t.Error("auth absent")
				}
				_, _ = w.Write([]byte(c.body))
			}))
			defer server.Close()
			r := newRPC(ownedConfig{rpc: strings.TrimPrefix(server.URL, "http://"), user: "fixture", password: "public"})
			defer r.client.CloseIdleConnections()
			_, e := r.call(context.Background(), "getblockhash", []any{0})
			if (e == nil) != c.ok || calls != 1 {
				t.Fatalf("ok=%v calls=%d", e == nil, calls)
			}
			if _, e = r.call(context.Background(), "generate", []any{1}); !errors.Is(e, ErrInvalid) || calls != 1 {
				t.Fatal("RPC allowlist widened")
			}
		})
	}
	redirects := 0
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { redirects++ }))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	r := newRPC(ownedConfig{rpc: strings.TrimPrefix(server.URL, "http://")})
	defer r.client.CloseIdleConnections()
	if _, e := r.call(context.Background(), "getcurrentnet", []any{}); e == nil || redirects != 0 {
		t.Fatal("followed redirect")
	}
}

func TestOwnedConfigRejectsSubstitution(t *testing.T) {
	home, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	for _, name := range []string{"primary-chain", "primary-logs"} {
		if e := os.Mkdir(filepath.Join(home, name), 0700); e != nil {
			t.Fatal(e)
		}
	}
	binary, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(home, "primary.conf")
	valid := "rpcuser=local-pilot\nrpcpass=" + strings.Repeat("01", 32) + "\nnotls=1\nrpclisten=127.0.0.1:12345\ndatadir=" + filepath.Join(home, "primary-chain") + "\nlogdir=" + filepath.Join(home, "primary-logs") + "\nnodnsseed=1\nlocalwalletpilot=1\nconnect=127.0.0.1:12346\nnolisten=1\n"
	done := make(chan struct{})
	owned := OwnedProcess{Cmd: &exec.Cmd{Path: binary, Args: []string{binary, "--configfile=" + path}, Process: &os.Process{Pid: os.Getpid()}}, Done: done}
	write := func(s string) {
		t.Helper()
		if e := os.WriteFile(path, []byte(s), 0600); e != nil {
			t.Fatal(e)
		}
	}
	write(valid)
	if _, e := configFor(home, "primary", owned); e != nil {
		t.Fatal(e)
	}
	for _, bad := range []string{valid + "addnode=example.org\n", strings.Replace(valid, "127.0.0.1:12346", "example.org:12346", 1), strings.Replace(valid, "nolisten=1", "listen=127.0.0.1:12347", 1), strings.Replace(valid, "rpcuser=local-pilot", "rpcuser=other", 1), strings.Replace(valid, "notls=1", "notls=0", 1)} {
		write(bad)
		if _, e := configFor(home, "primary", owned); e == nil {
			t.Fatal("accepted changed owner config")
		}
	}
	write(valid)
	close(done)
	if _, e := configFor(home, "primary", owned); !errors.Is(e, ErrOffline) {
		t.Fatal("accepted exited process")
	}
}

func TestHeaderWorkBudgetIncludesFailedHash(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal header identity test")
	}
	p := &Publisher{checked: map[string]string{}}
	h := chaincfg.MainNetParams.GenesisBlock.Header
	if e := p.verifyHeader(context.Background(), &h, strings.Repeat("01", 32)); !errors.Is(e, ErrUnavailable) || len(p.checked) != 1 {
		t.Fatal("failed work was not charged")
	}
	if e := p.verifyHeader(context.Background(), &h, chaincfg.MainNetParams.GenesisHash.String()); e != nil || len(p.checked) != 1 {
		t.Fatal("checked header not reused")
	}
	for i := 1; i < MaxStepBlocks; i++ {
		p.checked[strings.Repeat("x", i)] = strings.Repeat("02", 32)
	}
	h.Nonce++
	if e := p.verifyHeader(context.Background(), &h, strings.Repeat("02", 32)); !errors.Is(e, ErrCapacity) {
		t.Fatal("exceeded work budget")
	}
}

func TestCloseCancelsAndJoinsWork(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	proc := OwnedProcess{Cmd: &exec.Cmd{Process: &os.Process{Pid: os.Getpid()}}, Done: done}
	p := &Publisher{ctx: ctx, cancel: cancel, watchDone: make(chan struct{}), binding: OwnerBinding{Primary: proc, Peer: proc}, nodes: [2]*nodeRPC{newRPC(ownedConfig{}), newRPC(ownedConfig{})}}
	go func() { <-ctx.Done(); close(p.watchDone) }()
	work, end, e := p.begin(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	closed := make(chan struct{})
	go func() { p.Close(); close(closed) }()
	select {
	case <-work.Done():
	case <-time.After(time.Second):
		t.Fatal("work not cancelled")
	}
	select {
	case <-closed:
		t.Fatal("Close returned before join")
	default:
	}
	end()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("Close did not join")
	}
	if _, _, e := p.begin(context.Background()); e == nil {
		t.Fatal("reopened closed publisher")
	}
}

func TestRPCDuplicateNestedKeys(t *testing.T) {
	for _, raw := range []string{`{"a":{"x":1,"x":2}}`, `{"a":[{"x":1,"\u0078":2}]}`, strings.Repeat("[", 18) + "0" + strings.Repeat("]", 18)} {
		if uniqueJSON([]byte(raw)) == nil {
			t.Fatal("accepted malformed recursive JSON")
		}
	}
	if uniqueJSON([]byte(`{"a":[{"x":1},{"x":2}]}`)) != nil {
		t.Fatal("valid independent object keys refused")
	}
}

func TestPublisherProfileAndZeroValueRefusal(t *testing.T) {
	_, e := AttachOwned(context.Background(), OwnerBinding{})
	if wire.TestPoW {
		if !errors.Is(e, ErrProfile) {
			t.Fatal("cheap profile not refused first")
		}
	} else if e == nil {
		t.Fatal("unowned node attachment accepted")
	}
	p := &Publisher{}
	if _, e := p.Observe(context.Background()); !errors.Is(e, ErrOffline) {
		t.Fatal("zero Publisher became available")
	}
	if e := p.Close(); e != nil {
		t.Fatal(e)
	}
}
