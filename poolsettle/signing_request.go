// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettle

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"sync/atomic"

	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/wire/v2"
)

// ConfigBindingID preserves the existing closed Config grammar. The digest is
// public configuration correlation, not a unique Store or payment capability.
func ConfigBindingID(c Config) (string, error) {
	if validateConfig(c) != nil {
		return "", ErrConfig
	}
	raw, e := json.Marshal(c)
	if e != nil || len(raw) > 65536 {
		return "", ErrConfig
	}
	sum := signingHash("SLUG operator pool settlement config", raw)
	return hex.EncodeToString(sum[:]), nil
}
func signingHash(label string, raw []byte) [32]byte {
	h := sha256.New()
	h.Write([]byte(label))
	h.Write([]byte{0})
	h.Write([]byte("v1"))
	h.Write([]byte{0})
	var length [4]byte
	binary.LittleEndian.PutUint32(length[:], uint32(len(raw)))
	h.Write(length[:])
	h.Write(raw)
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

// Only comparable concrete owner pointers may bind the guarded operator path.
// Legacy Backend values can continue using RunDaily without calling these APIs.
func sameSigningBackend(a, b Backend) bool {
	if a == nil || b == nil {
		return false
	}
	x, y := reflect.ValueOf(a), reflect.ValueOf(b)
	return x.Kind() == reflect.Pointer && y.Kind() == reflect.Pointer && !x.IsNil() && !y.IsNil() && x.Type() == y.Type() && x.Type().Comparable() && x.Interface() == y.Interface()
}
func sameSigningConfig(a, b Config) bool {
	if a.Version != b.Version || a.ChangeScript != b.ChangeScript || a.MaxRounds != b.MaxRounds || a.MaxBatches != b.MaxBatches || len(a.Destinations) != len(b.Destinations) {
		return false
	}
	for i := range a.Destinations {
		if a.Destinations[i] != b.Destinations[i] {
			return false
		}
	}
	return true
}
func (s *Store) signingAlive() error {
	if s == nil {
		return ErrClosed
	}
	s.lifecycle.Lock()
	defer s.lifecycle.Unlock()
	if s.closing || s.db == nil {
		return ErrClosed
	}
	return nil
}

// CheckBackendBinding is a constructor-only audit of the exact writable store,
// original backend pointer and independently retained configuration. Never call
// it from Backend.Sign: RunDaily already holds the store gate at that point.
func (s *Store) CheckBackendBinding(ctx context.Context, backend Backend, expected Config, source poolshare.Config) error {
	if s != nil && s.auditOnly {
		return ErrInvalid
	}
	end, e := s.begin(ctx)
	if e != nil {
		return e
	}
	defer end()
	if !sameSigningBackend(s.backend, backend) || !sameSigningConfig(s.state.Config, expected) || s.state.Source != source {
		return ErrConfig
	}
	if e = s.verify(ctx); e != nil {
		return e
	}
	if e = ctx.Err(); e != nil {
		return e
	}
	return s.signingAlive()
}

type signingContextKey struct{}
type signingRequestBody struct {
	Version  uint32           `json:"version"`
	Config   Config           `json:"config"`
	Source   poolshare.Config `json:"source"`
	Inputs   []Coin           `json:"inputs"`
	Payments []Payment        `json:"payments"`
	FeeAtoms int64            `json:"fee_atoms"`
	Change   *Coin            `json:"change"`
	Unsigned string           `json:"unsigned"`
}
type settlementSigningRequest struct {
	store    *Store
	backend  Backend
	parent   context.Context
	body     signingRequestBody
	unsigned []byte
	digest   [32]byte
	active   atomic.Bool
}

func signingRequestDigest(body signingRequestBody) ([32]byte, error) {
	// Active input claims plus their possible change repeat at most the bounded
	// original state, with room for configuration and canonical transaction hex.
	const limit = 2*maxStateBytes + (1 << 20)
	raw, e := json.Marshal(body)
	if e != nil || len(raw) > limit {
		return [32]byte{}, ErrInvalid
	}
	return signingHash("SLUG operator pool signing request", raw), nil
}
func (c *settlementSigningRequest) check(ctx context.Context) error {
	if c == nil || c.store == nil || c.parent == nil || !c.active.Load() {
		return ErrInvalid
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	if e := c.parent.Err(); e != nil {
		return e
	}
	return c.store.signingAlive()
}

// CheckSigningRequest reads only the private active capability minted by the
// original Store's synchronous Sign call. No store gate, DB or node operation
// is acquired, and caller Coin.Claims alone never establish authority. A new
// child/context.WithoutCancel cannot revive the original cancelled operation.
func CheckSigningRequest(ctx context.Context, original *Store, backend Backend, expected Config, source poolshare.Config, inputs []Coin, unsigned []byte) error {
	if wire.TestPoW {
		return ErrProfile
	}
	if ctx == nil {
		return ErrInvalid
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	c, ok := ctx.Value(signingContextKey{}).(*settlementSigningRequest)
	if !ok || c == nil || c.store != original || !sameSigningBackend(c.backend, backend) {
		return ErrInvalid
	}
	if e := c.check(ctx); e != nil {
		return e
	}
	// Compare against owned immutable data before canonical digest allocation.
	// All input iteration is bounded by the original verified request's lengths.
	if !sameSigningConfig(c.body.Config, expected) || c.body.Source != source || !reflect.DeepEqual(c.body.Inputs, inputs) || !bytes.Equal(c.unsigned, unsigned) {
		return ErrInvalid
	}
	digest, e := signingRequestDigest(c.body)
	if e != nil || digest != c.digest {
		return ErrInvalid
	}
	return c.check(ctx)
}

// signSettlementRequest scopes invalidation to precisely this synchronous call,
// before RunDaily validates signatures, publishes a batch or may broadcast.
// Legacy backends see the same copied inputs/raw and can ignore the capability.
func (s *Store) signSettlementRequest(ctx context.Context, inputs []Coin, payments []Payment, change *Coin, fee int64, unsigned []byte) ([]byte, error) {
	body := signingRequestBody{Version: 1, Config: clone(s.state.Config), Source: s.state.Source, Inputs: clone(inputs), Payments: clone(payments), FeeAtoms: fee, Change: clone(change), Unsigned: hex.EncodeToString(unsigned)}
	digest, e := signingRequestDigest(body)
	if e != nil {
		return nil, e
	}
	c := &settlementSigningRequest{store: s, backend: s.backend, parent: ctx, body: body, unsigned: append([]byte(nil), unsigned...), digest: digest}
	c.active.Store(true)
	defer c.active.Store(false)
	return s.backend.Sign(context.WithValue(ctx, signingContextKey{}, c), clone(inputs), append([]byte{}, unsigned...))
}
