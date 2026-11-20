// Copyright (c) 2026 The SLUG developers. ISC license.
package recoveryfile

import (
	"errors"
	"io"
)

// CleanupError owns descriptors whose first Close reported a failure, including
// failures before Capture or Open could return their normal owner. Callers must
// retain this error until shutdown. Close never retries an OS descriptor: a
// failed close can already have released its number for reuse.
type CleanupError struct {
	owners []io.Closer
	cause  error
}

func (*CleanupError) Error() string   { return "recovery file cleanup failed" }
func (e *CleanupError) Unwrap() error { return e.cause }
func (e *CleanupError) Close() error  { return e }

func failedClose(owner io.Closer, err error) error {
	if err == nil {
		return nil
	}
	return &CleanupError{owners: []io.Closer{owner}, cause: errors.Join(ErrCleanup, err)}
}

func closeOwned(cause error, owners ...io.Closer) error {
	var failed []io.Closer
	for _, owner := range owners {
		if owner != nil {
			if e := owner.Close(); e != nil {
				cause = errors.Join(cause, e)
				failed = append(failed, owner)
			}
		}
	}
	if len(failed) != 0 {
		return &CleanupError{owners: failed, cause: errors.Join(ErrCleanup, cause)}
	}
	return cause
}
