//go:build desktopintegration && !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slugwallet/wallet"
)

// Separate from the approved fresh/funded acceptance. The runner supplies only
// a byte-verified COPY of the failed public-fixture setup, never the original.
func TestHostedAllocationPartialConstructorReopen(t *testing.T) {
	root, binary := os.Getenv("SLUG_ALLOCATION_PARTIAL_COPY"), os.Getenv("SLUG_ALLOCATION_PARTIAL_NODE")
	if root == "" || binary == "" {
		t.Skip("explicit copied partial fixture and normal node required")
	}
	if !filepath.IsAbs(root) || !filepath.IsAbs(binary) || filepath.Base(filepath.Dir(root)) != "owned-copy" {
		t.Fatal("owned copy required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	init, found, err := readHostedInit(root)
	if err != nil || !found {
		t.Fatal("original INIT missing", err)
	}
	guard, found, err := readHostedAllocationGuard(root)
	if err != nil || !found || !reflect.DeepEqual(guard.HostedInit, init) {
		t.Fatal("original allocation guard missing", err)
	}
	if _, found, err := readHostedReady(root); err != nil || found {
		t.Fatal("fixture is not the intended incomplete constructor", err)
	}
	var intent struct {
		Version          uint32                   `json:"version"`
		Fixture          string                   `json:"fixture"`
		GuardSHA256      string                   `json:"guard_sha256"`
		PoolAllocationID string                   `json:"pool_allocation_id"`
		Binding          wallet.AllocationBinding `json:"binding"`
	}
	intentPath := filepath.Join(root, "allocation-recovery", "setup", "zero256.json")
	if found, err := readHostedControl(intentPath, 8192, &intent); err != nil || !found || intent.Version != 1 || intent.Fixture != "zero256" || intent.GuardSHA256 != hostedCanonicalHash(guard) {
		t.Fatal("original zero fixture intent missing", err)
	}
	zeroGym := filepath.Join(root, "shared", "gyms", intent.Binding.AccountID)
	var originalRecipient struct {
		Application string                     `json:"application"`
		Version     uint32                     `json:"version"`
		Phase       string                     `json:"phase"`
		Binding     pilot.PoolRecipientBinding `json:"binding"`
	}
	if found, err := readHostedControl(filepath.Join(zeroGym, "SLUG-POOL-RECIPIENT.json"), 8192, &originalRecipient); err != nil || !found || originalRecipient.Phase != "initializing" || pilot.ValidatePoolRecipientBinding(originalRecipient.Binding) != nil || originalRecipient.Binding.Config.AccountID != intent.Binding.AccountID || originalRecipient.Binding.SourceBindingID != intent.Binding.SourceBindingID {
		t.Fatal("original initializing recipient missing", err)
	}
	immutable := []string{hostedInitName, hostedAllocationGuardName, "shared/SLUG-POOL-RUNTIME.json", "shared/SLUG-POOL-SOURCE.json", "allocation-recovery/SLUG-HOSTED-ALLOCATION-INIT.json", "allocation-recovery/setup/zero256.json", filepath.Join("shared", "gyms", intent.Binding.AccountID, "SLUG-POOL-GYM.json"), filepath.Join("shared", "gyms", intent.Binding.AccountID, "SLUG-POOL-RECIPIENT.json")}
	before := make(map[string]string)
	for _, name := range immutable {
		raw, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		h := sha256.Sum256(raw)
		before[name] = hex.EncodeToString(h[:])
	}
	c, err := NewHostedRecoveryPool(ctx, pilot.Config{DataDir: root, NodeBinary: binary}, pilot.PoolOptions{})
	if err != nil {
		t.Fatal("copied partial constructor did not resume", err)
	}
	defer func() {
		if c != nil {
			if err := c.Close(); err != nil {
				t.Error(err)
			}
		}
	}()
	if !reflect.DeepEqual(c.hosted.init, init) || !reflect.DeepEqual(c.hosted.allocation.guard, guard) || c.sourceID != intent.Binding.SourceBindingID || !hostedReadyMatches(init, c.hosted.ready) || !c.hosted.allocation.setup.Ready() || !c.runtime.Ready() {
		t.Fatal("resumption replaced an original owner or failed to complete READY")
	}
	var resumedIntent struct {
		Version          uint32                   `json:"version"`
		Fixture          string                   `json:"fixture"`
		GuardSHA256      string                   `json:"guard_sha256"`
		PoolAllocationID string                   `json:"pool_allocation_id"`
		Binding          wallet.AllocationBinding `json:"binding"`
	}
	if found, err := readHostedControl(intentPath, 8192, &resumedIntent); err != nil || !found || !reflect.DeepEqual(resumedIntent, intent) {
		t.Fatal("original pool allocation operation changed", err)
	}
	node := poolActualNode(t, root)
	defer node.client.CloseIdleConnections()
	checkNoWork := func() {
		t.Helper()
		var height int64
		var mempool []string
		if err := node.call(ctx, "getblockcount", nil, &height); err != nil || height != 0 {
			t.Fatal("partial resume mined or advanced its original genesis chain", err)
		}
		if err := node.call(ctx, "getrawmempool", nil, &mempool); err != nil || len(mempool) != 0 {
			t.Fatal("partial resume published a transaction", err)
		}
	}
	checkNoWork()
	counts := make(map[string]uint32)
	for _, fixture := range hostedFixtures {
		s := hostedAllocationLogin(t, ctx, c, fixture, "healthy")
		pool := s.Healthy.Pool
		if pool.State != "stopped" || pool.Attempts != 0 || pool.AssignmentCount != 0 || pool.Height != 0 || pool.WalletHeight != 0 || pool.SpendableAtoms != 0 || pool.ImmatureAtoms != 0 || s.Healthy.PendingReceiveID != "" || s.Healthy.IssuedReceiveID != "" {
			t.Fatal("resumed child fabricated activity or address issuance")
		}
		if fixture == "zero256" && (s.Account.ID != intent.Binding.AccountID || pool.PayoutAddress != originalRecipient.Binding.Recipient.Address) {
			t.Fatal("zero account or index0 recipient changed")
		}
		if originalRecipient.Binding.Recipient.Purpose != 84 || originalRecipient.Binding.Recipient.Branch != 0 || originalRecipient.Binding.Recipient.Index != 0 {
			t.Fatal("partial input was not the original native84/index0 recipient")
		}
		c.mu.Lock()
		session, view, child := c.session, c.viewID, c.child
		c.mu.Unlock()
		epoch := wallet.HostedAllocationEpochFence(c.hostedViewFence(session, view, func() poolChild { return child }))
		inv, err := c.hosted.allocation.setup.Inventory(fixture)
		if err != nil {
			t.Fatal(err)
		}
		status, err := inv.Status(ctx, session.Token, epoch)
		if err != nil || status.Allocations != 1 || status.Revision != 2 || status.PendingReceiveID != "" || status.IssuedReceiveID != "" {
			t.Fatal("recipient resume duplicated or left unresolved an allocation", err)
		}
		counts[fixture] = status.Allocations
		if fixture == "zero256" && !reflect.DeepEqual(inv.Binding(), intent.Binding) {
			t.Fatal("original zero inventory binding changed")
		}
		head, err := inv.CaptureRecoveryHead(ctx, session.Token, epoch)
		if err != nil {
			t.Fatal(err)
		}
		for _, account := range head.Manifest().Accounts {
			want := uint32(0)
			if account.Purpose == 84 {
				want = 1
			}
			if account.NextExternal != want || account.NextInternal != 0 {
				t.Fatal("resume advanced beyond one recipient and zero change")
			}
		}
		gym := filepath.Join(root, "shared", "gyms", s.Account.ID)
		for _, name := range []string{"SLUG-POOL-GYM-READY.json", "SLUG-POOL-RECIPIENT-READY.json"} {
			info, err := os.Lstat(filepath.Join(gym, name))
			if err != nil || !info.Mode().IsRegular() {
				t.Fatal("fixture READY missing", err)
			}
		}
		poolActualLogout(t, ctx, c)
	}
	checkNoWork()
	if err = c.Close(); err != nil {
		t.Fatal(err)
	}
	c = nil
	if conn, err := net.DialTimeout("tcp", node.address, time.Second); err == nil {
		conn.Close()
		t.Fatal("owned source survived close")
	}
	for name, expected := range before {
		raw, err := os.ReadFile(filepath.Join(root, name))
		h := sha256.Sum256(raw)
		if err != nil || hex.EncodeToString(h[:]) != expected {
			t.Fatal("resumption rewrote immutable source input", name, err)
		}
	}
	result := struct {
		Version          uint32            `json:"version"`
		SourceBindingID  string            `json:"source_binding_id"`
		ZeroAccountID    string            `json:"zero_account_id"`
		PoolAllocationID string            `json:"zero_pool_allocation_id"`
		AllocationCounts map[string]uint32 `json:"allocation_counts"`
		BothReady        bool              `json:"both_ready"`
		OriginalPins     bool              `json:"original_pins_retained"`
		NoWork           bool              `json:"no_mining_or_payment"`
		Joined           bool              `json:"closed"`
	}{1, intent.Binding.SourceBindingID, intent.Binding.AccountID, intent.PoolAllocationID, counts, true, true, true, true}
	// No tokens, configuration, database bytes, private export or signed payload.
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(filepath.Dir(root), "partial-public-result.json")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.Write(raw)
	if err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
}
