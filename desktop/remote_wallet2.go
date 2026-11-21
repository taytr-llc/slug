// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/wire/v2"
	"strings"
	"sync"
	"time"

	"github.com/taytr-llc/slug/desktop/internal/bridge"
	"github.com/taytr-llc/slug/desktop/pilot"
)

// No retained path, RPC value, token or arbitrary cause is displayed by Error.
type nativeDailyWalletError2 struct {
	cause    error
	retained *nativeDailyWalletOwner2
}

func (e *nativeDailyWalletError2) Error() string {
	var failure *bridge.RemotePoolFailure
	errors.As(e.cause, &failure)
	if errors.Is(e.cause, bridge.ErrRemotePoolOutcomeUnknown) || failure != nil && failure.Code == "outcome_unknown" {
		return "The remote request outcome is unknown. Do not repeat it automatically; reopen the original attachment to inspect its saved state."
	}
	if e.retained != nil || errors.Is(e.cause, errNativeRemoteCleanup) || errors.Is(e.cause, bridge.ErrRemotePoolBridgeCleanup) || failure != nil && failure.Code == "cleanup_incomplete" {
		return errNativeRemoteCleanup.Error()
	}
	if errors.Is(e.cause, bridge.ErrBusy) {
		return "The remote gym is busy. Wait for its current request to finish."
	}
	if failure != nil && failure.Code == "busy" {
		return "The remote gym is busy. Wait for its current request to finish."
	}
	return errNativeRemoteUnavailable.Error()
}
func (e *nativeDailyWalletError2) Unwrap() error { return e.cause }

// Any explicit version selector enters this strict route, even an invalid one.
func nativeDailyWalletRequested2(args []string) bool {
	for _, a := range args {
		if strings.HasPrefix(a, "-") && strings.SplitN(strings.TrimLeft(a, "-"), "=", 2)[0] == "daily-wallet-helper-version" {
			return true
		}
	}
	return false
}
func parseNativeDailyWalletArgs2(args []string) (bridge.RemotePoolLaunchSelection, error) {
	clean := make([]string, 0, len(args))
	seen := false
	for i := 0; i < len(args); i++ {
		a := args[i]
		parts := strings.SplitN(strings.TrimLeft(a, "-"), "=", 2)
		if strings.HasPrefix(a, "-") && parts[0] == "daily-wallet-helper-version" {
			if seen {
				return bridge.RemotePoolLaunchSelection{}, errNativeRemoteInvalid
			}
			seen = true
			v := ""
			if len(parts) == 2 {
				v = parts[1]
			} else {
				i++
				if i >= len(args) {
					return bridge.RemotePoolLaunchSelection{}, errNativeRemoteInvalid
				}
				v = args[i]
			}
			if v != "2" {
				return bridge.RemotePoolLaunchSelection{}, errNativeRemoteInvalid
			}
		} else {
			clean = append(clean, a)
		}
	}
	if !seen {
		return bridge.RemotePoolLaunchSelection{}, errNativeRemoteInvalid
	}
	return parseNativeDailyWalletArgs(clean)
}

type nativeDailyWalletOwner2 struct {
	mu       sync.Mutex
	life     context.Context
	cancel   context.CancelFunc
	closing  bool
	active   sync.WaitGroup
	gate     chan struct{}
	input    *nativeRemoteInput
	helper   *bridge.DailyWalletBridge2
	expected pilot.RecoverableRemotePoolConfig
	configID string
	once     sync.Once
	closeErr error
}

func (o *nativeDailyWalletOwner2) checkLocked(ctx context.Context) error {
	if o == nil || ctx == nil || o.life == nil || o.closing || o.input == nil {
		return errNativeRemoteUnavailable
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	if e := o.life.Err(); e != nil {
		return e
	}
	return o.input.check(ctx)
}
func (o *nativeDailyWalletOwner2) close() error {
	if o == nil {
		return nil
	}
	o.once.Do(func() {
		o.mu.Lock()
		o.closing = true
		if o.cancel != nil {
			o.cancel()
		}
		o.mu.Unlock()
		o.active.Wait()
		if o.helper != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			e := o.helper.Close(ctx)
			cancel()
			if e != nil {
				o.closeErr = errors.Join(errNativeRemoteCleanup, e)
				return
			}
		}
		if e := o.input.close(); e != nil {
			o.closeErr = errors.Join(errNativeRemoteCleanup, e)
		}
	})
	return o.closeErr
}
func (o *nativeDailyWalletOwner2) failure(cause error, retire bool) error {
	e := &nativeDailyWalletError2{cause: cause}
	if retire {
		if cleanup := o.close(); cleanup != nil {
			e.cause = errors.Join(cause, cleanup)
			e.retained = o
		}
	}
	return e
}

// This leaf reads exactly one independently selected PUBLIC config. Private
// RPC/bearer inputs are paths only and are opened by the original helper.
func selectNativeDailyWallet2(ctx context.Context, path string) (out *nativeDailyWalletOwner2, resultErr error) {
	if wire.TestPoW || ctx == nil || ctx.Err() != nil {
		return nil, errNativeRemoteUnavailable
	}
	life, cancel := context.WithCancel(ctx)
	o := &nativeDailyWalletOwner2{life: life, cancel: cancel, gate: make(chan struct{}, 1)}
	defer func() {
		if resultErr != nil {
			resultErr = o.failure(resultErr, true)
			if o.closeErr != nil {
				out = o
			}
		}
	}()
	var e error
	o.input, e = selectNativeRemoteInput(life, path, pilot.RecoverableRemotePoolMaxConfigBytes)
	if e != nil {
		return nil, e
	}
	raw, e := o.input.read(life)
	if e != nil {
		return nil, e
	}
	o.expected, e = pilot.ParseRecoverableRemotePoolConfig(raw)
	clear(raw)
	if e != nil {
		return nil, e
	}
	o.configID, e = pilot.RecoverableRemotePoolConfigID(o.expected)
	if e != nil {
		return nil, e
	}
	if e = o.checkLocked(life); e != nil {
		return nil, e
	}
	return o, nil
}
func openNativeDailyWallet2(ctx context.Context, helper string, selection bridge.RemotePoolLaunchSelection) (out *nativeDailyWalletOwner2, resultErr error) {
	for _, path := range []string{helper, selection.ExistingRoot, selection.AttachmentRoot, selection.ConfigPath, selection.NodeRPCPath, selection.PoolCredentialPath, selection.AdmissionCredentialPath} {
		if !nativeRemotePath(path) {
			return nil, errNativeRemoteInvalid
		}
	}
	o, e := selectNativeDailyWallet2(ctx, selection.ConfigPath)
	if e != nil {
		return o, e
	}
	defer func() {
		if resultErr != nil {
			resultErr = o.failure(resultErr, true)
			if o.closeErr != nil {
				out = o
			}
		}
	}()
	work, cancel := context.WithTimeout(o.life, 2*time.Minute)
	defer cancel()
	o.helper, e = bridge.LaunchDailyWallet2(work, helper, selection, o.expected)
	if e != nil {
		return nil, e
	}
	if e = o.checkLocked(work); e != nil {
		return nil, e
	}
	return o, nil
}

// Only the concrete owner and opaque original context authorize dispatch.
// View IDs are comparison data; they can never manufacture bridge authority.
func (o *nativeDailyWalletOwner2) capture(view string, requireView bool) (*bridge.DailyWalletContext2, error) {
	if o == nil {
		return nil, errNativeRemoteUnavailable
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if e := o.checkLocked(o.life); e != nil {
		return nil, e
	}
	if o.helper == nil {
		return nil, errNativeRemoteUnavailable
	}
	original, e := o.helper.Context()
	if requireView {
		if e != nil || original == nil || original.View().ViewID != view {
			return nil, errNativeRemoteStale
		}
		return original, nil
	}
	// Snapshot may be signed out. The bridge independently checks nil versus
	// its original live view, including a concurrent expiry or retirement.
	return original, nil
}
func (o *nativeDailyWalletOwner2) call(command, arg string, original *bridge.DailyWalletContext2, sequence, capture, before uint64, after, limit uint32, txid string, index uint32, batch uint64) (out pilot.DailyWalletResponse2, resultErr error) {
	if o == nil {
		return out, &nativeDailyWalletError2{cause: errNativeRemoteUnavailable}
	}
	select {
	case o.gate <- struct{}{}:
		defer func() { <-o.gate }()
	default:
		return out, &nativeDailyWalletError2{cause: bridge.ErrBusy}
	}
	o.mu.Lock()
	e := o.checkLocked(o.life)
	if e == nil && o.helper == nil {
		e = errNativeRemoteUnavailable
	}
	if e != nil {
		o.mu.Unlock()
		return out, o.failure(e, true)
	}
	if original != nil && !o.helper.ContextMatches(original) {
		o.mu.Unlock()
		return out, o.failure(errNativeRemoteUnavailable, false)
	}
	o.active.Add(1)
	work, cancel := context.WithTimeout(o.life, 2*time.Minute)
	o.mu.Unlock()
	// Gate remains owned through the final disclosure fence. Close cancels
	// work then waits for active.Done before joining helper and selected file.
	defer func() {
		workErr := work.Err()
		cancel()
		o.mu.Lock()
		check := o.checkLocked(o.life)
		if check == nil {
			check = workErr
		}
		if resultErr == nil && check == nil {
			if out.ConfigID != o.configID {
				check = errNativeRemoteIntegrity
			} else if out.Snapshot == nil {
				if original == nil || !o.helper.ContextMatches(original) || out.ViewID != original.View().ViewID {
					check = errNativeRemoteStale
				}
			} else if out.Snapshot.View != nil {
				current, ce := o.helper.Context()
				if ce != nil || !o.helper.ContextMatches(current) || current.View() != *out.Snapshot.View || command != "login_fixture" && current != original {
					check = errNativeRemoteStale
				}
			} else if command != "snapshot" && command != "logout" {
				check = errNativeRemoteUnavailable
			}
		}
		o.mu.Unlock()
		o.active.Done()
		if check != nil {
			out = pilot.DailyWalletResponse2{}
			if command != "snapshot" {
				check = errors.Join(bridge.ErrRemotePoolOutcomeUnknown, check)
			}
			resultErr = o.failure(errors.Join(resultErr, check), true)
		} else if resultErr != nil {
			out = pilot.DailyWalletResponse2{}
			resultErr = o.failure(resultErr, false)
		}
	}()
	switch command {
	case "snapshot":
		return o.helper.Snapshot(work, original)
	case "login_fixture":
		return o.helper.LoginFixture(work, arg)
	case "start":
		return o.helper.Start(work, original)
	case "stop":
		return o.helper.Stop(work, original)
	case "logout":
		return o.helper.Logout(work, original)
	case "daily_payout_begin":
		return o.helper.BeginDailyPayoutRead(work, original, sequence, capture, before)
	case "daily_payout_read":
		return o.helper.DailyPayoutRead(work, original, sequence)
	case "daily_payout_cancel":
		return o.helper.CancelDailyPayoutRead(work, original, sequence)
	case "wallet_activity_begin":
		return o.helper.BeginWalletActivityRead(work, original, sequence)
	case "wallet_activity_read":
		return o.helper.WalletActivityRead(work, original, sequence)
	case "wallet_activity_cancel":
		return o.helper.CancelWalletActivityRead(work, original, sequence)
	case "wallet_activity_page":
		return o.helper.WalletActivityPage(work, original, capture, after, limit)
	case "wallet_activity_detail":
		return o.helper.WalletActivityDetail(work, original, capture, txid)
	case "daily_payout_observe":
		return o.helper.ObserveDailyPayout(work, original, capture, batch, txid, index)
	default:
		return out, errNativeRemoteInvalid
	}
}

// Exactly these fourteen public methods are registered in this selected webview.
// No legacy Gym service, path chooser, arbitrary dispatcher or restart exists.
type DailyRemoteGym2 struct{ owner *nativeDailyWalletOwner2 }

func (g *DailyRemoteGym2) Snapshot() (pilot.DailyWalletResponse2, error) {
	if g == nil || g.owner == nil {
		return pilot.DailyWalletResponse2{}, &nativeDailyWalletError2{cause: errNativeRemoteUnavailable}
	}
	v, e := g.owner.capture("", false)
	if e != nil {
		return pilot.DailyWalletResponse2{}, g.owner.failure(e, true)
	}
	return g.owner.call("snapshot", "", v, 0, 0, 0, 0, 0, "", 0, 0)
}
func (g *DailyRemoteGym2) LoginFixture(fixture string) (pilot.DailyWalletResponse2, error) {
	if g == nil || g.owner == nil {
		return pilot.DailyWalletResponse2{}, &nativeDailyWalletError2{cause: errNativeRemoteUnavailable}
	}
	if fixture != g.owner.expected.Base.ExpectedLocal.Recipient.Config.Fixture {
		return pilot.DailyWalletResponse2{}, g.owner.failure(errNativeRemoteInvalid, false)
	}
	return g.owner.call("login_fixture", fixture, nil, 0, 0, 0, 0, 0, "", 0, 0)
}
func (g *DailyRemoteGym2) dispatch(command, view string) (pilot.DailyWalletResponse2, error) {
	if g == nil || g.owner == nil {
		return pilot.DailyWalletResponse2{}, &nativeDailyWalletError2{cause: errNativeRemoteUnavailable}
	}
	v, e := g.owner.capture(view, true)
	if e != nil {
		return pilot.DailyWalletResponse2{}, g.owner.failure(e, !errors.Is(e, errNativeRemoteStale))
	}
	return g.owner.call(command, "", v, 0, 0, 0, 0, 0, "", 0, 0)
}
func (g *DailyRemoteGym2) Start(view string) (pilot.DailyWalletResponse2, error) {
	return g.dispatch("start", view)
}
func (g *DailyRemoteGym2) Stop(view string) (pilot.DailyWalletResponse2, error) {
	return g.dispatch("stop", view)
}
func (g *DailyRemoteGym2) Logout(view string) (pilot.DailyWalletResponse2, error) {
	return g.dispatch("logout", view)
}

func (g *DailyRemoteGym2) menuAction(command string) func() error {
	if g == nil || g.owner == nil {
		return func() error { return errNativeRemoteUnavailable }
	}
	o := g.owner
	v, e := o.capture("", false)
	if e != nil {
		return func() error { return o.failure(e, true) }
	}
	if v == nil || (command != "start" && command != "stop") {
		return func() error { return errNativeRemoteUnavailable }
	}
	return func() error { _, e := o.call(command, "", v, 0, 0, 0, 0, 0, "", 0, 0); return e }
}

func (g *DailyRemoteGym2) payout(command, view, sequence, capture, before string) (pilot.DailyWalletResponse2, error) {
	seq, e := dailyWalletUint(sequence, false)
	if e != nil {
		return pilot.DailyWalletResponse2{}, &nativeDailyWalletError2{cause: e}
	}
	cap, e := dailyWalletUint(capture, true)
	if e != nil {
		return pilot.DailyWalletResponse2{}, &nativeDailyWalletError2{cause: e}
	}
	cur, e := dailyWalletUint(before, true)
	if e != nil {
		return pilot.DailyWalletResponse2{}, &nativeDailyWalletError2{cause: e}
	}
	if g == nil || g.owner == nil {
		return pilot.DailyWalletResponse2{}, &nativeDailyWalletError2{cause: errNativeRemoteUnavailable}
	}
	v, e := g.owner.capture(view, true)
	if e != nil {
		return pilot.DailyWalletResponse2{}, g.owner.failure(e, !errors.Is(e, errNativeRemoteStale))
	}
	return g.owner.call(command, "", v, seq, cap, cur, 0, 0, "", 0, 0)
}
func (g *DailyRemoteGym2) BeginDailyPayoutRead(view, sequence, capture, before string) (pilot.DailyWalletResponse2, error) {
	return g.payout("daily_payout_begin", view, sequence, capture, before)
}
func (g *DailyRemoteGym2) DailyPayoutRead(view, sequence string) (pilot.DailyWalletResponse2, error) {
	return g.payout("daily_payout_read", view, sequence, "0", "0")
}
func (g *DailyRemoteGym2) CancelDailyPayoutRead(view, sequence string) (pilot.DailyWalletResponse2, error) {
	return g.payout("daily_payout_cancel", view, sequence, "0", "0")
}

func (g *DailyRemoteGym2) ownRead(command, view, sequence, capture string, after, limit uint32, txid string, index uint32, batch string) (pilot.DailyWalletResponse2, error) {
	seq, e := dailyWalletUint(sequence, true)
	if e != nil {
		return pilot.DailyWalletResponse2{}, &nativeDailyWalletError2{cause: e}
	}
	cap, e := dailyWalletUint(capture, true)
	if e != nil {
		return pilot.DailyWalletResponse2{}, &nativeDailyWalletError2{cause: e}
	}
	bat, e := dailyWalletUint(batch, true)
	if e != nil {
		return pilot.DailyWalletResponse2{}, &nativeDailyWalletError2{cause: e}
	}
	if g == nil || g.owner == nil {
		return pilot.DailyWalletResponse2{}, &nativeDailyWalletError2{cause: errNativeRemoteUnavailable}
	}
	v, e := g.owner.capture(view, true)
	if e != nil {
		return pilot.DailyWalletResponse2{}, g.owner.failure(e, !errors.Is(e, errNativeRemoteStale))
	}
	return g.owner.call(command, "", v, seq, cap, 0, after, limit, txid, index, bat)
}
func (g *DailyRemoteGym2) BeginWalletActivityRead(v, s string) (pilot.DailyWalletResponse2, error) {
	return g.ownRead("wallet_activity_begin", v, s, "0", 0, 0, "", 0, "0")
}
func (g *DailyRemoteGym2) WalletActivityRead(v, s string) (pilot.DailyWalletResponse2, error) {
	return g.ownRead("wallet_activity_read", v, s, "0", 0, 0, "", 0, "0")
}
func (g *DailyRemoteGym2) CancelWalletActivityRead(v, s string) (pilot.DailyWalletResponse2, error) {
	return g.ownRead("wallet_activity_cancel", v, s, "0", 0, 0, "", 0, "0")
}
func (g *DailyRemoteGym2) WalletActivityPage(v, c string, a, l uint32) (pilot.DailyWalletResponse2, error) {
	return g.ownRead("wallet_activity_page", v, "0", c, a, l, "", 0, "0")
}
func (g *DailyRemoteGym2) WalletActivityDetail(v, c, tx string) (pilot.DailyWalletResponse2, error) {
	return g.ownRead("wallet_activity_detail", v, "0", c, 0, 0, tx, 0, "0")
}
func (g *DailyRemoteGym2) ObserveDailyPayout(v, c, b, tx string, i uint32) (pilot.DailyWalletResponse2, error) {
	return g.ownRead("daily_payout_observe", v, "0", c, 0, 0, tx, i, b)
}
