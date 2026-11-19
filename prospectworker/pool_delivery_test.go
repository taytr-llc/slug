//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package prospectworker

import (
	"context"
	"encoding/hex"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/taytr-llc/slug/btcec/v2"
	"github.com/taytr-llc/slug/gymauth"
	"github.com/taytr-llc/slug/poolservice"
	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/prospect"
	"github.com/taytr-llc/slug/prospectjob"
	"github.com/taytr-llc/slug/prospectservice"
)

func pdFound(t *testing.T, c PoolConfig, work poolshare.MiningWork, tc pwOracleCase, box *PoolOutbox) PoolEntry {
	t.Helper()
	scan := PoolScan{Version: PoolVersion1, Work: work, NextNonce: uint64(tc.Nonce)}
	if err := box.saveScan(&scan); err != nil {
		t.Fatal(err)
	}
	if err := box.reserve(scan); err != nil {
		t.Fatal(err)
	}
	check, err := poolCheck(PoolEntry{Work: work, Nonce: tc.Nonce, WorkHash: tc.Hash}, c)
	if err != nil || check.Header != tc.Header || check.DuplicateKey != tc.Key || check.Block != tc.Block || check.Prospect != tc.Prospect {
		t.Fatal("fixture structure differs from independent oracle", err)
	}
	scan.NextNonce++
	entry, err := box.found(poolEntryForCheck(scan, check), scan)
	if err != nil {
		t.Fatal(err)
	}
	return entry
}

func TestPoolDeliveryBroadcastPrecedesSigningDespitePoolFailure(t *testing.T) {
	c, work, tc := pwFixture(t, "coinbase-only", math.MaxUint32, false)
	box, _ := pwBox(t, c)
	entry := pdFound(t, c, work, tc, box)
	var order []string
	node := &pwNode{call: func(action string) {
		order = append(order, action)
		rows, err := box.List(0, 1)
		if err != nil || len(rows) != 1 || rows[0].Fighter == nil || rows[0].Fighter.Authorization != nil {
			t.Fatal("broadcast lost durable unsigned evidence", err)
		}
		if action == "canonical" && rows[0].Fighter.BlockState != "submitted" {
			t.Fatal("canonical probe preceded durable broadcast transition")
		}
	}}
	a := &pwAuthority{c: c, w: work, lookup: func() (poolshare.Submission, error) {
		order = append(order, "lookup")
		return poolshare.Submission{}, &poolservice.ClientError{Code: "busy"}
	}}
	ad := &pdPendingAdmission{pwAdmission: &pwAdmission{c: c}, submit: func() {
		order = append(order, "admit")
		rows, err := box.List(0, 1)
		if err != nil || len(rows) != 1 || rows[0].Fighter.Authorization == nil || rows[0].Fighter.BlockState != "canonical" {
			t.Fatal("admission preceded durable authorization or block publication", err)
		}
	}}
	w, err := NewPool(c, a, ad, node, func(_ context.Context, context prospect.Context, p prospect.Proof) (gymauth.Authorization, error) {
		order = append(order, "sign")
		if !reflect.DeepEqual(p, entry.Fighter.Candidate.Proof) {
			t.Fatal("signer received a different proof")
		}
		// Public fixture scalar one; this checks ordering, not a signature vector.
		key, _ := btcec.PrivKeyFromBytes([]byte{1})
		defer key.Zero()
		if hex.EncodeToString(key.PubKey().SerializeCompressed()) != work.Job.PublicKey {
			t.Fatal("fixture signing identity changed")
		}
		return gymauth.SignAdmission(context, p, key)
	}, box)
	if err != nil {
		t.Fatal(err)
	}
	pwClock(w, work)
	if _, err := w.deliver(context.Background(), &entry); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(order, []string{"broadcast", "canonical", "lookup", "sign", "admit"}) {
		t.Fatalf("D69 publication order: %v", order)
	}
	if entry.PoolState != "pending" || entry.LastError != "busy" || entry.Fighter.AdmissionState != "pending" || ad.calls != 1 || a.submits != 0 {
		t.Fatal("pool unavailability suppressed separate fighter admission")
	}
}

type pdPendingAdmission struct {
	*pwAdmission
	submit func()
}

func (a *pdPendingAdmission) SubmitManifest(_ context.Context, j prospectjob.ManifestJob, p prospect.Proof, auth gymauth.Authorization) (prospectservice.ManifestAdmissionResult, error) {
	a.calls++
	a.submit()
	if gymauth.VerifyAuthorization(j.Context, p, auth) != nil {
		return prospectservice.ManifestAdmissionResult{}, &prospectservice.ClientError{Code: "invalid_request"}
	}
	return prospectservice.ManifestAdmissionResult{}, &prospectservice.ClientError{Code: "unavailable"}
}

func TestPoolDeliveryExpiredWinnerCreditsPoolWithoutNewBirth(t *testing.T) {
	c, work, tc := pwFixture(t, "coinbase-only", math.MaxUint32, false)
	box, path := pwBox(t, c)
	entry := pdFound(t, c, work, tc, box)
	late := work.Job.ExpiresAtMS + 100
	a := &pwAuthority{c: c, w: work, submit: func() (poolshare.Submission, error) {
		r := pwReceipt(c, work, tc)
		r.ReceivedAtMS, r.AcceptedAtMS, r.BlockObservedAtMS = late, late, late
		r.Acceptance = "canonical-late-winner"
		return r, nil
	}}
	ad := &pwAdmission{c: c}
	node := &pwNode{}
	w, err := NewPool(c, a, ad, node, func(context.Context, prospect.Context, prospect.Proof) (gymauth.Authorization, error) {
		t.Fatal("expired unsigned job reached fighter signer")
		return gymauth.Authorization{}, nil
	}, box)
	if err != nil {
		t.Fatal(err)
	}
	w.now = func() time.Time { return time.UnixMilli(late) }
	if _, err := w.deliver(context.Background(), &entry); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(node.calls, []string{"broadcast", "canonical"}) || a.submits != 1 || a.issues != 0 || ad.calls != 0 || entry.PoolState != "accepted" || entry.PoolReceipt.Acceptance != "canonical-late-winner" || entry.Fighter.AdmissionState != "expired" || entry.Fighter.Authorization != nil || entry.Fighter.Receipt != nil {
		t.Fatalf("late pool credit crossed D80 fighter boundary: %+v", entry)
	}
	if err := box.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenPoolOutbox(path, c)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	rows, err := reopened.List(0, 1)
	if err != nil || len(rows) != 1 || !reflect.DeepEqual(rows[0], entry) {
		t.Fatal("late credit/expired birth boundary did not survive restart", err)
	}
}

type pdHistoricalAdmission struct{ *pwAdmission }

func (a *pdHistoricalAdmission) SubmitManifest(ctx context.Context, j prospectjob.ManifestJob, p prospect.Proof, auth gymauth.Authorization) (prospectservice.ManifestAdmissionResult, error) {
	r, err := a.pwAdmission.SubmitManifest(ctx, j, p, auth)
	r.Inserted, r.CurrentNodeChecked, r.ContextSource = false, false, "historical-receipt"
	return r, err
}

func TestPoolDeliverySignedExpiredWorkRecoversOriginalFighter(t *testing.T) {
	c, work, tc := pwFixture(t, "coinbase-only", 0, true)
	f := pwAdmissionFixture(t)
	box, _ := pwBox(t, c)
	entry := pdFound(t, c, work, tc, box)
	entry.Fighter.Authorization, entry.Fighter.AdmissionState = &f.Auth, "pending"
	if err := box.update(entry); err != nil {
		t.Fatal(err)
	}
	a := &pwAuthority{c: c, w: work}
	ad := &pdHistoricalAdmission{&pwAdmission{c: c, f: f}}
	w, err := NewPool(c, a, ad, &pwNode{}, func(context.Context, prospect.Context, prospect.Proof) (gymauth.Authorization, error) {
		t.Fatal("retained authorization was signed again")
		return gymauth.Authorization{}, nil
	}, box)
	if err != nil {
		t.Fatal(err)
	}
	w.now = func() time.Time { return time.UnixMilli(work.Job.ExpiresAtMS + 100) }
	if _, err := w.deliver(context.Background(), &entry); err != nil {
		t.Fatal(err)
	}
	if entry.Fighter.AdmissionState != "accepted" || entry.Fighter.Receipt.Inserted || !reflect.DeepEqual(entry.Fighter.Receipt.Record, f.Record) || ad.calls != 1 || a.issues != 0 || a.lookups != 0 || a.submits != 0 {
		t.Fatal("expired historical receipt changed birth or performed new work")
	}
}
