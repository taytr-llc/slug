//go:build !linux && (!darwin || !cgo)

// Copyright (c) 2026 The SLUG developers. ISC license.
package pooloperator

import "context"

type provisionFiles struct{ changed bool }

func openProvisionFiles(string, bool) (*provisionFiles, error) { return nil, ErrUnsupported }
func (*provisionFiles) inventory() error                       { return ErrUnsupported }
func (*provisionFiles) check(context.Context) error            { return ErrUnsupported }
func (*provisionFiles) child(string, bool) (bool, error)       { return false, ErrUnsupported }
func (*provisionFiles) read(context.Context, string, string, int) ([]byte, bool, error) {
	return nil, false, ErrUnsupported
}
func (*provisionFiles) ensure(context.Context, string, string, []byte, int, bool) error {
	return ErrUnsupported
}
func (*provisionFiles) canGenerate() bool { return false }
func (*provisionFiles) close() error      { return nil }
