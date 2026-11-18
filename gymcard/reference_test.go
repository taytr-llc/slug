// Copyright (c) 2026 The SLUG developers. ISC license.
package gymcard

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/taytr-llc/slug/gymevent"
	"github.com/taytr-llc/slug/gymseed"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const cardReferenceSHA = "c52f513f8dfda34c884312c6a2caaf93b429bbf788aaf7c2cba3fbd901329790"

type cardReference struct {
	Pin struct {
		Value   Pin
		StoreID string `json:"store_id"`
	}
	Rules struct{ ID string }
	DTOs  []struct {
		Kind, Name, Context, Canonical string
		Value                          json.RawMessage
	}
	Cases []struct {
		Name, Kind, Context string
		Base                int
		Recipe              struct {
			Edits []struct {
				Op, Path string
				Value    json.RawMessage
			}
			Style   string
			Raw     *string
			Replace []string `json:"replace_once"`
			Pad     int      `json:"pad_bytes"`
		}
		Accept       bool
		InputBytes   int    `json:"input_bytes"`
		InputSHA     string `json:"input_sha256"`
		CanonicalSHA string `json:"canonical_sha256"`
	} `json:"codec_cases"`
	Matcher []struct {
		Name      string
		Inputs    json.RawMessage
		CardID    string `json:"card_id"`
		InputID   string `json:"input_id"`
		Seed      string
		Accept    bool
		Plan      *Plan
		Canonical string
	} `json:"matcher_cases"`
	Calendar []struct {
		Saturday string
		Event    gymevent.Event
		CardID   string `json:"card_id"`
	}
	Identity []struct {
		Name, Scope  string
		Source       gymseed.CardSource
		SourceDigest string `json:"source_digest"`
		CardID       string `json:"card_id"`
		InputID      string `json:"input_id"`
		Seed         string
		BoutIDs      []string `json:"bout_ids"`
		BoutSeeds    []string `json:"bout_seeds"`
	} `json:"identity_cases"`
}

func readCardReference(t testing.TB) cardReference {
	t.Helper()
	b, e := os.ReadFile("testdata/reference/card_v1.json")
	if e != nil {
		t.Fatal(e)
	}
	if fmt.Sprintf("%x", sha256.Sum256(b)) != cardReferenceSHA {
		t.Fatal("frozen independent corpus changed")
	}
	var c cardReference
	if e = json.Unmarshal(b, &c); e != nil {
		t.Fatal(e)
	}
	return c
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

func referenceCardCodec(p Pin, kind string, raw []byte) ([]byte, error, bool) {
	switch kind {
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
	case "ProfessionalRecord":
		v, e := ParseProfessionalRecord(p, raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalProfessionalRecord(p, v)
		return b, e, true
	case "ReasonCounts":
		v, e := ParseReasonCounts(p, raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalReasonCounts(p, v)
		return b, e, true
	case "Counts":
		v, e := ParseCounts(p, raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalCounts(p, v)
		return b, e, true
	case "GymCounts":
		v, e := ParseGymCounts(p, raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalGymCounts(p, v)
		return b, e, true
	case "Entrant":
		v, e := ParseEntrant(p, raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalEntrant(p, v)
		return b, e, true
	case "Bout":
		v, e := ParseBout(p, raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalBout(p, v)
		return b, e, true
	case "Plan":
		v, e := ParsePlan(p, raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalPlan(p, v)
		return b, e, true
	case "Record":
		v, e := ParseRecord(p, raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalRecord(p, v)
		return b, e, true
	case "Status":
		v, e := ParseStatus(p, raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalStatus(p, v)
		return b, e, true
	case "BuildResult":
		v, e := ParseBuildResult(p, raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalBuildResult(p, v)
		return b, e, true
	case "ReadResult":
		v, e := ParseReadResult(p, raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalReadResult(p, v)
		return b, e, true
	case "HistoryPage":
		v, e := ParseHistoryPage(p, raw)
		if e != nil {
			return nil, e, true
		}
		b, e := MarshalHistoryPage(p, v)
		return b, e, true
	case "CardSource":
		v, e := gymseed.ParseCardSource(p.SourcePin, raw)
		if e != nil {
			return nil, e, true
		}
		b, e := gymseed.MarshalCardSource(p.SourcePin, v)
		return b, e, true
	case "CardState":
		v, e := gymseed.ParseCardState(p.SourcePin, raw)
		if e != nil {
			return nil, e, true
		}
		b, e := gymseed.MarshalCardState(p.SourcePin, v)
		return b, e, true
	case "CardView":
		v, e := gymseed.ParseCardView(p.SourcePin, raw)
		if e != nil {
			return nil, e, true
		}
		b, e := gymseed.MarshalCardView(p.SourcePin, v)
		return b, e, true
	}
	return nil, nil, false
}

func referenceRecipe(t *testing.T, c cardReference, i int) []byte {
	t.Helper()
	row := c.Cases[i]
	n := referenceTree([]byte(c.DTOs[row.Base].Canonical))
	for _, e := range row.Recipe.Edits {
		parts := strings.Split(e.Path, "/")[1:]
		for j := range parts {
			parts[j] = strings.ReplaceAll(strings.ReplaceAll(parts[j], "~1", "/"), "~0", "~")
		}
		parent := n
		for _, s := range parts[:len(parts)-1] {
			_, parent = parent.child(s)
		}
		key := parts[len(parts)-1]
		j, node := parent.child(key)
		switch e.Op {
		case "set":
			value := referenceTree(e.Value)
			if j < 0 {
				parent.Keys = append(parent.Keys, key)
				parent.Children = append(parent.Children, value)
			} else {
				parent.Children[j] = value
			}
		case "delete":
			parent.Children = append(parent.Children[:j], parent.Children[j+1:]...)
			if parent.Kind == '{' {
				parent.Keys = append(parent.Keys[:j], parent.Keys[j+1:]...)
			}
		case "reverse":
			for a, b := 0, len(node.Children)-1; a < b; a, b = a+1, b-1 {
				node.Children[a], node.Children[b] = node.Children[b], node.Children[a]
			}
		default:
			t.Fatal("unknown independent recipe")
		}
	}
	b := n.encode(false)
	if row.Recipe.Style == "reversed_whitespace" {
		var pretty bytes.Buffer
		if e := json.Indent(&pretty, n.encode(true), "", " "); e != nil {
			t.Fatal(e)
		}
		b = pretty.Bytes()
	}
	if row.Recipe.Raw != nil {
		b = []byte(*row.Recipe.Raw)
	}
	if len(row.Recipe.Replace) > 0 {
		b = bytes.Replace(b, []byte(row.Recipe.Replace[0]), []byte(row.Recipe.Replace[1]), 1)
	}
	if row.Recipe.Pad > 0 {
		b = append(b, bytes.Repeat([]byte{' '}, row.Recipe.Pad-len(b))...)
	}
	if len(b) != row.InputBytes || fmt.Sprintf("%x", sha256.Sum256(b)) != row.InputSHA {
		t.Fatalf("recipe input mismatch %s bytes %d/%d sha %x", row.Name, len(b), row.InputBytes, sha256.Sum256(b))
	}
	return b
}
func TestCardReferencePublicCodecs(t *testing.T) {
	requireNormal(t)
	c := readCardReference(t)
	checked := 0
	for i, row := range c.Cases {
		if row.Context != "core" {
			continue
		}
		_, _, supported := referenceCardCodec(c.Pin.Value, row.Kind, []byte("{}"))
		if !supported {
			continue
		}
		checked++
		t.Run(row.Name, func(t *testing.T) {
			b := referenceRecipe(t, c, i)
			out, e, _ := referenceCardCodec(c.Pin.Value, row.Kind, b)
			if (e == nil) != row.Accept {
				t.Fatalf("accept=%v expected=%v error=%v", e == nil, row.Accept, e)
			}
			if e == nil && fmt.Sprintf("%x", sha256.Sum256(out)) != row.CanonicalSHA {
				t.Fatalf("canonical mismatch sha %x expected %s", sha256.Sum256(out), row.CanonicalSHA)
			}
		})
	}
	t.Logf("%d exact frozen public core/source codec recipes; private storage and helper codecs have separate coverage", checked)
}
func TestCardReferenceDerivationCalendarMatcher(t *testing.T) {
	requireNormal(t)
	c := readCardReference(t)
	id, e := ID(c.Pin.Value)
	if e != nil || id != c.Pin.StoreID || RulesID() != c.Rules.ID {
		t.Fatal("identity", e)
	}
	for _, d := range c.DTOs {
		if d.Kind == "Record" && d.Context == "core" {
			r, e := ParseRecord(c.Pin.Value, []byte(d.Canonical))
			if e != nil {
				t.Fatal(d.Name, e)
			}
			p, e := DerivePlan(c.Pin.Value, r.Source)
			if e != nil || !equal(p, r.Plan) {
				t.Fatal("full expected plan", d.Name, e)
			}
		}
	}
	for _, x := range c.Identity {
		if !strings.HasPrefix(x.Scope, "validated_") {
			if _, e := gymseed.ValidateCardSource(c.Pin.Value.SourcePin, x.Source); e == nil {
				t.Fatal("recipe-only source admitted", x.Name)
			}
			continue
		}
		p, e := DerivePlan(c.Pin.Value, x.Source)
		d, de := gymseed.CardSourceDigest(c.Pin.Value.SourcePin, x.Source)
		if e != nil || de != nil || d != x.SourceDigest || p.CardID != x.CardID || p.InputID != x.InputID || p.Seed != x.Seed {
			t.Fatal("source identity", x.Name, e, de)
		}
		for i, b := range p.Bouts {
			if b.BoutID != x.BoutIDs[i] || b.Seed != x.BoutSeeds[i] {
				t.Fatal("bout preimage", x.Name)
			}
		}
	}
	for _, x := range c.Calendar {
		e, err := gymevent.EventForDate(c.Pin.Value.SourcePin.SourcePin, x.Saturday)
		if err != nil || !equal(e, x.Event) {
			t.Fatal("calendar", x.Saturday, err)
		}
		id, err := CardID(c.Pin.Value, e)
		if err != nil || id != x.CardID {
			t.Fatal("calendar card ID", x.Saturday, err)
		}
	}
	for _, x := range c.Matcher {
		var entrants []Entrant
		if e := json.Unmarshal(bytes.ReplaceAll(x.Inputs, []byte(`"class_":`), []byte(`"class":`)), &entrants); e != nil {
			if x.Accept {
				t.Fatal("accepted matcher scalar", x.Name, e)
			}
			continue
		}
		sort.Slice(entrants, func(i, j int) bool {
			a, b := entrants[i], entrants[j]
			if a.Class != b.Class {
				return classIndex(a.Class) < classIndex(b.Class)
			}
			if a.GymID != b.GymID {
				return a.GymID < b.GymID
			}
			return a.ProspectID < b.ProspectID
		})
		v := Plan{Version: 1, CardID: x.CardID, InputID: x.InputID, Seed: x.Seed, RecordBasis: RecordBasis, Entrants: append([]Entrant{}, entrants...), Gyms: []GymCounts{}, Bouts: []Bout{}}
		v = match(c.Pin.Value, v)
		_, e := ValidatePlan(c.Pin.Value, v)
		if (e == nil) != x.Accept {
			t.Fatal("scalar matcher refusal", x.Name, e)
		}
		if e == nil {
			b, _ := MarshalPlan(c.Pin.Value, v)
			if string(b) != x.Canonical {
				t.Fatal("scalar matcher output", x.Name)
			}
		}
	}
	t.Logf("%d calendar identities, %d matcher cases, %d source identity controls", len(c.Calendar), len(c.Matcher), len(c.Identity))
}

func TestCardStandaloneStatusCannotInferRetainedSourceEquality(t *testing.T) {
	requireNormal(t)
	c := readCardReference(t)
	for _, d := range c.DTOs {
		if d.Kind != "ReadResult" || d.Context != "core" {
			continue
		}
		result, err := ParseReadResult(c.Pin.Value, []byte(d.Canonical))
		if err != nil {
			t.Fatal(err)
		}
		if !result.Status.Usable {
			continue
		}
		result.Status.Usable = false
		if _, err = ValidateStatus(c.Pin.Value, result.Status); err != nil {
			t.Fatal("standalone false is structural, retained digest unavailable", err)
		}
		if _, err = ValidateReadResult(c.Pin.Value, result); err == nil {
			t.Fatal("full matching source must be usable")
		}
		return
	}
	t.Fatal("no complete healthy result")
}
