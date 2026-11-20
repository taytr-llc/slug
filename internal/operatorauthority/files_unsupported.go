//go:build !linux && (!darwin || !cgo)

// Copyright (c) 2026 The SLUG developers. ISC license.
// Unsupported platforms fail closed, never generic os.Open.
package operatorauthority

import "context"

type selectedSeed struct{}

func loadSeed(context.Context, string) (*selectedSeed, [32]byte, error) {
	return nil, [32]byte{}, ErrUnsupported
}
func (*selectedSeed) check(context.Context) error { return ErrUnsupported }
func (*selectedSeed) close() error                { return nil }
