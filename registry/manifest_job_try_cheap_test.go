//go:build slugtestpow

// Copyright (c) 2026 The SLUG developers. ISC license.
package registry

import (
	"context"
	"testing"

	"github.com/taytr-llc/slug/prospectjob"
)

func TestManifestTryJobCheapRefusesBeforeOwner(t *testing.T) {
	var absent *ManifestStore
	got, err := absent.TryGetManifestJob(context.Background(), "not-an-authorized-job")
	if err == nil || got != (prospectjob.ManifestJob{}) || err.Error() != manifestProduction().Error() {
		t.Fatal("cheap reader reached owner", err)
	}
}
