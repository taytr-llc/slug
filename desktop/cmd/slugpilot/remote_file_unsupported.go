//go:build !linux && (!darwin || !cgo)

// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import "context"

type remoteLaunchInput struct{}

func selectRemoteLaunchInput(context.Context, string, int) (*remoteLaunchInput, error) {
	return nil, errRemoteLaunchUnsupported
}
func (*remoteLaunchInput) read(context.Context) ([]byte, error) {
	return nil, errRemoteLaunchUnsupported
}
func (*remoteLaunchInput) check(context.Context) error { return errRemoteLaunchUnsupported }
func (*remoteLaunchInput) close() error                { return nil }
