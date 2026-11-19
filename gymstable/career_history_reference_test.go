// Copyright (c) 2026 The SLUG developers. ISC license.
package gymstable

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/internal/careerstate"
)

type historyVector struct {
	Name  string `json:"name"`
	Input struct {
		Activation careerstate.Activation `json:"activation"`
		Events     []careerstate.Event    `json:"events"`
		Status     Status                 `json:"status"`
		Assignment Assignment             `json:"assignment"`
		AsOfMS     int64                  `json:"as_of_ms"`
		ClockMS    int64                  `json:"clock_ms"`
	} `json:"input"`
	Expected struct {
		Capture CareerHistoryCapture `json:"capture"`
		Rows    json.RawMessage      `json:"rows"`
		Pages   []struct {
			After uint32            `json:"after"`
			Limit uint32            `json:"limit"`
			Value CareerHistoryPage `json:"value"`
		} `json:"pages"`
	} `json:"expected"`
}
type historyControl struct {
	Name    string          `json:"name"`
	Kind    string          `json:"kind"`
	Value   json.RawMessage `json:"value"`
	Accept  bool            `json:"accept"`
	Input   string          `json:"input"`
	Context struct {
		Capture    CareerHistoryCapture   `json:"capture"`
		Activation careerstate.Activation `json:"activation"`
		Limit      *uint32                `json:"limit"`
	} `json:"context"`
}
type historyReference struct {
	History struct {
		Histories []historyVector `json:"histories"`
		DTOs      []struct {
			Name      string `json:"name"`
			Kind      string `json:"kind"`
			Canonical string `json:"canonical"`
			SHA256    string `json:"sha256"`
		} `json:"dtos"`
		Controls      []historyControl `json:"controls"`
		IndexControls []struct {
			Name    string              `json:"name"`
			Events  []careerstate.Event `json:"events"`
			AsOfMS  int64               `json:"as_of_ms"`
			ClockMS int64               `json:"clock_ms"`
			Accept  bool                `json:"accept"`
		} `json:"index_controls"`
	} `json:"history"`
}

func loadHistoryReference(t *testing.T) historyReference {
	t.Helper()
	raw, e := os.ReadFile("testdata/career-history-reference/corpus.json")
	if e != nil {
		t.Fatal(e)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "18e51e25c6760f756159e12a92265b517438152533d439d541d4696044e60283" {
		t.Fatal("frozen independent corpus changed")
	}
	var out historyReference
	if e = json.Unmarshal(raw, &out); e != nil {
		t.Fatal(e)
	}
	return out
}
func historyVectorReader(t *testing.T, v historyVector, calls *int) *careerstate.Reader {
	t.Helper()
	scope := careerstate.Scope{StableStoreIDs: [2]string{v.Input.Status.StoreID, strings.Repeat("e", 64)}, CardStoreID: strings.Repeat("f", 64)}
	r, e := careerstate.NewReader(v.Input.Activation, scope, func(context.Context) (careerstate.State, error) {
		*calls++
		return careerstate.State{Activation: v.Input.Activation, Scope: scope, ClockMS: v.Input.ClockMS, Events: v.Input.Events}, nil
	})
	if e != nil {
		t.Fatal(e)
	}
	return r
}

// The frozen Python corpus predates this implementation. Public-source cases
// retain old verification provenance; extra histories are explicitly models.
func TestCareerHistoryIndependentReference(t *testing.T) {
	x := loadHistoryReference(t)
	if len(x.History.Histories) != 28 || len(x.History.Controls) != 35 {
		t.Fatal("incomplete frozen corpus")
	}
	for _, v := range x.History.Histories {
		t.Run(v.Name, func(t *testing.T) {
			calls := 0
			r := historyVectorReader(t, v, &calls)
			h, e := r.CaptureHistory(t.Context(), v.Input.Assignment.ProspectID, v.Input.AsOfMS)
			if e != nil || calls != 1 {
				t.Fatal("capture must use one state", calls, e)
			}
			s := h.Snapshot()
			total, ok := s.Totals(v.Input.Assignment.ProspectID)
			if !ok || s.Mode() != careerstate.Current || !reflect.DeepEqual(s.ProspectIDs(), []string{v.Input.Assignment.ProspectID}) {
				t.Fatal("snapshot context")
			}
			st := v.Input.Status
			st.LastAsOfMS = v.Input.AsOfMS
			c := CareerHistoryCapture{Version: 1, Status: st, Activation: s.Activation(), CareerDigest: s.Digest(), AsOfMS: s.AsOfMS(), Assignment: v.Input.Assignment, PrefixCount: h.PrefixCount(), EventCount: h.EventCount(), Totals: total}
			c.HistoryID = CareerHistoryID(c)
			if c != v.Expected.Capture {
				t.Fatalf("capture differs from independent expected: got %+v", c)
			}
			if _, e = ValidateCareerHistoryCapture(c); e != nil {
				t.Fatal(e)
			}
			all := []CareerHistoryRow{}
			for after := uint32(0); after < h.EventCount(); after += 8 {
				rows, e := h.Rows(after, 8)
				if e != nil {
					t.Fatal(e)
				}
				all = append(all, rows...)
			}
			raw, _ := json.Marshal(all)
			if !bytes.Equal(raw, v.Expected.Rows) {
				t.Fatal("canonical independent rows differ")
			}
			for _, p := range v.Expected.Pages {
				rows, e := h.Rows(p.After, p.Limit)
				if e != nil {
					t.Fatal(e)
				}
				got := CareerHistoryPage{Version: 1, Capture: c, After: p.After, Next: p.After + uint32(len(rows)), Items: rows}
				got.HasMore = got.Next < c.EventCount
				if !reflect.DeepEqual(got, p.Value) {
					t.Fatal("independent page differs", p.After, p.Limit)
				}
				if _, e = ValidateCareerHistoryPageLimit(got, p.Limit); e != nil {
					t.Fatal(e)
				}
				enc, e := MarshalCareerHistoryPage(got)
				if e != nil {
					t.Fatal(e)
				}
				parsed, e := ParseCareerHistoryPage(enc)
				if e != nil || !reflect.DeepEqual(got, parsed) {
					t.Fatal("page codec", e)
				}
			}
			if calls != 1 || h.RetainedBytes()+8192+4096 > MaxCareerHistoryRetainedBytes {
				t.Fatal("recaptured owner or retained cap", calls, h.RetainedBytes())
			}
			// The unchanged original Snapshot equation must agree as a regression
			// comparison, in addition to the independent expected digest above.
			old, e := r.Snapshot(t.Context(), []string{v.Input.Assignment.ProspectID}, v.Input.AsOfMS, careerstate.Current)
			if e != nil || old.Digest() != s.Digest() {
				t.Fatal("old prefix equation changed", e)
			}
		})
	}
}
func TestCareerHistoryIndependentCodecs(t *testing.T) {
	x := loadHistoryReference(t)
	for _, d := range x.History.DTOs {
		t.Run(d.Name, func(t *testing.T) {
			var b []byte
			var e error
			switch d.Kind {
			case "CareerHistoryCapture":
				var v CareerHistoryCapture
				v, e = ParseCareerHistoryCapture([]byte(d.Canonical))
				if e == nil {
					b, e = MarshalCareerHistoryCapture(v)
				}
			case "CareerHistoryPage":
				var v CareerHistoryPage
				v, e = ParseCareerHistoryPage([]byte(d.Canonical))
				if e == nil {
					b, e = MarshalCareerHistoryPage(v)
				}
			default:
				t.Fatal("unknown frozen kind")
			}
			h := sha256.Sum256(b)
			if e != nil || string(b) != d.Canonical || hex.EncodeToString(h[:]) != d.SHA256 {
				t.Fatal("canonical frozen DTO mismatch", e)
			}
		})
	}
	for _, c := range x.History.Controls {
		t.Run(c.Name, func(t *testing.T) {
			var e error
			switch c.Kind {
			case "CareerHistoryCapture":
				_, e = ParseCareerHistoryCapture(c.Value)
			case "CareerHistoryRow":
				_, e = ParseCareerHistoryRow(c.Value, c.Context.Capture)
			case "CareerHistoryPage":
				var p CareerHistoryPage
				p, e = ParseCareerHistoryPage(c.Value)
				if e == nil && c.Context.Limit != nil {
					_, e = ValidateCareerHistoryPageLimit(p, *c.Context.Limit)
				}
			case "Event":
				var v careerstate.Event
				e = decode(c.Value, MaxCareerHistoryEventSize, 8, &v, false)
				if e == nil {
					_, e = careerstate.ValidateIndex(c.Context.Activation, []careerstate.Event{v})
				}
			case "lexical":
				var v struct {
					N uint32 `json:"n"`
				}
				e = decode([]byte(c.Input), 128, 2, &v, false)
			default:
				t.Fatal("unhandled control", c.Kind)
			}
			if (e == nil) != c.Accept {
				t.Fatal("acceptance disagrees with frozen control", c.Accept, e)
			}
		})
	}
	v := x.History.Histories[0]
	for _, c := range x.History.IndexControls {
		t.Run(c.Name, func(t *testing.T) {
			v.Input.Events = c.Events
			v.Input.AsOfMS = c.AsOfMS
			v.Input.ClockMS = c.ClockMS
			calls := 0
			r := historyVectorReader(t, v, &calls)
			h, e := r.CaptureHistory(t.Context(), v.Input.Assignment.ProspectID, c.AsOfMS)
			if (e == nil) != c.Accept || e != nil && h != nil {
				t.Fatal("index acceptance mismatch", c.Accept, e)
			}
		})
	}
}

func TestCareerHistoryClosedLexicalGrammar(t *testing.T) {
	x := loadHistoryReference(t)
	var raw string
	for _, d := range x.History.DTOs {
		if d.Kind == "CareerHistoryPage" && strings.Contains(d.Name, "genuine_win") {
			raw = d.Canonical
			break
		}
	}
	if raw == "" {
		t.Fatal("missing frozen page")
	}
	cases := map[string]string{
		"duplicate":     strings.Replace(raw, `"version":1`, `"version":1,"version":1`, 1),
		"unknown":       strings.Replace(raw, `"version":1`, `"extra":0,"version":1`, 1),
		"alias":         strings.Replace(raw, `"version":1`, `"Version":1`, 1),
		"missing":       strings.Replace(raw, `"version":1,`, "", 1),
		"bool":          strings.Replace(raw, `"ordinal":1`, `"ordinal":true`, 1),
		"fraction":      strings.Replace(raw, `"ordinal":1`, `"ordinal":1.0`, 1),
		"exponent":      strings.Replace(raw, `"ordinal":1`, `"ordinal":1e0`, 1),
		"negative-zero": strings.Replace(raw, `"after":0`, `"after":-0`, 1),
		"overflow":      strings.Replace(raw, `"ordinal":1`, `"ordinal":4294967296`, 1),
		"unsafe":        strings.Replace(raw, `"checkpoint":27`, `"checkpoint":9007199254740992`, 1),
		"trailing":      raw + ` {}`,
		"null":          strings.Replace(raw, `"items":[`, `"items":null,"other":[`, 1),
		"oversize":      raw + strings.Repeat(" ", MaxCareerHistoryPageSize),
		"invalid-utf8":  string(append([]byte(raw), 0xff)),
		"year-zero":     strings.Replace(raw, `"saturday":"2026-12-05"`, `"saturday":"0000-01-01"`, 1),
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			if input == raw {
				t.Fatal("mutation did not touch frozen input")
			}
			p, e := ParseCareerHistoryPage([]byte(input))
			if e == nil || p.Version != 0 {
				t.Fatal("malformed page disclosed", e)
			}
		})
	}
	// Object order is not authority: a reordered, whitespace-padded object
	// still canonicalizes to the exact already frozen declaration order.
	var obj map[string]json.RawMessage
	if e := json.Unmarshal([]byte(raw), &obj); e != nil {
		t.Fatal(e)
	}
	reordered, _ := json.Marshal(obj)
	p, e := ParseCareerHistoryPage(append([]byte(" \n"), reordered...))
	if e != nil {
		t.Fatal("valid reordered JSON", e)
	}
	canonical, e := MarshalCareerHistoryPage(p)
	if e != nil || string(canonical) != raw {
		t.Fatal("canonical order changed", e)
	}
}
