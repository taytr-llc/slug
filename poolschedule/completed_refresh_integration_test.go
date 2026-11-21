//go:build !slugtestpow && (linux || (darwin && cgo))

// Copyright (c) 2026 The SLUG developers. ISC license.
package poolschedule

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/prospectwork"
)

func completedRefreshKey(prefix string, n uint64) string {
	k := make([]byte, len(prefix)+8)
	copy(k, prefix)
	binary.BigEndian.PutUint64(k[len(prefix):], n)
	return string(k)
}

// Only the scheduler and ledger are physically reopened here. The same concrete
// source/round owners remain borrowed. Reads occur after both writable owners
// have joined; OpenDaily reauthenticates the complete original source again.
func completedRefreshReopen(t *testing.T, h *dailyScheduleHarness, s *Scheduler, path string, c Config) (*Scheduler, map[string][]byte) {
	t.Helper()
	ctx := context.Background()
	if err := s.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if err := h.store.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := leveldb.OpenFile(filepath.Join(h.path, "db"), &opt.Options{ReadOnly: true, ErrorIfMissing: true, Strict: opt.StrictAll})
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string][]byte{}
	it := db.NewIterator(nil, nil)
	for it.Next() {
		if len(rows) >= 64 {
			it.Release()
			closeErr := db.Close()
			t.Fatal("bounded original ledger inventory", closeErr)
		}
		rows[string(it.Key())] = append([]byte(nil), it.Value()...)
	}
	err = it.Error()
	it.Release()
	closeErr := db.Close()
	if err != nil || closeErr != nil {
		t.Fatal(err, closeErr)
	}
	h.store, err = poolsettle.OpenDaily(ctx, h.path, h.config, h.source, h.backend)
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenDaily(ctx, path, c, h.store)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := reopened.Close(ctx); err != nil {
			t.Error(err)
		}
	})
	return reopened, rows
}

func TestDailyScheduleCompletedRefreshImportsOnceAcrossReopen(t *testing.T) {
	if now := time.Now().UTC(); now.Hour() == 23 && now.Minute() >= 58 {
		t.Skip("same-day refresh test requires two real UTC minutes before midnight")
	}
	h := newDailyScheduleHarness(t)
	h.confirmFirst(t)
	s, path, config := dailyScheduleFixture(t, h, 0)
	ctx := context.Background()
	day := time.Now().UTC().Format("2006-01-02")
	h.backend.loseAck = true
	if err := oneCycle(t, s); err != nil {
		t.Fatal(err)
	}
	if h.backend.signCalls != 1 || h.backend.feeCalls != 1 || len(h.backend.broadcastBytes) != 1 {
		t.Fatal("original A preparation inventory")
	}
	signedA := append([]byte(nil), h.backend.broadcastBytes[0]...)
	h.backend.confirmations = 1
	if err := oneCycle(t, s); err != nil {
		t.Fatal(err)
	}
	status, err := s.Status(ctx)
	if err != nil || status.State.DailyCursor != day || status.State.Attempt.ReportReason != "daily-batch-complete" {
		t.Fatal("original completed cursor", status, err)
	}
	s, before := completedRefreshReopen(t, h, s, path, config)
	bodyA := before[completedRefreshKey("b/", 1)]
	creditA := before[completedRefreshKey("c/", 1)]
	var body struct{ Raw string }
	if len(bodyA) == 0 || len(creditA) == 0 || json.Unmarshal(bodyA, &body) != nil || body.Raw != hex.EncodeToString(signedA) {
		t.Fatal("original immutable A did not precede reopen")
	}
	// A new fixed original template is actually hashed under normal PoW. Only
	// canonical/depth and funding observations use the existing controlled node.
	h.node.template.Context.ParentHeight += 61
	h.node.template.Context.ParentHash = strings.Repeat("a", 64)
	h.node.template.Block.Height = int64(h.node.template.Context.ParentHeight) + 1
	h.node.template.Block.PreviousHash = h.node.template.Context.ParentHash
	workB, err := h.pool.IssueMiningWork(ctx, settlementTestToken)
	if err != nil {
		t.Fatal(err)
	}
	_, _, payout := settlementFixtureKey(3)
	prepared, err := prospectwork.PrepareManifestWork(workB.Job, workB.Template, payout, 0, strings.Repeat("f", 64))
	if err != nil {
		t.Fatal(err)
	}
	var winner prospectwork.ShareCheck
	for nonce := uint32(0); nonce < 64; nonce++ {
		candidate, err := prepared.CheckNonce(ctx, nonce)
		if err != nil {
			t.Fatal(err)
		}
		if candidate.Block {
			winner = candidate
			break
		}
	}
	if winner.Candidate == nil {
		t.Fatal("bounded normal B search exhausted")
	}
	h.node.winner = winner
	receipt, err := h.pool.Submit(ctx, settlementTestToken, workB.Job.ID, winner.Nonce)
	if err != nil || receipt.AcceptedSequence != 2 {
		t.Fatal(receipt, err)
	}
	if _, err = h.source.Sync(ctx); err != nil {
		t.Fatal(err)
	}
	if row, err := h.source.Refresh(ctx, winner.WorkHash); err != nil || row.Confirmation == nil {
		t.Fatal(row, err)
	}
	assertCountsAndBalance := func() {
		t.Helper()
		if h.backend.signCalls != 1 || h.backend.feeCalls != 1 || h.backend.broadcasts != 1 || len(h.backend.broadcastBytes) != 1 || !bytes.Equal(signedA, h.backend.broadcastBytes[0]) {
			t.Fatal("refresh quoted, signed or broadcast a replacement payout")
		}
		r, err := h.store.View(ctx)
		if err != nil || r.Day != day || r.State != "complete" || r.Reason != "daily-batch-complete" || len(r.Balances) != 1 {
			t.Fatal("same-day refreshed report", r, err)
		}
		b := r.Balances[0]
		if b.GymID != h.gym || b.Confirmed != (poolsettle.Fraction{Numerator: "990000000", Denominator: "1"}) || b.Available != b.Confirmed || b.ReservedAtoms != 0 || b.PaidAtoms != 990000000 {
			t.Fatal("B credit or A paid amount changed", b)
		}
		st, err := s.Status(ctx)
		if err != nil || st.State.DailyCursor != day || st.State.Attempt.Kind != "daily" || st.State.Attempt.ReportReason != "daily-batch-complete" {
			t.Fatal("occupied cursor not retained", st, err)
		}
	}
	for repeat := 0; repeat < 2; repeat++ {
		if err = oneCycle(t, s); err != nil {
			t.Fatal(err)
		}
		assertCountsAndBalance()
	}
	s, after := completedRefreshReopen(t, h, s, path, config)
	if !bytes.Equal(bodyA, after[completedRefreshKey("b/", 1)]) || !bytes.Equal(creditA, after[completedRefreshKey("c/", 1)]) {
		t.Fatal("refresh rewrote original A body or credit")
	}
	var head struct {
		Credits, Batches, Events uint64
		Pending                  uint64 `json:"pending_batch"`
		Day                      string `json:"last_day"`
	}
	if json.Unmarshal(after["head"], &head) != nil || head.Credits != 2 || head.Batches != 1 || head.Events != 3 || head.Pending != 0 || head.Day != day {
		t.Fatal("exact once-only ledger inventory", head)
	}
	var creditB poolsettle.Credit
	if json.Unmarshal(after[completedRefreshKey("c/", 2)], &creditB) != nil || creditB.RoundHash != winner.WorkHash || creditB.Coin.Atoms != 1000000000 || len(creditB.Coin.Claims) != 2 {
		t.Fatal("B original credit identity", creditB)
	}
	want := map[string]poolsettle.Fraction{"": {Numerator: "10000000", Denominator: "1"}, h.gym: {Numerator: "990000000", Denominator: "1"}}
	for _, claim := range creditB.Coin.Claims {
		amount, ok := want[claim.GymID]
		if !ok || claim.RoundHash != winner.WorkHash || claim.Amount != amount {
			t.Fatal("B exact 99/1 claims", claim)
		}
		delete(want, claim.GymID)
	}
	if len(want) != 0 {
		t.Fatal("missing B claim")
	}
	if err = oneCycle(t, s); err != nil {
		t.Fatal(err)
	}
	assertCountsAndBalance()
	s, final := completedRefreshReopen(t, h, s, path, config)
	if !bytes.Equal(bodyA, final[completedRefreshKey("b/", 1)]) || !bytes.Equal(after[completedRefreshKey("c/", 2)], final[completedRefreshKey("c/", 2)]) || !bytes.Equal(creditA, final[completedRefreshKey("c/", 1)]) {
		t.Fatal("repeat/reopen changed immutable payment or credits")
	}
	var finalHead struct{ Credits, Batches, Events uint64 }
	if json.Unmarshal(final["head"], &finalHead) != nil || !reflect.DeepEqual(finalHead, struct{ Credits, Batches, Events uint64 }{2, 1, 3}) {
		t.Fatal("repeat/reopen duplicated credit", finalHead)
	}
}
