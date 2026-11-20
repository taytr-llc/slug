// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterownership

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestOwnedChainSourceExactAmounts(t *testing.T) {
	for _, test := range []struct {
		raw   string
		atoms int64
	}{
		{"0.005", 500000}, {"5e-3", 500000}, {"0.00500000", 500000},
		{"1e-8", 1}, {"0.00000001", 1}, {"10", 1000000000},
		{"0", 0}, {"1E+2", 10000000000}, {"0.000000010", 1},
	} {
		if got, ok := chainAtoms([]byte(test.raw)); !ok || got != test.atoms {
			t.Fatalf("exact amount %s: %d %v", test.raw, got, ok)
		}
	}
	for _, raw := range []string{"null", `"0.005"`, "-0", "-0.005", "0.005000001", "1e-9", "1e99", "1e-99", "92233720369", "1/2", "NaN", "00.5", "0.005 0.005", "1e1000000000"} {
		if _, ok := chainAtoms([]byte(raw)); ok {
			t.Fatalf("inexact or malformed amount accepted: %s", raw)
		}
	}
}

func TestOwnedChainSourceFreshUnspent(t *testing.T) {
	s, f := chainServer(t)
	ctx := context.Background()
	valid := `{"bestblock":"` + strings.Repeat("1", 64) + `","confirmations":2,"value":0.005,"scriptPubKey":{"asm":"OP_TRUE","hex":"51","type":"nonstandard"},"coinbase":false}`
	set := func(raw string) { f.mu.Lock(); f.txoutRaw = json.RawMessage(raw); f.mu.Unlock() }
	check := func() error { return s.CheckUnspent(ctx, strings.Repeat("4", 64), 0, 500000, []byte{0x51}) }
	set(valid)
	if err := check(); err != nil {
		t.Fatal("exact confirmed output", err)
	}
	for _, raw := range []string{
		"null",
		strings.Replace(valid, `"confirmations":2`, `"confirmations":0`, 1),
		strings.Replace(valid, `"confirmations":2`, `"confirmations":4`, 1),
		strings.Replace(valid, `"value":0.005`, `"value":0.0050000001`, 1),
		strings.Replace(valid, `"value":0.005`, `"value":0.006`, 1),
		strings.Replace(valid, `"value":0.005`, `"value":0.005,"value":0.005`, 1),
		strings.Replace(valid, `"hex":"51"`, `"hex":"52"`, 1),
		strings.Replace(valid, `"hex":"51"`, `"hex":"51","hex":"51"`, 1),
		strings.Replace(valid, `"coinbase":false`, `"coinbase":null`, 1),
		strings.Replace(valid, `"asm":"OP_TRUE"`, `"asm":null`, 1),
		strings.Replace(valid, `"type":"nonstandard"`, `"type":"nonstandard","unknown":0`, 1),
		strings.Replace(valid, strings.Repeat("1", 64), strings.Repeat("2", 64), 1),
	} {
		set(raw)
		if err := check(); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("unavailable output accepted: %s: %v", raw, err)
		}
	}
	set(valid)
	f.mu.Lock()
	f.txoutAdvances = true
	f.mu.Unlock()
	if err := check(); !errors.Is(err, ErrStale) {
		t.Fatal("concurrent chain change", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := check(); !errors.Is(err, ErrClosed) {
		t.Fatal("closed source revived", err)
	}
}
