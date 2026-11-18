// Copyright (c) 2026 The SLUG developers. ISC license.
package gymresult

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"github.com/taytr-llc/slug/chaincfg/v2"
	"github.com/taytr-llc/slug/gymbout"
	"github.com/taytr-llc/slug/gymevent"
	"os"
	"testing"
)

type referencePositive struct {
	Name, Kind, Canonical, SHA256 string
	Value                         json.RawMessage
}
type referenceCase struct {
	Name, Kind, Raw, RawHex, Canonical string
	Accept                             bool
	Recipe                             *struct {
		Base        string
		AppendSpace int `json:"append_space"`
	}
}
type coreReference struct {
	Pin      Pin
	Positive []referencePositive
	Cases    []referenceCase
	Facts    map[string]json.RawMessage
}

func loadReference(t *testing.T) coreReference {
	t.Helper()
	raw, e := os.ReadFile("testdata/reference/fragments/core.json")
	if e != nil {
		t.Fatal(e)
	}
	var v coreReference
	if e = json.Unmarshal(raw, &v); e != nil {
		t.Fatal(e)
	}
	return v
}
func referenceParse(p Pin, kind string, raw []byte) ([]byte, error) {
	var v any
	var e error
	switch kind {
	case "Pin":
		v, e = ParsePin(raw)
	case "Info":
		v, e = ParseInfo(p, raw)
	case "Statement":
		v, e = ParseStatement(p, raw)
	case "SignedStatement":
		v, e = ParseSignedStatement(p, raw)
	case "Publication":
		v, e = ParsePublication(p, raw)
	case "Inclusion":
		v, e = ParseInclusion(p, raw)
	case "PriorInclusion":
		v, e = ParsePriorInclusion(p, raw)
	case "Finality":
		v, e = ParseFinality(p, raw)
	case "Conflict":
		v, e = ParseConflict(p, raw)
	case "Observation":
		v, e = ParseObservation(p, raw)
	case "Record":
		v, e = ParseRecord(p, raw)
	case "Status":
		v, e = ParseStatus(p, raw)
	case "ActionResult":
		v, e = ParseActionResult(p, raw)
	case "ReadResult":
		v, e = ParseReadResult(p, raw)
	case "Tip":
		v, e = ParseTip(p, raw)
	case "Outpoint":
		v, e = ParseOutpoint(p, raw)
	case "FundingCoin":
		v, e = ParseFundingCoin(p, raw)
	case "Anchor":
		v, e = ParseAnchor(p, raw)
	case "Cursor":
		v, e = ParseCursor(p, raw)
	case "ScanRestart":
		v, e = ParseScanRestart(p, raw)
	case "ProofSummary":
		v, e = ParseProofSummary(p, raw)
	case "ResultSource":
		v, e = gymbout.ParseResultSource(p.SourcePin, raw)
	case "ResultSummary":
		var x gymbout.ResultSummary
		e = decode(raw, 2048, &x, false)
		if e == nil {
			if x.EndRound < 1 || x.EndRound > 3 || x.EndTick < 1 || x.EndTick > 60 || !optionalID(x.Winner) || !oneOf(x.Method, "decision_draw", "double_corner_stoppage", "decision", "ko", "tko_referee", "tko_three_knockdowns", "tko_corner") || (x.Winner == "") != oneOf(x.Method, "decision_draw", "double_corner_stoppage") {
				e = ErrInvalid
			}
		}
		v = x
	case "SignedCheckpoint":
		var x chaincfg.SignedCheckpoint
		e = decode(raw, 4096, &x, false)
		if e == nil && !validCheckpoint(p, x) {
			e = ErrInvalid
		}
		v = x
	case "Marker":
		var x marker
		e = decode(raw, MaxMarkerSize, &x, false)
		id, _ := ID(p)
		if e == nil && (x.Application != "slug-candidate-results" || x.Version != 1 || x.StoreID != id || !equal(x.Pin, p)) {
			e = ErrInvalid
		}
		v = x
	case "Init":
		var x initIntent
		e = decode(raw, MaxMarkerSize, &x, false)
		id, _ := ID(p)
		if e == nil && (x.Version != 1 || x.StoreID != id || !equal(x.Pin, p) || !timestamp(x.SourceCreatedAtMS)) {
			e = ErrInvalid
		}
		v = x
	case "Metadata":
		var x metadata
		e = decode(raw, MaxMetadataSize, &x, false)
		id, _ := ID(p)
		if e == nil && !validMeta(x, p, id) {
			e = ErrInvalid
		}
		v = x
	case "Reservation":
		var x reservation
		e = decode(raw, 256, &x, false)
		if e == nil && (x.Version != 1 || !hashID(x.ResultKey) || !hashID(x.TxID)) {
			e = ErrInvalid
		}
		v = x
	case "IndexEntry":
		var x indexRow
		e = decode(raw, 512, &x, false)
		if e == nil {
			_, d := gymevent.EventForDate(p.SourcePin.SourcePin.SourcePin.SourcePin, x.Saturday)
			if d != nil || !hashID(x.ResultKey) || !hashID(x.BoutID) || !hashID(x.RecordDigest) {
				e = ErrInvalid
			}
		}
		v = x
	default:
		return nil, ErrInvalid
	}
	if e != nil {
		return nil, e
	}
	return json.Marshal(v)
}
func TestFrozenReferenceCore(t *testing.T) {
	f := loadReference(t)
	bases := map[string]string{}
	for _, x := range f.Positive {
		bases[x.Name] = x.Canonical
		t.Run("positive/"+x.Name, func(t *testing.T) {
			got, e := referenceParse(f.Pin, x.Kind, []byte(x.Canonical))
			if e != nil {
				t.Fatal(e)
			}
			if string(got) != x.Canonical {
				t.Fatalf("canonical mismatch for %s", x.Kind)
			}
		})
	}
	for _, x := range f.Cases {
		t.Run("codec/"+x.Name, func(t *testing.T) {
			raw := []byte(x.Raw)
			if x.RawHex != "" {
				var e error
				raw, e = hex.DecodeString(x.RawHex)
				if e != nil {
					t.Fatal(e)
				}
			}
			if x.Recipe != nil {
				base, ok := bases[x.Recipe.Base]
				if !ok {
					t.Fatal("missing recipe base")
				}
				raw = append([]byte(base), bytes.Repeat([]byte{' '}, x.Recipe.AppendSpace)...)
			}
			got, e := referenceParse(f.Pin, x.Kind, raw)
			if (e == nil) != x.Accept {
				t.Fatalf("accept=%v want=%v error=%v", e == nil, x.Accept, e)
			}
			if x.Accept && x.Canonical != "" && string(got) != x.Canonical {
				t.Fatal("canonical mismatch")
			}
		})
	}
	t.Logf("frozen positives=%d controls=%d", len(f.Positive), len(f.Cases))
}
