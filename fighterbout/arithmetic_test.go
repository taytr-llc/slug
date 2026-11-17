// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterbout

import (
	"errors"
	"math"
	"math/big"
	"testing"
)

func TestRoundEven(t *testing.T) {
	for _, tc := range []struct{ n, d, want int64 }{
		{0, 1, 0}, {1, 2, 0}, {3, 2, 2}, {5, 2, 2}, {7, 2, 4},
		{-1, 2, 0}, {-3, 2, -2}, {-5, 2, -2}, {-7, 2, -4},
		{math.MinInt64, 1, math.MinInt64}, {math.MaxInt64, 1, math.MaxInt64},
		{math.MinInt64, math.MaxInt64, -1}, {math.MaxInt64 - 1, math.MaxInt64, 1},
	} {
		got, err := roundEven(tc.n, tc.d)
		if err != nil || got != tc.want {
			t.Fatalf("R(%d,%d) = %d,%v; want %d", tc.n, tc.d, got, err, tc.want)
		}
	}
	for _, d := range []int64{0, -1, math.MinInt64} {
		if got, err := roundEven(1, d); got != 0 || !errors.Is(err, ErrArithmetic) {
			t.Fatal("accepted nonpositive denominator")
		}
	}
}

func TestCheckedArithmeticAgainstBigIntegers(t *testing.T) {
	values := []int64{math.MinInt64, math.MinInt64 + 1, -2700000000, -Scale, -3, -1,
		0, 1, 2, Scale, 2700000000, math.MaxInt64 - 1, math.MaxInt64}
	for _, x := range values {
		for _, y := range values {
			for _, operation := range []string{"add", "subtract", "multiply"} {
				want := new(big.Int)
				var got int64
				var err error
				switch operation {
				case "add":
					want.Add(big.NewInt(x), big.NewInt(y))
					got, err = checkedAdd(x, y)
				case "multiply":
					want.Mul(big.NewInt(x), big.NewInt(y))
					got, err = checkedProduct(x, y)
				case "subtract":
					want.Sub(big.NewInt(x), big.NewInt(y))
					a := arithmetic{}
					got, err = a.sub(x, y), a.err
				}
				if want.IsInt64() {
					if err != nil || got != want.Int64() {
						t.Fatalf("%s(%d,%d) = %d,%v; want %s", operation, x, y, got, err, want)
					}
				} else if !errors.Is(err, ErrArithmetic) || got != 0 {
					t.Fatalf("%s overflow was not rejected", operation)
				}
			}
		}
	}
	// Round exact rational quotients against an independent arbitrary-precision
	// comparison, including both signs and every tie in this finite grid.
	for n := int64(-1000); n <= 1000; n++ {
		for d := int64(1); d <= 31; d++ {
			q, remainder := new(big.Int), new(big.Int)
			q.QuoRem(big.NewInt(n), big.NewInt(d), remainder)
			twice := new(big.Int).Lsh(new(big.Int).Abs(remainder), 1)
			cmp := twice.Cmp(big.NewInt(d))
			if cmp > 0 || cmp == 0 && q.Bit(0) == 1 {
				if n < 0 {
					q.Sub(q, big.NewInt(1))
				} else {
					q.Add(q, big.NewInt(1))
				}
			}
			got, err := roundEven(n, d)
			if err != nil || got != q.Int64() {
				t.Fatalf("R(%d,%d) = %d,%v; want %s", n, d, got, err, q)
			}
		}
	}
}

func TestFixedPointNestedOrderAndStickyError(t *testing.T) {
	a := arithmetic{}
	if got := a.mul(a.mul(500001, 500001), 500001); got != 125001 || a.err != nil {
		t.Fatalf("nested multiplication %d,%v", got, a.err)
	}
	if got := a.div(-5, 2*Scale); got != -2 || a.err != nil {
		t.Fatalf("signed scaled division %d,%v", got, a.err)
	}
	a.product(math.MaxInt64, 2)
	if a.add(1, 2) != 0 || !errors.Is(a.err, ErrArithmetic) {
		t.Fatal("arithmetic failure did not remain sticky")
	}
}
