// Copyright (c) 2026 The SLUG developers. ISC license.
package poolshare

import (
	"context"
	"errors"
	"testing"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

// A slow high-water sync must not let a later record write use a timestamp
// captured before expiry. Completion of the record's own atomic write may occur
// later; the acceptance convention is the time immediately before that write.
func TestPoolFinalAcceptanceDoesNotReusePreSyncTime(t *testing.T) {
	h := setupPool(t, "", 0)
	guardFinished, recordWritten := false, false
	h.node.onCurrent = func() {
		h.now = h.source.Job.ExpiresAtMS - 1
		guardFinished = true
	}
	write := h.pool.writeBatch
	h.pool.writeBatch = func(batch *leveldb.Batch, options *opt.WriteOptions) error {
		detector := new(submissionBatchDetector)
		if err := batch.Replay(detector); err != nil {
			return err
		}
		if guardFinished && !detector.found {
			// Model a separate clock write taking the operation past expiry.
			h.now = h.source.Job.ExpiresAtMS + 1
		}
		if detector.found {
			recordWritten = true
			if h.now >= h.source.Job.ExpiresAtMS {
				t.Error("ordinary acceptance started after expiry using a stale timestamp")
			}
		}
		return write(batch, options)
	}
	got, err := h.pool.Submit(context.Background(), poolTestToken, h.source.Job.ID, h.ordinary.Nonce)
	if recordWritten {
		if err != nil || got.AcceptedAtMS >= h.source.Job.ExpiresAtMS {
			t.Fatal("invalid accepted-time boundary", err)
		}
	} else if !errors.Is(err, ErrExpired) {
		t.Fatal("expired operation was neither atomically accepted in time nor refused", err)
	}
}
