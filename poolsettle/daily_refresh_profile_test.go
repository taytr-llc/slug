// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettle

import (
	"context"
	"reflect"
	"testing"

	"github.com/taytr-llc/slug/wire/v2"
)

func TestDailyCompletedRefreshProfileAndMissingOwner(t *testing.T) {
	var s *Store
	report, err := s.RefreshCompletedDaily(context.Background(), "2030-01-01")
	want := ErrClosed
	if wire.TestPoW {
		want = ErrProfile
	}
	if err != want || !reflect.DeepEqual(report, Report{}) {
		t.Fatal(report, err)
	}
}
