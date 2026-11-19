// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"github.com/taytr-llc/slug/walletwithdraw"
	"github.com/taytr-llc/slugwallet/wallet"
)

// Private concrete-resource seam. It carries no account authority; every durable
// application publication uses the original child's separate capability fence.
type withdrawalBackend interface {
	Prepare(context.Context, walletwithdraw.Request) (walletwithdraw.Draft, error)
	WalletRecord(context.Context, string) (wallet.CandidateWithdrawalRecord, error)
	Sign(context.Context, walletwithdraw.Draft) ([]byte, error)
	Reconcile(context.Context, *walletwithdraw.Reconciliation) error
	Inspect(context.Context, walletwithdraw.Draft) error
	Deliver(context.Context, *walletwithdraw.Delivery) (walletwithdraw.DeliveryResult, string, error)
	Observe(context.Context, *walletwithdraw.Owner, string) (walletwithdraw.ObservationInput, error)
	Close() error
}

var _ withdrawalBackend = (*poolWithdrawalResources)(nil)
