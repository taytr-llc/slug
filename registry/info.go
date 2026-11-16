// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC license.

package registry

// StoreInfo is an immutable metadata snapshot. Counts are durable resource
// usage, not product limits or evidence of current prospect eligibility.
type StoreInfo struct {
	Version  uint32
	Network  uint32
	Genesis  string
	Records  uint64
	Jobs     uint64
	ReadOnly bool
}

// Info returns metadata without hashing or observing the authority clock. Do
// not call it from a Store callback: callbacks already hold the store lock.
func (store *Store) Info() (StoreInfo, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if err := store.checkState(); err != nil {
		return StoreInfo{}, err
	}
	return StoreInfo{Version: store.meta.Version, Network: store.meta.Network, Genesis: store.meta.Genesis, Records: store.meta.LastSequence, Jobs: store.meta.JobCount, ReadOnly: store.readOnly}, nil
}
