// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettle

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"hash"
	"reflect"
	"sync/atomic"

	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/pooltxshape"
	"github.com/taytr-llc/slug/wire/v2"
)

const dailySigningClaims = 32768
const dailySigningBytes int64 = 640 << 20

type dailySigningContextKey struct{}
type dailySigningRequest struct {
	store    *Store
	backend  Backend
	parent   context.Context
	body     signingRequestBody
	unsigned []byte
	digest   [32]byte
	active   atomic.Bool
}

// DailySigningCheck is an opaque view of an original, active synchronous Sign
// request. It cannot be created from payment DTOs, used as a signer, or retained
// after Sign returns. A zero value refuses all operations.
type DailySigningCheck struct{ request *dailySigningRequest }

func (c *DailySigningCheck) Check(ctx context.Context) error {
	if wire.TestPoW {
		return ErrProfile
	}
	if c == nil || c.request == nil || ctx == nil {
		return ErrInvalid
	}
	r := c.request
	if r.store == nil || r.parent == nil || !r.active.Load() || r.store.daily == nil || r.body.Version != 2 || r.store.state.Config.Version != 2 {
		return ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := r.parent.Err(); err != nil {
		return err
	}
	return r.store.signingAlive()
}

// Recheck performs one full exact body comparison before final disclosure.
// Per-input work uses Check and does not re-encode the entire request.
func (c *DailySigningCheck) Recheck(ctx context.Context, inputs []Coin, unsigned []byte) error {
	if err := c.Check(ctx); err != nil {
		return err
	}
	r := c.request
	if !reflect.DeepEqual(r.body.Inputs, inputs) || !bytes.Equal(r.unsigned, unsigned) {
		return ErrInvalid
	}
	digest, err := dailySigningDigest(ctx, r.body)
	if err != nil {
		return err
	}
	if digest != r.digest {
		return ErrInvalid
	}
	return c.Check(ctx)
}

// CaptureDailySigningCheck only accepts the private capability minted by this
// exact Store/backend operation. No Store gate, database or RPC is acquired.
func CaptureDailySigningCheck(ctx context.Context, original *Store, backend Backend, expected Config, source poolshare.Config, inputs []Coin, unsigned []byte) (*DailySigningCheck, error) {
	if wire.TestPoW {
		return nil, ErrProfile
	}
	if ctx == nil {
		return nil, ErrInvalid
	}
	r, ok := ctx.Value(dailySigningContextKey{}).(*dailySigningRequest)
	if !ok || r == nil || original == nil || original.daily == nil || r.store != original || !sameSigningBackend(r.backend, backend) || !sameSigningConfig(r.body.Config, expected) || r.body.Source != source {
		return nil, ErrInvalid
	}
	c := &DailySigningCheck{request: r}
	if err := c.Recheck(ctx, inputs, unsigned); err != nil {
		return nil, err
	}
	return c, nil
}

func (s *Store) CheckDailyBackendBinding(ctx context.Context, backend Backend, expected Config, source poolshare.Config) error {
	if wire.TestPoW {
		return ErrProfile
	}
	if s == nil || s.daily == nil || s.auditOnly {
		return ErrConfig
	}
	end, err := s.begin(ctx)
	if err != nil {
		return err
	}
	defer end()
	if validateDailyConfig(expected) != nil || !sameSigningBackend(s.backend, backend) || !sameSigningConfig(s.state.Config, expected) || s.state.Source != source {
		return ErrConfig
	}
	if _, err = poolshare.DailyConfigID(source); err != nil {
		return ErrConfig
	}
	if err = s.verify(ctx); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	return s.signingAlive()
}

func dailyOwnedCoins(coins []Coin) []Coin {
	owned := append([]Coin(nil), coins...)
	for i := range owned {
		owned[i].Claims = append([]Claim(nil), coins[i].Claims...)
	}
	return owned
}

// Check counts and scalar lengths before copying. Full economic validity comes
// from the original Store's audited plan; these are bounded-codec checks only.
func dailySigningBounds(body signingRequestBody) error {
	if body.Version != 2 || len(body.Inputs) < 1 || len(body.Inputs) > pooltxshape.MaxNativeInputs || len(body.Payments) < 1 || len(body.Payments) > 256 || len(body.Unsigned) == 0 || len(body.Unsigned) > 2*pooltxshape.MaxStandardWeight || validateDailyConfig(body.Config) != nil {
		return ErrInvalid
	}
	if _, err := poolshare.DailyConfigID(body.Source); err != nil {
		return ErrInvalid
	}
	checkCoins := func(coins []Coin) error {
		total := 0
		for _, coin := range coins {
			if len(coin.TxID) > 64 || len(coin.Script) > 44 || len(coin.BlockHash) > 64 || len(coin.Claims) == 0 || len(coin.Claims) > dailySigningClaims-total {
				return ErrInvalid
			}
			total += len(coin.Claims)
			for _, claim := range coin.Claims {
				if len(claim.RoundHash) > 64 || len(claim.GymID) > 64 || len(claim.Amount.Numerator) > maxFractionDigits || len(claim.Amount.Denominator) > maxFractionDigits {
					return ErrInvalid
				}
			}
		}
		return nil
	}
	if err := checkCoins(body.Inputs); err != nil {
		return err
	}
	if body.Change != nil {
		if err := checkCoins([]Coin{*body.Change}); err != nil {
			return err
		}
	}
	for _, p := range body.Payments {
		if len(p.GymID) > 64 || len(p.Script) > 44 {
			return ErrInvalid
		}
	}
	return nil
}

// canonicalSigningWriter emits the existing closed ordered JSON grammar in
// small pieces. Historical batches are absent; at most one Claim is encoded at
// once. The independent worst-case bound is below this 640MiB stream ceiling.
type canonicalSigningWriter struct {
	ctx   context.Context
	h     hash.Hash
	bytes int64
	err   error
}

func (w *canonicalSigningWriter) put(raw []byte) {
	if w.err != nil {
		return
	}
	if err := w.ctx.Err(); err != nil {
		w.err = err
		return
	}
	if int64(len(raw)) > dailySigningBytes-w.bytes {
		w.err = ErrInvalid
		return
	}
	w.bytes += int64(len(raw))
	_, w.err = w.h.Write(raw)
}
func (w *canonicalSigningWriter) literal(s string) { w.put([]byte(s)) }
func (w *canonicalSigningWriter) value(v any) {
	if w.err != nil {
		return
	}
	raw, err := json.Marshal(v)
	if err != nil {
		w.err = ErrInvalid
		return
	}
	w.put(raw)
}
func (w *canonicalSigningWriter) coin(c *Coin) {
	if c == nil {
		w.literal("null")
		return
	}
	w.literal(`{"txid":`)
	w.value(c.TxID)
	w.literal(`,"index":`)
	w.value(c.Index)
	w.literal(`,"atoms":`)
	w.value(c.Atoms)
	w.literal(`,"script":`)
	w.value(c.Script)
	w.literal(`,"coinbase":`)
	w.value(c.Coinbase)
	w.literal(`,"block_hash":`)
	w.value(c.BlockHash)
	w.literal(`,"block_height":`)
	w.value(c.BlockHeight)
	w.literal(`,"claims":[`)
	for i := range c.Claims {
		if i != 0 {
			w.literal(",")
		}
		w.value(c.Claims[i])
	}
	w.literal("]}")
}
func dailySigningDigest(ctx context.Context, body signingRequestBody) ([32]byte, error) {
	if ctx == nil {
		return [32]byte{}, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return [32]byte{}, err
	}
	if err := dailySigningBounds(body); err != nil {
		return [32]byte{}, err
	}
	h := sha256.New()
	h.Write([]byte("SLUG operator pool daily signing request\x00v2\x00"))
	w := canonicalSigningWriter{ctx: ctx, h: h}
	w.literal(`{"version":2,"config":`)
	w.value(body.Config)
	w.literal(`,"source":`)
	w.value(body.Source)
	w.literal(`,"inputs":[`)
	for i := range body.Inputs {
		if i != 0 {
			w.literal(",")
		}
		w.coin(&body.Inputs[i])
	}
	w.literal(`],"payments":`)
	w.value(body.Payments)
	w.literal(`,"fee_atoms":`)
	w.value(body.FeeAtoms)
	w.literal(`,"change":`)
	w.coin(body.Change)
	w.literal(`,"unsigned":`)
	w.value(body.Unsigned)
	w.literal("}")
	if w.err != nil {
		return [32]byte{}, w.err
	}
	var digest [32]byte
	copy(digest[:], h.Sum(nil))
	return digest, ctx.Err()
}

func (s *Store) signDailySettlementRequest(ctx context.Context, inputs []Coin, payments []Payment, change *Coin, fee int64, unsigned []byte) ([]byte, error) {
	if wire.TestPoW {
		return nil, ErrProfile
	}
	if ctx == nil || s == nil || s.daily == nil || s.auditOnly || s.state.Config.Version != 2 || len(unsigned) == 0 || len(unsigned) > pooltxshape.MaxStandardWeight {
		return nil, ErrInvalid
	}
	body := signingRequestBody{Version: 2, Config: s.state.Config, Source: s.state.Source, Inputs: inputs, Payments: payments, FeeAtoms: fee, Change: change, Unsigned: hex.EncodeToString(unsigned)}
	if err := dailySigningBounds(body); err != nil {
		return nil, err
	}
	body.Config.Destinations = append([]Destination(nil), body.Config.Destinations...)
	body.Inputs = dailyOwnedCoins(inputs)
	body.Payments = append([]Payment(nil), payments...)
	if change != nil {
		own := dailyOwnedCoins([]Coin{*change})
		body.Change = &own[0]
	}
	digest, err := dailySigningDigest(ctx, body)
	if err != nil {
		return nil, err
	}
	r := &dailySigningRequest{store: s, backend: s.backend, parent: ctx, body: body, unsigned: append([]byte(nil), unsigned...), digest: digest}
	r.active.Store(true)
	defer r.active.Store(false)
	return s.backend.Sign(context.WithValue(ctx, dailySigningContextKey{}, r), dailyOwnedCoins(inputs), append([]byte(nil), unsigned...))
}
