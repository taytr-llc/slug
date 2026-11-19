// Copyright (c) 2026 The SLUG developers. ISC license.
package gymbout

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/taytr-llc/slug/fighterbout"
	"github.com/taytr-llc/slug/gymcard"
)

func TestNameV2DescriptorCannotReplaceBookedSource(t *testing.T) {
	requireNormal(t)
	ref := readReference(t)
	if len(ref.Compositions) == 0 {
		t.Fatal("missing frozen v1 composition")
	}
	for _, f := range ref.Compositions {
		got, err := DeriveInput(f.Pin, f.Source)
		if err != nil || !reflect.DeepEqual(got, f.Input) {
			t.Fatal("unchanged v1 input differs from frozen reference", err)
		}
		for n := range f.Source.Corners {
			raw, _ := json.Marshal(f.Source)
			var bad gymcard.BoutSource
			if json.Unmarshal(raw, &bad) != nil {
				t.Fatal("copy")
			}
			// Invalid counterexample only, never a relabelled valid v2 receipt.
			bad.Corners[n].Member.Assignment.DescriptorID = "cd02ae57b5918548244be2cabbe6c0c433feb5a1c165745e2e5c6bf235128e75"
			got, err := DeriveInput(f.Pin, bad)
			if err == nil || got != (fighterbout.Input{}) {
				t.Fatal("supported foreign descriptor replaced booked source")
			}
		}
	}
}
