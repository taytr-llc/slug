// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"encoding/binary"
	"sync"
	"testing"
	"time"

	"github.com/taytr-llc/slug/chainhash/v2"
	"golang.org/x/crypto/argon2"
)

func TestBlockHeaderHashCache(t *testing.T) {
	cache := newBlockHeaderHashCache(2)
	first, second, third := [blockHeaderLen]byte{1}, [blockHeaderLen]byte{2}, [blockHeaderLen]byte{3}
	hash := chainhash.Hash{1}
	cache.add(first, hash)

	// Values passed in and returned are copies, not aliases of cache state.
	first[0] = 9
	hash[0] = 9
	got, ok := cache.lookup([blockHeaderLen]byte{1})
	if !ok || got != (chainhash.Hash{1}) {
		t.Fatal("caller mutation changed a stored key or digest")
	}
	got[0] = 8
	if got, ok := cache.lookup([blockHeaderLen]byte{1}); !ok || got != (chainhash.Hash{1}) {
		t.Fatal("mutating a returned digest changed cache state")
	}

	cache.add(second, chainhash.Hash{2})
	// A concurrent duplicate insertion neither replaces the existing result
	// nor advances FIFO eviction; callers only insert local deterministic hashes.
	cache.add([blockHeaderLen]byte{1}, chainhash.Hash{1})
	cache.add(third, chainhash.Hash{3})
	if _, ok := cache.lookup([blockHeaderLen]byte{1}); ok {
		t.Fatal("oldest header was not evicted")
	}
	if got, ok := cache.lookup(second); !ok || got != (chainhash.Hash{2}) {
		t.Fatal("duplicate insertion changed eviction order")
	}
	if len(cache.entries) != 2 {
		t.Fatal("cache exceeded its configured bound")
	}

	// Every serialized byte participates in identity; a cached digest may
	// not be returned for a different nonce, target, timestamp, or hash byte.
	for i := range third {
		changed := third
		changed[i] ^= 0x80
		if _, ok := cache.lookup(changed); ok {
			t.Fatalf("ignored serialized header byte %d", i)
		}
	}

	disabled := newBlockHeaderHashCache(0)
	disabled.add(first, hash)
	if _, ok := disabled.lookup(first); ok {
		t.Fatal("zero-capacity cache stored a result")
	}
}

func TestBlockHeaderHashCacheConcurrent(t *testing.T) {
	// Synthetic values exercise locking and eviction under -race without
	// creating concurrent 256 MiB Argon2 arenas in the test process.
	cache := newBlockHeaderHashCache(32)
	var workers sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		workers.Add(1)
		go func(worker int) {
			defer workers.Done()
			for i := 0; i < 500; i++ {
				key := [blockHeaderLen]byte{byte(worker), byte(i), byte(i >> 8)}
				hash := chainhash.Hash{byte(worker), byte(i), byte(i >> 8)}
				cache.add(key, hash)
				if got, ok := cache.lookup(key); ok && got != hash {
					t.Errorf("returned a digest for the wrong header")
					return
				}
			}
		}(worker)
	}
	workers.Wait()
	if len(cache.entries) > 32 {
		t.Fatal("concurrent insertion exceeded cache bound")
	}
}

func directArgon2HeaderHash(header BlockHeader, memory uint32) chainhash.Hash {
	// Independently construct the canonical 80-byte header and call Argon2id
	// with literal parameters supplied by the profile test, not BlockHash or
	// its cache or implementation constants.
	var raw [80]byte
	binary.LittleEndian.PutUint32(raw[0:4], uint32(header.Version))
	copy(raw[4:36], header.PrevBlock[:])
	copy(raw[36:68], header.MerkleRoot[:])
	binary.LittleEndian.PutUint32(raw[68:72], uint32(header.Timestamp.Unix()))
	binary.LittleEndian.PutUint32(raw[72:76], header.Bits)
	binary.LittleEndian.PutUint32(raw[76:80], header.Nonce)
	digest := argon2.IDKey(raw[:], raw[:], 1, memory, 1, 32)
	var hash chainhash.Hash
	copy(hash[:], digest)
	return hash
}

func testBlockHashMemoizationPreservesArgon2(t *testing.T, memory uint32) {
	t.Helper()
	header := BlockHeader{
		Version:    1,
		PrevBlock:  chainhash.Hash{0x11},
		MerkleRoot: chainhash.Hash{0x22},
		Timestamp:  time.Unix(1760000000, 0),
		Bits:       0x207fffff,
		Nonce:      17,
	}
	want := directArgon2HeaderHash(header, memory)
	if got := header.BlockHash(); got != want {
		t.Fatalf("uncached digest %s != independent Argon2id %s", got, want)
	}
	if got := header.BlockHash(); got != want {
		t.Fatalf("cached digest %s != independent Argon2id %s", got, want)
	}

	changes := []struct {
		name   string
		change func(*BlockHeader)
	}{
		{"version", func(h *BlockHeader) { h.Version++ }},
		{"previous hash", func(h *BlockHeader) { h.PrevBlock[0]++ }},
		{"merkle root", func(h *BlockHeader) { h.MerkleRoot[0]++ }},
		{"timestamp", func(h *BlockHeader) { h.Timestamp = h.Timestamp.Add(time.Second) }},
		{"bits", func(h *BlockHeader) { h.Bits-- }},
		{"nonce", func(h *BlockHeader) { h.Nonce++ }},
	}
	for _, test := range changes {
		t.Run(test.name, func(t *testing.T) {
			changed := header
			test.change(&changed)
			wantChanged := directArgon2HeaderHash(changed, memory)
			if got := changed.BlockHash(); got != wantChanged || got == want {
				t.Fatalf("header mutation returned wrong digest %s (want %s)", got, wantChanged)
			}
		})
	}
	if got := header.BlockHash(); got != want {
		t.Fatal("hashing mutated headers damaged the original cached digest")
	}

	// A warm hit must not allocate another memory-hard arena. Measure bytes
	// per operation rather than using a timing threshold tied to one machine.
	benchmark := testing.Benchmark(func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			if header.BlockHash() != want {
				b.Fatal("cached digest changed")
			}
		}
	})
	if benchmark.AllocedBytesPerOp() > 1024 {
		t.Fatalf("warm header hash allocates %d bytes per operation", benchmark.AllocedBytesPerOp())
	}
	t.Logf("warm hash: %d bytes/op, %d ns/op", benchmark.AllocedBytesPerOp(), benchmark.NsPerOp())
}
