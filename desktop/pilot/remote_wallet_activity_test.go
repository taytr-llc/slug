package pilot

import (
	"context"
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/wire/v2"
	"github.com/taytr-llc/slugwallet/wallet"
)

// Reuses independently frozen Set70 records and expected values. This tests
// the shared projection/new native cache, not a fabricated live attachment.
func TestDailyWalletOwnActivityIndependentProjectionAndPages(t *testing.T) {
	f, e := gymaccount.FixtureIdentity("zero256")
	if e != nil {
		t.Fatal(e)
	}
	identity := walletIdentity{RootPublicKey: f.RootPublicKey, GymPublicKey: f.GymPublicKey, GymID: f.GymID}
	tip, e := chainhash.NewHashFromStr(genesis)
	if e != nil {
		t.Fatal(e)
	}
	for _, row := range activityReferenceCorpus(t).Transactions.Positives {
		t.Run(row.Name, func(t *testing.T) {
			source := walletActivityRead{at: 1795121300000, snapshot: wallet.OwnActivitySnapshot{Version: 1, RootPublicKey: f.RootPublicKey, GymPublicKey: f.GymPublicKey, GymID: f.GymID, SyncedHeight: row.Height, SyncedHash: *tip, Records: []wallet.OwnActivityRecord{activityReferenceRecord(t, row.Record)}}}
			payload, meta, e := projectWalletActivityCapture(t.Context(), identity, source)
			if e != nil || !reflect.DeepEqual(payload.items, []WalletActivityItem{row.Expected.Item}) {
				t.Fatal("independent projection", e)
			}
			if payload.job != (WalletActivityJob{}) {
				t.Fatal("DAILY projection forged legacy job")
			}
			cache := &remoteWalletActivityCache{capture: RemoteWalletActivityCapture{Sequence: 7, Metadata: meta}, payload: payload}
			page, e := remoteActivityPage(cache, 7, 0, 8)
			if e != nil || page.Next != 1 || page.HasMore || !reflect.DeepEqual(page.Items, []WalletActivityItem{row.Expected.Item}) {
				t.Fatal("captured positions", e)
			}
			page.Items[0].TxID = strings.Repeat("f", 64)
			if page.Items[0].FeeAtoms != nil {
				*page.Items[0].FeeAtoms = -1
			}
			if !WalletActivityItemEqual(cache.payload.items[0], row.Expected.Item) {
				t.Fatal("caller aliases cache")
			}
			empty, e := remoteActivityPage(cache, 7, 1, 8)
			if e != nil || empty.Items == nil || len(empty.Items) != 0 || empty.Next != 1 || empty.HasMore {
				t.Fatal("exact captured end", e)
			}
			if _, e = remoteActivityPage(cache, 8, 0, 1); e != ErrActivityNotFound {
				t.Fatal("foreign capture", e)
			}
			for _, q := range [][2]uint32{{0, 0}, {0, 9}, {2, 1}, {math.MaxUint32, 8}} {
				if _, e = remoteActivityPage(cache, 7, q[0], q[1]); e == nil {
					t.Fatal("invalid cursor", q)
				}
			}
			got, outputs, e := projectWalletActivity(t.Context(), payload.records[0], meta, true)
			if e != nil || !WalletActivityItemEqual(got, row.Expected.Item) || !reflect.DeepEqual(outputs, row.Expected.Outputs) {
				t.Fatal("original raw detail", e)
			}
			source.snapshot.GymID = strings.Repeat("0", 64)
			if _, _, e = projectWalletActivityCapture(t.Context(), identity, source); e == nil {
				t.Fatal("foreign wallet")
			}
		})
	}
}
func TestDailyWalletOwnActivitySequenceAndJoinedCancellation(t *testing.T) {
	a := &RemotePoolAttachment{}
	if j, e := a.selectWalletActivityLocked(1); e != nil || j.Sequence != 0 {
		t.Fatal(e)
	}
	for _, seq := range []uint64{0, 2, math.MaxUint64} {
		if _, e := a.selectWalletActivityLocked(seq); e == nil {
			t.Fatal("unadmitted sequence", seq)
		}
	}
	a.ownActivitySequence = 4
	a.ownActivityLast = RemoteWalletActivityJob{Sequence: 4, State: "failed", Error: "busy"}
	if j, e := a.selectWalletActivityLocked(4); e != nil || j != a.ownActivityLast {
		t.Fatal("original unknown reply recovery", e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a.life = ctx
	work, stop := context.WithTimeout(ctx, 30*time.Second)
	op := &remoteWalletActivityOperation{job: RemoteWalletActivityJob{Sequence: 5, State: "working"}, ctx: work, cancel: stop, done: make(chan struct{})}
	prior := &remoteWalletActivityCache{capture: RemoteWalletActivityCapture{Sequence: 3}}
	a.ownActivity, a.ownActivityCache, a.ownActivitySequence = op, prior, 5
	op.previous = prior
	a.active.Add(1)
	if _, e := a.selectWalletActivityLocked(6); e != ErrRemotePoolBusy {
		t.Fatal(e)
	}
	entered := make(chan struct{})
	go func() {
		<-work.Done()
		close(entered)
		a.finishWalletActivityRead(op, &remoteWalletActivityCache{capture: RemoteWalletActivityCapture{Sequence: 5}}, nil)
	}()
	a.cancelWalletActivityRead()
	<-entered
	a.active.Wait()
	if a.ownActivity != nil || a.ownActivityCache != prior || a.ownActivityLast != (RemoteWalletActivityJob{Sequence: 5, State: "cancelled"}) {
		t.Fatal("cancel published/replaced dated data")
	}
	select {
	case <-op.done:
	default:
		t.Fatal("cancel returned without join")
	}
	a.ownActivitySequence = math.MaxUint64
	a.ownActivityLast = RemoteWalletActivityJob{Sequence: math.MaxUint64, State: "done"}
	if _, e := a.selectWalletActivityLocked(1); e == nil {
		t.Fatal("sequence wrapped")
	}
}
func TestDailyWalletSelectedPaymentDuplicateWindowsAndConflict(t *testing.T) {
	b, p := dailyReadPageFixture()
	payment := poolsettle.PayoutItem{Day: "1970-01-01", CreatedAtMS: 1, TxID: strings.Repeat("a", 64), OutputIndex: 2, Atoms: poolsettle.MinimumAtoms, RecordedState: "confirmed"}
	p.Items = []poolsettle.DailyPayoutItem{{BatchSequence: 18, Payment: payment}}
	c, e := freshDailyPayoutCapture(1, b, 2, p)
	if e != nil {
		t.Fatal(e)
	}
	got, e := selectedDailyPayment(c, 1, 18, payment.TxID, 2)
	if e != nil || got != payment {
		t.Fatal("identical first/current duplicate", e)
	}
	c.Stale = true
	if got, e = selectedDailyPayment(c, 1, 18, payment.TxID, 2); e != nil || got != payment {
		t.Fatal("dated stale membership", e)
	}
	c.Current.Items[0].Payment.Atoms++
	if _, e = selectedDailyPayment(c, 1, 18, payment.TxID, 2); e != ErrRemotePoolChanged {
		t.Fatal("conflicting duplicate", e)
	}
	for _, q := range []struct {
		seq, batch uint64
		txid       string
		index      uint32
	}{{2, 18, payment.TxID, 2}, {1, 17, payment.TxID, 2}, {1, 18, strings.Repeat("b", 64), 2}, {1, 18, payment.TxID, 3}} {
		if _, e = selectedDailyPayment(c, q.seq, q.batch, q.txid, q.index); e == nil {
			t.Fatal("unretained payment", q)
		}
	}
}
func TestDailyWalletOwnActivityModeAndCancellationBeforeOwners(t *testing.T) {
	a := &RemotePoolAttachment{config: RemotePoolConfig{Version: 2}, view: RemotePoolView{ViewID: "original"}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, e := a.BeginWalletActivityRead(ctx, "original", 1)
	if wire.TestPoW {
		if e != ErrRemotePoolProfile {
			t.Fatal(e)
		}
		return
	}
	if !errors.Is(e, context.Canceled) || a.ownActivity != nil || a.ownActivitySequence != 0 {
		t.Fatal("cancel admission", e)
	}
	a.config.Version = 1
	if _, e = a.BeginWalletActivityRead(context.Background(), "original", 1); e != ErrRemotePoolConfig {
		t.Fatal(e)
	}
	if _, e = a.ObserveDailyPayout(context.Background(), "original", 1, 1, strings.Repeat("a", 64), 0); e != ErrRemotePoolConfig {
		t.Fatal(e)
	}
}

// Concrete schema14 allocation owner and original account epoch. No node sync
// or READY is invented; native watched-wallet success remains actual acceptance.
func TestDailyWalletOriginalAllocatorActivityEpoch(t *testing.T) {
	h, s := ownedStartupWallet(t)
	ctx := context.Background()
	original := s.runtime.allocation2
	out, e := original.CaptureOwnActivity(ctx)
	if e != nil || out.GymID != s.runtime.identity.GymID || out.RootPublicKey != s.runtime.identity.RootPublicKey || out.GymPublicKey != s.runtime.identity.GymPublicKey {
		t.Fatal("original allocator read", e)
	}
	if s.runtime.allocation != nil || s.runtime.allocation2 != original {
		t.Fatal("allocator replaced")
	}
	h.revoked.Store(true)
	if _, e = original.CaptureOwnActivity(ctx); e == nil {
		t.Fatal("revoked original epoch read")
	}
	h.revoked.Store(false)
	if e = h.accounts.Logout(ctx, s.login.Token); e != nil {
		t.Fatal(e)
	}
	if _, e = original.CaptureOwnActivity(ctx); e == nil {
		t.Fatal("original logout read")
	}
	if e = s.Close(); e != nil {
		t.Fatal("original wallet join", e)
	}
}
