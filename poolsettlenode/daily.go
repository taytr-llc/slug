// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettlenode

import (
	"context"
	"encoding/hex"
	"math"
	"sync"
	"sync/atomic"

	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/pooltxshape"
	"github.com/taytr-llc/slug/wire/v2"
)

const dailyMaxClaims = 65536
const dailyMaxOrigins = 65536

// Inspect authenticates the whole live frontier before settlement decides
// whether the ready set fits one transaction. This read-only bound must not
// become a signing, fee-quote or broadcast input limit.
const dailyMaxFrontierCoins = 16384
const dailyWorkers = 4

func (b *Client) inputLimit() int {
	if b.daily {
		return pooltxshape.MaxNativeInputs
	}
	return poolsettle.MaxInputs
}
func (b *Client) rawLimit() int {
	if b.daily {
		return pooltxshape.MaxStandardWeight
	}
	return poolsettle.MaxTransactionBytes
}
func (b *Client) quoteVSize(inputs, payments int) (int64, error) {
	if !b.daily {
		return conservativeVSize(inputs, payments)
	}
	shape, err := pooltxshape.NativeP2WPKH(inputs, payments, true)
	if err != nil {
		return 0, ErrInvalid
	}
	return shape.VSize, nil
}

// Shape checks are not signature verification or journal authority. Broadcast
// still makes exactly one send and leaves uncertainty to the original Store.
func (b *Client) dailyBroadcastShape(tx *wire.MsgTx) error {
	change := false
	for i, out := range tx.TxOut {
		if len(out.PkScript) != 22 || out.PkScript[0] != 0 || out.PkScript[1] != 20 {
			return ErrInvalid
		}
		if hex.EncodeToString(out.PkScript) == b.config.PoolScript {
			if i != len(tx.TxOut)-1 {
				return ErrInvalid
			}
			change = true
		}
	}
	payees := len(tx.TxOut)
	if change {
		payees--
	}
	shape, err := pooltxshape.NativeP2WPKH(len(tx.TxIn), payees, change)
	if err != nil {
		return ErrInvalid
	}
	for _, in := range tx.TxIn {
		if len(in.Witness) != 2 || len(in.Witness[0]) < 2 || len(in.Witness[0]) > 73 || len(in.Witness[1]) != 33 {
			return ErrInvalid
		}
	}
	stripped, raw := int64(tx.SerializeSizeStripped()), int64(tx.SerializeSize())
	if stripped != shape.StrippedBytes || raw > shape.RawBytes || 3*stripped+raw > pooltxshape.MaxStandardWeight {
		return ErrInvalid
	}
	return nil
}

type dailyOrigin struct {
	hash      string
	height    uint32
	canonical string
}
type dailyPlan struct {
	coins   []poolsettle.Coin
	origins []dailyOrigin
	byCoin  [][]int
}

// Validate all counts and origin identities before copying or network I/O.
// A second pass takes bounded owned copies; no caller slice survives admission.
func (b *Client) dailyPlan(ctx context.Context, coins []poolsettle.Coin) (dailyPlan, error) {
	var p dailyPlan
	if len(coins) > dailyMaxFrontierCoins {
		return p, ErrUnavailable
	}
	total := 0
	for _, c := range coins {
		if err := ctx.Err(); err != nil {
			return p, err
		}
		if len(c.Claims) == 0 || len(c.Claims) > 256*257 || len(c.Claims) > dailyMaxClaims-total {
			return p, ErrUnavailable
		}
		total += len(c.Claims)
	}
	seen := make(map[struct {
		id    string
		index uint32
	}]bool, len(coins))
	indices := make(map[string]int)
	for _, c := range coins {
		if err := ctx.Err(); err != nil {
			return dailyPlan{}, err
		}
		if !b.validCoin(c) {
			return dailyPlan{}, ErrUnavailable
		}
		key := struct {
			id    string
			index uint32
		}{c.TxID, c.Index}
		if seen[key] {
			return dailyPlan{}, ErrUnavailable
		}
		seen[key] = true
		for i, claim := range c.Claims {
			if i%256 == 0 {
				if err := ctx.Err(); err != nil {
					return dailyPlan{}, err
				}
			}
			if !hash(claim.RoundHash) || claim.RoundHeight == 0 || claim.RoundHeight > math.MaxInt32 {
				return dailyPlan{}, ErrUnavailable
			}
			if index, ok := indices[claim.RoundHash]; ok {
				if p.origins[index].height != claim.RoundHeight {
					return dailyPlan{}, ErrUnavailable
				}
			} else {
				if len(p.origins) >= dailyMaxOrigins {
					return dailyPlan{}, ErrUnavailable
				}
				indices[claim.RoundHash] = len(p.origins)
				p.origins = append(p.origins, dailyOrigin{hash: claim.RoundHash, height: claim.RoundHeight})
			}
		}
	}
	p.coins = make([]poolsettle.Coin, len(coins))
	p.byCoin = make([][]int, len(coins))
	for i, c := range coins {
		if err := ctx.Err(); err != nil {
			return dailyPlan{}, err
		}
		p.coins[i] = c
		p.coins[i].Claims = append([]poolsettle.Claim(nil), c.Claims...)
		p.byCoin[i] = make([]int, len(c.Claims))
		for j, claim := range p.coins[i].Claims {
			p.byCoin[i][j] = indices[claim.RoundHash]
		}
	}
	return p, ctx.Err()
}

// At most four synchronous RPC chains run at once. First failure cancels and
// joins every worker before returning. This private helper carries no authority
// and never retains work across Client operations or releases the original gate.
func dailyParallel(ctx context.Context, count int, run func(context.Context, int) error) error {
	work, cancel := context.WithCancel(ctx)
	defer cancel()
	var next atomic.Int64
	var wg sync.WaitGroup
	errors := make(chan error, 1)
	workers := dailyWorkers
	if count < workers {
		workers = count
	}
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				if work.Err() != nil {
					return
				}
				i := int(next.Add(1) - 1)
				if i >= count {
					return
				}
				if err := run(work, i); err != nil {
					select {
					case errors <- err:
					default:
					}
					cancel()
					return
				}
			}
		}()
	}
	wg.Wait()
	select {
	case err := <-errors:
		return err
	default:
	}
	return ctx.Err()
}

func (b *Client) inspectDaily(ctx context.Context, coins []poolsettle.Coin) ([]poolsettle.Funding, error) {
	p, err := b.dailyPlan(ctx, coins)
	if err != nil {
		return nil, err
	}
	before, err := b.snapshot(ctx)
	if err != nil {
		return nil, err
	}
	if err = dailyParallel(ctx, len(p.origins), func(work context.Context, i int) error {
		o := &p.origins[i]
		if o.height > before.Height {
			return nil
		}
		if b.call(work, "getblockhash", []any{o.height}, &o.canonical) != nil || !hash(o.canonical) {
			return ErrUnavailable
		}
		return nil
	}); err != nil {
		return nil, err
	}
	out := make([]poolsettle.Funding, len(p.coins))
	if err = dailyParallel(ctx, len(p.coins), func(work context.Context, i int) error {
		c := p.coins[i]
		f := poolsettle.Funding{TxID: c.TxID, Index: c.Index, State: "funding-conflict"}
		for _, index := range p.byCoin[i] {
			o := p.origins[index]
			if o.canonical != o.hash {
				out[i] = f
				return nil
			}
		}
		var err error
		out[i], err = b.inspectCoin(work, c, before, f)
		return err
	}); err != nil {
		return nil, err
	}
	if err = dailyParallel(ctx, len(p.origins), func(work context.Context, i int) error {
		o := p.origins[i]
		if o.height > before.Height {
			return nil
		}
		var canonical string
		if b.call(work, "getblockhash", []any{o.height}, &canonical) != nil || canonical != o.canonical {
			return ErrUnavailable
		}
		return nil
	}); err != nil {
		return nil, err
	}
	if err = b.unchanged(ctx, before); err != nil {
		return nil, err
	}
	return out, nil
}
