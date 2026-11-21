// Copyright (c) 2026 The SLUG developers. ISC license.
// Package pooltxshape sizes explicitly selected native P2WPKH transaction
// shapes. It provides no signing, funding, account or broadcast authority.
package pooltxshape

import (
	"errors"
	"math"
)

const MaxNativeInputs = 1464
const MaxPayees = 256
const MaxStandardWeight = 400000

var ErrInvalid = errors.New("invalid native pool transaction shape")
var ErrCapacity = errors.New("native pool transaction exceeds standard weight")

// Shape uses conservative full native witnesses, including the signature's
// sighash byte. WitnessBytes includes marker and flag. RawBytes is never vsize.
type Shape struct {
	StrippedBytes int64
	WitnessBytes  int64
	RawBytes      int64
	Weight        int64
	VSize         int64
}

func checkedAdd(values ...int64) (int64, bool) {
	var total int64
	for _, value := range values {
		if value < 0 || value > math.MaxInt64-total {
			return 0, false
		}
		total += value
	}
	return total, true
}

func checkedMul(a, b int64) (int64, bool) {
	if a < 0 || b < 0 || a != 0 && b > math.MaxInt64/a {
		return 0, false
	}
	return a * b, true
}

// countBytes is used only after the public counts have been bounded below
// 65536. The 252/253 transition applies independently to inputs and outputs.
func countBytes(n int64) int64 {
	if n < 253 {
		return 1
	}
	return 3
}

// NativeP2WPKH sizes a fully specified count of native84 inputs, native payees
// and optional native change. It does not choose inputs, payment subsets or
// fee policy. Counts and arithmetic are checked before any size is returned.
// A positive result cannot widen an independently selected legacy API/profile.
func NativeP2WPKH(inputs, payees int, change bool) (Shape, error) {
	if inputs < 1 || inputs > MaxNativeInputs || payees < 1 || payees > MaxPayees {
		return Shape{}, ErrInvalid
	}
	n, outputs := int64(inputs), int64(payees)
	if change {
		outputs++
	}
	inputBytes, ok := checkedMul(41, n)
	if !ok {
		return Shape{}, ErrInvalid
	}
	outputBytes, ok := checkedMul(31, outputs)
	if !ok {
		return Shape{}, ErrInvalid
	}
	stripped, ok := checkedAdd(4, countBytes(n), inputBytes, countBytes(outputs), outputBytes, 4)
	if !ok {
		return Shape{}, ErrInvalid
	}
	witness, ok := checkedMul(109, n)
	if !ok {
		return Shape{}, ErrInvalid
	}
	witness, ok = checkedAdd(2, witness)
	if !ok {
		return Shape{}, ErrInvalid
	}
	raw, ok := checkedAdd(stripped, witness)
	if !ok {
		return Shape{}, ErrInvalid
	}
	strippedWeight, ok := checkedMul(4, stripped)
	if !ok {
		return Shape{}, ErrInvalid
	}
	weight, ok := checkedAdd(strippedWeight, witness)
	if !ok {
		return Shape{}, ErrInvalid
	}
	if weight > MaxStandardWeight {
		return Shape{}, ErrCapacity
	}
	roundedWeight, ok := checkedAdd(weight, 3)
	if !ok {
		return Shape{}, ErrInvalid
	}
	return Shape{StrippedBytes: stripped, WitnessBytes: witness, RawBytes: raw, Weight: weight, VSize: roundedWeight / 4}, nil
}
