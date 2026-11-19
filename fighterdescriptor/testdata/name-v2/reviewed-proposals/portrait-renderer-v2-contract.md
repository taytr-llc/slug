# Deterministic portrait SVG candidate v2 byte contract

Status: proposed contract for root review before independent oracle generation.
This is an explicit compatibility addition for manifest v2, not a geometry,
appearance, browser-pixel or product-design change. No implementation, golden
vectors, production activation or current-default change is authorized by
this document alone.

## Exact inherited contract and tables

Incorporate the complete archived renderer-v1 contract at
`inherited/portrait-renderer-v1-contract.md`, raw SHA256:

```
8ae3c8dbe92fbef719880227e107cb98cbfd6419d984cad08ebb5d5e0a9c0df5
```

Use the same complete immutable v1 table bytes at
`inherited/full-tables-v1.json`, raw SHA256:

```
9f9380b708cae73b8d2641eaf04ecefb3a3b7fa9dd1b25b1f234f6511780b8c7
```

Their original manifest-table domain-separated hash remains:

```
72231fbe707cea9a65dd5b92f7b2719953795bceadbbf9e3b8e0b8f4aaef3f44
```

This v1 table supplies all color, shape, build and rarity-ring data. Do not
interpret the names-only v2 table as a new portrait table or regenerate palette
or geometry entries. The archived v1 arithmetic, formatting, curly-hair integer
centers, exact paint inventory/order, attribute order, escaping, envelope
structure, description text and output bounds are normative unchanged inputs.
Only the explicit renderer identity/manifest compatibility/digest changes and
the manifest-derived display-name label described below replace v1 behavior.

## Supported input tuple and complete validation

Explicit renderer selector is integer 2; renderer ID is exactly
`slug-fighter-portrait-candidate-v2`. It accepts only the fully regenerated
manifest defined in `fighter-manifest-v2-contract.md` with:

```
manifest_version = 2
generator_id = "slug-fighter-manifest-candidate-v2"
table_hash = "b537490e1e602e27de01975aeec36f5fb53ba9a3a35f98143e8ecb7f9e2687c2"
dna.dna_version = 1
portrait.portrait_version = 1
```

The exact companion manifest-contract hash is pinned together with this
contract by the new descriptor; filenames alone are not authority. All fields,
including name, must equal complete v2 regeneration for the canonical embedded
seed. Validate the whole manifest before using any drawing index. No partial
range-only validation, arbitrary text/SVG, caller palette/geometry/dimensions,
fallback component or new random draw is accepted.

Typed input is an owned value. Canonical byte input is bounded to 8,192 bytes,
copied into an owned buffer, and passed through the strict complete v2 parser
with its depth-8 and canonical ASCII rules. Failure returns no partial SVG or
digest. No filesystem, browser, clock, network or locale participates.

The old renderer selector 1 remains restricted to its exact v1 manifest tuple.
Renderer 1 with manifest 2 and renderer 2 with manifest 1 both fail closed.
Unknown selectors or mixed table/generator/version tuples are refused. There
is no latest-version fallback or reinterpretation of old portrait digests.

## Exact drawing and serialization

For a fixed seed, the v2 manifest's DNA, physical/style values and portrait
traits equal v1. V2 therefore uses identical selected graphics, coordinate
values, palette values, path commands, layer order and drawing attributes.
All inherited arithmetic remains exact: signed integer/rational operations,
round-to-nearest ties-to-even, minimal ASCII decimals, no negative zero and no
float/trigonometric/locale-generated output. There are no additional graphics.

The only full-SVG byte differences from rendering that seed under the v1
contract are (a) the accessible name derived from the v2 name rather than the
v1 name, in the two existing label positions, and (b) the explicit data-renderer
attribute value. If the two versions happen to select the same display name,
only the renderer attribute differs. The drawing body is identical.

The root opening tag is exactly:

```xml
<svg xmlns="http://www.w3.org/2000/svg" width="200" height="200" viewBox="0 0 200 200" preserveAspectRatio="xMidYMid meet" overflow="hidden" role="img" aria-label="{label}" data-renderer="slug-fighter-portrait-candidate-v2">
```

Immediately append, without whitespace between elements:

```xml
<title>{label}</title><desc>Deterministic appearance preview only. No work, ownership or birth verification.</desc>
```

Before escaping, label is exactly ASCII `Portrait preview of ` followed by the
validated v2 `Name.Display`. Apply the inherited escaping order exactly:
`&` to `&amp;`, then `<` to `&lt;`, `>` to `&gt;`, double quote to `&quot;`, and
apostrophe to `&apos;`. The same escaped label occurs in aria-label and title.
This matters for the reviewed O'Brien, O'Donnell and O'Sullivan surnames.
Do not use HTML-specific alternate escapes, numeric entity substitutions or
an additional escaping pass. No caller-provided label exists.

Append the complete inherited paint inventory in its exact v1 order, then
`</svg>`. Output is entirely ASCII with no BOM, XML declaration, DOCTYPE,
comments, pretty-print whitespace or terminal newline. Self-closing tags,
attribute order and path number formatting remain exact v1 bytes. The maximum
output is 32,768 bytes. Width/height/viewBox and all interaction/accessibility
semantics apart from the name text and explicit renderer metadata are unchanged.
No cross-browser raster-pixel or screen-reader acceptance is implied.

An implementation may factor private shared geometry code, but may not create
a caller-edited fake v1 manifest and bypass its validation. Validate the exact
supported v2 tuple first, then render the inherited geometry with explicit v2
metadata. The old renderer's output and rejection behavior must remain exact.

## SVG digest and descriptor

Digest renders the complete validated canonical v2 SVG, then returns lowercase
hexadecimal SHA256 of:

```
ASCII "SLUG fighter portrait SVG" || 00 || ASCII "v2" || 00
|| canonical v2 SVG bytes
```

Do not expose a digest helper that blesses arbitrary caller SVG. This digest
identifies an appearance document, not qualifying work, ownership or birth.

Descriptor schema/domain remain version 1. The complete new descriptor has
DNA version 1, ManifestVersion 2, the exact v2 generator/table/contract hashes,
PortraitTraitVersion 1, RendererVersion 2, the exact v2 RendererID and this
document's raw SHA256 as RendererContractHash. Separate proposed binding metadata
records both new contract hashes without self-referential document fields.
Production descriptor resolution stays unavailable. Existing default selection,
v1 descriptors, receipts, sources, images and expected vectors remain unchanged.

## Independent derivation and review boundary

Root reviews this exact contract before independent SVG vectors are produced.
The oracle uses independently derived v2 manifests plus the archived v1
arithmetic/paint/escaping reference, with an explicit v2 renderer envelope and
digest domain. It never substitutes Go renderer output as an expectation.
Include names containing the actual apostrophe components, complete manifest
validation failures, wrong renderer/manifest pairings and unchanged v1 output.
Separately compare the graphics body with v1 for equal seeds. No visual design
change, real-person portrait implication or production rollout is authorized.
