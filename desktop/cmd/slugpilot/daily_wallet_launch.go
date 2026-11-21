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

// This wrapper retains the same original input/controller owner. Embedded
// control methods keep their original before/after file and lifetime checks.
type dailyWalletLaunchOwner struct{ *remoteLaunchOwner }

func (o *dailyWalletLaunchOwner) DailyWalletSnapshot(ctx context.Context) (pilot.RemotePoolWireSnapshot, error) {
	if o == nil || o.remoteLaunchOwner == nil {
		return pilot.RemotePoolWireSnapshot{}, errRemoteLaunchUnavailable
	}
	s, e := o.dailyCall(ctx, "snapshot", "")
	if e != nil {
		return pilot.RemotePoolWireSnapshot{}, e
	}
	return pilot.RemotePoolWireSnapshot{Version: s.Version, ConfigID: s.ConfigID, State: s.State, Account: s.Account, View: s.View, Remote: s.Remote, Error: s.Error}, nil
}

// Fixed read dispatch keeps public config descriptors alive over the original
// controller's own admission/disclosure fences. No caller callback is exposed.
func (o *dailyWalletLaunchOwner) payoutCall(ctx context.Context, command, view string, sequence, capture, before uint64) (job pilot.RemoteDailyPayoutJob, read pilot.RemoteDailyPayoutRead, resultErr error) {
	if o == nil || o.remoteLaunchOwner == nil {
		return job, read, errRemoteLaunchUnavailable
	}
	o.mu.Lock()
	e := o.checkLocked(ctx)
	if e == nil && o.controller == nil {
		e = errRemoteLaunchUnavailable
	}
	if e != nil {
		o.mu.Unlock()
		return job, read, errors.Join(e, o.Close())
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
			job = pilot.RemoteDailyPayoutJob{}
			read = pilot.RemoteDailyPayoutRead{}
			resultErr = errors.Join(resultErr, check, o.Close())
		}
	}()
	switch command {
	case "begin":
		job, e = o.controller.BeginDailyPayoutRead(work, view, sequence, capture, before)
	case "read":
		read, e = o.controller.DailyPayoutRead(work, view, sequence)
	case "cancel":
		e = o.controller.CancelDailyPayoutRead(work, view, sequence)
	default:
		e = errRemoteLaunchInvalid
	}
	return job, read, e
}
func (o *dailyWalletLaunchOwner) BeginDailyPayoutRead(ctx context.Context, view string, sequence, capture, before uint64) (pilot.RemoteDailyPayoutJob, error) {
	j, _, e := o.payoutCall(ctx, "begin", view, sequence, capture, before)
	return j, e
}
func (o *dailyWalletLaunchOwner) DailyPayoutRead(ctx context.Context, view string, sequence uint64) (pilot.RemoteDailyPayoutRead, error) {
	_, r, e := o.payoutCall(ctx, "read", view, sequence, 0, 0)
	return r, e
}
func (o *dailyWalletLaunchOwner) CancelDailyPayoutRead(ctx context.Context, view string, sequence uint64) error {
	_, _, e := o.payoutCall(ctx, "cancel", view, sequence, 0, 0)
	return e
}

func runDailyWalletLaunch(ctx context.Context, input io.ReadCloser, output io.WriteCloser, p remoteLaunchPaths, options pilot.PoolOptions) error {
	original, _, e := openRemoteLaunchSelection(ctx, p, options, true, true)
	if e != nil {
		return e
	}
	owner := &dailyWalletLaunchOwner{original}
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
		e = pilot.ServeDailyWallet(ctx, input, output, owner, selected)
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

var _ pilot.DailyWalletControllerAPI = (*dailyWalletLaunchOwner)(nil)
