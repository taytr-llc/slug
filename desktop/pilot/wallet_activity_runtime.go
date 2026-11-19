// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"time"

	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/wire/v2"
	"github.com/taytr-llc/slugwallet/wallet"
)

type walletActivityRead struct {
	snapshot wallet.OwnActivitySnapshot
	at       int64
}

// The admitted child task owns wallet lifetime. Network calls happen outside
// r.mu; only bounded local reads and live sync samples take that mutex.
func (r *walletRuntime) captureWalletActivity(ctx context.Context, config prospectnode.RPCConfig) (walletActivityRead, error) {
	if ctx == nil || r == nil || wire.TestPoW {
		return walletActivityRead{}, ErrActivityUnavailable
	}
	node, e := newWithdrawalNode(config)
	if e != nil {
		return walletActivityRead{}, ErrActivityUnavailable
	}
	defer node.close()
	tip := func(ctx context.Context) (localTip, error) {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		var out localTip
		if node.read(ctx, "getbestblock", nil, &out, "hash", "height") != nil || out.Height < 0 || !payoutNonzeroID(out.Hash) {
			return localTip{}, ErrActivityUnavailable
		}
		return out, nil
	}
	local := func(ctx context.Context, read bool) (walletActivityRead, localTip, error) {
		if !r.mu.TryLock() {
			return walletActivityRead{}, localTip{}, gymstable.ErrBusy
		}
		defer r.mu.Unlock()
		sample := func() (localTip, error) {
			if r.closed || r.wallet == nil || r.wallet.Manager == nil || r.backend == nil || r.backend.Disconnected() || !r.wallet.ChainSynced() {
				return localTip{}, ErrActivityUnavailable
			}
			s := r.wallet.Manager.SyncedTo()
			if s.Height < 0 || !payoutNonzeroID(s.Hash.String()) {
				return localTip{}, ErrActivityUnavailable
			}
			return localTip{s.Hash.String(), s.Height}, nil
		}
		before, e := sample()
		if e != nil {
			return walletActivityRead{}, localTip{}, e
		}
		var out walletActivityRead
		if read {
			out.snapshot, e = r.wallet.CaptureOwnActivity(ctx)
			if e != nil {
				return walletActivityRead{}, localTip{}, e
			}
			if out.snapshot.RootPublicKey != r.identity.RootPublicKey || out.snapshot.GymPublicKey != r.identity.GymPublicKey || out.snapshot.GymID != r.identity.GymID || out.snapshot.SyncedHeight != uint32(before.Height) || out.snapshot.SyncedHash.String() != before.Hash {
				return walletActivityRead{}, localTip{}, ErrActivityUnavailable
			}
		}
		after, e := sample()
		if e != nil || before != after {
			return walletActivityRead{}, localTip{}, ErrActivityUnavailable
		}
		if e = ctx.Err(); e != nil {
			return walletActivityRead{}, localTip{}, e
		}
		return out, after, nil
	}
	return captureWalletActivityObserved(ctx, tip, local, time.Now)
}

// Finite seam for races in a dated local observation. It cannot create wallet
// authority, and the concrete entry above supplies both retained owners.
func captureWalletActivityObserved(ctx context.Context, tip func(context.Context) (localTip, error), local func(context.Context, bool) (walletActivityRead, localTip, error), clock func() time.Time) (walletActivityRead, error) {
	if ctx == nil || tip == nil || local == nil || clock == nil {
		return walletActivityRead{}, ErrActivityUnavailable
	}
	if e := ctx.Err(); e != nil {
		return walletActivityRead{}, e
	}
	before, e := tip(ctx)
	if e != nil {
		return walletActivityRead{}, e
	}
	out, live, e := local(ctx, true)
	if e != nil {
		return walletActivityRead{}, e
	}
	if live != before {
		return walletActivityRead{}, ErrActivityUnavailable
	}
	after, e := tip(ctx)
	if e != nil {
		return walletActivityRead{}, e
	}
	_, last, e := local(ctx, false)
	if e != nil {
		return walletActivityRead{}, e
	}
	if after != before || last != before {
		return walletActivityRead{}, ErrActivityUnavailable
	}
	if e = ctx.Err(); e != nil {
		return walletActivityRead{}, e
	}
	out.at = clock().UnixMilli()
	if !payoutTime(out.at, false) {
		return walletActivityRead{}, ErrActivityUnavailable
	}
	return out, nil
}
func walletActivityTaskError(e error) string {
	if errors.Is(e, gymstable.ErrCapacity) || errors.Is(e, wallet.ErrOwnActivityCapacity) {
		return "capacity"
	}
	if errors.Is(e, gymstable.ErrBusy) {
		return "busy"
	}
	return "activity_unavailable"
}
