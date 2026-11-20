// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"bytes"
	"testing"
)

func TestChainObservationInstanceIdentity(t *testing.T) {
	first := make([]byte, 32)
	first[31] = 1
	second := make([]byte, 32)
	second[31] = 2
	a, err := newChainObservationInstance(bytes.NewReader(first))
	if err != nil {
		t.Fatal(err)
	}
	b, err := newChainObservationInstance(bytes.NewReader(second))
	if err != nil {
		t.Fatal(err)
	}
	if a != "0000000000000000000000000000000000000000000000000000000000000001" || a == b {
		t.Fatal("public lifetime identity lost exact entropy")
	}
	if _, err = newChainObservationInstance(bytes.NewReader(make([]byte, 32))); err == nil {
		t.Fatal("zero instance accepted")
	}
	if _, err = newChainObservationInstance(bytes.NewReader(first[:31])); err == nil {
		t.Fatal("short entropy accepted")
	}
	if _, err = handleGetChainObservation(&rpcServer{}, nil, nil); err == nil {
		t.Fatal("missing instance accepted")
	}
}
