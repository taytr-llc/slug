// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"context"
	"math"

	"github.com/taytr-llc/slug/fighterownership"
	"github.com/taytr-llc/slug/fightertransfer"
	"github.com/taytr-llc/slug/wire/v2"
)

func validateOwnedMempoolEntry(ctx context.Context, reply, raw []byte, txid string) error {
	if ctx == nil || len(raw) == 0 || len(raw) > 12000 || withdrawalRPCJSON(reply) != nil {
		return fighterownership.ErrUnavailable
	}
	if e := ctx.Err(); e != nil {
		return e
	}
	// Discovery performs the existing bounded framing walk before typed raw
	// decoding. The saved leg already passed the concrete journal's validator.
	d, e := fightertransfer.Discover(ctx, raw)
	if e != nil || d.TxID.String() != txid {
		return fighterownership.ErrUnavailable
	}
	var tx wire.MsgTx
	r := bytes.NewReader(raw)
	if tx.Deserialize(r) != nil || r.Len() != 0 || tx.TxHash().String() != txid {
		return fighterownership.ErrUnavailable
	}
	fields, e := withdrawalFields(reply, "size", "vsize", "weight", "height", "wtxid")
	if e != nil {
		return fighterownership.ErrUnavailable
	}
	var size, vsize, weight, height int64
	var wtxid string
	for _, field := range []struct {
		name string
		out  any
	}{{"size", &size}, {"vsize", &vsize}, {"weight", &weight}, {"height", &height}, {"wtxid", &wtxid}} {
		if withdrawalField(fields, field.name, field.out) != nil {
			return fighterownership.ErrUnavailable
		}
	}
	wantWeight := int64(tx.SerializeSizeStripped())*3 + int64(len(raw))
	// Entry height is its admission height and may predate the observed tip;
	// a retained mempool entry may also survive a chain rollback.
	if size != int64(len(raw)) || weight != wantWeight || vsize != (wantWeight+3)/4 || height < 0 || height > math.MaxInt32 || wtxid != tx.WitnessHash().String() {
		return fighterownership.ErrUnavailable
	}
	return ctx.Err()
}
