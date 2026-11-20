// Copyright (c) 2026 The SLUG developers. ISC license.
package rpcclient

import (
	"errors"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/btcjson"
)

func TestChainObservationStrictResponse(t *testing.T) {
	// Literal protocol example, not captured implementation output. The max
	// uint64 boundary must survive as exact decimal text in JSON consumers.
	const raw = `{"version":1,"instance_id":"0000000000000000000000000000000000000000000000000000000000000001","revision":"18446744073709551615","hash":"0000000000000000000000000000000000000000000000000000000000000002","height":3}`
	receive := func(s string) (*btcjson.GetChainObservationResult, error) {
		ch := make(chan *Response, 1)
		ch <- &Response{result: []byte(s)}
		return FutureGetChainObservationResult(ch).Receive()
	}
	got, err := receive(raw)
	if err != nil || got.Revision != "18446744073709551615" || got.Height != 3 {
		t.Fatal("exact uint64 observation", err)
	}
	bad := []string{
		strings.Replace(raw, `"version":1`, `"version":2`, 1),
		strings.Replace(raw, `"version":1`, `"version":1,"version":1`, 1),
		strings.Replace(raw, `"height":3`, `"height":-1`, 1),
		strings.Replace(raw, `"height":3`, `"height":2147483648`, 1),
		strings.Replace(raw, `"height":3`, `"height":3.0`, 1),
		strings.Replace(raw, `"height":3`, `"height":3,"extra":true`, 1),
		strings.Replace(raw, `"revision":"18446744073709551615"`, `"revision":18446744073709551615`, 1),
		strings.Replace(raw, "0000000000000000000000000000000000000000000000000000000000000001", strings.Repeat("0", 64), 1),
		raw + " ", raw + raw, "null", "{}", strings.Repeat(" ", 513),
	}
	for _, revision := range []string{"0", "01", "+1", "1.0", "1e3", "18446744073709551616", "-1", ""} {
		bad = append(bad, strings.Replace(raw, "18446744073709551615", revision, 1))
	}
	for i, value := range bad {
		if got, err := receive(value); got != nil || !errors.Is(err, ErrInvalidChainObservation) {
			t.Fatalf("accepted malformed response %d: %v", i, err)
		}
	}
}

func TestChainObservationOnlyExactBusyRetries(t *testing.T) {
	for _, test := range []struct {
		code    btcjson.RPCErrorCode
		message string
		busy    bool
	}{
		{btcjson.ErrRPCInternal.Code, "chain observation busy", true},
		{btcjson.ErrRPCInternal.Code, "chain observation unavailable", false},
		{btcjson.ErrRPCInternal.Code, "chain observation busy ", false},
		{btcjson.ErrRPCInvalidParams.Code, "chain observation busy", false},
	} {
		original := btcjson.NewRPCError(test.code, test.message)
		ch := make(chan *Response, 1)
		ch <- &Response{err: original}
		value, err := FutureGetChainObservationResult(ch).Receive()
		if value != nil || errors.Is(err, ErrChainObservationBusy) != test.busy {
			t.Fatal("busy classification", err)
		}
		if !test.busy && err != original {
			t.Fatal("unrelated RPC error was replaced")
		}
	}
}
