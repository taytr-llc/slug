// Copyright (c) 2026 The SLUG developers. ISC license.
package poolround

import (
	"fmt"
	"reflect"
	"testing"
)

// The existing independently frozen rational corpus remains the oracle.
// Partitions model page boundaries; these inputs do not authorize stored work.
func TestRoundStreamIndependentWindowPartitions(t *testing.T) {
	cfg, entries, cases := modeledReference(t)
	for _, tc := range cases {
		for cut := uint64(0); cut <= tc.Through; cut++ {
			t.Run(fmt.Sprintf("through-%d-cut-%d", tc.Through, cut), func(t *testing.T) {
				builder := newRoundDerivation(cfg)
				got := make([]Round, 0)
				for _, page := range [][]int{{0, int(cut)}, {int(cut), int(tc.Through)}} {
					for _, entry := range entries[page[0]:page[1]] {
						round, err := builder.add(entry)
						if err != nil {
							t.Fatal(err)
						}
						if round != nil {
							got = append(got, *round)
						}
					}
				}
				if len(got) != len(tc.Rounds) {
					t.Fatal("partition changed number of rounds")
				}
				var closed uint64
				for i, r := range got {
					projection := modeledRound{r.Sequence, r.BlockHash, r.FirstAccepted, r.LastAccepted, r.ShareKeys, r.RewardAtoms, r.Allocations}
					if !reflect.DeepEqual(projection, tc.Rounds[i]) {
						t.Fatal("page boundary changed independent allocation/window", projection, tc.Rounds[i])
					}
					closed = r.LastAccepted
				}
				if tc.Through-closed != tc.Pending {
					t.Fatal("page boundary consumed unfinished shares")
				}
			})
		}
	}
}

func TestRoundStreamDuplicateGuardsCrossPartitions(t *testing.T) {
	cfg, entries, _ := modeledReference(t)
	for _, mode := range []string{"receipt", "share", "winner", "acceptance-gap"} {
		t.Run(mode, func(t *testing.T) {
			builder := newRoundDerivation(cfg)
			for _, entry := range entries[:4] {
				if _, err := builder.add(entry); err != nil {
					t.Fatal(err)
				}
			}
			next := cloneEntry(entries[4])
			switch mode {
			case "receipt":
				next.Share.Sequence = entries[0].Share.Sequence
			case "share":
				next.Share.DuplicateKey = entries[0].Share.DuplicateKey
			case "winner":
				next.Share.WorkHash = entries[2].Share.WorkHash
			case "acceptance-gap":
				next.Share.AcceptedSequence++
			}
			if _, err := builder.add(next); err == nil {
				t.Fatal("partition erased global duplicate/order guard")
			}
		})
	}
}
