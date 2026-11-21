//go:build !linux && (!darwin || !cgo)

// Copyright (c) 2026 The SLUG developers. ISC license.
package pooloperator

type operatorFiles struct{}

func openOperatorFiles(string, Setup, bool) (*operatorFiles, error) { return nil, ErrUnsupported }
func (*operatorFiles) check() error                                 { return ErrUnsupported }
func (*operatorFiles) child(string) (string, bool, error)           { return "", false, ErrUnsupported }
func (*operatorFiles) adopt(string) error                           { return ErrUnsupported }
func (*operatorFiles) markReady() error                             { return ErrUnsupported }
func (*operatorFiles) close() error                                 { return nil }

func openOperatorFilesVersion(string, Setup, bool, uint32) (*operatorFiles, error) {
	return nil, ErrUnsupported
}
