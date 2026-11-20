// Copyright (c) 2026 The SLUG developers. ISC license.
package registry

// ErrPublicCapacity identifies only an exact, validated public-journal limit.
// It preserves the old ErrPublicOwnership classification and error text. Generic
// malformed ownership evidence must never be interpreted as financial drain.
var ErrPublicCapacity error = &publicCapacityError{}

type publicCapacityError struct{}

func (*publicCapacityError) Error() string { return ErrPublicOwnership.Error() }
func (*publicCapacityError) Unwrap() error { return ErrPublicOwnership }
