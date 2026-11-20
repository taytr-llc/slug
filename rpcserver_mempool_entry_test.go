// Copyright (c) 2026 The SLUG developers. ISC license.
package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/btcjson"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/mempool"
	"github.com/taytr-llc/slug/mempool/mempooltest"
)

// This transport-only model embeds the inherited mock; any accidental
// whole-pool/transaction fallback is an unexpected mock call and fails.
type mempoolEntryRPCModel struct {
	*mempooltest.MockTxMempool
	id     chainhash.Hash
	calls  int
	err    error
	closed chan struct{}
}

func (m *mempoolEntryRPCModel) EntrySnapshot(id *chainhash.Hash) (*btcjson.GetMempoolEntrySnapshotResult, error) {
	m.calls++
	m.id = *id
	if m.closed != nil {
		close(m.closed)
	}
	if m.err != nil {
		return nil, m.err
	}
	return &btcjson.GetMempoolEntrySnapshotResult{Size: 150, Vsize: 145, Weight: 579, Height: 101, Wtxid: strings.Repeat("2", 64), Time: 123456, Fee: 0.0001, Depends: []string{}}, nil
}

func TestGetMempoolEntryBoundedRPC(t *testing.T) {
	id := strings.Repeat("ab", 32)
	var request btcjson.Request
	if e := json.Unmarshal([]byte(`{"jsonrpc":"1.0","id":1,"method":"getmempoolentry","params":["`+id+`"]}`), &request); e != nil {
		t.Fatal(e)
	}
	cmd, e := btcjson.UnmarshalCmd(&request)
	if e != nil {
		t.Fatal(e)
	}
	pool := &mempoolEntryRPCModel{MockTxMempool: &mempooltest.MockTxMempool{}}
	s := &rpcServer{cfg: rpcserverConfig{TxMemPool: pool}}
	got, e := handleGetMempoolEntry(s, cmd, nil)
	if e != nil || pool.calls != 1 || pool.id.String() != id {
		t.Fatal("exact command lookup", got, e)
	}
	entry, ok := got.(*btcjson.GetMempoolEntrySnapshotResult)
	if !ok || entry.Size != 150 || entry.Vsize != 145 || entry.Weight != 579 || entry.Wtxid != strings.Repeat("2", 64) {
		t.Fatal("result projection", got)
	}
	for _, test := range []struct {
		name string
		err  error
		code btcjson.RPCErrorCode
	}{
		{"absent", mempool.ErrEntryNotFound, btcjson.ErrRPCInvalidAddressOrKey},
		{"busy", mempool.ErrEntryBusy, btcjson.ErrRPCInternal.Code},
		{"capacity", mempool.ErrEntryCapacity, btcjson.ErrRPCInternal.Code},
	} {
		t.Run(test.name, func(t *testing.T) {
			pool.err = test.err
			got, e := handleGetMempoolEntry(s, cmd, nil)
			rpc, ok := e.(*btcjson.RPCError)
			if got != nil || !ok || rpc.Code != test.code {
				t.Fatal(got, e)
			}
		})
	}
	pool.err = nil
	before := pool.calls
	for _, bad := range []string{"1", strings.ToUpper(id), strings.Repeat("g", 64)} {
		_, e = handleGetMempoolEntry(s, btcjson.NewGetMempoolEntryCmd(bad), nil)
		rpc, ok := e.(*btcjson.RPCError)
		if !ok || rpc.Code != btcjson.ErrRPCInvalidParameter {
			t.Fatal("bad ID", bad, e)
		}
	}
	if pool.calls != before {
		t.Fatal("malformed ID reached pool")
	}
	closed := make(chan struct{})
	close(closed)
	if got, e = handleGetMempoolEntry(s, cmd, closed); got != nil || e == nil || pool.calls != before {
		t.Fatal("closed request read pool", got, e)
	}
	pool.closed = make(chan struct{})
	if got, e = handleGetMempoolEntry(s, cmd, pool.closed); got != nil || e == nil {
		t.Fatal("disconnect returned entry", got, e)
	}
	// The real empty pool also returns the exact documented missing code.
	s.cfg.TxMemPool = mempool.New(&mempool.Config{})
	if _, e = handleGetMempoolEntry(s, cmd, nil); e == nil || e.(*btcjson.RPCError).Code != -5 {
		t.Fatal(e)
	}
	if _, ok = rpcUnimplemented["getmempoolentry"]; ok || rpcHandlers["getmempoolentry"] == nil {
		t.Fatal("handler unavailable")
	}
	if _, ok = rpcLimited["getmempoolentry"]; !ok {
		t.Fatal("read method missing from limited RPC")
	}
	if _, e = newHelpCacher().rpcMethodHelp("getmempoolentry"); e != nil {
		t.Fatal(e)
	}
}
