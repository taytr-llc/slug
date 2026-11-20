// Copyright (c) 2026 The SLUG developers. ISC license.
package fightercarrier

import "context"

// OpenAudit validates the exact existing journal without creating or removing
// files. Its returned owner refuses TryAcquire; it cannot sign or dispatch.
func OpenAudit(ctx context.Context, path string, expected Identity) (*Journal, error) {
	return openJournalMode(ctx, path, expected, false, true)
}
