# Set31 independent coordinated identity fixtures

This corpus was prepared outside both repositories before set31 implementation,
then copied into chain testdata without changing the oracle or expected JSON.
`preparation_verification.json` preserves that earlier command/hash evidence.
The runtime bridge is `gymidentity/coordinated_reference_test.go`. No provider
resource, wallet or live deployment is created by these fixtures.
All signing uses the intentionally public RSA private example in
[RFC7515 Appendix A.2](https://www.rfc-editor.org/rfc/rfc7515.html#appendix-A.2.1).
It is not an operator/provider credential. Do not configure a real provider with
this key.

```sh
python3 gymidentity/testdata/coordinated/reference_coordinator.py --check
```

`--generate` writes `coordinated_identity_v1.json` in this directory. The script
uses only Python's standard library. It accesses no network, imports no Go code,
reads no private files and performs no wallet/key generation. Its raw modular
exponentiation is variable time and suitable only for this published fixture.

Before producing new tokens it checks the copied baseline source/data hashes,
reproduces the RFC's exact signature, and regenerates all152 existing identity
vectors (23 accepted). `baseline/` contains unchanged copies of that independent
reference and its prior corpus. The prior reference first verifies each produced
signature using the public exponent; expected bytes never come from Go output.

The new fixed clock is1700000000 Unix seconds. Header, payload and signature use
canonical unpadded base64url; ordinary bodies use compact ordered JSON. Negative
vectors intentionally contain correctly signed invalid forms. The new verifier
configuration pins issuer `https://identity.slug.invalid`, audiences
`slug-provider-fixture-login` / `slug-provider-fixture-reset`, epoch claim
`slug_auth_epoch` and kid `rfc7515-a2`. This does not add MFA or provider-assurance
rules.

Each case records its expected immutable binding, flow/event/nonce, verification
clock, mathematical-signature status and **verifier** acceptance. Accepted output
omits `VerifierConfigHash`: the real verifier computes that from its configuration;
no synthetic digest is claimed to be its expected output. Correctly signed subject
changes, unknown/relabelled original authentication IDs and reused provider
operations can pass verification while failing the coordinator's durable checks.
They are not mislabeled as cryptographic failures.

The corpus covers both action domains and every required claim, immutable scope
substitution, missing/null/unknown/case-alias/duplicate/escaped keys, malformed
UTF8, exact JSON integer epochs including9007199254740993 and MaxUint64, reset
increment overflow, type/audience/header restrictions, nonce/flow/event binding,
freshness and encoded/decoded size bounds. No float64 or JavaScript Number is used
to carry epochs. A consumer must likewise preserve uint64 values exactly.

`coordinator_scenarios` are labeled **unexecuted contract expectations**. They
cover original-auth reuse, current-epoch relabeling, two handles for one operation,
one handle for two operations, stage confusion, unknown login outcomes, historical
receipts, unchanged reset verification times and pending barriers without provider
proof. These entries are a future test plan, not evidence that a journal/crash or
provider implementation has passed. They invent no abort or timeout-release rule.

Tuple-index vectors include exact length-prefixed bytes and SHA256 for the
versioned original-authentication/reset-operation domains. The explicit frozen
contract order is **trust digest, issuer, subject, tenant, client, connection,
provider ID**. The earlier review's prose listed subject later; parent confirmed
the exact contract order is authoritative. Each component is LE32 byte length plus
UTF8; hex remains lowercase ASCII, not decoded32-byte data. The `ab`-repeated trust
digest is an explicit synthetic codec input, not a derived verifier ConfigDigest.
Same-component/different-domain and ambiguous-concatenation vectors check that
domain separation and length prefixes change the actual encoded/hash result.

Source contracts:

- `/Users/taytrrip/slug/set31-coordinator-contract.md`
- `/Users/taytrrip/slug/set31-provider-review.md`

The contract's unresolved real-provider conformance gates remain unresolved.
These public fixture vectors do not establish a real tenant's original-auth,
precommit, operation-correlation, reset-completion or exactly-once guarantees.
