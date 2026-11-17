# Independent manifest-service transport reference

This is the set39 reference corpus for approved contract
`c25e2fb7f7361a0207acea952c44d62e7fcc9a7ebeeba5887fb2cf3d2b9ebfdf`.
It was generated before reading any set39 Go runtime or result. It creates no
wallet, key, provider, actual job, receipt, server or public-chain operation.

`reference_manifest_service.py` uses Python standard-library JSON, integer,
SHA256, base64, URL/IP and public-certificate parsing operations. The complete
cryptographic proof/job/auth/DNA/manifest/portrait expectations are copied
unchanged from the frozen independent set36/37 references, whose source and
input hashes are retained in `inputs/admission_reference`. The original
isolated Argon helper is an archived provenance input and is not invoked by
this new transport generator. No SLUG Go code is imported or executed.

New expectations cover:

- 102 client/server configuration cases: exact field schemas, mode selection,
  endpoint normalization, all three trust modes, token exclusion, immutable
  authority separation, custom certificate set sorting/deduplication, strict
  PEM refusal,64/65 distinct public credentials, raw byte caps and canonical
  normalization expansion beyond the output cap.
- 130 issuance response cases, including the original127 independent Job2
  parser cases byte for byte, plus authority, requested-gym and current-target
  substitutions. A bad response after dispatch is outcome-unknown even when
  the operation was issuance; this does not promise exactly-once issuance.
- 339 admission response cases: all five original receipt families as new and
  historical results, maximum16,384-byte coinbase/20-sibling proof, genuine
  block-winner eligibility, alternate valid fixture signatures, changed-policy
  historical recovery, every nested object’s missing/null/unknown/duplicate
  fields, noncanonical numeric/hex encodings, exact component byte limits,
  depth and73,728-byte response limit.
- 39 stable error-envelope cases pin the status/code/outcome flag relationship.
  The worker config is a separately typed public fixture with the independently
  derived transport ID. It contains no bearer credential.

The two certificate inputs are **already published Go standard-library public
test certificates**. `certificate_provenance.json` records the exact source
version, symbol, original source hash and selected PEM hash. No private key
bytes were copied or used, and no certificate was generated. Python/OpenSSL
checks that these public inputs parse as certificates; trust fingerprints and
the full domain-separated trust-ID preimages are independently derived from
the raw DER bytes. `inputs/GO_LICENSE` retains the upstream license. The
64/65-client boundary uses the public deterministic secp256k1 multiples1..65,
not generated or retained wallet keys.

Not every storage mutation is a client rejection. In particular, the frozen37
`sequence_above_meta` and `accepted_above_clock` examples remain valid transport
statements when sequence is positive and acceptance is inside the original job
interval. A client has no independent witness to the authority’s registry count
or clock. The corpus marks these as accepted and explains why. The same applies
to a self-consistent acceptance-time rewrite: public work, signature and static
binding do not authenticate the historical time or node observation. An
authenticated configured authority remains a trust boundary. The corpus does
not claim arbitrary JSON is an admitted record or current ownership.

The script’s config routines model only the explicit fixture grammar needed
for this corpus; they are not a general URL, PEM or production service library.
Receipt outcomes are explicit contract mutations of verified frozen inputs,
not a second generic admission verifier. JSON formatting tolerance is separate
from decoded canonical field values and immutable storage bytes.

Regenerate and check without Go or network:

```sh
python3 reference_manifest_service.py
python3 reference_manifest_service.py --check
```

`input_hashes.json` pins all49 original input/provenance files. The generator
refuses altered inputs. `verification.json` records absolute Python3.9/3.14
interpreter paths, varied hash seeds/time zones, exit codes, timings and stable
before/after hashes. Temporary certificate parsing files are automatically
removed before completion. The Go bridge may adapt to the frozen public API;
it must never regenerate expected values from the implementation.
