// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"fmt"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/prospectservice"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type payoutReaderFunc func(context.Context) (poolservice.PayoutHistoryResponse, error)

func TestPoolPayoutTaskFailureCategories(t *testing.T) {
	for _, test := range []struct {
		err  error
		want string
	}{
		{gymstable.ErrCapacity, "capacity"},
		{gymstable.ErrBusy, "busy"},
		{&poolservice.ClientError{Code: "capacity"}, "capacity"},
		{&poolservice.ClientError{Code: "rate_limited"}, "busy"},
		{context.DeadlineExceeded, "payout_unavailable"},
	} {
		if got := payoutTaskError(test.err); got != test.want {
			t.Fatalf("task category %v: %q != %q", test.err, got, test.want)
		}
	}
}

func (f payoutReaderFunc) Payouts(c context.Context) (poolservice.PayoutHistoryResponse, error) {
	return f(c)
}
func payoutFixture(t *testing.T, n int) (*PoolChild, *poolDesktopHarness, poolservice.PayoutHistoryResponse) {
	t.Helper()
	c, _, h, _ := poolChildLifecycleFixture(t)
	c.stable.manifest.AuthorityPort = 19345
	trust, e := prospectservice.ManifestTransportTrustID(prospectservice.ManifestClientConfig{Version: 3, URL: c.stable.manifest.origin(), Token: strings.Repeat("0", 64), AuthorityPin: c.stable.manifest.AuthorityPin})
	if e != nil {
		t.Fatal(e)
	}
	c.stable.manifest.TransportTrustID = trust
	f, _ := gymaccount.FixtureIdentity("zero256")
	recipient, e := expectedPoolRecipient(walletIdentity{RootPublicKey: f.RootPublicKey, GymPublicKey: f.GymPublicKey, GymID: f.GymID})
	if e != nil {
		t.Fatal(e)
	}
	source := h.config.Source
	source.AuthorityPin = c.stable.manifest.AuthorityPin
	binding, e := poolservice.BindingID(source)
	if e != nil {
		t.Fatal(e)
	}
	c.resources.Recipient = PoolRecipientBinding{Version: 1, Config: PoolRecipientConfig{Version: 1, AccountAuthorityID: h.account.Account.AuthorityID, AccountID: h.account.Account.AccountID, Fixture: "zero256", Identity: f, Source: source}, SourceBindingID: binding, Recipient: poolPublicRecipient(recipient)}
	history := poolsettle.PayoutHistory{Version: 1, RecipientScript: recipient.Script, Balance: poolsettle.Balance{GymID: f.GymID, Confirmed: poolsettle.Fraction{Numerator: "0", Denominator: "1"}, Available: poolsettle.Fraction{Numerator: "0", Denominator: "1"}, PaidAtoms: int64(n) * poolsettle.MinimumAtoms}, Items: []poolsettle.PayoutItem{}}
	for i := n; i > 0; i-- {
		day := time.Date(2025, 1, i, 12, 0, 0, 0, time.UTC)
		history.Items = append(history.Items, poolsettle.PayoutItem{Day: day.Format("2006-01-02"), CreatedAtMS: day.UnixMilli(), TxID: fmt.Sprintf("%064x", i), OutputIndex: 1, Atoms: poolsettle.MinimumAtoms, RecordedState: "confirmed"})
	}
	if n > 0 {
		history.SettlementObservedAtMS = history.Items[0].CreatedAtMS + 1000
	}
	out := poolservice.PayoutHistoryResponse{Version: 1, BindingID: binding, History: history}
	if _, e := poolsettle.ValidatePayoutHistory(history, f.GymID); e != nil {
		t.Fatal(e)
	}
	return c, h, out
}
func payoutDone(t *testing.T, c *PoolChild, id string) PoolPayoutJob {
	t.Helper()
	var job PoolPayoutJob
	poolBackupWait(t, func() bool {
		var e error
		job, e = c.PoolPayoutJob(t.Context(), c.stable.view.ViewID, id)
		return e == nil && job.State != "running"
	})
	return job
}
func TestPoolPayoutCaptureOwnsPagesAndRetainsOldOnFailedRefresh(t *testing.T) {
	c, h, source := payoutFixture(t, 16)
	var calls atomic.Int32
	c.payoutReader = payoutReaderFunc(func(context.Context) (poolservice.PayoutHistoryResponse, error) {
		calls.Add(1)
		if h.sessions.active.Load() {
			t.Error("source read held account lease")
		}
		return source, nil
	})
	v := c.stable.view.ViewID
	a, b, d := strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("e", 64)
	c.snapshot.State = "running" // capture neither requires nor changes worker state.
	if _, e := c.BeginPoolPayoutHistory(t.Context(), v, a); e != nil {
		t.Fatal(e)
	}
	j := payoutDone(t, c, a)
	if j.State != "done" || j.Capture.Count != 16 {
		t.Fatal(j.State)
	}
	c.snapshot.State = "stopped"
	p, e := c.PoolPayoutPage(t.Context(), v, a, 0, 8)
	if e != nil || p.Next != 8 || !p.HasMore {
		t.Fatal(e)
	}
	first := p.Items[0]
	source.History.Items[0].Atoms++
	p.Items[0].Atoms++
	p, e = c.PoolPayoutPage(t.Context(), v, a, 0, 8)
	if e != nil || p.Items[0] != first || calls.Load() != 1 {
		t.Fatal("capture/page alias or repeated source read", e)
	}
	p, e = c.PoolPayoutPage(t.Context(), v, a, 8, 8)
	if e != nil || p.Next != 16 || p.HasMore {
		t.Fatal(e)
	}
	c.payoutReader = payoutReaderFunc(func(context.Context) (poolservice.PayoutHistoryResponse, error) {
		return poolservice.PayoutHistoryResponse{}, &poolservice.ClientError{Code: "busy"}
	})
	if _, e = c.BeginPoolPayoutHistory(t.Context(), v, b); e != nil {
		t.Fatal(e)
	}
	if j = payoutDone(t, c, b); j.State != "failed" || j.Error != "busy" {
		t.Fatal(j)
	}
	if _, e = c.PoolPayoutPage(t.Context(), v, a, 0, 8); e != nil {
		t.Fatal("failed refresh discarded capture", e)
	}
	snap, e := c.PoolPayoutSnapshot(t.Context())
	if e != nil || snap.CaptureJobID != a || snap.LastJobID != b {
		t.Fatal(e)
	}
	source.History.Items[0].Atoms--
	c.payoutReader = payoutReaderFunc(func(context.Context) (poolservice.PayoutHistoryResponse, error) { return source, nil })
	if _, e = c.BeginPoolPayoutHistory(t.Context(), v, d); e != nil {
		t.Fatal(e)
	}
	if j = payoutDone(t, c, d); j.State != "done" {
		t.Fatal(j)
	}
	if _, e = c.PoolPayoutPage(t.Context(), v, a, 0, 8); !errors.Is(e, ErrPayoutNotFound) {
		t.Fatal("old capture rebound", e)
	}
}
func TestPoolPayoutStopBackupCloseJoinIndependentSlot(t *testing.T) {
	for _, mode := range []string{"stop", "backup", "close"} {
		t.Run(mode, func(t *testing.T) {
			c, h, source := payoutFixture(t, 1)
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			defer once.Do(func() { close(release) })
			c.payoutReader = payoutReaderFunc(func(ctx context.Context) (poolservice.PayoutHistoryResponse, error) {
				if h.sessions.active.Load() {
					t.Error("lease across source")
				}
				close(entered)
				<-ctx.Done()
				<-release
				return source, ctx.Err()
			})
			id := strings.Repeat("e", 64)
			if _, e := c.BeginPoolPayoutHistory(t.Context(), c.stable.view.ViewID, id); e != nil {
				t.Fatal(e)
			}
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("read did not enter")
			}
			done := make(chan error, 1)
			go func() {
				switch mode {
				case "stop":
					c.Stop()
					done <- nil
				case "close":
					done <- c.Close()
				case "backup":
					op, e := c.beginPoolBackup(t.Context(), true)
					if e == nil {
						c.finishPoolBackup(op, "")
					}
					done <- e
				}
			}()
			poolBackupWait(t, func() bool { c.mu.Lock(); defer c.mu.Unlock(); return c.payoutOp != nil && c.payoutOp.ctx.Err() != nil })
			select {
			case <-done:
				t.Fatal("owner released before source joined")
			case <-time.After(20 * time.Millisecond):
			}
			c.mu.Lock()
			reserved := c.payoutScratch
			c.mu.Unlock()
			if reserved != PoolPayoutScratchBytes {
				t.Fatal("reservation released early")
			}
			if h.sessions.active.Load() {
				t.Fatal("join held original lease")
			}
			once.Do(func() { close(release) })
			select {
			case e := <-done:
				if e != nil {
					t.Fatal(e)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("join did not finish")
			}
			c.mu.Lock()
			defer c.mu.Unlock()
			if c.payoutOp != nil || c.payoutScratch != 0 {
				t.Fatal("slot leaked")
			}
		})
	}
}
func TestPoolPayoutWalletReadSelectionAndOriginalView(t *testing.T) {
	c, h, source := payoutFixture(t, 1)
	id := strings.Repeat("a", 64)
	v := c.stable.view.ViewID
	c.payoutReader = payoutReaderFunc(func(context.Context) (poolservice.PayoutHistoryResponse, error) { return source, nil })
	if _, e := c.BeginPoolPayoutHistory(t.Context(), v, id); e != nil {
		t.Fatal(e)
	}
	if payoutDone(t, c, id).State != "done" {
		t.Fatal("capture failed")
	}
	item := source.History.Items[0]
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	var calls atomic.Int32
	c.payoutObserve = func(ctx context.Context, selected walletPayoutSelection) (walletPayoutRead, error) {
		calls.Add(1)
		if h.sessions.active.Load() {
			t.Error("wallet read held account lease")
		}
		if selected.TxID != item.TxID || selected.OutputIndex != item.OutputIndex || selected.Atoms != item.Atoms || selected.RecipientScript != source.History.RecipientScript {
			t.Error("wrong selected payment")
		}
		close(entered)
		<-release
		return walletPayoutRead{State: "confirmed", ObservedAtMS: time.Now().UnixMilli(), WalletHeight: 10, WalletTip: strings.Repeat("f", 64), Confirmations: 2, BlockHash: strings.Repeat("e", 64)}, nil
	}
	if _, e := c.ReadPoolPayoutTransaction(t.Context(), v, id, strings.Repeat("9", 64), 0); !errors.Is(e, ErrPayoutNotFound) || calls.Load() != 0 {
		t.Fatal("arbitrary tx reached wallet", e)
	}
	result := make(chan PoolPayoutObservation, 1)
	failure := make(chan error, 1)
	go func() {
		o, e := c.ReadPoolPayoutTransaction(t.Context(), v, id, item.TxID, item.OutputIndex)
		result <- o
		failure <- e
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("wallet read did not enter")
	}
	if _, e := c.BeginPoolPayoutHistory(t.Context(), v, strings.Repeat("c", 64)); !errors.Is(e, gymstable.ErrBusy) {
		t.Fatal("replacement raced wallet", e)
	}
	if e := h.account.ViewLease(t.Context(), func(_ gymaccount.Account, check func() error) error { return check() }); e != nil {
		t.Fatal("account blocked by wallet", e)
	}
	h.mu.Lock()
	h.view = false
	h.mu.Unlock()
	once.Do(func() { close(release) })
	select {
	case e := <-failure:
		if e == nil {
			t.Fatal("revoked wallet result returned")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("wallet did not join")
	}
	if o := <-result; o.JobID != "" {
		t.Fatal("revoked payload escaped")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.payoutScratch != 0 {
		t.Fatal("wallet reservation leaked")
	}
}
