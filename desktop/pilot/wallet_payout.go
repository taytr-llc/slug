package pilot

import (
	"context"
	"errors"
	"time"

	"github.com/taytr-llc/slug/poolsettle"
)

// This is a dated read of the already-owned wallet, not payment authority.
// The child owns selection membership and original-view publication fences.
type walletPayoutSelection struct {
	TxID            string
	OutputIndex     uint32
	Atoms           int64
	RecipientScript string
}

type walletPayoutRead struct {
	State         string
	ObservedAtMS  int64
	WalletHeight  uint32
	WalletTip     string
	Confirmations uint32
	BlockHash     string
}

var (
	errWalletPayoutUnavailable = errors.New("wallet payout observation unavailable")
	errWalletPayoutBusy        = errors.New("wallet payout observation busy")
)

const (
	maxWalletPayoutResponse = 524288
	maxWalletPayoutTime     = int64(253402300799999)
)

func payoutLowerHex(s string, size int, nonzero bool) bool {
	if len(s) != size {
		return false
	}
	zero := true
	for _, c := range s {
		if c < '0' || c > '9' && c < 'a' || c > 'f' {
			return false
		}
		zero = zero && c == '0'
	}
	return !nonzero || !zero
}

func (s walletPayoutSelection) valid() bool {
	return payoutLowerHex(s.TxID, 64, false) && s.OutputIndex <= 255 &&
		s.Atoms >= poolsettle.MinimumAtoms && s.Atoms <= poolsettle.MaxMoney &&
		payoutLowerHex(s.RecipientScript, 44, false) && s.RecipientScript[:4] == "0014"
}

type walletPayoutTip struct {
	synced bool
	height int32
	hash   string
}

func (t walletPayoutTip) valid() bool {
	return t.synced && t.height >= 0 && payoutLowerHex(t.hash, 64, true)
}

// ObservePoolPayout retains wallet lifetime across exactly one RPC. The wallet's
// sync flag can lag a disconnect; this is explicitly a wallet-reported view.
func (r *walletRuntime) ObservePoolPayout(ctx context.Context, selected walletPayoutSelection) (walletPayoutRead, error) {
	if ctx == nil || r == nil || !selected.valid() {
		return walletPayoutRead{}, errWalletPayoutUnavailable
	}
	if err := ctx.Err(); err != nil {
		return walletPayoutRead{}, err
	}
	if !r.mu.TryLock() {
		return walletPayoutRead{}, errWalletPayoutBusy
	}
	defer r.mu.Unlock()
	if r.closed || r.wallet == nil || r.wallet.Manager == nil || r.rpc == nil {
		return walletPayoutRead{}, errWalletPayoutUnavailable
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	sample := func() walletPayoutTip {
		stamp := r.wallet.Manager.SyncedTo()
		return walletPayoutTip{r.wallet.ChainSynced(), stamp.Height, stamp.Hash.String()}
	}
	return observeWalletPayout(ctx, selected, sample, r.rpc.payoutTransaction, time.Now)
}

// The finite seam lets tests model synchronization races without inventing a
// second live wallet or weakening the concrete owner above.
func observeWalletPayout(ctx context.Context, selected walletPayoutSelection,
	sample func() walletPayoutTip,
	query func(context.Context, string) (walletPayoutTransaction, bool, error),
	clock func() time.Time) (walletPayoutRead, error) {
	if ctx == nil || !selected.valid() || sample == nil || query == nil || clock == nil {
		return walletPayoutRead{}, errWalletPayoutUnavailable
	}
	if err := ctx.Err(); err != nil {
		return walletPayoutRead{}, err
	}
	before := sample()
	if !before.valid() {
		return walletPayoutRead{}, errWalletPayoutUnavailable
	}
	tx, missing, err := query(ctx, selected.TxID)
	if ctx.Err() != nil {
		return walletPayoutRead{}, ctx.Err()
	}
	if err != nil {
		return walletPayoutRead{}, errWalletPayoutUnavailable
	}
	after := sample()
	if !after.valid() || before != after {
		return walletPayoutRead{}, errWalletPayoutUnavailable
	}
	result := walletPayoutRead{State: "not_observed", WalletHeight: uint32(after.height), WalletTip: after.hash}
	if !missing {
		if tx.txid != selected.TxID || tx.confirmations < 0 || tx.confirmations > int64(after.height)+1 ||
			!validateWalletPayoutTransaction(tx.raw, selected) {
			return walletPayoutRead{}, errWalletPayoutUnavailable
		}
		result.State = "unconfirmed"
		if tx.confirmations == 0 {
			if tx.blockHash != "" {
				return walletPayoutRead{}, errWalletPayoutUnavailable
			}
		} else {
			if !payoutLowerHex(tx.blockHash, 64, true) {
				return walletPayoutRead{}, errWalletPayoutUnavailable
			}
			result.State, result.BlockHash = "confirmed", tx.blockHash
			result.Confirmations = uint32(tx.confirmations)
		}
	}
	result.ObservedAtMS = clock().UnixMilli()
	if result.ObservedAtMS <= 0 || result.ObservedAtMS > maxWalletPayoutTime {
		return walletPayoutRead{}, errWalletPayoutUnavailable
	}
	if err := ctx.Err(); err != nil {
		return walletPayoutRead{}, err
	}
	return result, nil
}
