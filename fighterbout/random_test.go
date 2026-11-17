// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterbout

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
)

func TestUniformRejectionAndLimits(t *testing.T) {
	// For n=10, threshold=6; both rejected words must be consumed.
	words := []uint64{0, 5, 26}
	calls := 0
	got, err := uniform(10, func() (uint64, error) {
		word := words[calls]
		calls++
		return word, nil
	})
	if err != nil || got != 6 || calls != 3 {
		t.Fatalf("rejection %d,%v after %d", got, err, calls)
	}
	calls = 0
	got, err = uniform(10, func() (uint64, error) { calls++; return 0, nil })
	if got != 0 || !errors.Is(err, ErrStream) || calls != 256 {
		t.Fatal("per-draw rejection limit")
	}
	calls = 0
	got, err = uniform(0, func() (uint64, error) { calls++; return 0, nil })
	if got != 0 || !errors.Is(err, ErrStream) || calls != 0 {
		t.Fatal("zero bound consumed a word")
	}
}

func TestStreamSharedBudgetAndCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	d := newDraws(ctx, strings.Repeat("0", 64))
	d.streams[0].budget.count = MaxRawWords - 1
	if _, err := d.streams[1].next(); err != nil {
		t.Fatal(err)
	}
	if word, err := d.streams[4].next(); word != 0 || !errors.Is(err, ErrStream) {
		t.Fatal("shared ceiling not enforced")
	}
	if d.streams[4].counter != 0 || d.streams[1].counter != 1 {
		t.Fatal("failed word advanced a counter")
	}
	cancel()
	if word, err := d.streams[2].next(); word != 0 || !errors.Is(err, context.Canceled) {
		t.Fatal("cancellation did not precede stream work")
	}
	d = newDraws(context.Background(), strings.Repeat("0", 64))
	d.streams[0].counter = math.MaxUint64
	if word, err := d.streams[0].next(); word != 0 || !errors.Is(err, ErrStream) {
		t.Fatal("counter wrap accepted")
	}
}

func TestChanceEndpointsConsumeAndReject(t *testing.T) {
	calls := 0
	d := draws{nextWord: func(stream int) (uint64, error) {
		if stream != 4 {
			t.Fatal("chance used another stream")
		}
		calls++
		return 1000000, nil // Accepts rejection threshold and maps to zero.
	}}
	if d.chance(0) || !d.chance(Scale) || calls != 2 || d.err != nil {
		t.Fatal("endpoint probability did not consume a word")
	}
	if d.chance(-1) || calls != 2 || !errors.Is(d.err, ErrArithmetic) {
		t.Fatal("invalid probability was accepted")
	}
}
