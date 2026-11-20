// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"errors"
	"github.com/taytr-llc/slug/desktop/internal/bridge"
	"testing"
)

type ownedCloseOnceFailure struct {
	calls int
	err   error
}

func (c *ownedCloseOnceFailure) Close() error {
	c.calls++
	if c.calls == 1 {
		return c.err
	}
	return nil
}
func TestOwnedNativeRecoveryRetainsFailedClose(t *testing.T) {
	failure := errors.New("descriptor close failed")
	owner := &ownedCloseOnceFailure{err: failure}
	g := &Gym{}
	g.exportMu.Lock()
	e := g.closeOwnedRecoverySelection(owner)
	g.exportMu.Unlock()
	if !errors.Is(e, failure) || !errors.Is(e, bridge.ErrRecoveryOutcomeUnknown) || g.owned.recoveryCleanup.owner != owner {
		t.Fatal("original cleanup owner/error lost", e)
	}
	for n := 0; n < 2; n++ {
		if e = g.joinedOwnedRecovery(); !errors.Is(e, failure) {
			t.Fatal("shutdown laundered close error", e)
		}
	}
	if owner.calls != 1 {
		t.Fatal("retried a close with unknown outcome")
	}
	// A later dialog is blocked by the retained failure before app or helper use.
	if _, e = g.ownedRecovery("unavailable", true); !errors.Is(e, failure) {
		t.Fatal("replacement selection allowed", e)
	}
	if g.owned.recoveryCleanup.owner != owner {
		t.Fatal("original cleanup owner replaced")
	}
}
