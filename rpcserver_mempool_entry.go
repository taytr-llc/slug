// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"errors"

	"github.com/taytr-llc/slug/btcjson"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/mempool"
)

func handleGetMempoolEntry(s *rpcServer, cmd interface{}, closed <-chan struct{}) (interface{}, error) {
	select {
	case <-closed:
		return nil, btcjson.NewRPCError(btcjson.ErrRPCInternal.Code, "mempool entry request closed")
	default:
	}
	c, ok := cmd.(*btcjson.GetMempoolEntryCmd)
	if !ok || c == nil || len(c.TxID) != 64 {
		return nil, btcjson.NewRPCError(btcjson.ErrRPCInvalidParameter, "expected canonical transaction ID")
	}
	id, err := chainhash.NewHashFromStr(c.TxID)
	if err != nil || id.String() != c.TxID {
		return nil, btcjson.NewRPCError(btcjson.ErrRPCInvalidParameter, "expected canonical transaction ID")
	}
	if s == nil {
		return nil, btcjson.NewRPCError(btcjson.ErrRPCInternal.Code, "mempool entry unavailable")
	}
	// Existing mempool consumers retain their interface unchanged. The node's
	// concrete TxPool implements this narrow read with one original lock.
	pool, ok := s.cfg.TxMemPool.(interface {
		EntrySnapshot(*chainhash.Hash) (*btcjson.GetMempoolEntrySnapshotResult, error)
	})
	if !ok {
		return nil, btcjson.NewRPCError(btcjson.ErrRPCInternal.Code, "mempool entry unavailable")
	}
	entry, err := pool.EntrySnapshot(id)
	select {
	case <-closed:
		return nil, btcjson.NewRPCError(btcjson.ErrRPCInternal.Code, "mempool entry request closed")
	default:
	}
	if errors.Is(err, mempool.ErrEntryNotFound) {
		return nil, btcjson.NewRPCError(btcjson.ErrRPCInvalidAddressOrKey, "Transaction not in mempool")
	}
	if err != nil || entry == nil {
		message := "mempool entry unavailable"
		if errors.Is(err, mempool.ErrEntryBusy) {
			message = "mempool entry read busy"
		}
		if errors.Is(err, mempool.ErrEntryCapacity) {
			message = "mempool entry exceeds read capacity"
		}
		return nil, btcjson.NewRPCError(btcjson.ErrRPCInternal.Code, message)
	}
	return entry, nil
}
