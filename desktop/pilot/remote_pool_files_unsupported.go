//go:build !linux && (!darwin || !cgo)

// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

type remotePoolFiles struct{}

func openRemotePoolFiles(string, RemotePoolConfig, bool) (*remotePoolFiles, error) {
	return nil, errRemotePoolCredentialUnsupported
}
func (*remotePoolFiles) check() error { return errRemotePoolCredentialUnsupported }
func (*remotePoolFiles) child(string) (string, bool, error) {
	return "", false, errRemotePoolCredentialUnsupported
}
func (*remotePoolFiles) adopt(string) error { return errRemotePoolCredentialUnsupported }
func (*remotePoolFiles) markReady() error   { return errRemotePoolCredentialUnsupported }
func (*remotePoolFiles) close() error       { return nil }

func openRemotePoolFilesSelection(string, RemotePoolConfig, bool, *RecoverableRemotePoolConfig) (*remotePoolFiles, error) {
	return nil, errRemotePoolCredentialUnsupported
}
