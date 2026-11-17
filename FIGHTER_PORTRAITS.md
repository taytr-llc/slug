# Deterministic portrait SVG candidate contract

Status: approved engineering candidate for a future preview renderer, frozen by the root review on November 16, 2026. This document does not implement a renderer, generate image files, activate an official generator, assign names or portraits to admitted fighters, or authorize production rollout. The frozen manifest candidate remains a preview. D68/D69 admission and birth are unchanged, and Mike's D74 joint design review remains required before friends/tester or public release.

The intended result is **identical SVG bytes for the same validated manifest and explicit renderer version**. Cross-browser raster pixels, antialiasing, display color and assistive-technology behavior are not claimed identical.

## Inputs and provenance

Read sources:

- `/Users/taytrrip/slug/slug-sim-export/sim/portraits.py`, SHA256 `9bac3d966e7093e0665cd9c9a683e7a6ab73381948dccfd0dc8cac8c98a75a72`.
- `/Users/taytrrip/slug/fighter-manifest-contract.md`, SHA256 `ba8d33dbad3722eed6b4406ef120f946d7e06e1ea04305cbe8648889ba169e80`.
- Staged `/Users/taytrrip/slug/set33-manifest-stage/manifest.go`, SHA256 `86457589a7748cb0d2862be61afded284fc88714cd37ee9fcb5c49eaf4350290`.
- Staged `tables_v1.json`, raw-file SHA256 `9f9380b708cae73b8d2641eaf04ecefb3a3b7fa9dd1b25b1f234f6511780b8c7`. This is distinct from the domain-separated manifest table hash below.
- Independent `set33-manifest-stage/testdata/manifests_v1.json`, SHA256 `75eb01acb79f45b74a6445edf956c5e3f3b68024998ae56ab44a7ed07cf15f3d`.

Only the frozen `fightermanifest.Manifest` is a rendering input. Accept exactly:

- `manifest_version=1`, `generator_id="slug-fighter-manifest-candidate-v1"`;
- table hash `72231fbe707cea9a65dd5b92f7b2719953795bceadbbf9e3b8e0b8f4aaef3f44`;
- unchanged complete DNA v1, physical/name/style fields and `portrait_version=1`;
- all values exactly equal to regeneration from the embedded canonical seed.

The prototype's `_traits(fid)` hashing of a text identifier is **not** used. Trait selection has already been frozen by the manifest's independent streams. The renderer performs no random draws, no new seed hashing to select appearance, and no changes to DNA, names, styles, colors or scar probabilities. It reads `Portrait` indices, `Physical.Class`, `DNA.Rarity`, and the validated display name. Other physical fields do not scale the drawing; stance does not mirror it.

The staged manifest type has exported fields but an unexported `validate`. A future typed renderer should call its public `Marshal(m)` to obtain validation rather than trust a caller-constructed struct or duplicate a partial validator. A byte-input convenience method must check the size, copy at most 8192 bytes into an owned buffer, then use the existing strict `Parse` boundary: at most 8192 bytes, depth 8, canonical ASCII JSON, no unknown/missing/duplicate/case/escaped-alias fields, wrong numeric types, reordered fields or trailing data. Typed callers also receive complete regeneration validation. No index is used before this validation; no fallback class, rarity, style, color or version exists.

The staged manifest package does not expose mutable table objects. The renderer should embed an identical copy of the frozen table bytes, verify the existing domain-separated table hash before use, and build its private immutable color/shape/build/ring lookup from those verified bytes. It must not load palettes from disk or accept caller tables. An internal shared read-only asset package is an equivalent later implementation choice if it preserves exactly the same bytes and ownership; no new public table-mutation API is needed.

## Version/API candidate

Proposed package `fighterportrait` (separate from manifest generation):

```go
const Version1 uint32 = 1
const RendererID = "slug-fighter-portrait-candidate-v1"
const MaxSVGBytes = 32768

func Render(version uint32, manifest fightermanifest.Manifest) ([]byte, error)
func RenderCanonical(version uint32, canonicalManifest []byte) ([]byte, error)
func Digest(version uint32, manifest fightermanifest.Manifest) (string, error)
```

These are pure, bounded library operations. They have no filesystem, browser, RPC, image loading, locale, network or clock dependency. No raw SVG input, caller-supplied dimensions, text, colors, CSS, IDs, paths or markup are accepted. An error returns no partial SVG. The manifest is a value-only DTO in the staged API; validate the owned value and render that value.

`Digest` renders canonical bytes, then returns lowercase SHA256 hex over:

```
ASCII "SLUG fighter portrait SVG" || 00 || ASCII "v1" || 00 || canonical SVG bytes
```

This optional digest identifies the preview document, not a fighter, work proof, ownership or birth. Do not expose a digest operation that blesses arbitrary caller SVG. Renderer version and renderer ID are distinct from manifest version, portrait trait version and DNA version. Any geometry, arithmetic, serialization, description or accessibility-byte change requires a new renderer version/ID. A new manifest/table version is refused until an explicit compatibility mapping exists. No `latest` fallback.

## Arithmetic and formatting

All coordinates use exact integers in hundredths of one viewBox unit. All products and sums below are rational with denominator dividing 100, except the explicitly rounded chest/build calculations. Use checked signed 64-bit intermediates or another exact representation; never float32/64, locale formatting, trig or browser-computed geometry.

`R(n/d)` means round the exact rational to nearest integer, ties to even. For negative inputs, `R(-x)=-R(x)`. Implement absolute numerator quotient/remainder, increment when `2*remainder>d`, or when equality holds and the quotient is odd; apply the sign afterward. There is no negative zero.

`F` serializes an exact hundredth-unit value as minimal ASCII decimal: optional `-`, integer part without leading zeroes, then a decimal point only when needed; at most two fractional digits; strip trailing fractional zeroes. Examples: `0`, `-18`, `2.5`, `83.55`, `110.56`; never `-0`, `1.0`, `1e2`, `+1`, `.5`, comma decimal separators or binary-float tails. Fixed radii, opacity and stroke-width numbers use the same minimal decimal spelling. Opacity `0.30` becomes `0.3` and `0.10` becomes `0.1`.

Variables:

- `c=100`; `(t,s,b,j)` are the table head shape's top, side, bottom and jaw.
- `S = build_by_class[Physical.Class] + R(shoulder_jitter/2)`.
- `CI=R(22*S/100)`, `CO=R(46*S/100)`, `PO=R(40*S/100)`, `PM=R(20*S/100)`, `PD=R((16800+13*S)/100)`, `NX=R(26*S/100)`.
- `J=b-2`; `Y=115` for full beard (index 4), otherwise 117.
- Skin, shadow, hair, background and ring are exact lowercase color strings in the pinned tables. Build values are light 54, welter 59, middle 64, light_heavy 69, heavy 74; S therefore lies in [51,77].

Exact jitter rounding is:

| jitter | -6 | -5 | -4 | -3 | -2 | -1 | 0 | 1 | 2 | 3 | 4 | 5 | 6 |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| R(jitter/2) | -3 | -2 | -2 | -2 | -1 | 0 | 0 | 0 | 1 | 2 | 2 | 2 | 3 |

The source's rational values and round-even intent are preserved. Its Python binary-float text artifacts are not a byte-compatibility requirement.

### Common head path

Define `H(c,t,s,b,j)` with `h=b-t`. The following is a command specification; expressions are evaluated before emitting numbers:

```
M c,t
C (c+7*s/10),t (c+s),(t+35*h/100) (c+s),((t+b)/2)
C (c+s),(b-28*h/100) (c+j),b c,b
C (c-j),b (c-s),(b-28*h/100) (c-s),((t+b)/2)
C (c-s),(t+35*h/100) (c-7*s/10),t c,t
Z
```

The actual `d` attribute is one line: a single ASCII space between command letters and coordinate pairs and between successive pairs; coordinates within a pair use one comma; a single space precedes `Z`; no leading/trailing whitespace. Do not combine or remove commands even if a geometric optimizer would consider them equivalent.

### Curly hair without runtime trig

The prototype uses angle `pi*(10+21*k)/125`, k=0..5, and rounds each circle center to a whole unit. Freeze the following integer centers in this renderer instead of computing trigonometry at runtime, in listed order:

| Head shape | Six `(cx,cy)` circle centers |
| --- | --- |
| round | (73,69), (80,63), (93,59), (107,59), (120,63), (127,69) |
| square | (72,71), (79,65), (92,61), (108,61), (121,65), (128,71) |
| long | (77,63), (83,57), (94,53), (106,53), (117,57), (123,63) |
| wide | (69,71), (77,65), (92,61), (108,61), (123,65), (131,71) |

These centers were calculated independently for this document with 80-digit decimal arithmetic and Taylor sin/cos evaluation, using the exact rational angles and round-even. No staged Go renderer or rendered image was used. Each circle has radius 8.5. Future oracle acceptance must independently reproduce these literals and compare the original Python geometry before freezing full SVG vectors.

## Document envelope and serialization

The root opening tag is exactly the following, with `{label}` replaced as described below:

```xml
<svg xmlns="http://www.w3.org/2000/svg" width="200" height="200" viewBox="0 0 200 200" preserveAspectRatio="xMidYMid meet" overflow="hidden" role="img" aria-label="{label}" data-renderer="slug-fighter-portrait-candidate-v1">
```

Immediately append, without whitespace between elements:

```xml
<title>{label}</title><desc>Deterministic appearance preview only. No work, ownership or birth verification.</desc>
```

`label` is exactly ASCII `Portrait preview of ` followed by validated `Name.Display`. Names come only from the manifest's frozen tables. Emit XML-escaped text/attribute content with a single defined escaping order (`&`→`&amp;`, `<`→`&lt;`, `>`→`&gt;`, `"`→`&quot;`, `'`→`&apos;`), even though present names need none. There is no caller label parameter. No `id`, `aria-labelledby`, or `aria-describedby` is emitted, so repeated inline portraits cannot introduce duplicate DOM IDs. The title remains a document label; `aria-label` supplies the explicit image name. No interactive role, tab stop, control or event is present.

Append the graphics below in exact paint order, then `</svg>`. UTF8 output is entirely ASCII, without BOM, XML declaration, DOCTYPE, comments, processing instructions, pretty-print whitespace or terminal newline. All graphics are direct children and use self-closing tags ending `/>`. Attributes use double quotes, one ASCII space between attributes, and the following order when present:

- `rect`: x, y, width, height, rx, fill, stroke, stroke-width, stroke-linecap, opacity.
- `ellipse`: cx, cy, rx, ry, fill, stroke, stroke-width, stroke-linecap, opacity, transform.
- `circle`: cx, cy, r, fill, stroke, stroke-width, stroke-linecap, opacity.
- `path`: d, fill, stroke, stroke-width, stroke-linecap, opacity.

All paths explicitly set fill to a frozen color or `none`. Specifying `fill="none"` on the source's straight stroked paths makes their existing zero-area fill explicit. Other attributes are emitted only where specified below. No optimizer, attribute sorter, serializer-specific empty-element spelling or number reformatter may change output.

The square viewBox, intrinsic 200×200 dimensions, centered `meet` scaling and explicit viewport clipping preserve the drawing's proportions and cropped lower arms. Host layout may display it at another square size without changing canonical bytes. It must not distort the aspect ratio or mirror/recolor parts. See the official [SVG coordinate and viewBox specification](https://www.w3.org/TR/SVG2/coords.html#ViewBoxAttribute) for the viewport semantics; these are rendering conventions, not a pixel-equality claim.

## Exact paint inventory

Notation below is only for this specification: `P(commands; attrs)` emits a path, `Rct` a rect, `E` an ellipse, `Crc` a circle. `fill=skin/shadow/hair/bg/ring` means the pinned selected color, not a literal variable. Path expressions use the grammar above. All unspecified opacity is 1 and omitted. `cap=round` means `stroke-linecap="round"`. Body side loops always run `q=-1` then `q=+1`.

### 1. Background, neck, torso and arms

1. `Rct(width=200,height=200,fill=bg)`; x/y omitted as in the source.
2. `E(cx=100,cy=86,rx=92,ry=82,fill=#ffffff,opacity=0.05)`.
3. `Rct(x=88,y=110,width=24,height=30,rx=6,fill=shadow)`.
4. One torso silhouette, fill=skin:

```
M (c-17),132
C (c-36),137 (c-S-10),140 (c-S-13),154
C (c-S-15),168 (c-S-12),184 (c-S-11),200
L (c+S+11),200
C (c+S+12),184 (c+S+15),168 (c+S+13),154
C (c+S+10),140 (c+36),137 (c+17),132
Q c,142 (c-17),132 Z
```

For each q in [-1,+1], append these three paths in order:

- Arm shadow, fill=shadow, opacity=0.12:

```
M (c+q*(S-8)),154
C (c+q*(S+3)),168 (c+q*(S-1)),186 (c+q*S),200
L (c+q*(S+11)),200
C (c+q*(S+12)),184 (c+q*(S+15)),168 (c+q*(S+13)),154
C (c+q*(S+4)),146 (c+q*(S-2)),148 (c+q*(S-8)),154 Z
```

- Inner arm crease, fill=none, stroke=shadow, stroke-width=2.2, cap=round, opacity=0.55:

```
M (c+q*(S-8)),154
C (c+q*(S+3)),168 (c+q*(S-1)),184 (c+q*(S-2)),200
```

- Deltoid cut, fill=none, stroke=shadow, stroke-width=2, cap=round, opacity=0.4:

```
M (c+q*(S+9)),144
C (c+q*(S+2)),152 (c+q*(S-3)),158 (c+q*(S-7)),163
```

### 2. Chest, ears and head

Append these in order:

1. `P(M (c-CI),143 L (c-CO),147; fill=none,stroke=shadow,stroke-width=2,cap=round,opacity=0.35)`.
2. `P(M (c+CI),143 L (c+CO),147; same attributes)`.
3. `P(M (c-PO),167 Q (c-PM),PD (c-2),168; fill=none,stroke=shadow,stroke-width=2,cap=round,opacity=0.45)`.
4. `P(M (c+2),168 Q (c+PM),PD (c+PO),167; same attributes)`.
5. `P(M c,150 L c,(150+PM); fill=none,stroke=shadow,stroke-width=1.6,opacity=0.25)`; no cap attribute.
6. Nipple circles q=-1 then +1: `Crc(cx=c+q*NX,cy=171,r=1.7,fill=shadow,opacity=0.5)`.
7. Left then right ear: `E(cx=c-s then c+s,cy=97,rx=5,ry=8,fill=shadow)`.
8. `P(H(c,t,s,b,j); fill=skin)`.

The body stays shirtless. The unused prototype tank colors are not read, reintroduced or converted into new options. The shoulder, arm and chest geometry remains one connected torso with the source's shadow layers.

### 3. Facial hair, before scalp hair and features

Exactly one facial-hair branch:

- none (index 0): nothing.
- stubble (index 1), fill=hair, opacity=0.3:

```
M (c-j-4),(J-26)
C (c-j),J (c+j),J (c+j+4),(J-26)
C (c+14),(J-16) (c-14),(J-16) (c-j-4),(J-26) Z
```

- mustache (index 2) or goatee (index 3), first the following path, fill=hair:

```
M (c-13),111 C (c-6),107 (c-2),110 c,112
C (c+2),110 (c+6),107 (c+13),111
C (c+7),116 (c-7),116 (c-13),111 Z
```

  For goatee only, then `E(cx=c,cy=124,rx=8,ry=6,fill=hair)`.
- beard (index 4), fill=hair:

```
M (c-s+3),94
C (c-s+1),(J-8) (c-j),(J+3) c,(J+3)
C (c+j),(J+3) (c+s-1),(J-8) (c+s-3),94
C (c+15),110 (c-15),110 (c-s+3),94 Z
```

### 4. Scalp hair

Indices are the pinned table order:

0. Bald: `E(cx=c-35*s/100,cy=t+9,rx=10,ry=5,fill=#ffffff,opacity=0.1,transform="rotate(-18 F(c-35*s/100) F(t+9))")`. The transform has single spaces, no commas; the `F(...)` notation means evaluated formatted numbers.
1. Buzz: `P(H(c,t-1,s,84,s-4); fill=hair,opacity=0.92)`.
2. Crop, fill=hair:

```
M (c-s),92 C (c-s),(t+2) (c-s/2),(t-3) c,(t-3)
C (c+s/2),(t-3) (c+s),(t+2) (c+s),92
L (c+s),80 L (c-s),80 Z
```

3. Mohawk, fill=hair:

```
M (c-7),90 C (c-11),(t-6) (c-5),(t-22) c,(t-24)
C (c+5),(t-22) (c+11),(t-6) (c+7),90 Z
```

4. Swept, fill=hair:

```
M (c-s),94 C (c-s-2),(t-2) (c-4*s/10),(t-8) (c+6),(t-6)
C (c+8*s/10),(t-4) (c+s+2),(t+8) (c+s),88
C (c+4*s/10),(t+16) (c-s/2),(t+14) (c-s),94 Z
```

5. Curly: emit six `Crc(cx,cy,r=8.5,fill=hair)` at the frozen head-shape centers, then `P(H(c,t,s-2,76,s-6); fill=hair)`.
6. Flat top, fill=hair:

```
M (c-s+4),84 L (c-s+2),(t+2) L (c+s-2),(t+2) L (c+s-4),84
C (c+4*s/10),(t+14) (c-4*s/10),(t+14) (c-s+4),84 Z
```

7. Fade, in this order:

```
P(H(c,t-1,s,80,s-3); fill=hair)
P(H(c,t+3,s-3,84,s-6); fill=skin)
P(H(c,t+3,s-3,80,s-6); fill=hair,opacity=0.55)
```

### 5. Brows, eyes and nose

All brows are fill=none, stroke=hair, stroke-width=3.4, cap=round. Two paths in order:

- Flat (index 0): `M 83,83 L 95,83`; `M 105,83 L 117,83`.
- Inward (index 1): `M 83,81 L 95,85`; `M 117,81 L 105,85`.
- Arched (index 2): `M 83,84 Q 89,79 95,84`; `M 105,84 Q 111,79 117,84`.

For eye x=89 then 111, emit:

1. `E(cx=x,cy=93,rx=5.4,ry=3.4,fill=#ece5d8)`.
2. `Crc(cx=x,cy=93.4,r=1.9,fill=#1c1712)`.
3. `Crc(cx=x+0.7,cy=92.6,r=0.6,fill=#ffffff)`.

Then nose `P(M 100,99 L 97,108 Q 100,110.5 103,108; fill=none,stroke=shadow,stroke-width=1.8,cap=round)`.

### 6. Mouth, scar and frame

Mouth uses the Y defined above:

- Flat (index 0): `P(M 92,Y L 108,Y; fill=none,stroke=#6e4630,stroke-width=2.4,cap=round)`.
- Smirk (index 1): `P(M 92,(Y+1) Q 102,(Y+2) 109,(Y-2); same attributes)`.
- Grin (index 2): first `Rct(x=91,y=Y-3,width=18,height=7,rx=3,fill=#4a2c1c)`, then `Rct(x=91,y=Y-3,width=18,height=3,rx=1.5,fill=#e8e2d4)`.
- Grimace (index 3): `Rct(x=90,y=Y-3,width=20,height=6.5,rx=2,fill=#e8e2d4,stroke=#6e4630,stroke-width=1.2)`, then teeth paths x=96.5 then 103.5: `P(M x,(Y-3) L x,(Y+3.5); fill=none,stroke=#6e4630,stroke-width=1)`; no cap.

Scar is taken directly from the validated enum; do not resample or inspect numeric parity:

- none: nothing.
- left_brow: `P(M 82,79 L 93,86; fill=none,stroke=#e8c9a0,stroke-width=1.7,cap=round,opacity=0.85)`.
- right_cheek: `P(M 113,103 L 118,111; same attributes)`.

Finally `Rct(x=2.5,y=2.5,width=195,height=195,rx=12,fill=none,stroke=ring,stroke-width=3)`. Frame rings use exact DNA rarity names and table colors: club #8b93a1, professional #7fa6c9, contender #b48ce0, elite #d7dde6, generational #e8b64c. A contradictory caller ring/color is not an input.

## Independent expected examples already available

These are numerical/document-fragment expectations, not claims that full SVG byte goldens or visual tests have run. Manifest selections below come from the independent frozen corpus. Geometry expectations were computed directly from the rational formulas above, without invoking `portrait_svg`, a staged Go renderer, image tools or a rasterizer.

### Rounding and geometry boundaries

- Shoulder half-ties: jitter -5→-2, -3→-2, -1→0, +1→0, +3→2, +5→2.
- At S=75, CI=16 (`16.5` ties down to even), CO=34 (`34.5` ties down), PO=30, PM=15, PD=178, NX=20 (`19.5` ties up). These are helper vectors; a render API must still require a real regenerated manifest.
- ViewBox/layout remains exactly 200×200; for light / jitter -6, S=51; for heavy / jitter +6, S=77. No input height/reach stretch or southpaw mirroring.
- All four curly-center rows above are literal independent expected vectors.

### Zero-seed preview

Full seed is 64 zeroes; manifest case `fixed_0`, digest `4d5c2d86fa2c0e6f7d9f42c508ca1dce8ef232488ed5247d1f89547645efa58a`.

Emil Wolfe: welter/club; skin 1, hair style 1 (buzz), hair color 4, facial 1 (stubble), head 1 (square), mouth 3, brow 1, background 4, scar none, shoulder jitter -5. Expected S=57; CI=13, CO=26, PO=23, PM=11, PD=175, NX=15. Colors skin #e0ac7e, shadow #c68d55, hair #b5b0a8, background #252a24, ring #8b93a1.

Exact canonical head element:

```xml
<path d="M 100,58 C 122.4,58 132,83.55 132,94.5 C 132,110.56 127,131 100,131 C 73,131 68,110.56 68,94.5 C 68,83.55 77.6,58 100,58 Z" fill="#e0ac7e"/>
```

Exact canonical body element:

```xml
<path d="M 83,132 C 64,137 33,140 30,154 C 28,168 31,184 32,200 L 168,200 C 169,184 172,168 170,154 C 167,140 136,137 117,132 Q 100,142 83,132 Z" fill="#e0ac7e"/>
```

Exact buzz head data is `M 100,57 C 122.4,57 132,66.45 132,70.5 C 132,76.44 128,84 100,84 C 72,84 68,76.44 68,70.5 C 68,66.45 77.6,57 100,57 Z`, fill hair, opacity=0.92. The accessible label is `Portrait preview of Emil Wolfe`. There is no scar element.

### One representative for each hair branch

`integer N` below means an exactly 32-byte big-endian integer seed, rendered as 64 lowercase hex characters; it is not decimal seed text. The parenthetical case names resolve full seeds in the pinned independent corpus.

| Hair branch | Seed/corpus case | Shape | Class, jitter, S | CI,CO,PO,PM,PD,NX |
| --- | --- | --- | --- | --- |
| bald | integer 2 (`coverage_integer_2`), Dre Brand | round | welter,-3,57 | 13,26,23,11,175,15 |
| buzz | integer 0 (`fixed_0`), Emil Wolfe | square | welter,-5,57 | 13,26,23,11,175,15 |
| crop | integer 29016 / hex 7158 (`fixed_8`), Milo Iversen | long | welter,+6,62 | 14,29,25,12,176,16 |
| mohawk | bytes 00 through 1f (`fixed_2`), Otto Parisi | long | light_heavy,+3,71 | 16,33,28,14,177,18 |
| swept | integer 10 (`coverage_integer_10`), Roman Doyle | square | middle,-6,61 | 13,28,24,12,176,16 |
| curly | integer 8 (`coverage_integer_8`), Marcus Lund | wide | welter,-6,56 | 12,26,22,11,175,15 |
| flat_top | `1f08a4ac11d4c60ebecbb8dc85401fa2756576c3d1920c45cad5f7113b587ee0` (`fixed_5`), Kenny Varga | long | light_heavy,+4,71 | 16,33,28,14,177,18 |
| fade | `7893676099cf9985bb198dfb6a347d9422a19f2422cf3326a999a7230ce246cd` (`fixed_3`), Emil Mercer | square | welter,-4,57 | 13,26,23,11,175,15 |

For curly integer 8, expected six circle centers are `(69,71),(77,65),(92,61),(108,61),(123,65),(131,71)`, r=8.5, fill #5a3a22, followed by H(100,58,33,76,29). This case has goatee (index 3), grin (index 2), arched brows (index 2), skin/shadow `#a06638`/`#7d4c26`, background #2a2622 and professional ring #7fa6c9.

Additional acceptance examples:

- integer 3 (`fixed_6`), Igor Brandt: elite/light_heavy, full beard 4, wide 3, flat_top 6, grin 2, arched 2, left_brow scar, jitter -5. Expected S=67, J=126, mouth Y=115; scar exactly `M 82,79 L 93,86`; ring #d7dde6. This independently exercises beard mouth displacement and the left scar.
- all-ff seed (`fixed_1`), Yuri Lund: club/welter, stubble 1, square 1, buzz 1, grimace 3, inward 1, right_cheek scar, jitter -1. Expected S=59, Y=117; scar exactly `M 113,103 L 118,111`.
- integer 4 (`coverage_integer_4`), Omar Lindqvist: light / jitter -2 gives S=53, round head, fade, no facial hair; preserves narrow build.
- integer 11 (`coverage_integer_11`), Hector Petrov: heavy / jitter +1 gives S=74, crop, square head, mustache, left_brow scar; preserves wide build without rounding +0.5 upward.
- integer 5 (`fixed_7`), Kenny Kovac: contender/middle, S=64, round/fade/goatee/flat mouth and flat brows, left_brow scar; ring #b48ce0.
- `fixed_5` above supplies generational ring #e8b64c; integer 8 supplies professional; zero supplies club. No rarity is inferred from color.

## Bounded markup and consumer boundary

There are at most 43 graphics elements, plus root/title/desc, depth 2; a curly+goatee+grimace+scar combination gives the maximum. No per-input repetition is unbounded: two arms, two nipples, two ears, six curls, two brows, two eyes×three elements, and at most two teeth lines. Each graphic element must remain below 512 ASCII bytes under these fixed templates; root/text envelope below 1024; enforce the overall 32768-byte cap while appending, not after producing an unbounded string.

The only tags are `svg`, `title`, `desc`, `rect`, `ellipse`, `circle`, `path`. The only attributes are those fixed above. No `script`, `foreignObject`, `image`, `use`, `a`, `style`, `defs`, filters, fonts, gradients, animation, event attributes, `href`, `xlink`, `url(...)`, entity declaration, external resource, import, nested SVG or input markup is emitted. `xmlns` is a fixed namespace declaration, not a fetched resource. The sole transform is the literal-form bald-shine rotation. No SVG uploaded by a caller is sanitized and passed through; the renderer constructs every byte from constants and validated scalar values.

A future caller should use the returned SVG as a locally generated image document or an isolated controlled SVG subtree. Host CSS can override SVG presentation attributes; therefore arbitrary host styles are outside deterministic rendering guarantees. For an HTML `<img>` embedding, the caller must set `alt` to the same safe label, since the image element needs its own accessible name. Inline usage uses the root role/label above. A visible adjacent caption should identify this as a preview and expose rarity/class in text rather than relying only on frame color. The drawing is not interactive and must not become a tab stop. Actual VoiceOver/screen-reader and small/large display checks remain acceptance work; no WCAG conformance is asserted by this document. Official references: [SVG title/description structure](https://www.w3.org/TR/SVG2/struct.html#DescriptionAndTitleElements) and [SVG accessibility mappings](https://www.w3.org/TR/svg-aam-1.0/#mapping_additional_nd) (the latter is a working draft).

## Future independent acceptance, before implementation is called ready

1. Use the approved arithmetic/template/markup contract above. Build a separate reference that consumes only the frozen manifest oracle/tables and this contract, never Go renderer output. Preserve source/provenance hashes.
2. Produce exact full SVG bytes and the domain-separated SVG digest for the existing manifest corpus, plus the selected examples above. Regeneration must compare bytes, not merely recompute a stored digest. Full SVG goldens belong to the separate independent oracle artifact; this contract itself supplies numeric/fragments only.
3. Prove every hair, face shape, facial hair, mouth, brow, skin, hair/background color, scar, class, rarity and jitter boundary is covered. Synthetic trait tuples may test private geometry helpers, but the public renderer must reject any tuple not matching complete manifest regeneration.
4. Assert exact canonical attribute/number/path order, root/title/desc, no IDs, maximum element/depth/byte bounds, escaped text, and a positive tag/attribute allowlist using a parser with external entities disabled. Repeat the same render/digest without mutation and concurrently; no global mutable scratch buffers.
5. Reject unknown renderer/manifest/portrait versions, wrong table hash, every altered trait/name/rarity/class, negative/out-of-range indices, invalid JSON and oversize/deep/noncanonical input before drawing; no partial output or fallback.
6. Check all rational rounding ties and curly literal coordinates independently. Compare original Python and proposed geometry numerically for every head shape, S in 51..77 and each feature branch; tiny binary-float text artifacts are intentionally normalized, not ignored test failures.
7. Only after a renderer exists, perform a bounded visual review at small and large sizes and actual accessibility checks in supported desktop/web engines. No image tools or rendered output were used in this preparation. Cross-engine screenshots may reveal rendering defects but are not cryptographic pixel goldens.
8. Keep this entirely a preview. An official appearance assignment still requires an explicit issuance/version binding, preserved historical IDs and a separate approved rollout. D74 review is not replaced by renderer tests.

## Approved engineering choices and deferred UI integration

The root agreed to retain 200×200 source geometry/layering/class-rarity mapping, exact rational hundredths with round-even, frozen independently calculated curly centers, byte-deterministic SVG rather than pixel identity, and accessibility text without duplicate DOM IDs. Those choices are reflected above.

The root approved the proposed `fighterportrait` API, `Version1`, renderer ID and digest domain, 32 KiB output cap, intrinsic 200×200 root and explicit overflow clipping, exact label/description wording and ordered attributes, explicit `fill="none"` normalization, and exact rational decimal rules. These choices are frozen for the independent oracle and subsequent implementation.

The embedding surface (image document or controlled inline SVG) and supported browser/accessibility test matrix remain deferred until concrete UI integration. They do not block the pure renderer or authorize a UI or official generator activation. Any change to the frozen output bytes after this candidate is published requires a new renderer version/ID.
No unresolved item requires re-asking settled DNA, name/trait selection, rarity, ownership, birth, account, production-wallet or provider policy. No source file in set32/set33 or the extracted simulation was changed for this document.

Preparation checks: all 27 supported integer shoulder widths (51 through 77) were checked for exact-rational chest rounding agreement with the prototype's arithmetic; the existing independent manifest corpus covers every enumerated portrait option. The 8 representative hair cases and additional scar/build/rarity cases were selected from that corpus, not regenerated by Go. This check produced numeric data only, not SVG/image files.
