//go:build !darwin && !linux

// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"errors"
	"os"
)

// An unqualified platform must not fall back to blocking device/FIFO opens.
func openInput(string) (*os.File, error) {
	return nil, errors.New("bounded evidence file opening is unsupported on this platform")
}
