// Copyright (c) 2026 The SLUG developers. ISC license.
package pooloperator

import "errors"

var (
	ErrInvalid     = errors.New("invalid pool operator setup")
	ErrUnavailable = errors.New("pool operator input unavailable")
	ErrIntegrity   = errors.New("pool operator input changed")
	ErrCleanup     = errors.New("pool operator cleanup incomplete")
	ErrUnsupported = errors.New("pool operator platform unsupported")
)
