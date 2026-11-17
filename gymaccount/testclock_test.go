//go:build slugtestclock

// Copyright (c) 2026 The SLUG developers. ISC license.
package gymaccount

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/wire/v2"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFixtureScopedClockDoesNotChangeOtherHandles(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal fixture profile")
	}
	a, b := int64(1700000000000), int64(1800000000000)
	pa, pb := filepath.Join(t.TempDir(), "a"), filepath.Join(t.TempDir(), "b")
	sa, e := CreateFixtureForTestClock(pa, accountTestConfig(), func() time.Time { return time.UnixMilli(a) })
	if e != nil {
		t.Fatal(e)
	}
	defer sa.Close()
	sb, e := CreateFixtureForTestClock(pb, accountTestConfig(), func() time.Time { return time.UnixMilli(b) })
	if e != nil {
		t.Fatal(e)
	}
	defer sb.Close()
	for i, s := range []*Store{sa, sb} {
		now := a
		if i == 1 {
			now = b
		}
		if _, e := s.Login(context.Background(), Principal{Issuer: "fixture:local", Subject: "alice", AuthenticatedAtMS: now}); e != nil {
			t.Fatal(e)
		}
	}
	if sa.meta.ClockMS != a || sb.meta.ClockMS != b {
		t.Fatal("clock scope leaked")
	}
	sa.Close()
	a++
	sa, e = OpenFixtureForTestClock(pa, accountTestConfig(), func() time.Time { return time.UnixMilli(a) })
	if e != nil {
		t.Fatal(e)
	}
	defer sa.Close()
	if _, e = sa.Login(context.Background(), Principal{Issuer: "fixture:local", Subject: "bob", AuthenticatedAtMS: a}); e != nil {
		t.Fatal(e)
	}
	if sa.meta.ClockMS != a || sb.meta.ClockMS != b {
		t.Fatal("reopen clock scope leaked")
	}
}
func TestFixtureScopedClockNilAndCheapRefuseBeforeFilesystem(t *testing.T) {
	p := filepath.Join(t.TempDir(), "absent")
	if _, e := CreateFixtureForTestClock(p, accountTestConfig(), nil); e == nil {
		t.Fatal("nil clock")
	}
	if wire.TestPoW {
		if _, e := CreateFixtureForTestClock(p, accountTestConfig(), time.Now); !errors.Is(e, ErrForbidden) {
			t.Fatal(e)
		}
	}
	if _, e := os.Lstat(p); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("created refused store")
	}
}
