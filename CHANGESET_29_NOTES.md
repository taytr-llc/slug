# Change set 29: fixture recovery vault and delegated mining signatures

This set is built on `slug-recovery-vault`, stacked on set-28 implementation
`ca1a55b49d2a352d30eb692ef90ffbab8912466e`. The recovery vault lives in the
separate wallet repository; the ten-module chain workspace and seed deployment
remain independent of wallet custody.

The vault retains two separately encrypted payloads for each published fixture:
full recovery material and the narrowly derived gym signing key. Original-gym
recovery and mining during D72's hold use only the gym envelope. Full recovery
export and wrapping-key changes require their own capabilities and remain
blocked for the approved 24 hours. The service never returns a scalar or data
key, and offers no arbitrary-signing or generic-decryption API.

The account authority exposes an owned public schema-2 trust snapshot and
configured identity under authorization. Full-material provisioning also
requires fresh authentication and no hold, even before first enrollment.
External wrapping work releases authority/store leases; publication rechecks
the original token, generation and envelope revision. Interrupted provisioning
resumes the same durable reservation. Account enrollment and vault receipt
linking are separate transactions with explicit reconciliation.

The desktop worker can use an explicitly configured private admission signer.
It checks the mining session before and after the call and verifies the returned
typed gym signature. Provider failure cannot fall back to the local wallet.
Existing desktop defaults, fixture identities, address derivation and recovery
formats retain their behavior; this is not a new-device watch-only wallet UI.

The vault's fixed snapshot includes the exact validated default manifest.
It does not track subsequent address cursors and must not be presented as a
complete current backup of an evolving production wallet. Exact AES-GCM/AAD
contracts, wrapper trust and storage limitations are documented in the paired
wallet repository's `RECOVERY_VAULT.md`.

## Verification

Independent encryption vectors pass. Full shared account race passed in 7.060
seconds and desktop pilot race in 334.558 seconds. Later independent review
reproduced a proof-slice alias in the delegated signer: a callback could mutate
the adapter's verification baseline. The worker already rejected the mismatch
before admission, but mining could stall. The adapter now owns its baseline
and gives the callback a separate copy; the reproducing regression and all
focused signer race tests pass in 1.572 seconds. Durable recovery, reset-race,
final frozen-source normal-PoW integration and release gates are tracked below.

The complete vault race suite passed in 14.843 seconds, normal suite in 11.086
seconds and tagged suite in 0.454 seconds. Five real SIGKILL stages preserve the
original reservation across both stores. Independent corruption, reset/rotation
races, uncertain writes, capacity reservations, clock rollback and missing-material
checks pass. Cheap-profile refusal was tested with valid inputs and a mutant
that removes the guard; both constructors then fail their refusal tests.

The final four-flow normal-PoW acceptance passed in 119.657 seconds: existing
continuation 23.47, signed identity/reset 34.67, existing account lifecycle 31.89,
and vault recovery/mining 29.23. The vault flow proves actual file-only fresh
restore before a local wallet exists, exact recovery after rewrap, no fallback
on a signing outage, same-wallet reset/reopen and gym-only held mining with
historical birth preserved. All 291 captured sources and the frozen node remained
unchanged. All 15 distinct listeners closed; no owned processes remained. The
preliminary source-changing run remains separately labelled, and no new
funded-spend or complete evolving-wallet backup claim is made.

All ten chain and six wallet modules, native/Linux node builds and scoped vet pass.
The paired source pin, required chain smoke, exact wallet CI and packaged-helper
acceptance are recorded in the workspace release report after completion.

## Remaining production work

This set supports only the two published test identities, private local stores
and test wrapping providers. It creates no real provider tenant, KMS resource,
production wallet or public service. Production profile allocation, operator
resource setup, issuance, current recovery metadata, disaster recovery and a
provider that demonstrates the ordered reset contract remain launch gates.
The existing Mac Account Pilot is an ad-hoc signed fixture app; release signing,
notarization and Windows runtime acceptance are separate work.

Paired wallet implementation: `6a7db25ce7a76e203265dda29dfa5f062eb3fdcf`; desktop dependency: `v0.0.0-20261116230226-6a7db25ce7a7`. Wallet CI pins shared authority `9c7640e0f82bc0d213c8e6a749a99b7b9eaccbbd`.
