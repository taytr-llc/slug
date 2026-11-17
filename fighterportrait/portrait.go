// Copyright (c) 2026 The SLUG developers. ISC license.
package fighterportrait

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/taytr-llc/slug/fightermanifest"
)

const (
	Version1     uint32 = 1
	RendererID          = "slug-fighter-portrait-candidate-v1"
	MaxSVGBytes         = 32768
	digestDomain        = "SLUG fighter portrait SVG\x00v1\x00"
	tableDomain         = "SLUG fighter manifest tables\x00v1\x00"
	tableHash           = "72231fbe707cea9a65dd5b92f7b2719953795bceadbbf9e3b8e0b8f4aaef3f44"
)

// ErrPortrait identifies an unsupported renderer, invalid complete manifest,
// or violated internal output bound. No partial document accompanies an error.
var ErrPortrait = errors.New("invalid fighter portrait")

//go:embed tables_v1.json
var tableBytes string

type headShape struct {
	Name   string `json:"name"`
	Top    int64  `json:"top"`
	Side   int64  `json:"side"`
	Bottom int64  `json:"bottom"`
	Jaw    int64  `json:"jaw"`
}
type portraitTables struct {
	Skin       [6][2]string      `json:"skin_colors"`
	Hair       [6]string         `json:"hair_colors"`
	Background [8]string         `json:"background_colors"`
	Head       [4]headShape      `json:"head_shapes"`
	Build      map[string]int64  `json:"build_by_class"`
	Ring       map[string]string `json:"ring_by_rarity"`
}

// The verified embedded table is private and is never mutated after startup.
var tables = func() portraitTables {
	t, err := loadTables(tableBytes)
	if err != nil {
		panic("fighter portrait frozen tables mismatch")
	}
	return t
}()

func loadTables(raw string) (portraitTables, error) {
	h := sha256.Sum256([]byte(tableDomain + raw))
	if hex.EncodeToString(h[:]) != tableHash {
		return portraitTables{}, ErrPortrait
	}
	var t struct {
		Portrait portraitTables `json:"portrait"`
	}
	if json.Unmarshal([]byte(raw), &t) != nil {
		return portraitTables{}, ErrPortrait
	}
	return t.Portrait, nil
}

// Render validates the entire manifest before using any rendering index. The
// value-only input and newly allocated result have no shared mutable buffers.
func Render(version uint32, manifest fightermanifest.Manifest) ([]byte, error) {
	if version != Version1 || !supported(manifest) {
		return nil, ErrPortrait
	}
	if _, err := fightermanifest.Marshal(manifest); err != nil {
		return nil, ErrPortrait
	}
	return render(manifest)
}

// RenderCanonical owns a bounded copy before applying the manifest package's
// strict, complete canonical JSON parser. It accepts no raw SVG or free text.
func RenderCanonical(version uint32, canonicalManifest []byte) ([]byte, error) {
	if version != Version1 || len(canonicalManifest) == 0 || len(canonicalManifest) > fightermanifest.MaxEncodedBytes {
		return nil, ErrPortrait
	}
	owned := bytes.Clone(canonicalManifest)
	m, err := fightermanifest.Parse(owned)
	if err != nil || !supported(m) {
		return nil, ErrPortrait
	}
	return render(m)
}

// Pin the compatibility mapping explicitly even if the manifest package later
// supports additional formats. A new table or generator is not a fallback.
func supported(m fightermanifest.Manifest) bool {
	return m.Version == 1 && m.Generator == "slug-fighter-manifest-candidate-v1" &&
		m.Tables == tableHash && m.DNA.DNAVersion == 1 && m.Portrait.Version == 1
}

// Digest identifies the canonical preview document, not a fighter or proof.
func Digest(version uint32, manifest fightermanifest.Manifest) (string, error) {
	raw, err := Render(version, manifest)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	h.Write([]byte(digestDomain))
	h.Write(raw)
	return hex.EncodeToString(h.Sum(nil)), nil
}

// roundEven implements exact signed rational nearest-even rounding. All
// renderer calls are bounded by validated S in [51,77], so |n| < 20000.
// The unsigned magnitude also keeps the helper defined at MinInt64.
func roundEven(n, d int64) int64 {
	if d <= 0 {
		panic("invalid fixed-point denominator")
	}
	negative := n < 0
	mag := uint64(n)
	if negative {
		mag = uint64(-(n + 1)) + 1
	}
	q, r := mag/uint64(d), mag%uint64(d)
	if r > uint64(d)-r || (r == uint64(d)-r && q%2 == 1) {
		q++
	}
	if negative {
		return -int64(q)
	}
	return int64(q)
}

// number formats hundredths without floating point, locale or negative zero.
func number(n int64) string {
	negative := n < 0
	mag := uint64(n)
	if negative {
		mag = uint64(-(n + 1)) + 1
	}
	s := strconv.FormatUint(mag/100, 10)
	if fraction := mag % 100; fraction != 0 {
		s += "."
		if fraction < 10 {
			s += "0"
		}
		s += strconv.FormatUint(fraction, 10)
		s = strings.TrimRight(s, "0")
	}
	if negative {
		s = "-" + s
	}
	return s
}

func u(n int64) int64        { return n * 100 }
func pair(x, y int64) string { return number(x) + "," + number(y) }
func xmlText(s string) string {
	// This helper receives only a validated frozen display name.
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;").Replace(s)
}

// Each path below has a fixed number of bounded coordinates. No input can
// choose a command or grow its point list.
type path struct{ strings.Builder }

func (p *path) cmd(command string, coordinates ...int64) {
	if p.Len() > 0 {
		p.WriteByte(' ')
	}
	p.WriteString(command)
	for i := 0; i < len(coordinates); i += 2 {
		p.WriteByte(' ')
		p.WriteString(pair(coordinates[i], coordinates[i+1]))
	}
}
func (p *path) close() { p.WriteString(" Z") }
func head(c, t, s, b, j int64) string {
	var p path
	h := b - t
	p.cmd("M", u(c), u(t))
	p.cmd("C", u(c)+70*s, u(t), u(c+s), u(t)+35*h, u(c+s), 50*(t+b))
	p.cmd("C", u(c+s), u(b)-28*h, u(c+j), u(b), u(c), u(b))
	p.cmd("C", u(c-j), u(b), u(c-s), u(b)-28*h, u(c-s), 50*(t+b))
	p.cmd("C", u(c-s), u(t)+35*h, u(c)-70*s, u(t), u(c), u(t))
	p.close()
	return p.String()
}

type attr struct{ key, value string }

func a(k, v string) attr           { return attr{k, v} }
func n(k string, v int64) attr     { return a(k, number(v)) }
func whole(k string, v int64) attr { return n(k, u(v)) }

type document struct {
	data     []byte
	err      error
	graphics int
}

func (d *document) append(s string) {
	if d.err != nil {
		return
	}
	if len(s) > MaxSVGBytes-len(d.data) {
		d.err = ErrPortrait
		return
	}
	d.data = append(d.data, s...)
}
func (d *document) element(tag string, attrs ...attr) {
	if d.err != nil {
		return
	}
	var b strings.Builder
	b.WriteByte('<')
	b.WriteString(tag)
	for _, at := range attrs {
		b.WriteByte(' ')
		b.WriteString(at.key)
		b.WriteString(`="`)
		b.WriteString(at.value)
		b.WriteByte('"')
	}
	b.WriteString("/>")
	if b.Len() >= 512 || d.graphics >= 43 {
		d.err = ErrPortrait
		return
	}
	d.graphics++
	d.append(b.String())
}
func (d *document) path(data, fill string, attrs ...attr) {
	all := make([]attr, 0, len(attrs)+2)
	all = append(all, a("d", data), a("fill", fill))
	all = append(all, attrs...)
	d.element("path", all...)
}
func lineAttrs(color, width, opacity string, cap bool) []attr {
	attrs := []attr{a("stroke", color), a("stroke-width", width)}
	if cap {
		attrs = append(attrs, a("stroke-linecap", "round"))
	}
	if opacity != "" {
		attrs = append(attrs, a("opacity", opacity))
	}
	return attrs
}

// Coordinate arithmetic below is int64 and runs only after complete
// regeneration validation. The largest intermediate is < 2000000; products
// therefore cannot overflow. S and head geometry come from the hash-pinned
// table and the validated [-6,6] jitter, never untrusted arbitrary dimensions.
func render(m fightermanifest.Manifest) ([]byte, error) {
	p := m.Portrait
	h := tables.Head[p.HeadShape]
	t, s, b, j := h.Top, h.Side, h.Bottom, h.Jaw
	const c int64 = 100
	S := tables.Build[m.Physical.Class] + roundEven(int64(p.ShoulderJitter), 2)
	CI, CO := roundEven(22*S, 100), roundEven(46*S, 100)
	PO, PM := roundEven(40*S, 100), roundEven(20*S, 100)
	PD, NX := roundEven(16800+13*S, 100), roundEven(26*S, 100)
	skin, shadow := tables.Skin[p.Skin][0], tables.Skin[p.Skin][1]
	hair, bg, ring := tables.Hair[p.HairColor], tables.Background[p.Background], tables.Ring[m.DNA.Rarity]
	label := xmlText("Portrait preview of " + m.Name.Display)
	envelope := `<svg xmlns="http://www.w3.org/2000/svg" width="200" height="200" viewBox="0 0 200 200" preserveAspectRatio="xMidYMid meet" overflow="hidden" role="img" aria-label="` + label + `" data-renderer="` + RendererID + `"><title>` + label + `</title><desc>Deterministic appearance preview only. No work, ownership or birth verification.</desc>`
	d := document{data: make([]byte, 0, 8192)}
	if len(envelope) >= 1024 {
		return nil, ErrPortrait
	}
	d.append(envelope)
	d.element("rect", whole("width", 200), whole("height", 200), a("fill", bg))
	d.element("ellipse", whole("cx", 100), whole("cy", 86), whole("rx", 92), whole("ry", 82), a("fill", "#ffffff"), a("opacity", "0.05"))
	d.element("rect", whole("x", 88), whole("y", 110), whole("width", 24), whole("height", 30), whole("rx", 6), a("fill", shadow))
	var torso path
	torso.cmd("M", u(c-17), u(132))
	torso.cmd("C", u(c-36), u(137), u(c-S-10), u(140), u(c-S-13), u(154))
	torso.cmd("C", u(c-S-15), u(168), u(c-S-12), u(184), u(c-S-11), u(200))
	torso.cmd("L", u(c+S+11), u(200))
	torso.cmd("C", u(c+S+12), u(184), u(c+S+15), u(168), u(c+S+13), u(154))
	torso.cmd("C", u(c+S+10), u(140), u(c+36), u(137), u(c+17), u(132))
	torso.cmd("Q", u(c), u(142), u(c-17), u(132))
	torso.close()
	d.path(torso.String(), skin)
	for _, q := range []int64{-1, 1} {
		var arm, crease, cut path
		arm.cmd("M", u(c+q*(S-8)), u(154))
		arm.cmd("C", u(c+q*(S+3)), u(168), u(c+q*(S-1)), u(186), u(c+q*S), u(200))
		arm.cmd("L", u(c+q*(S+11)), u(200))
		arm.cmd("C", u(c+q*(S+12)), u(184), u(c+q*(S+15)), u(168), u(c+q*(S+13)), u(154))
		arm.cmd("C", u(c+q*(S+4)), u(146), u(c+q*(S-2)), u(148), u(c+q*(S-8)), u(154))
		arm.close()
		d.path(arm.String(), shadow, a("opacity", "0.12"))
		crease.cmd("M", u(c+q*(S-8)), u(154))
		crease.cmd("C", u(c+q*(S+3)), u(168), u(c+q*(S-1)), u(184), u(c+q*(S-2)), u(200))
		d.path(crease.String(), "none", lineAttrs(shadow, "2.2", "0.55", true)...)
		cut.cmd("M", u(c+q*(S+9)), u(144))
		cut.cmd("C", u(c+q*(S+2)), u(152), u(c+q*(S-3)), u(158), u(c+q*(S-7)), u(163))
		d.path(cut.String(), "none", lineAttrs(shadow, "2", "0.4", true)...)
	}
	for _, q := range []int64{-1, 1} {
		var collar path
		collar.cmd("M", u(c+q*CI), u(143))
		collar.cmd("L", u(c+q*CO), u(147))
		d.path(collar.String(), "none", lineAttrs(shadow, "2", "0.35", true)...)
	}
	var left, right, middle path
	left.cmd("M", u(c-PO), u(167))
	left.cmd("Q", u(c-PM), u(PD), u(c-2), u(168))
	right.cmd("M", u(c+2), u(168))
	right.cmd("Q", u(c+PM), u(PD), u(c+PO), u(167))
	d.path(left.String(), "none", lineAttrs(shadow, "2", "0.45", true)...)
	d.path(right.String(), "none", lineAttrs(shadow, "2", "0.45", true)...)
	middle.cmd("M", u(c), u(150))
	middle.cmd("L", u(c), u(150+PM))
	d.path(middle.String(), "none", lineAttrs(shadow, "1.6", "0.25", false)...)
	for _, q := range []int64{-1, 1} {
		d.element("circle", whole("cx", c+q*NX), whole("cy", 171), n("r", 170), a("fill", shadow), a("opacity", "0.5"))
	}
	for _, x := range []int64{c - s, c + s} {
		d.element("ellipse", whole("cx", x), whole("cy", 97), whole("rx", 5), whole("ry", 8), a("fill", shadow))
	}
	d.path(head(c, t, s, b, j), skin)
	face(&d, p.FacialHair, c, s, b, j, hair)
	scalp(&d, p.HairStyle, p.HeadShape, c, t, s, hair, skin)
	features(&d, p.Brow, p.Mouth, p.FacialHair, p.Scar, hair, shadow)
	d.element("rect", n("x", 250), n("y", 250), whole("width", 195), whole("height", 195), whole("rx", 12), a("fill", "none"), a("stroke", ring), a("stroke-width", "3"))
	d.append("</svg>")
	if d.err != nil {
		return nil, d.err
	}
	return d.data, nil
}
