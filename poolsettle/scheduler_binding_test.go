// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettle

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/taytr-llc/slug/poolshare"
	"github.com/taytr-llc/slug/wire/v2"
)

type bindingTrapBackend struct{ calls int }

func (b *bindingTrapBackend) Inspect(context.Context, []Coin) ([]Funding, error) {
	b.calls++
	return nil, ErrBackend
}
func (b *bindingTrapBackend) EstimateFee(context.Context, []Coin, []Payment, string) (int64, error) {
	b.calls++
	return 0, ErrBackend
}
func (b *bindingTrapBackend) Sign(context.Context, []Coin, []byte) ([]byte, error) {
	b.calls++
	return nil, ErrBackend
}
func (b *bindingTrapBackend) ObserveTransaction(context.Context, string) (TransactionObservation, error) {
	b.calls++
	return TransactionObservation{}, ErrBackend
}
func (b *bindingTrapBackend) Broadcast(context.Context, []byte) (string, error) {
	b.calls++
	return "", ErrBackend
}

func bindingStateBytes(t *testing.T, s *Store) []byte {
	t.Helper()
	raw, err := s.db.Get(stateKey, nil)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestSettlementSchedulerBindingOwnedConfiguration(t *testing.T) {
	h := newSettlementHarness(t)
	ctx := context.Background()
	expectedSource, err := h.store.SourceConfig(ctx)
	if err != nil {
		t.Fatal(err)
	}
	before := bindingStateBytes(t, h.store)
	backend := &bindingTrapBackend{}
	h.store.backend = backend
	config, source, err := h.store.SchedulerBinding(ctx)
	if err != nil || !reflect.DeepEqual(config, h.config) || source != expectedSource {
		t.Fatal("binding differs from independently configured store/source", err)
	}
	// A caller cannot mutate the retained destination slice through this read.
	_, alternateScript, _ := settlementFixtureKey(5)
	config.Destinations[0].Script = alternateScript
	config.Destinations = append(config.Destinations, Destination{GymID: h.gym, Script: alternateScript})
	source.PayoutAddress = "changed caller copy"
	config, source, err = h.store.SchedulerBinding(ctx)
	if err != nil || !reflect.DeepEqual(config, h.config) || source != expectedSource {
		t.Fatal("caller mutation reached retained binding", err)
	}
	if backend.calls != 0 || !bytes.Equal(before, bindingStateBytes(t, h.store)) || len(h.store.state.Credits) != 0 || len(h.store.state.Batches) != 0 {
		t.Fatal("binding read called backend, imported credit or wrote state")
	}
	h.reopen(t)
	config, source, err = h.store.SchedulerBinding(ctx)
	if err != nil || !reflect.DeepEqual(config, h.config) || source != expectedSource {
		t.Fatal("exact original reopen changed logical binding", err)
	}
}

func TestSettlementSchedulerBindingOriginalOwnerRefusals(t *testing.T) {
	h := newSettlementHarness(t)
	ctx := context.Background()
	before := bindingStateBytes(t, h.store)
	check := func(got Config, source poolshare.Config, err, want error) {
		t.Helper()
		if !errors.Is(err, want) || !reflect.DeepEqual(got, Config{}) || source != (poolshare.Config{}) {
			t.Fatalf("refusal disclosed binding: config=%+v source=%+v err=%v", got, source, err)
		}
	}
	c, s, err := h.store.SchedulerBinding(nil)
	check(c, s, err, ErrInvalid)
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	c, s, err = h.store.SchedulerBinding(cancelled)
	check(c, s, err, context.Canceled)
	h.store.gate <- struct{}{}
	c, s, err = h.store.SchedulerBinding(ctx)
	<-h.store.gate
	check(c, s, err, ErrBusy)
	if !bytes.Equal(before, bindingStateBytes(t, h.store)) {
		t.Fatal("refused read changed settlement state")
	}
	if err = h.store.Close(); err != nil {
		t.Fatal(err)
	}
	c, s, err = h.store.SchedulerBinding(ctx)
	check(c, s, err, ErrClosed)
	audit, err := OpenAudit(h.path, h.config, h.source)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = audit.Close() })
	c, s, err = audit.SchedulerBinding(ctx)
	check(c, s, err, ErrInvalid)
	if !bytes.Equal(before, bindingStateBytes(t, audit)) {
		t.Fatal("audit refusal changed settlement state")
	}
	if err = audit.Close(); err != nil {
		t.Fatal(err)
	}
	h.reopen(t)
	if err = h.source.Close(); err != nil {
		t.Fatal(err)
	}
	c, s, err = h.store.SchedulerBinding(ctx)
	check(c, s, err, ErrSource)
	if !bytes.Equal(before, bindingStateBytes(t, h.store)) {
		t.Fatal("unavailable original source changed settlement state")
	}
}

func TestSettlementSchedulerBindingRejectsCheapProfile(t *testing.T) {
	if !wire.TestPoW {
		t.Skip("cheap-profile refusal")
	}
	var s *Store
	c, source, err := s.SchedulerBinding(context.Background())
	if !errors.Is(err, ErrProfile) || !reflect.DeepEqual(c, Config{}) || source != (poolshare.Config{}) {
		t.Fatal("binding accessed state before cheap-profile refusal", err)
	}
}
