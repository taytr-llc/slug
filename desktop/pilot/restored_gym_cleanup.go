// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"errors"
	"io"
)

// RestoredGymFileCleanupError retains each exact failed closer from the first
// joined attempt. Close reports the same failure; it never retries an OS Close
// or launders a later nil into proof that the earlier cleanup was complete.
// Before a session exists this error is the cleanup owner. An admitted session
// adopts it and keeps its root lock on any such uncertainty.
type RestoredGymFileCleanupError struct {
	owners []io.Closer
	cause  error
}

func (*RestoredGymFileCleanupError) Error() string   { return "restored gym file cleanup failed" }
func (e *RestoredGymFileCleanupError) Unwrap() error { return e.cause }
func (e *RestoredGymFileCleanupError) Close() error  { return e }

func restoredClose(cause error, closers ...io.Closer) error {
	var failed []io.Closer
	for _, c := range closers {
		if c == nil {
			continue
		}
		if e := c.Close(); e != nil {
			cause = errors.Join(cause, e)
			failed = append(failed, c)
		}
	}
	if len(failed) > 0 {
		return &RestoredGymFileCleanupError{owners: failed, cause: cause}
	}
	return cause
}
