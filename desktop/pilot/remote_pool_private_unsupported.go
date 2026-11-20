//go:build !linux && (!darwin || !cgo)

// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import "context"

type remotePoolPrivateInput struct{}

func selectRemotePoolPrivateInput(context.Context, string, int) (*remotePoolPrivateInput, error) {
	return nil, errRemotePoolCredentialUnsupported
}
func (*remotePoolPrivateInput) read(context.Context) ([]byte, error) {
	return nil, errRemotePoolCredentialUnsupported
}
func (*remotePoolPrivateInput) check(context.Context) error {
	return errRemotePoolCredentialUnsupported
}
func (*remotePoolPrivateInput) close() error { return nil }
