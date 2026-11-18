// Copyright (c) 2026 The SLUG developers. ISC license.
package gymresult

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/taytr-llc/slug/wire/v2"
)

func TestCareerCheapProfileRefusesBeforeFilesystemEffects(t *testing.T) {
	if !wire.TestPoW {
		t.Skip("cheap-profile refusal control")
	}
	path := filepath.Join(t.TempDir(), "absent")
	for name, call := range map[string]func() error{
		"check": func() error { return CheckCareerActivation(context.Background(), path, Pin{}, nil) },
		"activate": func() error {
			s, e := ActivateCareer(context.Background(), path, Pin{}, nil)
			if s != nil {
				t.Fatal("published handle")
			}
			return e
		},
		"resume": func() error {
			s, e := ResumeCareerActivation(context.Background(), path, Pin{}, nil)
			if s != nil {
				t.Fatal("published handle")
			}
			return e
		},
		"open": func() error {
			s, e := OpenCareer(context.Background(), path, Pin{}, nil)
			if s != nil {
				t.Fatal("published handle")
			}
			return e
		},
	} {
		if e := call(); !errors.Is(e, ErrProfile) {
			t.Fatal(name, e)
		}
		if _, e := os.Lstat(path); !errors.Is(e, os.ErrNotExist) {
			t.Fatal("filesystem effect", name, e)
		}
	}
}
