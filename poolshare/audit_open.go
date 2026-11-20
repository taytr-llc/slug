// Copyright (c) 2026 The SLUG developers. ISC license.
package poolshare

import "github.com/taytr-llc/slug/registry"

// OpenAudit opens only an established journal with independently retained pins.
// It runs the same complete stored-state and source crosslink audit as Open,
// without a live backend or repair. The returned owner permits evidence reads
// only; all mutation paths refuse before backend access. Close remains required.
func OpenAudit(path string, expected Config, authority *registry.ManifestStore) (*Pool, error) {
	return openMode(path, expected, authority, nil, nil, true)
}
