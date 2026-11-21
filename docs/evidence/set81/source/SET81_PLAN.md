# Set81 proposal: finish retained fighter delivery at pool drain

Status: source-only plan, not implementation or qualification. Root selected this
priority after Set80. No Set81 product code, producer output, Go execution,
network access, or repository edits are authorized by this document. Start only
after Set80's accepted A actual, desktop race qualification, final checks and
commit are complete; bind that exact committed predecessor before authoring.

## Problem and minimum outcome

The automatic worker already reconciles retained outbox records before its
new-issue/capacity branches (`prospectworker/pool_worker.go`). It synchronizes a
fighter authorization before delivery and preserves pending work on capacity.
There is no missing generic worker retry mechanism.

The operator currently couples two different admissions. In
`desktop/pooloperator/capacity_drain.go`, every latched intake reason drains both
the pool source and `admissionServer`. In
`prospectservice/manifest_fenced_server.go`, the exact original proof and any
historical receipt are prepared first; a nonhistorical proof then receives
`capacity` from the admission drain guard. This can reject a still-valid fighter
from an already-issued job solely because the pool's ordinary financial intake
filled. After expiry, the same guard also precedes the original issuer's final
clock check, so a signed, never-admitted record can keep receiving capacity
instead of an authoritative expired terminal.

Set81 should let the explicitly selected DAILY operator finish eligible fighter
admission from original issued jobs while pool-only intake is drained, within
the existing independent admission/publication limits. It must also distinguish
historical recovery from never-admitted expiry. It is a finite availability fix,
not continuous capacity, a new birth rule, or permission to renew old work.

## Existing policy and original authority

- D68 keeps the original five-minute issuance window and issuance-time birth.
  New acceptance at or after expiry is forbidden. A job may yield multiple
  qualifying proofs; this plan introduces no one-fighter-per-job restriction.
- D69 still broadcasts a block first and permits the original prospect only
  through current-parent eligibility or the exact canonical winning block,
  with the original job still unexpired at final commit.
- Pool target, prospect target, block target and their delivery are independent.
  A pool receipt is neither necessary nor sufficient for fighter admission.
- Existing original-session capture/commit/recheck, gym signature, proof,
  immutable descriptor and ownership publication checks remain required.
- D80 grants no fighter exception to expired work. D79/D81 accounting and D84
  complete-payout deferral are unchanged. An existing signed payment recovers
  only its exact original bytes.

No new user policy question is needed for this scope. Late fighter grace,
reserved births beyond signed authority capacity, cap enlargement, pruning,
rollover, or migration are explicitly outside it.

The binding is the original concrete ManifestStore supplied by the operator,
not a caller-supplied job DTO. `PrepareManifestAdmission` loads that store's job,
checks gym/key/signature/proof and recovers its exact historical record.
`CommitManifestAdmission` rechecks the original store/pin/job/prepared state and
original session. Admission exposes no job issuance route. In Set80's selected
recoverable issuer, direct and legacy issue APIs refuse; request/job reverse
references and the source/issuer census are already audited by the original
constructors. These existing boundaries suffice for the original-issued-job
restriction; do not add a generic authority callback or a circular
`prospectservice` to `poolshare` dependency. Do not require a pool receipt or
invent a one-proof-per-job rule. A foreign, missing or substituted job remains a
refusal even when its DTO matches the public pin.

## Minimal vertical implementation

Preserve the old V1 runtime/constructors and existing direct drain API behavior.
Select the new behavior only through the already explicit DAILY operator
composition, including its recoverable variant. The first implementation should
change the following seams, adding a narrow private or explicitly selected
helper only where needed:

1. `desktop/pooloperator/capacity_drain.go` and its runtime state: distinguish
   pool-only drain from an independent hard admission/publication hold under
   the existing `intakeMu`. Keep the same original owners, bounded contention
   handling and joined transition semantics. A later hard hold must escalate
   and remain sticky even if the first reason was `pool_capacity`; the current
   first-reason-only latch cannot safely make this decision.
2. `desktop/pooloperator/construction.go` / `runtime.go`: after physical reopen
   at an already-full source, the admission server starts paused. Enable only
   eligible original-job delivery after the existing original public/round
   readiness prerequisites succeed, independently of the permanently drained
   mining branch. Merely omitting DrainOperatorIntake leaves this restart case
   paused forever. Do not re-enable a hard admission/publication hold.
3. `prospectservice/manifest_fenced_server.go` and the original registry staged
   admission seam: retain historical-first handling, then distinguish a
   never-admitted expired job using the original issuer clock/lifecycle and
   original proof/session checks. This selected preflight must not create a
   record, sign/publish a birth, or authorize a later commit. Final commit still
   samples expiry again. Never infer expiry from the client clock or turn a
   generic capacity/unavailable/mixed error into expired.
   Bind the decision to the exact original prepared owner, retain persisted
   clock high-water rules and the original session recheck, and preserve
   corruption, poison and failed-write precedence. A full public store or
   failed clock persistence must not become expired just because wall time
   elapsed. Historical receipt recovery remains before clock/node effects.
4. `prospectworker/pool_delivery.go` should need no behavioral widening. Its
   existing authenticated definite `expired` handling can terminate a pending
   never-admitted result. Keep historical receipt recovery and unknown-outcome
   retention unchanged; add integration assertions instead of a new retry loop.

No persisted schema, marker, signed pin, selected source/profile ID or network
response grammar change is expected. If implementation cannot retain these
properties, stop for a narrow root review before introducing a new format.

### Reason and readiness matrix

This is the proposed closed policy table, not generated product output. Only
the concrete `pool_capacity` branch is relaxed in this first slice; broader
reason classes require separate source evidence and root selection.

| Original state / reason | New pool issuance and ordinary intake | New fighter record for original job | Historical fighter receipt |
| --- | --- | --- | --- |
| Startup; original readiness incomplete | Paused | Paused | Original authenticated recovery |
| Ready, capacity available | Existing behavior | Existing full admission checks | Original authenticated recovery |
| Exact pool-only `pool_capacity`, both original readiness prerequisites met | Permanently drained | Allowed only while original job is valid and independent authority has room | Original authenticated recovery |
| Pool drain after reopen, readiness not yet established | Drained | Paused until original readiness | Original authenticated recovery |
| `public_capacity` or `admission_capacity`, including after an earlier pool drain | Drained | Hard hold; never re-enabled by later readiness | Original authenticated recovery |
| `round_capacity`, `no_reservation`, `legacy_over_bound`, unknown reason | Existing fail-closed behavior | No new relaxation in Set81 | Original authenticated recovery where already supported |
| Corruption, source mismatch, revoked session, cancellation or mixed fatal error | Existing refusal/join | Existing refusal; no soft-drain conversion | No bypass of the same original fences |

A signed authority hard cap can still prevent a valid live proof from being
admitted. Set81 promises completion only within remaining real authority space.
Never hide that limit by treating every retained authorization as reserved
admission capacity. Repeated or concurrent readiness/capacity updates must not
clear a prior hard hold or lose a newly observed hard reason.

## Three separate finite boundaries

1. **Source/job/request lifetime.** Source2 permits at most8,192 works and16,384
   receipts, subject to selected byte limits. Set80 MaxRequests equals the
   selected MaxWorks and covers terminal requests too. The worker has at most
   4,096 shared request/result slots and1GiB; it reserves the first result before
   dispatch. Renewals, parent changes, request terminals and actual work all
   consume finite capacity. Pausing new requests must not erase old work or q/e/h/p
   evidence. Set81 neither replenishes nor migrates these budgets.
2. **Signed admission/publication lifetime.** The original pin fixes1,000
   admissions,16,384 publications,4,096 carriers,1,024 checkpoints,256 cutoffs,
   65,536 headers and256MiB logical bytes. Set80 request/job holdback is additive
   to the existing unsigned-admission publication reserve; it does not reserve
   unlimited future births. Preserve the exact checked prewrite limits and
   mixed-error distinction. Increasing local storage cannot enlarge this pin.
3. **Expired never-admitted proof.** A valid original signature retained before
   expiry is not proof of prior admission. An original historical receipt wins
   unchanged; otherwise final expiry remains terminal with zero new birth.
   Capacity is not permission to keep trying a new birth after expiry.

## Acceptance matrix and existing independent inputs

Freeze a hand-derived state/error/count table before implementation. Reuse the
existing independent byte/proof inputs listed below; do not manufacture expected
results by running the new product code. Controlled node/session/clock fixtures
must be labeled as conditional component tests, never actual network/custody.

| Case | Required evidence |
| --- | --- |
| Valid original job, pool fills before fighter delivery | Real original source/issuer and independently derived ordinary limit; new pool intake refuses; still-valid prospect admits once through original session and node checks; birth retains original issued time |
| Prospect-only proof | No pool receipt required; independently qualifying prospect still follows full admission rules within original limits |
| Block-first proof | Original broadcast precedes signing/admission; only exact canonical winning block exception qualifies; side branch/stale ordinary proof refuses |
| Source already full on physical reopen | Original source/issuer/round/ledger/outbox reopen and audit; admission begins paused, then becomes available only after both original readiness observations; mining never reopens |
| Pool drain followed by hard public/admission capacity | Hard hold escalates despite original first reason; repeated readiness, concurrency and reopen cannot restore new admission; historical receipt remains recoverable |
| Missing/foreign job, wrong gym/key/proof, revoked/replaced session | Refuse without new record/publication; correct public DTO alone never grants original authority |
| Commit crosses original expiry | Strict final expiry with no new record; no new local grace or reset; preserve original clock rollback/refusal behavior |
| Never-admitted signed work after real expiry while drained | Authenticated definite expired response; worker terminal persists across reopen; zero fighter birth/publication, no replacement request or job |
| Already-admitted receipt after expiry and hard drain | Lost acknowledgment and physical process reopen return byte-identical original receipt/birth exactly once, without new node admission or signature |
| Capacity vs mixed integrity / canceled owner / failed close | Exact capacity alone may drain; mixed fatal stays fatal; all original gates/leases/processes join; cleanup failures retain original owners |
| Financial coexistence | Original pending payment body/raw/txid unchanged, no extra quote/sign/send during fighter recovery; maturity and actual scheduled payout remain correct; full D84 oversized case retains all earnings and consumes no slot |
| Cross-mode/profile | Existing V1 paused/drain/refusal tests remain exact; cheap profile cannot create/run selected owners |

Use `registry/testdata/manifest/reference/manifest_admission_v1.json` for inherited
manifest/proof/birth bytes; `prospectwork/testdata/pool_share_reference/pool_share_v1.json`
for original normal-PoW pool/prospect/block cases; and
`poolsettle/testdata/daily-admission-actual/reference-v1.json` (Set78,1d2b1462)
for the derived two-row capacity/economics fixture. Existing
`testdata/public-daily/reference-v1.json` (Set79,a5c65673),
`testdata/issue-recovery/reference-v1.json` (Set80,4bbe4eb9), and the
independent actual wrapper selection0f0890e5 provide the exact mode/config/
recipient/request bindings. Preserve their generators, qualification records
and original failure history. Before implementation, copy only allowlisted
public inputs and bind full hashes/lengths at the committed Set80 predecessor.

For actual acceptance, adapt the reviewed standalone operator/headless CLI,
normal-node, original schema16/schema14 wallet and public TLS relay mechanisms.
Use the same independently derived two-row profile. A bounded relay holds the
original `/v3/admissions` request until original pool drain is observed, while
leaving enough of the original five-minute window; it cannot rewrite work,
time, receipt, session or policy. Release to original service, lose one success
reply, then stop/join and reopen the same outbox/operator to recover exactly.
Do not exceed the original per-call deadline while waiting for pool drain; any
different existing maintenance-interval selection must be independently bound
before execution. The separate expired case retains an original signed pending
proof by stopping/joining its worker while an original admission call is held,
within that call's unchanged bound. Wait the remaining real job lifetime with
the worker stopped, then reopen the same owner and retry the exact proof. This
must verify zero birth plus durable expired completion; it must not hold a live
30-second HTTP request open for five minutes. No test clock,
shipping hook, live DB read or post-hoc acceptance relaxation. Capture original
closed rows, bounded public diagnostics, all child generations, exact response
identities and complete source/profile/cleanup evidence. Keep any scenario that
cannot finish its original window as a failed attempt rather than extending it.

Normal/race component suites should cover the same original-owner regressions;
cheap tests are refusal-only. Repeat affected operator/admission/worker and
financial suites, final static/build checks and exact-tip CI after source freeze.
Actual financial values must use existing independently derived scenarios; any
necessary new scenario arithmetic is frozen independently before product runs.

## Remaining milestone after this slice

Set78/79 already qualified finite DAILY late-winner recovery, public TLS/history,
original-wallet worker/headless composition and real UTC scheduled payouts.
Set81 does not reopen those as missing features and does not relabel fixture
proofs as a deployed public host. Set80 qualifications stay independently scoped.

Continuous retention still requires a reviewed explicit continuation design:
old published jobs can retain D80 canonical-late-winner rights after expiry;
global duplicate guards, fixed round membership, exact financial carry and
signed pending payments cannot be deleted/reset to regain capacity. Set80's
never-published issued_unused terminal is not a general retirement rule.
Sustained ordinary-share/job-refresh load, signed-authority continuation,
disk-full/recovery operations and selected host/production custody remain open.
A bounded exhaustion/drain pilot is distinct from a continuous public service.
Login/onboarding remains later; the joint D74 design review remains last before
release. This plan does not close broad launch readiness.

## Public input identities observed during planning

These are read-only existing corpus hashes, not new generated expectations.
Rebind their provenance to the final Set80 commit before implementation; do not
refresh or relabel them using product outputs. No private runtime files were read.

| Repository-relative public input | Bytes | SHA256 |
| --- | ---: | --- |
| `registry/testdata/manifest/reference/manifest_admission_v1.json` | 1207894 | `d32a8653c76c643addb3df091ca8d7dfdba66c2486f2ca305e718113f71d5794` |
| `prospectwork/testdata/pool_share_reference/pool_share_v1.json` | 33879 | `2210d992e618b7a0952736dede1263b678f1a7a2d55d14ccde1cc6b29317de87` |
| `poolsettle/testdata/daily-admission-actual/reference-v1.json` | 29980 | `1d2b1462296bd022c53df9dc583db3c38cd13a8e13e84f9983f2505b3aefb08f` |
| `testdata/public-daily/reference-v1.json` | 3176583 | `a5c65673442846249b500d0db05c80652b5350ed1c032a586d275265698d25e3` |
| `testdata/issue-recovery/reference-v1.json` | 6134980 | `4bbe4eb9556ed5a894cee6ff565b0dcca805f27fea2d77cfe3a56242dc3dac91` |
| `desktop/pooloperator/testdata/issue-recovery-actual/inputs/actual-reference.json` | 173644 | `0f0890e529b64c10ed000673aab7861b9bd526057cc3c2bbbe05b868028f3d10` |
| `desktop/pooloperator/testdata/issue-recovery-actual/inputs/path-c-reference.json` | 46498 | `a48a854d20298fced726b816d11690aa45e9f4184e3b161cbc7c258b74897386` |
