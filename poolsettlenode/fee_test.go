// Copyright (c) 2026 The SLUG developers. ISC license.
package poolsettlenode

import (
	"encoding/json"
	"os"
	"testing"
)

func TestIndependentFeeArithmetic(t *testing.T) {
	raw, err := os.ReadFile("testdata/fee.json")
	if err != nil {
		t.Fatal(err)
	}
	var refs struct {
		Version int
		Sizes   []struct {
			Inputs, Payments int
			Rate, VSize, Fee int64
		}
		Rates []struct {
			Decimal string
			Rate    int64
		}
	}
	if json.Unmarshal(raw, &refs) != nil || refs.Version != 1 || len(refs.Sizes) != 32 || len(refs.Rates) != 6 {
		t.Fatal("independent corpus")
	}
	for _, x := range refs.Sizes {
		size, e := conservativeVSize(x.Inputs, x.Payments)
		if e != nil || size != x.VSize {
			t.Fatal("independent vsize", x, size, e)
		}
		fee, e := quotedAtoms(x.Rate, size, x.Fee)
		if e != nil || fee != x.Fee {
			t.Fatal("independent fee", x, fee, e)
		}
		if x.Fee > 1 {
			if _, e = quotedAtoms(x.Rate, size, x.Fee-1); e == nil {
				t.Fatal("fee cap clipped")
			}
		}
	}
	for _, x := range refs.Rates {
		got, e := estimatedRate([]byte(x.Decimal), 1000000000)
		if e != nil || got != x.Rate {
			t.Fatal("rate units", x, got, e)
		}
	}
	for _, v := range []string{"-1", "0", "10.000000001", "1e1000000", "1e-1000000", "01", "\"0.001\"", "NaN", "1/2", "null", ""} {
		if _, e := estimatedRate([]byte(v), 1000000000); e == nil {
			t.Fatal("invalid rate accepted", v)
		}
	}
	for _, v := range [][2]int{{0, 1}, {1, 0}, {257, 1}, {1, 257}, {-1, 1}} {
		if _, e := conservativeVSize(v[0], v[1]); e == nil {
			t.Fatal("invalid count accepted")
		}
	}
}
