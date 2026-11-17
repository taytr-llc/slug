// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterbout

import "math"

// roundEven divides on unsigned magnitude, including MinInt64, without
// overflowing a signed negation or twice a potentially large remainder.
func roundEven(n, d int64) (int64, error) {
	if d <= 0 {
		return 0, ErrArithmetic
	}
	magnitude := uint64(n)
	if n < 0 {
		magnitude = uint64(-(n + 1)) + 1
	}
	den := uint64(d)
	q, r := magnitude/den, magnitude%den
	if r > den-r || r == den-r && q&1 != 0 {
		q++
	}
	if n < 0 {
		if q > uint64(math.MaxInt64)+1 {
			return 0, ErrArithmetic
		}
		if q == uint64(math.MaxInt64)+1 {
			return math.MinInt64, nil
		}
		return -int64(q), nil
	}
	if q > math.MaxInt64 {
		return 0, ErrArithmetic
	}
	return int64(q), nil
}

func checkedAdd(a, b int64) (int64, error) {
	if b > 0 && a > math.MaxInt64-b || b < 0 && a < math.MinInt64-b {
		return 0, ErrArithmetic
	}
	return a + b, nil
}

func checkedProduct(a, b int64) (int64, error) {
	if a == 0 || b == 0 {
		return 0, nil
	}
	if a > 0 {
		if b > 0 && a > math.MaxInt64/b || b < 0 && b < math.MinInt64/a {
			return 0, ErrArithmetic
		}
	} else if b > 0 {
		if a < math.MinInt64/b {
			return 0, ErrArithmetic
		}
	} else if a < math.MaxInt64/b {
		return 0, ErrArithmetic
	}
	return a * b, nil
}

// arithmetic preserves the first error while keeping the model expressions
// in their contract order. Every caller checks err before publishing a state.
type arithmetic struct{ err error }

func (a *arithmetic) result(v int64, err error) int64 {
	if a.err == nil {
		a.err = err
	}
	if a.err != nil {
		return 0
	}
	return v
}
func (a *arithmetic) add(x, y int64) int64 { return a.result(checkedAdd(x, y)) }
func (a *arithmetic) sub(x, y int64) int64 {
	if y == math.MinInt64 {
		if x >= 0 {
			return a.result(0, ErrArithmetic)
		}
		return a.add(a.add(x, math.MaxInt64), 1)
	}
	return a.add(x, -y)
}
func (a *arithmetic) product(x, y int64) int64 { return a.result(checkedProduct(x, y)) }
func (a *arithmetic) round(x, d int64) int64   { return a.result(roundEven(x, d)) }
func (a *arithmetic) mul(x, y int64) int64     { return a.round(a.product(x, y), Scale) }
func (a *arithmetic) div(x, y int64) int64     { return a.round(a.product(x, Scale), y) }
func clamp(x, lo, hi int64) int64              { return min(max(x, lo), hi) }
