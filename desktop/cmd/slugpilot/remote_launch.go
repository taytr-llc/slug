// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"context"
	"errors"
	"io"
	"net/url"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/taytr-llc/slug/desktop/accountpilot"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/prospectnode"
	"github.com/taytr-llc/slug/wire/v2"
)

var (
	errRemoteLaunchInvalid     = errors.New("remote helper selection invalid")
	errRemoteLaunchUnavailable = errors.New("remote helper unavailable")
	errRemoteLaunchIntegrity   = errors.New("remote helper original input changed")
	errRemoteLaunchCleanup     = errors.New("remote helper cleanup incomplete")
	errRemoteLaunchUnsupported = errors.New("remote helper platform unsupported")
)

// Every error crossing main is static. The original concrete owner remains
// reachable when cleanup fails; neither paths nor RPC values enter Error().
type remoteLaunchError struct {
	cause    error
	retained *remoteLaunchOwner
}

func (e *remoteLaunchError) Error() string {
	if e != nil && e.retained != nil {
		return errRemoteLaunchCleanup.Error()
	}
	return errRemoteLaunchUnavailable.Error()
}
func (e *remoteLaunchError) Unwrap() error { return e.cause }

type remoteLaunchPaths struct {
	data, attachment, config, rpc, poolCredential, admissionCredential string
}

// This scan only selects private diagnostics. Validation below rejects unknown,
// duplicate and legacy options, including explicitly false legacy booleans.
func remoteLaunchRequested(args []string) bool {
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			continue
		}
		name := strings.SplitN(strings.TrimLeft(a, "-"), "=", 2)[0]
		if name == "daily-recoverable-issue" || name == "daily" || name == "candidate-remote-pool" || strings.HasPrefix(name, "remote-") {
			return true
		}
	}
	return false
}
func remoteLaunchPath(s string) bool {
	return len(s) > 1 && len(s) <= 4096 && filepath.IsAbs(s) && filepath.Clean(s) == s && !strings.ContainsAny(s, "\x00\r\n\t")
}
func validateRemoteLaunchArgs(args []string) error {
	if len(args) < 1 || len(args) > 19 {
		return errRemoteLaunchInvalid
	}
	seen := make(map[string]bool, 8)
	for i := 0; i < len(args); i++ {
		a := args[i]
		if len(a) > 4160 || !strings.HasPrefix(a, "-") {
			return errRemoteLaunchInvalid
		}
		nameValue := strings.SplitN(strings.TrimPrefix(strings.TrimPrefix(a, "-"), "-"), "=", 2)
		name := nameValue[0]
		if seen[name] {
			return errRemoteLaunchInvalid
		}
		seen[name] = true
		value := ""
		if name == "daily-recoverable-issue" || name == "candidate-remote-pool" || name == "daily" {
			if len(nameValue) == 2 && nameValue[1] != "true" {
				return errRemoteLaunchInvalid
			}
			continue
		}
		switch name {
		case "data-dir", "remote-attachment-root", "remote-config", "remote-node-rpc", "remote-pool-credential", "remote-admission-credential", "candidate-name-version":
		default:
			return errRemoteLaunchInvalid
		}
		if len(nameValue) == 2 {
			value = nameValue[1]
		} else {
			i++
			if i >= len(args) {
				return errRemoteLaunchInvalid
			}
			value = args[i]
		}
		if name == "candidate-name-version" {
			if value != "1" && value != "2" {
				return errRemoteLaunchInvalid
			}
		} else if !remoteLaunchPath(value) {
			return errRemoteLaunchInvalid
		}
	}
	if seen["daily-recoverable-issue"] {
		if seen["daily"] || seen["candidate-remote-pool"] {
			return errRemoteLaunchInvalid
		}
	} else if !seen["candidate-remote-pool"] {
		return errRemoteLaunchInvalid
	}
	for _, n := range []string{"data-dir", "remote-attachment-root", "remote-config", "remote-node-rpc", "remote-pool-credential", "remote-admission-credential"} {
		if !seen[n] {
			return errRemoteLaunchInvalid
		}
	}
	return nil
}

// Parse the existing strict RPC grammar, then enforce the attachment's existing
// literal-loopback profile. This never constructs a client or contacts a node.
func parseRemoteLaunchRPC(raw []byte) (prospectnode.RPCConfig, error) {
	c, e := prospectnode.ParseRPCConfig(raw)
	if e != nil || c.Version != 1 || len(c.URL) > 4096 || len(c.Username) > 4096 || len(c.Password) > 4096 || c.CACertificate != "" {
		return prospectnode.RPCConfig{}, errRemoteLaunchInvalid
	}
	u, e := url.Parse(c.URL)
	if e != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.User != nil || u.Port() == "" || u.Path != "" || u.RawPath != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawFragment != "" || u.Opaque != "" || u.String() != c.URL || strings.ContainsAny(c.URL, "\x00\r\n\t") {
		return prospectnode.RPCConfig{}, errRemoteLaunchInvalid
	}
	p, e := strconv.Atoi(u.Port())
	if e != nil || p < 1 || p > 65535 || strconv.Itoa(p) != u.Port() {
		return prospectnode.RPCConfig{}, errRemoteLaunchInvalid
	}
	return c, nil
}

// No token getter or generic constructor callback: this owner retains only two
// selected files and the original concrete controller. Credential file ownership
// belongs to the existing attachment credential owner, selected by path there.
type remoteLaunchOwner struct {
	mu         sync.Mutex
	life       context.Context
	cancel     context.CancelFunc
	closing    bool
	active     sync.WaitGroup
	files      [2]*remoteLaunchInput
	controller *accountpilot.RemotePoolController
	once       sync.Once
	closeErr   error
}

func (o *remoteLaunchOwner) checkLocked(ctx context.Context) error {
	if ctx == nil || o == nil || o.life == nil || o.closing {
		return errRemoteLaunchUnavailable
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	if e := o.life.Err(); e != nil {
		return e
	}
	for _, f := range o.files {
		if f == nil {
			return errRemoteLaunchUnavailable
		}
		if e := f.check(ctx); e != nil {
			return e
		}
	}
	return nil
}
func (o *remoteLaunchOwner) Close() error {
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
		if o.controller != nil {
			if e := o.controller.Close(); e != nil {
				o.closeErr = errors.Join(errRemoteLaunchCleanup, pilot.ErrRemotePoolCleanup, e)
				return // retain both file owners behind the unjoined controller
			}
		}
		for i := len(o.files) - 1; i >= 0; i-- {
			if e := o.files[i].close(); e != nil {
				o.closeErr = errors.Join(o.closeErr, errRemoteLaunchCleanup, pilot.ErrRemotePoolCleanup, e)
			}
		}
	})
	return o.closeErr
}

// Fixed-purpose dispatch is the only way parsed secrets reach the original
// constructor. Raw buffers are cleared immediately; immutable RPC strings are
// retained only by the concrete controller for its original node attachment.
func openRemoteLaunch(ctx context.Context, p remoteLaunchPaths, options pilot.PoolOptions) (*remoteLaunchOwner, pilot.RemotePoolConfig, error) {
	return openRemoteLaunchMode(ctx, p, options, false)
}
func openRemoteLaunchMode(ctx context.Context, p remoteLaunchPaths, options pilot.PoolOptions, daily bool) (*remoteLaunchOwner, pilot.RemotePoolConfig, error) {
	return openRemoteLaunchSelection(ctx, p, options, daily, false)
}
func openRemoteLaunchSelection(ctx context.Context, p remoteLaunchPaths, options pilot.PoolOptions, daily, recovery bool) (owner *remoteLaunchOwner, expected pilot.RemotePoolConfig, resultErr error) {
	if recovery && wire.TestPoW {
		return nil, expected, pilot.ErrRemotePoolProfile
	}
	if ctx == nil || ctx.Err() != nil {
		return nil, expected, errRemoteLaunchUnavailable
	}
	for _, path := range []string{p.data, p.attachment, p.config, p.rpc, p.poolCredential, p.admissionCredential} {
		if !remoteLaunchPath(path) {
			return nil, expected, errRemoteLaunchInvalid
		}
	}
	life, cancel := context.WithCancel(ctx)
	o := &remoteLaunchOwner{life: life, cancel: cancel}
	defer func() {
		if resultErr != nil {
			closeErr := o.Close()
			resultErr = &remoteLaunchError{cause: errors.Join(resultErr, closeErr)}
			if closeErr != nil {
				resultErr.(*remoteLaunchError).retained = o
				owner = o
			}
		}
	}()
	var e error
	limit := pilot.RemotePoolMaxConfigBytes
	if recovery {
		limit = pilot.RecoverableRemotePoolMaxConfigBytes
	}
	o.files[0], e = selectRemoteLaunchInput(ctx, p.config, limit)
	if e != nil {
		return nil, expected, e
	}
	raw, e := o.files[0].read(ctx)
	if e != nil {
		return nil, expected, e
	}
	parse := pilot.ParseRemotePoolConfig
	if daily {
		parse = pilot.ParseDailyRemotePoolConfig
	}
	var recoverable pilot.RecoverableRemotePoolConfig
	if recovery {
		recoverable, e = pilot.ParseRecoverableRemotePoolConfig(raw)
		expected = recoverable.Base
	} else {
		expected, e = parse(raw)
	} // exact grammar; no LF stripping
	clear(raw)
	if e != nil {
		return nil, pilot.RemotePoolConfig{}, e
	}
	o.files[1], e = selectRemoteLaunchInput(ctx, p.rpc, prospectnode.MaxRPCConfigSize)
	if e != nil {
		return nil, expected, e
	}
	raw, e = o.files[1].read(ctx)
	if e != nil {
		return nil, expected, e
	}
	rpc, e := parseRemoteLaunchRPC(raw)
	clear(raw)
	if e != nil {
		return nil, expected, e
	}
	if e = o.checkLocked(ctx); e != nil {
		return nil, expected, e
	}
	open := accountpilot.OpenRemotePoolController
	if daily {
		open = accountpilot.OpenDailyRemotePoolController
	}
	if recovery {
		o.controller, e = accountpilot.OpenRecoverableDailyRemotePoolController(ctx, p.data, options, pilot.RecoverableRemotePoolInputs{AttachmentRoot: p.attachment, Config: recoverable, NodeRPC: rpc, PoolCredentialPath: p.poolCredential, AdmissionCredentialPath: p.admissionCredential})
	} else {
		o.controller, e = open(ctx, p.data, options, pilot.RemotePoolInputs{AttachmentRoot: p.attachment, Config: expected, NodeRPC: rpc, PoolCredentialPath: p.poolCredential, AdmissionCredentialPath: p.admissionCredential})
	}
	if e != nil {
		return nil, expected, e
	}
	if e = o.checkLocked(ctx); e != nil {
		return nil, expected, e
	}
	return o, expected, nil
}

func (o *remoteLaunchOwner) call(ctx context.Context, command, arg string) (out pilot.RemotePoolWireSnapshot, resultErr error) {
	if o == nil {
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
	done := make(chan struct{})
	stop := context.AfterFunc(o.life, func() { defer close(done); cancel() })
	o.mu.Unlock()
	// Join the cancellation callback before releasing the active-call ownership.
	defer func() {
		if !stop() {
			<-done
		}
		cancel()
		o.mu.Lock()
		checkErr := o.checkLocked(ctx)
		o.mu.Unlock()
		o.active.Done()
		if checkErr != nil {
			out = pilot.RemotePoolWireSnapshot{}
			resultErr = errors.Join(resultErr, checkErr, o.Close())
		}
	}()
	switch command {
	case "snapshot":
		return o.controller.WireSnapshot(work)
	case "login_fixture":
		return out, o.controller.LoginFixture(work, arg)
	case "start":
		return out, o.controller.Start(work, arg)
	case "stop":
		return out, o.controller.Stop(work, arg)
	case "logout":
		return out, o.controller.Logout(work, arg)
	default:
		return out, errRemoteLaunchInvalid
	}
}
func (o *remoteLaunchOwner) WireSnapshot(ctx context.Context) (pilot.RemotePoolWireSnapshot, error) {
	return o.call(ctx, "snapshot", "")
}
func (o *remoteLaunchOwner) LoginFixture(ctx context.Context, s string) error {
	_, e := o.call(ctx, "login_fixture", s)
	return e
}
func (o *remoteLaunchOwner) Start(ctx context.Context, s string) error {
	_, e := o.call(ctx, "start", s)
	return e
}
func (o *remoteLaunchOwner) Stop(ctx context.Context, s string) error {
	_, e := o.call(ctx, "stop", s)
	return e
}
func (o *remoteLaunchOwner) Logout(ctx context.Context, s string) error {
	_, e := o.call(ctx, "logout", s)
	return e
}

func runRemoteLaunch(ctx context.Context, input io.ReadCloser, output io.WriteCloser, p remoteLaunchPaths, options pilot.PoolOptions) error {
	o, expected, e := openRemoteLaunch(ctx, p, options)
	if e != nil {
		return e
	}
	// Go otherwise exits on fd1/fd2 EPIPE before deferred resource joins. This
	// process-global setting is reached only by the explicit remote helper mode.
	signal.Ignore(syscall.SIGPIPE)
	e = pilot.ServeRemotePool(ctx, input, output, o, expected)
	closeErr := o.Close() // sticky join; never retries raw failed OS closes
	if e == nil && closeErr == nil {
		return nil
	}
	err := &remoteLaunchError{cause: errors.Join(e, closeErr)}
	if closeErr != nil {
		err.retained = o
	}
	return err
}
