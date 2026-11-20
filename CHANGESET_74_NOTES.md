# Change set 74 — public pool runtime and scheduled payouts

Branch `slug-public-pool-runtime`, stacked from completed Set73 maintenance
`1aba70ca36dfbd8e492f64b33c8d329f682de56c`. The live seed remains release
`e3eb4d495455383121bc1138f2cc010c90c6fb72`. This set is in progress; no public
pool deployment or scheduled-payout acceptance is claimed.

The work order is public mining service and scheduled payouts, then sign-in and
onboarding. D74 remains the final joint product review. Preserve D58/D79/D80/D81:
per-block difficulty-weighted rounds, confirmation at 60 descendants, payment
only after coinbase maturity, one daily batch, 1 SLUG minimum, exact carry,
network fees from the pool's 1% only, and exact-byte recovery without advances.

## Completed component: recover without preparing a new payment

`poolsettle.Store.RecoverPending` uses the original transaction-recovery prefix
but returns before importing new credits, selecting funding, quoting a fee,
signing or occupying a new daily slot. It can observe or resend only the exact
already-retained transaction. Successful `complete` with no reason describes
recovery, not today's payment; uninspected available funding is conservatively
zero. Existing RunDaily behavior and the authoritative Batch.Day remain intact.

`SchedulerBinding` audits the original writable settlement and returns owned
configuration copies. It refuses read-only handles and performs no backend
operation or financial mutation. Its logical configuration digest is not a
unique database identity; the runtime must retain the original paths/owners.

Qualification: `artifacts/set74/settlement-attempt1/report.json` passed with
3,792 source/reference inputs unchanged under the existing Go 1.27.4 no-CSE
candidate profile. Focused normal tests passed in 14.814 seconds, race tests in
77.154 seconds, cheap-PoW refusal tests in 2.922 seconds and vet in 0.445 seconds.
Every owned process exited cleanly. Coverage includes no pre-due preparation,
lost-ack exact-byte retry, next-day new-credit separation, unresolved/conflicted
funding, cancellation, capacity, uncertain writes/reopen, original-owner binding
and the existing transaction/maturity/history controls. No node or public chain
was mined by these library checks. This is not full service or wallet acceptance.

## Completed component: explicit operator key owner

The internal operator owner pins separate public admission, publication, cutoff
and checkpoint roles. Online construction accepts only the first three private
file selections; public checkpoint imports need no checkpoint custody. It
retains exact original file descriptors and identity, refuses permissions/ACLs,
links and replacements, and joins signing/commit operations before Close.

Owner qualification passed on macOS: normal 1.364 seconds, race 3.094 seconds,
vet 0.185 seconds and the no-CGO refusal control 0.668 seconds. All 1,986 bound
inputs/profile remained unchanged and six process groups joined cleanly. Tests
include 32 independent signatures plus actual added-file/parent ACL refusal.
Evidence: `artifacts/set74/operatorauthority-focused-attempt4/report.json`.

The first attempt found that macOS denies the security-xattr query even without
an ACL. The corrected implementation uses the retained-descriptor ACL API;
Darwin operator custody requires CGO and explicitly refuses without it. Preserve
the first defect, a harness-only SDK symlink admission failure, and two corrected
test expectations separately. Existing node/helper settings remain unchanged.
Linux execution and downstream operator integration are still required.

## Completed component: operator admission/publication stores

Explicit version-2 profiles select independently supplied operator public keys
and fresh V2 file/database guards. Legacy V1 constructors and validation remain
closed to those profiles. Writable stores retain the original concrete owner;
audit readers need no signer. Admission, checkpoint-publication and exact
pending chain/cutoff commits retain the original operation through durable
publication. A shaped chain reference alone cannot obtain a signature.
Checkpoint import accepts already-signed public evidence, never private custody.

The independent public corpus reproduced identically under Python 3.14 and 3.9.
Registry tests consume 48 positive and 92 refusal cases, 12 canonical raw cases,
12 guard projections and four checkpoint-role cases; the remaining six finality
frames and ten frame refusals belong to the later ownership-core consumer.

`artifacts/set74/registry-attempt2/report.json` passed with 3,846 source/reference
inputs unchanged: normal 3.278 seconds, race 9.477 seconds, cheap-profile controls
1.435 seconds, no-CGO pure codecs 1.655 seconds and vet 0.404 seconds. All owned
processes joined cleanly. Concrete store tests include normal-PoW admission,
exact pending reopen, wrong original-owner refusal, audit/mixed-guard refusal,
checkpoint import without custody, and cancellation during synchronous commit.

Attempt1 passed normal/race but wrongly ran normal-chain checkpoint positives
under the incompatible cheap genesis. The successor preserves those inputs and
asserts all 12 checkpoint/publication refusals there; no production validator was
weakened. Darwin lifecycle tests select CGO as required by the qualified owner;
pure codecs remain supported without it. This is local component qualification,
not a public operator deployment or complete core/remote-reader acceptance.

## Completed component: recovery-first daily scheduler

`poolschedule` retains one original concrete settlement store and a separate
bounded schedule journal. Every start/retry recovers retained payments before
considering a new daily batch. Only a completed daily result advances the UTC
cursor; recovery, deferral, uncertainty and missing funding never pretend a
payment completed. An uncertain completion write poisons the handle and requires
reopen/recovery. Close cancels and joins active work before releasing storage.

The independent scheduler corpus contains 37 positive, 35 refusal and six
completion-guard cases. `artifacts/set74/scheduler-attempt2/report.json` passed:
compile 1.554 seconds, normal 5.925 seconds, race 22.472 seconds, cheap-profile
codec/refusal 3.281 seconds and vet 0.491 seconds. All 4,443 paired-repository
inputs and the profile remained unchanged; five process groups joined cleanly.
Concrete tests cover before-due recovery, due dispatch, same-day suppression,
uncertain completion/reopen, original-owner binding and joined cancellation.
Attempt1 was a test-only return-arity compilation error, retained separately.
The directory is trusted operator storage; pre-open checks do not promise
protection from a hostile local writer racing LevelDB's background path access.

## Completed component: concrete payout-node connection

`poolsettlenode` supplies funding inspection, fee quotation, transaction
observation and exact-byte broadcast. It has no key or signer. It requires
authenticated HTTPS with selected trust or literal-loopback HTTP, verifies the
Phase A identity and activation, checks original canonical mature funding even
through change, and distinguishes definite indexed absence from unavailable
evidence. It sends once; the settlement journal owns uncertain-delivery recovery.

Fee policy explicitly selects a node estimate or fixed atoms per kilobyte.
Independent rational arithmetic and serialized-size vectors cover decimal units,
conservative native SegWit witness size and CompactSize boundaries. Unavailable
estimation and configured caps cause deferral, never silent fallback or clipping.
Peer review found and fixed contradictory error/result envelopes and omitted
critical zero-valued fields before execution; negative controls preserve both.

`artifacts/set74/settlement-node-attempt2/report.json` passed: normal 1.197
seconds, race 3.680 seconds, cheap-profile controls 1.150 seconds and vet 0.291
seconds. All 3,869 source/reference inputs and the profile stayed unchanged;
four process groups joined cleanly. Tests exercise local HTTP/TLS models,
maturity, orphaned original funding, authentication/trust, malformed replies,
fee caps, lost broadcast acknowledgment and joined cancellation. Attempt1
compiled but the sandbox denied its local listener; unchanged sources passed
with local-listener permission. This is not an actual funded-wallet payout.
See `POOL_PAYOUT_RUNTIME.md` for the component contract and remaining integration.

## Completed component: operator ownership and public reader

Explicit operator issuer/replica/audit constructors select the V2 profile and
persist the expected role in both marker and metadata. Issuer operations retain
the same concrete online owner as the original registry; replicas and audit
readers have no signer. An immediate original-owner operation spans signing and
durable commit, including exact pending chain/cutoff recovery. The offline
checkpoint key is not loaded. Existing V1 APIs and record bytes remain intact.

The separate `/slug/ownership/v2` route uses independently expected profile and
version selection. Remote reconstruction verifies signed public evidence and
normal work; it cannot create a missing finality frame. Changed persisted roles,
substituted equal-key owners, mixed guards and incomplete public evidence refuse.
No constructor migrates an old history or changes chain order/finality.

`artifacts/set74/ownership-core-attempt1/report.json` passed with all 3,879
source/reference inputs and the profile unchanged: complete fighterownership
normal tests 9.120 seconds, complete race tests 18.126 seconds, operator
cheap-profile controls 2.334 seconds, no-CGO public/replica checks 2.810 seconds,
and fighterownership/registry vet 0.458 seconds. Five process groups joined
cleanly. The independent corpus supplies six frame positives, ten frame refusals
and six conditional compositions. Concrete tests cover public normal-work
admission reconstruction, pending reopen, boundary/cutoff, owner cancellation
during commit, ownerless serving and remote refusal to sign. Synthetic branches
in the pure corpus are not claimed as normal-work chain evidence.

Pre-execution source review corrected the new final operation check to occur
before successful cutoff completion cancels its own replay context; the original
stage is preserved externally. This component qualification does not establish
an actual funded operator transfer, remote service deployment or native V2 gym
integration.

## Completed component: immediate settlement signing authority

The original settlement now attaches a private, active capability only around
its synchronous backend Sign call. It binds the exact store/backend pointers,
source/configuration, verified inputs/claims, plan and unsigned bytes. The
dedicated wallet can check this without reentering the already-held settlement
gate. Retained or cancellation-stripped contexts cannot revive it; it expires
before signature validation, durable batch publication or broadcast. Existing
backends still receive the same inputs and the financial ledger is unchanged.

`CheckBackendBinding` audits the original store during constructor binding;
`ConfigBindingID` provides only public configuration correlation. No DTO or
matching digest grants signing authority. The independent wallet16 corpus
reproduced byte-identically under Python 3.14 and 3.9 and supplies three config
positives and six refusals here; wallet-specific vectors await their consumer.

`artifacts/set74/signing-capability-attempt1/report.json` passed: normal 8.836
seconds, race 29.072 seconds, cheap-profile/config controls 2.752 seconds and vet
0.451 seconds. All 3,883 inputs/profile remained unchanged; four process groups
joined cleanly. Controls include active/expired/foreign capability, cancellation,
noncomparable backend refusal, no reentrant gate, and the existing durable-before-
broadcast, exact retry, midnight, uncertain-write and signer-mutation tests.
This capability is not the dedicated wallet implementation or an actual payout.

## Remaining integration

- Compose the qualified operator issuer/replay with standalone pool issuance and
  maintenance. The official checkpoint key stays offline; signed checkpoint
  import verifies public evidence only.
- Independently running public pool with authenticated TLS miner transport,
  immutable enrollment/payees and joined operator lifetime.
- Concrete guarded wallet settlement backend and persisted recovery-first UTC
  scheduling, followed by actual payout, restart and lost-reply acceptance.
- Capacity continuity or an explicit finite-run limit, then remote-host rehearsal.

No real key or seed has been read or created. Existing fixture applications,
production genesis/custody gates and final native/design acceptance remain
separate. Operator host/custody configuration will be made concrete before any
required deployment input is requested.
