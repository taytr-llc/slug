package pilot

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/taytr-llc/slug/gymseed"
	"github.com/taytr-llc/slug/wire/v2"
)

// Preserve the independent corpus's JSON field order while applying its finite
// input recipes. Each expanded byte string is checked against the frozen hash
// before any runtime parser is consulted.
type seedReferenceNode struct {
	scalar any
	names  []string
	values []*seedReferenceNode
	kind   byte
}

func seedReferenceParse(d *json.Decoder) (*seedReferenceNode, error) {
	x, e := d.Token()
	if e != nil {
		return nil, e
	}
	n := &seedReferenceNode{scalar: x}
	if x == json.Delim('{') {
		n.kind = '{'
		for d.More() {
			k, e := d.Token()
			if e != nil {
				return nil, e
			}
			v, e := seedReferenceParse(d)
			if e != nil {
				return nil, e
			}
			n.names = append(n.names, k.(string))
			n.values = append(n.values, v)
		}
		_, e = d.Token()
	} else if x == json.Delim('[') {
		n.kind = '['
		for d.More() {
			v, e := seedReferenceParse(d)
			if e != nil {
				return nil, e
			}
			n.values = append(n.values, v)
		}
		_, e = d.Token()
	}
	return n, e
}
func (n *seedReferenceNode) encode(reverse bool) []byte {
	var b bytes.Buffer
	switch n.kind {
	case '{':
		b.WriteByte('{')
		for j := range n.names {
			i := j
			if reverse {
				i = len(n.names) - 1 - j
			}
			if j > 0 {
				b.WriteByte(',')
			}
			k, _ := json.Marshal(n.names[i])
			b.Write(k)
			b.WriteByte(':')
			b.Write(n.values[i].encode(reverse))
		}
		b.WriteByte('}')
	case '[':
		b.WriteByte('[')
		for i, v := range n.values {
			if i > 0 {
				b.WriteByte(',')
			}
			b.Write(v.encode(reverse))
		}
		b.WriteByte(']')
	default:
		x, _ := json.Marshal(n.scalar)
		b.Write(x)
	}
	return b.Bytes()
}

type seedReferenceCase struct {
	Kind, Name   string
	DTO          int
	Accepted     bool
	Format       string
	Bytes        int
	InputBytes   int      `json:"input_bytes"`
	InputSHA     string   `json:"input_sha256"`
	Expected     *int     `json:"expected_dto"`
	CanonicalSHA string   `json:"canonical_sha256"`
	Replace      []string `json:"replace_first"`
	Mutations    []struct {
		Op, Path string
		Value    json.RawMessage
	}
	RawHex string `json:"raw_hex"`
}

func seedReferenceRecipe(t *testing.T, canonical string, c seedReferenceCase) []byte {
	t.Helper()
	d := json.NewDecoder(strings.NewReader(canonical))
	d.UseNumber()
	n, e := seedReferenceParse(d)
	if e != nil {
		t.Fatal(e)
	}
	for _, m := range c.Mutations {
		if !strings.HasPrefix(m.Path, "/") || strings.Contains(m.Path[1:], "/") {
			t.Fatal("unsupported frozen mutation path", m.Path)
		}
		name := strings.ReplaceAll(strings.ReplaceAll(m.Path[1:], "~1", "/"), "~0", "~")
		i := 0
		for i < len(n.names) && n.names[i] != name {
			i++
		}
		switch m.Op {
		case "delete":
			if i == len(n.names) {
				t.Fatal("missing mutation key")
			}
			n.names = append(n.names[:i], n.names[i+1:]...)
			n.values = append(n.values[:i], n.values[i+1:]...)
		case "set":
			v := json.NewDecoder(bytes.NewReader(m.Value))
			v.UseNumber()
			value, e := seedReferenceParse(v)
			if e != nil {
				t.Fatal(e)
			}
			if i == len(n.names) {
				n.names = append(n.names, name)
				n.values = append(n.values, value)
			} else {
				n.values[i] = value
			}
		default:
			t.Fatal("unsupported mutation", m.Op)
		}
	}
	b := n.encode(false)
	switch c.Format {
	case "", "canonical":
	case "reversed_whitespace":
		var p bytes.Buffer
		if json.Indent(&p, n.encode(true), "", " ") != nil {
			t.Fatal("indent")
		}
		b = append([]byte(" \n"), p.Bytes()...)
		b = append(b, []byte("\t\n")...)
	case "escaped_first_key":
		needle := []byte(`"` + n.names[0] + `"`)
		escaped := []byte(fmt.Sprintf(`"\u%04x%s"`, n.names[0][0], n.names[0][1:]))
		b = bytes.Replace(b, needle, escaped, 1)
	case "duplicate_first_key", "duplicate_escaped_key":
		name := n.names[0]
		key, _ := json.Marshal(name)
		if c.Format == "duplicate_escaped_key" {
			key = []byte(fmt.Sprintf(`"\u%04x%s"`, name[0], name[1:]))
		}
		extra := append([]byte{','}, key...)
		extra = append(extra, ':')
		extra = append(extra, n.values[0].encode(false)...)
		b = append(append(b[:len(b)-1], extra...), '}')
	case "pad_to_cap":
		if len(b) > c.Bytes {
			t.Fatal("padding bound")
		}
		b = append(b, bytes.Repeat([]byte{' '}, c.Bytes-len(b))...)
	default:
		t.Fatal("unknown format", c.Format)
	}
	if c.RawHex != "" {
		b, e = hex.DecodeString(c.RawHex)
		if e != nil {
			t.Fatal(e)
		}
	}
	if len(c.Replace) > 0 {
		if len(c.Replace) != 2 {
			t.Fatal("replace")
		}
		b = bytes.Replace(b, []byte(c.Replace[0]), []byte(c.Replace[1]), 1)
	}
	h := sha256.Sum256(b)
	if len(b) != c.InputBytes || hex.EncodeToString(h[:]) != c.InputSHA {
		t.Fatalf("independent recipe mismatch len=%d want=%d sha=%x want=%s", len(b), c.InputBytes, h, c.InputSHA)
	}
	return b
}

func TestSeedIndependentTransport(t *testing.T) {
	raw, e := os.ReadFile("../../gymseed/testdata/reference/seed_v1.json")
	if e != nil {
		t.Fatal(e)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "2b8d78e8ef665e372543eb87bb5d25a975026a0ae946b15a871ac97bbf2153f0" {
		t.Fatal("frozen corpus pin")
	}
	if wire.TestPoW {
		t.Skip("production profile fixture View pins")
	}
	var doc struct {
		DTOs     []struct{ Kind, Name, Canonical string }
		Cases    []seedReferenceCase `json:"codec_cases"`
		Protocol []struct {
			Name         string
			Accepted     bool
			Response     json.RawMessage
			CanonicalSHA string `json:"canonical_sha256"`
		} `json:"protocol_cases"`
	}
	if json.Unmarshal(raw, &doc) != nil {
		t.Fatal("corpus decode")
	}
	count := 0
	for _, c := range doc.Cases {
		if !strings.HasPrefix(c.Kind, "Seed") {
			continue
		}
		count++
		t.Run(c.Kind+"/"+c.Name, func(t *testing.T) {
			if c.DTO < 0 || c.DTO >= len(doc.DTOs) {
				t.Fatal("dto")
			}
			b := seedReferenceRecipe(t, doc.DTOs[c.DTO].Canonical, c)
			var got []byte
			var e error
			switch c.Kind {
			case "SeedRequest":
				var q SeedRequest
				q, e = ParseSeedRequest(b)
				if e == nil {
					got, e = MarshalSeedRequest(q)
				}
			case "SeedResponse":
				var r SeedResponse
				r, e = ParseSeedResponse(b)
				if e == nil {
					got, e = MarshalSeedResponse(r)
				}
			case "SeedSnapshot":
				var s SeedSnapshot
				_, e = seedDecode(b, MaxSeedMetadataSize, 16, &s)
				if e == nil && !seedSnapshotValid(s) {
					e = gymseed.ErrInvalid
				}
				if e == nil {
					got, e = json.Marshal(s)
				}
			case "SeedJob":
				var j SeedJob
				_, e = seedDecode(b, MaxSeedJobSize, 16, &j)
				if e == nil && !seedJobValid(&j) {
					e = gymseed.ErrInvalid
				}
				if e == nil {
					got, e = json.Marshal(j)
				}
			default:
				t.Fatal(c.Kind)
			}
			if (e == nil) != c.Accepted {
				t.Fatalf("accepted=%v want=%v err=%v", e == nil, c.Accepted, e)
			}
			if e == nil {
				if c.Expected == nil || *c.Expected >= len(doc.DTOs) || string(got) != doc.DTOs[*c.Expected].Canonical {
					t.Fatal("canonical bytes")
				}
				h := sha256.Sum256(got)
				if hex.EncodeToString(h[:]) != c.CanonicalSHA {
					t.Fatal("canonical hash")
				}
			}
		})
	}
	if count != 920 {
		t.Fatal("helper corpus count", count)
	}
	// Stateful negative correlation belongs to the native consumer. Every
	// independently accepted complete envelope must also pass this parser.
	positives := 0
	for _, c := range doc.Protocol {
		if !c.Accepted {
			continue
		}
		positives++
		t.Run("protocol-positive/"+c.Name, func(t *testing.T) {
			v, e := ParseSeedResponse(c.Response)
			if e != nil {
				t.Fatal(e)
			}
			b, e := MarshalSeedResponse(v)
			if e != nil {
				t.Fatal(e)
			}
			h := sha256.Sum256(b)
			if hex.EncodeToString(h[:]) != c.CanonicalSHA {
				t.Fatal("protocol canonical hash")
			}
		})
	}
	if positives < 30 {
		t.Fatal("protocol coverage", positives)
	}
	t.Log("independent recipes", count, "positive protocol envelopes", positives)
}
