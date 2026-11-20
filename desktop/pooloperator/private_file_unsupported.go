//go:build !linux && (!darwin || !cgo)

// Copyright (c) 2026 The SLUG developers. ISC license.
package pooloperator

import "context"

type privateInput struct{}

func selectPrivateInput(context.Context, string, int) (*privateInput, error) {
	return nil, ErrUnsupported
}
func (*privateInput) read(context.Context) ([]byte, error) { return nil, ErrUnsupported }
func (*privateInput) check(context.Context) error          { return ErrUnsupported }
func (*privateInput) close() error                         { return nil }
