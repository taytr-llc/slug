// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/desktop/pilot"
	"io"
	"os/signal"
	"syscall"
)

type dailyWalletLaunchOwner2 struct{ *dailyWalletLaunchOwner }
type dailyWalletActivityResult struct {
	job         pilot.RemoteWalletActivityJob
	read        pilot.RemoteWalletActivityRead
	page        pilot.RemoteWalletActivityPage
	detail      pilot.RemoteWalletActivityDetail
	observation pilot.RemoteDailyPayoutObservation
}

// Fixed dispatch preserves the original protected file and controller owner.
func (o *dailyWalletLaunchOwner2) activityCall(ctx context.Context, command, view string, sequence, batch uint64, after, limit uint32, txid string, index uint32) (out dailyWalletActivityResult, resultErr error) {
	if o == nil || o.dailyWalletLaunchOwner == nil || o.remoteLaunchOwner == nil {
		return out, errRemoteLaunchUnavailable
	}
	o.mu.Lock()
	e := o.checkLocked(ctx)
	if e == nil && o.controller == nil {
		e = errRemoteLaunchUnavailable
	}
	if e != nil {
		o.mu.Unlock()
		return out, errors.Join(e, o.Close())
	}
	o.active.Add(1)
	work, cancel := context.WithCancel(ctx)
	joined := make(chan struct{})
	stop := context.AfterFunc(o.life, func() { defer close(joined); cancel() })
	o.mu.Unlock()
	defer func() {
		if !stop() {
			<-joined
		}
		cancel()
		o.mu.Lock()
		check := o.checkLocked(ctx)
		o.mu.Unlock()
		o.active.Done()
		if check != nil {
			out = dailyWalletActivityResult{}
			resultErr = errors.Join(resultErr, check, o.Close())
		}
	}()
	switch command {
	case "begin":
		out.job, e = o.controller.BeginWalletActivityRead(work, view, sequence)
	case "read":
		out.read, e = o.controller.WalletActivityRead(work, view, sequence)
	case "cancel":
		e = o.controller.CancelWalletActivityRead(work, view, sequence)
	case "page":
		out.page, e = o.controller.WalletActivityPage(work, view, sequence, after, limit)
	case "detail":
		out.detail, e = o.controller.WalletActivityDetail(work, view, sequence, txid)
	case "observe":
		out.observation, e = o.controller.ObserveDailyPayout(work, view, sequence, batch, txid, index)
	default:
		e = errRemoteLaunchInvalid
	}
	return out, e
}
func (o *dailyWalletLaunchOwner2) BeginWalletActivityRead(ctx context.Context, v string, s uint64) (pilot.RemoteWalletActivityJob, error) {
	r, e := o.activityCall(ctx, "begin", v, s, 0, 0, 0, "", 0)
	return r.job, e
}
func (o *dailyWalletLaunchOwner2) WalletActivityRead(ctx context.Context, v string, s uint64) (pilot.RemoteWalletActivityRead, error) {
	r, e := o.activityCall(ctx, "read", v, s, 0, 0, 0, "", 0)
	return r.read, e
}
func (o *dailyWalletLaunchOwner2) CancelWalletActivityRead(ctx context.Context, v string, s uint64) error {
	_, e := o.activityCall(ctx, "cancel", v, s, 0, 0, 0, "", 0)
	return e
}
func (o *dailyWalletLaunchOwner2) WalletActivityPage(ctx context.Context, v string, s uint64, a, l uint32) (pilot.RemoteWalletActivityPage, error) {
	r, e := o.activityCall(ctx, "page", v, s, 0, a, l, "", 0)
	return r.page, e
}
func (o *dailyWalletLaunchOwner2) WalletActivityDetail(ctx context.Context, v string, s uint64, tx string) (pilot.RemoteWalletActivityDetail, error) {
	r, e := o.activityCall(ctx, "detail", v, s, 0, 0, 0, tx, 0)
	return r.detail, e
}
func (o *dailyWalletLaunchOwner2) ObserveDailyPayout(ctx context.Context, v string, s, b uint64, tx string, i uint32) (pilot.RemoteDailyPayoutObservation, error) {
	r, e := o.activityCall(ctx, "observe", v, s, b, 0, 0, tx, i)
	return r.observation, e
}
func runDailyWalletLaunch2(ctx context.Context, input io.ReadCloser, output io.WriteCloser, p remoteLaunchPaths, options pilot.PoolOptions) error {
	original, _, e := openRemoteLaunchSelection(ctx, p, options, true, true)
	if e != nil {
		return e
	}
	owner := &dailyWalletLaunchOwner2{&dailyWalletLaunchOwner{original}}
	// Read the exact already-owned public config again, never a second path or
	// credential owner. Its descriptor is rechecked throughout all later calls.
	raw, e := original.files[0].read(ctx)
	var selected pilot.RecoverableRemotePoolConfig
	if e == nil {
		selected, e = pilot.ParseRecoverableRemotePoolConfig(raw)
	}
	clear(raw)
	if e == nil {
		signal.Ignore(syscall.SIGPIPE)
		e = pilot.ServeDailyWallet2(ctx, input, output, owner, selected)
	}
	closed := original.Close()
	if e == nil && closed == nil {
		return nil
	}
	failure := &remoteLaunchError{cause: errors.Join(e, closed)}
	if closed != nil {
		failure.retained = original
	}
	return failure
}

var _ pilot.DailyWalletControllerAPI2 = (*dailyWalletLaunchOwner2)(nil)
