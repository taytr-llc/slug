# Independent portrait SVG candidate oracle

This directory is a self-contained Python standard-library reference for the approved `slug-fighter-portrait-candidate-v1` contract. It is a preview artifact, with no official fighter assignment or generator activation. It does not validate work, ownership or birth. D74 joint design review remains required before tester/public release.

The approved contract is copied to `inputs/portrait-renderer-contract.md`; its SHA256 is `8ae3c8dbe92fbef719880227e107cb98cbfd6419d984cad08ebb5d5e0a9c0df5`. Root review approved the API/version/ID/digest, 32 KiB cap, root attributes, accessibility text and exact arithmetic/serialization. UI embedding and the browser/accessibility matrix remain deferred to concrete integration.

## Reproduce

From this directory, using Python 3.9 or newer:

```sh
python3 reference_portraits.py --check
```

`--check` independently rebuilds every SVG and the complete corpus, then byte-compares the JSON and all 50 individual SVG files. It also verifies every pinned input hash, regenerates manifest validation from the independent manifest/DNA reference, reproduces curl centers at 90-digit decimal precision, checks rounding and all trait branches, enforces a positive XML allowlist, exercises invalid inputs, and compares the numerical geometry against the original simulation Python. It writes no output artifacts.

To deliberately regenerate after a reviewed oracle change:

```sh
python3 reference_portraits.py --generate
python3 reference_portraits.py --check
```

Generation writes only `portraits_v1.json` and `svg/*.svg`. Golden changes require review; do not update them from a Go renderer's output. The generator performs no network access, browser rendering, image generation, random draws, clock reads or key operations. All copied input paths are local to this directory; absolute original paths in `input_hashes.json` are provenance, not execution dependencies.

## Corpus schema

`portraits_v1.json` contains:

- `preview_only`, `renderer_version`, `renderer_id`, `digest_domain_hex`, `approved_contract_sha256` and pinned input provenance;
- `cases`: 50 objects with `name`, `seed`, `manifest`, `manifest_digest`, `svg`, `svg_sha256`, `svg_digest`, `graphics_count`, `element_count`, `depth`, and `byte_length`;
- `rounding_vectors`: every integer shoulder width 51 through 77 and its six exact chest measurements;
- `curly_vectors`: all four head-shape rows of six independently calculated centers;
- `jitter_rounding`: every jitter -6 through +6;
- `coverage`: all portrait indices, scar states, jitters, classes and rarities represented by the real manifest corpus;
- `validation`: numerical comparison/rejection counts and measured output bounds.

`svg` is an exact ASCII string with no terminal newline. `svg_sha256` is the plain file checksum. `svg_digest` is SHA256 over `SLUG fighter portrait SVG\x00v1\x00` followed by the exact SVG bytes. It identifies the preview document, not a fighter. Individual `svg/{name}.svg` files are byte-identical to their corpus strings.

`manifest_digest` and the manifest cases come from the existing independently derived manifest corpus. Before drawing, the oracle strictly parses and regenerates the complete manifest using the pinned independent Python manifest and DNA references. Mutating an otherwise in-range trait, class, rarity or display name is rejected. The old simulation's text-ID trait hash is never used for selection.

## Independence and verification

`reference_portraits.py` implements each approved shape with exact `Fraction` arithmetic, its own signed ties-to-even function, minimal hundredth decimals, and fixed attribute/path order. It uses no Go implementation or Go-generated expectation. The curly literal table is reproduced through a separate 90-digit decimal Taylor sin/cos calculation.

Only after the expected SVG is generated, a secondary comparison calls the pinned original `portraits.py` with trait selection replaced by the independently frozen manifest values. This checks ordered graphics, attributes and numeric path/shape values, allowing only the explicitly approved absent-fill normalization, decimal spelling cleanup and less than `2e-12` source floating-point residue. It does not supply golden bytes. No source rasterizer or contact-sheet entry point is invoked.

Current evidence:

- 50 exact full SVG cases, covering every selected trait option, class and rarity;
- 289 original-source numerical geometry comparisons, including all shoulder widths, every head/hair combination and facial-hair/mouth/brow/scar combinations;
- 118 negative/limit checks, including the inherited 64 independent malformed-manifest inputs, unknown renderer versions, derived-field tampering, output bounds and forbidden XML;
- maximum real corpus output: 4,039 bytes and 41 graphic elements;
- synthetic private-helper maximum-layer case: 4,200 bytes, 43 graphic elements, 46 elements including root/title/description, depth 2;
- output cap remains 32 KiB. The synthetic geometry helper does not bypass validation in the public `render` entry point.

`verification.json` records the exact commands, elapsed durations, Python/platform details and final source/artifact hashes. These are byte/numeric checks, not browser pixel, visual-quality or screen-reader acceptance. No such acceptance is claimed. Go implementation comparison and actual accessibility/UI review are separate gates.
