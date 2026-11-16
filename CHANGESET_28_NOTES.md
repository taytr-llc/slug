# Change set 28: signed identity assertions and durable reset ordering

Implemented on `slug-identity-recovery`, stacked on set-27 implementation
`63a3b9820d7b28c4af0e1c121860761c236e55cc`.

The next account boundary validates signed provider assertions and prevents
pre-reset authentication from acquiring a new SLUG session after reset. An
explicit schema-2 authority pins the verifier configuration, consumes durable
login nonces, and binds sessions to the provider's authentication epoch. Token
issuance time is not treated as fresh authentication.

A verified reset first commits a durable barrier and revokes existing sessions.
The provider then completes its credential transaction and presents evidence for
the same reset event. Completion preserves the wallet and gym, starts the
approved 24-hour hold and requires the existing original-gym recovery proof.
An interrupted reset remains blocked across restart until the same event is
reconciled. This pending transaction state is separate from the fixed D72 hold.

The verifier uses pinned public RSA keys with a narrow RS256 assertion profile,
separate login and reset audiences, strict bounded parsing and a distinct reset
token type. It fetches no token-selected URLs or keys. The authentication epoch
and ordered reset events are explicit SLUG provider requirements, not properties
that every OIDC provider supplies.

## Scope and acceptance

Only the two published fixture wallets are enabled. Schema 1 and the set-27
native Account Pilot retain their existing behavior and directories. Schema 2
is explicitly created in a new directory; it has no legacy-login/reset bypass or
implicit migration. D70's 24 words, empty phrase passphrase and D72's hold rules
remain unchanged. This set introduces no MFA or manual-recovery policy.

The independent RFC7515/Python corpus covers 152 cases, including 23 accepted
assertions, and reproduces the published RFC signature before constructing
SLUG assertions. Normal, race, tagged and regeneration checks pass. The full
identity package race passed in 1.500 seconds and the complete account authority
race in 6.712 seconds, preserving all schema-1 regressions. Actual SIGKILL after
acknowledged reset begin and completion proved durable recovery of each stage.

Review found and fixed two boundary cases: expired public nonce lookup now
persists clock high-water before rejection, and schema-2 capabilities fail
closed when fewer than two unreserved audit slots remain. Acknowledged reset
completion retains its reserved slot. Both failures have focused race coverage.

The final frozen-source normal-PoW acceptance passed in 88.283 seconds: existing
schema-1 continuation (21.17 seconds), new signed-identity/reset/wallet recovery
(34.50 seconds), and existing schema-1 account lifecycle (32.13 seconds). All 271
captured source files and the immutable set-27 node stayed unchanged during the
run. The new identity flow preserved the wallet, original gym, held export
refusal, hold deadline and historical births across pending reset and reopening;
mining remained available after original-key recovery. All 16 recorded listeners
closed and no owned processes remained. This is fixture acceptance, not a new
funded-spend claim; the separately pinned set-27 wallet CI supplies funded
restore/spend coverage across all four script types.

All ten chain and six wallet modules build. Native and Linux normal-node builds,
identity vet, formatting, diff checks and independent session review pass.
Detailed commands, source hashes and limitations are in the workspace reports
`set28-account-store-checks.json`, `set28-build-checks.json` and
`set28-identity-smoke.json`. The required change-set chain smoke and publication
are recorded in the release report after completion.

Real browser authorization/PKCE, provider tenants, key rotation, arbitrary user
registration, production derivation allocation, hosted recovery vaults and real
key issuance remain subsequent integration work. The shipped desktop does not
accept raw JWTs through its webview, and no public account endpoint or wallet
service is added to the Railway seed.
