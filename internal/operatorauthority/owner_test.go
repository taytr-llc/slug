//go:build linux || (darwin && cgo)

// Copyright (c) 2026 The SLUG developers. ISC license.
package operatorauthority

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/wire/v2"
)

const corpusHash = "7328ea2102c9ac993281bd732590c3e734eda26a3d180c2fa98bbd37da02c6b4"
const recipeHash = "4de611d99f258dc40e8413c3fead1fc5da3471d7e93216707283cf28758101d1"

type authorityCase struct {
	Family          string            `json:"family"`
	Name            string            `json:"name"`
	Value           json.RawMessage   `json:"value"`
	CanonicalHex    string            `json:"canonical_hex"`
	CanonicalSHA256 string            `json:"canonical_sha256"`
	IDs             map[string]string `json:"ids"`
}
type authorityReference struct {
	Literals struct {
		Known     map[string]string `json:"known_fixture_keys"`
		Synthetic map[string]string `json:"synthetic_role_keys"`
	} `json:"literals"`
	Positives []authorityCase `json:"positives"`
}
type authorityRecipe struct {
	Seeds map[string]string `json:"synthetic_role_seed_ascii"`
}
type authorityPin struct {
	Version uint32 `json:"version"`
	Source  struct {
		Network uint32 `json:"network"`
		Genesis string `json:"genesis"`
	} `json:"source"`
	Admission   string `json:"admission_key"`
	Publication string `json:"publication_key"`
	Cutoff      string `json:"cutoff_key"`
	Checkpoint  string `json:"checkpoint_key"`
}

var referenceOnce sync.Once
var referenceValue authorityReference
var recipeValue authorityRecipe
var referenceError error

func readReferenceFile(path, expected string) ([]byte, error) {
	f, e := os.Open(path)
	if e != nil {
		return nil, e
	}
	info, e := f.Stat()
	if e != nil || info.Size() < 1 || info.Size() > 2<<20 {
		f.Close()
		return nil, errors.New("reference size")
	}
	raw, e := io.ReadAll(io.LimitReader(f, (2<<20)+1))
	closeErr := f.Close()
	if e != nil {
		return nil, e
	}
	if len(raw) > 2<<20 {
		return nil, errors.New("reference grew")
	}
	if closeErr != nil {
		return nil, closeErr
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != expected {
		return nil, errors.New("reference hash")
	}
	return raw, nil
}
func reference(t *testing.T) (authorityReference, authorityRecipe) {
	t.Helper()
	referenceOnce.Do(func() {
		raw, e := readReferenceFile("testdata/reference/authority-v2.json", corpusHash)
		if e == nil {
			e = json.Unmarshal(raw, &referenceValue)
		}
		if e == nil {
			raw, e = readReferenceFile("testdata/reference/recipe.json", recipeHash)
		}
		if e == nil {
			e = json.Unmarshal(raw, &recipeValue)
		}
		referenceError = e
	})
	if referenceError != nil {
		t.Fatal(referenceError)
	}
	return referenceValue, recipeValue
}
func referenceBinding(t *testing.T, name string) Binding {
	t.Helper()
	ref, _ := reference(t)
	for _, row := range ref.Positives {
		if row.Family != "pin" || row.Name != name {
			continue
		}
		var p authorityPin
		if e := json.Unmarshal(row.Value, &p); e != nil {
			t.Fatal(e)
		}
		h, e := chainhash.NewHashFromStr(p.Source.Genesis)
		if e != nil {
			t.Fatal(e)
		}
		return Binding{Version: p.Version, PinID: row.IDs["pin_id"], Keys: Keys{p.Admission, p.Publication, p.Cutoff, p.Checkpoint}, Network: wire.BitcoinNet(p.Source.Network), Genesis: *h}
	}
	t.Fatal("missing independent binding", name)
	return Binding{}
}
func firstBinding(t *testing.T) Binding {
	t.Helper()
	ref, _ := reference(t)
	for _, row := range ref.Positives {
		if row.Family == "pin" && strings.HasSuffix(row.Name, "/operator_v2_distinct") {
			return referenceBinding(t, row.Name)
		}
	}
	t.Fatal("missing distinct public fixture")
	return Binding{}
}
func seedFor(t *testing.T, key string) [32]byte {
	t.Helper()
	ref, recipe := reference(t)
	for role, pub := range ref.Literals.Synthetic {
		if pub == key {
			text, ok := recipe.Seeds[role]
			if !ok {
				t.Fatal("missing seed recipe")
			}
			return sha256.Sum256([]byte(text))
		}
	}
	t.Fatal("only locked published synthetic seeds may be used")
	return [32]byte{}
}
func privateDir(t *testing.T) string {
	t.Helper()
	// The test may canonicalize its own temporary selection; the product never does.
	root, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	dir := filepath.Join(root, "authority")
	if e = os.Mkdir(dir, 0700); e != nil {
		t.Fatal(e)
	}
	if e = os.Chmod(dir, 0700); e != nil {
		t.Fatal(e)
	}
	return dir
}
func onlineFiles(t *testing.T, b Binding) OnlineSeedFiles {
	t.Helper()
	dir := privateDir(t)
	seen := map[string]string{}
	paths := []string{}
	for i, key := range []string{b.Keys.Admission, b.Keys.Publication, b.Keys.Cutoff} {
		if p, ok := seen[key]; ok {
			paths = append(paths, p)
			continue
		}
		p := filepath.Join(dir, fmt.Sprintf("role-%d.seed", i))
		seed := seedFor(t, key)
		if e := os.WriteFile(p, seed[:], 0600); e != nil {
			t.Fatal(e)
		}
		clear(seed[:])
		seen[key] = p
		paths = append(paths, p)
	}
	return OnlineSeedFiles{paths[0], paths[1], paths[2]}
}
func openOnlineTest(t *testing.T) (*Owner, Binding, OnlineSeedFiles) {
	t.Helper()
	b := firstBinding(t)
	paths := onlineFiles(t, b)
	owner, e := OpenOnline(t.Context(), b, paths)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		if e := owner.Close(); e != nil {
			t.Error(e)
		}
	})
	return owner, b, paths
}
func canonicalUnsigned(t *testing.T, row authorityCase) ([]byte, []byte) {
	t.Helper()
	raw, e := hex.DecodeString(row.CanonicalHex)
	if e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != row.CanonicalSHA256 {
		t.Fatal("row digest")
	}
	var v struct {
		Signature string `json:"signature"`
	}
	if e = json.Unmarshal(raw, &v); e != nil {
		t.Fatal(e)
	}
	sig, e := hex.DecodeString(v.Signature)
	if e != nil || len(sig) != 64 {
		t.Fatal("independent signature")
	}
	at := bytes.LastIndex(raw, []byte(`,"signature":`))
	if at < 0 {
		t.Fatal("terminal signature")
	}
	body := append(bytes.Clone(raw[:at]), '}')
	return body, sig
}
func admissionBody(t *testing.T, b Binding) ([]byte, []byte) {
	t.Helper()
	ref, _ := reference(t)
	for _, r := range ref.Positives {
		if r.Family == "admission" && bytes.Contains(r.Value, []byte(b.PinID)) {
			return canonicalUnsigned(t, r)
		}
	}
	t.Fatal("missing independent admission")
	return nil, nil
}
func waitClosed(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("bounded join timed out")
	}
}

func TestOperatorAuthorityIndependentNamedSignatures(t *testing.T) {
	ref, _ := reference(t)
	checks := 0
	for _, pin := range ref.Positives {
		if pin.Family != "pin" || !strings.Contains(pin.Name, "/operator_v2_") {
			continue
		}
		t.Run(pin.Name, func(t *testing.T) {
			b := referenceBinding(t, pin.Name)
			paths := onlineFiles(t, b)
			owner, e := OpenOnline(t.Context(), b, paths)
			if e != nil {
				t.Fatal(e)
			}
			defer owner.Close()
			op, e := owner.Begin(t.Context(), b)
			if e != nil {
				t.Fatal(e)
			}
			defer op.Close()
			for _, row := range ref.Positives {
				if row.Name != pin.Name && !strings.HasPrefix(row.Name, pin.Name+"/") {
					continue
				}
				if row.Family == "pin" {
					continue
				}
				body, want := canonicalUnsigned(t, row)
				if row.Family == "checkpoint" {
					var cp chaincfg.SignedCheckpoint
					raw, _ := hex.DecodeString(row.CanonicalHex)
					if e = json.Unmarshal(raw, &cp); e != nil {
						t.Fatal(e)
					}
					cp.Signature = nil
					preimage, e := cp.SigningBytes()
					if e != nil || hex.EncodeToString(preimage) != row.IDs["signing_bytes_hex"] {
						t.Fatal("checkpoint message differs")
					}
					if _, e = op.SignCheckpoint(cp); e != ErrUnavailable {
						t.Fatal("online checkpoint custody", e)
					}
					path := filepath.Join(privateDir(t), "checkpoint.seed")
					seed := seedFor(t, b.Keys.Checkpoint)
					if e = os.WriteFile(path, seed[:], 0400); e != nil {
						t.Fatal(e)
					}
					clear(seed[:])
					local, e := OpenLocalCheckpoint(t.Context(), b, LocalCheckpointSeedFile{path})
					if e != nil {
						t.Fatal(e)
					}
					localOp, e := local.Begin(t.Context(), b)
					if e != nil {
						local.Close()
						t.Fatal(e)
					}
					got, e := localOp.SignCheckpoint(cp)
					if e != nil || !bytes.Equal(got.Signature, want) {
						localOp.Close()
						local.Close()
						t.Fatal("checkpoint signature", e)
					}
					adm, _ := admissionBody(t, b)
					if _, e = localOp.SignAdmission(adm); e != ErrUnavailable {
						localOp.Close()
						local.Close()
						t.Fatal("local checkpoint granted online role", e)
					}
					if e = localOp.Close(); e != nil {
						t.Fatal(e)
					}
					if e = local.Close(); e != nil {
						t.Fatal(e)
					}
					checks++
					continue
				}
				var got [64]byte
				switch row.Family {
				case "admission":
					got, e = op.SignAdmission(body)
				case "publication":
					got, e = op.SignPublication(body)
				case "boundary":
					got, e = op.SignBoundary(body)
				case "cutoff":
					got, e = op.SignCutoff(body)
				case "frame":
					got, e = op.SignFinalityFrame(body)
				default:
					t.Fatal("unexpected signed family", row.Family)
				}
				if e != nil || !bytes.Equal(got[:], want) {
					t.Fatal(row.Family, "signature differs from independent bytes", e)
				}
				checks++
			}
		})
	}
	if checks != 32 {
		t.Fatal("exact operator signature coverage", checks)
	}
}

func TestOperatorAuthorityKeysAndBindingRefusals(t *testing.T) {
	ref, _ := reference(t)
	b := firstBinding(t)
	if e := ValidateKeys(b.Keys); e != nil {
		t.Fatal(e)
	}
	for _, key := range ref.Literals.Known {
		for role := 0; role < 4; role++ {
			bad := b.Keys
			targets := []*string{&bad.Admission, &bad.Publication, &bad.Cutoff, &bad.Checkpoint}
			*targets[role] = key
			if ValidateKeys(bad) == nil {
				t.Fatal("known fixture accepted in role", role)
			}
		}
	}
	for _, key := range []string{"", strings.Repeat("0", 64), strings.ToUpper(b.Keys.Admission), b.Keys.Admission + "00", "zz" + b.Keys.Admission[2:]} {
		bad := b.Keys
		bad.Admission = key
		if ValidateKeys(bad) == nil {
			t.Fatal("malformed key accepted")
		}
	}
	shared := b.Keys
	shared.Publication = shared.Admission
	shared.Cutoff = shared.Admission
	if ValidateKeys(shared) != nil {
		t.Fatal("explicit equal roles refused")
	}
	owner, b, _ := openOnlineTest(t)
	wrong := b
	wrong.PinID = strings.Repeat("9", 64)
	if op, e := owner.Begin(t.Context(), wrong); op != nil || e != ErrBinding {
		t.Fatal("foreign source", e)
	}
	wrong = b
	wrong.Keys.Publication = wrong.Keys.Admission
	if op, e := owner.Begin(t.Context(), wrong); op != nil || e != ErrBinding {
		t.Fatal("foreign role binding", e)
	}
	op, e := owner.Begin(t.Context(), b)
	if e != nil {
		t.Fatal(e)
	}
	defer op.Close()
	body, _ := admissionBody(t, b)
	for _, bad := range [][]byte{nil, []byte(`{`), bytes.Replace(body, []byte(b.PinID), []byte(strings.Repeat("9", 64)), 1), bytes.Repeat([]byte{'x'}, 69633)} {
		sig, e := op.SignAdmission(bad)
		if e == nil || sig != ([64]byte{}) {
			t.Fatal("invalid input disclosed signature")
		}
	}
	if _, e := owner.Begin(t.Context(), b); e != ErrBusy {
		t.Fatal("second operation admitted", e)
	}
	if e := owner.Check(t.Context(), b); e != nil {
		t.Fatal("cheap lifecycle check blocked on signing slot", e)
	}
	var zero Owner
	var zeroOp Operation
	if _, e := zero.Begin(t.Context(), b); e != ErrInvalid {
		t.Fatal("zero owner")
	}
	if e := zero.Close(); e != ErrInvalid {
		t.Fatal("zero close")
	}
	if e := zeroOp.Check(); e != ErrInvalid {
		t.Fatal("zero operation")
	}
	if e := zeroOp.Close(); e != ErrInvalid {
		t.Fatal("zero operation close")
	}
}

func TestOperatorAuthorityCloseJoinsOriginalOperation(t *testing.T) {
	owner, b, paths := openOnlineTest(t)
	op, e := owner.Begin(t.Context(), b)
	if e != nil {
		t.Fatal(e)
	}
	defer op.Close()
	done := make(chan struct{})
	errs := make(chan error, 1)
	go func() { errs <- owner.Close(); close(done) }()
	waitClosed(t, op.Context().Done())
	select {
	case <-done:
		t.Fatal("owner released a live operation")
	default:
	}
	if op.Check() != ErrClosed {
		t.Fatal("old operation stayed live")
	}
	if e = op.Close(); e != nil {
		t.Fatal(e)
	}
	waitClosed(t, done)
	if e = <-errs; e != nil {
		t.Fatal(e)
	}
	if _, e = owner.Begin(t.Context(), b); e != ErrClosed {
		t.Fatal("closed owner reopened")
	}
	if e = owner.Check(t.Context(), b); e != ErrClosed {
		t.Fatal("closed owner cheap check", e)
	}
	replacement, e := OpenOnline(t.Context(), b, paths)
	if e != nil {
		t.Fatal(e)
	}
	defer replacement.Close()
	fresh, e := replacement.Begin(t.Context(), b)
	if e != nil {
		t.Fatal(e)
	}
	defer fresh.Close()
	body, want := admissionBody(t, b)
	if sig, e := op.SignAdmission(body); e != ErrClosed || sig != ([64]byte{}) {
		t.Fatal("replacement revived old operation", e)
	}
	if sig, e := fresh.SignAdmission(body); e != nil || !bytes.Equal(sig[:], want) {
		t.Fatal("new independent owner", e)
	}
}

func TestOperatorAuthorityCancellationAndConcurrentClose(t *testing.T) {
	owner, b, _ := openOnlineTest(t)
	ctx, cancel := context.WithCancel(t.Context())
	op, e := owner.Begin(ctx, b)
	if e != nil {
		t.Fatal(e)
	}
	cancel()
	body, _ := admissionBody(t, b)
	if sig, e := op.SignAdmission(body); !errors.Is(e, context.Canceled) || sig != ([64]byte{}) {
		t.Fatal("cancelled signing", e)
	}
	if e = op.Close(); e != nil {
		t.Fatal(e)
	}
	op, e = owner.Begin(t.Context(), b)
	if e != nil {
		t.Fatal(e)
	}
	defer op.Close()
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if e := owner.Close(); e != nil {
				t.Error(e)
			}
		}()
	}
	waitClosed(t, op.Context().Done())
	if e = op.Close(); e != nil {
		t.Fatal(e)
	}
	joined := make(chan struct{})
	go func() { wg.Wait(); close(joined) }()
	waitClosed(t, joined)
	for _, key := range owner.keys {
		if len(key) != 0 {
			t.Fatal("owner retained key slice")
		}
	}
}

func TestOperatorAuthoritySignCloseRace(t *testing.T) {
	owner, b, _ := openOnlineTest(t)
	op, e := owner.Begin(t.Context(), b)
	if e != nil {
		t.Fatal(e)
	}
	defer op.Close()
	body, want := admissionBody(t, b)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			sig, e := op.SignAdmission(body)
			if e == nil {
				if !bytes.Equal(sig[:], want) {
					t.Error("bad disclosed signature")
				}
			} else if e != ErrBusy && e != ErrClosed {
				t.Error(e)
			}
		}()
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-start
		if e := op.Close(); e != nil {
			t.Error(e)
		}
	}()
	close(start)
	joined := make(chan struct{})
	go func() { wg.Wait(); close(joined) }()
	waitClosed(t, joined)
	if e := owner.Close(); e != nil {
		t.Fatal(e)
	}
}

func TestOperatorAuthorityOriginalOperationIdentity(t *testing.T) {
	owner, b, paths := openOnlineTest(t)
	replacement, e := OpenOnline(t.Context(), b, paths)
	if e != nil {
		t.Fatal(e)
	}
	defer replacement.Close()
	original, e := owner.Begin(t.Context(), b)
	if e != nil {
		t.Fatal(e)
	}
	defer original.Close()
	foreign, e := replacement.Begin(t.Context(), b)
	if e != nil {
		t.Fatal(e)
	}
	defer foreign.Close()
	if e = owner.CheckOperation(original, b); e != nil {
		t.Fatal(e)
	}
	if e = owner.CheckOperation(foreign, b); e != ErrBinding {
		t.Fatal("equal-key replacement operation admitted", e)
	}
	wrong := b
	wrong.PinID = strings.Repeat("8", 64)
	if e = owner.CheckOperation(original, wrong); e != ErrBinding {
		t.Fatal("operation source changed", e)
	}
	if e = original.Close(); e != nil {
		t.Fatal(e)
	}
	if e = original.Check(); e != ErrClosed {
		t.Fatal("released operation still live", e)
	}
	// Owner.Check observes the original operation context before CheckOperation
	// reaches Operation.Check. Close cancels that context; neither path admits.
	if e = owner.CheckOperation(original, b); e != context.Canceled {
		t.Fatal("released operation admitted", e)
	}
}

func TestOperatorAuthorityOriginalParentCancellation(t *testing.T) {
	b := firstBinding(t)
	paths := onlineFiles(t, b)
	life, cancel := context.WithCancel(t.Context())
	defer cancel()
	owner, e := OpenOnline(life, b, paths)
	if e != nil {
		t.Fatal(e)
	}
	defer owner.Close()
	op, e := owner.Begin(t.Context(), b)
	if e != nil {
		t.Fatal(e)
	}
	defer op.Close()
	cancel()
	waitClosed(t, op.Context().Done())
	body, _ := admissionBody(t, b)
	if sig, e := op.SignAdmission(body); e == nil || sig != ([64]byte{}) {
		t.Fatal("parent cancellation signed", e)
	}
	if e = owner.Check(t.Context(), b); e != ErrClosed {
		t.Fatal("original parent lifecycle not retained", e)
	}
}
