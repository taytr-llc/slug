//go:build slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package registry

import "testing"

func TestManifestTryInfoRefusesCheapProfileFirst(t *testing.T) {
	s := &ManifestStore{}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.TryInfo(); err == nil || err.Error() != "manifest authority requires production PoW; slugtestpow is unsupported" {
		t.Fatal("cheap profile was not refused before authority gate", err)
	}
	var absent *ManifestStore
	if _, err := absent.TryInfo(); err == nil || err.Error() != "manifest authority requires production PoW; slugtestpow is unsupported" {
		t.Fatal("cheap profile was not refused before nil state", err)
	}
}
