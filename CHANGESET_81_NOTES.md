# Set 81: retained fighter admission at daily pool capacity

Branch: `slug-pool-retained-admission`, stacked on Set 80 maintenance tip
`1f559221c3735227e8475731f2c73a74282db5b0`. Wallet source remains
`dfc5d9dd7e0f604486fbac812d0413fce1f45cd6`.

## Change

An ordinary pool-capacity hold used to stop both mining and delivery of a
fighter proof from an already-issued job. The explicitly selected DAILY
operator now keeps mining drained while allowing that retained proof through
the original issuer after public and round readiness. Independent publication,
admission, and authority limits still stop new admission. Later hard holds
remain effective even when pool capacity was the first reported cause.

The selected admission service also asks the original issuer to check expiry
before returning an intake hold. Only a synchronized original clock result and
successful original-session checks can produce a definite expired response.
That permits a stopped worker to reconcile expired retained work without
inventing a birth or repeatedly mistaking expiry for capacity.

Historical receipts and the final admission commit are unchanged. The former
recovers exact original evidence; the latter still checks expiry, session,
proof, node eligibility, and capacity. See [the behavior contract](POOL_RETAINED_ADMISSION.md).

## Scope

There is no new consensus rule, wire format, storage schema, signed profile,
pool-receipt requirement, or one-fighter-per-job limit. V1/default selection and
legacy constructors remain unchanged. Fighter delivery does not alter shares,
rounds, balances, or reserved payouts. D80 still permits no late fighter birth;
D84 still defers an entire oversized daily payout.

The 54-case pre-execution acceptance table and its two explicit corrections
remain separate evidence. Review corrected an error-precedence trap in the
first adapter draft: the unchanged lease helper could otherwise hide an outer
authorization failure behind an expiry result. The selected clock-only callback
now returns nil and captures its own result privately. Mixed poison remains
unknown and latches the existing service poison state.

## Verification

Focused normal and race checks each passed all 16 required top-level tests and
62 assertions. Both explicit cheap-PoW refusal controls passed. Tests cover
original-owner expiry, uncertain clock writes, error provenance, duplicates,
concurrent readiness and hard holds, full-source physical reopen, two distinct
same-job admissions, and exact historical recovery with a reserved payout
unchanged. A separate live-proof test reopens the actual issuer at its same
one-record service limit and refuses a second birth without consulting the node.

The first full core and operator attempts failed because the sandbox denied
`httptest` an IPv6 loopback listener. Their reports remain failed. Identical
successor runs with loopback access passed: 146 core and 65 operator top-level
tests, with unchanged sources, original deadlines, and clean process joins.
Before the contention successor below, the complete affected suites passed
361 top-level tests in each of normal and race mode (146 core, 65 operator, 73 service, and 77 worker). The six
expected cheap-profile skips remain explicit. All 17 module builds, affected
vet checks, formatting and whitespace checks pass with unchanged sources.

The normal-PoW smoke test passes isolated mining and report-only checkpoint
conflict checks, followed by read-only seed synchronization to height 61 and
restart. Compiled node/control inputs are unchanged from release `e3eb4d49`;
no seed rollout is needed. The initial closure diagnostic counted two Python
qualification helpers as compiled inputs; the corrected classification is
archived alongside it. Exact-tip CI remains pending.

The first actual A run failed its original 90-second readiness gate. Three
standalone status samples showed full pool intake and ready publication, but
zero displayed round summary with `busy`. All owners joined cleanly and source
bindings remained unchanged. Those samples establish failed readiness; they do
not prove that no round synchronization ever succeeded between samples. The
failed attempt is preserved in `docs/evidence/set81/actual/A1-failed/`.

The reviewed successor gives selected DAILY round Sync/List/Refresh their own
exact-Busy retries within the original 30-second step. Completed earlier phases
are not replayed, and the default path remains one call. A concrete original
issuer-gate regression requires reconciliation after release and cancellation
while still held, preserving signed payout A, ledger bytes and financial call
counts. This addresses the immediate-contention behavior without claiming that
the status samples identify which concrete gate caused every failed attempt.
The successor passes all 69 operator tests in normal mode and all 16 selected
round, intake and retained-admission tests under the race detector (71 passing
assertions). All 17 module builds and static checks pass again. Core, service
and worker implementation is unchanged from its earlier complete regressions.

Actual A2 passes the full-intake retained proof, original operator restart,
lost successful reply, identical historical replay and stopped-wallet reopen.
The operator then stays active during maturity: one scheduled transaction pays
19.8 SLUG from two rewards to the original wallet, with a 209-atom fee and one
target fighter admission. Source/profile/binary bindings remain unchanged and
all owners join. See [qualified A2](docs/evidence/set81/actual/A2-qualified/execution/report.json).

Actual B1 remains **failed**: after 100 blocks advanced while the operator was
stopped, public replay exceeded its original 90-second restart readiness gate.
It made partial progress and rounds were ready, but the harness never launched
the expiry retry or reached the final wallet audit. Its one observed scheduled
send is not a qualified B payout result. Component expiry tests pass separately;
they do not turn this actual run into a pass. Preserve [B1](docs/evidence/set81/actual/B1-failed/disposition.json)
and track the availability/rehearsal gap as [EF-12](docs/EDGE_CASE_FOLLOW_UPS.md#ef-12--extended-offline-catch-up-exceeded-the-restart-readiness-window).
Under Mike's latest triage, extended offline catch-up is deferred unless it
blocks the agreed bounded hosted test or reveals a correctness/security risk.

Coverage distinguishes component composition from direct actual scenarios.
The full-source fixture has an all-f pool target; it proves admission without
another pool submission, while inherited worker coverage proves prospect-only
classification. The one-record reopen is a service-limit test, not an exhausted
signed-authority history. Logical-byte refusal uses an explicitly conditional
private allowance. None changes a signed profile or asserts unlimited capacity.

Mike's November 21 priority separates public-launch capacity work from a safe
bounded test build. Continuous source/signed-authority expansion is deferred in
`docs/EDGE_CASE_FOLLOW_UPS.md`; hosted pool/payouts remain active, followed by
wallet balances/history, sign-in and the final joint D74 review. Marketplace
stays on hold. No existing capacity or custody guard is bypassed.
