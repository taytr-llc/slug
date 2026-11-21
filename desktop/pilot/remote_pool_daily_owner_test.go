// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/internal/mininglease"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/prospectwork"
	"github.com/taytr-llc/slugwallet/wallet"
)

// Real schema14 allocation, account and encrypted fixture vault owners. No
// node/watch/READY is fabricated. The positive signature is only the original
// custody capability; the attachment must refuse this not-yet-watched wallet.
func TestDailyRemoteOriginalWalletRecipientAndMiningEpoch(t *testing.T) {
	h, s := ownedStartupWallet(t)
	ctx := context.Background()
	identity, e := gymaccount.FixtureIdentity("zero256")
	if e != nil {
		t.Fatal(e)
	}
	cfg := PoolRecipientConfig{Version: 1, AccountAuthorityID: h.accounts.AuthorityID(), AccountID: s.login.Account.ID, Fixture: "zero256", Identity: identity, Source: s.intent.Source}
	recipient, _, e := poolRecipientExpected(cfg)
	if e != nil {
		t.Fatal(e)
	}
	a := s.runtime.allocation2
	plan, e := a.PlanRecipient(ctx, s.intent.PoolAllocationID)
	if e != nil {
		t.Fatal(e)
	}
	reservation, e := a.Reserve(ctx, plan)
	if e != nil {
		t.Fatal(e)
	}
	receipt, e := a.Consume(ctx, reservation)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = a.Acknowledge(ctx, receipt); e != nil {
		t.Fatal(e)
	}
	binding, e := wallet.AllocationBindingID2(s.intent.Binding)
	if e != nil {
		t.Fatal(e)
	}
	expected := RemotePoolLocalBinding{Version: 1, ManagerSchema: 14, WalletOwnerID: s.intent.Pin.WalletOwnerID, AllocationBindingID: binding, OriginalSourceBindingID: s.intent.SourceBindingID, Recipient: recipient}
	before, e := os.ReadFile(filepath.Join(s.root, ownedWalletIntentName))
	if e != nil {
		t.Fatal(e)
	}
	if local, e := claimRemotePoolLocal(ctx, s, expected); e == nil || local != nil || s.remoteClaimed {
		t.Fatal("unwatched original wallet became an attachment")
	}
	if _, e = os.Stat(filepath.Join(s.root, ownedWalletReadyName)); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("refusal repaired READY", e)
	}
	after, e := os.ReadFile(filepath.Join(s.root, ownedWalletIntentName))
	if e != nil || string(before) != string(after) {
		t.Fatal("original wallet intent changed", e)
	}
	status, e := s.entry.inventory.Status(ctx, s.login.Token, wallet.HostedAllocationEpochFence(s.epoch))
	if e != nil || status.Allocations != 1 {
		t.Fatal("recipient allocation duplicated", e)
	}

	local := &remotePoolLocal{session: s, expected: expected}
	mining, e := h.accounts.MintMining(ctx, s.login.Token)
	if e != nil {
		t.Fatal(e)
	}
	lease, e := mininglease.Capture(ctx, remotePoolMiningSessions{local}, mining.Token)
	if e != nil || lease.PublicKey() != identity.GymPublicKey {
		t.Fatal("original separate Mining", e)
	}
	if _, e = mininglease.Capture(ctx, remotePoolMiningSessions{local}, s.login.Token); e == nil {
		t.Fatal("login substituted for Mining")
	}
	// Reuse the qualified public template as input, then build a fresh bounded
	// job for this actual local gym. Neither an issued job nor PoW is claimed.
	raw, e := os.ReadFile("../../testdata/public-daily/reference-v1.json")
	if e != nil {
		t.Fatal(e)
	}
	_ = remoteDailyOracle(t) // Pins the exact bytes before using the template.
	var corpus struct {
		Outbox struct {
			Scans []struct {
				Value struct {
					Work struct {
						Template prospectwork.Template `json:"template"`
					} `json:"work"`
				} `json:"value"`
			} `json:"scan_cases"`
		} `json:"outbox"`
	}
	if json.Unmarshal(raw, &corpus) != nil || len(corpus.Outbox.Scans) == 0 {
		t.Fatal("template input")
	}
	template := corpus.Outbox.Scans[0].Value.Work.Template
	template.Context.ProspectTarget = s.intent.Source.Policy.ProspectTarget
	job, e := prospectjob.NewManifest(template.Context, s.intent.Source.AuthorityPin.AuthorityID, s.intent.Source.AuthorityPin.DescriptorID, identity.GymPublicKey, strings.Repeat("42", 32), int64(template.Context.MinTimestamp)*1000)
	if e != nil {
		t.Fatal(e)
	}
	candidate, e := prospectwork.BuildManifest(job, template, s.intent.Source.PayoutAddress, 0)
	if e != nil {
		t.Fatal(e)
	}
	auth, e := h.vault.SignAdmission(ctx, mining.Token, job.Context, candidate.Proof, s.epoch)
	if e != nil || auth.PublicKey != identity.GymPublicKey || gymauth.VerifyAuthorization(job.Context, candidate.Proof, auth) != nil || lease.Check() != nil {
		t.Fatal("original retained vault signing", e)
	}
	remote := RemotePoolConfig{Version: 2, Source: s.intent.Source}
	remote.Source.Version = 2
	leaf := &RemotePoolAttachment{config: remote, local: local, life: ctx, ready: true, view: RemotePoolView{GymID: identity.GymID, GymPublicKey: identity.GymPublicKey}}
	if auth, e = leaf.signAdmission(ctx, job.Context, candidate.Proof); e == nil || auth.PublicKey != "" || auth.Signature != "" {
		t.Fatal("daily signer bypassed missing original READY", e)
	}
	h.revoked.Store(true)
	if auth, e = h.vault.SignAdmission(ctx, mining.Token, job.Context, candidate.Proof, s.epoch); e == nil || auth.Signature != "" {
		t.Fatal("revoked original epoch signed", e)
	}
	h.revoked.Store(false)
	if e = h.accounts.Logout(ctx, s.login.Token); e != nil {
		t.Fatal(e)
	}
	if lease.Check() == nil {
		t.Fatal("original Mining survived logout")
	}
	local.close()
	if s.closed.Load() {
		t.Fatal("borrowed local close owned the wallet")
	}
	if e = s.Close(); e != nil {
		t.Fatal("original wallet join", e)
	}
}
