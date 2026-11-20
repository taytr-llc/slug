// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterownership

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

type cutoffPrefixInput struct {
	Boundary SignedBoundary  `json:"boundary"`
	Frames   []FinalityFrame `json:"frames"`
}

func cutoffPrefixMaps(v cutoffPrefixInput) (map[uint32]FinalityFrame, map[uint32]FinalityFrameBody) {
	fs := map[uint32]FinalityFrame{}
	bs := map[uint32]FinalityFrameBody{}
	for _, f := range v.Frames {
		fs[f.Sequence] = f
		bs[f.Sequence] = f.FinalityFrameBody
	}
	return fs, bs
}
func TestOwnershipCutoffIndependentFrozenPrefix(t *testing.T) {
	_, pins := coreReference(t)
	raw, e := os.ReadFile("testdata/reference/public-core-v1.json")
	if e != nil {
		t.Fatal(e)
	}
	// Non-prefix state-case inputs have other typed shapes, so filter before decoding.
	var outer struct {
		States []json.RawMessage `json:"state_cases"`
	}
	if json.Unmarshal(raw, &outer) != nil {
		t.Fatal("corpus")
	}
	for _, row := range outer.States {
		var label struct {
			Name string `json:"name"`
		}
		json.Unmarshal(row, &label)
		if !strings.HasPrefix(label.Name, "cutoff-") {
			continue
		}
		var x struct {
			Name     string            `json:"name"`
			Input    cutoffPrefixInput `json:"input"`
			Expected struct {
				Sequence   uint32 `json:"sequence"`
				BodyHead   string `json:"body_head"`
				SignedHead string `json:"signed_head"`
			} `json:"expected"`
		}
		if json.Unmarshal(row, &x) != nil {
			t.Fatal("prefix case")
		}
		t.Run(x.Name, func(t *testing.T) {
			p := pins[x.Input.Boundary.PinID]
			fs, bs := cutoffPrefixMaps(x.Input)
			got, e := cutoffFrames(p, x.Input.Boundary.Intent, fs, bs)
			if e != nil {
				t.Fatal(e)
			}
			body, signed := zeroID, zeroID
			if len(got) > 0 {
				last := got[len(got)-1]
				body, e = objectDigest("SLUG fighter ownership finality body", last.FinalityFrameBody)
				if e != nil {
					t.Fatal(e)
				}
				signed, e = FinalityFrameID(p, last)
				if e != nil {
					t.Fatal(e)
				}
			}
			if uint32(len(got)) != x.Expected.Sequence || body != x.Expected.BodyHead || signed != x.Expected.SignedHead {
				t.Fatal("captured prefix changed")
			}
			if x.Expected.Sequence == 1 {
				delete(fs, 1)
				if _, e = cutoffFrames(p, x.Input.Boundary.Intent, fs, bs); !errors.Is(e, ErrUnavailable) {
					t.Fatal("required signature gap", e)
				}
			}
		})
	}
}
