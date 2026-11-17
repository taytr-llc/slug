# Independent manifest descriptor and Job v2 reference

This is an engineering candidate corpus for the approved set36 contract. It does
not activate an official fighter generator, authenticate an issuer, issue a job,
verify work, mine, broadcast, establish birth, or change a registry or worker.

The approved contract is copied unchanged at `inputs/manifest-job-contract.md`.
Its SHA256 is
`6a761c2e78081178e45c493953b78759f721ad3772a4e1fe684f38ec5fb0cd66`.
`approval.json` records the parent approval separately so the approved bytes stay
immutable. The descriptor uses the archived original manifest contract and
approved portrait contract, rather than later mutable status documents.

## Independence and provenance

`reference_manifest_jobs.py` uses Python's standard library and copied, original
independent Python references only. It has not read the staged Go renderer,
descriptor, job, or builder implementation or their outputs to produce expected
values. Fourteen pinned input files are listed in `input_hashes.json`; every run
checks their raw SHA256 before importing the reference modules. The original
reference directory layout is retained under `inputs/reference_repo` so their
relative imports remain unchanged. No existing source or fixture was rewritten.

The archived manifest and portrait contract hashes are raw SHA256. The manifest
table selector uses its existing domain-separated hash. The script checks all
three independently from the copied bytes before constructing the descriptor.

The old references supply independently written secp256k1/RFC6979/DER routines,
context policy and gym identity encoding, v1 job encoding, transaction encoding,
merkle arithmetic and synthetic templates. The new descriptor codec, Job v2
preimage, canonical transport corpus, changed coinbase commitment and complete
candidate assembly are computed here. Existing v1 job preimages, commitments,
authorization digests/signatures and three old template candidate objects are
recomputed and compared with the copied original fixtures. Raw input hashes also
preserve their exact archived bytes.

This script does not call any Argon helper, Go command, node, network, signer
service or real issuer. The copied old proofs include independently computed
production-Argon fixtures, preserved without rehashing in this codec slice. New
candidate proofs have nonce zero and are **not** asserted to qualify as work.
Authorization examples use the published test scalar 1 and attest the ordinary
v2 proof fields; their signatures do not establish work or job issuance.

## Reproduction

From this directory, using Python 3.9 or later:

```sh
python3 -B reference_manifest_jobs.py --check
```

`--check` regenerates all expectations in memory, rechecks original parity and
asserts exact equality to `manifest_job_v1.json`; it does not modify a file.
`--generate` deliberately rewrites that one corpus file. Expected arithmetic is
integer-only, deterministic JSON uses explicit field order, and no current time,
randomness, locale or hash iteration order contributes to the bytes.

`verification.json` and `verification.log` record checks with absolute Python
3.9.6 and 3.14.3 interpreters, varied hash seeds/time zones, and identical source
hashes before and after the checks. No platform-specific executable path is
required by the generator.

## Corpus schema

The top-level object has `oracle_version: 1`, `preview_only: true`, approved
contract/domain bytes, input provenance, and these categories:

| Field | Contents |
| --- | --- |
| `descriptors` | 2 exact canonical descriptor JSON strings and IDs: compiled candidate and unsupported counterfactual; production unavailable for both. |
| `jobs` | 7 complete jobs, canonical JSON, every 328-byte preimage and commitment script. All but the explicitly unsupported counterfactual are candidate-supported. Includes both public keys, minimum/maximal issuance, zero authority/challenge, authority-only mutation and payable-template context. |
| `candidates` | 3 exact unmined Candidate v1 / Proof v2 outputs: coinbase-only, dependent legacy transactions, and mixed witness transactions. Includes original template, payout, extra nonce, complete block, merkle root, coinbase txid, base/serialized sizes and payout script. |
| `authorizations` | 3 independently derived v2 digests and deterministic low-S signatures for those candidate proofs; `work_verified` and `issued` are false. |
| `descriptor_inputs` | 68 raw JSON byte cases encoded as hex, with `accepted` and decoded `expected` or null. Descriptor parsing is byte-canonical. |
| `job_inputs` | 127 raw JSON byte cases encoded as hex. Job parsing permits standard whitespace, object reordering and escaped spellings, but requires canonical decoded values and exact schema. |
| `structural_descriptor_cases` | 59 typed structural inputs with accept/refusal, canonical bytes/ID when valid, candidate support and empty production status. Includes zero hashes, positive unknown component versions, uint32 limits and 1/96/97-character identifier bounds. |
| `constructor_cases` | 39 NewManifest inputs with complete expected job or null, including exact int64 lifetime boundaries, malformed IDs/keys, unsupported descriptor and context refusals. |
| `normalization_cases` | 3 explicit constructor/descriptor normalization cases. Only hash hex is lowercased; identifier case remains significant and can resolve to nothing. |
| `mutation_cases` | 20 immutable field changes without recomputing the ID, all refused. |
| `resolver_inputs` | 10 candidate/production lookup cases including known, unsupported, zero, uppercase and short IDs; documented error category is included. |
| `counterfactual` | Separate statement of mathematical commitment separation and operational refusal. |
| `old_parity` | Exact checks of unchanged v1 job and template expectations; no claim to rerun old PoW. |
| `validation` | Counts and explicit absence of header PoW computation or production activation. |

Typed malformed JSON values outside Go DTO ranges may fail decoding before a
runtime typed call; that is a valid refusal, not a reason to truncate/coerce an
input. Runtime tests should also check zero/nil error outputs and ownership;
those are implementation properties rather than additional golden bytes.

## Support, counterfactual and transport boundaries

The sole candidate descriptor ID is
`c1a8650a39f695814db67fddf79815ef249a13a5c0cc5af0b906b061c4c3a303`.
Descriptor structural validation deliberately accepts well-formed unknown
component tuples. It does not resolve or activate them. The production catalog
remains empty.

The second descriptor keeps DNA version 1 and changes only renderer selection
and its contract hash. Its independent descriptor ID, hypothetical JobID and
generic proof-v2 commitment differ while the other job fields remain equal. A
private oracle-only switch computes that counterfactual; it is not a proposed
runtime API. NewManifest, ValidateManifest, ParseManifestJob, commitment and
builder operational entry points must all refuse the unsupported descriptor.
Tests must not register it as a second supported runtime bundle.

Descriptor JSON is compact ordered ASCII, at most 2,048 bytes and depth 1;
escaped spelling, whitespace and reordered fields are refused. Job JSON is at
most 8,192 bytes and depth 2, with the nested raw context at most 4,096 bytes.
Job transport spelling may differ, but its decoded fields must match the exact
canonical supported value. Exact whole-job 8,192 and nested-context 4,096 limits
are accepted; one byte beyond each is refused. Duplicate decoded names, missing,
null, unknown, case aliases, invalid UTF-8, arrays, excessive depth, trailing
objects/data, numeric fractions/exponents/overflow and stored uppercase values
are refused. Old v1 input is refused by the new reader. The archived old v1
reader/fixtures remain separate; this script does not silently upgrade them.

The descriptor maximum size is a defensive limit; the bounded typed field
grammar cannot produce a valid 2,048-byte descriptor. Padding to that size is
therefore deliberately a noncanonical refusal, not a purported valid edge.

No set35 current-condition/XP selector, account authority, fighter ownership,
fight/league rule or mutable activation metadata is committed by this static
DNA/manifest/portrait descriptor.
