# Independent identity and reset assertions

`reference_identity.py` uses only Python's standard library and the **public
RSA private example from [RFC 7515 Appendix A.2](https://www.rfc-editor.org/rfc/rfc7515.html#appendix-A.2)**.
It performs explicit PKCS#1 v1.5/SHA256 encoding and modular exponentiation,
first reproducing the RFC's exact published signature (including CRLF payload
bytes). The key is deliberately public and must never configure a real identity
provider. `rfc7515_rsa.json` makes the same public fixture available to integration
tests that require current-time or newly generated authority nonce assertions.

The static corpus was generated from the agreed claim contract before reading
the Go verifier or its output. Regenerate with `python3 reference_identity.py
--generate`, or check both committed JSON files with `python3
reference_identity.py --check`. No network, key-file inputs, wallet creation or
external Python dependencies are used. The fixed verification time is
1700000000 seconds since the Unix epoch; each case states its own clock.

The corpus separates mathematical signature validity from claim acceptance.
Most negative cases are correctly signed, including unknown issuers/audiences,
wrong nonce, authentication freshness, epoch overflow, cross-domain reset/login
use, reset stages, duplicate/escaped JSON keys, forbidden header key URLs,
noncanonical numeric fields, and resource boundaries. Runtime tests independently
verify their RSA signatures with Go's standard library before testing rejection.

An accepted assertion is **not a session or completed reset**. Login epochs are
checked against durable account state later. Reset `verified_at` stays immutable
across fresh signed retries; the verifier permits old event verification times,
while the account store requires freshness for a *new* begin and an exact pending
event before completion. Token `iat` and expiry remain independently bounded.
Hold duration, replay behavior, nonce consumption, provider account mapping,
and cancellation/commit races belong to account-store and integration tests.

Email, `amr`, `acr` and ordinary unknown metadata do not select a principal or
introduce an MFA policy. Exact `(issuer, subject)` is the principal boundary.
No production tenant, MFA requirement, recovery factor or real key is invented
by these fixtures.
