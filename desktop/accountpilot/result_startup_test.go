//go:build !slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package accountpilot

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/desktop/pilot"
	"github.com/taytr-llc/slug/gymaccount"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymresult"
	"github.com/taytr-llc/slug/gymseed"
	"os"
	"path/filepath"
	"testing"
)

func resultDefaultFactories() resultAccountFactories {
	d := defaultEventFactories()
	d.child = pilot.NewResultManifestAccount
	return resultAccountFactories{boutAccountFactories{cardAccountFactories{seedAccountFactories{d, gymseed.Open, gymseed.ResumePrepared}, gymcard.Open, gymcard.ResumePrepared}, gymbout.Open, gymbout.ResumePrepared}, gymresult.Open, gymresult.ResumePrepared}
}
func TestResultStartupCancelledAdmissionAndJoinedLegacyOpen(t *testing.T) {
	root, e := filepath.EvalSymlinks(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	cfg := pilot.Config{DataDir: filepath.Join(root, "accounts"), NodeBinary: "/public-fixture/node"}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if c, e := NewResults(ctx, cfg); c != nil || !errors.Is(e, context.Canceled) {
		t.Fatal("cancelled admission", c, e)
	}
	if _, e = os.Lstat(cfg.DataDir); !os.IsNotExist(e) {
		t.Fatal("cancelled admission touched path")
	}
	c, e := NewResults(t.Context(), cfg)
	if e != nil {
		t.Fatal(e)
	}
	authority := c.store.AuthorityID()
	if e = c.Close(); e != nil {
		t.Fatal(e)
	}
	ctx, cancel = context.WithCancel(t.Context())
	deps := resultDefaultFactories()
	open := deps.bout.card.seed.event.openAccount
	var captured *gymaccount.Store
	deps.bout.card.seed.event.openAccount = func(path string, c gymaccount.Config) (*gymaccount.Store, error) {
		s, e := open(path, c)
		captured = s
		cancel()
		return s, e
	}
	if c, e = newResults(ctx, cfg, deps); c != nil || !errors.Is(e, context.Canceled) {
		t.Fatal("legacy cancellation publication", c, e)
	}
	if captured == nil {
		t.Fatal("legacy call not executed")
	}
	// A fresh full reopen proves the synchronous legacy store and instance lock
	// were joined/closed; no detached timed-out open still owns either lock.
	c, e = NewResults(t.Context(), cfg)
	if e != nil {
		t.Fatal("legacy cleanup", e)
	}
	defer c.Close()
	if c.store.AuthorityID() != authority {
		t.Fatal("authority replaced")
	}
}
