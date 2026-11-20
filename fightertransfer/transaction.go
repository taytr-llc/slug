// Copyright (c) 2026 The SLUG developers. ISC license.
package fightertransfer

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"

	"github.com/taytr-llc/slug/chainhash/v2"
	"github.com/taytr-llc/slug/wire/v2"
)

type rawReader struct {
	raw []byte
	pos int
	err error
}

func (r *rawReader) take(n uint64) []byte {
	if r.err != nil {
		return nil
	}
	if n > uint64(len(r.raw)-r.pos) {
		r.err = invalid("truncated transaction")
		return nil
	}
	b := r.raw[r.pos : r.pos+int(n)]
	r.pos += int(n)
	return b
}
func (r *rawReader) number(n uint64) uint64 {
	b := r.take(n)
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
	r.err = invalid("integer width")
	return 0
}
func (r *rawReader) count() uint64 {
	tag := r.number(1)
	if tag < 253 {
		return tag
	}
	n := uint64(2)
	minimum := uint64(253)
	if tag == 254 {
		n = 4
		minimum = 65536
	}
	if tag == 255 {
		n = 8
		minimum = 1 << 32
	}
	value := r.number(n)
	if r.err == nil && value < minimum {
		r.err = invalid("nonminimal CompactSize")
	}
	return value
}
func (r *rawReader) blob(limit uint64) []byte {
	n := r.count()
	if r.err == nil && n > limit {
		r.err = invalid("script or witness item size")
	}
	return r.take(n)
}

type transactionMode uint8

const (
	parentMode transactionMode = iota
	commitMode
	spendMode
)

type parsedTransaction struct {
	tx         *wire.MsgTx
	hash       chainhash.Hash
	outputs    map[uint32]*wire.TxOut // Only the at-most16 selected indices, borrowed script slices.
	inputCount uint64
	firstInput wire.OutPoint
}

// scanTransaction prewalks all framing before retaining any variable-sized
// lists. Parent lists/witnesses are walked without allocation; irrelevant script
// bytes are skipped within the full containing transaction, not an executing
// script limit. Current carrier arrays are separately bounded to16 inputs/3 outputs.
func scanTransaction(ctx context.Context, raw []byte, mode transactionMode, wanted map[uint32]bool, observe ...func(bool, []byte)) (parsedTransaction, error) {
	var result parsedTransaction
	if ctx == nil {
		return result, invalid("nil context")
	}
	if e := ctx.Err(); e != nil {
		return result, e
	}
	if len(raw) > MaxRawSize {
		return result, fmt.Errorf("%w: transaction bytes", ErrCapacity)
	}
	if len(raw) < 10 {
		return result, invalid("transaction size")
	}
	r := rawReader{raw: raw}
	version := int32(r.number(4))
	witness := false
	if r.pos < len(raw) && raw[r.pos] == 0 {
		witness = true
		if r.number(1) != 0 || r.number(1) != 1 {
			return result, invalid("witness flags")
		}
	}
	mainStart := r.pos
	n := r.count()
	if r.err != nil {
		return result, r.err
	}
	if n == 0 || n > uint64((len(raw)-r.pos)/41) {
		return result, invalid("input count")
	}
	result.inputCount = n
	if mode == commitMode && n > MaxInputs || mode == spendMode && n != 1 {
		return result, invalid("carrier input count")
	}
	var tx *wire.MsgTx
	if mode != parentMode {
		tx = &wire.MsgTx{Version: version, TxIn: make([]*wire.TxIn, int(n))}
	}
	for i := uint64(0); i < n; i++ {
		if i%256 == 0 {
			if e := ctx.Err(); e != nil {
				return result, e
			}
		}
		hash := r.take(32)
		index := uint32(r.number(4))
		limit := uint64(len(raw))
		if mode != parentMode {
			limit = MaxScriptSize
		}
		script := r.blob(limit)
		sequence := uint32(r.number(4))
		if r.err != nil {
			return result, r.err
		}
		if i == 0 {
			copy(result.firstInput.Hash[:], hash)
			result.firstInput.Index = index
		}
		if len(observe) != 0 {
			observe[0](true, script)
		}
		if tx != nil {
			in := &wire.TxIn{PreviousOutPoint: wire.OutPoint{Index: index}, SignatureScript: script, Sequence: sequence}
			copy(in.PreviousOutPoint.Hash[:], hash)
			tx.TxIn[i] = in
		}
	}
	outCount := r.count()
	if r.err != nil {
		return result, r.err
	}
	if outCount == 0 || outCount > uint64((len(raw)-r.pos)/9) {
		return result, invalid("output count")
	}
	if mode == commitMode && outCount != 3 || mode == spendMode && outCount > 2 {
		return result, invalid("carrier output count")
	}
	if tx != nil {
		tx.TxOut = make([]*wire.TxOut, int(outCount))
	} else {
		result.outputs = make(map[uint32]*wire.TxOut, len(wanted))
	}
	var total int64
	for i := uint64(0); i < outCount; i++ {
		if i%256 == 0 {
			if e := ctx.Err(); e != nil {
				return result, e
			}
		}
		value := r.number(8)
		limit := uint64(len(raw))
		if mode != parentMode {
			limit = MaxScriptSize
		}
		script := r.blob(limit)
		if r.err != nil {
			return result, r.err
		}
		if value > uint64(maxMoney) || int64(value) > maxMoney-total {
			return result, invalid("output value")
		}
		total += int64(value)
		if len(observe) != 0 {
			observe[0](false, script)
		}
		if tx != nil {
			tx.TxOut[i] = &wire.TxOut{Value: int64(value), PkScript: script}
		} else if wanted[uint32(i)] {
			result.outputs[uint32(i)] = &wire.TxOut{Value: int64(value), PkScript: script}
		}
	}
	mainEnd := r.pos
	if witness {
		any := false
		for i := uint64(0); i < n; i++ {
			if i%256 == 0 {
				if e := ctx.Err(); e != nil {
					return result, e
				}
			}
			items := r.count()
			if r.err != nil {
				return result, r.err
			}
			if items > uint64(len(raw)-r.pos) {
				return result, invalid("witness count")
			}
			if mode != parentMode && items > 2 {
				return result, invalid("carrier witness count")
			}
			any = any || items != 0
			if tx != nil {
				tx.TxIn[i].Witness = make(wire.TxWitness, int(items))
			}
			for j := uint64(0); j < items; j++ {
				if j%256 == 0 {
					if e := ctx.Err(); e != nil {
						return result, e
					}
				}
				limit := uint64(len(raw))
				if mode != parentMode {
					limit = 520
				}
				item := r.blob(limit)
				if r.err != nil {
					return result, r.err
				}
				if tx != nil {
					tx.TxIn[i].Witness[j] = item
				}
			}
		}
		if !any {
			return result, invalid("superfluous witness")
		}
	}
	lockStart := r.pos
	lock := uint32(r.number(4))
	if r.err != nil {
		return result, r.err
	}
	if r.pos != len(raw) {
		return result, invalid("trailing transaction bytes")
	}
	if e := ctx.Err(); e != nil {
		return result, e
	}
	h := sha256.New()
	h.Write(raw[:4])
	h.Write(raw[mainStart:mainEnd])
	h.Write(raw[lockStart:])
	sum := sha256.Sum256(h.Sum(nil))
	copy(result.hash[:], sum[:])
	if tx != nil {
		tx.LockTime = lock
	}
	result.tx = tx
	return result, nil
}
