# Change set 31 — fixture provider coordination

Branch: `slug-provider-coordinator`, stacked on set 30's
`slug-production-profile`. This set adds a durable coordinator between signed
fixture identity evidence and the existing schema-2 account authority. It does
not enable a public provider endpoint or change the desktop's fixture login UI.

## Behavior

`gymidentity` verifies a separate, closed coordinated claim contract using the
existing bounded RS256 parser and the published RFC 7515 key. Required signed
bindings cover coordinator, authority, tenant, client, connection, server-issued
flow/nonce, original authentication or reset operation, and exact uint64 epochs.
Existing general identity methods keep their prior behavior.

`gymprovider` pins that public fixture trust and journals reservations, immutable
authentication origins, reset operations and acknowledgements with synchronous
LevelDB batches. It binds each provider operation to one server-issued reset
handle, keeps original authentication time and epoch across reuse, and delegates
account epochs, sessions, pending barriers and holds to `gymaccount`.

A provider receives permission to finish a credential change only after an
exact pending authority barrier and durable journal acknowledgement. Completed
historical begins return the original completed receipt with `CanCommit=false`.
Completion retries preserve the original generation and hold. Abandoned pending
resets remain pending until the same committed operation supplies signed
completion; there is no timeout unlock or invented successful callback.

The journal does not retain assertions or session credentials. A lost login
acknowledgement cannot reconstruct a token; unknown consumption requires a new
login flow, and an origin still only prepared after an uncertain outcome cannot
become accepted by reuse. Durably accepted origins remain reusable within their
original freshness even if the first response was lost. Durable capacity counts include abandoned reservations. Corrupt or
uncertain state fails closed until a valid reopen. Clock highwater persists on
expiry rejection and in prepared batches, including a verification that crosses
a second boundary. Closing drains accepted operations before closing the owned
journal; it does not close the borrowed account authority.

See `GYM_PROVIDER.md` for exact fields, tuple encodings, state transitions and
remaining real-provider guarantees.

## Independent and composed verification

The independent Python oracle first reproduces the RFC signature and the prior
152 identity cases, then checks 306 coordinated cases: 28 accepted and 278
rejected. Correctly signed semantic conflicts are distinguished from signature
failures. Canonical epochs include values above JavaScript's safe integer range
and MaxUint64. The corpus's declarative coordinator scenarios remain labelled
unexecuted; the Go journal tests supply separate runtime evidence.

The journal tests cover immutable-origin and operation substitutions, capacity,
uncertain writes, corruption, clock rollback, close/cancellation, and eight real
subprocess-kill boundaries. Reopening both databases checks bounded orphan
attempts, unavailable session credentials and exact reset replay.

The test-only Auth0-shaped JavaScript harness passes 58 cases with a synced
fixture resolver, exact BigInt serialization, one bounded operation deadline,
static errors and documented event/handler shapes. It models required provider
behavior; it does not prove a real tenant's route coverage, operation identity,
original-authentication provenance or abandoned-flow reconciliation.

The new normal-PoW desktop composition runs both published wallets through
actual authorized export and file-only restore, enrollment, mining, abandoned
reset/restart, exactly one simulated provider credential mutation, completion,
same-gym recovery and continued mining under the hold. It checks stale original
authentication rejection, other-account isolation, preserved fighter birth,
blocked export/rewrap and zero recovery-envelope unwraps during held mining.
The first diagnostic run passed in about 55 seconds and closed all six owned
listeners; a source change during that run correctly prevented it being accepted
as frozen-source evidence. The final run passed in 55.54 seconds (package
55.930 seconds, wrapper 56.897 seconds), with all captured source hashes stable,
six configuration-discovered listeners closed, eight retained listener records
checked by the test, and no remaining owned processes. Both accounts performed
exactly one simulated credential mutation and zero held recovery unwraps.

Post-fix full race suites passed: `gymprovider` 9.223 seconds and `gymidentity`
2.510 seconds. Tagged suites passed in 7.565 and 0.822 seconds respectively.
All ten chain and six unchanged wallet module builds, relevant vet, and native/
Linux normal node builds passed with stable runtime hashes. The independent
JavaScript harness passed 58 cases in 1.714 seconds. Workspace reports preserve
exact source hashes and earlier diagnostic evidence. The required per-set smoke
and subsequent release observation are recorded separately at promotion.

## Preserved boundaries and release gates

The wallet source remains set 30, including the unfinalized production issuance
gate, existing schemas and candidate profile. Its exact CI and funded four-scope
recovery/spend evidence remain applicable. This set introduces no wallet entropy,
operator key, real provider/KMS resource, public-chain mining or consensus change.
The set-30 Mac artifact remains the latest UI build; this coordinator is a
library and test composition, not a newly integrated hosted-login screen.

Mike's D74 thorough joint design review remains mandatory before friends/tester
or public product release. The newly supplied simulator and prototype handoff
is preserved in the workspace with provenance/checksums; its compatibility audit
and subsequent ports are separate work. The spec takes precedence over Python,
the marketplace pricing proposal remains unsettled, and first-pass mockups do
not constitute the final product design review.
