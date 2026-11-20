// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/taytr-llc/slug/desktop/internal/bridge"
	"github.com/taytr-llc/slug/desktop/internal/poolselection"
	"github.com/taytr-llc/slug/desktop/pilot"
)

var (
	errNativeRemoteStale       = errors.New("The original remote view is no longer current.")
	errNativeRemoteInvalid     = errors.New("Choose the explicit remote pool mode and its six input paths.")
	errNativeRemoteUnavailable = errors.New("The original remote pool connection is unavailable.")
	errNativeRemoteIntegrity   = errors.New("The selected remote pool configuration changed. Close this connection.")
	errNativeRemoteCleanup     = errors.New("The original remote pool helper did not close cleanly; its owners are retained.")
	errNativeRemoteUnsupported = errors.New("Protected remote pool inputs are unsupported on this platform.")
)

// No retained path, RPC value, token or arbitrary cause is displayed by Error.
type nativeRemoteError struct {
	cause    error
	retained *nativeRemoteOwner
}

func (e *nativeRemoteError) Error() string {
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
func (e *nativeRemoteError) Unwrap() error { return e.cause }

func nativeRemoteRequested(args []string) bool {
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			continue
		}
		name := strings.SplitN(strings.TrimLeft(arg, "-"), "=", 2)[0]
		if name == "candidate-remote-pool" || strings.HasPrefix(name, "remote-") {
			return true
		}
	}
	return false
}
func nativeRemotePath(path string) bool {
	return len(path) > 1 && len(path) <= 4096 && filepath.IsAbs(path) && filepath.Clean(path) == path && !strings.ContainsAny(path, "\x00\r\n\t")
}

// Parse before any default-directory discovery, environment selection or I/O.
// Unknown legacy selectors are refused even when explicitly false.
func parseNativeRemoteArgs(args []string) (out bridge.RemotePoolLaunchSelection, resultErr error) {
	if len(args) < 1 || len(args) > 18 {
		return out, errNativeRemoteInvalid
	}
	values := make(map[string]string, 8)
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if len(arg) > 4160 || !strings.HasPrefix(arg, "-") {
			return out, errNativeRemoteInvalid
		}
		parts := strings.SplitN(strings.TrimPrefix(strings.TrimPrefix(arg, "-"), "-"), "=", 2)
		name := parts[0]
		if _, ok := values[name]; ok {
			return out, errNativeRemoteInvalid
		}
		if name == "candidate-remote-pool" {
			if len(parts) == 2 && parts[1] != "true" {
				return out, errNativeRemoteInvalid
			}
			values[name] = "true"
			continue
		}
		switch name {
		case "data-dir", "remote-attachment-root", "remote-config", "remote-node-rpc", "remote-pool-credential", "remote-admission-credential", "candidate-name-version":
		default:
			return out, errNativeRemoteInvalid
		}
		var value string
		if len(parts) == 2 {
			value = parts[1]
		} else {
			i++
			if i >= len(args) {
				return out, errNativeRemoteInvalid
			}
			value = args[i]
		}
		if name == "candidate-name-version" {
			if value != "1" && value != "2" {
				return out, errNativeRemoteInvalid
			}
		} else if !nativeRemotePath(value) {
			return out, errNativeRemoteInvalid
		}
		values[name] = value
	}
	for _, name := range []string{"candidate-remote-pool", "data-dir", "remote-attachment-root", "remote-config", "remote-node-rpc", "remote-pool-credential", "remote-admission-credential"} {
		if values[name] == "" {
			return out, errNativeRemoteInvalid
		}
	}
	version, explicit := values["candidate-name-version"]
	if !explicit {
		version = "1"
	}
	options, e := poolselection.Options(true, true, version, explicit, values["data-dir"], true)
	if e != nil {
		return out, errNativeRemoteInvalid
	}
	return bridge.RemotePoolLaunchSelection{ExistingRoot: values["data-dir"], AttachmentRoot: values["remote-attachment-root"], ConfigPath: values["remote-config"], NodeRPCPath: values["remote-node-rpc"], PoolCredentialPath: values["remote-pool-credential"], AdmissionCredentialPath: values["remote-admission-credential"], Options: options}, nil
}

type nativeRemoteOwner struct {
	mu       sync.Mutex
	life     context.Context
	cancel   context.CancelFunc
	closing  bool
	active   sync.WaitGroup
	gate     chan struct{}
	input    *nativeRemoteInput
	helper   *bridge.RemotePoolBridge
	expected pilot.RemotePoolConfig
	configID string
	once     sync.Once
	closeErr error
}

func (o *nativeRemoteOwner) checkLocked(ctx context.Context) error {
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
func (o *nativeRemoteOwner) close() error {
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
func (o *nativeRemoteOwner) failure(cause error, retire bool) error {
	e := &nativeRemoteError{cause: cause}
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
func selectNativeRemote(ctx context.Context, path string) (out *nativeRemoteOwner, resultErr error) {
	if ctx == nil || ctx.Err() != nil {
		return nil, errNativeRemoteUnavailable
	}
	life, cancel := context.WithCancel(ctx)
	o := &nativeRemoteOwner{life: life, cancel: cancel, gate: make(chan struct{}, 1)}
	defer func() {
		if resultErr != nil {
			resultErr = o.failure(resultErr, true)
			if o.closeErr != nil {
				out = o
			}
		}
	}()
	var e error
	o.input, e = selectNativeRemoteInput(life, path, pilot.RemotePoolMaxConfigBytes)
	if e != nil {
		return nil, e
	}
	raw, e := o.input.read(life)
	if e != nil {
		return nil, e
	}
	o.expected, e = pilot.ParseRemotePoolConfig(raw)
	clear(raw)
	if e != nil {
		return nil, e
	}
	o.configID, e = pilot.RemotePoolConfigID(o.expected)
	if e != nil {
		return nil, e
	}
	if e = o.checkLocked(life); e != nil {
		return nil, e
	}
	return o, nil
}
func openNativeRemote(ctx context.Context, helper string, selection bridge.RemotePoolLaunchSelection) (out *nativeRemoteOwner, resultErr error) {
	for _, path := range []string{helper, selection.ExistingRoot, selection.AttachmentRoot, selection.ConfigPath, selection.NodeRPCPath, selection.PoolCredentialPath, selection.AdmissionCredentialPath} {
		if !nativeRemotePath(path) {
			return nil, errNativeRemoteInvalid
		}
	}
	o, e := selectNativeRemote(ctx, selection.ConfigPath)
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
	o.helper, e = bridge.LaunchRemotePool(work, helper, selection, o.expected)
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
func (o *nativeRemoteOwner) capture(view string, requireView bool) (*bridge.RemotePoolContext, error) {
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
func (o *nativeRemoteOwner) call(command, arg string, original *bridge.RemotePoolContext) (out pilot.RemotePoolWireSnapshot, resultErr error) {
	if o == nil {
		return out, &nativeRemoteError{cause: errNativeRemoteUnavailable}
	}
	select {
	case o.gate <- struct{}{}:
		defer func() { <-o.gate }()
	default:
		return out, &nativeRemoteError{cause: bridge.ErrBusy}
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
			} else if out.View != nil {
				current, ce := o.helper.Context()
				if ce != nil || !o.helper.ContextMatches(current) || current.View() != *out.View || (command != "login_fixture" && current != original) {
					check = errNativeRemoteUnavailable
				}
			} else if command != "snapshot" && command != "logout" {
				check = errNativeRemoteUnavailable
			}
		}
		o.mu.Unlock()
		o.active.Done()
		if check != nil {
			out = pilot.RemotePoolWireSnapshot{}
			if command != "snapshot" {
				check = errors.Join(bridge.ErrRemotePoolOutcomeUnknown, check)
			}
			resultErr = o.failure(errors.Join(resultErr, check), true)
		} else if resultErr != nil {
			out = pilot.RemotePoolWireSnapshot{}
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
	default:
		return out, errNativeRemoteInvalid
	}
}

// Exactly these five public methods are registered in the remote webview.
// No legacy Gym service, path chooser, arbitrary dispatcher or restart exists.
type RemoteGym struct{ owner *nativeRemoteOwner }

func (g *RemoteGym) Snapshot() (pilot.RemotePoolWireSnapshot, error) {
	if g == nil || g.owner == nil {
		return pilot.RemotePoolWireSnapshot{}, &nativeRemoteError{cause: errNativeRemoteUnavailable}
	}
	v, e := g.owner.capture("", false)
	if e != nil {
		return pilot.RemotePoolWireSnapshot{}, g.owner.failure(e, true)
	}
	return g.owner.call("snapshot", "", v)
}
func (g *RemoteGym) LoginFixture(fixture string) (pilot.RemotePoolWireSnapshot, error) {
	if g == nil || g.owner == nil {
		return pilot.RemotePoolWireSnapshot{}, &nativeRemoteError{cause: errNativeRemoteUnavailable}
	}
	if fixture != g.owner.expected.ExpectedLocal.Recipient.Config.Fixture {
		return pilot.RemotePoolWireSnapshot{}, g.owner.failure(errNativeRemoteInvalid, false)
	}
	return g.owner.call("login_fixture", fixture, nil)
}
func (g *RemoteGym) dispatch(command, view string) (pilot.RemotePoolWireSnapshot, error) {
	if g == nil || g.owner == nil {
		return pilot.RemotePoolWireSnapshot{}, &nativeRemoteError{cause: errNativeRemoteUnavailable}
	}
	v, e := g.owner.capture(view, true)
	if e != nil {
		return pilot.RemotePoolWireSnapshot{}, g.owner.failure(e, !errors.Is(e, errNativeRemoteStale))
	}
	return g.owner.call(command, "", v)
}
func (g *RemoteGym) Start(view string) (pilot.RemotePoolWireSnapshot, error) {
	return g.dispatch("start", view)
}
func (g *RemoteGym) Stop(view string) (pilot.RemotePoolWireSnapshot, error) {
	return g.dispatch("stop", view)
}
func (g *RemoteGym) Logout(view string) (pilot.RemotePoolWireSnapshot, error) {
	return g.dispatch("logout", view)
}

func (g *RemoteGym) menuAction(command string) func() error {
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
	return func() error { _, e := o.call(command, "", v); return e }
}
