// Copyright (c) 2026 The SLUG developers. ISC license.
package gymbout

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/taytr-llc/slug/fighterbout"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/wire/v2"
	"os"
	"strconv"
	"strings"
	"testing"
)

const referenceSHA = "09f0430789328b9226fc4a24aae3cdb07beeb8b41d61b42cd33e4ba3121fa2ec"

type referenceComposition struct {
	Name, Provenance string
	CardPin          gymcard.Pin `json:"card_pin"`
	Pin              Pin
	Source           gymcard.BoutSource
	SourceCreated    int64 `json:"source_store_created_at_ms"`
	StoreCreated     int64 `json:"store_created_at_ms"`
	RunAt            int64 `json:"run_at_ms"`
	Input            fighterbout.Input
	InputID          string `json:"input_id"`
	Result           fighterbout.Result
	ResultDigest     string `json:"result_digest"`
	Record           Record
	RecordDigest     string `json:"record_digest"`
	CanonicalSource  string `json:"canonical_source"`
	CanonicalInput   string `json:"canonical_input"`
	CanonicalResult  string `json:"canonical_result"`
	CanonicalRecord  string `json:"canonical_record"`
}
type referenceDTO struct {
	Kind, Name, Context, Canonical string
	FixtureIndex                   int `json:"fixture_index"`
	Value                          json.RawMessage
}
type referenceRecipeSpec struct {
	Op, Path, Old, New, Key string
	Value                   json.RawMessage
	Bytes                   int
	Edits                   []struct {
		Path  string
		Value json.RawMessage
	}
}
type referenceCase struct {
	Name, Kind, Context string
	FixtureIndex        int `json:"fixture_index"`
	Base                int
	Recipe              referenceRecipeSpec
	Accept              bool
	InputSHA            string `json:"input_sha256"`
	InputBytes          int    `json:"input_bytes"`
	CanonicalRef        *int   `json:"canonical_ref"`
	Canonical           *string
}
type boutReference struct {
	Rules        struct{ ID string }
	Compositions []referenceComposition
	DTOs         []referenceDTO
	Cases        []referenceCase `json:"codec_cases"`
	Calendar     []struct {
		Saturday string
		Cutoff   int64 `json:"cutoff_ms"`
		Deadline int64 `json:"incident_deadline_ms"`
	} `json:"calendar_cases"`
}

func readReference(t testing.TB) boutReference {
	t.Helper()
	raw, e := os.ReadFile("testdata/reference/bout_v1.json")
	if e != nil {
		t.Fatal(e)
	}
	if fmt.Sprintf("%x", sha256.Sum256(raw)) != referenceSHA {
		t.Fatal("frozen reference changed")
	}
	var c boutReference
	if e = json.Unmarshal(raw, &c); e != nil {
		t.Fatal(e)
	}
	return c
}
func requireNormal(t *testing.T) {
	t.Helper()
	if wire.TestPoW {
		t.Skip("normal authority profile")
	}
}

type referenceJSON struct {
	Kind     byte
	Keys     []string
	Children []*referenceJSON
	Scalar   any
}

func referenceTree(raw []byte) *referenceJSON {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var read func() *referenceJSON
	read = func() *referenceJSON {
		v, e := d.Token()
		if e != nil {
			panic(e)
		}
		n := &referenceJSON{Scalar: v}
		if q, ok := v.(json.Delim); ok {
			n.Kind = byte(q)
			for d.More() {
				if q == '{' {
					k, _ := d.Token()
					n.Keys = append(n.Keys, k.(string))
				}
				n.Children = append(n.Children, read())
			}
			d.Token()
		}
		return n
	}
	return read()
}
func (n *referenceJSON) encode(reverse bool) []byte {
	if n.Kind == 0 {
		b, _ := json.Marshal(n.Scalar)
		return b
	}
	var b bytes.Buffer
	b.WriteByte(n.Kind)
	for pos := range n.Children {
		if pos > 0 {
			b.WriteByte(',')
		}
		i := pos
		if reverse && n.Kind == '{' {
			i = len(n.Children) - 1 - pos
		}
		if n.Kind == '{' {
			k, _ := json.Marshal(n.Keys[i])
			b.Write(k)
			b.WriteByte(':')
		}
		b.Write(n.Children[i].encode(reverse))
	}
	if n.Kind == '{' {
		b.WriteByte('}')
	} else {
		b.WriteByte(']')
	}
	return b.Bytes()
}
func (n *referenceJSON) child(k string) (int, *referenceJSON) {
	if n.Kind == '[' {
		i, e := strconv.Atoi(k)
		if e != nil || i < 0 || i >= len(n.Children) {
			return -1, nil
		}
		return i, n.Children[i]
	}
	for i, key := range n.Keys {
		if k == key {
			return i, n.Children[i]
		}
	}
	return -1, nil
}

func referenceInput(t *testing.T, c boutReference, row referenceCase) []byte {
	t.Helper()
	n := referenceTree([]byte(c.DTOs[row.Base].Canonical))
	r := row.Recipe
	edit := func(path string, value json.RawMessage, remove bool) {
		parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
		parent := n
		for _, p := range parts[:len(parts)-1] {
			_, parent = parent.child(p)
		}
		k := parts[len(parts)-1]
		i, _ := parent.child(k)
		if remove {
			parent.Children = append(parent.Children[:i], parent.Children[i+1:]...)
			parent.Keys = append(parent.Keys[:i], parent.Keys[i+1:]...)
		} else if i < 0 {
			parent.Keys = append(parent.Keys, k)
			parent.Children = append(parent.Children, referenceTree(value))
		} else {
			parent.Children[i] = referenceTree(value)
		}
	}
	switch r.Op {
	case "set":
		edit(r.Path, r.Value, false)
	case "remove":
		edit(r.Path, nil, true)
	case "set_many":
		for _, e := range r.Edits {
			edit(e.Path, e.Value, false)
		}
	}
	b := n.encode(false)
	switch r.Op {
	case "identity", "set", "remove", "set_many":
	case "whitespace_reversed":
		var pretty bytes.Buffer
		json.Indent(&pretty, n.encode(true), "", " ")
		b = append([]byte(" \n\t"), pretty.Bytes()...)
		b = append(b, []byte("\r\n")...)
	case "duplicate_top", "escaped_key_duplicate":
		key := r.Key
		if key == "" {
			key = n.Keys[0]
		}
		_, v := n.child(key)
		k, _ := json.Marshal(key)
		if r.Op == "escaped_key_duplicate" {
			k = []byte(fmt.Sprintf("\"\\u%04x%s\"", key[0], key[1:]))
		}
		b = append(b[:len(b)-1], ',')
		b = append(b, k...)
		b = append(b, ':')
		b = append(b, v.encode(false)...)
		b = append(b, '}')
	case "replace_once":
		b = bytes.Replace(b, []byte(r.Old), []byte(r.New), 1)
	case "pad":
		b = append(b, bytes.Repeat([]byte{' '}, r.Bytes-len(b))...)
	default:
		t.Fatal("unknown recipe", r.Op)
	}
	if len(b) != row.InputBytes || fmt.Sprintf("%x", sha256.Sum256(b)) != row.InputSHA {
		t.Fatalf("recipe bytes differ: %s %d/%d sha %x", row.Name, len(b), row.InputBytes, sha256.Sum256(b))
	}
	return b
}
func referenceCodec(ctx context.Context, p Pin, kind string, raw []byte) ([]byte, error, bool) {
	switch kind {
	case "Marker":
		var v marker
		if e := decode(raw, MaxMarkerSize, &v, false); e != nil {
			return nil, e, true
		}
		id, _ := ID(p)
		if v.Application != "SLUG candidate bouts" || v.Version != 1 || v.StoreID != id || !equal(v.Pin, p) {
			return nil, ErrInvalid, true
		}
		b, e := json.Marshal(v)
		return b, e, true
	case "Init":
		var v initIntent
		if e := decode(raw, MaxMetadataSize, &v, false); e != nil {
			return nil, e, true
		}
		id, _ := ID(p)
		if v.Version != 1 || v.StoreID != id || !equal(v.Pin, p) || !timestamp(v.SourceCreatedAtMS) {
			return nil, ErrInvalid, true
		}
		b, e := json.Marshal(v)
		return b, e, true
	case "Metadata":
		var v metadata
		if e := decode(raw, MaxMetadataSize, &v, false); e != nil {
			return nil, e, true
		}
		info := Info{Version: v.Version, StoreID: v.StoreID, Pin: v.Pin, SourceCreatedAtMS: v.SourceCreatedAtMS, CreatedAtMS: v.CreatedAtMS, BoutCount: v.BoutCount}
		if validateInfo(p, info) != nil || !timestamp(v.ClockMS) || v.ClockMS < v.CreatedAtMS {
			return nil, ErrInvalid, true
		}
		b, e := json.Marshal(v)
		return b, e, true

	case "Pin":
		v, e := ParsePin(raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalPin(v)
		return b, e, true
	case "Info":
		v, e := ParseInfo(p, raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalInfo(p, v)
		return b, e, true
	case "SavedBout":
		v, e := ParseSavedBout(p, raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalSavedBout(p, v)
		return b, e, true
	case "Status":
		v, e := ParseStatus(p, raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalStatus(p, v)
		return b, e, true
	case "CardStatus":
		v, e := ParseCardStatus(p, raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalCardStatus(p, v)
		return b, e, true
	case "Record":
		v, e := ParseRecord(ctx, p, raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalRecord(ctx, p, v)
		return b, e, true
	case "RunResult":
		v, e := ParseRunResult(ctx, p, raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalRunResult(ctx, p, v)
		return b, e, true
	case "ReadResult":
		v, e := ParseReadResult(ctx, p, raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalReadResult(ctx, p, v)
		return b, e, true
	case "BoutSource":
		v, e := gymcard.ParseBoutSource(p.SourcePin, raw)
		if e != nil {
			return nil, e, true
		}
		b, e := gymcard.MarshalBoutSource(p.SourcePin, v)
		return b, e, true
	case "BoutState":
		v, e := gymcard.ParseBoutState(p.SourcePin, raw)
		if e != nil {
			return nil, e, true
		}
		b, e := gymcard.MarshalBoutState(p.SourcePin, v)
		return b, e, true
	case "BoutView":
		v, e := gymcard.ParseBoutView(p.SourcePin, raw)
		if e != nil {
			return nil, e, true
		}
		b, e := gymcard.MarshalBoutView(p.SourcePin, v)
		return b, e, true
	case "BoutRef":
		v, e := gymcard.ParseBoutRef(p.SourcePin, raw)
		if e != nil {
			return nil, e, true
		}
		b, e := gymcard.MarshalBoutRef(p.SourcePin, v)
		return b, e, true
	case "Bout":
		v, e := gymcard.ParseBout(p.SourcePin, raw)
		if e != nil {
			return nil, e, true
		}
		b, e := gymcard.MarshalBout(p.SourcePin, v)
		return b, e, true
	}
	return nil, nil, false
}

func TestBoutReferenceCodecs(t *testing.T) {
	requireNormal(t)
	c := readReference(t)
	count := 0
	ctx := context.Background()
	for _, row := range c.Cases {
		if row.Context != "core" {
			continue
		}
		p := c.Compositions[row.FixtureIndex].Pin
		_, _, ok := referenceCodec(ctx, p, row.Kind, []byte("{}"))
		if !ok {
			continue
		}
		count++
		t.Run(row.Name, func(t *testing.T) {
			b := referenceInput(t, c, row)
			out, e, _ := referenceCodec(ctx, p, row.Kind, b)
			if (e == nil) != row.Accept {
				t.Fatalf("accepted=%v expected=%v error=%v", e == nil, row.Accept, e)
			}
			if e == nil {
				expected := ""
				if row.CanonicalRef != nil {
					expected = c.DTOs[*row.CanonicalRef].Canonical
				} else if row.Canonical != nil {
					expected = *row.Canonical
				} else {
					t.Fatal("missing canonical expectation")
				}
				if string(out) != expected {
					t.Fatalf("canonical mismatch got %x expected %x", sha256.Sum256(out), sha256.Sum256([]byte(expected)))
				}
			}
		})
	}
	t.Logf("%d independently frozen core/source codec recipes", count)
}
func TestBoutReferenceCompositionsAndCalendar(t *testing.T) {
	requireNormal(t)
	c := readReference(t)
	ctx := context.Background()
	for _, x := range c.Compositions {
		t.Run(x.Name, func(t *testing.T) {
			p, e := CandidatePin(x.CardPin)
			if e != nil || !equal(p, x.Pin) {
				t.Fatal("candidate pin", e)
			}
			in, e := DeriveInput(p, x.Source)
			if e != nil || !equal(in, x.Input) || inputDigest(in) != x.InputID {
				t.Fatal("derived input", e)
			}
			r, e := fighterbout.Simulate(ctx, in)
			if e != nil || !equal(r, x.Result) || resultDigest(r) != x.ResultDigest {
				t.Fatal("unchanged engine full transcript", e)
			}
			b, e := MarshalRecord(ctx, p, x.Record)
			if e != nil || string(b) != x.CanonicalRecord || recordDigest(x.Record) != x.RecordDigest {
				t.Fatal("record/domain identity", e)
			}
		})
	}
	for _, x := range c.Calendar {
		v, e := gymevent.EventForDate(c.Compositions[0].Pin.SourcePin.SourcePin.SourcePin, x.Saturday)
		if e != nil || v.CutoffMS != x.Cutoff || v.IncidentDeadlineMS != x.Deadline {
			t.Fatal("calendar", x.Saturday, e)
		}
	}
	t.Logf("%d compositions and %d calendar dates; original actual booking is conflicted, arithmetic fixture is not new Run authority", len(c.Compositions), len(c.Calendar))
}
