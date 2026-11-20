//go:build !linux && (!darwin || !cgo)

// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"context"
	"errors"
	"testing"
)

func TestRemoteHelperUnsupportedPrivateInputs(t *testing.T) {
	f, e := selectRemoteLaunchInput(context.Background(), "/not-read/private-config", 64)
	if f != nil || !errors.Is(e, errRemoteLaunchUnsupported) {
		t.Fatal("unsupported platform opened private inputs", e)
	}
}
