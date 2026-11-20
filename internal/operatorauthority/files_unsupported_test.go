//go:build !linux && (!darwin || !cgo)

// Copyright (c) 2026 The SLUG developers. ISC license.
package operatorauthority

import "testing"

func TestOperatorAuthorityUnsupportedSelection(t *testing.T) {
	selected, seed, e := loadSeed(t.Context(), "/synthetic/never-opened.seed")
	if selected != nil || seed != ([32]byte{}) || e != ErrUnsupported {
		t.Fatal("unsupported fallback", e)
	}
}
