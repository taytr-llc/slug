// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"sync"

	"github.com/taytr-llc/slug/chainhash/v2"
)

// maxCachedBlockHeaders bounds digest memoization to slightly more than two
// maximum-sized headers messages. Mining and unique headers may evict old
// entries; this cache reduces repeated hashing, not the cost of unique work.
const maxCachedBlockHeaders = 4096

var blockHeaderHashes = newBlockHeaderHashCache(maxCachedBlockHeaders)

// blockHeaderHashCache maps exact serialized header bytes to locally computed
// digests. Neither keys nor values alias caller memory. It caches no validation
// decisions: changes to target, time, ancestry, or validation flags must still
// be checked by callers even when a digest is already available.
//
// FIFO eviction avoids changing the cache on a hit. All operations hold the
// mutex only briefly; concurrent misses may compute the same digest twice.
type blockHeaderHashCache struct {
	mutex   sync.Mutex
	entries map[[blockHeaderLen]byte]chainhash.Hash
	keys    [][blockHeaderLen]byte
	next    int
}

func newBlockHeaderHashCache(capacity int) *blockHeaderHashCache {
	return &blockHeaderHashCache{
		entries: make(map[[blockHeaderLen]byte]chainhash.Hash, capacity),
		keys:    make([][blockHeaderLen]byte, capacity),
	}
}

func (c *blockHeaderHashCache) lookup(key [blockHeaderLen]byte) (chainhash.Hash, bool) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	hash, ok := c.entries[key]
	return hash, ok
}

func (c *blockHeaderHashCache) add(key [blockHeaderLen]byte, hash chainhash.Hash) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if len(c.keys) == 0 {
		return
	}
	if _, exists := c.entries[key]; exists {
		return
	}
	if len(c.entries) == len(c.keys) {
		delete(c.entries, c.keys[c.next])
	}
	c.keys[c.next] = key
	c.next = (c.next + 1) % len(c.keys)
	c.entries[key] = hash
}
