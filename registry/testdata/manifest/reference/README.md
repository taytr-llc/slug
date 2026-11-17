# Independent candidate manifest admission reference

This corpus supplies complete **hypothetical** schema-5 receipts and public
fixture storage bytes for the approved set37 contract. Normal Argon2id work,
signatures, DNA, manifest and portrait bytes are independently calculated.
Authority namespace, issuance/acceptance times and eligibility observations
are fixed test inputs. They do not establish that any authority issued a job,
admitted a prospect, or observed a canonical block. No node, live service,
registry, wallet, real key, broadcast or official activation is involved.

The approved contract is pinned unchanged in
`inputs/manifest-admission-contract.md`, SHA256
`d51178db1decf13ad09986802bb3eb7861827d372a7f35e99c13adc739420f27`.
`approval.json` is the parent's exact separate approval record. Neither approval
nor this corpus authorizes production activation.

## Sources and independence

`reference_manifest_admission.py` consumes only these independent inputs:

- The frozen set36 Python descriptor/job/candidate/signature reference and its
  entire input tree, copied unchanged under `inputs/job_reference`.
- The frozen Python portrait oracle and its pinned manifest, DNA, table and
  original geometry inputs, copied unchanged under `inputs/portrait_reference`.
- The original standalone `argon_oracle.go` used by the earlier independent
  prospect reference. It imports only the standard library and
  `golang.org/x/crypto/argon2`, and receives explicit 80-byte header inputs.

All 35 copied files are individually pinned in `input_hashes.json` and checked
before use. The minimal standalone module and checksum file are additionally
pinned in the Python script. The source contracts, old oracle scripts and old
fixture bytes have not been modified. No new Go runtime, encoder, renderer,
registry implementation, stage output or tests were read to derive expected
values. The standalone Go primitive is the sole exception to Python-only
execution, and it implements only Argon2id, not any SLUG protocol operation.

Every regeneration recomputes the entire set36 corpus exactly and compares it
with its copied frozen bytes, preserving v1 job/template compatibility checks.
It also re-renders all 50 old portrait cases to their original SVG/digest bytes.
The new receipt expectations use the resulting work hash as an input to the
independent DNA/manifest/portrait algorithms. No future Go output is used as a
golden. The script's small mutation validator is limited to its exact fixture
family; it is not a general-purpose store parser or portable receipt verifier.

## Reproduction

Python 3.9 or later and a locally installed Go toolchain compatible with the
pinned standalone module are required. Run from this directory:

```sh
python3 -B reference_manifest_admission.py --check
```

The script builds the copied primitive in a temporary directory and removes its
executable afterward. It sets `GOWORK=off`, `GOTOOLCHAIN=local`, `GOPROXY=off`,
`GOSUMDB=off` and `GOFLAGS=-mod=readonly`; an unavailable dependency/toolchain
fails rather than downloads. The module pins x/crypto v0.57.0 and x/sys v0.48.0
with the existing reviewed checksums, and has no SLUG module dependency or
replacement. `GO` can select an installed Go executable; otherwise PATH is used.
An unset GOCACHE defaults to a temporary-system cache outside the artifact tree.

Each distinct tested header is hashed once per regeneration, serially, with
literal production parameters: header as both input and salt, Argon2id time 1,
memory 262,144 KiB, parallelism 1 and 32-byte output. The display work hash is
the reversed raw digest. This is six 256 MiB calculations, not six concurrent
arenas. The public nonce search is capped at 16 attempts per fixture and selects
the first matching winning/nonwinning classification; its fixed actual search
uses six total attempts. Nothing is transmitted or submitted.

`--check` is read-only for all corpus/input files and compares exact JSON bytes.
`--generate` rewrites only `manifest_admission_v1.json`. All expected protocol
arithmetic, source selection, ordering and timestamps are deterministic and do
not depend on local time, locale, random data or dictionary hash order.
`verification.json`/`.log` record absolute Python 3.9/3.14 checks, standalone Go
version, commands, timings and stable before/after source hashes. Build-cache
files and temporary executables are not normative output.

## Exact corpus schema and use

The top-level object has `oracle_version:1`, candidate-only/trust-limit flags,
the approved contract hash, source provenance and:

| Field | Meaning |
| --- | --- |
| `descriptor`, `pin` | Exact sole supported candidate and expected schema/chain/authority/descriptor pin. |
| `marker`, `marker_canonical` | Complete required marker object; canonical string includes exactly one terminal LF. |
| `issued_jobs` | Two full Job v2 objects for public scalars 1 and 2, canonical bytes, 328-byte preimages and proof-v2 commitment scripts. |
| `public_fixture_random_stream_hex` | Authority32 followed by first and second challenge32. A private test constructor consumes the first 32 bytes; issuance consumes the remaining two chunks. Never a production RNG setting. |
| `issued_at_ms` | Fixed positive fixture time used for both job issuances, before any admission in this storage scenario. |
| `cases` | Five ordered complete proof/auth/Verified/ManifestRecord examples, their exact canonical record bytes, independent manifest/SVG, static hashes, block target, nonce and every attempted raw Argon digest/display hash. |
| `metadata`, `metadata_canonical` | Final required metadata object/bytes after both issuances and all five records. No terminal LF. |
| `expected_info` | Complete writable-store public info projection after those operations. |
| `database_rows` | Exact final meta, job and record values; record keys use full ProspectID, and sequence keys include their big-endian uint64 encoded as hex. This describes expected logical rows, not a manufactured LevelDB database. |
| `storage_inputs` | 170 marker/metadata/record byte-encoding cases with `input_hex` and `accepted_encoding`. Required-field/null/duplicate/alias/escape/order/depth/size/newline rules are tested; only the canonical fixture spelling is accepted in each group. |
| `record_mutations` | 26 inconsistent complete receipts, all refused, including unsupported binding, wrong own job, birth/clock interval, hash/DNA mismatch, high-S signature and a nonwinner falsely claiming winning-block eligibility. |
| `alternate_authorizations` | Two distinct valid low-S authorizations for the same public proof/key, with the original receipt sequence/signature that a duplicate must retain. The explicit nonce 1 is public test-only data. |
| `pin_cases` | 10 pin cases distinguishing canonical shape from exact configured-authority match. A zero authority ID has a valid encoding but does not match this fixture's different pinned authority. |
| `trust_limit_case` | A self-consistent acceptance-time rewrite within the same issued interval. It passes local structural/cryptographic checks but cannot authenticate actual acceptance; this explicitly prevents an overclaim about unsigned evidence. |
| `validation` | Counts, fixed resource ceilings, preserved old expectations and explicit absence of actual node/store contact or production activation. |

The complete `expected_record` field order is version, sequence, context, proof,
verified, authorization, job, birth, eligibility, static_binding. All children
are required values. Static binding contains version 1, descriptor ID, manifest
digest and portrait digest. Manifest/SVG strings are auxiliary oracle evidence;
they are not included in the canonical stored receipt.

Reproduce issuance in job order at `issued_at_ms`, then records in cases order,
setting each private test clock to the provided `accepted_at_ms`. The mandatory
trusted test guard supplies only that case's `eligibility_input`; tests must not
mistake these synthetic observations for an actual node guard. The same issued
job admits multiple distinct qualifying proofs, while the second public key has
a separate gym/job. Actual node integration must obtain its own observations.

## Five cases and resource meaning

1. Coinbase-only, public key 1, nonwinning work, current-parent observation.
2. Dependent legacy transactions, same job/key, nonwinning work, current-parent.
3. Mixed witness transactions, same job/key, block-target-winning work, proposed
   canonical-winning-block observation at parent height plus one.
4. Mixed witness transactions, public key 2, nonwinning work, current-parent.
5. Maximum proof shape, public key 1, nonwinning work: exactly 16,384 stripped
   coinbase bytes and 20 independent SHA256d merkle siblings. Two nonreserved
   zero-value OP_RETURN padding outputs keep each script below 10,000 bytes.

The first four contain independently serialized complete block candidates with
the original synthetic template transaction/witness bytes, explicit payout,
coinbase and merkle calculations. Those templates use synthetic outpoints;
having complete bytes does not establish UTXO validity or node acceptance.
The fifth has no complete template or block: `candidate`/`template` are null and
its opaque right-hand merkle subtrees are intentionally not asserted to be
actual transactions. It is a valid conditional prospect proof shape, not proof
of a full block. In particular this fixture cannot exercise D69 canonicality.

The maximum canonical receipt here is 37,490 bytes, comfortably below 65,536.
A maximum valid proof does not imply that the entire receipt size ceiling is
reachable. Over-size storage/input cases are explicit malformed-byte refusals,
not fabricated valid transactions. The page limit is eight records and at most
eight serial hashes, with the contract's 524,297-byte encoded upper bound.

Eligibility and birth fields remain hypothetical trusted-authority assertions.
Even a valid block-target hash cannot prove canonical inclusion. A valid gym
authorization signs the proof, not authority issuance or acceptance time.
Neither exact static hashes nor an unsigned JSON pin establishes issuer honesty,
prevents a whole-store rewrite, or grants current ownership/roster eligibility.
Independent real-node and durability tests must establish their own narrower
claims; this oracle supplies expected bytes and cryptographic arithmetic only.
