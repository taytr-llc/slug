// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slugwallet/wallet"
)

func activityFixture(t *testing.T) (*PoolChild, *poolDesktopHarness, walletActivityRead, chan struct{}) {
	t.Helper()
	c, h, _ := payoutFixture(t, 0)
	// Real account callbacks serialize; the older CAS fixture does not model
	// concurrent polling and publication. Never mistake that for a nested lease.
	gate := make(chan struct{}, 1)
	lease := c.stable.stableConfig.Lease
	c.stable.stableConfig.Lease = func(ctx context.Context, cap gymaccount.Capability, fn func(gymaccount.Account, func() error) error) error {
		select {
		case gate <- struct{}{}:
		case <-ctx.Done():
			return ctx.Err()
		}
		defer func() { <-gate }()
		return lease(ctx, cap, fn)
	}
	source := walletActivityRead{at: 1795121300000}
	f := c.resources.Recipient.Config.Identity
	hash, _ := chainhash.NewHashFromStr(genesis)
	source.snapshot = wallet.OwnActivitySnapshot{Version: 1, RootPublicKey: f.RootPublicKey, GymPublicKey: f.GymPublicKey, GymID: f.GymID, SyncedHeight: 110, SyncedHash: *hash, Records: []wallet.OwnActivityRecord{}}
	corpus := activityReferenceCorpus(t)
	for _, row := range corpus.Transactions.Positives[:2] {
		source.snapshot.Records = append(source.snapshot.Records, activityReferenceRecord(t, row.Record))
	}
	sort.Slice(source.snapshot.Records, func(i, j int) bool {
		return source.snapshot.Records[i].TxID.String() < source.snapshot.Records[j].TxID.String()
	})
	c.activityRead = func(ctx context.Context) (walletActivityRead, error) {
		activityNoLease(t, ctx, gate)
		return cloneActivityRead(source), nil
	}
	return c, h, source, gate
}
func cloneActivityRead(x walletActivityRead) walletActivityRead {
	x.snapshot.Records = append([]wallet.OwnActivityRecord{}, x.snapshot.Records...)
	for i := range x.snapshot.Records {
		r := &x.snapshot.Records[i]
		r.Raw = append([]byte{}, r.Raw...)
		r.Credits = append([]wallet.OwnActivityCredit{}, r.Credits...)
		r.Debits = append([]wallet.OwnActivityDebit{}, r.Debits...)
		if r.Block != nil {
			b := *r.Block
			r.Block = &b
		}
	}
	return x
}
func activityNoLease(t *testing.T, ctx context.Context, gate chan struct{}) {
	t.Helper()
	probe, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	defer cancel()
	select {
	case gate <- struct{}{}:
		<-gate
	case <-probe.Done():
		t.Error("wallet/projection callback held account lease")
	}
}
func activityDone(t *testing.T, c *PoolChild, id string) WalletActivityJob {
	t.Helper()
	var out WalletActivityJob
	poolBackupWait(t, func() bool {
		j, e := c.WalletActivityJob(t.Context(), c.stable.view.ViewID, id)
		if e == nil {
			out = j
		}
		return e == nil && j.State != "running"
	})
	return out
}
func activityStart(t *testing.T, c *PoolChild, id string) WalletActivityJob {
	t.Helper()
	if _, e := c.BeginWalletActivity(t.Context(), c.stable.view.ViewID, id); e != nil {
		t.Fatal(e)
	}
	j := activityDone(t, c, id)
	if j.State != "done" {
		t.Fatal(j.State, j.Error)
	}
	return j
}
func TestWalletActivityCapturePagesDetailAndFailedRefresh(t *testing.T) {
	c, _, source, gate := activityFixture(t)
	v := c.stable.view.ViewID
	a, b, d := strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("e", 64)
	var reads atomic.Int32
	c.activityRead = func(ctx context.Context) (walletActivityRead, error) {
		activityNoLease(t, ctx, gate)
		reads.Add(1)
		return cloneActivityRead(source), nil
	}
	c.snapshot.State = "running"
	j := activityStart(t, c, a)
	if j.Capture.Count != 2 || c.snapshot.State != "running" {
		t.Fatal("capture altered mining or count")
	}
	c.snapshot.State = "stopped"
	if _, e := c.BeginWalletActivity(t.Context(), v, a); e != nil || reads.Load() != 1 {
		t.Fatal("same native action was repeated", e)
	}
	p, e := c.WalletActivityPage(t.Context(), v, a, 0, 1)
	if e != nil || p.Next != 1 || !p.HasMore {
		t.Fatal(e)
	}
	first := CloneWalletActivityItem(p.Items[0])
	detail, e := c.WalletActivityDetail(t.Context(), v, a, first.TxID)
	if e != nil || !WalletActivityItemEqual(first, detail.Item) || len(detail.Outputs) != int(first.OutputCount) {
		t.Fatal(e)
	}
	*p.Items[0].FeeAtoms = 123
	detail.Outputs[0].Atoms++
	p, e = c.WalletActivityPage(t.Context(), v, a, 0, 8)
	if e != nil || !WalletActivityItemEqual(first, p.Items[0]) || reads.Load() != 1 {
		t.Fatal("retained projection aliased caller or reread wallet", e)
	}
	if _, e = c.WalletActivityDetail(t.Context(), v, a, strings.Repeat("f", 64)); !errors.Is(e, ErrActivityNotFound) {
		t.Fatal("foreign transaction selection", e)
	}
	c.activityRead = func(context.Context) (walletActivityRead, error) {
		return walletActivityRead{}, wallet.ErrOwnActivityCapacity
	}
	if _, e = c.BeginWalletActivity(t.Context(), v, b); e != nil {
		t.Fatal(e)
	}
	failed := activityDone(t, c, b)
	if failed.State != "failed" || failed.Error != "capacity" {
		t.Fatal(failed)
	}
	if _, e = c.WalletActivityPage(t.Context(), v, a, 0, 8); e != nil {
		t.Fatal("failed refresh lost old capture", e)
	}
	c.activityRead = func(context.Context) (walletActivityRead, error) { return cloneActivityRead(source), nil }
	activityStart(t, c, d)
	if _, e = c.WalletActivityPage(t.Context(), v, a, 0, 8); !errors.Is(e, ErrActivityNotFound) {
		t.Fatal("old cursor adopted new capture", e)
	}
}
func TestWalletActivityOriginalViewAndFailedOuterAdmission(t *testing.T) {
	t.Run("outer-admission", func(t *testing.T) {
		c, _, _, _ := activityFixture(t)
		lease := c.stable.stableConfig.Lease
		c.stable.stableConfig.HistoryPublicationLease = func(ctx context.Context, fn func(gymaccount.Account, func() error) error) error {
			e := lease(ctx, gymaccount.View, fn)
			return errors.Join(e, gymaccount.ErrUnauthorized)
		}
		var reads atomic.Int32
		c.activityRead = func(context.Context) (walletActivityRead, error) { reads.Add(1); return walletActivityRead{}, nil }
		if _, e := c.BeginWalletActivity(t.Context(), c.stable.view.ViewID, strings.Repeat("a", 64)); e == nil {
			t.Fatal("failed outer fence admitted")
		}
		if reads.Load() != 0 || c.activityOp != nil || c.activityScratch != 0 {
			t.Fatal("failed outer fence started work or retained reservation")
		}
	})
	t.Run("reset", func(t *testing.T) {
		c, h, source, gate := activityFixture(t)
		entered, release := make(chan struct{}), make(chan struct{})
		var once sync.Once
		defer once.Do(func() { close(release) })
		c.activityRead = func(ctx context.Context) (walletActivityRead, error) {
			activityNoLease(t, ctx, gate)
			close(entered)
			<-release
			return cloneActivityRead(source), nil
		}
		id := strings.Repeat("b", 64)
		if _, e := c.BeginWalletActivity(t.Context(), c.stable.view.ViewID, id); e != nil {
			t.Fatal(e)
		}
		<-entered
		c.mu.Lock()
		op := c.activityOp
		c.mu.Unlock()
		h.mu.Lock()
		h.view = false
		h.token = strings.Repeat("2", 64)
		h.mu.Unlock()
		once.Do(func() { close(release) })
		<-op.done
		if c.activityCache != nil {
			t.Fatal("old-view capture published")
		}
		if _, e := c.WalletActivityPage(t.Context(), c.stable.view.ViewID, id, 0, 8); e == nil {
			t.Fatal("replacement token disclosed old view")
		}
	})
}
func TestWalletActivityFailedPublicationRestoresOldCapture(t *testing.T) {
	c, _, _, _ := activityFixture(t)
	a, b := strings.Repeat("a", 64), strings.Repeat("b", 64)
	activityStart(t, c, a)
	lease := c.stable.stableConfig.Lease
	var calls atomic.Int32
	c.stable.stableConfig.HistoryPublicationLease = func(ctx context.Context, fn func(gymaccount.Account, func() error) error) error {
		n := calls.Add(1)
		e := lease(ctx, gymaccount.View, fn)
		if n == 2 {
			return errors.Join(e, gymaccount.ErrUnauthorized)
		}
		return e
	}
	if _, e := c.BeginWalletActivity(t.Context(), c.stable.view.ViewID, b); e != nil {
		t.Fatal(e)
	}
	c.mu.Lock()
	op := c.activityOp
	c.mu.Unlock()
	if op != nil {
		<-op.done
	}
	c.stable.stableConfig.HistoryPublicationLease = nil
	if c.activityCache == nil || c.activityCache.job.JobID != a {
		t.Fatal("failed outer publication replaced old capture")
	}
}
func TestWalletActivityStopBackupCloseJoinCaptureAndProjection(t *testing.T) {
	for _, kind := range []string{"capture", "detail"} {
		for _, mode := range []string{"stop", "backup", "close"} {
			t.Run(kind+"/"+mode, func(t *testing.T) {
				c, _, source, gate := activityFixture(t)
				a := strings.Repeat("a", 64)
				v := c.stable.view.ViewID
				entered, release := make(chan struct{}), make(chan struct{})
				var once sync.Once
				defer once.Do(func() { close(release) })
				result := make(chan error, 1)
				if kind == "capture" {
					c.activityRead = func(ctx context.Context) (walletActivityRead, error) {
						activityNoLease(t, ctx, gate)
						close(entered)
						<-ctx.Done()
						<-release
						return cloneActivityRead(source), ctx.Err()
					}
					if _, e := c.BeginWalletActivity(t.Context(), v, a); e != nil {
						t.Fatal(e)
					}
				} else {
					activityStart(t, c, a)
					txid := source.snapshot.Records[0].TxID.String()
					c.activityProject = func(ctx context.Context, r wallet.OwnActivityRecord, m WalletActivityCapture, detail bool) (WalletActivityItem, []WalletActivityOutput, error) {
						activityNoLease(t, ctx, gate)
						close(entered)
						<-ctx.Done()
						<-release
						return projectWalletActivity(ctx, r, m, detail)
					}
					go func() { _, e := c.WalletActivityDetail(t.Context(), v, a, txid); result <- e }()
				}
				select {
				case <-entered:
				case <-time.After(3 * time.Second):
					t.Fatal("operation never entered")
				}
				if _, e := c.BeginWalletActivity(t.Context(), v, strings.Repeat("f", 64)); !errors.Is(e, gymstable.ErrBusy) {
					t.Fatal("overlap admitted", e)
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
				poolBackupWait(t, func() bool {
					c.mu.Lock()
					defer c.mu.Unlock()
					return c.activityOp != nil && c.activityOp.ctx.Err() != nil
				})
				select {
				case <-done:
					t.Fatal("lifetime released before local work joined")
				case <-time.After(20 * time.Millisecond):
				}
				activityNoLease(t, t.Context(), gate)
				c.mu.Lock()
				reserved := c.activityScratch
				c.mu.Unlock()
				if reserved != WalletActivityScratchBytes {
					t.Fatal("work reservation released early")
				}
				once.Do(func() { close(release) })
				select {
				case e := <-done:
					if e != nil {
						t.Fatal(e)
					}
				case <-time.After(3 * time.Second):
					t.Fatal("cleanup failed to join")
				}
				if kind == "detail" {
					if e := <-result; e == nil {
						t.Fatal("cancelled detail disclosed")
					}
				}
				c.mu.Lock()
				defer c.mu.Unlock()
				if c.activityOp != nil || c.activityProjection != nil || c.activityScratch != 0 {
					t.Fatal("activity owner leaked")
				}
			})
		}
	}
}
func TestWalletActivityCancelRetainsPriorAndDoesNotCancelDone(t *testing.T) {
	c, _, source, _ := activityFixture(t)
	a, b := strings.Repeat("a", 64), strings.Repeat("b", 64)
	activityStart(t, c, a)
	entered := make(chan struct{})
	c.activityRead = func(ctx context.Context) (walletActivityRead, error) {
		close(entered)
		<-ctx.Done()
		return cloneActivityRead(source), ctx.Err()
	}
	if _, e := c.BeginWalletActivity(t.Context(), c.stable.view.ViewID, b); e != nil {
		t.Fatal(e)
	}
	<-entered
	if _, e := c.CancelWalletActivity(t.Context(), c.stable.view.ViewID, b); e != nil {
		t.Fatal(e)
	}
	if j := activityDone(t, c, b); j.State != "cancelled" {
		t.Fatal(j)
	}
	j, e := c.CancelWalletActivity(t.Context(), c.stable.view.ViewID, a)
	if e != nil || j.State != "done" {
		t.Fatal("completed capture changed", e)
	}
}
