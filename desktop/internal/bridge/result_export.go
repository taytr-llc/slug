// Copyright (c) 2026 The SLUG developers. ISC license.
package bridge

import (
	"context"
	"github.com/taytr-llc/slug/desktop/pilot"
)

// The destination is supplied only by trusted native selection or an explicit
// test destination. JavaScript has no path-bearing Result API.
func (b *StableBridge) ExportResultProof(ctx context.Context, view, date, bid, path string) pilot.ResultResponse {
	return b.ResultRequest(ctx, pilot.ResultRequest{Command: "result_export", ViewID: view, Saturday: date, BoutID: bid, Path: path})
}
