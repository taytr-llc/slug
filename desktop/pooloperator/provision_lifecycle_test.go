//go:build !slugtestpow && (linux || (darwin && cgo))

// Copyright (c) 2026 The SLUG developers. ISC license.
package pooloperator

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/taytr-llc/slug/prospectnode"
)

type provisionNoEntropy struct{ calls int }

func (r *provisionNoEntropy) Read([]byte) (int, error) {
	r.calls++
	return 0, errors.New("test forbids replacement entropy")
}

type provisionTestSelection struct {
	v            provisionVector
	root, inputs string
	args         []string
	capsule      []byte
	now          func() time.Time
}

func newProvisionTestSelection(t *testing.T) *provisionTestSelection {
	t.Helper()
	refs := provisionReference(t)
	v := refs.Positives[0]
	inputs := opSetupTestDir(t)
	root := filepath.Join(opSetupTestDir(t), "provision")
	files := map[string][]byte{"policy": provisionHex(t, v.PolicyFile), "certificate": []byte(refs.CertificateFixture.ChainPEM), "public-pass": []byte("published provision public password"), "unlock-pass": []byte("published provision private password")}
	for name, raw := range files {
		if e := os.WriteFile(filepath.Join(inputs, name), raw, 0600); e != nil {
			t.Fatal(e)
		}
	}
	args := []string{"create", "--policy-file", filepath.Join(inputs, "policy"), "--root", root, "--tls-certificate-file", filepath.Join(inputs, "certificate"), "--wallet-public-passphrase-file", filepath.Join(inputs, "public-pass"), "--wallet-unlock-passphrase-file", filepath.Join(inputs, "unlock-pass")}
	f := &provisionTestSelection{v, root, inputs, args, provisionCapsule(t, v), func() time.Time { return time.UnixMilli(v.Material.CreatedAtMS) }}
	t.Cleanup(func() { clear(f.capsule) })
	return f
}
func provisionTestOutput(t *testing.T) *os.File {
	t.Helper()
	f, e := os.CreateTemp(opSetupTestDir(t), "out-")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = f.Close() })
	return f
}
func (f *provisionTestSelection) run(t *testing.T, command string, entropy io.Reader) ([]byte, error) {
	t.Helper()
	args := append([]string(nil), f.args...)
	args[0] = command
	out, errout := provisionTestOutput(t), provisionTestOutput(t)
	e := runProvisionCommand(context.Background(), args, out, errout, entropy, f.now)
	b, re := os.ReadFile(out.Name())
	if re != nil {
		t.Fatal(re)
	}
	diag, re := os.ReadFile(errout.Name())
	if re != nil {
		t.Fatal(re)
	}
	for _, secret := range []string{f.v.Backup.Mnemonic, "published provision private password"} {
		if bytes.Contains(b, []byte(secret)) || bytes.Contains(diag, []byte(secret)) {
			t.Fatal("private output")
		}
	}
	return b, e
}
func (f *provisionTestSelection) expected(t *testing.T) map[string][]byte {
	t.Helper()
	return map[string][]byte{provisionIntentName: provisionHex(t, f.v.Intent), "offline/material-v1.bin": append([]byte(nil), f.capsule...), "offline/" + provisionBackupName: provisionHex(t, f.v.BackupFile), "online/admission.seed": append([]byte(nil), f.capsule[32:64]...), "online/publication.seed": append([]byte(nil), f.capsule[64:96]...), "online/cutoff.seed": append([]byte(nil), f.capsule[96:128]...), "public/setup.json": provisionHex(t, f.v.SetupFile), provisionReadyName: provisionHex(t, f.v.Ready)}
}
func (f *provisionTestSelection) assertFiles(t *testing.T) {
	t.Helper()
	for name, want := range f.expected(t) {
		got, e := os.ReadFile(filepath.Join(f.root, name))
		if e != nil || !bytes.Equal(got, want) {
			t.Fatal("exact retained output", name, e)
		}
	}
}
func (f *provisionTestSelection) prefix(t *testing.T, n int) {
	t.Helper()
	if e := os.Mkdir(f.root, 0700); e != nil {
		t.Fatal(e)
	}
	order := []string{provisionIntentName, "offline/material-v1.bin", "offline/" + provisionBackupName, "online/admission.seed", "online/publication.seed", "online/cutoff.seed", "public/setup.json"}
	expected := f.expected(t)
	for _, name := range order[:n] {
		p := filepath.Join(f.root, name)
		if e := os.MkdirAll(filepath.Dir(p), 0700); e != nil {
			t.Fatal(e)
		}
		if e := os.WriteFile(p, expected[name], 0600); e != nil {
			t.Fatal(e)
		}
	}
}
func TestProvisionCreateVerifyLostReadyAndRuntimeInit(t *testing.T) {
	f := newProvisionTestSelection(t)
	out, e := f.run(t, "create", bytes.NewReader(f.capsule))
	if e != nil {
		t.Fatal("create", e)
	}
	if !bytes.Contains(out, []byte(`"code":"provisioned"`)) {
		t.Fatal("missing public result")
	}
	f.assertFiles(t)
	noEntropy := &provisionNoEntropy{}
	for _, command := range []string{"verify", "resume"} {
		out, e = f.run(t, command, noEntropy)
		if e != nil || !bytes.Contains(out, []byte(`"code":"verified"`)) {
			t.Fatal("exact established verification", command, e)
		}
	}
	if noEntropy.calls != 0 {
		t.Fatal("established verification drew entropy")
	}
	// A complete wallet with lost READY is reused through Open, never created.
	if e = os.Remove(filepath.Join(f.root, provisionReadyName)); e != nil {
		t.Fatal(e)
	}
	if _, e = f.run(t, "resume", noEntropy); e != nil {
		t.Fatal("lost READY", e)
	}
	if noEntropy.calls != 0 {
		t.Fatal("lost READY drew entropy")
	}
	f.assertFiles(t)
	// The generated exact Setup/wallet/seeds feed the real stopped runtime host.
	// Init/open verify node identity and activation metadata. A bounded HTTP fixture
	// proves child binding; no Run, mining or actual chain claim follows.
	leafSeed := sha256.Sum256([]byte("SLUG set74 public offline provision certificate leaf\x00v1\x00"))
	key := ed25519.NewKeyFromSeed(leafSeed[:])
	defer clear(key)
	der, e := x509.MarshalPKCS8PrivateKey(key)
	if e != nil {
		t.Fatal(e)
	}
	defer clear(der)
	keyPath := filepath.Join(f.inputs, "tls-key")
	if e = os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), 0600); e != nil {
		t.Fatal(e)
	}
	var metadataCalls atomic.Int32
	node := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, password, ok := r.BasicAuth()
		var q struct {
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
			ID     uint64            `json:"id"`
		}
		if !ok || username != "synthetic" || password != "synthetic" || r.Method != "POST" || json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&q) != nil {
			http.Error(w, "invalid fixture request", 400)
			return
		}
		var result any
		switch q.Method {
		case "getcurrentnet":
			if len(q.Params) != 0 {
				http.Error(w, "invalid parameters", 400)
				return
			}
			result = f.v.Policy.Network
		case "getblockhash":
			if len(q.Params) != 1 || string(q.Params[0]) != "0" {
				http.Error(w, "invalid height", 400)
				return
			}
			result = f.v.Policy.Genesis
		case "getbestblock":
			if len(q.Params) != 0 {
				http.Error(w, "invalid parameters", 400)
				return
			}
			result = map[string]any{"hash": f.v.Policy.Genesis, "height": 0}
		case "getblockchaininfo":
			if len(q.Params) != 0 {
				http.Error(w, "invalid parameters", 400)
				return
			}
			deployments := map[string]any{}
			for _, name := range []string{"csv", "segwit", "taproot"} {
				deployments[name] = map[string]any{"status": "defined", "always_active_height": 62}
			}
			result = map[string]any{"chain": "mainnet", "blocks": 0, "bestblockhash": f.v.Policy.Genesis, "bip9_softforks": deployments}
		default:
			t.Errorf("unexpected constructor RPC %s", q.Method)
			http.Error(w, "unexpected method", 400)
			return
		}
		metadataCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(struct {
			JSONRPC string `json:"jsonrpc"`
			Result  any    `json:"result"`
			Error   any    `json:"error"`
			ID      uint64 `json:"id"`
		}{"1.0", result, nil, q.ID})
	}))
	defer node.Close()
	rpc := prospectnode.RPCConfig{Version: 1, URL: node.URL, Username: "synthetic", Password: "synthetic"}
	rpcRaw, _ := json.Marshal(rpc)
	rpcPath := filepath.Join(f.inputs, "rpc")
	if e = os.WriteFile(rpcPath, rpcRaw, 0600); e != nil {
		t.Fatal(e)
	}
	runtimeRoot := filepath.Join(opSetupTestDir(t), "operator")
	args := []string{"init", "--setup-file", filepath.Join(f.root, "public/setup.json"), "--root", runtimeRoot, "--wallet-root", filepath.Join(f.root, "wallet"), "--wallet-public-passphrase-file", filepath.Join(f.inputs, "public-pass"), "--wallet-unlock-passphrase-file", filepath.Join(f.inputs, "unlock-pass"), "--admission-seed-file", filepath.Join(f.root, "online/admission.seed"), "--publication-seed-file", filepath.Join(f.root, "online/publication.seed"), "--cutoff-seed-file", filepath.Join(f.root, "online/cutoff.seed"), "--rpc-config-file", rpcPath, "--tls-certificate-file", filepath.Join(f.inputs, "certificate"), "--tls-key-file", keyPath}
	for _, command := range []string{"init", "status"} {
		args[0] = command
		if e = RunCommand(context.Background(), args, provisionTestOutput(t), provisionTestOutput(t)); e != nil {
			t.Fatal("real stopped runtime composition", command, e)
		}
	}
	if metadataCalls.Load() != 14 {
		t.Fatal("each init/status must verify identity and activation metadata", metadataCalls.Load())
	}
	f.assertFiles(t)
}
func TestProvisionFinitePartialPrefixes(t *testing.T) {
	for n := 1; n <= 7; n++ {
		t.Run(strconv.Itoa(n), func(t *testing.T) {
			f := newProvisionTestSelection(t)
			f.prefix(t, n)
			var source io.Reader = &provisionNoEntropy{}
			if n == 1 {
				source = bytes.NewReader(f.capsule)
			}
			if _, e := f.run(t, "resume", source); e != nil {
				t.Fatal("exact prefix recovery", n, e)
			}
			if r, ok := source.(*provisionNoEntropy); ok && r.calls != 0 {
				t.Fatal("published material replaced")
			}
			f.assertFiles(t)
		})
	}
}
func TestProvisionHoldsAndEstablishedMissing(t *testing.T) {
	for _, kind := range []string{"partial-material", "mismatched-seed", "empty-wallet", "missing-established"} {
		t.Run(kind, func(t *testing.T) {
			f := newProvisionTestSelection(t)
			f.prefix(t, 7)
			switch kind {
			case "partial-material":
				if e := os.WriteFile(filepath.Join(f.root, "offline/material-v1.bin"), f.capsule[:127], 0600); e != nil {
					t.Fatal(e)
				}
			case "mismatched-seed":
				if e := os.WriteFile(filepath.Join(f.root, "online/admission.seed"), bytes.Repeat([]byte{1}, 32), 0600); e != nil {
					t.Fatal(e)
				}
			case "empty-wallet":
				if e := os.Mkdir(filepath.Join(f.root, "wallet"), 0700); e != nil {
					t.Fatal(e)
				}
			case "missing-established":
				if e := os.WriteFile(filepath.Join(f.root, provisionReadyName), provisionHex(t, f.v.Ready), 0600); e != nil {
					t.Fatal(e)
				}
				if e := os.Remove(filepath.Join(f.root, "online/cutoff.seed")); e != nil {
					t.Fatal(e)
				}
			}
			before := provisionTestTree(t, f.root)
			r := &provisionNoEntropy{}
			_, e := f.run(t, "resume", r)
			if e == nil || r.calls != 0 {
				t.Fatal("held outcome resumed", e)
			}
			if kind == "empty-wallet" && !errors.Is(e, ErrProvisionIncompleteWallet) {
				t.Fatal("missing incomplete-wallet category", e)
			}
			if !reflect.DeepEqual(before, provisionTestTree(t, f.root)) {
				t.Fatal("held files changed")
			}
		})
	}
}
func provisionTestTree(t *testing.T, path string) map[string][32]byte {
	t.Helper()
	out := map[string][32]byte{}
	e := filepath.WalkDir(path, func(p string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if !d.IsDir() {
			raw, e := os.ReadFile(p)
			if e != nil {
				return e
			}
			rel, _ := filepath.Rel(path, p)
			out[rel] = sha256.Sum256(raw)
			clear(raw)
		}
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	return out
}
func TestProvisionWrongPassCancellationAndLostOutput(t *testing.T) {
	f := newProvisionTestSelection(t)
	if _, e := f.run(t, "create", bytes.NewReader(f.capsule)); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(f.inputs, "unlock-pass"), []byte("wrong synthetic password"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := f.run(t, "verify", &provisionNoEntropy{}); e == nil {
		t.Fatal("wrong passphrase accepted")
	}
	f.assertFiles(t)
	g := newProvisionTestSelection(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r := &provisionNoEntropy{}
	if e := runProvisionCommand(ctx, g.args, provisionTestOutput(t), provisionTestOutput(t), r, g.now); e == nil || r.calls != 0 {
		t.Fatal("canceled command progressed")
	}
	if _, e := os.Lstat(g.root); !os.IsNotExist(e) {
		t.Fatal("canceled root created")
	}
	h := newProvisionTestSelection(t)
	out := provisionTestOutput(t)
	if e := out.Close(); e != nil {
		t.Fatal(e)
	}
	e := runProvisionCommand(context.Background(), h.args, out, provisionTestOutput(t), bytes.NewReader(h.capsule), h.now)
	if !errors.Is(e, ErrCommandOutcomeUnknown) {
		t.Fatal("committed lost output not unknown", e)
	}
	h.assertFiles(t)
	if _, e = h.run(t, "resume", &provisionNoEntropy{}); e != nil {
		t.Fatal("exact output-loss recovery", e)
	}
}
func TestProvisionOriginalFilesAndStickyCleanup(t *testing.T) {
	f := newProvisionTestSelection(t)
	f.prefix(t, 2)
	owner, e := openProvisionFiles(f.root, false)
	if e != nil {
		t.Fatal(e)
	}
	defer owner.close()
	raw, exists, e := owner.read(context.Background(), "offline", "material-v1.bin", 128)
	clear(raw)
	if e != nil || !exists {
		t.Fatal(e)
	}
	p := filepath.Join(f.root, "offline/material-v1.bin")
	if e = os.Rename(p, p+".held"); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(p, f.capsule, 0600); e != nil {
		t.Fatal(e)
	}
	if owner.check(context.Background()) == nil {
		t.Fatal("same bytes in replacement descriptor accepted")
	}
	// A failed exact descriptor close must retain the root owner and remain sticky.
	f2 := newProvisionTestSelection(t)
	f2.prefix(t, 1)
	files, e := openProvisionFiles(f2.root, false)
	if e != nil {
		t.Fatal(e)
	}
	if _, _, e = files.read(context.Background(), "", provisionIntentName, 131072); e != nil {
		t.Fatal(e)
	}
	input := files.inputs[provisionIntentName]
	if e = input.file.Close(); e != nil {
		t.Fatal(e)
	}
	o := &provisionCommandOwner{common: &commandOwner{}, files: files}
	e = o.close()
	var retained *ProvisionCleanupError
	if !errors.As(e, &retained) || retained.owner != o || o.close() != e {
		t.Fatal("cleanup owner/sticky result lost")
	}
	// The test induced EBADF after a known earlier successful close. Release only
	// its still-open directory descriptors explicitly so this test leaks no lock.
	for _, d := range files.base.children {
		_ = d.file.Close()
	}
	for _, d := range files.base.dirs {
		_ = d.file.Close()
	}
	if !strings.Contains(e.Error(), "cleanup") {
		t.Fatal("closed cleanup code")
	}
}
