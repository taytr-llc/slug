# Set45 independent candidate seed reference

Freeze/approval status is supplied by `freeze.json` when present; until root approves that exact record this tree is a review candidate, not authorized for runtime comparison. This tree was authored from frozen core `51ddfc57931b1937f918f990a36c050dc9874b2d1a60b069273e314e317523df` and helper `0abdcfa84e2213bd644ad2bfe43789961c3231ef10841121fa2f039776fb8f7e`, under approval `c02f51d028a1cbd6d047901c0a229b0094498ec8fc9088038abba4b363ad8116`. Parallel Go authorship is allowed; this reference producer has not read new Set45 Go source or output. No expected value was obtained from that implementation. Root review and an explicit corpus freeze must precede comparison/copy.

`input_hashes.json` pins each exact input and its origin. The inherited Set44 corpus itself is not recursively copied: three canonical receipt strings, the coordinator Pin, two current-view projections, six canonical public admission strings, calendar checksums/samples and the exact 1,294-byte TZif are extracted by documented JSON pointers. Source bytes are unchanged. Six old admission records inherit prior independent normal-work evidence; no new claim that their modeled cutoff journals were executed is made. The five-class receipt is explicitly synthetic structural/history input. Other retained genuine receipts cover only the classes their old records actually contain.

`crypto.py` implements Ed25519 with fixed integer arithmetic (including an RFC8032 empty-message control), secp256k1 key1, RFC6979-HMAC-SHA256, strict low-S DER and BIP143, wire transaction parsing and txid/witness Merkle construction. `common.py` derives new domains, signatures, carrier IDs and Gregorian/POSIX-tail calendar values. The checkpoint message uses **internal wire-order** genesis/hash; new domains use **display-order** decoded bytes. These are separate recipes. Correctly signed alternate rules, checkpoint key or publication-chain identifier for the same pinned signer/source-event conflict key are retained only as equivocation evidence; they never make that alternate bundle supported. A main Record must still match its compiled declaration. Only explicitly published fixture key material appears; no production key or operator wallet is read.

`reference_blocks.py` constructs complete binary block bodies and calls only the byte-pinned prior standalone `inputs/argon/argon_oracle.go` for `argon2.IDKey(header,header,1,262144,1,32)`. That helper imports no project package. A finite nonce search takes the first qualifying nonce in `0..255`, retaining all attempted header outputs. The work table is serialized in sorted header order, independent of incremental authoring/cache insertion order. The linked bodies, fork bodies and exact 4,000,000-byte earlier-carrier body are **wire/Merkle/normal-work fixtures, not a validating-node accepted chain**: funding and foreign transaction input references are synthetic. Full consensus, real UTXO funding/maturity, chain admission, actual reorg and relay remain separate runtime tests. A compact Anchor is never presented as a self-contained payload proof; full body files permit independent inclusion inspection.

`schema.py` supplies closed finite reference codecs and correlation checks. It validates the new DTO rules, cryptography, bounds and nested canonical ordering. For inherited Set44 data it recognizes only the three retained exact receipt values after recursive field normalization. This deliberately bounded reference is not a general old-authority validator or a public receipt-import capability. Offline reference validation may rederive public fixture signatures; this does not authorize signing, full-block work or source projection in runtime helper polling. Header hashes in finite codec fixtures are looked up only in the independently computed normal-work table; arbitrary headers are not treated as trusted.

`reference_state.py` models finite atomic state boundaries, not RPC, mutexes, filesystem operations, account sessions or SIGKILL. Internal action names such as build/arm/ack are reference decomposition of the frozen public Publish behavior, not extra public APIs. `distinct_modeled_block_heights` is the work referenced at that modeled boundary, **not** a measurement of runtime RPC or hash counts. Funding restart summary states preserve reservations but do not purport to reproduce every raw inventory row. Model clock writes are authenticated local observations, not a transcript of all source-store highwaters; source-clock metadata and account timing still require runtime tests. Initialization cases likewise describe allowed durable recovery outcomes without claiming actual I/O. Runtime crash/lease/cancellation tests remain required.

## Corpus interface

`seed_v1.json` has `version:1` and these consumer sections:

- `rules`, `pin`: exact ordered values, canonical bytes, preimages and IDs; `fixture_keys`: explicitly public fixture material.
- `calendar.rows`: all 3,861 Saturdays in 2026–2099, exact cutoff/following-Saturday deadline, old outer event and new EventKey. The final deadline may lie in 2100; event selection may not. Integer calendar derivation is cross-checked against `ZoneInfo.from_file` on the pinned TZif and the original full Set44 calendar checksum. Host timezone data is never used.
- `crypto`: declaration/preimages/signature, unchanged-checkpoint preimage, fixed publication/BIP143/RFC6979 bytes, and tx-count-aware Merkle cases. `block_fixture_index` names the complete fixture/body/work index.
- `dtos`: each row has `kind`, `name`, ordered `value`, exact `canonical` string and SHA256 of those canonical bytes; Record rows additionally have the domain-separated RecordDigest. Marker/Init/Metadata/Rules rows are internal wire/storage/reference shapes, excluded from tolerant public-parser mutation cases, not a promise of extra exported functions. A stored marker is its canonical string plus LF.
- `codec_cases`: `kind`, `name`, zero-based `dto`, finite input recipe, expected `accepted`, exact input length/SHA256, and `expected_dto`/canonical SHA for successes. Successful normalization must equal `dtos[expected_dto].canonical`. Rejected rows have no expected canonical value. No implementation output supplies these bytes.
- `protocol_cases`: complete request and response values, current view, optional `expected_job`, optional immutable `terminal_digest`, optional expected CreatedAt, accepted bit and canonical response SHA. Terminal digest is plain SHA256 of exact canonical **SeedJob**, not RecordDigest. An expected job may be an earlier queued or terminal object; null kind is the bounded snapshot-discovered-kind case. Correlation is additional to structural parsing.
- `record_states`: each record is retained once as exact `canonical` and domain `digest`. Scenario step `expected.record_digest` selects it. `scenarios` contain named atomic model actions, error, metadata/reservations/send-count expectation, derived Status and referenced work heights. `initialization_cases`, `carrier_cases`, `scan_fold_cases`, exact-decimal RPC amounts and maturity/selection arithmetic are separate finite controls.

## Deterministic input recipes

All recipes start with a deep copy of `dtos[dto].value`, unless `raw_hex` supplies complete input bytes. `mutations` is an ordered array of `{op,path,value?}` using JSON Pointer decoding (`~1`→`/`, `~0`→`~`), with no wildcards:

- `set`: replace a map field/array index, or add the named map field.
- `delete`: remove the named field/index; no value.
- `append`: append value to the array at path.

Then apply `format`, default `canonical`: compact ASCII JSON in current insertion order. `reversed_whitespace` recursively reverses object keys, keeps array order, uses `json.dumps(...,ensure_ascii=True,indent=1)` with one leading space/newline and trailing tab/newline. `escaped_first_key` replaces the first key's first character with its lowercase four-digit `\uXXXX` spelling. `duplicate_first_key` or `duplicate_escaped_key` appends a second copy of that first decoded key before the final brace. `pad_to_cap` appends ASCII spaces until exactly `bytes`. For SeedRequest/SeedResponse this byte string excludes the mandatory framing LF, so `len(input)+1` must fit the cap; core standalone transport limits do not add that helper delimiter. Optional `replace_first:[old,new]` then replaces the first exact ASCII substring. Recipe execution verifies the recorded length/SHA before parsing. No unbounded/random mutation language exists.

## Regeneration

From this directory, using Python 3.9+:

```
python3 -B crypto.py
python3 -B reference_blocks.py
python3 -B reference_seed.py --check
python3 -B verify_reference.py
```

Ordinary checks never invoke a project Go package or regenerate work hashes. To independently recompute the retained normal-work table, build the exact copied standalone helper with `GOWORK=off GOPROXY=off` and the existing local module cache to `/private/tmp/slug-set45-argon-reference`, then run `python3 -B reference_blocks.py --verify-work`. It uses one serial 256 MiB Argon workspace. `--generate` is an authoring mode; frozen consumers should use only checks. Do not overwrite a frozen corpus to accommodate runtime output.

No public mining, provider call, GUI/D74 check, official activation, unbiasedness guarantee, live chain checkpoint installation or result/XP authority is established here. Fixture checkpoint signatures certify only the exact candidate rule exercised by these independent inputs.
