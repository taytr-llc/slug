# Change set 44 — candidate event snapshots

Branch `slug-event-snapshot`, stacked from set 43 implementation
`17e0362244c2bd419ed30ee902887b0ece2ad268`. Implementation and focused validation
are complete, with final source gates and reviewed source hashes recorded.
Exact package acceptance and deployment remain pending; no release or live
observation is claimed yet.

D76 and D77 are accepted and recorded in the spec and `EVENT_LOCK_POLICY.md`.
They freeze the complete noon eligibility/ownership/condition snapshot and
require waiting without seed replacement or automatic cancellation, with a
chain-incident escalation threshold at the next Saturday lock.

This slice implements bounded, recoverable candidate snapshots using retained
roster/entry history and original source evidence. `EVENT_SNAPSHOTS.md` describes
the candidate scope, lifecycle, desktop route and remaining authority gates.
The exact core/helper contract was frozen before implementation/reference
comparison; a separate terminal-history-cursor clarification preserves the
original contract record. The independent Python reference is frozen at
`gymevent/testdata/reference`: corpus SHA-256
`9287f92c0cb46c585b44274c70ae57c5d6c9a4234e7078fd1d69a200644f996a`,
freeze `08fe99f3d6763289397dc3ce96bc96c42ad8d089e5d09a895e908429bafa63d0`.
It contains 3,861 calendar cases, 36 modeled histories, 14 receipts, 97 DTOs,
543 codec cases, 136 command correlations, 35 modeled scenarios and 42
initialization cases. Four Python-version/timezone regeneration checks were
byte-identical. The producer did not read new Go source or output; runtime
authorship proceeded in parallel after contract freeze. The fixture
assignment model cannot establish transferable production ownership. Actual
seed selection/finality and operator incident handling remain later integration
requirements. Existing D65/condition/fight-engine behavior, birth, chain
consensus, wallet derivation and D72 restrictions remain unchanged.

The prior set's package/deployment closeout is recorded in
`CHANGESET_43_NOTES.md`. Mike's portrait check remains pending. Hold one full,
thorough joint D74 design review at the end of the major-flow build-out, when a
representative working product is ready, before friends/tester/public release.
This slice's code, DOM and helper checks do not replace that session.

## Completed source verification

All ten chain-module normal builds pass. Core normal tests cover `gymevent`,
`gymstable`, `gymentry` and `gymaccount`; focused race, scoped-clock,
cheap-PoW refusal, codec fuzzing, vet and formatting gates pass. Independent
comparisons cover the frozen calendar, DTO/codec bytes, historical journal and
initialization models, participant snapshots and complete receipt digests.
Runtime tests separately exercise genuine normal-PoW sources, bounded journal
replay, writer fences, clock rollback, original-session revocation, uncertain
writes and durable replay. Modeled oracle scenarios are not substituted for
these runtime tests.

Helper normal/race checks preserve the existing Manifest, Stable and Entry
families alongside Event v1. The helper comparison checks 230 frozen DTO/codec
cases, and final focused tests cover queue publication, shared-lane exclusion,
reciprocal cache reservation, stopped reads, immutable cached results,
post-commit uncertainty and borrowed-store lifetime. An ordinary-build import
probe rejects `NewEventsForTestClock`; the identical tagged compile control
succeeds. The tagged fixture verifier retains the same published identities,
reset nonce and cancellation checks, changing only the scoped authentication
timestamp after successful verification.

Actual parent installation SIGKILL tests pass at `event-prepared`,
`event-created` and `event-ready`, including second-fixture preservation, empty
prepared recovery, retained partial-state refusal, old-route refusal and
missing-ready refusal. Normal/race package times were 43.096/252.792 seconds.
Final actual `NewEventsForTestClock` → `ServeEvents` → native pipe composition
passes in 23.244/115.063 seconds, normal/race. Two fresh public fixture wallets
produce genuine normal-PoW records and saved entries before cutoff; both gyms
then capture, read and replay the immutable receipt. Account switching, stale
views, reset-held reads and history are checked. Each final run closes eight
captured private listeners and four exact owned node commands; no owned node
process remains.

Native main/helper/bridge normal tests and the full bridge race suite pass,
followed by final focused Event normal/race and vet checks. All 146 tests across
the five frontend suites pass,
including selected-date invalidation, bounded retained render identities and
visible capacity refusal. Packaging shell syntax checks pass. Independent
core/cutoff, helper lifetime and native/frontend reviews report no open finding
within their recorded scopes. These are source and component gates, not exact
bundle acceptance or visual acceptance.

Workspace evidence is recorded in `set44-core-checks.json`,
`set44-helper-implementation-checks.json`, `set44-helper-owned-files.json`,
`set44-root-final-delta.json` and `set44-final-build-checks.json`, with exact
source hashes and linked logs. Earlier diagnostics remain preserved: the first
root capture passed every command but detected an unrelated reference-test
edit; the installer/composition capture passed every command but detected the
capture-time validator and regression changes. Final affected checks and
composition bind the corrected bytes. Core vet also prompted a named-field-only
correction to two identical fence literals; exact before/diff evidence and
final core gates cover that dependency change without repeating installation.
Earlier sandbox and harness failures remain diagnostic records, not passed
acceptance. Review fixes include the capture-versus-event-creation time bound,
the queued-capture date race, capacity feedback and duplicate render-result
retention.

The regular Candidate package remains a normal-PoW, real-clock build. Closed
snapshot composition uses the separately compiled `slugtestclock` test harness,
with genuine immutable fixture records. The normal package cannot claim a
successful real-clock closed cutoff before the next eligible Saturday; that
coverage remains explicit debt. The event seed authority is unavailable in this
slice, so a calendar deadline is not a measured chain incident.

The actual native test-clock composition advances from one minute before cutoff
to one second after it. Following-Saturday/DST arithmetic is covered by the
independent calendar/core tests, not a native session jumped forward one week.
No GUI/window, portrait-render review, screen-reader acceptance, D74 completion,
official transferable ownership or seed-finality observation is claimed.

## Subsequent engineering closeout

Implementation `a00ada0d2d3c5e903a4160e598905c667c10ee32` was pushed and
promoted as master `1fc307e07eec1be7f2466e0aabedb505f8844b0d`. The exact Mac
bundle passed its two-fixture acceptance in 111.905 seconds. Independent
verification covered 14 normal-work records, 40 stable items, six fight
previews, ten entry receipts, six real-clock event phases and coordinator
restart. All 35 deliberate evidence mutations were rejected as intended.

Railway deployment `164d4386-deef-4502-8466-a5dd290f81f0` succeeded with the
same volume, network and configuration. Normal-PoW smoke and the full
900.044-second live observation passed: 31 samples retained height 61, one
seed peer and 60 checkpoint descendants. Chain verification, stale-template
refusal, restart and clean owned-process shutdown passed. No public-chain
mining occurred. The seed remains node-only.

Remote GitHub returned no exact-commit workflow runs or checks; remote CI is
not claimed passed. The local gates and their diagnostics remain preserved.
The first commit attempt stopped on whitespace in an original reference
input; the reviewed single-path exception preserved its exact bytes. The
first binary freeze could not locate the symbol tool under inherited cache
settings; the unchanged script passed with the configured writable cache.
Neither attempt changed runtime source or reached deployment.

Workspace `set44-release.json` binds the completed evidence and
`set44-closeout.md` records the later rollout separately from the original
pre-package capture. Real-clock closed-event coverage, the portrait check and
the complete joint D74 review remain pending as described above.
