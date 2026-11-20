// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"context"
	"github.com/taytr-llc/slug/poolservice"
)

func (a *PoolSessionAuthority) Payouts(ctx context.Context) (poolservice.PayoutHistoryResponse, error) {
	var out poolservice.PayoutHistoryResponse
	e := a.withClient(ctx, false, func(c context.Context, client *poolservice.Client) (err error) {
		out, err = client.Payouts(c)
		return err
	})
	if e != nil {
		return poolservice.PayoutHistoryResponse{}, e
	}
	return out, nil
}

var _ poolservice.PayoutReader = (*PoolSessionAuthority)(nil)
