// Copyright (c) 2026 The SLUG developers. ISC license.
package pilot

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/taytr-llc/slug/internal/careerstate"
	"os"
	"testing"
)

type historyWireReference struct {
	Wire struct {
		DTOs []struct {
			ID, Kind  string
			Canonical string `json:"canonical_json"`
		}
		Codecs []struct {
			ID, Kind string
			Raw      string `json:"raw_base64"`
			SHA      string `json:"input_sha256"`
			Bytes    int    `json:"input_bytes"`
			Accept   bool
		}
	}
}

func historyReference(t *testing.T) historyWireReference {
	t.Helper()
	b, e := os.ReadFile("../../gymstable/testdata/career-history-reference/corpus.json")
	if e != nil {
		t.Fatal(e)
	}
	if fmt.Sprintf("%x", sha256.Sum256(b)) != "18e51e25c6760f756159e12a92265b517438152533d439d541d4696044e60283" {
		t.Fatal("frozen history corpus changed")
	}
	var c historyWireReference
	if e = json.Unmarshal(b, &c); e != nil {
		t.Fatal(e)
	}
	if len(c.Wire.DTOs) != 65 || len(c.Wire.Codecs) != 67 {
		t.Fatal("coverage changed")
	}
	return c
}
func parseHistoryReference(kind string, raw []byte) ([]byte, error) {
	if kind == "CareerHistoryRequest" {
		v, e := ParseCareerHistoryRequest(raw)
		if e != nil {
			return nil, e
		}
		return MarshalCareerHistoryRequest(v)
	}
	if kind == "CareerHistoryResponse" {
		v, e := ParseCareerHistoryResponse(raw)
		if e != nil {
			return nil, e
		}
		return MarshalCareerHistoryResponse(v)
	}
	arm := "snapshot"
	if kind == "CareerHistoryJob" {
		arm = "job"
	}
	if kind == "CareerHistoryPageResult" {
		arm = "page"
	}
	frames := map[string][]byte{"snapshot": []byte("null"), "job": []byte("null"), "page": []byte("null")}
	frames[arm] = raw
	input := []byte(`{"id":1,"version":1,"ok":true,"snapshot":`)
	input = append(input, frames["snapshot"]...)
	input = append(input, []byte(`,"job":`)...)
	input = append(input, frames["job"]...)
	input = append(input, []byte(`,"page":`)...)
	input = append(input, frames["page"]...)
	input = append(input, []byte(`,"error":""}`)...)
	v, e := ParseCareerHistoryResponse(input)
	if e != nil {
		return nil, e
	}
	switch arm {
	case "snapshot":
		return json.Marshal(v.Snapshot)
	case "job":
		return json.Marshal(v.Job)
	default:
		return json.Marshal(v.Page)
	}
}
func TestCareerHistoryIndependentWire(t *testing.T) {
	c := historyReference(t)
	for _, d := range c.Wire.DTOs {
		t.Run(d.ID, func(t *testing.T) {
			b, e := parseHistoryReference(d.Kind, []byte(d.Canonical))
			if e != nil || !bytes.Equal(b, []byte(d.Canonical)) {
				t.Fatalf("canonical: %v", e)
			}
		})
	}
	for _, d := range c.Wire.Codecs {
		t.Run(d.ID, func(t *testing.T) {
			b, e := base64.StdEncoding.DecodeString(d.Raw)
			if e != nil || len(b) != d.Bytes || fmt.Sprintf("%x", sha256.Sum256(b)) != d.SHA {
				t.Fatal("input recipe binding")
			}
			_, e = parseHistoryReference(d.Kind, b)
			if (e == nil) != d.Accept {
				t.Fatalf("accept %v want %v: %v", e == nil, d.Accept, e)
			}
		})
	}
}

func TestCareerHistoryRejectsYearZeroWithoutChangingLegacyEvent(t *testing.T) {
	for _, d := range historyReference(t).Wire.DTOs {
		if d.Kind != "CareerHistoryPageResult" {
			continue
		}
		var p CareerHistoryPageResult
		if e := json.Unmarshal([]byte(d.Canonical), &p); e != nil {
			t.Fatal(e)
		}
		if len(p.Page.Items) == 0 {
			continue
		}
		r := p.Page.Items[0]
		r.Event.Saturday = "0000-01-01"
		if _, e := careerstate.ValidateIndex(p.Page.Capture.Activation, []careerstate.Event{r.Event}); e != nil {
			t.Fatal("control is not accepted by inherited Event grammar", e)
		}
		if historyRowValid(r, p.Page.Capture) {
			t.Fatal("new History context accepted year zero")
		}
		return
	}
	t.Fatal("no nonempty independent page")
}
