//go:build slugtestpow

package gymstable

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestOwnedStableCheapProfileRefusal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-created")
	for _, open := range []func() (*OwnedStore, error){func() (*OwnedStore, error) { return CreateOwned(path, OwnedPin{}, nil) }, func() (*OwnedStore, error) { return OpenOwned(path, OwnedPin{}, nil) }, func() (*OwnedStore, error) { return OpenOwnedAudit(path, OwnedPin{}, nil) }} {
		s, e := open()
		if s != nil || !errors.Is(e, ErrProfile) {
			t.Fatalf("profile refusal: %v", e)
		}
	}
	if _, e := os.Lstat(path); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("profile refusal mutated filesystem")
	}
	s := new(OwnedStore)
	if _, e := s.Status(context.Background(), Session{}); !errors.Is(e, ErrProfile) {
		t.Fatal(e)
	}
	if _, e := s.PrepareSlot(context.Background(), Session{}, OwnedSlotRequest{}); !errors.Is(e, ErrProfile) {
		t.Fatal(e)
	}
}
