// Copyright (c) 2026 The SLUG developers. ISC license.
package poolshare

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/taytr-llc/slug/prospectwork"
	"github.com/taytr-llc/slug/wire/v2"
)

func TestPoolMiningWorkSingleIssuanceAndOwnedRecovery(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal source fixture")
	}
	h := setupPool(t, "", 0)
	h.pool.now = time.Now
	before, e := h.authority.Info()
	if e != nil {
		t.Fatal(e)
	}
	w, e := h.pool.IssueMiningWork(context.Background(), poolTestToken)
	if e != nil {
		t.Fatal(e)
	}
	after, e := h.authority.Info()
	if e != nil || after.Jobs != before.Jobs+1 || h.node.proposals != 1 {
		t.Fatal("export issued more than one durable job", e)
	}
	legacy, e := h.pool.Work(context.Background(), poolTestToken, w.Job.ID)
	if e != nil {
		t.Fatal(e)
	}
	prepared, e := prospectwork.PrepareManifestWork(w.Job, w.Template, h.config.PayoutAddress, 0, h.config.PoolTarget)
	if e != nil || !reflect.DeepEqual(prepared.Candidate(), legacy.Candidate) {
		t.Fatal("mining export differs from original candidate", e)
	}
	if w.Template.Block.CoinbaseValue == nil {
		t.Fatal("missing template amount")
	}
	(*w.Template.Block.CoinbaseValue)++
	recovered, e := h.pool.RecoverMiningWork(context.Background(), poolTestToken, w.Job.ID)
	if e != nil || reflect.DeepEqual(w, recovered) {
		t.Fatal("returned template aliases durable work", e)
	}
	prepared, e = prospectwork.PrepareManifestWork(recovered.Job, recovered.Template, h.config.PayoutAddress, 0, h.config.PoolTarget)
	if e != nil || !reflect.DeepEqual(prepared.Candidate(), legacy.Candidate) {
		t.Fatal("recovered template changed", e)
	}
	h.now = w.Job.ExpiresAtMS + 1000
	h.pool.now = func() time.Time { return time.UnixMilli(h.now) }
	if _, e = h.pool.RecoverMiningWork(context.Background(), poolTestToken, w.Job.ID); e != nil {
		t.Fatal("historical work unavailable after expiry", e)
	}
	info, e := h.authority.Info()
	if e != nil || info.Jobs != after.Jobs {
		t.Fatal("recovery issued a new job", e)
	}
	h.sessions.key = "02c6047f9441ed7d6d3045406e95c07cd85aef011134fd65ddfa65673a7ee568f"
	if _, e = h.pool.RecoverMiningWork(context.Background(), poolTestToken, w.Job.ID); !errors.Is(e, ErrUnauthorized) {
		t.Fatal("cross-gym work recovery", e)
	}
}
