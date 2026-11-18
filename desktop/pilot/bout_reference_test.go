// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/wire/v2"
	"os"
	"strings"
	"testing"
)

type boutRecipe struct {
	Op, Path, Key, Old, New string
	Value                   json.RawMessage
	Bytes                   int
	Edits                   []struct {
		Path  string
		Value json.RawMessage
	}
}

func boutReferenceBytes(t *testing.T, base string, r boutRecipe) []byte {
	t.Helper()
	adapt := cardReferenceRecipe{}
	switch r.Op {
	case "identity":
		return []byte(base)
	case "set":
		adapt.Edits = append(adapt.Edits, struct {
			Op, Path string
			Value    json.RawMessage
		}{"set", r.Path, r.Value})
	case "set_many":
		for _, e := range r.Edits {
			adapt.Edits = append(adapt.Edits, struct {
				Op, Path string
				Value    json.RawMessage
			}{"set", e.Path, e.Value})
		}
	case "remove":
		adapt.Edits = append(adapt.Edits, struct {
			Op, Path string
			Value    json.RawMessage
		}{"delete", r.Path, nil})
	case "reverse":
		adapt.Edits = append(adapt.Edits, struct {
			Op, Path string
			Value    json.RawMessage
		}{"reverse", r.Path, nil})
	case "pad":
		adapt.Pad = &r.Bytes
	case "replace_once":
		adapt.Replace = []string{r.Old, r.New}
	case "whitespace_reversed":
		adapt.Style = "reversed_whitespace"
		return append(append([]byte(" \n\t"), cardReferenceBytes(t, base, adapt)...), []byte("\r\n")...)
	case "duplicate_top", "escaped_key_duplicate":
		d := json.NewDecoder(strings.NewReader(base))
		d.UseNumber()
		n, e := seedReferenceParse(d)
		if e != nil {
			t.Fatal(e)
		}
		key := n.names[0]
		var v *seedReferenceNode
		if r.Key != "" {
			key = r.Key
		}
		v, _ = cardReferenceChild(t, n, key)
		k, _ := json.Marshal(key)
		if r.Op == "escaped_key_duplicate" {
			k = []byte(`"\u` + hex.EncodeToString([]byte{0, key[0]}) + key[1:] + `"`)
		}
		b := append([]byte(base[:len(base)-1]), ',')
		b = append(b, k...)
		b = append(b, ':')
		b = append(b, v.encode(false)...)
		return append(b, '}')
	default:
		t.Fatal("recipe", r.Op)
	}
	return cardReferenceBytes(t, base, adapt)
}
func TestBoutIndependentTransport(t *testing.T) {
	raw, e := os.ReadFile("../../gymbout/testdata/reference/bout_v1.json")
	if e != nil {
		t.Fatal(e)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "09f0430789328b9226fc4a24aae3cdb07beeb8b41d61b42cd33e4ba3121fa2ec" {
		t.Fatal("frozen corpus changed")
	}
	if wire.TestPoW {
		t.Skip("normal profile reference")
	}
	var doc struct {
		DTOs  []struct{ Kind, Name, Canonical, Context string }
		Cases []struct {
			Name, Kind, Context string
			Base                int
			Recipe              boutRecipe
			InputBytes          int    `json:"input_bytes"`
			InputSHA            string `json:"input_sha256"`
			Accept              bool
			CanonicalRef        *int `json:"canonical_ref"`
			Canonical           *string
		} `json:"codec_cases"`
	}
	if json.Unmarshal(raw, &doc) != nil {
		t.Fatal("corpus")
	}
	for _, c := range doc.Cases {
		if c.Context != "transport" || c.Kind == "BoutPortrait" || c.Kind == "BoutPortraitResponse" {
			continue
		}
		t.Run(c.Name, func(t *testing.T) {
			b := boutReferenceBytes(t, doc.DTOs[c.Base].Canonical, c.Recipe)
			h := sha256.Sum256(b)
			if len(b) != c.InputBytes || hex.EncodeToString(h[:]) != c.InputSHA {
				t.Fatalf("materialization bytes%d/%d sha%x/%s", len(b), c.InputBytes, h, c.InputSHA)
			}
			var got []byte
			var err error
			switch c.Kind {
			case "BoutRequest":
				var v BoutRequest
				v, err = ParseBoutRequest(b)
				if err == nil {
					got, err = MarshalBoutRequest(v)
				}
			case "BoutResponse":
				var v BoutResponse
				v, err = ParseBoutResponse(b)
				if err == nil {
					got, err = MarshalBoutResponse(v)
				}
			case "BoutSnapshot":
				var v BoutSnapshot
				_, err = boutDecode(b, MaxBoutMetadataSize, 16, &v)
				if err == nil && !boutSnapshotValid(v) {
					err = gymbout.ErrInvalid
				}
				got, _ = json.Marshal(v)
			case "BoutJob":
				var v BoutJob
				_, err = boutDecode(b, MaxBoutJobSize, 16, &v)
				if err == nil && !boutJobValid(&v) {
					err = gymbout.ErrInvalid
				}
				got, _ = json.Marshal(v)
			case "Record":
				var v struct {
					Record struct {
						Record gymbout.Record `json:"record"`
					} `json:"record"`
				}
				_, err = boutDecode(append(append([]byte(`{"record":{"record":`), b...), []byte(`}}`)...), MaxBoutResponseSize, 16, &v)
				if err == nil && !boutRecordShape(v.Record.Record) {
					err = gymbout.ErrInvalid
				}
				got, _ = json.Marshal(v.Record.Record)
			case "BoutSource":
				var v struct {
					Record struct {
						Record struct {
							Source gymcard.BoutSource `json:"source"`
						} `json:"record"`
					} `json:"record"`
				}
				_, err = boutDecode(append(append([]byte(`{"record":{"record":{"source":`), b...), []byte(`}}}`)...), MaxBoutResponseSize, 16, &v)
				if err == nil && !boutSourceShape(v.Record.Record.Source) {
					err = gymbout.ErrInvalid
				}
				got, _ = json.Marshal(v.Record.Record.Source)
			default:
				t.Fatal("unknown transport kind", c.Kind)
			}
			if (err == nil) != c.Accept {
				t.Fatalf("accept%v want%v: %v", err == nil, c.Accept, err)
			}
			if err == nil {
				want := c.Canonical
				if c.CanonicalRef != nil {
					want = &doc.DTOs[*c.CanonicalRef].Canonical
				}
				if want != nil && !bytes.Equal(got, []byte(*want)) {
					t.Fatal("canonical mismatch")
				}
			}
		})
	}
}
