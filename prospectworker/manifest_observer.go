// Copyright (c) 2026 The SLUG developers. ISC license.
package prospectworker

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/taytr-llc/slug/registry"
)

// ManifestReceiptObserver receives an owned verified receipt only after its
// outbox transition synchronized. It runs synchronously outside journal locks.
// It must not reenter Run or Close its dependencies. It is never a replay API.
type ManifestReceiptObserver func(context.Context, registry.ManifestRecord) error

var ErrManifestObserver = errors.New("manifest receipt observer unavailable")

func (worker *ManifestWorker) notifyReceipt(ctx context.Context, record registry.ManifestRecord) (err error) {
	if worker.observer == nil || ctx.Err() != nil {
		return nil
	}
	defer func() {
		if recover() != nil {
			err = ErrManifestObserver
		}
		if err != nil {
			worker.set("failed", "observer_unavailable")
			err = ErrManifestObserver
		}
	}()
	raw, boundErr := json.Marshal(record)
	if boundErr != nil || len(raw) > registry.MaxRecordSize {
		return ErrManifestObserver
	}
	record.Proof.MerkleBranch = append([]string{}, record.Proof.MerkleBranch...)
	if worker.observer(ctx, record) != nil {
		return ErrManifestObserver
	}
	return nil
}
