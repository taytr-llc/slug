//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package registry

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestManifestTryInfoOwnedNonblockingState(t *testing.T) {
	s, _, _, _ := manifestTestStore(t)
	want, err := s.Info()
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.TryInfo()
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatal("different immutable metadata", got, err)
	}
	got.Pin.AuthorityID = "caller mutation"
	again, err := s.TryInfo()
	if err != nil || !reflect.DeepEqual(again, want) {
		t.Fatal("caller mutated authority metadata", err)
	}
	s.mu.Lock()
	done := make(chan error, 1)
	go func() { _, e := s.TryInfo(); done <- e }()
	select {
	case err := <-done:
		if !errors.Is(err, ErrBusy) {
			t.Error("busy authority returned", err)
		}
	case <-time.After(time.Second):
		s.mu.Unlock()
		<-done
		t.Fatal("TryInfo waited for proof gate")
	}
	s.mu.Unlock()
	s.poisoned = errors.New("fixture uncertain write")
	if _, err = s.TryInfo(); !errors.Is(err, ErrPoisoned) {
		t.Fatal("poisoned authority disclosed metadata", err)
	}
	s.poisoned = nil
	s.readOnly = true
	if got, err = s.TryInfo(); err != nil || !got.ReadOnly {
		t.Fatal("read-only state lost", got, err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = s.TryInfo(); !errors.Is(err, ErrClosed) {
		t.Fatal("closed authority disclosed metadata", err)
	}
	var absent *ManifestStore
	if _, err = absent.TryInfo(); !errors.Is(err, ErrClosed) {
		t.Fatal("nil authority disclosed metadata", err)
	}
}
