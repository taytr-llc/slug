// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterownership

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"testing"

	"github.com/taytr-llc/slug/registry"
)

type operatorCoreReference struct {
	Version   uint32
	Positives []struct {
		Family, Name string
		Value        json.RawMessage
		CanonicalHex string `json:"canonical_hex"`
		IDs          map[string]string
	}
	Refusals []struct {
		Name, Family    string
		ExpectedPin     Pin    `json:"expected_pin"`
		SelectedProfile uint32 `json:"selected_profile"`
		Value           json.RawMessage
	}
	Compositions map[string]operatorCoreComposition `json:"conditional_compositions"`
}
type operatorCoreComposition struct {
	Pin       Pin
	Admission AdmissionStatement
	Boundary  SignedBoundary
	Cutoff    CutoffRecord
	Frame     FinalityFrame
	State     EffectiveState `json:"conditional_state"`
	Map       OwnershipMap   `json:"conditional_map"`
}

func operatorCoreCorpus(t *testing.T) operatorCoreReference {
	t.Helper()
	raw, e := os.ReadFile("../registry/testdata/operator-reference/authority-v2.json")
	if e != nil {
		t.Fatal(e)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "7328ea2102c9ac993281bd732590c3e734eda26a3d180c2fa98bbd37da02c6b4" {
		t.Fatal("independent corpus changed")
	}
	// Preserve the file hash, then remove only presentation whitespace so raw
	// nested evidence reaches the canonical closed codecs exactly.
	var compact bytes.Buffer
	if e = json.Compact(&compact, raw); e != nil {
		t.Fatal(e)
	}
	var c operatorCoreReference
	if e = json.Unmarshal(compact.Bytes(), &c); e != nil {
		t.Fatal(e)
	}
	if c.Version != 1 || len(c.Positives) != 54 || len(c.Refusals) != 102 || len(c.Compositions) != 6 {
		t.Fatal("independent inventory")
	}
	return c
}
func operatorCoreCodec(p Pin, selected uint32) (ownershipCodec, error) {
	if selected == 1 {
		if _, e := ID(p); e != nil {
			return ownershipCodec{}, e
		}
		return ownershipCodec{}, nil
	}
	if selected != 2 {
		return ownershipCodec{}, ErrSource
	}
	profile, e := registry.NewOperatorProfile(p)
	if e != nil {
		return ownershipCodec{}, e
	}
	return selectedOperatorCodec(profile)
}

// Signature and conditional composition vectors are not proof of a normal-work
// branch. Genesis replay has separate concrete-node integration below.
func TestOwnershipOperatorReferenceFrames(t *testing.T) {
	c := operatorCoreCorpus(t)
	n := 0
	for _, row := range c.Positives {
		if row.Family != "frame" {
			continue
		}
		n++
		t.Run(row.Name, func(t *testing.T) {
			x, ok := c.Compositions[row.Name]
			if !ok {
				t.Fatal("composition absent")
			}
			codec, e := operatorCoreCodec(x.Pin, x.Pin.Version)
			if e != nil {
				t.Fatal(e)
			}
			raw, e := hex.DecodeString(row.CanonicalHex)
			if e != nil {
				t.Fatal(e)
			}
			var f FinalityFrame
			if e = decode(raw, 4096, &f); e != nil {
				t.Fatal(e)
			}
			if e = codec.ValidateFinalityFrame(x.Pin, f); e != nil {
				t.Fatal(e)
			}
			id, e := codec.FinalityFrameID(x.Pin, f)
			if e != nil || id != row.IDs["frame_id"] {
				t.Fatal("frame ID", e)
			}
			body, e := objectDigest("SLUG fighter ownership finality body", f.FinalityFrameBody)
			if e != nil || body != row.IDs["body_id"] {
				t.Fatal("body ID", e)
			}
			out, e := json.Marshal(f)
			if e != nil || !bytes.Equal(out, raw) {
				t.Fatal("frame canonical bytes", e)
			}
			if x.Pin.Version == 2 {
				public, e := NewOperatorCodec(codec.profile)
				if e != nil || public.ValidateFinalityFrame(f) != nil {
					t.Fatal("operator public codec", e)
				}
				if ValidateFinalityFrame(x.Pin, f) == nil {
					t.Fatal("legacy accepted operator frame")
				}
			}
		})
	}
	if n != 6 {
		t.Fatal("frame inventory", n)
	}
}
func TestOwnershipOperatorReferenceFrameRefusals(t *testing.T) {
	c := operatorCoreCorpus(t)
	n := 0
	for _, row := range c.Refusals {
		if row.Family != "frame" {
			continue
		}
		n++
		t.Run(row.Name, func(t *testing.T) {
			codec, e := operatorCoreCodec(row.ExpectedPin, row.SelectedProfile)
			if e == nil {
				var f FinalityFrame
				e = decode(row.Value, 4096, &f)
				if e == nil {
					e = codec.ValidateFinalityFrame(row.ExpectedPin, f)
				}
			}
			if e == nil {
				t.Fatal("accepted independent refusal")
			}
		})
	}
	if n != 10 {
		t.Fatal("frame refusal inventory", n)
	}
}
func TestOwnershipOperatorReferenceConditionalComposition(t *testing.T) {
	for name, x := range operatorCoreCorpus(t).Compositions {
		t.Run(name, func(t *testing.T) {
			codec, e := operatorCoreCodec(x.Pin, x.Pin.Version)
			if e != nil {
				t.Fatal(e)
			}
			initial, e := codec.InitialState(x.Pin, x.Admission)
			if e != nil {
				t.Fatal(e)
			}
			var finalEvidence FinalStateEvidence
			if e = decode(x.State.Evidence, 4096, &finalEvidence); e != nil {
				t.Fatal(e)
			}
			if initial.StateID != finalEvidence.PriorStateID || initial.BirthAdmissionID != x.State.BirthAdmissionID || initial.Generation != 0 {
				t.Fatal("birth/predecessor composition")
			}
			if e = codec.ValidateEffectiveState(x.Pin, x.State); e != nil {
				t.Fatal(e)
			}
			raw, e := codec.MarshalEffectiveState(x.Pin, x.State)
			if e != nil {
				t.Fatal(e)
			}
			parsed, e := codec.ParseEffectiveState(x.Pin, raw)
			if e != nil || parsed.StateID != x.State.StateID {
				t.Fatal("state canonical parse", e)
			}
			mapID, e := codec.OwnershipMapID(x.Pin, x.Map)
			if e != nil || mapID != x.Cutoff.OwnershipDigest || mapID != x.Frame.OwnershipMapID {
				t.Fatal("map composition", e)
			}
			fp, e := codec.FinalizedPrefixID(x.Pin, x.Map)
			if e != nil || fp != x.Frame.FinalizedPrefixID || fp != x.Cutoff.Head.FinalizedPrefixID {
				t.Fatal("finalized prefix composition", e)
			}
			digest, e := replayDigest(x.Cutoff.Head, mapID)
			if e != nil || digest != x.Cutoff.Head.ReplayDigest || codec.ValidateHead(x.Pin, x.Cutoff.Head) != nil {
				t.Fatal("head composition", e)
			}
			if _, e = codec.cutoffFrames(x.Pin, x.Boundary.Intent, map[uint32]FinalityFrame{1: x.Frame}, map[uint32]FinalityFrameBody{1: x.Frame.FinalityFrameBody}); e != nil {
				t.Fatal("frozen frame prefix", e)
			}
			if _, e = codec.cutoffFrames(x.Pin, x.Boundary.Intent, map[uint32]FinalityFrame{}, map[uint32]FinalityFrameBody{1: x.Frame.FinalityFrameBody}); e == nil {
				t.Fatal("missing signed prefix accepted")
			}
			bad := x.State
			bad.PinID = zeroID
			if codec.ValidateEffectiveState(x.Pin, bad) == nil {
				t.Fatal("cross-profile state accepted")
			}
		})
	}
}
