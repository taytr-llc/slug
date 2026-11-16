//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package registry

import (
	"errors"
	"testing"
)

func TestInfoTracksDurableCountsAndFailsClosed(t *testing.T) {
	f := birthFixture(t)
	s, path, _ := newBirthStore(t, f)
	initial, err := s.Info()
	if err != nil || initial.Version != Version3 || initial.Network != f.Context.Network || initial.Genesis != f.Context.Genesis || initial.Records != 0 || initial.Jobs != 0 {
		t.Fatalf("initial info: %+v %v", initial, err)
	}
	issueFixtureJob(t, s, f)
	if _, _, err := s.RecordJob(f.Proof, f.Authorization, acceptJob); err != nil {
		t.Fatal(err)
	}
	info, err := s.Info()
	if err != nil || info.Jobs != 1 || info.Records != 1 {
		t.Fatalf("durable counts: %+v %v", info, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Info(); !errors.Is(err, ErrClosed) {
		t.Fatal("closed store returned metadata", err)
	}
	s, err = Open(path, true)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	got, err := s.Info()
	info.ReadOnly = true
	if err != nil || got != info {
		t.Fatalf("read-only reopened counts changed: %+v %v", got, err)
	}
	s.poisoned = errors.New("simulated uncertain write")
	if _, err := s.Info(); !errors.Is(err, ErrPoisoned) {
		t.Fatal("poisoned store returned metadata", err)
	}
}
