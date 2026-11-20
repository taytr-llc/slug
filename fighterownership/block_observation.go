// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterownership

import (
	"bytes"
	"context"
	"encoding/hex"
)

// VerifyBlockTransaction verifies bounded transaction framing, the exact
// transaction Merkle root and unique transaction IDs, then locates the exact
// retained bytes. It performs no header hashing and makes no proof-of-work or
// canonicality claim: the caller must retain its original concrete node and
// bracket the supplied canonical header with an unchanged observation.
func VerifyBlockTransaction(ctx context.Context, raw []byte, header ChainHeader, txid string, expected []byte) (uint32, error) {
	if ctx == nil || len(header.Raw) != 80 || !chainHex(header.Hash, 32, false) || !chainHex(txid, 32, false) || len(expected) == 0 || len(expected) > 12000 {
		return 0, ErrInvalid
	}
	if e := ctx.Err(); e != nil {
		return 0, e
	}
	txs, e := parseBlock(ctx, raw, HeaderRow{Height: header.Height, Hash: header.Hash, Header: hex.EncodeToString(header.Raw)})
	if e != nil {
		return 0, e
	}
	for i := range txs {
		if e := ctx.Err(); e != nil {
			return 0, e
		}
		if txs[i].id.String() == txid {
			if !bytes.Equal(txs[i].raw, expected) {
				return 0, ErrConflict
			}
			return uint32(i), nil
		}
	}
	return 0, ErrNotFound
}
