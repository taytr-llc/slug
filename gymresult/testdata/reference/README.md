# Independent Set48 result-publication reference

This directory defines finite reference expectations before Set48 Go authoring.
It does not import or execute project Go code, invoke a node, read a database,
use private wallet keys, mine, relay, or grant production result authority.
All fixture signing material is deliberately published test material.

## Authority and provenance

`input_index.json` pins every copied input byte, its original file hash and,
for extracted JSON, the exact pointer. The genuine input is the saved November
28 Set47 C1 bout from `bout-chain-attempt2/public-bout-native-first.json`.
Its full saved Card and Seed records, both fixture Views, original independent
verification reports, and original 176-block public evidence are retained.
The later source-conflicted status is a separate input. No conflicted journal
is cleared, repaired, backdated or relabelled healthy. The new publication
examples use the earlier healthy saved snapshot as hypothetical input.

The checker independently regenerates that saved input/result once with the
unchanged, byte-pinned Set42 Python engine and verifies its exact IDs/digests.
This is reference provenance work, not a new fight, a production action or an
engine invocation by a Result codec. Compact ResultSource validation cannot
prove omitted full transcripts; actual runtime authority must come from the
concrete audited saved-Bout source. Original local-clock, fixture-assignment,
checkpoint and external-admission limitations remain.

`source_index.json` binds the pre48 Go sources consulted for inherited byte
contracts. It is not a compilation graph. The frozen contract source-basis
input additionally pins the selected 415 preimplementation files. No new Go
output is an expected value. Exact AST function/constant extraction allowlists
and selected-source digests appear in the corpus. Old Python inputs are copied
verbatim; purpose-specific corrections live only in new producer files.

## Equations, codecs and models

`common.py` loads pinned manifest, condition, calendar and engine primitives.
`crypto_result.py` derives ResultSource/Rules/Pin/Store/ResultKey/Statement
identities, published Ed25519 fixtures, key2 P2WPKH, BIP143/RFC6979/low-S raw
transactions and unchanged checkpoint message bytes. Retained-transaction
validation verifies its exact signature without signing or recreating it;
valid alternate transaction/checkpoint signatures have explicit controls.
The result and seed domains, funds and certificates remain disjoint.

`schema_core.py` implements closed public core shapes and separate full-core
versus cheap-transport validation. Both verify public Ed25519 signatures.
Transport never generates a manifest/condition, simulates, signs, hashes a
Merkle proof or performs RPC. Core compact validation may rederive the available
source semantics, but never simulates omitted results. Standalone values only
claim the relationships their fields can establish; full Action/Read validation
also binds all immutable source facts. New source-conflict overlays preserve
last result-chain observation facts and their time while exposing conflict;
that old current_finality bit alone is not current usability.

`reference_results.py` supplies canonical positive DTOs and finite parser/
semantic controls. Additional summary enums and disputed statements are
structural models, not additional saved or mined bouts. `reference_store.py`
models canonical row/reservation relationships, startup inventories and the
256-entry four-field/96KiB private-index bound. It does not execute fsync,
locks, crashes, account capabilities or process lifetimes. The helper-owned
`schema_wire.py`, `reference_wire.py`, and `reference_state.py` model exact
wire/selection/terminal correlation and explicit owner/phase/cache behavior.
They are expectations for later tests, not runtime proof. Protocol `known_publication` is request-captured immutable statement/transaction/raw/checkpoint/finality context; it is never authority discovered from the new response.

## Proof scope

`reference_proof.py` includes complete structural proof bundles with the
original genuine source records and headers, plus **unmined modeled** funding
and result blocks/headers. It explicitly reports `normal_pow_checked=false`;
no newly generated block is asserted normal-PoW or consensus valid. Normal
Argon values for the original 176 headers are inherited only from their exact
prior independently verified bytes/report, not a new project hash routine.
Future actual isolated-node acceptance must separately verify every exported
header with an independent normal-Argon implementation.

The finite checker verifies canonical shape, source hash links, signatures,
header DAG closure/order/deduplication, selected full bodies, Merkle/witness
commitments, exact raw result transaction and retained funding occurrence.
It includes separate current/historical branches and a typed-unavailable
original-funding-not-on-inclusion-path case. It does not claim full-chain UTXO
validity, public best-chain selection, seed firstness, new admission proofs,
trusted wall clocks or production ownership. Old seed crypto/parser inputs are
unchanged. The new result parser additionally enforces unique first coinbase
and commitment-present nonce requirements. Source signature/transport checks
never become chain authority.

The proof limit remains 512 unique headers, three full bodies each at most
4,000,000 bytes, and 24MiB including LF; transport/cache limits are unchanged.
The independently reviewed maximum-envelope formula and calculator are pinned
inputs. Modeled proof positives are structural positives only, never a bypass
of the normal adapter's Argon checks.

## Reproduction and review

Run `python3 -B verify_reference.py --check --report REPORT.json` to regenerate
and compare all canonical bytes and cases. `check_reproducibility.py` runs the
system and Homebrew Python serially with different hash seeds/timezones and
preserves their separate reports/logs. It never invokes Go. Verification binds
source hashes and input bytes before/after and traps prohibited codec work.
A failed report or reviewed prior producer is preserved under `drafts/` or
separate attempt logs. No expected result is changed to fit runtime output.

`ownership.json` records the distinct producer owners and independent reviewers.
The final `file_index.json` inventories the candidate after dual reproduction;
only root can approve the reference-only freeze/commit. No Set48 runtime
implementation is authorized by this directory alone.
