# Change set 34 — deterministic candidate fighter portraits

Branch: `slug-fighter-portraits`, stacked on set 33 implementation
`8da1c1699c23e326045197663ff412f8a298bc02`.

## What changed

`fighterportrait` renders a complete validated candidate manifest into exact
200-by-200 SVG bytes. The supplied portrait source's geometry, colors, feature
branches and paint order are retained. Integer hundredths and exact rational
round-to-even replace binary floats; independently calculated curly-hair
centers are frozen literals. Renderer version, compatibility mapping and the
separate domain-separated SVG digest are explicit. The package validates all
manifest fields by regeneration before indexing or rendering.

Output is bounded to 32768 bytes, 43 graphics and 46 total elements. Markup
comes only from fixed templates and hash-pinned tables; it accepts no uploaded
SVG, custom labels, external assets or mutable palettes. Returned buffers are
owned. Accessible preview labels, descriptions and the absence of shared DOM
IDs are fixed in the renderer bytes. This is not a claim of screen-reader or
cross-browser pixel equivalence.

`slugfighter portrait --work-hash HEX` prints canonical SVG; `--digest` prints
its separate digest. Existing DNA and manifest routes remain intact. The frozen
contract is archived as `FIGHTER_PORTRAITS.md`; its preparation-stage status
and source paths record provenance. Runtime acceptance is recorded here and
in the workspace reports rather than rewriting the frozen artifact.

## Verification and open checks

The independent Python oracle was derived without reading Go renderer outputs.
All 50 complete SVG documents, raw hashes and domain-separated digests match.
It covers every appearance branch, class, rarity and jitter, 27 shoulder-width
rounding vectors, four independently reproduced curly-center rows, 118 invalid
or bounded cases and 289 numerical geometry comparisons with the prototype.
Three reference regenerations under varied hash seed/timezone passed unchanged.
The largest corpus SVG is 4039 bytes; maximum synthetic layering is 4200 bytes,
43 graphics and 46 total elements.

Final staged normal and race tests passed in 0.513 and 2.267 seconds, with all
82 captured files unchanged. Vet/format and the 564310-execution fuzz gate
passed. Root integration has separate all-module, CLI and per-set node checks;
release results are recorded only when complete.

A runtime-generated gallery with all 50 portraits and 64/200/400px views is
retained in workspace `artifacts/set34/portrait-review`. Exact output was
compared with independent bytes. The browser security policy rejected local
file URLs, so visual and actual assistive-technology review remain open; a
user rendering check was requested. No alternate browser route was used.
This limitation must remain visible and cannot be called a completed visual
or accessibility review. Mike's later D74 joint review is still required.

## Preserved behavior

This is a pure preview library/CLI, with no official appearance assignment,
new receipt/job, ownership, birth, consensus, wallet or desktop UI change.
The set-32 Mac pilot remains the current GUI artifact. Before player-facing
portrait integration, finish visual/accessibility acceptance and explicit
issuance/version binding. Permanent genesis and product release remain gated.
