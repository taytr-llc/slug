// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettlenode

import (
	"errors"
	"testing"

	"github.com/taytr-llc/slug/wire/v2"
)

func TestDailyNodeProfileOriginalOwner(t *testing.T) {
	var absent *Client
	if !errors.Is(absent.CheckDailyProfile(), ErrInvalid) || !errors.Is(new(Client).CheckDailyProfile(), ErrInvalid) {
		t.Fatal("nil or unconstructed profile admitted")
	}
	if wire.TestPoW {
		t.Skip("constructed normal node owner; cheap constructor separately refuses")
	}
	m, _ := newDailyModel(1)
	daily := dailyClient(t, m, true)
	legacy := dailyClient(t, m, false)
	before := m.calls.Load()
	for i := 0; i < 10; i++ {
		if e := daily.CheckDailyProfile(); e != nil {
			t.Fatal(e)
		}
		if !errors.Is(legacy.CheckDailyProfile(), ErrInvalid) {
			t.Fatal("legacy owner upgraded")
		}
	}
	if m.calls.Load() != before || !daily.daily || legacy.daily {
		t.Fatal("profile check performed I/O or changed selection")
	}
	daily.cancel()
	if !errors.Is(daily.CheckDailyProfile(), ErrClosed) {
		t.Fatal("canceled original owner admitted")
	}
	if e := daily.Close(); e != nil {
		t.Fatal(e)
	}
	if !errors.Is(daily.CheckDailyProfile(), ErrClosed) || m.calls.Load() != before {
		t.Fatal("closed owner revived or queried")
	}
}
