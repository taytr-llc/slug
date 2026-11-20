// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"math"

	"github.com/taytr-llc/slug/address/v2"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/gymstable"
	"github.com/taytr-llc/slug/poolsettle"
	"github.com/taytr-llc/slug/wire/v2"
	"github.com/taytr-llc/slugwallet/wallet"
)

// Retain only owned raw records and public summaries. Decoded transactions and
// output descriptions belong to one bounded projection, never the cache.
type walletActivityCache struct {
	job     WalletActivityJob
	records []wallet.OwnActivityRecord
	items   []WalletActivityItem
}

// Framing is checked before MsgTx allocates slices from untrusted counts. These
// are local display bounds, not transaction validity or consensus rules.
func walletActivityTx(ctx context.Context, raw []byte) (*wire.MsgTx, error) {
	if ctx == nil {
		return nil, gymstable.ErrInvalid
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	if len(raw) > 262144 {
		return nil, gymstable.ErrCapacity
	}
	p := 0
	take := func(n uint64) bool {
		if n > uint64(len(raw)-p) {
			return false
		}
		p += int(n)
		return true
	}
	compact := func() (uint64, error) {
		if e := ctx.Err(); e != nil {
			return 0, e
		}
		if p >= len(raw) {
			return 0, ErrActivityUnavailable
		}
		b := raw[p]
		p++
		var n uint64
		switch b {
		case 253:
			if !take(2) {
				return 0, ErrActivityUnavailable
			}
			n = uint64(binary.LittleEndian.Uint16(raw[p-2 : p]))
			if n < 253 {
				return 0, ErrActivityUnavailable
			}
		case 254:
			if !take(4) {
				return 0, ErrActivityUnavailable
			}
			n = uint64(binary.LittleEndian.Uint32(raw[p-4 : p]))
			if n <= math.MaxUint16 {
				return 0, ErrActivityUnavailable
			}
		case 255:
			if !take(8) {
				return 0, ErrActivityUnavailable
			}
			n = binary.LittleEndian.Uint64(raw[p-8 : p])
			if n <= math.MaxUint32 {
				return 0, ErrActivityUnavailable
			}
		default:
			n = uint64(b)
		}
		return n, nil
	}
	if !take(4) {
		return nil, ErrActivityUnavailable
	}
	witness := p < len(raw) && raw[p] == 0
	if witness {
		if !take(2) || raw[p-1] != 1 {
			return nil, ErrActivityUnavailable
		}
	}
	ni, e := compact()
	if e != nil {
		return nil, e
	}
	if ni > 256 {
		return nil, gymstable.ErrCapacity
	}
	if ni == 0 {
		return nil, ErrActivityUnavailable
	}
	for i := uint64(0); i < ni; i++ {
		if !take(36) {
			return nil, ErrActivityUnavailable
		}
		n, e := compact()
		if e != nil {
			return nil, e
		}
		if n > 10240 {
			return nil, gymstable.ErrCapacity
		}
		if !take(n) || !take(4) {
			return nil, ErrActivityUnavailable
		}
	}
	no, e := compact()
	if e != nil {
		return nil, e
	}
	if no > 256 {
		return nil, gymstable.ErrCapacity
	}
	if no == 0 {
		return nil, ErrActivityUnavailable
	}
	var outputTotal int64
	for i := uint64(0); i < no; i++ {
		if !take(8) {
			return nil, ErrActivityUnavailable
		}
		amount := int64(binary.LittleEndian.Uint64(raw[p-8 : p]))
		if amount < 0 || amount > poolsettle.MaxMoney-outputTotal {
			return nil, ErrActivityUnavailable
		}
		outputTotal += amount
		n, e := compact()
		if e != nil {
			return nil, e
		}
		if n > 10240 {
			return nil, gymstable.ErrCapacity
		}
		if !take(n) {
			return nil, ErrActivityUnavailable
		}
	}
	if witness {
		total := uint64(0)
		for i := uint64(0); i < ni; i++ {
			n, e := compact()
			if e != nil {
				return nil, e
			}
			if n > 128 || total+n > 4096 {
				return nil, gymstable.ErrCapacity
			}
			total += n
			for j := uint64(0); j < n; j++ {
				s, e := compact()
				if e != nil {
					return nil, e
				}
				if s > 262144 {
					return nil, gymstable.ErrCapacity
				}
				if !take(s) {
					return nil, ErrActivityUnavailable
				}
			}
		}
		if total == 0 {
			return nil, ErrActivityUnavailable
		}
	}
	if !take(4) || p != len(raw) {
		return nil, ErrActivityUnavailable
	}
	var tx wire.MsgTx
	if tx.Deserialize(bytes.NewReader(raw)) != nil {
		return nil, ErrActivityUnavailable
	}
	var encoded bytes.Buffer
	if tx.Serialize(&encoded) != nil || !bytes.Equal(encoded.Bytes(), raw) {
		return nil, ErrActivityUnavailable
	}
	seen := make(map[wire.OutPoint]bool, len(tx.TxIn))
	for _, in := range tx.TxIn {
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		if seen[in.PreviousOutPoint] {
			return nil, ErrActivityUnavailable
		}
		seen[in.PreviousOutPoint] = true
		if in.PreviousOutPoint.Hash == (chainhash.Hash{}) && in.PreviousOutPoint.Index == math.MaxUint32 && len(tx.TxIn) != 1 {
			return nil, ErrActivityUnavailable
		}
	}
	return &tx, nil
}

func walletActivityAddress(script []byte) string {
	var a address.Address
	var e error
	switch {
	case len(script) == 25 && bytes.Equal(script[:3], []byte{0x76, 0xa9, 0x14}) && bytes.Equal(script[23:], []byte{0x88, 0xac}):
		a, e = address.NewAddressPubKeyHash(script[3:23], &chaincfg.MainNetParams)
	case len(script) == 23 && bytes.Equal(script[:2], []byte{0xa9, 0x14}) && script[22] == 0x87:
		a, e = address.NewAddressScriptHashFromHash(script[2:22], &chaincfg.MainNetParams)
	case len(script) == 22 && script[0] == 0 && script[1] == 20:
		a, e = address.NewAddressWitnessPubKeyHash(script[2:], &chaincfg.MainNetParams)
	case len(script) == 34 && script[0] == 0 && script[1] == 32:
		a, e = address.NewAddressWitnessScriptHash(script[2:], &chaincfg.MainNetParams)
	case len(script) == 34 && script[0] == 0x51 && script[1] == 32:
		a, e = address.NewAddressTaproot(script[2:], &chaincfg.MainNetParams)
	default:
		return ""
	}
	if e != nil || a == nil {
		return ""
	}
	return a.EncodeAddress()
}

func projectWalletActivity(ctx context.Context, record wallet.OwnActivityRecord, capture WalletActivityCapture, detail bool) (WalletActivityItem, []WalletActivityOutput, error) {
	var empty WalletActivityItem
	tx, e := walletActivityTx(ctx, record.Raw)
	if e != nil {
		return empty, nil, e
	}
	if tx.TxHash() != record.TxID || record.Credits == nil || record.Debits == nil || len(record.Credits) > len(tx.TxOut) || len(record.Debits) > len(tx.TxIn) {
		return empty, nil, ErrActivityUnavailable
	}
	digest := sha256.Sum256(record.Raw)
	item := WalletActivityItem{TxID: record.TxID.String(), RawSHA256: hex.EncodeToString(digest[:]), RecordedAtMS: record.RecordedAtMS, InputCount: uint32(len(tx.TxIn)), OutputCount: uint32(len(tx.TxOut)), OwnedInputCount: uint32(len(record.Debits)), OwnedOutputCount: uint32(len(record.Credits))}
	item.Coinbase = len(tx.TxIn) == 1 && tx.TxIn[0].PreviousOutPoint.Hash == (chainhash.Hash{}) && tx.TxIn[0].PreviousOutPoint.Index == math.MaxUint32
	if record.Block != nil {
		item.Block = &WalletActivityBlock{Height: record.Block.Height, Hash: record.Block.Hash.String(), TimeMS: record.Block.TimeMS}
		if record.Block.Height > capture.WalletHeight {
			return empty, nil, ErrActivityUnavailable
		}
		item.Confirmations = capture.WalletHeight - record.Block.Height + 1
	}
	var total int64
	for _, o := range tx.TxOut {
		if e := ctx.Err(); e != nil {
			return empty, nil, e
		}
		if o.Value < 0 || o.Value > poolsettle.MaxMoney-total {
			return empty, nil, ErrActivityUnavailable
		}
		total += o.Value
	}
	for i, d := range record.Debits {
		if e := ctx.Err(); e != nil {
			return empty, nil, e
		}
		if d.Index >= item.InputCount || i > 0 && record.Debits[i-1].Index >= d.Index || d.Atoms < 0 || d.Atoms > poolsettle.MaxMoney-item.SpentAtoms {
			return empty, nil, ErrActivityUnavailable
		}
		item.SpentAtoms += d.Atoms
	}
	for i, c := range record.Credits {
		if e := ctx.Err(); e != nil {
			return empty, nil, e
		}
		if c.Index >= item.OutputCount || i > 0 && record.Credits[i-1].Index >= c.Index || c.Atoms != tx.TxOut[c.Index].Value || c.Atoms < 0 || c.Atoms > poolsettle.MaxMoney-item.ReceivedAtoms {
			return empty, nil, ErrActivityUnavailable
		}
		item.ReceivedAtoms += c.Atoms
		if c.Change {
			item.ChangeAtoms += c.Atoms
		}
	}
	item.NetAtoms = item.ReceivedAtoms - item.SpentAtoms
	if !item.Coinbase && item.OwnedInputCount == item.InputCount {
		fee := item.SpentAtoms - total
		if fee < 0 {
			return empty, nil, ErrActivityUnavailable
		}
		item.FeeAtoms = &fee
	}
	if ValidateWalletActivityItem(item, capture) != nil {
		return empty, nil, ErrActivityUnavailable
	}
	var outputs []WalletActivityOutput
	if detail {
		outputs = make([]WalletActivityOutput, len(tx.TxOut))
		ci := 0
		for i, o := range tx.TxOut {
			if e := ctx.Err(); e != nil {
				return empty, nil, e
			}
			out := WalletActivityOutput{Index: uint32(i), Atoms: o.Value, Address: walletActivityAddress(o.PkScript)}
			if ci < len(record.Credits) && record.Credits[ci].Index == uint32(i) {
				out.Owned, out.Change = true, record.Credits[ci].Change
				ci++
			}
			outputs[i] = out
		}
	}
	return item, outputs, nil
}

// The wallet API transfers ownership of this bounded snapshot. No whole raw
// capture copy is made here or in the read/status paths.
func buildWalletActivityCache(ctx context.Context, job WalletActivityJob, source walletActivityRead) (*walletActivityCache, error) {
	if ctx == nil {
		return nil, gymstable.ErrInvalid
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	s := source.snapshot
	if s.Version != 1 || s.RootPublicKey != job.Binding.RootPublicKey || s.GymPublicKey != job.Binding.GymPublicKey || s.GymID != job.Binding.GymID || s.Records == nil || len(s.Records) > 256 {
		return nil, ErrActivityUnavailable
	}
	meta := WalletActivityCapture{Version: 1, CapturedAtMS: source.at, WalletHeight: s.SyncedHeight, WalletTip: s.SyncedHash.String(), NodeHeight: s.SyncedHeight, NodeTip: s.SyncedHash.String(), Count: uint32(len(s.Records)), HasOlder: s.HasOlder}
	if ValidateWalletActivityCapture(meta) != nil {
		return nil, ErrActivityUnavailable
	}
	cache := &walletActivityCache{records: s.Records, items: make([]WalletActivityItem, 0, len(s.Records))}
	total := 0
	seen := make(map[chainhash.Hash]bool, len(s.Records))
	for _, record := range s.Records {
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		if len(record.Raw) > 8388608-total {
			return nil, gymstable.ErrCapacity
		}
		total += len(record.Raw)
		if seen[record.TxID] {
			return nil, ErrActivityUnavailable
		}
		seen[record.TxID] = true
		item, _, e := projectWalletActivity(ctx, record, meta, false)
		if e != nil {
			return nil, e
		}
		if len(cache.items) > 0 && !WalletActivityItemLess(cache.items[len(cache.items)-1], item) {
			return nil, ErrActivityUnavailable
		}
		cache.items = append(cache.items, item)
	}
	summary, e := json.Marshal(cache.items)
	if e != nil || len(summary) > 524288 {
		return nil, gymstable.ErrCapacity
	}
	job.State, job.Capture = "done", &meta
	cache.job = job
	return cache, nil
}
