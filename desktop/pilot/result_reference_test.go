// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymresult"
	"github.com/taytr-llc/slug/wire/v2"
	"os"
	"strings"
	"testing"
)

type resultReferenceCase struct {
	Category, Kind, Name, Canonical, Base string
	Accept                                bool
	CanonicalRef                          string `json:"canonical_ref"`
	InputSHA                              string `json:"input_sha256"`
	InputBytes                            int    `json:"input_bytes"`
	Recipe                                struct {
		Edits []struct {
			Op    string
			Path  []any
			Value json.RawMessage
		}
		Style   string
		Replace []string `json:"replace_once"`
		Pad     *int     `json:"pad_bytes"`
		Raw     string   `json:"raw_hex"`
		Append  string   `json:"append_hex"`
		Rehash  bool     `json:"rehash_record_status"`
	}
}

func resultReferenceBytes(t *testing.T, base string, c resultReferenceCase) []byte {
	t.Helper()
	adapt := cardReferenceRecipe{}
	for _, e := range c.Recipe.Edits {
		keys := []string{}
		for _, k := range e.Path {
			keys = append(keys, fmt.Sprint(k))
		}
		adapt.Edits = append(adapt.Edits, struct {
			Op, Path string
			Value    json.RawMessage
		}{e.Op, "/" + strings.Join(keys, "/"), e.Value})
	}
	b := cardReferenceBytes(t, base, adapt)
	if c.Recipe.Rehash {
		d := json.NewDecoder(bytes.NewReader(b))
		d.UseNumber()
		n, e := seedReferenceParse(d)
		if e != nil {
			t.Fatal(e)
		}
		rr, _ := cardReferenceChild(t, n, "record")
		row, _ := cardReferenceChild(t, rr, "record")
		st, _ := cardReferenceChild(t, rr, "status")
		_, i := cardReferenceChild(t, st, "record_digest")
		raw := row.encode(false)
		h := sha256.New()
		h.Write([]byte("SLUG candidate result record\x00v1\x00"))
		h.Write(resultLE(uint32(len(raw))))
		h.Write(raw)
		v, _ := json.Marshal(hex.EncodeToString(h.Sum(nil)))
		dec := json.NewDecoder(bytes.NewReader(v))
		st.values[i], e = seedReferenceParse(dec)
		if e != nil {
			t.Fatal(e)
		}
		b = n.encode(false)
	}
	switch c.Recipe.Style {
	case "", "canonical":
	case "reversed":
		d := json.NewDecoder(bytes.NewReader(b))
		d.UseNumber()
		n, e := seedReferenceParse(d)
		if e != nil {
			t.Fatal(e)
		}
		b = n.encode(true)
	case "whitespace":
		var dst bytes.Buffer
		if json.Indent(&dst, b, "", "  ") != nil {
			t.Fatal("indent")
		}
		b = dst.Bytes()
	default:
		t.Fatal("style")
	}
	if len(c.Recipe.Replace) > 0 {
		if len(c.Recipe.Replace) != 2 || !bytes.Contains(b, []byte(c.Recipe.Replace[0])) {
			t.Fatal("replace")
		}
		b = bytes.Replace(b, []byte(c.Recipe.Replace[0]), []byte(c.Recipe.Replace[1]), 1)
	}
	if c.Recipe.Raw != "" {
		var e error
		b, e = hex.DecodeString(c.Recipe.Raw)
		if e != nil {
			t.Fatal(e)
		}
	}
	if c.Recipe.Pad != nil {
		if len(b) > *c.Recipe.Pad {
			t.Fatal("pad")
		}
		b = append(b, bytes.Repeat([]byte{' '}, *c.Recipe.Pad-len(b))...)
	}
	if c.Recipe.Append != "" {
		v, e := hex.DecodeString(c.Recipe.Append)
		if e != nil {
			t.Fatal(e)
		}
		b = append(b, v...)
	}
	return b
}
func resultReferenceParseTransport(kind string, b []byte) (out []byte, err error) {
	switch kind {
	case "ResultRequest":
		var v ResultRequest
		v, err = ParseResultRequest(b)
		if err == nil {
			out, err = MarshalResultRequest(v)
		}
	case "ResultResponse":
		var v ResultResponse
		v, err = ParseResultResponse(b)
		if err == nil {
			out, err = MarshalResultResponse(v)
		}
	case "ResultSnapshot":
		var v ResultSnapshot
		_, err = resultDecode(b, MaxResultMetadataSize, 20, &v)
		if err == nil && !resultSnapshotValid(v) {
			err = gymresult.ErrInvalid
		}
		out, _ = json.Marshal(v)
	case "ResultJob":
		var v ResultJob
		_, err = resultDecode(b, MaxResultJobSize, 20, &v)
		if err == nil && !resultJobValid(&v) {
			err = gymresult.ErrInvalid
		}
		out, _ = json.Marshal(v)
	default:
		err = fmt.Errorf("kind %s", kind)
	}
	return
}
func TestResultIndependentTransport(t *testing.T) {
	if wire.TestPoW {
		t.Skip("normal profile reference")
	}
	raw, e := os.ReadFile("../../gymresult/testdata/reference/fragments/wire.json")
	if e != nil {
		t.Fatal(e)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "aa60fdb0267a77365039d95545b61e8a4adb256bad8366325d89fdf081611408" {
		t.Fatal("frozen wire fragment changed")
	}
	var core struct{ Pin gymresult.Pin }
	rawCore, e := os.ReadFile("../../gymresult/testdata/reference/fragments/core.json")
	if e != nil || json.Unmarshal(rawCore, &core) != nil {
		t.Fatal("core context")
	}
	bound := func(kind string, b []byte) error {
		var job *ResultJob
		var response ResultResponse
		if kind == "ResultJob" {
			var j ResultJob
			if json.Unmarshal(b, &j) != nil {
				return gymresult.ErrInvalid
			}
			job = &j
		} else if kind == "ResultResponse" {
			if json.Unmarshal(b, &response) != nil {
				return gymresult.ErrInvalid
			}
			job = response.Job
		}
		if job != nil {
			ev, e := gymevent.EventForDate(core.Pin.SourcePin.SourcePin.SourcePin.SourcePin, job.Saturday)
			if e != nil {
				return e
			}
			card, e := gymcard.CardID(core.Pin.SourcePin.SourcePin, ev)
			if e != nil {
				return e
			}
			if card != job.CardID || job.ResultKey != resultKeyFor(core.Pin, card, job.BoutID) {
				return gymresult.ErrInvalid
			}
		}
		return nil
	}
	var d struct{ Cases []resultReferenceCase }
	if e = json.Unmarshal(raw, &d); e != nil {
		t.Fatal(e)
	}
	dtos := map[string]resultReferenceCase{}
	for _, c := range d.Cases {
		if c.Category == "dto" {
			dtos[c.Name] = c
			t.Run(c.Name, func(t *testing.T) {
				got, e := resultReferenceParseTransport(c.Kind, []byte(c.Canonical))
				if e != nil {
					t.Fatal(e)
				}
				if string(got) != c.Canonical {
					t.Fatal("canonical")
				}
			})
		}
	}
	for _, c := range d.Cases {
		if c.Category != "codec" {
			continue
		}
		t.Run(c.Name, func(t *testing.T) {
			base, ok := dtos[c.Base]
			if !ok {
				t.Fatal("base")
			}
			b := resultReferenceBytes(t, base.Canonical, c)
			h := sha256.Sum256(b)
			if len(b) != c.InputBytes || hex.EncodeToString(h[:]) != c.InputSHA {
				t.Fatalf("materialization %d/%d %x/%s", len(b), c.InputBytes, h, c.InputSHA)
			}
			got, e := resultReferenceParseTransport(c.Kind, b)
			if e == nil {
				e = bound(c.Kind, b)
			}
			if (e == nil) != c.Accept {
				t.Fatalf("accepted %v want %v: %v", e == nil, c.Accept, e)
			}
			if e == nil {
				want := c.Canonical
				if c.CanonicalRef != "" {
					want = dtos[c.CanonicalRef].Canonical
				}
				if string(got) != want {
					t.Fatal("canonical")
				}
			}
		})
	}
}
