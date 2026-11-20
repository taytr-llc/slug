// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettle

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/wire/v2"
)

type guardedSigningBackend struct {
	Backend
	original *Store
	config   Config
	source   poolshare.Config
	during   func(context.Context, []Coin, []byte) error
	saved    context.Context
	inputs   []Coin
	unsigned []byte
}

func (b *guardedSigningBackend) Sign(ctx context.Context, coins []Coin, raw []byte) ([]byte, error) {
	b.saved = ctx
	b.inputs = clone(coins)
	b.unsigned = append([]byte(nil), raw...)
	if e := CheckSigningRequest(ctx, b.original, b, b.config, b.source, coins, raw); e != nil {
		return nil, e
	}
	if b.during != nil {
		if e := b.during(ctx, coins, raw); e != nil {
			return nil, e
		}
	}
	signed, e := b.Backend.Sign(ctx, coins, raw)
	if e != nil {
		return nil, e
	}
	if e = CheckSigningRequest(ctx, b.original, b, b.config, b.source, coins, raw); e != nil {
		return nil, e
	}
	return signed, nil
}
func signingBackend(t *testing.T, h *settlementHarness) *guardedSigningBackend {
	t.Helper()
	c, source, e := h.store.SchedulerBinding(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	b := &guardedSigningBackend{Backend: h.backend, original: h.store, config: c, source: source}
	h.store.backend = b
	return b
}

// A value embedding an interface and a slice satisfies Backend but cannot be
// compared as an interface. The constructor must refuse it without panicking.
type incomparableSigningBackend struct {
	Backend
	Values []int
}

func TestSettlementSigningRequestOriginalCapability(t *testing.T) {
	h := newSettlementHarness(t)
	ctx := context.Background()
	b := signingBackend(t, h)
	if e := h.store.CheckBackendBinding(ctx, b, b.config, b.source); e != nil {
		t.Fatal(e)
	}
	before := recoveryDisk(t, h.store)
	b.during = func(ctx context.Context, coins []Coin, raw []byte) error {
		if !bytes.Equal(before, recoveryDisk(t, h.store)) {
			t.Fatal("sign capability created durable ledger rows")
		}
		if e := h.store.CheckBackendBinding(ctx, b, b.config, b.source); !errors.Is(e, ErrBusy) {
			t.Fatal("constructor binding reentered held store gate", e)
		}
		wrongBackend := &guardedSigningBackend{Backend: h.backend}
		foreignStore := &Store{}
		for _, test := range []struct {
			name string
			run  func() error
		}{
			{"absent", func() error {
				return CheckSigningRequest(context.Background(), h.store, b, b.config, b.source, coins, raw)
			}},
			{"foreign-store", func() error { return CheckSigningRequest(ctx, foreignStore, b, b.config, b.source, coins, raw) }},
			{"equal-replacement-backend", func() error { return CheckSigningRequest(ctx, h.store, wrongBackend, b.config, b.source, coins, raw) }},
			{"noncomparable-backend", func() error {
				return CheckSigningRequest(ctx, h.store, incomparableSigningBackend{Backend: h.backend, Values: []int{1}}, b.config, b.source, coins, raw)
			}},
			{"changed-payee", func() error {
				c := clone(b.config)
				c.Destinations[0].Script = "0014" + strings.Repeat("f", 40)
				return CheckSigningRequest(ctx, h.store, b, c, b.source, coins, raw)
			}},
			{"changed-source", func() error {
				s := b.source
				s.MaxWorks++
				return CheckSigningRequest(ctx, h.store, b, b.config, s, coins, raw)
			}},
			{"changed-input", func() error {
				v := clone(coins)
				v[0].Atoms--
				return CheckSigningRequest(ctx, h.store, b, b.config, b.source, v, raw)
			}},
			{"changed-claim", func() error {
				v := clone(coins)
				v[0].Claims[0].Amount.Numerator = "0"
				return CheckSigningRequest(ctx, h.store, b, b.config, b.source, v, raw)
			}},
			{"changed-unsigned", func() error {
				v := append([]byte(nil), raw...)
				v[len(v)-1] ^= 1
				return CheckSigningRequest(ctx, h.store, b, b.config, b.source, coins, v)
			}},
		} {
			if e := test.run(); e == nil {
				t.Fatal(test.name, "accepted")
			}
		}
		return CheckSigningRequest(ctx, h.store, b, b.config, b.source, coins, raw)
	}
	h.backend.beforeBroadcast = func() {
		if len(h.store.state.Batches) != 1 || h.store.state.Batches[0].State != "prepared" {
			t.Fatal("broadcast preceded original signed-batch commit")
		}
		if e := CheckSigningRequest(b.saved, h.store, b, b.config, b.source, b.inputs, b.unsigned); e == nil {
			t.Fatal("sign capability survived synchronous Sign return")
		}
	}
	r, e := h.store.RunDaily(ctx)
	if e != nil || r.State != "pending" || h.backend.signCalls != 1 || h.backend.broadcasts != 1 {
		t.Fatal("real guarded signing path", r, e)
	}
	if e = CheckSigningRequest(context.WithoutCancel(b.saved), h.store, b, b.config, b.source, b.inputs, b.unsigned); e == nil {
		t.Fatal("retained context revived capability")
	}
	// A restart/recovery signs nothing new and remains the original ledger path.
	h.backend.beforeBroadcast = nil
	h.backend.confirmations = 1
	if _, e = h.store.RecoverPending(ctx); e != nil || h.backend.signCalls != 1 {
		t.Fatal("capability added a second payment path", e)
	}
}
func TestSettlementSigningRequestCancellationAndBindingRefusals(t *testing.T) {
	h := newSettlementHarness(t)
	b := signingBackend(t, h)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for _, test := range []struct {
		name string
		run  func() error
	}{
		{"nil", func() error { var s *Store; return s.CheckBackendBinding(context.Background(), b, b.config, b.source) }},
		{"foreign-backend", func() error { return h.store.CheckBackendBinding(context.Background(), h.backend, b.config, b.source) }},
		{"value-backend", func() error {
			return h.store.CheckBackendBinding(context.Background(), incomparableSigningBackend{Backend: h.backend, Values: []int{1}}, b.config, b.source)
		}},
		{"changed-config", func() error {
			c := clone(b.config)
			c.MaxRounds++
			return h.store.CheckBackendBinding(context.Background(), b, c, b.source)
		}},
		{"changed-source", func() error {
			s := b.source
			s.MaxWorks++
			return h.store.CheckBackendBinding(context.Background(), b, b.config, s)
		}},
		{"audit-only", func() error {
			s := &Store{auditOnly: true}
			return s.CheckBackendBinding(context.Background(), b, b.config, b.source)
		}},
	} {
		if e := test.run(); e == nil {
			t.Fatal(test.name, "bound")
		}
	}
	before := recoveryDisk(t, h.store)
	b.during = func(signCtx context.Context, coins []Coin, raw []byte) error {
		cancel()
		if e := CheckSigningRequest(context.WithoutCancel(signCtx), h.store, b, b.config, b.source, coins, raw); !errors.Is(e, context.Canceled) {
			t.Fatal("original cancellation removed by child context", e)
		}
		return signCtx.Err()
	}
	if _, e := h.store.RunDaily(ctx); !errors.Is(e, context.Canceled) {
		t.Fatal("cancelled original signed", e)
	}
	if h.backend.signCalls != 0 || h.backend.broadcasts != 0 || !bytes.Equal(before, recoveryDisk(t, h.store)) {
		t.Fatal("cancelled capability changed financial ledger")
	}
	if e := CheckSigningRequest(context.WithoutCancel(b.saved), h.store, b, b.config, b.source, b.inputs, b.unsigned); e == nil {
		t.Fatal("cancelled retained capability revived")
	}
	if e := h.store.Close(); e != nil {
		t.Fatal(e)
	}
	if e := h.store.CheckBackendBinding(context.Background(), b, b.config, b.source); !errors.Is(e, ErrClosed) {
		t.Fatal("closed owner rebound", e)
	}
}
func TestSettlementConfigBindingValidation(t *testing.T) {
	c := Config{Version: 1, ChangeScript: "0014" + strings.Repeat("1", 40), Destinations: []Destination{{GymID: strings.Repeat("2", 64), Script: "0014" + strings.Repeat("3", 40)}}, MaxRounds: 4, MaxBatches: 4}
	id, e := ConfigBindingID(c)
	if e != nil || len(id) != 64 {
		t.Fatal(e)
	}
	bad := clone(c)
	bad.Destinations[0].Script = bad.ChangeScript
	if id, e = ConfigBindingID(bad); e == nil || id != "" {
		t.Fatal("digest bypassed closed config validation")
	}
	// Exact digest oracle is supplied by the separately frozen wallet16 corpus;
	// this test deliberately does not generate an expected digest with Go.
}
func TestSettlementSigningRequestRejectsCheapProfile(t *testing.T) {
	if !wire.TestPoW {
		t.Skip("cheap profile only")
	}
	if e := CheckSigningRequest(context.Background(), nil, nil, Config{}, poolshare.Config{}, nil, nil); !errors.Is(e, ErrProfile) {
		t.Fatal(e)
	}
	var s *Store
	if e := s.CheckBackendBinding(context.Background(), nil, Config{}, poolshare.Config{}); !errors.Is(e, ErrProfile) {
		t.Fatal(e)
	}
}

func TestSettlementConfigBindingIndependentReference(t *testing.T) {
	raw, e := os.ReadFile("testdata/operator-wallet/reference-v1.json")
	if e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != "c1f2d725fcdbc3af7ba1b0a30f4dd4dfc6983352ffb0b4f72d83f7c09ac7b087" {
		t.Fatal("independent wallet/config corpus changed")
	}
	var corpus struct {
		Guards []struct {
			Fixture    string `json:"fixture"`
			Settlement Config `json:"settlement"`
			Guard      struct {
				ConfigID string `json:"settlement_config_id"`
			} `json:"guard"`
		} `json:"guards"`
		Refusals []struct {
			Name   string `json:"name"`
			Config Config `json:"config"`
		} `json:"config_refusals"`
	}
	if json.Unmarshal(raw, &corpus) != nil || len(corpus.Guards) != 3 || len(corpus.Refusals) != 6 {
		t.Fatal("reference coverage changed")
	}
	for _, row := range corpus.Guards {
		if id, e := ConfigBindingID(row.Settlement); e != nil || id != row.Guard.ConfigID {
			t.Fatal("independent config digest", row.Fixture, id, e)
		}
	}
	for _, row := range corpus.Refusals {
		if id, e := ConfigBindingID(row.Config); e == nil || id != "" {
			t.Fatal("independent config refusal", row.Name)
		}
	}
}
