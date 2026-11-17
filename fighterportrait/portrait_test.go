// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterportrait

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"math"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/taytr-llc/slug/fightermanifest"
)

func zeroManifest(t *testing.T) fightermanifest.Manifest {
	t.Helper()
	m, err := fightermanifest.Generate(1, [32]byte{})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestCompleteValidationBeforeDrawing(t *testing.T) {
	base := zeroManifest(t)
	// Mutate every scalar field, including those not otherwise drawn. Reflection
	// is test-only and prevents a newly added DTO field escaping this boundary.
	var mutate func(reflect.Value, string)
	mutate = func(v reflect.Value, prefix string) {
		for i := 0; i < v.NumField(); i++ {
			field := v.Type().Field(i)
			if v.Field(i).Kind() == reflect.Struct {
				mutate(v.Field(i), prefix+field.Name+".")
				continue
			}
			name := prefix + field.Name
			t.Run(name, func(t *testing.T) {
				m := base
				value := reflect.ValueOf(&m).Elem()
				for _, part := range strings.Split(name, ".") {
					value = value.FieldByName(part)
				}
				switch value.Kind() {
				case reflect.String:
					value.SetString(value.String() + `<script onload="bad"/>`)
				case reflect.Int:
					value.SetInt(value.Int() + 1)
				case reflect.Uint32:
					value.SetUint(value.Uint() + 1)
				default:
					t.Fatalf("new field kind needs refusal test: %s", value.Kind())
				}
				out, err := Render(1, m)
				if !errors.Is(err, ErrPortrait) || out != nil {
					t.Fatalf("accepted altered %s: %v", name, err)
				}
				digest, err := Digest(1, m)
				if !errors.Is(err, ErrPortrait) || digest != "" {
					t.Fatal("invalid digest succeeded")
				}
			})
		}
	}
	mutate(reflect.ValueOf(base), "")
	for _, value := range []int{math.MinInt, -1, 100, math.MaxInt} {
		m := base
		m.Portrait.HeadShape = value
		if out, err := Render(1, m); err == nil || out != nil {
			t.Fatal("bad index rendered")
		}
	}
	m := base
	m.Name.Display = strings.Repeat("x", 1<<20)
	if out, err := Render(1, m); err == nil || out != nil {
		t.Fatal("unbounded name rendered")
	}
	for _, version := range []uint32{0, 2, math.MaxUint32} {
		if out, err := Render(version, base); err == nil || out != nil {
			t.Fatal("unknown version rendered")
		}
		if out, err := RenderCanonical(version, []byte("{}")); err == nil || out != nil {
			t.Fatal("unknown byte renderer")
		}
		if out, err := Digest(version, base); err == nil || out != "" {
			t.Fatal("unknown digest version")
		}
	}
}

func TestCanonicalInputRefusals(t *testing.T) {
	raw, err := fightermanifest.Marshal(zeroManifest(t))
	if err != nil {
		t.Fatal(err)
	}
	cases := [][]byte{
		nil, {}, []byte("null"), []byte("{}"), []byte("<svg/>"), []byte(strings.Repeat(" ", fightermanifest.MaxEncodedBytes+1)),
		append(bytes.Clone(raw), '\n'), append([]byte(" "), raw...), append(bytes.Clone(raw), raw...),
		bytes.Replace(raw, []byte(`"manifest_version":1`), []byte(`"manifest_version":1,"manifest_version":1`), 1),
		bytes.Replace(raw, []byte(`"manifest_version":1`), []byte(`"manifest_version":1,"MANIFEST_VERSION":1`), 1),
		bytes.Replace(raw, []byte(`"manifest_version"`), []byte(`"manifest\u005fversion"`), 1),
		bytes.Replace(raw, []byte(`"manifest_version":1`), []byte(`"manifest_version":1.0`), 1),
		bytes.Replace(raw, []byte(`"manifest_version":1`), []byte(`"manifest_version":1e0`), 1),
		bytes.Replace(raw, []byte(`"manifest_version":1`), []byte(`"manifest_version":"1"`), 1),
		bytes.Replace(raw, []byte(`"manifest_version":1,`), nil, 1),
		bytes.Replace(raw, []byte(`"portrait_version":1`), []byte(`"portrait_version":null`), 1),
		[]byte(strings.Repeat(`{"a":`, 20) + "1" + strings.Repeat("}", 20)),
		bytes.Replace(raw, []byte(`"skin":1`), []byte(`"skin":-1`), 1),
	}
	for i, input := range cases {
		if output, err := RenderCanonical(1, input); err == nil || output != nil {
			t.Errorf("input %d accepted", i)
		}
	}
}

// Parse, then positively allow only the renderer's exact element/attribute
// vocabulary. encoding/xml has no external entity loader; directives and
// processing instructions are rejected rather than interpreted.
func assertSafeDocument(t *testing.T, raw []byte) int {
	t.Helper()
	if len(raw) == 0 || len(raw) > MaxSVGBytes || raw[len(raw)-1] == '\n' {
		t.Fatal("document bound/ending")
	}
	for _, c := range raw {
		if c > 127 {
			t.Fatal("non-ASCII output")
		}
	}
	allowed := map[string]string{
		"svg":   "xmlns width height viewBox preserveAspectRatio overflow role aria-label data-renderer",
		"title": "", "desc": "",
		"rect":    "x y width height rx fill stroke stroke-width stroke-linecap opacity",
		"ellipse": "cx cy rx ry fill stroke stroke-width stroke-linecap opacity transform",
		"circle":  "cx cy r fill stroke stroke-width stroke-linecap opacity",
		"path":    "d fill stroke stroke-width stroke-linecap opacity",
	}
	decoder := xml.NewDecoder(bytes.NewReader(raw))
	decoder.Strict = true
	depth, graphics := 0, 0
	var stack []string
	var order []string
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		switch x := token.(type) {
		case xml.StartElement:
			depth++
			stack = append(stack, x.Name.Local)
			fields, ok := allowed[x.Name.Local]
			if !ok || depth > 2 || x.Name.Space != "http://www.w3.org/2000/svg" {
				t.Fatal("unexpected element", x.Name, depth)
			}
			if depth == 1 && x.Name.Local != "svg" {
				t.Fatal("unexpected root")
			}
			if depth == 2 {
				order = append(order, x.Name.Local)
				if x.Name.Local != "title" && x.Name.Local != "desc" {
					graphics++
				}
			}
			last := -1
			seen := map[string]bool{}
			for _, at := range x.Attr {
				if at.Name.Space != "" || seen[at.Name.Local] {
					t.Fatal("attribute namespace/duplicate", at.Name)
				}
				seen[at.Name.Local] = true
				position := -1
				for i, key := range strings.Fields(fields) {
					if key == at.Name.Local {
						position = i
					}
				}
				if position < 0 || position <= last {
					t.Fatal("unexpected attribute/order", at.Name)
				}
				last = position
			}
		case xml.EndElement:
			depth--
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) != 2 || (stack[1] != "title" && stack[1] != "desc") {
				t.Fatalf("unexpected text %q", x)
			}
		case xml.Directive, xml.ProcInst, xml.Comment:
			t.Fatal("unexpected markup token")
		default:
			t.Fatal("unexpected token")
		}
	}
	if depth != 0 || len(order) < 2 || order[0] != "title" || order[1] != "desc" || graphics > 43 {
		t.Fatal("element order/bounds")
	}
	return graphics
}

func TestConcurrentOwnershipAndSafety(t *testing.T) {
	m := zeroManifest(t)
	before := m
	want, err := Render(1, m)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := fightermanifest.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	digest, err := Digest(1, m)
	if err != nil {
		t.Fatal(err)
	}
	assertSafeDocument(t, want)
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 4; j++ {
				got, err := RenderCanonical(1, raw)
				if err != nil || !bytes.Equal(got, want) {
					t.Error("render mismatch", err)
					return
				}
				got[0] = 'X'
				again, err := Render(1, m)
				if err != nil || !bytes.Equal(again, want) {
					t.Error("shared output", err)
					return
				}
				h, err := Digest(1, m)
				if err != nil || h != digest {
					t.Error("digest mismatch", err)
					return
				}
			}
		}()
	}
	wg.Wait()
	if m != before {
		t.Fatal("input mutated")
	}
	result, err := RenderCanonical(1, raw)
	if err != nil {
		t.Fatal(err)
	}
	raw[0] = 'X'
	if !bytes.Equal(result, want) {
		t.Fatal("input aliases returned bytes")
	}
}

func TestExactArithmeticAndBoundedEmitter(t *testing.T) {
	for _, v := range []struct{ n, d, want int64 }{{-5, 2, -2}, {-3, 2, -2}, {-1, 2, 0}, {1, 2, 0}, {3, 2, 2}, {5, 2, 2}, {1650, 100, 16}, {3450, 100, 34}, {1950, 100, 20}, {math.MinInt64, 1, math.MinInt64}} {
		if got := roundEven(v.n, v.d); got != v.want {
			t.Fatalf("round %d/%d=%d", v.n, v.d, got)
		}
	}
	for _, v := range []struct {
		n    int64
		want string
	}{{0, "0"}, {-1800, "-18"}, {250, "2.5"}, {8355, "83.55"}, {11056, "110.56"}, {1, "0.01"}, {-10, "-0.1"}, {math.MinInt64, "-92233720368547758.08"}} {
		if got := number(v.n); got != v.want {
			t.Fatalf("format %d: %q", v.n, got)
		}
	}
	if got := xmlText(`&<>"'`); got != "&amp;&lt;&gt;&quot;&apos;" {
		t.Fatal("escape order", got)
	}
	if _, err := loadTables(tableBytes + "\n"); err == nil {
		t.Fatal("tampered embedded bytes accepted")
	}
	d := document{data: make([]byte, MaxSVGBytes-1)}
	d.append("xx")
	if d.err == nil || len(d.data) != MaxSVGBytes-1 {
		t.Fatal("append bound applied too late")
	}
	d = document{}
	d.element("path", a("d", strings.Repeat("x", 512)))
	if d.err == nil || len(d.data) != 0 {
		t.Fatal("element bound applied too late")
	}
	d = document{graphics: 43}
	d.element("circle", a("r", "1"))
	if d.err == nil || len(d.data) != 0 {
		t.Fatal("element count bound")
	}
}

func FuzzCanonicalPortrait(f *testing.F) {
	m, _ := fightermanifest.Generate(1, [32]byte{})
	raw, _ := fightermanifest.Marshal(m)
	f.Add(uint32(1), raw)
	f.Add(uint32(0), []byte("{}"))
	fixtures, err := os.ReadFile("testdata/reference/portraits_v1.json")
	if err != nil {
		f.Fatal(err)
	}
	var corpus referenceCorpus
	if err = json.Unmarshal(fixtures, &corpus); err != nil {
		f.Fatal(err)
	}
	for _, tc := range corpus.Cases {
		canonical, err := fightermanifest.Marshal(tc.Manifest)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(uint32(1), canonical)
	}
	f.Fuzz(func(t *testing.T, version uint32, data []byte) {
		out, err := RenderCanonical(version, data)
		if err != nil {
			if out != nil {
				t.Fatal("partial failure output")
			}
			return
		}
		assertSafeDocument(t, out)
		parsed, err := fightermanifest.Parse(data)
		if err != nil {
			t.Fatal("renderer accepted invalid manifest")
		}
		typed, err := Render(version, parsed)
		if err != nil || !bytes.Equal(typed, out) {
			t.Fatal("entrypoint disagreement")
		}
	})
}
