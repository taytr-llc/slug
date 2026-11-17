// Copyright (c) 2026 The SLUG developers. ISC license.
package prospectworker

import (
	"context"
	"errors"
)

// ManifestCycleHook schedules trusted bounded work on Run's sequential lane.
// It runs outside account, outbox and worker locks before any cycle transport or
// hash. It may not reenter Run or close the worker's dependencies.
type ManifestCycleHook func(context.Context) error

var ErrManifestCycleHook = errors.New("manifest cycle unavailable")

func (worker *ManifestWorker) runCycleHook(ctx context.Context) (err error) {
	if worker.cycleHook == nil || ctx.Err() != nil {
		return nil
	}
	defer func() {
		if recover() != nil {
			err = ErrManifestCycleHook
		}
		if ctx.Err() != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
			err = nil
		}
		if err != nil {
			worker.set("failed", "cycle_unavailable")
			err = ErrManifestCycleHook
		}
	}()
	return worker.cycleHook(ctx)
}
