// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"github.com/taytr-llc/slug/poolsettle"
	"time"
)

// Remote settlement fields remain dated under their original revision. The
// final fields report only what this same already-owned wallet observed.
type RemoteDailyPayoutObservation struct {
	CaptureSequence                      uint64
	Binding                              RemoteDailyPayoutBinding
	Revision, HighBatch                  uint64
	CapturedAtMS, SettlementObservedAtMS int64
	Stale                                bool
	BatchSequence                        uint64
	Payment                              poolsettle.PayoutItem
	ObservedAtMS                         int64
	State, Error                         string
	WalletSynced                         bool
	WalletHeight                         uint32
	WalletTip                            string
	Confirmations                        uint32
	BlockHash                            string
}

func selectedDailyPayment(c *RemoteDailyPayoutCapture, sequence, batch uint64, txid string, index uint32) (poolsettle.PayoutItem, error) {
	if c == nil || sequence == 0 || c.Sequence != sequence {
		return poolsettle.PayoutItem{}, ErrDailyPayoutReadNotFound
	}
	if batch == 0 || batch > c.First.HighBatch || !payoutLowerHex(txid, 64, false) || index > 255 {
		return poolsettle.PayoutItem{}, ErrDailyPayoutReadInvalid
	}
	var selected poolsettle.PayoutItem
	found := false
	for _, page := range []poolsettle.DailyPayoutPage{c.First, c.Current} {
		for _, row := range page.Items {
			if row.BatchSequence == batch && row.Payment.TxID == txid && row.Payment.OutputIndex == index {
				// First==Current, or a cached first-window replay, is an identical copy.
				// Conflicting copies must never become a caller-selected amount.
				if found && selected != row.Payment {
					return poolsettle.PayoutItem{}, ErrRemotePoolChanged
				}
				selected, found = row.Payment, true
			}
		}
	}
	if !found {
		return poolsettle.PayoutItem{}, ErrDailyPayoutReadNotFound
	}
	return selected, nil
}
func (a *RemotePoolAttachment) ObserveDailyPayout(ctx context.Context, view string, sequence, batch uint64, txid string, index uint32) (RemoteDailyPayoutObservation, error) {
	if e := a.activitySelected(); e != nil {
		return RemoteDailyPayoutObservation{}, e
	}
	work, end, e := a.begin(ctx, view)
	if e != nil {
		return RemoteDailyPayoutObservation{}, e
	}
	defer end()
	work, cancel := context.WithTimeout(work, 5*time.Second)
	defer cancel()
	var cache *RemoteDailyPayoutCapture
	var out RemoteDailyPayoutObservation
	e = a.dailyPayoutFence(work, func() error {
		cache = a.dailyPayout
		payment, err := selectedDailyPayment(cache, sequence, batch, txid, index)
		if err != nil {
			return err
		}
		out = RemoteDailyPayoutObservation{CaptureSequence: sequence, Binding: cache.Binding, Revision: cache.First.Revision, HighBatch: cache.First.HighBatch, CapturedAtMS: cache.CapturedAtMS, SettlementObservedAtMS: cache.First.ObservedAtMS, Stale: cache.Stale, BatchSequence: batch, Payment: payment}
		return nil
	})
	if e != nil {
		return RemoteDailyPayoutObservation{}, e
	}
	read, e := a.local.session.runtime.ObservePoolPayout(work, walletPayoutSelection{TxID: txid, OutputIndex: index, Atoms: out.Payment.Atoms, RecipientScript: out.Binding.RecipientScript})
	if work.Err() != nil {
		return RemoteDailyPayoutObservation{}, work.Err()
	}
	if e == errWalletPayoutBusy {
		return RemoteDailyPayoutObservation{}, ErrRemotePoolBusy
	}
	if e != nil {
		out.State, out.Error, out.ObservedAtMS = "unavailable", "wallet_unavailable", time.Now().UnixMilli()
	} else {
		out.State, out.ObservedAtMS, out.WalletSynced = read.State, read.ObservedAtMS, true
		out.WalletHeight, out.WalletTip, out.Confirmations, out.BlockHash = read.WalletHeight, read.WalletTip, read.Confirmations, read.BlockHash
	}
	e = a.dailyPayoutFence(work, func() error {
		if a.dailyPayout != cache {
			return ErrDailyPayoutReadNotFound
		}
		return nil
	})
	if e != nil {
		return RemoteDailyPayoutObservation{}, e
	}
	return out, nil
}
