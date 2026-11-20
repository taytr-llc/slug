// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/desktop/pilot"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// PoolBridge uses the existing closed stable family, plus a separate pool
// snapshot/control family. A pending request selects the decoder before I/O.
type PoolBridge struct{ *StableBridge }

var ErrRecoveryOutcomeUnknown = errors.New("The recovery file may already be saved. Check the selected destination before trying again.")

func LaunchPool(helper, dataDir, node string) (*PoolBridge, error) {
	return LaunchPoolWithOptions(helper, dataDir, node, pilot.PoolOptions{})
}
func LaunchPoolWithOptions(helper, dataDir, node string, options pilot.PoolOptions) (*PoolBridge, error) {
	return launchPoolMode(helper, dataDir, node, options, pilot.PoolMode)
}

func LaunchHostedPoolWithOptions(helper, dataDir, node string, options pilot.PoolOptions) (*PoolBridge, error) {
	return launchPoolMode(helper, dataDir, node, options, pilot.HostedPoolMode)
}
func LaunchHostedRecoveryWithOptions(helper, dataDir, node string, options pilot.PoolOptions) (*PoolBridge, error) {
	return launchPoolMode(helper, dataDir, node, options, pilot.HostedRecoveryPoolMode)
}

func launchPoolMode(helper, dataDir, node string, options pilot.PoolOptions, mode string) (*PoolBridge, error) {
	options, e := pilot.ResolvePoolOptions(options)
	if e != nil || !filepath.IsAbs(dataDir) || filepath.Clean(dataDir) != dataDir || strings.ContainsAny(dataDir, "\x00\r\n\t") {
		return nil, ErrUnavailable
	}
	flag := "--candidate-pool"
	if mode == pilot.HostedRecoveryPoolMode {
		flag = "--candidate-hosted-recovery"
	} else if mode == pilot.HostedPoolMode {
		flag = "--candidate-hosted-pool"
	} else if mode != pilot.PoolMode {
		return nil, ErrUnavailable
	}
	args := []string{flag, "--data-dir", dataDir, "--node-binary", node}
	original, e := pilot.ResolvePoolOptions(pilot.PoolOptions{})
	if e != nil {
		return nil, ErrUnavailable
	}
	if options.CandidateDescriptorID != original.CandidateDescriptorID {
		args = append(args, "--candidate-name-version=2")
	}
	b, e := startStable(exec.Command(helper, args...))
	if e != nil {
		return nil, e
	}
	b.poolMode = true
	b.poolExpectedMode = mode
	b.poolExpectedDescriptor = options.CandidateDescriptorID
	b.entriesEnabled, b.eventsEnabled = true, true
	b.seedsEnabled, b.cardsEnabled, b.boutsEnabled, b.resultsEnabled, b.careersEnabled = true, true, true, true, true
	return &PoolBridge{b}, nil
}

func (b *StableBridge) selectedPoolMode() string {
	if b.poolExpectedMode == "" {
		return pilot.PoolMode // Legacy in-package process fixtures only.
	}
	return b.poolExpectedMode
}

// Mode is the launch selection, never a mode learned from helper responses.
func (b *PoolBridge) Mode() string {
	if b == nil || b.StableBridge == nil {
		return pilot.PoolMode
	}
	return b.selectedPoolMode()
}
func (b *PoolBridge) PoolRequest(ctx context.Context, req pilot.PoolRequest) (pilot.PoolResponse, error) {
	id, e := b.begin(ctx)
	if e != nil {
		return pilot.PoolResponse{}, e
	}
	defer func() { <-b.gate }()
	req.ID = id
	raw, e := json.Marshal(req)
	if e != nil {
		return pilot.PoolResponse{}, ErrUnavailable
	}
	if _, e = pilot.ParsePoolRequest(raw); e != nil {
		return pilot.PoolResponse{}, ErrUnavailable
	}
	backup := oneOf(req.Command, "export_recovery", "verify_backup", "cancel_pool_backup")
	if backup && (b.manifestView == nil || b.manifestView.ViewID != req.ViewID) {
		return pilot.PoolResponse{}, ErrUnavailable
	}
	if oneOf(req.Command, "login_fixture", "logout", "reset_fixture") {
		b.clearStableView()
		b.manifestView = nil
	}
	budget := 30 * time.Second
	if b.Mode() == pilot.HostedPoolMode && oneOf(req.Command, "login_fixture", "reset_fixture") {
		budget = 2 * time.Minute
	}
	b.mu.Lock()
	initial := req.Command == "snapshot" && !b.poolHandshakeUsed
	if initial {
		b.poolHandshakeUsed = true
		b.poolHandshakePending = true
		budget = 180 * time.Second
	}
	b.mu.Unlock()
	if initial {
		defer func() { b.mu.Lock(); b.poolHandshakePending = false; b.mu.Unlock() }()
	}
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	f, e := b.exchangeExpected(ctx, &stableExpected{id: id, pool: true}, raw)
	if e != nil || f.pool == nil {
		if initial {
			b.interruptStartup()
		}
		return pilot.PoolResponse{}, ErrUnavailable
	}
	b.mu.Lock()
	b.poolStartupComplete = true
	b.mu.Unlock()
	r := *f.pool
	if !r.OK {
		if req.Command == "export_recovery" && r.Error == "export_uncertain" {
			return pilot.PoolResponse{}, ErrRecoveryOutcomeUnknown
		}
		return pilot.PoolResponse{}, errors.New("The shared pool could not complete this request.")
	}
	correct := false
	switch req.Command {
	case "manifest_portrait":
		correct = r.Portrait != nil && r.Portrait.Sequence == req.Sequence && validManifestPortrait(r.Portrait) && b.manifestView != nil && r.Portrait.View == *b.manifestView
	case "manifest_receipt":
		correct = r.ManifestReceipt != nil && r.ManifestReceipt.Record.Sequence == req.Sequence && validManifestReceipt(r.ManifestReceipt) && b.manifestView != nil && r.ManifestReceipt.View == *b.manifestView
	default:
		correct = r.Snapshot != nil
	}
	if !correct {
		b.fail(ErrUnavailable)
		return pilot.PoolResponse{}, ErrUnavailable
	}
	if backup && (r.Snapshot == nil || r.Snapshot.View == nil || r.Snapshot.View.ViewID != req.ViewID || b.manifestView == nil || *r.Snapshot.View != *b.manifestView) {
		return pilot.PoolResponse{}, ErrUnavailable
	}
	if s := r.Snapshot; s != nil {
		if s.View != nil {
			if !b.matchesPoolSelection(s) {
				b.fail(ErrUnavailable)
				return pilot.PoolResponse{}, ErrUnavailable
			}
			b.poolSourceID = s.SourceBindingID
			p := s.View.AuthorityPin
			b.poolSourcePin = &p
		}
		if s.View == nil || b.view != nil && *b.view != *s.View {
			b.clearStableView()
		}
		b.manifestView = nil
		if s.View != nil {
			v := *s.View
			b.manifestView = &v
		}
		binding, bindingErr := pilot.PoolPayoutBindingForSnapshot(*s)
		activityBinding, activityErr := pilot.WalletActivityBindingForSnapshot(*s)
		b.mu.Lock()
		if activityErr == nil {
			if b.activityBinding == nil || *b.activityBinding != activityBinding {
				b.activityAttempt, b.activityCapture, b.activityRows = nil, nil, nil
			}
			b.activityBinding = &activityBinding
		} else {
			b.activityBinding, b.activityAttempt, b.activityCapture, b.activityRows = nil, nil, nil, nil
		}
		if bindingErr == nil {
			if b.withdrawalPool == nil || b.withdrawalPool.View == nil || *b.withdrawalPool.View != *s.View {
				b.withdrawalBinding, b.withdrawalExpected, b.withdrawalOperation = nil, nil, nil
			}
			owned := *s
			view := *s.View
			owned.View = &view
			b.withdrawalPool = &owned
			b.payoutBinding = &binding
		} else {
			b.payoutBinding = nil
			b.withdrawalPool, b.withdrawalBinding, b.withdrawalExpected, b.withdrawalOperation = nil, nil, nil, nil
		}
		b.mu.Unlock()
	}
	return r, nil
}

// Closing an input pipe cannot reach a constructor which has not started its
// reader yet. Interrupt only this owned helper's bounded startup context.
func (b *PoolBridge) interruptStartup() {
	if b.cmd != nil && b.cmd.Process != nil {
		_ = b.cmd.Process.Signal(os.Interrupt)
	}
}
func (b *PoolBridge) Close() error {
	b.mu.Lock()
	pending := !b.poolStartupComplete
	b.mu.Unlock()
	b.fail(ErrClosed)
	if pending {
		b.interruptStartup()
	}
	return b.StableBridge.Close()
}

// The first public source must match the launch selection; later responses must
// also retain the complete original source. Empty is the legacy v1 constructor
// expectation used by old in-package process fixtures, never disk discovery.
func (b *PoolBridge) matchesPoolSelection(s *pilot.PoolSnapshot) bool {
	options, e := pilot.ResolvePoolOptions(pilot.PoolOptions{CandidateDescriptorID: b.poolExpectedDescriptor})
	if e != nil || s == nil || s.View == nil || s.View.AuthorityPin.DescriptorID != options.CandidateDescriptorID {
		return false
	}
	return b.poolSourceID == "" || b.poolSourceID == s.SourceBindingID && b.poolSourcePin != nil && *b.poolSourcePin == s.View.AuthorityPin
}
