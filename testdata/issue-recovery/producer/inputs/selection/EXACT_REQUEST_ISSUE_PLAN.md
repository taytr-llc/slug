# Set80 proposal: recover one exact daily Issue request

Status: source-only design and acceptance plan for root review. No implementation,
reference output, execution, migration, deployment or readiness claim. Set79's
frozen sources and evidence are unchanged.

## Outcome and scope

A worker that loses an Issue response, or restarts after recording its local
intent, can resolve that **same request** through the original selected pool and
issuer. Resolution yields either the exact originally committed work or a durable
terminal result proving that this request never published pool work. It never
adopts the latest job, silently creates a replacement request, renews a job's
birth/expiry, clears an unidentified old hold, or changes share/payment policy.

This is the next finite functional slice. It is not history continuation, a
larger signed authority profile, real-host handoff, onboarding or a new UI flow.
The original source/round/ledger/backend and D79–D84 behavior remain intact.

## Current source evidence

* `prospectworker/pool_daily_issue.go`: `issue_pending=true` is synchronized before
  dispatch; the completion token is RAM-only. Only the same active attempt can
  store a validated scan or clear a definite no-effect result. `pool_worker.go`
  preserves the hold across restart while reconciling already retained deliveries.
* `poolservice/daily_server.go` and `daily_client.go`: Issue sends `{}`. Existing
  work recovery requires a job ID that a lost Issue response may not disclose.
* `poolshare/pool.go:137–228`: issuer commit precedes proposal checks and
  `saveWork`. The source explicitly permits an unused issuer job after failure.
* `registry/manifest_staged.go`: preparation is volatile; one synchronized commit
  assigns the original challenge/time/job and increments JobCount. There is no
  durable client-request mapping. A replayed preparation is not recovery.
* `poolshare/store.go:341–391`: `saveWork` refuses an expired job. Recovery cannot
  turn the issuer/source gap into a new birth or bypass this guard.
* The registry and source audit complete closed namespaces. Adding ignored keys
  or relaxing old decoders is not a compatible solution.

These facts rule out a client-only retry, an HTTP cache written after success,
and a request-to-job index that omits the original source template.

## Explicit selection, without economic repinning

Propose a fresh `RecoverableIssue1` storage/transport selection layered over the
exact selected Base Setup2, Source Config2, ManifestAuthorityPin5, public ownership
profile2 and daily settlement configuration. Freeze a small descriptor containing
`version=1`, the Base Setup2 ID and Source Config2 ID. Its separately domain-bound
ID is a correlation value, never signing, session or source authority. Base Setup2
does not contain this descriptor, so its digest has no cycle.

Use explicit `Create/OpenRecoverableDaily` source and operator constructors,
`NewRecoverableDailyServer/Client`, and corresponding worker/outbox constructors.
Names are proposed; the byte/API contract is the next freeze, before producer or
Go work. They return/retain the existing concrete owners, not substitute authority
interfaces. The normal `--daily` path and all current constructors stay strict.
The new operator flag/config wrapper must be explicit and mutually exclusive.

Fresh registry/source markers and metadata wrappers require this descriptor;
old opens must reject before use. Underlying ManifestJob2, public admission3,
work, receipt, round, credit, payout and schema16 guard bytes remain unchanged.
The selected registry wrapper may retain the existing inner Version5 metadata
and pin; it must not label new request namespaces as old Version5 storage.
Source metadata similarly retains its inner Config2 rather than inventing new
economic limits. New full audits recognize exactly the new namespaces. The
existing paired source/ledger binding still authenticates the same Base Config2;
the issuance selection is additionally checked by original runtime composition.

Select a distinct pool HTTP protocol/path and trust domain, proposed `/pool/v3`,
with the required issuance-profile ID in client configuration/trust and response
envelopes. This is unrelated to the existing admission-only HTTP v3. Preserve
source ID, settlement ID, recipient, gym and TLS checks on every response. The
old empty Issue frame must fail in this mode; old v1/v2 endpoints and outboxes
must reject the new frames. Do not let the old no-request Issue method issue on
a selected recoverable owner. Likewise, direct legacy `IssueManifestJob` and
legacy registry Prepare/Commit issuance must refuse that selected owner; only
the request-bound path can create its jobs.
Existing fenced admission and ownership publication retain their original jobs,
signature domains and concrete signer. No existing root is rewritten or adopted.

The first scope therefore needs no wallet migration, changed public signature
domain, altered signed limits or old history import. Any discovered need to
change those is a design finding to bring back before implementation.

## One request and three durable owners

Use a nonzero 32-byte random public request ID, generated by the product worker
before its first dispatch. It is neither a secret nor gym authentication. An
authenticated request is bound to the selected descriptor, original source,
public key/gym and exact money binding. Equality is over canonical full fields,
not the request ID alone. Never persist a bearer token or a process-local lease.

| Owner | Required new durable evidence |
|---|---|
| Original outbox | One active intent: request ID, complete immutable selection, and phase. Preserve it across cancellation, Stop, crash and reopen. Completed/terminal request records remain bounded and auditable. |
| Original source | Request record, immutable intent digest, exact owned template, authenticated gym/key and reserved row/byte capacity before issuer effects. A completed record refers to the exact existing `w/jobID` row; that row retains the template. |
| Original registry | Atomic request-to-job record binding the source intent digest and original owner/key, stored with the original job row and JobCount/clock update. A sealed no-job record is distinct from an issued job. |

The source and issuer request keys select the profile plus request ID; the row
binds the exact gym/key, so reuse by another gym refuses without disclosing the
original result. All inverse references/counts must be audited. Unknown, missing, extra, conflicting
or cross-owner rows are corruption, not absence. No persisted boolean substitutes
for proof of either journal. No template or original job is discarded by terminal
resolution. Finalizing an intent may replace its duplicated template with a
reference only when the identical template is durably retained in its work row.
Open performs full read/audit of legitimate intermediate states, not repair,
issuance or node I/O. Only a later authenticated Resume on the original opened
owners can advance them; read-only Lookup never changes them.

The concrete server derives the expected money binding from original configured
enrollment and rejects a supplied mismatch. The source trusts only its attached
profile/controller, never claimant-supplied configuration. The registry operation
uses an opaque preparation created by that original registry; parsing a request
or supplying a job/challenge never creates issuance authority.

## Commit and recovery protocol

1. **Local intent.** With no active scan, synchronize the outbox request and its
   space reservation. Only then release the mutex and call `ResumeIssue(id)`.
   Starting and resuming use this one idempotent command; a separate read-only
   `LookupIssue(id)` can observe it but an absent lookup never authorizes clearing
   the intent. A delayed original request may still arrive.
2. **Source admission.** Under the original source gate and current original
   Mining lease, look up this request first. For a new request, check exact
   configuration, membership, intake and all original capacity reservations;
   resolve/own/validate one original template, then synchronize its intent and
   byte reservation. An exact retry reuses it. It never refreshes that template.
3. **Issuer commit.** Resolve the exact registry request before acquiring entropy
   or sampling a birth clock. If absent, prepare outside the account lease,
   then atomically commit request mapping + original ManifestJob2 + metadata under
   the original session recheck. Existing mappings return the same original job
   without new entropy, time, job or counter increment. An exact no-job seal
   prevents any later delayed commit for that request. Mismatch is fatal.
4. **Source completion.** Reconstruct the original job/template; perform the
   existing complete preparation, proposal/current-context and expiry checks.
   Synchronize work + completed request + all indexes/accounting together under
   the original session fence. No response containing work precedes this commit.
   Reserve enough space at step 2 so completion is not defeated by another new
   request consuming its bytes. Only then disclose fully bound owned work.
5. **Local completion.** Validate the entire bound response and exact request ID.
   In one outbox sync, retain the exact scan/cursor and completed request and
   clear the active intent. A crash or write uncertainty reopens/audits that same
   owner; it cannot choose a new ID. Network and complete work validation never
   hold the outbox mutex.

**Issuer/source gap:** after a restart, the durable source intent plus exact
registry mapping supply the original job/template; do not issue a second job.
If source completion already committed, always replay it, even after expiry or
drain. Work lookup retains its current historical meaning; it is not fresh
eligibility or permission to sign an expired fighter admission.

If the original job expired under the original persisted clock rules before any
work commit, the selected coordinator may
durably close this request as `issued_unused`, carrying that exact job ID and
intent digest. It must first fully audit both owners and prove the source has no
completed work or completion index for this request. Since every work disclosure
follows the atomic source completion, this proves that this protocol did not
publish work. Retain the job/template/evidence and charge the original issuer
count. This preserves the existing unused-job behavior rather than making late
`saveWork` legal. A source work row always overrides any proposed unused result;
an inconsistent terminal row is corruption, never an excuse to hide it.

For a source intent with **no issuer job**, a positive trusted current-parent
mismatch may close it as `not_issued`: first atomically seal the original registry
request against delayed commits, then finalize the source result. A crash between
those commits resumes that exact seal. The implementation must obtain positive
context evidence through the original node adapter: for example, resolve and
validate its current template under the same policy, recheck that context, compare
its parent to the retained intent and discard this new template. Do not install
it in the request. A generic node error, Busy,
timeout, cancellation, storage absence in an unaudited owner, or a missing HTTP
response is not such evidence. If that narrow evidence seam is unavailable, this
case remains held until reviewed; do not silently replace its template.

Both terminal replies are required exact-state responses, not generic HTTP
`no effect` errors. Their durable outbox commit clears only this intent. A later
ordinary worker iteration may start a new request under normal admission; the
terminal result does not itself authorize it. Revocation or a dead original
session prevents commit/disclosure, while retained evidence remains recoverable
by a later legitimately authorized owner of the same selected gym. No automatic
session substitution occurs within an attempt.

## Budgets, uncertainty and lifecycle

* Derive request-count ceilings from the already selected MaxWorks, including
  sealed/unused requests; no unbounded tombstone namespace or new arbitrary cap.
  In the fresh selected journals, `Works <= Jobs <= Requests <= MaxWorks`;
  source intents can temporarily precede registry request records, which the
  cross-owner audit distinguishes from a missing record for completed work.
  The outbox additionally counts retained request evidence against its existing
  selected record/byte bounds; it cannot spend a second unaccounted record budget.
  Reserve worst-case request/index/template growth and the existing canonical
  receipt holdback before first effects. Include registry request bytes in the
  unchanged signed/public logical-byte audit. Freeze independent byte formulas
  before implementation; selected configurations that no longer fit refuse.
* Source request completion converts an existing reservation, never adds an
  unreserved late work slot. New requests obey intake/drain; exact completed
  lookup and terminal reconciliation remain available at capacity. A pending
  pre-issuer intent cannot smuggle fresh issuance past a new drain: either live
  admission still permits its first commit or the audited request remains held/
  is safely sealed with a specific terminal reason. No financial cap is bypassed.
* Existing 30-second operation deadlines remain. Replays are separately bounded
  operations on the same ID; no deadline extension or silent endless retry loop.
  Pure Busy may be retried only outside the account lease. Mixed failures retain
  their fatal/unknown meaning. A synchronized-intent/storage uncertainty never
  becomes a definite no-effect reply. No account lease or outbox mutex is held
  over network work. Preserve the existing bounded original source gate across
  its node calls; this slice does not invent a second in-flight serialization path.
* Original pool/issuer gates serialize duplicate requests and commits. Recheck
  context, selected owners, closing/poison state and original session at effects
  and disclosure. Close/Stop joins owned workers/calls before releasing original
  DBs, leases or credentials. Failed cleanup retains exact original owners.
* Existing ID-less Version2 holds remain held. No guessing from timestamps,
  latest jobs, equal templates, nonce searches or a replacement source/root.

## Small implementation boundary after approval

| Slice | Concrete existing seams / proposed additive leaves |
|---|---|
| Registry | `registry/manifest_jobs.go`, `manifest_staged.go`, `manifest_store.go`, `public_store.go`, `public_audit.go`, `operator_store.go`; additive selected issue-journal codec/commit/audit. Preserve old pin/signature codecs. |
| Source | `poolshare/pool.go`, `store.go`, daily admission/audit and binding; additive request codec, original-owner coordinator and full crosslink audit. Old non-request Issue is fenced on the new owner. |
| Transport | `poolservice/daily_{config,client,server}.go`, strict codec and `poolingress` selection; additive exact Resume/Lookup request/result frames and concrete selected constructors. |
| Worker | `prospectworker/pool_daily_issue.go`, outbox codec/audit, `pool_worker.go`; additive selected durable request states and original-attempt recovery. Existing delivery/nonce paths remain. |
| Composition | Explicit setup/provision/runtime/credential/remote-config wrappers plus headless selection in `desktop/pooloperator` and `desktop/pilot`; retain same concrete schema14/16 owners and existing signer purposes. No UI/onboarding change. |

Deliver these as one integrated selected path. A generic journal or server index
without worker and crash-gap consumption does not close the blocker. Round and
settlement algorithms should need only compatibility/binding regressions, not
economic changes. Exact file ownership is assigned after the byte contract.

## Independent contract and acceptance before claiming closure

Freeze ordered descriptor/marker/metadata/request/result bytes, domains, request
key framing, legal state transitions, counters and maximum byte formulas before
Go. Use the unchanged qualified Set79 public work/composition inputs; preserve
all old fixtures. A separate Python producer derives canonical bytes, IDs,
positive/negative state traces and bounds without importing product outputs.
Root alone reviews/admit/runs it in two existing interpreters. Conditional state
traces are not evidence of atomic writes, authentication, PoW or process recovery.

Required implementation acceptance:

1. Strict legacy/new cross-mode refusal at marker, metadata, setup, transport,
   credential and outbox boundaries; unchanged V1/V2 corpus bytes and old holds.
2. Crash/fault cuts after local intent but before dispatch; source intent;
   issuer commit before source save; work commit before response; response before
   local scan sync; and uncertain local/server sync. Fresh-process recovery must
   find the exact durable state with no second job or replaced template.
3. Concurrent same-ID calls, delayed original responses, replay after a newer
   completed operation, ID reuse with another gym/key/profile/money binding,
   and malformed/unknown result states. No cross-account disclosure or response
   from an obsolete worker clearing a current intent.
4. Exact original birth/expiry/challenge/header/work bytes and one registry job
   increment; source work increments zero or one as its terminal state requires.
   Completed work replays after expiry/drain. Unpublished expired jobs become
   retained issued-unused only through the audited terminal transition; source
   work/index mismatch fails instead. Not-issued seal races must never commit a
   job afterward. Transient node failure is not a stale-parent proof.
5. Missing/extra/inverse/mutated rows, count/byte overflow, source and issuer
   capacity independently, reservation converted once, unknown commit/reopen,
   cancellation/revocation at every fence, and original Close/cleanup failure.
   Exhaustion must not hide corruption or disable existing exact result lookup.
6. One bounded actual normal-node/TLS/headless-worker rehearsal: fault the Issue
   response after original commit, join and restart the actual client, recover
   the same work and produce a normal accepted result through existing paths.
   A separate fresh server process exercises the issuer/source gap. Reuse the
   existing independently qualified payout economics if a payout is included;
   do not derive expected amounts from Go. Assert all job/work/request counts,
   original outbox rows, no duplicate birth/credit/payment, and complete joins.
   Existing late-winner/transfer proofs remain regressions, not reopened projects.

Successful bounded candidate acceptance would close unidentified-response loss
for **new selected requests**. It would not prove continuous miner load, limitless
capacity, cold-audit scalability, production custody, hosted payouts or launch.

## Decisions

No new economic or user decision is required for this design within the existing
authorization to complete the public pool. Root must select the exact format and
terminal-state contract before implementation; that is an engineering review,
not a repeated user permission request. This plan does not authorize migration
of old held roots, request-history deletion, release of old liabilities, new
signed limits or live deployment. Any later real host/domain, custody/enrollment
or rollout choice is separate and requires concrete operator context, not keys
in chat. Follow AGENTS' public-pool-before-onboarding order and keep D74 last.
