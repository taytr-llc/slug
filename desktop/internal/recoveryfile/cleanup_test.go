// Copyright (c) 2026 The SLUG developers. ISC license.
package recoveryfile

import (
	"context"
	"errors"
	"os"
	"testing"
)

type cleanupProbe struct {
	calls int
	err   error
}

func (p *cleanupProbe) Close() error {
	p.calls++
	if p.calls == 1 {
		return p.err
	}
	return nil
}

func TestConstructorCleanupRetainsOriginalOwners(t *testing.T) {
	cause := errors.New("selection rejected")
	fault := errors.New("close failed after releasing descriptor")
	first := &cleanupProbe{err: fault}
	second := &cleanupProbe{}
	e := closeOwned(cause, first, second)
	var owner *CleanupError
	if !errors.As(e, &owner) || !errors.Is(e, cause) || !errors.Is(e, fault) || !errors.Is(e, ErrCleanup) {
		t.Fatal("constructor cleanup lost its cause or concrete owner", e)
	}
	if len(owner.owners) != 1 || owner.owners[0] != first || first.calls != 1 || second.calls != 1 {
		t.Fatal("cleanup did not retain the exact failed owner")
	}
	if owner.Close() != owner || owner.Close() != owner || first.calls != 1 {
		t.Fatal("cleanup retried an OS close or erased the original failure")
	}
	if closeOwned(cause, &cleanupProbe{}) != cause {
		t.Fatal("successful cleanup changed the original error classification")
	}
}

func TestPublishFailedCloseRetainsDescriptorOwner(t *testing.T) {
	_, h, path := newOutput(t)
	closeFile := h.ops.closeFile
	var original *os.File
	calls := 0
	h.ops.closeFile = func(f *os.File) error {
		original = f
		calls++
		return errors.Join(closeFile(f), fault)
	}
	e := h.Publish(context.Background(), []byte("private fixture bytes"), allow)
	var cleanup *CleanupError
	if !errors.As(e, &cleanup) || !errors.Is(e, fault) || errors.Is(e, ErrOutcomeUnknown) {
		t.Fatal("prepublication close failure classification", e)
	}
	if len(cleanup.owners) != 1 || cleanup.owners[0] != original || calls != 1 {
		t.Fatal("lost original publication descriptor")
	}
	for range 2 {
		var retained *CleanupError
		if !errors.As(h.Close(), &retained) || retained != cleanup || calls != 1 {
			t.Fatal("handle close discarded or retried failed publication cleanup")
		}
	}
	absent(t, path)
	noTemps(t, path)
}

func TestConstructorCleanupSingleOwnerRetainsAllFailures(t *testing.T) {
	fault := errors.New("descriptor close failed")
	first, second, third := &cleanupProbe{err: fault}, &cleanupProbe{err: fault}, &cleanupProbe{err: fault}
	e := closeOwned(ErrInvalid, first, second)
	var original *CleanupError
	if !errors.As(e, &original) || len(original.owners) != 2 || original.owners[0] != first || original.owners[1] != second {
		t.Fatal("single adopted error lost a sibling owner", e)
	}
	e = closeOwned(e, third)
	var outer *CleanupError
	if !errors.As(e, &outer) || outer == original || len(outer.owners) != 1 || outer.owners[0] != third {
		t.Fatal("later cleanup did not retain its own descriptor", e)
	}
	var nested *CleanupError
	if !errors.As(outer.Unwrap(), &nested) || nested != original || !errors.Is(outer, ErrInvalid) {
		t.Fatal("outer cleanup discarded earlier owners or cause")
	}
	if outer.Close() != outer || original.Close() != original || first.calls != 1 || second.calls != 1 || third.calls != 1 {
		t.Fatal("cleanup repeated a descriptor close")
	}
}
