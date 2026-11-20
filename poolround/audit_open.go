// Copyright (c) 2026 The SLUG developers. ISC license.
package poolround

import "github.com/taytr-llc/slug/poolshare"

// OpenAudit opens only an established journal with independently retained pins.
// It runs the same complete stored-state and source crosslink audit as Open,
// without a live backend or repair. The returned owner permits evidence reads
// only; all mutation paths refuse before backend access. Close remains required.
func OpenAudit(path string, limits Limits, source *poolshare.Pool) (*Store, error) {
	return openMode(path, limits, source, nil, true)
}
