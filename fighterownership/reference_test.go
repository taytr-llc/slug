// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterownership

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/registry"
	"github.com/taytr-llc/slug/wire/v2"
	"os"
	"strconv"
	"testing"
)

const coreReferenceSHA = "1145dede4fcfa453e8bb1cdffff01079f6cca1df3bbe810dc32868207292b507"

type coreCase struct {
	Family    string          `json:"family"`
	Name      string          `json:"name"`
	Value     json.RawMessage `json:"value"`
	Canonical string          `json:"canonical_hex"`
	IDs       map[string]any  `json:"ids"`
}
type coreCorpus struct {
	Positives []coreCase `json:"positives"`
	Negatives []coreCase `json:"negatives"`
	Raw       []struct {
		Name  string `json:"name"`
		Raw   string `json:"raw_hex"`
		Valid bool   `json:"valid"`
	} `json:"raw_cases"`
}

func coreReference(t *testing.T) (coreCorpus, map[string]Pin) {
	t.Helper()
	raw, e := os.ReadFile("testdata/reference/public-core-v1.json")
	if e != nil {
		t.Fatal(e)
	}
	sum := sha256.Sum256(raw)
	if hex.EncodeToString(sum[:]) != coreReferenceSHA {
		t.Fatal("independent corpus changed")
	}
	var c coreCorpus
	if json.Unmarshal(raw, &c) != nil || len(c.Positives) != 46 || len(c.Negatives) != 27 || len(c.Raw) != 7 {
		t.Fatal("reference cases missing")
	}
	pins := map[string]Pin{}
	for _, v := range c.Positives {
		if v.Family == "pin" {
			var p Pin
			if json.Unmarshal(v.Value, &p) != nil {
				t.Fatal("pin")
			}
			pins[v.IDs["pin_id"].(string)] = p
		}
	}
	return c, pins
}
func casePin(raw []byte, pins map[string]Pin) Pin {
	var v struct {
		PinID string `json:"pin_id"`
	}
	json.Unmarshal(raw, &v)
	if p, ok := pins[v.PinID]; ok {
		return p
	}
	for _, p := range pins {
		return p
	}
	return Pin{}
}
func referenceObject(p Pin, family string, raw []byte) (any, map[string]string, error) {
	ids := map[string]string{}
	var v any
	var e error
	switch family {
	case "pin":
		var x Pin
		e = decode(raw, 8192, &x)
		if e == nil {
			_, e = ValidatePin(x)
		}
		if e == nil {
			ids["pin_id"], e = ID(x)
		}
		v = x
	case "admission":
		var x AdmissionStatement
		x, e = registry.ParsePublicAdmission(p, raw)
		if e == nil {
			ids["admission_id"], e = registry.PublicAdmissionID(p, x)
		}
		v = x
	case "state":
		var x EffectiveState
		x, e = ParseEffectiveState(p, raw)
		ids["state_id"] = x.StateID
		v = x
	case "map":
		var x OwnershipMap
		x, e = ParseOwnershipMap(p, raw)
		if e == nil {
			ids["map_id"], e = OwnershipMapID(p, x)
			ids["finalized_prefix_id"], _ = FinalizedPrefixID(p, x)
		}
		v = x
	case "checkpoint":
		var x chaincfg.SignedCheckpoint
		e = decode(raw, 8192, &x)
		if e == nil {
			var b []byte
			b, e = x.SigningBytes()
			ids["preimage_hex"] = hex.EncodeToString(b)
		}
		v = x
	case "publication":
		var x PublicationEntry
		x, e = registry.ParsePublicPublication(p, raw)
		if e == nil {
			ids["publication_id"], e = registry.PublicPublicationID(p, x)
		}
		v = x
	case "head":
		var x HeadRef
		e = decode(raw, 8192, &x)
		if e == nil {
			ids["head_id"], e = HeadID(p, x)
		}
		v = x
	case "frame_body":
		var x FinalityFrameBody
		e = decode(raw, 8192, &x)
		if e == nil {
			e = validateFrameBody(p, x)
		}
		if e == nil {
			ids["body_id"], e = objectDigest("SLUG fighter ownership finality body", x)
		}
		v = x
	case "frame":
		var x FinalityFrame
		e = decode(raw, 8192, &x)
		if e == nil {
			ids["frame_id"], e = FinalityFrameID(p, x)
			ids["signed_head"] = ids["frame_id"]
			ids["body_head"], _ = objectDigest("SLUG fighter ownership finality body", x.FinalityFrameBody)
			ids["sequence"] = strconv.FormatUint(uint64(x.Sequence), 10)
		}
		v = x
	case "finality_witness":
		var x FinalityWitness
		e = decode(raw, 4096, &x)
		if e == nil {
			e = validateFinalityWitness(p, x)
		}
		if e == nil {
			ids["frontier_evidence_id"], e = objectDigest("SLUG fighter ownership frontier evidence", x.Frontier)
		}
		v = x
	case "boundary":
		var x SignedBoundary
		x, e = registry.ParsePublicBoundary(p, raw)
		if e == nil {
			ids["boundary_id"], e = registry.PublicBoundaryID(p, x)
		}
		v = x
	case "cutoff":
		var x CutoffRecord
		x, e = registry.ParsePublicCutoff(p, raw)
		if e == nil {
			ids["cutoff_id"], e = registry.PublicCutoffID(p, x)
		}
		v = x
	default:
		e = ErrInvalid
	}
	return v, ids, e
}
func TestOwnershipIndependentCanonicalReference(t *testing.T) {
	c, pins := coreReference(t)
	for _, x := range c.Positives {
		t.Run(x.Family+"/"+x.Name, func(t *testing.T) {
			raw, e := hex.DecodeString(x.Canonical)
			if e != nil {
				t.Fatal(e)
			}
			p := casePin(raw, pins)
			value, ids, e := referenceObject(p, x.Family, raw)
			if wire.TestPoW && x.Family == "publication" {
				var entry PublicationEntry
				if json.Unmarshal(raw, &entry) == nil && entry.Kind == "checkpoint" {
					if e == nil {
						t.Fatal("normal-source checkpoint accepted under different profile genesis")
					}
					return
				}
			}
			if e != nil {
				t.Fatal(e)
			}
			got, _ := json.Marshal(value)
			if !bytes.Equal(got, raw) {
				t.Fatal("canonical bytes changed")
			}
			for k, want := range x.IDs {
				if ids[k] != fmt.Sprint(want) {
					t.Fatalf("%s got %s want %s", k, ids[k], want)
				}
			}
			if x.Family == "frame" {
				f := value.(FinalityFrame)
				sig, e := signObject("SLUG fighter ownership finality statement", f)
				if e != nil || sig != f.Signature {
					t.Fatal("independent fixture signature", e)
				}
			}
		})
	}
	for _, x := range c.Raw {
		t.Run(x.Name, func(t *testing.T) {
			raw, _ := hex.DecodeString(x.Raw)
			_, e := registry.ParsePublicManifestPin(raw)
			if (e == nil) != x.Valid {
				t.Fatal("raw verdict", e)
			}
		})
	}
	// Explicit Go-only type grammar controls supplement the conditional Python
	// model; they are not new expected cryptographic vectors.
	var canonical []byte
	for _, v := range c.Positives {
		if v.Family == "pin" {
			canonical, _ = hex.DecodeString(v.Canonical)
			break
		}
	}
	for _, replacement := range []string{`"version":true`, `"version":null`, `"version":1.0`, `"version":1e0`} {
		raw := bytes.Replace(canonical, []byte(`"version":1`), []byte(replacement), 1)
		if _, e := registry.ParsePublicManifestPin(raw); e == nil {
			t.Fatal("typed version accepted", replacement)
		}
	}
}
func TestOwnershipIndependentNegativeReference(t *testing.T) {
	c, pins := coreReference(t)
	for _, x := range c.Negatives {
		t.Run(x.Family+"/"+x.Name, func(t *testing.T) {
			p := casePin(x.Value, pins)
			var e error
			switch x.Family {
			case "cutoff_frame_prefix":
				var value cutoffPrefixInput
				if json.Unmarshal(x.Value, &value) != nil {
					t.Fatal("prefix fixture")
				}
				p = pins[value.Boundary.PinID]
				frames, bodies := cutoffPrefixMaps(value)
				_, e = cutoffFrames(p, value.Boundary.Intent, frames, bodies)
			case "publication_prefix":
				var values []PublicationEntry
				json.Unmarshal(x.Value, &values)
				if len(values) > 0 {
					p = pins[values[0].PinID]
				}
				e = verifyPublicationPrefix(p, values)
			case "frame_prefix":
				var values []FinalityFrame
				json.Unmarshal(x.Value, &values)
				if len(values) > 0 {
					p = pins[values[0].PinID]
				}
				e = verifyFramePrefix(p, values)
			case "storage":
				var rows []struct {
					Key   string `json:"key_hex"`
					Value string `json:"value_hex"`
				}
				json.Unmarshal(x.Value, &rows)
				seen := map[string]bool{}
				for _, row := range rows {
					if seen[row.Key] {
						e = ErrCorrupt
					}
					seen[row.Key] = true
				}
			default:
				raw := bytes.TrimSpace(x.Value) // RawMessage preserves exact producer field order.
				_, _, e = referenceObject(p, x.Family, raw)
			}
			if e == nil {
				t.Fatal("independent refusal accepted")
			}
		})
	}
}
