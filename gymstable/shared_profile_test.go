//go:build slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/taytr-llc/slug/internal/careerstate"
)

func TestSharedStableCheapProfileRefusesBeforeState(t *testing.T) {
	pin := readCoreCorpus(t).Pins[0].Pin
	pin.Version = SharedPinVersion2
	path := filepath.Join(t.TempDir(), "untouched")
	for _, constructor := range []func() (*Store, error){
		func() (*Store, error) { return CreateShared(path, pin, nil) },
		func() (*Store, error) { return OpenShared(path, pin, nil) },
		func() (*Store, error) { return OpenSharedCareer(path, pin, nil, careerstate.Activation{}) },
		func() (*Store, error) { return ResumeSharedCareerBinding(path, pin, nil, careerstate.Activation{}) },
	} {
		if s, e := constructor(); s != nil || !errors.Is(e, ErrProfile) {
			t.Fatal("cheap shared constructor", s, e)
		}
		if _, e := os.Stat(path); !os.IsNotExist(e) {
			t.Fatal("cheap constructor touched storage", e)
		}
	}
	s := &Store{}
	ctx := context.Background()
	if p, e := s.SharedStatus(ctx, Session{}); !errors.Is(e, ErrProfile) || p.Version != 0 {
		t.Fatal("cheap shared status", e)
	}
	if p, e := s.SharedRecent(ctx, Session{}, 8); !errors.Is(e, ErrProfile) || p.Version != 0 {
		t.Fatal("cheap shared recent", e)
	}
	if p, h, e := s.PrepareCareerSharedRecent(ctx, Session{}, 8); !errors.Is(e, ErrProfile) || p.Version != 0 || h != nil {
		t.Fatal("cheap shared career recent", e)
	}
}
