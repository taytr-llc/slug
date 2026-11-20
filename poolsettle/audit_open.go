// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettle

import "github.com/taytr-llc/slug/poolround"

// OpenAudit opens only an established journal with independently retained pins.
// It runs the same complete stored-state and source crosslink audit as Open,
// without a live backend or repair. The returned owner permits evidence reads
// only; all mutation paths refuse before backend access. Close remains required.
func OpenAudit(path string, expected Config, source *poolround.Store) (*Store, error) {
	return openMode(path, expected, source, nil, true)
}
