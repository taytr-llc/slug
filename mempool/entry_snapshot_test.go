// Copyright (c) 2026 The SLUG developers. ISC license.
package mempool

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/taytr-llc/slug/btcutil/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/mining"
	"github.com/taytr-llc/slug/wire/v2"
)

func TestMempoolEntrySnapshotExactOwnedProjection(t *testing.T) {
	// Independently assembled three-input witness transaction: 143 base
	// bytes, 150 total bytes, weight 579, virtual size 145. Two inputs use
	// one pooled parent; the third uses an orphan-only parent. This is a
	// private store fixture, not a claim of consensus/relay acceptance.
	input := func(parent, index string) string { return strings.Repeat(parent, 32) + index + "000000" + "00ffffffff" }
	raw, e := hex.DecodeString("01000000000103" + input("11", "00") + input("11", "01") + input("22", "00") + "0101000000000000000151" + "0101aa0000" + "00000000")
	if e != nil || len(raw) != 150 {
		t.Fatal("independent frame", len(raw), e)
	}
	var msg wire.MsgTx
	if e = msg.Deserialize(bytes.NewReader(raw)); e != nil {
		t.Fatal(e)
	}
	tx := btcutil.NewTx(&msg)
	parentA := msg.TxIn[0].PreviousOutPoint.Hash
	parentB := msg.TxIn[2].PreviousOutPoint.Hash
	unrelated := chainhash.Hash{3}
	mp := &TxPool{
		pool: map[chainhash.Hash]*TxDesc{
			*tx.Hash(): {TxDesc: mining.TxDesc{Tx: tx, Height: 101, Added: time.Unix(123456, 0), Fee: 10000}},
			parentA:    {}, unrelated: nil,
		},
		orphans: map[chainhash.Hash]*orphanTx{parentB: {}},
		cfg:     Config{BestHeight: func() int32 { t.Fatal("entry called chain height"); return 0 }},
	}
	entry, e := mp.EntrySnapshot(tx.Hash())
	if e != nil {
		t.Fatal(e)
	}
	witnessHash := sha256.Sum256(raw)
	witnessHash = sha256.Sum256(witnessHash[:])
	for i, j := 0, 31; i < j; i, j = i+1, j-1 {
		witnessHash[i], witnessHash[j] = witnessHash[j], witnessHash[i]
	}
	if entry.Size != 150 || entry.Vsize != 145 || entry.Weight != 579 || entry.Wtxid != hex.EncodeToString(witnessHash[:]) || entry.Height != 101 || entry.Time != 123456 || entry.Fee != 0.0001 || !reflect.DeepEqual(entry.Depends, []string{strings.Repeat("11", 32)}) {
		t.Fatalf("incorrect exact entry: %+v", entry)
	}
	encoded, e := json.Marshal(entry)
	if e != nil {
		t.Fatal(e)
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(encoded, &fields) != nil || len(fields) != 8 {
		t.Fatal("unsupported fields", string(encoded))
	}
	for _, k := range []string{"size", "vsize", "weight", "height", "wtxid", "time", "fee", "depends"} {
		if _, ok := fields[k]; !ok {
			t.Fatal("missing field", k)
		}
	}
	entry.Depends[0] = "changed"
	again, e := mp.EntrySnapshot(tx.Hash())
	if e != nil || again.Depends[0] != strings.Repeat("11", 32) {
		t.Fatal("returned slice aliases pool", e)
	}
	delete(mp.pool, parentA)
	again, e = mp.EntrySnapshot(tx.Hash())
	if e != nil || again.Depends == nil || len(again.Depends) != 0 {
		t.Fatal("direct parents not sampled", e)
	}
	if entry.Height != 101 {
		t.Fatal("retained snapshot changed")
	}
}

func TestMempoolEntrySnapshotBoundedRefusals(t *testing.T) {
	mp := New(&Config{})
	id := chainhash.Hash{1}
	if _, e := mp.EntrySnapshot(&id); !errors.Is(e, ErrEntryNotFound) {
		t.Fatal(e)
	}
	mp.mtx.Lock()
	_, e := mp.EntrySnapshot(&id)
	mp.mtx.Unlock()
	if !errors.Is(e, ErrEntryBusy) {
		t.Fatal("busy read waited/accepted", e)
	}
	if _, e = mp.EntrySnapshot(nil); !errors.Is(e, ErrEntryUnavailable) {
		t.Fatal(e)
	}
	var missing *TxPool
	if _, e = missing.EntrySnapshot(&id); !errors.Is(e, ErrEntryUnavailable) {
		t.Fatal(e)
	}
	msg := wire.NewMsgTx(1)
	msg.TxIn = make([]*wire.TxIn, 4097)
	mp.pool[id] = &TxDesc{TxDesc: mining.TxDesc{Tx: btcutil.NewTx(msg)}}
	if _, e = mp.EntrySnapshot(&id); !errors.Is(e, ErrEntryCapacity) {
		t.Fatal("input cap before walk", e)
	}
	msg = wire.NewMsgTx(1)
	msg.AddTxOut(wire.NewTxOut(1, make([]byte, 4000000)))
	mp.pool[id] = &TxDesc{TxDesc: mining.TxDesc{Tx: btcutil.NewTx(msg)}}
	if _, e = mp.EntrySnapshot(&id); !errors.Is(e, ErrEntryCapacity) {
		t.Fatal("byte cap", e)
	}
}
