// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"math"

	"github.com/taytr-llc/slug/prospectworker"
)

// These counts are retained delivery state, not fighter ownership or chain
// finality. List performs the original outbox's canonical row validation.
func (a *RemotePoolAttachment) counts(ctx context.Context) (records, pending uint64, err error) {
	var after uint64
	for {
		if e := ctx.Err(); e != nil {
			return 0, 0, e
		}
		rows, e := a.box.List(after, 100)
		if e != nil {
			return 0, 0, e
		}
		if len(rows) == 0 {
			return records, pending, nil
		}
		for _, r := range rows {
			if r.Sequence != after+1 || r.Sequence > a.workerConfig.MaxRecords {
				return 0, 0, ErrRemotePoolChanged
			}
			after = r.Sequence
			records++
			todo := r.PoolState == "pending"
			if f := r.Fighter; f != nil {
				todo = todo || f.BlockState == "pending" || f.AdmissionState == "needs-signature" || f.AdmissionState == "pending"
			}
			if todo {
				pending++
			}
		}
	}
}
func (a *RemotePoolAttachment) Snapshot(ctx context.Context, view string) (RemotePoolSnapshot, error) {
	work, end, e := a.begin(ctx, view)
	if e != nil {
		return RemotePoolSnapshot{}, e
	}
	defer end()
	a.mu.Lock()
	state, code, status, run := a.state, a.lastError, a.last, a.run
	a.mu.Unlock()
	if run != nil {
		status = run.worker.Status()
		state = "running"
	}
	const maxSafe = uint64(9007199254740991)
	rate := status.HashesPerSecond * 1000
	if status.Attempts > maxSafe || math.IsNaN(rate) || math.IsInf(rate, 0) || rate < 0 || rate > float64(maxSafe) {
		return RemotePoolSnapshot{}, ErrRemotePoolUnavailable
	}
	records, pending, e := a.counts(work)
	if e != nil {
		return RemotePoolSnapshot{}, e
	}
	out := RemotePoolSnapshot{Version: 1, ConfigID: a.configID, View: a.view, SourceBindingID: a.config.Miners.SourceID, OriginalSourceBindingID: a.config.ExpectedLocal.OriginalSourceBindingID, State: state, WorkerState: remotePoolWorkerState(status.State), LastError: code, Attempts: status.Attempts, HashesPerSecondMilli: uint64(rate), Records: records, Pending: pending, WalletState: "unavailable", PayoutAddress: a.config.ExpectedLocal.Recipient.Recipient.Address}
	if out.LastError == "" {
		out.LastError = remotePoolPublicCode(status.LastError)
	}
	if run == nil {
		out.HashesPerSecondMilli = 0
	}
	w, we := a.local.session.runtime.Snapshot(work)
	if we == nil && w.Height >= 0 && w.Spendable >= 0 && w.Immature >= 0 && w.Unconfirmed >= 0 && w.Spendable <= 21000000*100000000 && w.Immature <= 21000000*100000000 && w.Unconfirmed <= 21000000*100000000 {
		out.WalletState = "syncing"
		if w.Synced {
			out.WalletState = "ready"
		}
		out.WalletHeight = uint32(w.Height)
		out.SpendableAtoms = w.Spendable
		out.ImmatureAtoms = w.Immature
		out.UnconfirmedAtoms = w.Unconfirmed
	}
	if e = a.check(work); e != nil {
		return RemotePoolSnapshot{}, e
	}
	return out, nil
}

var _ prospectworker.Node = (*remotePoolNode)(nil)

// All other worker errors remain a bounded unavailable code, never raw text.
func remotePoolPublicCode(code string) string {
	switch code {
	case "", "unavailable", "outcome_unknown", "busy", "capacity", "rate_limited", "expired", "rejected", "not_found", "unauthorized", "forbidden", "invalid_request", "too_large", "stale", "clock", "cycle_unavailable", "signer_unavailable", "worker_unavailable", "cleanup_incomplete":
		return code
	default:
		return "unavailable"
	}
}
func remotePoolWorkerState(state string) string {
	switch state {
	case "ready", "stopped", "failed", "paused", "preparing", "retrying", "hashing", "delivering":
		return state
	case "":
		return "stopped"
	default:
		return "unavailable"
	}
}
