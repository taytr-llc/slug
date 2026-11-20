package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/taytr-llc/slug/desktop/internal/bridge"
	"github.com/taytr-llc/slug/desktop/pilot"
)

func TestHostedRecoveryNativeNoDispatchIsDefinitive(t *testing.T) {
	for _, action := range []*nativeHostedReceiveAction{nil, {state: "failed"}} {
		g := &Gym{}
		g.hostedReceive.action = action
		_, err := g.ReceiveAddressStatus()
		var refusal *bridge.HostedRecoveryFailure
		if !errors.As(err, &refusal) || refusal.Code != "allocation_not_found" {
			t.Fatal("no-dispatch outcome was uncertain", err)
		}
	}
	if !nativeHostedPending(&nativeHostedReceiveAction{state: "unknown"}) || nativeHostedPending(&nativeHostedReceiveAction{state: "needs_resume"}) {
		t.Fatal("durable resume confused with live uncertainty")
	}
	for _, code := range []string{"busy", "allocation_not_found", "allocation_resume_required"} {
		g := &Gym{}
		g.hostedReceive.action = &nativeHostedReceiveAction{state: "failed", action: "preselected-before-bridge-admission", refusal: code}
		_, err := g.ReceiveAddressStatus()
		if !errors.Is(err, bridge.ErrUnavailable) {
			t.Fatal("cached failure survived loss of original helper", err)
		}
	}
}

func TestHostedRecoveryNativeInactiveDoesNotAssertJoinedCleanup(t *testing.T) {
	for _, state := range []string{"signing_in", "signing_out", "resetting", "expired", "failed"} {
		t.Run(state, func(t *testing.T) {
			g, pending := nativeEvidenceFixture(t)
			s := pilot.BlankHostedRecoverySnapshot()
			s.Account.State = state
			observed, view := hostedChildObservation(s)
			if err := g.observeEvidenceAccount(pending.helper, observed, view); err != nil {
				t.Fatal(err)
			}
			if g.evidence.pending != pending || pending.selection.Pin().Version != 1 {
				t.Fatal("transitional hosted state released original selection")
			}
			s.Account.State = "signed_out"
			observed, view = hostedChildObservation(s)
			if err := g.observeEvidenceAccount(pending.helper, observed, view); err != nil || pending.selection.Pin().Version != 0 {
				t.Fatal("joined signed-out observation did not release selection", err)
			}
		})
	}
}

func TestHostedRecoveryNativeNamespaceExclusion(t *testing.T) {
	parent := candidateTestRoot(t)
	root := filepath.Join(parent, "LocalHostedRecoveryPilot")
	if err := prepareHostedRecoveryDataParent(root, parent); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatal("native preflight created root")
	}
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"SLUG-HOSTED-ALLOCATION-V1.json", "SLUG-HOSTED-POOL-INIT.json", "SLUG-HOSTED-POOL-READY.json"} {
		if err := os.WriteFile(filepath.Join(root, marker), []byte("backend validates exact marker"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := prepareHostedRecoveryDataParent(root, parent); err != nil {
		t.Fatal("own markers", err)
	}
	if err := prepareHostedPoolDataParent(root, parent); err == nil {
		t.Fatal("old hosted mode adopted new root")
	}
	if err := preparePoolDataParent(root, parent, false); err == nil {
		t.Fatal("ordinary pool adopted new root")
	}
	for _, old := range []string{"LocalHostedPoolPilot", "LocalPoolPilot", "LocalManifestPilot"} {
		if err := os.Mkdir(filepath.Join(parent, old), 0700); err != nil {
			t.Fatal(err)
		}
		if err := prepareHostedRecoveryDataParent(filepath.Join(parent, old), parent); err == nil {
			t.Fatal("new mode adopted old root", old)
		}
	}
	if err := prepareHostedRecoveryDataParent(filepath.Join(root, "nested"), parent); err == nil {
		t.Fatal("nested managed root accepted")
	}
}
