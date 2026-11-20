//go:build !linux && (!darwin || !cgo)

// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import "context"

type nativeRemoteInput struct{}

func selectNativeRemoteInput(context.Context, string, int) (*nativeRemoteInput, error) {
	return nil, errNativeRemoteUnsupported
}
func (*nativeRemoteInput) read(context.Context) ([]byte, error) {
	return nil, errNativeRemoteUnsupported
}
func (*nativeRemoteInput) check(context.Context) error { return errNativeRemoteUnsupported }
func (*nativeRemoteInput) close() error                { return nil }
