// Copyright (c) 2026 The SLUG developers. ISC license.
package operatorauthority

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/wire/v2"
)

var (
	ErrInvalid     = errors.New("operator authority invalid input")
	ErrUnavailable = errors.New("operator authority unavailable")
	ErrBinding     = errors.New("operator authority binding mismatch")
	ErrBusy        = errors.New("operator authority busy")
	ErrClosed      = errors.New("operator authority closed")
	ErrIntegrity   = errors.New("operator authority integrity failure")
	ErrCleanup     = errors.New("operator authority cleanup incomplete")
	ErrUnsupported = errors.New("operator authority platform unsupported")
)

// Keys is independently expected public configuration, never secret material.
// Equality between role keys is allowed; each role is still explicitly selected.
type Keys struct{ Admission, Publication, Cutoff, Checkpoint string }

func ValidateKeys(k Keys) error {
	for _, text := range [...]string{k.Admission, k.Publication, k.Cutoff, k.Checkpoint} {
		if len(text) != 64 {
			return ErrInvalid
		}
		raw, e := hex.DecodeString(text)
		if e != nil || len(raw) != 32 || hex.EncodeToString(raw) != text {
			return ErrInvalid
		}
		var key [32]byte
		copy(key[:], raw)
		if key == ([32]byte{}) || knownFixtureKey(key) {
			return ErrBinding
		}
	}
	return nil
}

// Binding is copied from the independently selected operator profile. It does
// not validate that full profile or establish store authority. Genesis uses
// chainhash's internal byte order, not reversed display text.
type Binding struct {
	Version uint32 // exactly 2
	PinID   string // canonical lowercase nonzero 32-byte hex
	Keys    Keys
	Network wire.BitcoinNet
	Genesis chainhash.Hash
}

type OnlineSeedFiles struct{ Admission, Publication, Cutoff string }
type LocalCheckpointSeedFile struct{ Path string }

func (OnlineSeedFiles) String() string           { return "operator authority private file selection" }
func (OnlineSeedFiles) GoString() string         { return "operator authority private file selection" }
func (LocalCheckpointSeedFile) String() string   { return "operator authority private file selection" }
func (LocalCheckpointSeedFile) GoString() string { return "operator authority private file selection" }

type Capabilities struct{ Admission, Publication, Cutoff, Checkpoint bool }

const (
	admissionRole = iota
	publicationRole
	cutoffRole
	checkpointRole
)

// Owner is a concrete secret-file owner, not a public signing service. Callers
// retain the original pointer; reopening equal public keys cannot revive it.
type Owner struct {
	binding   Binding
	available [4]bool
	keys      [4]ed25519.PrivateKey // aliases only for deliberately shared role keys
	files     []*selectedSeed
	life      context.Context
	cancel    context.CancelFunc
	mu        sync.Mutex
	active    bool
	closing   atomic.Bool
	poisoned  atomic.Bool
	users     sync.WaitGroup
	done      chan struct{}
	closeErr  error
}

func (*Owner) String() string   { return "operator authority owner" }
func (*Owner) GoString() string { return "operator authority owner" }

// The online constructor has no checkpoint path, preventing accidental online
// loading of the official Mac-held checkpoint seed. Its public key is still pinned.
func OpenOnline(life context.Context, expected Binding, paths OnlineSeedFiles) (*Owner, error) {
	if paths.Admission == "" || paths.Publication == "" || paths.Cutoff == "" {
		return nil, ErrInvalid
	}
	return openSelected(life, expected, [4]string{paths.Admission, paths.Publication, paths.Cutoff, ""})
}

// Separate local tooling only; never selected by the online daemon. This does
// not authorize reading, converting, copying, or replacing the existing official key.
func OpenLocalCheckpoint(life context.Context, expected Binding, path LocalCheckpointSeedFile) (*Owner, error) {
	if path.Path == "" {
		return nil, ErrInvalid
	}
	return openSelected(life, expected, [4]string{"", "", "", path.Path})
}
func roleKeys(b Binding) [4]string {
	return [4]string{b.Keys.Admission, b.Keys.Publication, b.Keys.Cutoff, b.Keys.Checkpoint}
}
func validBinding(b Binding) bool {
	if b.Version != 2 || b.Network == 0 || b.Genesis == (chainhash.Hash{}) || ValidateKeys(b.Keys) != nil {
		return false
	}
	if len(b.PinID) != 64 {
		return false
	}
	raw, e := hex.DecodeString(b.PinID)
	if e != nil || len(raw) != 32 || hex.EncodeToString(raw) != b.PinID {
		return false
	}
	var id [32]byte
	copy(id[:], raw)
	return id != ([32]byte{})
}

// Static public fixture exclusions copied from the independently generated
// authority-v2.json SHA256 7328ea2102c9ac993281bd732590c3e734eda26a3d180c2fa98bbd37da02c6b4.
// No private-key derivation occurs in this pure configuration check.
func knownFixtureKey(k [32]byte) bool {
	switch hex.EncodeToString(k[:]) {
	case "fec1428673f6afe89264b3807344ae8b269d125a503855202687db5a7128579f":
		return true
	case "26f705c8b914e8608e725221b277e481ce57c8b6f09dbf21423b949d4939e29a":
		return true
	case "f1bee65bdc82723fbd180db7a0edb48354a9b8df038e6875b905cc2173a794fa":
		return true
	case "984ebcc3fd73fcf64dd8ebb6a03b01a6ed2403548a479ce3089dca1392841bcf":
		return true
	case "aee7dd24b6a03c5a02f452f5f588c8ca7e1dba000d9e21a736993e5842b30ca4":
		return true
	default:
		return false
	}
}

func openSelected(ctx context.Context, expected Binding, paths [4]string) (*Owner, error) {
	if ctx == nil || !validBinding(expected) {
		return nil, ErrInvalid
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	life, cancel := context.WithCancel(ctx)
	o := &Owner{binding: expected, life: life, cancel: cancel, done: make(chan struct{})}
	fail := func(cause error) (*Owner, error) {
		if e := o.Close(); e != nil {
			return o, errors.Join(cause, e)
		}
		return nil, cause
	}
	expectedKeys := roleKeys(expected)
	first := make(map[string]int, 4)
	for role, path := range paths {
		if path == "" {
			continue
		}
		if previous, exists := first[expectedKeys[role]]; exists {
			if paths[previous] != path {
				return fail(ErrInvalid)
			}
			o.keys[role], o.available[role] = o.keys[previous], true
			continue
		}
		for previous := 0; previous < role; previous++ {
			if paths[previous] == path {
				return fail(ErrBinding)
			}
		}
		// loadSeed's nonnil selection is retained even when its read/check fails.
		selected, seed, e := loadSeed(ctx, path)
		if selected != nil {
			o.files = append(o.files, selected)
		}
		if e != nil {
			clear(seed[:])
			return fail(e)
		}
		key := ed25519.NewKeyFromSeed(seed[:])
		clear(seed[:])
		if hex.EncodeToString(key[32:]) != expectedKeys[role] {
			clear(key)
			return fail(ErrBinding)
		}
		o.keys[role], o.available[role] = key, true
		first[expectedKeys[role]] = role
	}
	if e := o.checkFiles(ctx); e != nil {
		return fail(e)
	}
	return o, nil
}
func (o *Owner) Binding() Binding {
	if o == nil {
		return Binding{}
	}
	return o.binding
}
func (o *Owner) Capabilities() Capabilities {
	if o == nil {
		return Capabilities{}
	}
	return Capabilities{o.available[0], o.available[1], o.available[2], o.available[3]}
}

// Check is a nonblocking original-owner lifecycle check. It performs no file
// I/O, mutex acquisition or signing-slot admission, and grants no future commit.
func (o *Owner) Check(ctx context.Context, expected Binding) error {
	if o == nil || ctx == nil || o.life == nil || o.cancel == nil || o.done == nil {
		return ErrInvalid
	}
	if expected != o.binding {
		return ErrBinding
	}
	if o.closing.Load() {
		return ErrClosed
	}
	if o.poisoned.Load() {
		return ErrIntegrity
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	if o.life.Err() != nil {
		return ErrClosed
	}
	return nil
}

// CheckOperation rejects equal-key replacement owners. It is a cheap original
// capability/lifetime check only; the caller still owns its store CAS.
func (o *Owner) CheckOperation(op *Operation, expected Binding) error {
	if o == nil {
		return ErrInvalid
	}
	if op == nil || op.owner != o {
		return ErrBinding
	}
	if e := o.Check(op.ctx, expected); e != nil {
		return e
	}
	return op.Check()
}

// Begin registers immediate signing AND original store commit/disclosure work.
// Never put an Operation into a dormant prepared handle. Defer Close immediately.
func (o *Owner) Begin(ctx context.Context, expected Binding) (*Operation, error) {
	if o == nil || ctx == nil || o.life == nil || o.cancel == nil || o.done == nil {
		return nil, ErrInvalid
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closing.Load() || o.life.Err() != nil {
		return nil, ErrClosed
	}
	if o.poisoned.Load() {
		return nil, ErrIntegrity
	}
	if expected != o.binding {
		return nil, ErrBinding
	}
	if o.active {
		return nil, ErrBusy
	}
	work, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(o.life, cancel)
	op := &Operation{owner: o, ctx: work, cancel: cancel, stop: stop}
	o.active = true
	o.users.Add(1)
	return op, nil
}

type Operation struct {
	owner  *Owner
	ctx    context.Context
	cancel context.CancelFunc
	stop   func() bool
	mu     sync.Mutex // serializes Sign and Close, including a misused concurrent Close
	closed atomic.Bool
}

func (op *Operation) Context() context.Context {
	if op == nil {
		return nil
	}
	return op.ctx
}

// Check is a nonblocking lifecycle check, not a store CAS or file-identity check.
func (op *Operation) Check() error {
	if op == nil || op.owner == nil || op.ctx == nil || op.owner.life == nil {
		return ErrInvalid
	}
	if op.closed.Load() || op.owner.closing.Load() {
		return ErrClosed
	}
	if op.owner.poisoned.Load() {
		return ErrIntegrity
	}
	if e := op.ctx.Err(); e != nil {
		return e
	}
	if op.owner.life.Err() != nil {
		return ErrClosed
	}
	return nil
}
func (op *Operation) Close() error {
	if op == nil {
		return nil
	}
	if op.owner == nil || op.ctx == nil || op.cancel == nil || op.stop == nil {
		return ErrInvalid
	}
	op.mu.Lock()
	defer op.mu.Unlock()
	if op.closed.Swap(true) {
		return nil
	}
	op.stop()
	op.cancel()
	o := op.owner
	o.mu.Lock()
	o.active = false
	o.users.Done()
	o.mu.Unlock()
	return nil
}
func (o *Owner) Close() error {
	if o == nil {
		return nil
	}
	if o.life == nil || o.cancel == nil || o.done == nil {
		return ErrInvalid
	}
	o.mu.Lock()
	first := !o.closing.Swap(true)
	if first {
		o.cancel()
	}
	o.mu.Unlock()
	if !first {
		<-o.done
		return o.closeErr
	}
	o.users.Wait()
	for i := range o.keys {
		clear(o.keys[i])
		o.keys[i] = nil
	}
	var cleanup error
	for _, f := range o.files {
		if f.close() != nil {
			cleanup = ErrCleanup
		}
	}
	o.closeErr = cleanup // channel close publishes this immutable result
	close(o.done)
	return cleanup
}
func (o *Owner) checkFiles(ctx context.Context) error {
	if ctx == nil {
		return ErrInvalid
	}
	for _, f := range o.files {
		if e := f.check(ctx); e != nil {
			return e
		}
	}
	return ctx.Err()
}
func (o *Owner) poison() { o.poisoned.Store(true); o.cancel() }

// These internal cryptographic operations do NOT verify admission, replay,
// bracket/canonical evidence or durable intent. Only original registry/core
// capabilities may supply the canonical unsigned bytes and later publish them.
func (op *Operation) SignAdmission(b []byte) ([64]byte, error) {
	return op.signStatement(admissionRole, "SLUG fighter ownership admission statement", b, 69632)
}
func (op *Operation) SignPublication(b []byte) ([64]byte, error) {
	return op.signStatement(publicationRole, "SLUG fighter ownership publication statement", b, 8192)
}
func (op *Operation) SignBoundary(b []byte) ([64]byte, error) {
	return op.signStatement(cutoffRole, "SLUG fighter ownership boundary statement", b, 8192)
}
func (op *Operation) SignCutoff(b []byte) ([64]byte, error) {
	return op.signStatement(cutoffRole, "SLUG fighter ownership cutoff statement", b, 8192)
}
func (op *Operation) SignFinalityFrame(b []byte) ([64]byte, error) {
	return op.signStatement(publicationRole, "SLUG fighter ownership finality statement", b, 8192)
}
func (op *Operation) signStatement(role int, domain string, input []byte, limit int) ([64]byte, error) {
	var zero [64]byte
	if op == nil || op.owner == nil || len(input) == 0 || len(input) > limit {
		return zero, ErrInvalid
	}
	// Caller must not mutate its input concurrently. Own bytes before hashing.
	body := bytes.Clone(input)
	// This is only a cheap binding/syntax check, not a duplicate public validator.
	prefix := []byte(`{"version":1,"pin_id":"` + op.owner.binding.PinID + `",`)
	if !bytes.HasPrefix(body, prefix) || !json.Valid(body) {
		return zero, ErrBinding
	}
	h := sha256.New()
	h.Write([]byte(domain))
	h.Write([]byte{0, 'v', '1', 0})
	var length [4]byte
	binary.LittleEndian.PutUint32(length[:], uint32(len(body)))
	h.Write(length[:])
	h.Write(body)
	return op.signMessage(role, h.Sum(nil))
}
func (op *Operation) SignCheckpoint(unsigned chaincfg.SignedCheckpoint) (chaincfg.SignedCheckpoint, error) {
	if op == nil || op.owner == nil || len(unsigned.Signature) != 0 || unsigned.Network != op.owner.binding.Network || unsigned.GenesisHash != op.owner.binding.Genesis {
		return chaincfg.SignedCheckpoint{}, ErrBinding
	}
	message, e := unsigned.SigningBytes()
	if e != nil {
		return chaincfg.SignedCheckpoint{}, ErrInvalid
	}
	sig, e := op.signMessage(checkpointRole, message)
	if e != nil {
		return chaincfg.SignedCheckpoint{}, e
	}
	unsigned.Signature = bytes.Clone(sig[:])
	return unsigned, nil
}
func (op *Operation) signMessage(role int, message []byte) ([64]byte, error) {
	var out [64]byte
	if op == nil || op.owner == nil {
		return out, ErrInvalid
	}
	if !op.mu.TryLock() {
		return out, ErrBusy
	}
	defer op.mu.Unlock()
	if e := op.Check(); e != nil {
		return out, e
	}
	o := op.owner
	if !o.available[role] {
		return out, ErrUnavailable
	}
	if e := o.checkFiles(op.ctx); e != nil {
		if op.ctx.Err() == nil && o.life.Err() == nil {
			o.poison()
		}
		return out, e
	}
	signature := ed25519.Sign(o.keys[role], message)
	expected, _ := hex.DecodeString(roleKeys(o.binding)[role])
	if !ed25519.Verify(expected, message, signature) {
		o.poison()
		return out, ErrIntegrity
	}
	if e := o.checkFiles(op.ctx); e != nil {
		if op.ctx.Err() == nil && o.life.Err() == nil {
			o.poison()
		}
		return out, e
	}
	if e := op.Check(); e != nil {
		return out, e
	}
	copy(out[:], signature)
	return out, nil
}
