// Copyright (c) 2026 The SLUG developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package peer

import (
	"runtime"
	"testing"
	"time"

	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/wire/v2"
)

// TestBlockMessageSummary ensures enabling per-peer debug logging cannot
// allocate a production Argon2id arena for every incoming or outgoing block.
func TestBlockMessageSummary(t *testing.T) {
	// Even an empty, invalid block reaches peer logging before validation.
	block := wire.MsgBlock{
		Header: wire.BlockHeader{
			Version:   4,
			PrevBlock: chainhash.Hash{1},
			Timestamp: time.Unix(1794700800, 0).UTC(),
			Bits:      0x207fffff,
			Nonce:     7,
		},
	}

	// This test is deliberately serial.  Allow ample incidental runtime
	// allocation while staying far below a 256 MiB PoW hash allocation.
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	summary := messageSummary(&block)
	runtime.ReadMemStats(&after)
	if allocated := after.TotalAlloc - before.TotalAlloc; allocated > 8<<20 {
		t.Fatalf("block summary allocated %d bytes; expected less than 8 MiB", allocated)
	}

	want := "prev 0000000000000000000000000000000000000000000000000000000000000001, " +
		"ver 4, 0 tx, bits 207fffff, nonce 7, 2026-11-15 00:00:00 +0000 UTC"
	if summary != want {
		t.Fatalf("unexpected block summary\n got: %s\nwant: %s", summary, want)
	}
}

func BenchmarkBlockMessageSummary(b *testing.B) {
	block := wire.MsgBlock{
		Header: wire.BlockHeader{
			Version:   4,
			PrevBlock: chainhash.Hash{1},
			Timestamp: time.Unix(1794700800, 0).UTC(),
			Bits:      0x207fffff,
			Nonce:     7,
		},
	}
	b.ReportAllocs()
	for b.Loop() {
		messageSummary(&block)
	}
}
