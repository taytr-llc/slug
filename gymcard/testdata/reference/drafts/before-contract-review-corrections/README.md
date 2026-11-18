# Set46 independent candidate-card reference

This is the independent Python reference for the exact jointly approved C1 candidate contract. Expected values are derived from the copied contracts and frozen pre-Set46 independent inputs. Runtime authors work concurrently under root approval; the reference producer has not read Set46 Go source or output. This is not a claim that these outputs preceded all runtime authoring. No runtime comparison is authorized until separate source review and corpus freeze.

C1 permits only two published fixture gyms, an explicit zero-professional-record candidate assumption, same-class matching and the unchanged inclusive half-to-double rating band. Only widening-to-five is waived. The corpus does not activate official booking, fights, results, XP, training, production ownership, random pool selection or a general league policy.

## Inputs and provenance

`extract_inputs.py` verifies the whole original Set45 corpus SHA before extracting exact JSON pointers. `input_index.json` records every origin SHA, pointer, extraction method, retained SHA and byte count. Extracted values are canonical JSON without LF; byte-copied originals remain byte-identical. Only 34 input files are retained (465,424 bytes), not recursively copied prior corpora.

The immutable finalized source retains the old Set45 normal-work/wire evidence and first certificate/finality bytes. Its six underlying admissions were previously independently verified with normal Argon; its cutoff/roster/entry histories are modeled. These six admissions cover middle, light_heavy and heavy. This reference does not repeat normal Argon or claim a validating-node accepted chain, first-anchor absence, actual account authorization or portable source authority. The previous independent verification record and normal-work table are pinned. Full consensus, live dependency leases, real sync/crash recovery and actual package flows are separate implementation gates.

The old `ordinary/synthetic-five` receipt deliberately repeats the same full prospect IDs across gyms. C1 now rejects that duplicate identity. We preserve it unchanged as input and include an explicit refusal. Standalone five-class matcher controls derive distinct openly synthetic IDs with `SHA256(ASCII("Set46 independent modeled synthetic prospect/") || ASCII(gymID) || "/" || ASCII(originalProspectID))`. Names/class/condition inputs are independently regenerated from the retained original manifest/DNA/condition sources; synthetic rating and eligibility overrides are clearly local matcher controls, not admitted receipts. No altered rating is presented as a valid full source.

`common.py` executes only named AST function definitions from the byte-pinned prior calendar and condition sources. It imports the exact old pure manifest and cryptographic modules. No original generator entrypoint, project package, Go output, network, mining, private key, fight simulation or statistical study is invoked. Gregorian/POSIX-tail calendar arithmetic is cross-checked using Python's date arithmetic and the exact retained 1,294-byte TZif, not host timezone data.

## Consumer schema

`card_v1.json` contains:

- `rules`, `pin`, `preimages`: exact ordered canonical rules and all domain/length/ID preimages, including full card-source and record preimages. Natural digest byte order is used for card recipes.
- `dtos`: `{kind,name,context,value,canonical,sha256}`. Each canonical string has no LF. This includes core primitives, source projection/state/view, full Record/Result, both fixture Views and helper Snapshot/Job/Request/Response forms.
- `codec_cases`: `{name,kind,context,base,recipe,input_bytes,input_sha256,accept,canonical_sha256,canonical_ref,canonical}`. `base` indexes `dtos`; on acceptance, `canonical_ref` points to the unchanged canonical base or `canonical` contains the exact changed canonical bytes. Refusals omit canonical-reference fields. All accepted nested object-key reorder controls rebuild canonical order recursively; array order remains semantic.
- `protocol_cases`: complete `{request,response,current_view,authority_pin,source_created_at_ms,created_at_ms,expected_job,terminal_digest,accept}` plus names. Expected-job metadata may be queued, running or terminal. The private terminal digest is SHA256 of the complete canonical CardJob, including nested completion-time Status; it is not a new public DTO field.
- `matcher_cases`: local input table, fixed identity inputs, expected Plan/canonical string or refusal, with scope label. The cases cover all four presence/eligibility choices on each side and equal/exact-double/outside ratings, all five unmatched reasons together, every class and global duplicate IDs.
- `band_cases`: scalar-only integer bounds and both ratio orientations. These are not CardSource validation.
- `identity_cases`: retained-source-valid observation changes versus explicitly invalid-signature/target **preimage-only** controls. The latter are never valid source or journal fixtures. CardID stays fixed for an event; evidence-only changes leave InputID/bout IDs/seeds unchanged; a target change changes the input/bout recipe.
- `calendar`: all 3,861 Saturdays in 2026–2099 with event/card identities and exact 167/168/169-hour following-Saturday intervals. The final deadline may be in 2100; supported event dates do not extend into 2100.
- `scenarios`, `initialization`, `history`: finite abstract journal/lease/fault expectations, not actual crash executions. History's 256 rows are audited ID/date index controls, **not** 256 complete authenticated source records. They specify at most eight selected full-row validations, exclusive cursor behavior and empty terminal cursors. Actual full-row corruption/cancellation and the 250ms helper deadline still need runtime tests.
- `bounds`: frozen formula-only conservative shape report; its simultaneous scalar maxima are size controls, not playable records.

### Finite mutation grammar

`materialize(base, recipe)` first deep-copies the base. `edits` is a sequential list of `{op,path,value?}` using slash-separated JSON pointer keys (RFC6901 `~0`/`~1` decoding) and decimal list indices. Allowed operations: `set` replaces a field/value (including a new field), `delete` removes a field/index, `append` appends to the array at the path, and `reverse` reverses the array at the path. Root replacement is permitted only for `set`.

Then `style` is `canonical` (default) or `reversed_whitespace`: the latter reverses every object key order recursively, preserves array order, and uses one-space indentation. Optional `raw` replaces the entire input with literal ASCII text. Optional `replace_once:[old,new]` replaces its first exact byte occurrence and refuses if absent. Optional `pad_bytes:N` appends ASCII spaces to exactly N bytes and refuses truncation. No other operation exists. Input SHA and length bind materialization. Helper Request/Response caps include an additional LF; standalone core caps do not. Exponents, floats, negative-zero, duplicate keys, type-confused booleans, null arrays and misplaced negative sentinels refuse.

## Validation context and limits

`core` regenerates full Record names/condition-derived matching facts from the retained complete source. Source receipt validation is deliberately a **finite exact retained-receipt domain**, not a replacement implementation of the whole Set44 journal validator. Copied old source values are canonicalized and compared; available CardSource declaration/signature, normal-work-table, Merkle, chronology and source cross-bindings are independently checked. Header work is looked up only in the pinned prior independently verified table, not recomputed.

`transport` checks closed shapes, local IDs/counts/reason/pair arithmetic, source/result bindings and canonical hashes. It never regenerates a name/condition, verifies signatures, hashes PoW, renders SVG or runs a fight. A coherent changed safe ASCII display name can therefore pass transport and standalone Plan validation while failing full Record validation. This does **not** authenticate an arbitrary name from the UI. The authoritative helper must have derived/checked it before publication. Parser outcomes reflect only available context; no generic trusted-source importer is provided.

Only inherited seed Status `scan_height` and `scan_tip_height` allow -1. Permission is passed as exact rooted paths to the Status validator; it is not a suffix-wide lexical exemption. Named Status nesting sites alone call that validator. Anchor/target heights and all other numeric fields remain nonnegative.

The journal model tracks card rows, local clock, source overlay, capacity and poison state. Its clock is an abstraction of successful local operations; it does not claim to reproduce every inherited seed/event/entry metadata write on early refusal. Source creation drift, relevant conflict, pre-sync cancellation and post-sync unknown disclosure are separate cases. Existing card replay preserves exact bytes and creation time even when conflicted or at capacity; fresh original authorization is still required.

## Verification and freeze

Run `python3 -B reference_card.py` to regenerate in memory and compare exact corpus bytes, and `python3 -B verify_reference.py` for input origin checks, independent rational-band boundary checks (150,000), separate matcher reason/count conservation, pinned-TZif calendar checks, all codecs/protocols, identity invariants and replay immutability. `verification_driver.py` runs checks with Python3.13 and3.14 in UTC and Pacific/Honolulu, retaining joined logs and before/after source hashes. Authoring `--generate` is forbidden after freeze without a separately approved successor.

The initial draft's synthetic duplicate-ID coverage issue is retained under `drafts/pre-synthetic-id-disambiguation/` with compressed original corpus/source hashes. The old input was never changed. Later local schema strengthening aligned conflict overlays to the retained first Finality time and rooted the inherited negative-sentinel permission; no Go output supplied corrections. Final source review and freeze will bind the exact final corpus and all files. Do not treat this README or a passing self-check as root's comparison authorization.
