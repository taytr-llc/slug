// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterownership

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"

	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/fightertransfer"
	"github.com/taytr-llc/slug/wire/v2"
)

// blockReader walks CompactSize framing before any count-derived allocation.
// Transaction spans borrow one bounded complete block; scripts are never cloned.
type blockReader struct {
	raw []byte
	pos int
	err error
}

func (r *blockReader) take(n uint64) []byte {
	if r.err != nil {
		return nil
	}
	if n > uint64(len(r.raw)-r.pos) {
		r.err = ErrCorrupt
		return nil
	}
	b := r.raw[r.pos : r.pos+int(n)]
	r.pos += int(n)
	return b
}
func (r *blockReader) num(n int) uint64 {
	b := r.take(uint64(n))
	if r.err != nil {
		return 0
	}
	switch n {
	case 1:
		return uint64(b[0])
	case 2:
		return uint64(binary.LittleEndian.Uint16(b))
	case 4:
		return uint64(binary.LittleEndian.Uint32(b))
	case 8:
		return binary.LittleEndian.Uint64(b)
	}
	r.err = ErrCorrupt
	return 0
}
func (r *blockReader) count() uint64 {
	n := r.num(1)
	if n < 253 {
		return n
	}
	width, min := 2, uint64(253)
	if n == 254 {
		width, min = 4, 65536
	}
	if n == 255 {
		width, min = 8, 1<<32
	}
	n = r.num(width)
	if n < min {
		r.err = ErrCorrupt
	}
	return n
}
func (r *blockReader) blob() { r.take(r.count()) }

type parsedBlockTx struct {
	raw       []byte
	id        chainhash.Hash
	discovery *fightertransfer.Discovery
	merkle    []string
}

func walkTx(ctx context.Context, r *blockReader) ([]byte, chainhash.Hash, error) {
	start := r.pos
	r.take(4)
	wit := false
	if r.err == nil && r.pos < len(r.raw) && r.raw[r.pos] == 0 {
		wit = true
		if r.num(1) != 0 || r.num(1) != 1 {
			return nil, chainhash.Hash{}, ErrCorrupt
		}
	}
	main := r.pos
	inputs := r.count()
	if r.err != nil || inputs == 0 || inputs > uint64((len(r.raw)-r.pos)/41) {
		return nil, chainhash.Hash{}, ErrCorrupt
	}
	for i := uint64(0); i < inputs; i++ {
		if i%128 == 0 {
			if e := ctx.Err(); e != nil {
				return nil, chainhash.Hash{}, e
			}
		}
		r.take(36)
		r.blob()
		r.take(4)
	}
	outputs := r.count()
	if r.err != nil || outputs == 0 || outputs > uint64((len(r.raw)-r.pos)/9) {
		return nil, chainhash.Hash{}, ErrCorrupt
	}
	var total int64
	for i := uint64(0); i < outputs; i++ {
		if i%128 == 0 {
			if e := ctx.Err(); e != nil {
				return nil, chainhash.Hash{}, e
			}
		}
		atoms := int64(r.num(8))
		if atoms < 0 || atoms > 21000000*100000000-total {
			return nil, chainhash.Hash{}, ErrCorrupt
		}
		total += atoms
		r.blob()
	}
	mainEnd := r.pos
	hasWitness := false
	if wit {
		for i := uint64(0); i < inputs; i++ {
			n := r.count()
			if r.err != nil || n > uint64(len(r.raw)-r.pos) {
				return nil, chainhash.Hash{}, ErrCorrupt
			}
			if n > 0 {
				hasWitness = true
			}
			for j := uint64(0); j < n; j++ {
				if j%128 == 0 {
					if e := ctx.Err(); e != nil {
						return nil, chainhash.Hash{}, e
					}
				}
				r.blob()
			}
		}
		if !hasWitness {
			return nil, chainhash.Hash{}, ErrCorrupt
		}
	}
	lock := r.pos
	r.take(4)
	if r.err != nil {
		return nil, chainhash.Hash{}, r.err
	}
	raw := r.raw[start:r.pos]
	if !wit {
		return raw, chainhash.DoubleHashH(raw), nil
	}
	h := sha256.New()
	h.Write(r.raw[start : start+4])
	h.Write(r.raw[main:mainEnd])
	h.Write(r.raw[lock:r.pos])
	first := h.Sum(nil)
	return raw, chainhash.Hash(sha256.Sum256(first)), nil
}
func parseBlock(ctx context.Context, raw []byte, expected HeaderRow) ([]parsedBlockTx, error) {
	if ctx == nil || len(raw) < 81 || len(raw) > 4000000 {
		return nil, ErrCapacity
	}
	header, _ := hex.DecodeString(expected.Header)
	if !bytes.Equal(raw[:80], header) {
		return nil, ErrCorrupt
	}
	r := blockReader{raw: raw, pos: 80}
	count := r.count()
	if r.err != nil || count == 0 || count > 1000000 || count > uint64((len(raw)-r.pos)/10) {
		return nil, ErrCorrupt
	}
	txs := make([]parsedBlockTx, 0, int(count))
	ids := make([]chainhash.Hash, 0, int(count))
	seen := map[chainhash.Hash]bool{}
	for n := uint64(0); n < count; n++ {
		if e := ctx.Err(); e != nil {
			return nil, e
		}
		b, id, e := walkTx(ctx, &r)
		if e != nil {
			return nil, e
		}
		if seen[id] {
			return nil, ErrCorrupt
		}
		seen[id] = true
		item := parsedBlockTx{raw: b, id: id}
		d, e := fightertransfer.Discover(ctx, b)
		if e == nil {
			item.discovery = &d
		} else if !errors.Is(e, fightertransfer.ErrNotCarrier) {
			return nil, e
		}
		txs = append(txs, item)
		ids = append(ids, id)
	}
	if r.pos != len(raw) {
		return nil, ErrCorrupt
	}
	tree, e := blockMerkleTree(ctx, ids)
	if e != nil {
		return nil, e
	}
	root := tree[len(tree)-1][0]
	recognized := 0
	for n := range txs {
		if txs[n].discovery == nil {
			continue
		}
		recognized++
		if recognized > 8192 {
			return nil, ErrCapacity
		}
		if e = ctx.Err(); e != nil {
			return nil, e
		}
		index := n
		branch := make([]string, 0, len(tree)-1)
		for _, level := range tree[:len(tree)-1] {
			other := index ^ 1
			if other >= len(level) {
				other = index
			}
			branch = append(branch, level[other].String())
			index /= 2
		}
		txs[n].merkle = branch
	}
	var h wire.BlockHeader
	if h.Deserialize(bytes.NewReader(header)) != nil || root != h.MerkleRoot {
		return nil, ErrCorrupt
	}
	return txs, nil
}
func blockMerkleTree(ctx context.Context, ids []chainhash.Hash) ([][]chainhash.Hash, error) {
	if ctx == nil || len(ids) == 0 {
		return nil, ErrInvalid
	}
	tree := [][]chainhash.Hash{ids}
	for len(tree[len(tree)-1]) > 1 {
		level := tree[len(tree)-1]
		next := make([]chainhash.Hash, 0, (len(level)+1)/2)
		for i := 0; i < len(level); i += 2 {
			if i%128 == 0 {
				if e := ctx.Err(); e != nil {
					return nil, e
				}
			}
			j := i + 1
			if j == len(level) {
				j = i
			}
			var pair [64]byte
			copy(pair[:32], level[i][:])
			copy(pair[32:], level[j][:])
			next = append(next, chainhash.DoubleHashH(pair[:]))
		}
		tree = append(tree, next)
	}
	if e := ctx.Err(); e != nil {
		return nil, e
	}
	return tree, nil
}
func transactionWitness(h HeaderRow, txs []parsedBlockTx, index int) TransactionWitness {
	return TransactionWitness{h.Height, h.Hash, uint32(index), uint32(len(txs)), txs[index].id.String(), hex.EncodeToString(txs[index].raw), append([]string{}, txs[index].merkle...)}
}

func checkTransactionWitness(ctx context.Context, tx TransactionWitness, h HeaderRow) error {
	raw, _ := hex.DecodeString(tx.Raw)
	r := blockReader{raw: raw}
	_, id, e := walkTx(ctx, &r)
	if e != nil || r.pos != len(raw) || id.String() != tx.TxID {
		return ErrCorrupt
	}
	index, count := tx.TransactionIndex, tx.TransactionCount
	hash := id
	used := 0
	for count > 1 {
		if used >= len(tx.MerkleBranch) {
			return ErrCorrupt
		}
		sibling, e := chainhash.NewHashFromStr(tx.MerkleBranch[used])
		if e != nil {
			return ErrCorrupt
		}
		if index == count-1 && count%2 == 1 && *sibling != hash {
			return ErrCorrupt
		}
		var pair [64]byte
		if index%2 == 0 {
			copy(pair[:32], hash[:])
			copy(pair[32:], sibling[:])
		} else {
			copy(pair[:32], sibling[:])
			copy(pair[32:], hash[:])
		}
		hash = chainhash.DoubleHashH(pair[:])
		used++
		index /= 2
		count = (count + 1) / 2
	}
	if used != len(tx.MerkleBranch) {
		return ErrCorrupt
	}
	headerRaw, _ := hex.DecodeString(h.Header)
	var header wire.BlockHeader
	if header.Deserialize(bytes.NewReader(headerRaw)) != nil || hash != header.MerkleRoot {
		return ErrCorrupt
	}
	return ctx.Err()
}
