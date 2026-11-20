// Copyright (c) 2026 The SLUG developers. ISC license.
package registry

// TryInfo returns the same owned public metadata as Info without waiting for
// issuance or proof verification. It performs no hashing or database I/O. An
// owner borrowing this authority for a bounded operation must treat ErrBusy as
// unavailable and may retry outside any account lease.
func (s *ManifestStore) TryInfo() (ManifestAuthorityInfo, error) {
	if err := manifestProduction(); err != nil {
		return ManifestAuthorityInfo{}, err
	}
	if s == nil {
		return ManifestAuthorityInfo{}, ErrClosed
	}
	if !s.mu.TryLock() {
		return ManifestAuthorityInfo{}, ErrBusy
	}
	defer s.mu.Unlock()
	if err := s.state(false); err != nil {
		return ManifestAuthorityInfo{}, err
	}
	return ManifestAuthorityInfo{s.meta.pin(), s.meta.Descriptor, s.meta.LastSequence, s.meta.JobCount, s.readOnly}, nil
}
