//go:build !darwin && !linux && !freebsd && !openbsd && !netbsd && !dragonfly

// Copyright (c) 2026 The SLUG developers. ISC license.
package poolminer

type credentialFiles struct {
	path      string
	markerRaw []byte
}

func openCredentialFiles(string, bool) (*credentialFiles, error) { return nil, ErrProfile }
func (*credentialFiles) initialize([]byte) error                 { return ErrProfile }
func (*credentialFiles) check() error                            { return ErrProfile }
func (*credentialFiles) sync() error                             { return ErrProfile }
func (*credentialFiles) close() error                            { return nil }

func (*credentialFiles) auditDBBeforeOpen() error { return ErrProfile }
