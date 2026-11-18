package pilot

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/taytr-llc/slug/gymcard"
	"github.com/taytr-llc/slug/wire/v2"
	"os"
	"strconv"
	"strings"
	"testing"
)

type cardReferenceRecipe struct {
	Edits []struct {
		Op, Path string
		Value    json.RawMessage
	} `json:"edits"`
	Style   string   `json:"style"`
	Raw     *string  `json:"raw"`
	Replace []string `json:"replace_once"`
	Pad     *int     `json:"pad_bytes"`
}

func cardReferenceChild(t *testing.T, n *seedReferenceNode, k string) (*seedReferenceNode, int) {
	t.Helper()
	if n.kind == '[' {
		i, e := strconv.Atoi(k)
		if e != nil || i < 0 || i >= len(n.values) {
			t.Fatal("array path", k)
		}
		return n.values[i], i
	}
	for i, name := range n.names {
		if name == k {
			return n.values[i], i
		}
	}
	return nil, -1
}
func cardReferenceBytes(t *testing.T, base string, r cardReferenceRecipe) []byte {
	t.Helper()
	d := json.NewDecoder(strings.NewReader(base))
	d.UseNumber()
	n, e := seedReferenceParse(d)
	if e != nil {
		t.Fatal(e)
	}
	for _, edit := range r.Edits {
		var value *seedReferenceNode
		if edit.Op == "set" || edit.Op == "append" {
			d := json.NewDecoder(bytes.NewReader(edit.Value))
			d.UseNumber()
			value, e = seedReferenceParse(d)
			if e != nil {
				t.Fatal(e)
			}
		}
		if edit.Path == "" {
			if edit.Op != "set" {
				t.Fatal("root mutation")
			}
			n = value
			continue
		}
		if !strings.HasPrefix(edit.Path, "/") {
			t.Fatal("pointer")
		}
		keys := strings.Split(edit.Path[1:], "/")
		for i := range keys {
			keys[i] = strings.ReplaceAll(strings.ReplaceAll(keys[i], "~1", "/"), "~0", "~")
		}
		parent := n
		for _, k := range keys[:len(keys)-1] {
			parent, _ = cardReferenceChild(t, parent, k)
			if parent == nil {
				t.Fatal("missing path")
			}
		}
		key := keys[len(keys)-1]
		target, index := cardReferenceChild(t, parent, key)
		switch edit.Op {
		case "set":
			if index < 0 {
				parent.names = append(parent.names, key)
				parent.values = append(parent.values, value)
			} else {
				parent.values[index] = value
			}
		case "delete":
			if index < 0 {
				t.Fatal("delete missing")
			}
			parent.values = append(parent.values[:index], parent.values[index+1:]...)
			if parent.kind == '{' {
				parent.names = append(parent.names[:index], parent.names[index+1:]...)
			}
		case "append":
			if target == nil || target.kind != '[' {
				t.Fatal("append target")
			}
			target.values = append(target.values, value)
		case "reverse":
			if target == nil || target.kind != '[' {
				t.Fatal("reverse target")
			}
			for i, j := 0, len(target.values)-1; i < j; i, j = i+1, j-1 {
				target.values[i], target.values[j] = target.values[j], target.values[i]
			}
		default:
			t.Fatal("unknown edit", edit.Op)
		}
	}
	b := n.encode(false)
	switch r.Style {
	case "", "canonical":
	case "reversed_whitespace":
		var out bytes.Buffer
		if json.Indent(&out, n.encode(true), "", " ") != nil {
			t.Fatal("indent")
		}
		b = out.Bytes()
	default:
		t.Fatal("style")
	}
	if r.Raw != nil {
		b = []byte(*r.Raw)
	}
	if len(r.Replace) > 0 {
		if len(r.Replace) != 2 || !bytes.Contains(b, []byte(r.Replace[0])) {
			t.Fatal("replace")
		}
		b = bytes.Replace(b, []byte(r.Replace[0]), []byte(r.Replace[1]), 1)
	}
	if r.Pad != nil {
		if len(b) > *r.Pad {
			t.Fatal("pad")
		}
		b = append(b, bytes.Repeat([]byte{' '}, *r.Pad-len(b))...)
	}
	return b
}
func TestCardIndependentTransport(t *testing.T) {
	raw, e := os.ReadFile("../../gymcard/testdata/reference/card_v1.json")
	if e != nil {
		t.Fatal(e)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "c52f513f8dfda34c884312c6a2caaf93b429bbf788aaf7c2cba3fbd901329790" {
		t.Fatal("frozen corpus changed")
	}
	if wire.TestPoW {
		t.Skip("independent transport fixture uses normal profile")
	}
	var doc struct {
		DTOs  []struct{ Kind, Name, Canonical, Context string }
		Cases []struct {
			Name, Kind, Context string
			Base                int
			Recipe              cardReferenceRecipe
			InputBytes          int    `json:"input_bytes"`
			InputSHA            string `json:"input_sha256"`
			Accept              bool
			CanonicalRef        *int `json:"canonical_ref"`
			Canonical           *string
			CanonicalSHA        string `json:"canonical_sha256"`
		} `json:"codec_cases"`
	}
	if e = json.Unmarshal(raw, &doc); e != nil {
		t.Fatal(e)
	}
	count := 0
	for _, c := range doc.Cases {
		if c.Context != "transport" {
			continue
		}
		count++
		t.Run(c.Name, func(t *testing.T) {
			if c.Base < 0 || c.Base >= len(doc.DTOs) {
				t.Fatal("base")
			}
			b := cardReferenceBytes(t, doc.DTOs[c.Base].Canonical, c.Recipe)
			h := sha256.Sum256(b)
			if len(b) != c.InputBytes || hex.EncodeToString(h[:]) != c.InputSHA {
				t.Fatalf("independent materialization len=%d/%d sha=%x/%s", len(b), c.InputBytes, h, c.InputSHA)
			}
			var got []byte
			var e error
			switch c.Kind {
			case "CardRequest":
				var x CardRequest
				x, e = ParseCardRequest(b)
				if e == nil {
					got, e = MarshalCardRequest(x)
				}
			case "CardResponse":
				var x CardResponse
				x, e = ParseCardResponse(b)
				if e == nil {
					got, e = MarshalCardResponse(x)
				}
			case "CardSnapshot":
				var x CardSnapshot
				_, e = cardDecode(b, MaxCardMetadataSize, 16, &x)
				if e == nil && !cardSnapshotValid(x) {
					e = gymcard.ErrInvalid
				}
				if e == nil {
					got, e = json.Marshal(x)
				}
			case "CardJob":
				var x CardJob
				_, e = cardDecode(b, MaxCardJobSize, 16, &x)
				if e == nil && !cardJobValid(&x) {
					e = gymcard.ErrInvalid
				}
				if e == nil {
					got, e = json.Marshal(x)
				}
			case "Record":
				// The runtime decoder admits Record only at its exact result path.
				// Exercise that path without inventing a public raw-record API.
				var nested struct {
					Record struct {
						Record gymcard.Record `json:"record"`
					} `json:"record"`
				}
				_, e = cardDecode(append(append([]byte(`{"record":{"record":`), b...), []byte(`}}`)...), gymcard.MaxRecordSize+64, 16, &nested)
				x := nested.Record.Record
				if e == nil && !cardRecordShape(x) {
					e = gymcard.ErrInvalid
				}
				if e == nil {
					got, e = json.Marshal(x)
				}
			default:
				t.Fatal("unknown transport", c.Kind)
			}
			if (e == nil) != c.Accept {
				t.Fatalf("accepted=%v want=%v err=%v", e == nil, c.Accept, e)
			}
			if c.Accept {
				want := ""
				if c.CanonicalRef != nil {
					want = doc.DTOs[*c.CanonicalRef].Canonical
				} else if c.Canonical != nil {
					want = *c.Canonical
				} else {
					t.Fatal("missing expected canonical")
				}
				if string(got) != want {
					t.Fatal("canonical bytes differ")
				}
				hash := sha256.Sum256(got)
				if hex.EncodeToString(hash[:]) != c.CanonicalSHA {
					t.Fatal("canonical digest")
				}
			}
		})
	}
	if count != 371 {
		t.Fatal("incomplete transport count", count)
	}
}

func TestCardStandaloneUsabilityRequiresRecordContext(t *testing.T) {
	raw, e := os.ReadFile("../../gymcard/testdata/reference/card_v1.json")
	if e != nil {
		t.Fatal(e)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "c52f513f8dfda34c884312c6a2caaf93b429bbf788aaf7c2cba3fbd901329790" {
		t.Fatal("reference pin")
	}
	if wire.TestPoW {
		t.Skip("normal fixture")
	}
	var doc struct {
		DTOs []struct {
			Kind, Name string
			Value      json.RawMessage
		}
	}
	if json.Unmarshal(raw, &doc) != nil {
		t.Fatal("decode")
	}
	var j CardJob
	for _, d := range doc.DTOs {
		if d.Kind == "CardJob" && d.Name == "zero256/completed" {
			if json.Unmarshal(d.Value, &j) != nil {
				t.Fatal("job")
			}
		}
	}
	if j.Result == nil || !cardJobValid(&j) || !j.Result.Status.Usable {
		t.Fatal("positive completed control")
	}
	s := j.Result.Status
	s.CreatedAtMS = s.Source.Status.FinalizedAtMS - 1
	if cardStatusShape(s) {
		t.Fatal("card creation before retained finality accepted")
	}
	s = j.Result.Status
	s.Usable = false
	if !cardStatusShape(s) {
		t.Fatal("standalone false cannot infer the saved source identity")
	}
	if cardResultShape(j.Result.Record, s) {
		t.Fatal("matching full result must have usable healthy source")
	}
	s = j.Result.Status
	s.Source.Status.ConflictCount = 1
	s.Source.Status.Phase = "conflict"
	s.Source.Status.CurrentFinality = false
	s.Source.Status.EscalationDue = s.Source.Status.ObservedAtMS >= s.Source.Status.Event.IncidentDeadlineMS
	if cardStatusShape(s) {
		t.Fatal("standalone true cannot hide conflict")
	}
	s.Usable = false
	if !cardStatusShape(s) {
		t.Fatal("explicit unusable conflict")
	}
}
