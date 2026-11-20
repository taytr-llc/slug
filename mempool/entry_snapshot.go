// Copyright (c) 2026 The SLUG developers. ISC license.
package mempool

import (
	"errors"

	"github.com/taytr-llc/slug/blockchain"
	"github.com/taytr-llc/slug/btcjson"
	"github.com/taytr-llc/slug/btcutil/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
)

var (
	ErrEntryNotFound    = errors.New("transaction not in mempool")
	ErrEntryBusy        = errors.New("mempool entry read busy")
	ErrEntryCapacity    = errors.New("mempool entry exceeds read capacity")
	ErrEntryUnavailable = errors.New("mempool entry unavailable")
)

// EntrySnapshot returns an owned projection of exactly one main-pool TxDesc.
// The original read lock covers the entry and its direct input-parent lookups;
// no chain/UTXO callback, whole-pool scan, or ancestry traversal occurs. Busy
// reads fail immediately so an RPC disconnect never waits for this lock.
// Admitted transaction bytes are bounded at 4,000,000 and input lookups at 4096.
func (mp *TxPool) EntrySnapshot(id *chainhash.Hash) (*btcjson.GetMempoolEntrySnapshotResult, error) {
	if mp == nil || id == nil {
		return nil, ErrEntryUnavailable
	}
	if !mp.mtx.TryRLock() {
		return nil, ErrEntryBusy
	}
	defer mp.mtx.RUnlock()
	desc, ok := mp.pool[*id]
	if !ok {
		return nil, ErrEntryNotFound
	}
	if desc == nil || desc.Tx == nil {
		return nil, ErrEntryUnavailable
	}
	tx := desc.Tx.MsgTx()
	if len(tx.TxIn) > 4096 {
		return nil, ErrEntryCapacity
	}
	size := tx.SerializeSize()
	if size > 4000000 {
		return nil, ErrEntryCapacity
	}
	if desc.Height < 0 || desc.Fee < 0 {
		return nil, ErrEntryUnavailable
	}
	weight := blockchain.GetTransactionWeight(desc.Tx)
	entry := &btcjson.GetMempoolEntrySnapshotResult{
		Size: int32(size), Vsize: int32(GetTxVirtualSize(desc.Tx)), Weight: weight,
		Height: int64(desc.Height), Wtxid: tx.WitnessHash().String(),
		Time: desc.Added.Unix(), Fee: btcutil.Amount(desc.Fee).ToBTC(),
		Depends: make([]string, 0),
	}
	seen := make(map[chainhash.Hash]bool, len(tx.TxIn))
	for _, input := range tx.TxIn {
		parent := input.PreviousOutPoint.Hash
		if _, present := mp.pool[parent]; present && !seen[parent] {
			entry.Depends = append(entry.Depends, parent.String())
			seen[parent] = true
		}
	}
	return entry, nil
}
