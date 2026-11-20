//go:build !linux && (!darwin || !cgo)

// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"context"
	"errors"
	"testing"
)

func TestNativeRemoteUnsupportedProtectedInput(t *testing.T) {
	if s, e := selectNativeRemoteInput(context.Background(), "/private/not-opened", 128); s != nil || !errors.Is(e, errNativeRemoteUnsupported) {
		t.Fatal("unsupported platform admitted input", e)
	}
}
