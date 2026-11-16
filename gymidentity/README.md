# Fixture identity verifier

`gymidentity` validates signed assertions for the explicitly created schema-2
`gymaccount` authority. The existing desktop fixture login and schema-1 stores
keep their existing behavior. This package does not provision a provider, run a
login HTTP endpoint, create credentials, or select a production wallet profile.

The provider contract has two additional requirements beyond ordinary OIDC:

1. Every login assertion carries a positive `uint64` authentication epoch under
   the configured claim name. It is the epoch of the **original authentication**
   and must remain unchanged on refresh. A fresh `iat`, new nonce, or lookup of
   the account's latest epoch cannot upgrade an old authentication.
2. Credential reset uses separately signed `begin` and `complete` assertions.
   The provider waits for the authority's durable begin acknowledgment before
   changing credentials. The authority revokes sessions and blocks access while
   pending. Completion pins the next epoch, requires wallet-control recovery,
   and starts the existing 24-hour D72 hold. A provider that cannot guarantee
   this ordering is incompatible with this adapter.

No MFA, email-verification, manual-review, or new recovery policy is inferred
from token claims. The existing same-wallet recovery and hold rules remain in
`gymaccount`. Ordinary ID tokens cannot authorize reset.

## Native API

Construct a verifier with `NewVerifier(Config)` using explicit public trust:
`Version: 1`, exact HTTPS `Issuer`, distinct `Audience` and `ResetAudience`, a
lowercase `EpochClaim`, and `PublicJWKS`. Pin `ConfigDigest()` in
`gymaccount.IdentityConfig` when creating or reopening a schema-2 authority.
The digest includes the complete normalized public keyset and all trust fields;
formatting and key order do not change it. Trust changes require an explicit
authority configuration change, not silent replacement at reopen.

For login, call `Store.IssueLogin(ctx, issuer)`, send its nonce to the trusted
provider, and pass the resulting assertion to
`Verifier.Login(ctx, store, attemptID, assertion)`. This helper reads the durable
nonce itself. The store consumes the attempt and creates its sole session in one
synchronous batch, checking the configured principal, pinned verifier digest,
credential epoch, reset barrier, and durable attempt sequence again at commit.
Replaying an attempt never creates another session. A pre-reset attempt cannot
be reused after completion, even with a fresh assertion. A lost session response
requires a new login attempt; raw session tokens are not recovered from disk.

`VerifyLogin(ctx, assertion, expectedNonce)` is also available for native trusted
adapters. Its output proves assertion validity, not account access; it must go
through `Store.LoginWithAssertion`. Do not accept `VerifiedLogin` or
`VerifiedReset` from JSON or webview input. Epochs remain Go `uint64`, including
values above JavaScript's exact integer range.

Use `VerifyReset(ctx, assertion)` then the matching `Store.BeginReset` or
`Store.CompleteReset`. Reset assertions bind issuer, subject, event ID, original
verification time, stage, and consecutive epochs. A fresh transport assertion
may reconcile an old immutable event after an outage. The store checks original
verification freshness only for a new begin, and requires identical event data
for retries. Identical replay does not start another hold. Pending reset never
expires into access or silently aborts.

## Assertion profile and limits

Only compact JWS `RS256` with pinned `kid` is accepted. Login header `typ` is
absent or `JWT`; reset requires `slug-reset+jwt`. Other header fields, including
`crit`, `jku`, `x5u`, and embedded `jwk`, are rejected. Public JWKS permits at most
eight RSA keys of 2048–4096 bits with exponent 65537, unique `kid`, and optional
`alg: RS256` and `use: sig`. Private key fields and other JWK members are rejected.
There is no key fetching, unknown-key refresh, alternate algorithm, or network I/O.

Login requires `iss`, `aud`, `sub`, `nonce`, `iat`, `exp`, `auth_time`, and the
configured epoch claim. Its nonce is the authority's canonical lowercase
32-byte hex value. Audience may be a string or at most eight unique strings;
multiple audiences require matching `azp`, and any supplied `azp` must match.
Reset audience is one exact string. Reset requires `iss`, `aud`, `sub`, `iat`,
`exp`, `event_id`, `stage`, `from_epoch`, `to_epoch`, and `verified_at`.
The event ID is canonical lowercase 32-byte hex, including the all-zero value;
the store determines whether the event exists. Epochs are positive and reset
increments exactly once without overflow.

All security numbers use unsigned decimal JSON integers, with no exponent,
fraction, quoted number, or floating-point conversion. Times are positive whole
Unix seconds converted safely to milliseconds. Transport lifetime and freshness
are at most 300 seconds; expiration is exclusive. Authentication must be no more
than 300 seconds old and no later than issuance. `iat` never substitutes for
`auth_time`. Optional `nbf` must be valid and already reached. No clock skew grace
is added. Ordinary extra payload claims are ignored and grant no capability.

Limits include the entire input: 64 KiB JWKS, 16 KiB compact assertion, decoded
2 KiB header, decoded 8 KiB payload, and 512-byte signature. Canonical unpadded
base64url, valid UTF-8, depth at most 16, and unique decoded object keys are
required, including ignored nested metadata. Invalid surrogate escapes are
rejected. Required null values and case aliases of security claims are rejected.
Errors contain no assertion, claims, provider response, URL, or key material.

## Evidence and remaining boundary

`testdata/reference_identity.py` independently signs and checks assertions using
Python's standard-library modular exponentiation and the **publicly disclosed
test key** from [RFC 7515 Appendix A.2](https://www.rfc-editor.org/rfc/rfc7515.html#appendix-A.2.1).
It reproduces the RFC signature before generating the corpus. Never use that
key as a real provider key. Unit tests exercise the pinned configuration, caller
ownership, cancellation, concurrency, durable-nonce adapter, and redacted errors.

The standard ID-token claim meanings follow
[OpenID Connect Core](https://openid.net/specs/openid-connect-core-1_0.html#IDTokenValidation).
The authentication epoch and reset barrier are additional explicit SLUG adapter
requirements; this package does not claim compatibility with arbitrary OIDC
providers. Real account provisioning, production wallet profile, recovery vault,
operator enrollment, and any provider-specific browser flow remain separate work.
