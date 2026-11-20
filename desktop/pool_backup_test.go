// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/desktop/internal/bridge"
	"github.com/taytr-llc/slug/desktop/internal/recoveryfile"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/fighterdescriptor"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/prospectservice"
	"github.com/taytr-llc/slug/registry"
)

func poolNativeRecoverySnapshot(t *testing.T) pilot.PoolSnapshot {
	t.Helper()
	f, e := gymaccount.FixtureIdentity("zero256")
	if e != nil {
		t.Fatal(e)
	}
	descriptor, e := fighterdescriptor.ID(fighterdescriptor.CandidateDescriptor())
	if e != nil {
		t.Fatal(e)
	}
	pin := registry.ManifestAuthorityPin{Version: 5, Network: f.Network, Genesis: f.Genesis, AuthorityID: strings.Repeat("a", 64), DescriptorID: descriptor}
	origin := "http://127.0.0.1:39111"
	trust, e := prospectservice.ManifestTransportTrustID(prospectservice.ManifestClientConfig{Version: 3, URL: origin, Token: strings.Repeat("0", 64), AuthorityPin: pin})
	if e != nil {
		t.Fatal(e)
	}
	s := pilot.BlankPoolSnapshot()
	s.Account = pilot.AccountSnapshot{State: "signed_in", Fixture: "zero256", ID: strings.Repeat("b", 64), Generation: 1, CanExport: true}
	s.View = &pilot.ManifestViewPin{Version: 1, ViewID: strings.Repeat("c", 64), AccountID: s.Account.ID, AccountAuthorityID: strings.Repeat("d", 64), Generation: 1, AuthorityPin: pin, AuthorityOrigin: origin, TransportTrustID: trust}
	s.SourceBindingID = strings.Repeat("e", 64)
	s.GymID = f.GymID
	s.GymPublicKey = f.GymPublicKey
	s.PayoutAddress = "slug1qzmtrqsfuaf6l6kkcsseumq26ukaphfj9s9ptme"
	s.WalletState = "ready"
	s.BackupState = "idle"
	if e := pilot.ValidatePoolSnapshot(s); e != nil {
		t.Fatal(e)
	}
	return s
}
func TestPoolNativeBackupPinsOriginalDialogView(t *testing.T) {
	for _, export := range []bool{false, true} {
		t.Run(map[bool]string{false: "verify", true: "export"}[export], func(t *testing.T) {
			s := poolNativeRecoverySnapshot(t)
			original := s.View.ViewID
			path := filepath.Join(candidateTestRoot(t), "recovery.json")
			if !export {
				if e := os.WriteFile(path, []byte("PUBLIC-FIXTURE-PRIVATE-CONTENT-BOUNDARY\n"), 0600); e != nil {
					t.Fatal(e)
				}
			}
			calls := 0
			request := func(_ context.Context, r pilot.PoolRequest) (pilot.PoolResponse, error) {
				calls++
				if calls == 1 {
					if r.Command != "snapshot" {
						t.Fatal("dialog before account capture")
					}
					return pilot.PoolResponse{Snapshot: &s}, nil
				}
				if r.ViewID != original || r.Selection == nil || r.Path != "" {
					t.Fatal("dialog substituted current view/path")
				}
				raw, _ := json.Marshal(r)
				if strings.Contains(string(raw), "PUBLIC-FIXTURE-PRIVATE-CONTENT-BOUNDARY") {
					t.Fatal("recovery contents crossed native pipe")
				}
				h, e := recoveryfile.Open(*r.Selection, nil)
				if e != nil {
					t.Fatal("native selection was not usable by helper", e)
				}
				if e = h.Close(); e != nil {
					t.Fatal(e)
				}
				return pilot.PoolResponse{}, errors.New("original view was replaced")
			}
			choose := func() (string, error) {
				if calls != 1 {
					t.Fatal("snapshot not captured before dialog")
				}
				s.View.ViewID = strings.Repeat("f", 64)
				return path, nil
			}
			if _, e := poolRecoveryAction(context.Background(), request, choose, nil, export); e == nil {
				t.Fatal("switched account accepted")
			}
			if calls != 2 {
				t.Fatal("unexpected request count")
			}
		})
	}
}
func TestPoolNativeBackupCapabilityAndPrivateSelection(t *testing.T) {
	for _, export := range []bool{false, true} {
		t.Run(map[bool]string{false: "held-verify", true: "held-export"}[export], func(t *testing.T) {
			s := poolNativeRecoverySnapshot(t)
			s.Account.Held = true
			s.Account.HoldUntilMS = 1795000000000
			s.Account.CanExport = false
			path := filepath.Join(candidateTestRoot(t), "recovery.json")
			if e := os.WriteFile(path, []byte("fixture\n"), 0600); e != nil {
				t.Fatal(e)
			}
			chosen, calls := false, 0
			request := func(_ context.Context, r pilot.PoolRequest) (pilot.PoolResponse, error) {
				calls++
				if r.Command == "snapshot" {
					return pilot.PoolResponse{Snapshot: &s}, nil
				}
				if r.Command != "verify_backup" || r.Selection == nil || r.Selection.Mode != recoveryfile.ReadMode || r.Selection.File == nil {
					t.Fatal("wrong held operation")
				}
				return pilot.PoolResponse{Snapshot: &s}, nil
			}
			choose := func() (string, error) { chosen = true; return path, nil }
			result, e := poolRecoveryAction(context.Background(), request, choose, nil, export)
			if export {
				if e == nil || chosen || calls != 1 {
					t.Fatal("held export reached dialog")
				}
			} else if e != nil || !chosen || result != "Backup verification started." {
				t.Fatal("held verification failed", e)
			}
		})
	}
}
func TestPoolNativeBackupUncertainSaveAndProtectedRoot(t *testing.T) {
	s := poolNativeRecoverySnapshot(t)
	root := candidateTestRoot(t)
	path := filepath.Join(root, "recovery.json")
	requests := 0
	request := func(_ context.Context, r pilot.PoolRequest) (pilot.PoolResponse, error) {
		if r.Command == "snapshot" {
			return pilot.PoolResponse{Snapshot: &s}, nil
		}
		requests++
		return pilot.PoolResponse{}, recoveryfile.ErrOutcomeUnknown
	}
	choose := func() (string, error) { return path, nil }
	if result, e := poolRecoveryAction(context.Background(), request, choose, nil, true); result != "" || !errors.Is(e, bridge.ErrRecoveryOutcomeUnknown) {
		t.Fatal("uncertain save reported success", e)
	}
	if _, e := poolRecoveryAction(context.Background(), request, choose, []string{root}, true); e == nil || requests != 1 {
		t.Fatal("managed destination submitted")
	}
}
