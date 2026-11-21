package pilot

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/wire/v2"
)

// Conditional shape/linkage cases, not source, ledger, View or wallet authority.
// Eighteen global batches produce windows [18..11], [10..3], [2..1]. None
// need belong to this gym: an empty first/second window must still continue.
func dailyReadPageFixture() (RemoteDailyPayoutBinding, poolsettle.DailyPayoutPage) {
	b := RemoteDailyPayoutBinding{View: RemotePoolView{GymID: strings.Repeat("1", 64)}, SourceBindingID: strings.Repeat("2", 64), SettlementConfigID: strings.Repeat("3", 64), RecipientScript: "0014" + strings.Repeat("4", 40)}
	p := poolsettle.DailyPayoutPage{Version: 2, BindingID: b.SourceBindingID, SettlementConfigID: b.SettlementConfigID, RecipientScript: b.RecipientScript, Revision: 9007199254740993, HighBatch: 18, BeforeBatch: 19, NextBefore: 11, ObservedAtMS: 1, Items: []poolsettle.DailyPayoutItem{}, Balance: poolsettle.DailyBalance{GymID: b.View.GymID, Confirmed: poolsettle.Fraction{Numerator: "0", Denominator: "1"}, Available: poolsettle.Fraction{Numerator: "0", Denominator: "1"}, PaidAtoms: 9007199254740993}}
	return b, p
}

func TestDailyRemotePayoutCapturedWindowsAndExactCounters(t *testing.T) {
	b, p := dailyReadPageFixture()
	c, e := freshDailyPayoutCapture(1, b, 2, p)
	if e != nil {
		t.Fatal(e)
	}
	raw, e := json.Marshal(c.First)
	if e != nil || !strings.Contains(string(raw), `"paid_atoms":"9007199254740993"`) || !strings.Contains(string(raw), `"revision":"9007199254740993"`) {
		t.Fatal("existing lossless page encoding", e)
	}
	for _, before := range []uint64{11, 3} {
		q, cached, e := dailyPayoutContinuation(c, 1, before)
		if e != nil || cached != nil || q.Revision != p.Revision || q.HighBatch != 18 {
			t.Fatal("exact next empty window", q, e)
		}
		next := p
		next.BeforeBatch = before
		next.NextBefore, next.Done = 3, false
		if before == 3 {
			next.NextBefore, next.Done = 0, true
		}
		c, e = extendDailyPayoutCapture(c, q, next)
		if e != nil {
			t.Fatal(e)
		}
	}
	if c.First.BeforeBatch != 19 || c.Current.BeforeBatch != 3 || !c.Current.Done || len(c.First.Items) != 0 || c.CapturedAtMS != 2 {
		t.Fatal("retained bounded windows")
	}
	if _, _, e := dailyPayoutContinuation(c, 1, 11); e == nil {
		t.Fatal("evicted window adopted")
	}
	for _, before := range []uint64{19, 3} {
		_, cached, e := dailyPayoutContinuation(c, 1, before)
		if e != nil || cached == nil || cached.BeforeBatch != before {
			t.Fatal("exact retained replay", e)
		}
	}
	c.Stale = true
	if _, cached, e := dailyPayoutContinuation(c, 1, 19); e != nil || cached == nil {
		t.Fatal("dated first page lost on stale", e)
	}
	if _, _, e := dailyPayoutContinuation(c, 1, 11); e != poolsettle.ErrStaleCapture {
		t.Fatal("stale scan continued", e)
	}
}

func TestDailyRemotePayoutHeaderAndCursorRefusals(t *testing.T) {
	b, p := dailyReadPageFixture()
	c, e := freshDailyPayoutCapture(1, b, 2, p)
	if e != nil {
		t.Fatal(e)
	}
	q, _, e := dailyPayoutContinuation(c, 1, 11)
	if e != nil {
		t.Fatal(e)
	}
	next := p
	next.BeforeBatch, next.NextBefore = 11, 3
	for _, name := range []string{"revision", "high", "cursor", "balance", "time", "source", "settlement", "recipient"} {
		t.Run(name, func(t *testing.T) {
			x := next
			switch name {
			case "revision":
				x.Revision++
			case "high":
				x.HighBatch++
			case "cursor":
				x.BeforeBatch--
			case "balance":
				x.Balance.PaidAtoms++
			case "time":
				x.ObservedAtMS++
			case "source":
				x.BindingID = strings.Repeat("5", 64)
			case "settlement":
				x.SettlementConfigID = strings.Repeat("5", 64)
			case "recipient":
				x.RecipientScript = "0014" + strings.Repeat("5", 40)
			}
			if out, e := extendDailyPayoutCapture(c, q, x); e == nil || out != nil {
				t.Fatal("changed header accepted")
			}
			if c.Current.BeforeBatch != 19 || c.Stale || c.First.Balance.PaidAtoms != 9007199254740993 {
				t.Fatal("failed continuation mutated capture")
			}
		})
	}
	if _, _, e := dailyPayoutContinuation(c, 2, 11); e == nil {
		t.Fatal("foreign capture")
	}
	for _, before := range []uint64{0, 10, 12, 20} {
		if _, _, e := dailyPayoutContinuation(c, 1, before); e == nil {
			t.Fatal("invented cursor", before)
		}
	}
	copy := cloneDailyPayoutCapture(c)
	copy.First.Balance.PaidAtoms = 1
	copy.Current.Items = append(copy.Current.Items, poolsettle.DailyPayoutItem{})
	if c.First.Balance.PaidAtoms == 1 || len(c.Current.Items) != 0 {
		t.Fatal("caller alias")
	}
	p.Items = []poolsettle.DailyPayoutItem{{BatchSequence: 18, Payment: poolsettle.PayoutItem{Day: "1970-01-01", CreatedAtMS: 1, TxID: strings.Repeat("6", 64), Atoms: poolsettle.MinimumAtoms, RecordedState: "confirmed"}}}
	rows, e := freshDailyPayoutCapture(2, b, 2, p)
	if e != nil {
		t.Fatal(e)
	}
	owned := cloneDailyPayoutCapture(rows)
	owned.First.Items[0].Payment.Atoms++
	owned.Current.Items[0].Payment.TxID = strings.Repeat("7", 64)
	if rows.First.Items[0].Payment.Atoms != poolsettle.MinimumAtoms || rows.Current.Items[0].Payment.TxID != p.Items[0].Payment.TxID {
		t.Fatal("retained item slice alias")
	}
}

func TestDailyRemotePayoutModeAndCancellationBeforeEffects(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	a := &RemotePoolAttachment{config: RemotePoolConfig{Version: 2}, view: RemotePoolView{ViewID: "view"}}
	_, e := a.BeginDailyPayoutRead(ctx, "view", 1, 0, 0)
	if wire.TestPoW {
		if e != ErrRemotePoolProfile {
			t.Fatal(e)
		}
		return
	}
	if !errors.Is(e, context.Canceled) || a.dailyRead != nil || a.dailyReadSequence != 0 {
		t.Fatal("canceled admission changed owner", e)
	}
	a.config.Version = 1
	if _, e = a.BeginDailyPayoutRead(context.Background(), "view", 1, 0, 0); e != ErrRemotePoolConfig {
		t.Fatal("legacy adopted daily read", e)
	}
}

func TestDailyRemotePayoutExactOperationSequence(t *testing.T) {
	a := &RemotePoolAttachment{}
	if j, e := a.selectDailyPayoutReadLocked(1, 0, 0); e != nil || j.Sequence != 0 {
		t.Fatal("first operation", e)
	}
	for _, seq := range []uint64{0, 2, math.MaxUint64} {
		if _, e := a.selectDailyPayoutReadLocked(seq, 0, 0); e == nil {
			t.Fatal("unadmitted sequence", seq)
		}
	}
	a.dailyReadSequence = 3
	a.dailyReadLast = RemoteDailyPayoutJob{Sequence: 3, CaptureSequence: 1, BeforeBatch: 11, State: "failed", Error: "busy"}
	if j, e := a.selectDailyPayoutReadLocked(3, 1, 11); e != nil || j != a.dailyReadLast {
		t.Fatal("exact failed repeat changed identity", e)
	}
	if _, e := a.selectDailyPayoutReadLocked(3, 0, 0); e == nil {
		t.Fatal("same sequence became refresh")
	}
	if _, e := a.selectDailyPayoutReadLocked(2, 0, 0); e == nil {
		t.Fatal("evicted sequence reused")
	}
	if j, e := a.selectDailyPayoutReadLocked(4, 0, 0); e != nil || j.Sequence != 0 {
		t.Fatal("next original sequence", e)
	}
	a.dailyRead = &remoteDailyPayoutOperation{job: RemoteDailyPayoutJob{Sequence: 4, State: "working"}}
	if _, e := a.selectDailyPayoutReadLocked(5, 0, 0); e != ErrRemotePoolBusy {
		t.Fatal("overlapping read", e)
	}
	a.dailyRead = nil
	a.dailyReadSequence = math.MaxUint64
	if _, e := a.selectDailyPayoutReadLocked(1, 0, 0); e == nil {
		t.Fatal("sequence wrap")
	}
}
