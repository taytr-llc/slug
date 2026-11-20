//go:build !linux && (!darwin || !cgo)

// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"errors"
	"github.com/taytr-llc/slug/wire/v2"
	"testing"
)

func TestRemotePoolCredentialsUnsupportedRefusal(t *testing.T) {
	if wire.TestPoW {
		t.Skip("separate cheap refusal")
	}
	c := remotePoolTestReference(t).Positives[0].Config
	o, e := openRemotePoolCredentials(context.Background(), c, remotePoolCredentialSelection{})
	if o != nil || !errors.Is(e, errRemotePoolCredentialUnsupported) {
		t.Fatal("unsupported credential custody admitted", e)
	}
}
