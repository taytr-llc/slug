// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

// Package bridge owns the single bundled supervisor process and its bounded,
// serialized public JSON-lines protocol. It never reads private key material.
package bridge

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os/exec"
	"regexp"
	"sync"
	"time"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymaccount"
)

var (
	ErrUnavailable = errors.New("Local pilot is unavailable. Quit and reopen the app.")
	ErrBusy        = errors.New("The pilot is finishing another request. Try again shortly.")
	ErrClosed      = errors.New("The local pilot is shutting down.")
	publicHex      = regexp.MustCompile(`^[a-f0-9]{64}$`)
	publicKey      = regexp.MustCompile(`^(02|03)[a-f0-9]{64}$`)
	payoutAddress  = regexp.MustCompile(`^[1-9A-HJ-NP-Za-km-z]{26,60}$`)
)

// Bridge has one request in flight, no unbounded request queue, and one reader.
// Closing stdin asks the helper to drain its real worker and child nodes; Close
// waits for the process rather than killing work in the middle of a sync write.
type Bridge struct {
	cmd       *exec.Cmd
	input     io.WriteCloser
	responses chan pilot.Response
	failed    chan struct{}
	done      chan struct{}
	readDone  chan struct{}
	gate      chan struct{}
	once      sync.Once
	mu        sync.Mutex
	failure   error
	waitErr   error
	nextID    uint64
}

// Launch starts the supervisor, which is stopped until Start is requested.
func Launch(helper, dataDir, node string) (*Bridge, error) {
	return start(exec.Command(helper, "--data-dir", dataDir, "--node-binary", node))
}
func LaunchAccounts(helper, dataDir, node string) (*Bridge, error) {
	return start(exec.Command(helper, "--account-fixtures", "--data-dir", dataDir, "--node-binary", node))
}
func start(cmd *exec.Cmd) (*Bridge, error) {
	input, err := cmd.StdinPipe()
	if err != nil {
		return nil, ErrUnavailable
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		input.Close()
		return nil, ErrUnavailable
	}
	cmd.Stderr = io.Discard
	if err = cmd.Start(); err != nil {
		input.Close()
		output.Close()
		return nil, ErrUnavailable
	}
	b := &Bridge{cmd: cmd, input: input, responses: make(chan pilot.Response, 1), failed: make(chan struct{}), done: make(chan struct{}), readDone: make(chan struct{}), gate: make(chan struct{}, 1)}
	go b.read(output)
	go func() {
		err := cmd.Wait()
		b.mu.Lock()
		b.waitErr = err
		b.mu.Unlock()
		b.fail(ErrUnavailable)
		close(b.done)
	}()
	return b, nil
}
func (b *Bridge) fail(err error) {
	b.once.Do(func() {
		b.mu.Lock()
		b.failure = err
		b.mu.Unlock()
		close(b.failed)
		_ = b.input.Close()
	})
}
func (b *Bridge) read(r io.Reader) {
	defer close(b.readDone)
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4096), pilot.MaxResponseSize)
	for scanner.Scan() {
		response, err := decode(scanner.Bytes())
		if err != nil {
			b.fail(ErrUnavailable)
			return
		}
		select {
		case b.responses <- response:
		case <-b.failed:
			return
		default:
			b.fail(ErrUnavailable)
			return
		}
	}
	b.fail(ErrUnavailable)
}
func decode(raw []byte) (pilot.Response, error) {
	var response pilot.Response
	if len(raw) == 0 || len(raw) >= pilot.MaxResponseSize || json.Unmarshal(raw, &response) != nil {
		return response, ErrUnavailable
	}
	// The helper emits canonical json.Marshal output. Exact round-trip rejects
	// duplicate/unknown/case-alias keys, nulls and trailing data at every level.
	canonical, err := json.Marshal(response)
	if err != nil || !bytes.Equal(raw, canonical) || response.ID == 0 || response.ID > 1<<53-1 {
		return response, ErrUnavailable
	}
	if !response.OK {
		if response.Snapshot != nil || response.Receipt != nil {
			return response, ErrUnavailable
		}
		switch response.Error {
		case "invalid_request", "invalid_frame", "start_unavailable", "receipt_unavailable", "export_unavailable", "backup_unavailable", "account_unavailable":
		default:
			return response, ErrUnavailable
		}
	} else if response.Error != "" || (response.Snapshot == nil) == (response.Receipt == nil) {
		return response, ErrUnavailable
	}
	if s := response.Snapshot; s != nil {
		switch s.State {
		case "stopped", "starting", "running", "stopping", "failed":
		default:
			return response, ErrUnavailable
		}
		if (s.Mode != pilot.Mode && s.Mode != pilot.AccountMode) || s.Prospects == nil || len(s.Prospects) > 100 || s.ProspectCount < uint64(len(s.Prospects)) || s.HashesPerSecond < 0 {
			return response, ErrUnavailable
		}
		if (s.Mode == pilot.AccountMode) != (s.Account != nil) {
			return response, ErrUnavailable
		}
		if s.Account != nil && !validAccountProjection(s) {
			return response, ErrUnavailable
		}

		if !oneOf(s.WalletState, "closed", "connecting", "syncing", "ready", "failed") ||
			!oneOf(s.BackupState, "idle", "running", "succeeded", "failed", "cancelled") ||
			!oneOf(s.BackupPhase, "", "opening", "checking", "restoring", "syncing", "verifying", "complete") ||
			!oneOf(s.BackupError, "", "backup_unavailable", "backup_invalid", "backup_mismatch", "backup_storage_failed") ||
			s.SpendableAtoms < 0 || s.SpendableAtoms > 1<<53-1 || s.ImmatureAtoms < 0 || s.ImmatureAtoms > 1<<53-1 ||
			s.WalletHeight > 1<<53-1 || math.IsNaN(s.HashesPerSecond) || math.IsInf(s.HashesPerSecond, 0) ||
			(s.GymID != "" && !publicHex.MatchString(s.GymID)) ||
			(s.GymPublicKey != "" && !publicKey.MatchString(s.GymPublicKey)) ||
			(s.PayoutAddress != "" && !payoutAddress.MatchString(s.PayoutAddress)) {
			return response, ErrUnavailable
		}
	}
	return response, nil
}

// validAccountProjection rejects contradictory or mixed-fixture UI authority.
// Epoch milliseconds must also fit JavaScript Date, not merely safe integers.
func validAccountProjection(s *pilot.Snapshot) bool {
	a := s.Account
	if !oneOf(a.State, "signed_out", "signing_in", "signed_in", "resetting", "signing_out", "failed", "expired") ||
		!oneOf(a.Fixture, "", "zero256", "incrementing256") || !oneOf(a.LastError, "", "account_unavailable") ||
		a.Generation > 1<<53-1 || a.HoldUntilMS < 0 || a.HoldUntilMS > 8640000000000000 ||
		(a.Held && (a.CanExport || a.HoldUntilMS == 0)) {
		return false
	}
	if a.State != "signed_in" {
		return a.ID == "" && a.Generation == 0 && a.HoldUntilMS == 0 && !a.CanExport && !a.Held &&
			s.GymID == "" && s.GymPublicKey == "" && s.PayoutAddress == "" &&
			s.SpendableAtoms == 0 && s.ImmatureAtoms == 0 && s.ProspectCount == 0 && len(s.Prospects) == 0
	}
	identity, err := gymaccount.FixtureIdentity(a.Fixture)
	return err == nil && publicHex.MatchString(a.ID) && a.Generation > 0 &&
		s.GymID == identity.GymID && s.GymPublicKey == identity.GymPublicKey
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func (b *Bridge) Request(ctx context.Context, command string, sequence uint64) (pilot.Response, error) {
	switch command {
	case "snapshot", "start", "stop", "logout", "reset_fixture":
		if sequence != 0 {
			return pilot.Response{}, ErrUnavailable
		}
	case "receipt":
		if sequence == 0 || sequence > 1000 {
			return pilot.Response{}, ErrUnavailable
		}
	default:
		return pilot.Response{}, ErrUnavailable
	}
	return b.request(ctx, pilot.Request{Command: command, Sequence: sequence})
}

func (b *Bridge) RequestFixture(ctx context.Context, fixture string) (pilot.Response, error) {
	if fixture != "zero256" && fixture != "incrementing256" {
		return pilot.Response{}, ErrUnavailable
	}
	return b.request(ctx, pilot.Request{Command: "login_fixture", Fixture: fixture})
}

// RequestPath accepts only a native dialog's selection. This method is not a
// webview service and never receives or returns recovery-file contents.
func (b *Bridge) RequestPath(ctx context.Context, command, path string) (pilot.Response, error) {
	if (command != "export_recovery" && command != "verify_backup") || !pilot.ValidRecoveryPath(path) {
		return pilot.Response{}, ErrUnavailable
	}
	return b.request(ctx, pilot.Request{Command: command, Path: path})
}

func (b *Bridge) request(ctx context.Context, request pilot.Request) (pilot.Response, error) {
	command, sequence := request.Command, request.Sequence
	select {
	case b.gate <- struct{}{}:
		defer func() { <-b.gate }()
	default:
		return pilot.Response{}, ErrBusy
	}
	select {
	case <-b.failed:
		return pilot.Response{}, b.error()
	default:
	}
	if ctx.Err() != nil {
		return pilot.Response{}, ctx.Err()
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	b.nextID++
	if b.nextID > 1<<53-1 {
		b.fail(ErrUnavailable)
		return pilot.Response{}, ErrUnavailable
	}
	request.ID = b.nextID
	raw, err := json.Marshal(request)
	if err != nil || len(raw) > pilot.MaxRequestSize {
		return pilot.Response{}, ErrUnavailable
	}
	raw = append(raw, '\n')
	// The request is bounded and the one-at-a-time protocol never
	// fills the pipe. A dead/stalled helper is drained on operation timeout.
	if deadline, ok := b.input.(interface{ SetWriteDeadline(time.Time) error }); ok {
		_ = deadline.SetWriteDeadline(time.Now().Add(5 * time.Second))
	}
	if n, err := b.input.Write(raw); err != nil || n != len(raw) {
		b.fail(ErrUnavailable)
		return pilot.Response{}, ErrUnavailable
	}
	select {
	case response := <-b.responses:
		if response.ID != b.nextID || (response.OK && ((command == "receipt") != (response.Receipt != nil))) {
			b.fail(ErrUnavailable)
			return pilot.Response{}, ErrUnavailable
		}
		if response.Receipt != nil && (response.Receipt.Sequence != sequence || response.Receipt.Version != 4) {
			b.fail(ErrUnavailable)
			return pilot.Response{}, ErrUnavailable
		}
		if !response.OK {
			return pilot.Response{}, errors.New("The local pilot could not complete this request.")
		}
		return response, nil
	case <-b.failed:
		return pilot.Response{}, b.error()
	case <-ctx.Done():
		b.fail(ErrUnavailable)
		return pilot.Response{}, ErrUnavailable
	}
}
func (b *Bridge) error() error { b.mu.Lock(); defer b.mu.Unlock(); return b.failure }

// Close returns only after the helper and its owned nodes have stopped. A
// failed drain is surfaced, never reported as a clean shutdown.
func (b *Bridge) Close() error {
	b.fail(ErrClosed)
	<-b.done
	<-b.readDone
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.waitErr != nil {
		return ErrUnavailable
	}
	return nil
}
