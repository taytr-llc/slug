# Edge-case follow-ups

Updated November 21, 2026. Owner: Codex with Mike for product decisions.
[Return to the active burn list](../LAUNCH_READINESS.md#active-test-build-burn-list).

This is the open follow-up log, not extra work required before a safe test build.
Mike's triage rule is: fix now only if an issue blocks the agreed test flow,
risks funds or lost data, or breaks security or payout correctness. Otherwise
record it here and continue. Public-launch-only items must not delay a bounded
test build. If an item below crosses that rule during testing, promote it to an
active blocker with the observed evidence; do not bypass its safety hold.

Current order: hosted pool and scheduled payouts → wallet balances and history
→ sign-in/onboarding → full joint D74 design review. Marketplace is on hold.
Existing expiry, custody, recovery, accounting and D84 rules remain in force.
A test build does not imply public deployment, permanent genesis or release
approval. Do not log a resolved historical failure as a new open defect.

## EF-01 — Finite mining, issuer and client history

**Status:** Open; public-launch capacity work. Nonblocking for a test that stays
within a declared, monitored finite envelope.

- **Trigger:** Retained jobs, shares, request/delivery rows or logical bytes reach
  their selected lifetime allowance. Source2 allows at most 8,192 works and
  16,384 receipts; ordinary intake is derived at no more than 4,095. The DAILY
  miner outbox has a separate allowance of at most 4,096 records, shared with
  recoverable request history. A smaller selected budget may stop first.
- **Player impact:** New mining pauses even after old payouts complete. Restart
  does not free the allowance. Existing work, receipts and balances remain held.
- **Evidence:** [Capacity inventory](../POOL_CAPACITY_CONTINUATION.md),
  [daily admission contract](../POOL_DAILY_ADMISSION.md) and
  [issue recovery contract](../POOL_ISSUE_RECOVERY.md). Finite full-intake and
  late-winner recovery are qualified; indefinite continuation is not.
- **Workaround:** Size and monitor the bounded test before enrollment; visibly
  drain new intake before exhaustion while retaining recovery and payouts. Do
  not reset journals, discard old jobs or repin wallets to gain room.
- **Revisit:** Before increasing the test envelope or promising continuous
  public mining; immediately if the agreed test flow reaches a cap.

## EF-02 — Separate signed fighter/publication limits

**Status:** Open; public-launch capacity work.

- **Trigger:** The selected authority reaches its candidate admission,
  publication, header or byte limit, independently of pool share capacity.
  The current fixed tuple includes 1,000 admissions and 16,384 publications.
- **Player impact:** New fighter admission/publication can pause while coin
  accounting continues. A still-pending proof can expire under its original
  five-minute job; capacity cannot grant a late birth.
- **Evidence:** [Public ownership limits](../PUBLIC_FIGHTER_OWNERSHIP.md),
  [capacity inventory](../POOL_CAPACITY_CONTINUATION.md) and
  [retained admission behavior](../POOL_RETAINED_ADMISSION.md).
- **Workaround:** Keep the test population and proof volume inside the signed
  allowance; report hard holds. Preserve historical receipts and original
  expiry. Extra disk or a new directory is not a larger authority profile.
- **Revisit:** Before expanding fighter/publication volume or selecting an
  explicit continuation profile; promote immediately if the test cannot finish.

## EF-03 — Long-history startup, audits and load

**Status:** Open; public-launch scale qualification.

- **Trigger:** Retained proof/financial history grows, a cold process reopens it,
  or simultaneous miners increase issue/share traffic and contention.
- **Player impact:** Startup, synchronization and payment preparation can take
  longer or report a hold. A small successful rehearsal does not establish
  sustained service at a larger population.
- **Evidence:** [Set76 qualification](../CHANGESET_76_NOTES.md) records a
  142.534-second cold source audit in its larger finite proof.
  [Capacity continuation](../POOL_CAPACITY_CONTINUATION.md) separates that result
  from sustained miner/load and hosted acceptance. Historical payments are
  re-observed before another batch under [settlement](../POOL_SETTLEMENT.md).
- **Workaround:** Qualify and monitor the small hosted test envelope and retain
  original timeouts/complete validation. Pause intake if it outgrows that
  envelope. Do not trust a persisted “verified” flag or skip old financial data.
- **Revisit:** Before increasing host load or retained-history size. General
  segmentation/continuation design is deferred; a live test-flow stall is not.

## EF-04 — An oversized whole daily payout may remain deferred

**Status:** Open operational/presentation follow-up; the D84 policy is settled.
Public-launch load concern, not a reason to change the bounded test payout.

- **Trigger:** All eligible balances and their required funding cannot fit one
  permitted transaction or its selected input/claim/weight limits.
- **Player impact:** The entire payout is deferred with balances carried forward.
  Waiting alone may never make the complete transaction smaller.
- **Evidence:** [D84 whole-payout deferral](../POOL_CAPACITY_CONTINUATION.md#whole-payout-deferral)
  and [daily capacity](../POOL_DAILY_CAPACITY.md).
- **Workaround:** Retain exact balances and show `transaction-capacity`; sign and
  send nothing and consume no daily slot. No oldest-first subset, extra payout,
  miner surcharge or reserve advance is authorized. Keep the test batch within
  its verified shape.
- **Revisit:** During wallet balance/history presentation, and before public
  load expansion. If an otherwise valid in-envelope test payout fails, fix it
  as payout correctness rather than deferring it here.

## EF-05 — Fee, minimum and funding deferrals need clear player reporting

**Status:** Open presentation/operations follow-up; accounting rules are settled.

- **Trigger:** A miner remains below 1 SLUG; funding is immature/conflicted;
  the pool's retained 1% cannot cover the fee; or the supported change would be
  dust. Earnings can be confirmed at sixty descendants before coinbase maturity.
- **Player impact:** Confirmed earnings do not necessarily produce a payment on
  the next run. The balance should explain the particular hold instead of
  appearing to lose money or silently skip payment.
- **Evidence:** [Settlement reasons and exact carry](../POOL_SETTLEMENT.md) and
  [payout history](../POOL_PAYOUT_HISTORY.md). These are existing guard paths,
  not evidence that an incorrect payment has occurred.
- **Workaround:** Use authenticated balance/history reports with their dated
  reasons. Carry fractions, wait for spendable funding, and defer rather than
  charging miners more or advancing pool reserves.
- **Revisit:** The wallet balances/history task immediately after hosted payouts;
  also before broader enrollment. Incorrect amounts, missing carry or a false
  payment status are immediate blockers.

## EF-06 — Sparse candidate cards leave fighters unmatched

**Status:** Open general-league implementation/qualification; nonblocking for
the scoped test card.

- **Trigger:** Too few opposing gyms or same-class, zero-record fighters satisfy
  the existing half-to-double rating band.
- **Player impact:** Some entrants receive no bout. General multi-gym matching
  and the exact record-filter, widening and priority mechanics remain
  unqualified; D28's next-week priority and D52's widening requirement stand.
- **Evidence:** [Candidate-card exception C1](../CANDIDATE_CARD_EXCEPTION.md) and
  [the D28/D52 rules](../BRAWLCHAIN_SPEC.md).
- **Workaround:** Use only the approved two-gym candidate exception, which waives
  the widen-to-five trigger. Report entered/paired/unmatched counts and each
  reason; do not widen the rating band or silently create production policy.
- **Revisit:** Full joint D74 review with actual unmatched-card evidence, before
  a production league uses broader matching.

## EF-07 — Display-name collisions and disambiguation

**Status:** Open design choice; nonblocking while stable fighter IDs are retained.

- **Trigger:** Two fighters receive the same display name, including under the
  expanded candidate tables as the population grows.
- **Player impact:** Names alone may make fighter selection or comparison
  ambiguous. A duplicate name is permitted by D51 and is not duplicate identity.
- **Evidence:** [Name diversity and reproducible collision scenarios](../NAME_DIVERSITY.md)
  and [D74 review preparation](../LAUNCH_READINESS.md#review-session-preparation).
- **Workaround:** Use immutable fighter IDs and existing distinguishing details;
  preserve assigned names and manifests. Never rename historical fighters to
  make a collision disappear.
- **Revisit:** D74, with the intended population and actual selection screens.
  Any ambiguity that could send a different fighter is an immediate test-flow
  or data/correctness issue, not a deferred cosmetic one.

## EF-08 — Portrait rendering/accessibility review remains pending

**Status:** Open visual/accessibility check for D74; no established rendering bug.

- **Trigger:** Candidate portraits are viewed in the target desktop/browser and
  assistive-technology environment rather than only through deterministic tests.
- **Player impact:** Rendering or accessible descriptions have not received the
  complete intended human review.
- **Evidence:** [Portrait contract](../FIGHTER_PORTRAITS.md),
  [Set34 notes](../CHANGESET_34_NOTES.md), and the readiness record: browser policy
  blocked the local gallery preview and Mike explicitly kept the check pending.
- **Workaround:** Keep the existing qualified renderer and approved logos; retain
  the gallery for the final review. Do not claim the blocked preview passed or
  redesign portraits as an incidental task.
- **Revisit:** D74 on the working build; promote earlier only if rendering prevents
  completion of an agreed test flow.

## EF-09 — Matchmaking/gameplay findings need a design decision, not a rating patch

**Status:** Open gameplay review; D65 remains unchanged.

- **Trigger:** Production-like matching revisits the studied initial-band edges
  or draw frequency on the Set42 engine.
- **Player impact:** The study found very strong favorites at the tested band
  edges and fewer draws than the earlier simulator. These findings do not prove
  the incumbent rating formula is broken.
- **Evidence:** [Rating validation report](../RATING_VALIDATION_REPORT.md),
  [D75 acceptance policy](../RATING_VALIDATION_POLICY.md), and the
  [review topics](../LAUNCH_READINESS.md#review-session-preparation).
- **Workaround:** Preserve D65 and the current engine; label preview/provisional
  results correctly. Do not change weights, matchmaking rules or fight balance
  to resolve an incidental observation.
- **Revisit:** D74 with the existing studies. A future rating change still needs
  the agreed population coverage and paired improvement interval excluding zero.

## EF-10 — Wallet activity/history is a dated capture until refreshed

**Status:** Open refresh/presentation follow-up for the next wallet task.

- **Trigger:** Transactions, blocks or wallet state change after an activity or
  history capture completes.
- **Player impact:** The captured list can lag current activity until explicit
  Refresh. It is dated evidence, not proof that no newer transaction exists.
  This does not mean the separately polled wallet balance is permanently frozen.
- **Evidence:** [Wallet activity capture rules](../WALLET_ACTIVITY.md) and
  [payout desktop observations](../POOL_PAYOUT_DESKTOP.md). Set73 separately
  records the five-second wallet-balance polling cadence.
- **Workaround:** Use Refresh; retain the previous capture visibly dated if it
  fails, and invalidate old detail handles after successful replacement.
- **Revisit:** Wallet balances/history immediately after hosted payouts, then
  D74 for refresh ergonomics. Wrong amounts, stale data labeled current or an
  unauthorized disclosure are immediate correctness/security blockers.

## EF-11 — Wider distribution and platform qualification

**Status:** Open; public-distribution work outside the current reviewed Mac test.

- **Trigger:** Installing beyond the qualified Mac environment, using older
  macOS/Windows, or distributing through a signed/notarized update channel.
- **Player impact:** Installation and runtime behavior on those targets are not
  yet qualified. The current ad-hoc-signed test build is not a notarized release.
- **Evidence:** [Readiness evidence](../LAUNCH_READINESS.md#current-evidence),
  [Set65 package limits](../CHANGESET_65_NOTES.md) and
  [desktop workstream](../BUILD_PLAN.md).
- **Workaround:** Use the reviewed build/platform and preserve explicit support
  limits. Do not claim older-OS or Windows acceptance or recommend disabling
  operating-system security protections.
- **Revisit:** Target-platform and distribution/update qualification before
  public release. Failure on the agreed test platform must be fixed now.

## EF-12 — Extended offline catch-up exceeded the restart readiness window

**Status:** Open observed availability issue and rehearsal gap; nonblocking for
the smaller qualified restart flow. Exact cause is not established.

- **Trigger:** In Set81 B1, the chain advanced 100 blocks while the operator was
  stopped. After reopening the same full source, publication replay made partial
  progress but remained unready through the original 90-second readiness gate.
- **Player impact:** Returning after an extended outage can leave mining or
  public fighter views waiting for catch-up. The rehearsal stopped before its
  expired-proof retry and final wallet audit, so neither is qualified by B1.
  No funds, security or data-loss incident surfaced; incomplete verification is
  not proof that every financial outcome was correct.
- **Evidence:** [Failed B1 disposition](evidence/set81/actual/B1-failed/disposition.json)
  and [original report](evidence/set81/actual/B1-failed/report.json).
  Rounds were ready, publication was explicitly unready, no poison was reported,
  and all owners joined. One scheduled send was observed but is not a qualified
  B payout result. [A2 passed](evidence/set81/actual/A2-qualified/execution/report.json)
  the smaller restart, lost-reply recovery and actual scheduled-payout flow.
- **Workaround:** Keep the operator active while maturity blocks accrue in the
  bounded hosted test, and use A2's smaller restart scope. Preserve visible holds,
  original expiry, full histories and financial checks. Do not reset state,
  extend deadlines or relabel B1 as passed.
- **Revisit:** Before qualifying extended outages or increasing public scale;
  immediately if this prevents the agreed hosted test flow or reveals a funds,
  data, security or payout-correctness risk.

## EF-13 — Established transport pins have no in-place rotation path

**Status:** Open operations follow-up; nonblocking with an unchanged endpoint
and a certificate valid throughout the bounded test window.

- **Trigger:** The pool needs a different public origin, TLS key or client CA,
  or its current leaf certificate approaches expiry.
- **Player impact:** An expired certificate prevents new TLS connections.
  Changing immutable transport identities cannot currently preserve established
  operator/attachment roots through a supported rotation command. Miner
  credential rotation does not rotate those transport pins.
- **Evidence:** [TLS selection checks](../poolingress/config.go),
  [operator marker equality](../desktop/pooloperator/filesystem_unix.go),
  [attachment identity](../desktop/pilot/remote_pool_config.go), and
  [provisioning certificate binding](../desktop/pooloperator/provision_codec.go).
  Source permits a same-key, same-host leaf under the existing client CA after
  complete stop/reopen, but that renewal flow has not been rehearsed. Offline
  provisioning resume/verify retains the original certificate bytes and is not
  a renewal command.
- **Workaround:** Keep the selected endpoint/key/CA and ensure certificate
  validity covers the test. Monitor expiry; preserve original provisioning
  inputs and journals. Do not replace selected files under a running owner,
  reset roots, disable TLS verification or repin clients to force recovery.
- **Revisit:** Before certificate renewal or an endpoint/CA/key change, and
  before continuous public operation. Promote immediately if this blocks the
  agreed test or requires responding to a compromised key.

## Adding or closing an entry

Use a stable EF number, open/closed status and scope label. Record **trigger,
player impact, evidence, workaround and revisit point**. Link observed reports
or source contracts; distinguish an observed failure from a hypothesis or an
intentional hold. Append new nonblocking findings here rather than expanding
an active change set. Close only with a linked fix/decision and verification;
preserve the original observation and explain any remaining limitation.

## EF-14 — Local Linux cross-compiler source is incomplete

**Status:** Open engineering portability issue; native Linux builds remain the
qualified path and this does not block the hosted test.

- **Trigger:** Cross-compiling the new host-only fixture helper from this Mac's
  Homebrew Go1.27.4 installation to Linux/amd64.
- **Player impact:** No observed runtime impact; that local cross-build cannot
  produce a Linux artifact. It does not invalidate the native Linux CI results.
- **Evidence:** Set82's retained local cross-build attempt reports missing
  standard-library source packages `internal/runtime/cgroup`,
  `internal/runtime/syscall/linux`, and vendored `golang.org/x/sys/cpu`. The
  native Linux image job and all seven exact-tip CI jobs pass at `2d79de95`; see
  [Set82 notes](../CHANGESET_82_NOTES.md) and
  [CI completion](evidence/set82/ci.json).
- **Workaround:** Build host artifacts natively in the already pinned Linux
  compiler image and verify their source, compiler settings and hashes. Do not
  change the installed compiler or substitute an unqualified profile.
- **Revisit:** When local Linux cross-builds are needed for distribution or
  developer portability. A failure of the selected native host build is a
  separate immediate test blocker.

## EF-15 — Final scratch PID1/provider shutdown remains unqualified

**Status:** Open provider/distribution qualification; nonblocking for the
explicit bounded admin-image test with its independently observed original
operator child. That test must still obtain all original cleanup results.

- **Trigger:** Moving from the disposable admin-image lifecycle witness to the
  final scratch image, where the original operator is PID1 and the provider
  controls its stop/restart.
- **Player impact:** A provider stopped/removed state or SSH disconnect alone
  does not establish that the original operator, wallet and file owners joined
  cleanly. This is a qualification gap, not an observed lost payout or data loss.
- **Evidence:** [Hosted shutdown scope](evidence/set82/hosted-attempt1/source/HOST_SHUTDOWN_EVIDENCE.md),
  [fixture lifecycle witness](evidence/set82/hosted-attempt1/source/lifecycle/README.md),
  and [failed attempt selection](evidence/set82/hosted-attempt1/actual/execution-selection.json).
  The original hosted attempt failed at init before any serve generation, so it
  supplies no scratch PID1 shutdown result.
- **Workaround:** For the bounded test, run the unchanged selected operator as
  UID10001 under the reviewed fixture-only witness; retain its actual child wait,
  monitor join, unchanged selection and complete stopped-owner audits. Do not
  infer cleanup from deployment state, remove uncertain locks, or relax payment
  checks. Final scratch provider supervision remains a separate claim.
- **Revisit:** Before using the final scratch deployment for unattended/public
  service; immediately if the selected bounded test cannot prove its original
  owners joined or exposes a funds, data, security or payout-correctness risk.

## EF-16 — An upload TLS failure needs deployment-state reconciliation

**Status:** Open recurrence/diagnostic follow-up. One observed failed upload was
reconciled; no general upload reliability defect or ongoing failure is proven.

- **Trigger:** A provider upload ends with the observed fatal TLS
  `BadRecordMac` error after `Indexing`/`Uploading`, leaving the CLI result alone
  insufficient to decide whether a deployment was created or is still active.
- **Player impact:** Setup can be delayed, and an immediate blind retry could
  leave the operator uncertain which deployment/image is selected. No mining or
  payout corruption was observed in this upload failure.
- **Evidence:** [Attempt 4 upload log](evidence/set82/hosted-attempt1/builds/operator-host-build-attempt4/build.log)
  and [saved provider FAILED reconfirmation](evidence/set82/hosted-attempt1/builds/operator-host-build-attempt4/provider-failure-reconciliation.json).
  Root reported its original FAILED check before retry; the saved connector
  read reconfirms that state later and must not be presented as a pre-retry
  timestamp. [Attempt 5](evidence/set82/hosted-attempt1/builds/operator-host-build-attempt5/build.log)
  is a separate successful build/deployment record.
- **Workaround:** Preserve the failed log, reconcile the exact original
  project/service/environment/deployment through the provider, and retry only
  after its state is understood. Keep the exact image/context and protected
  volume selection. Do not disable TLS verification or auto-reset roots.
- **Revisit:** If this recurs or prevents the agreed hosted test. Repeated or
  uncertain live-owner deployments become an active test blocker; a one-off
  reconciled upload does not require unrelated product work.

## EF-17 — Cleanup cancellation can obscure the primary startup failure

**Status:** Open diagnostic follow-up; not a payment-correctness change or a
blocker once the concrete startup configuration is corrected.

- **Trigger:** An operator startup task fails and `Runtime.Run` cancels its
  siblings during joined cleanup. The resulting error tree contains both the
  original failure and `context.Canceled`; the public command classifier reports
  `cancelled` before generic unavailable/integrity failures.
- **Player impact:** An operator may investigate a signal or user cancellation
  when the actual cause is a listener/configuration failure. No mining or payout
  occurred in the observed case; original child and monitor waits were retained.
- **Evidence:** Set82 hosted attempt3 emitted `serve/cancelled` without an
  external-signal marker. Its original owner records premature exit1 and joined
  monitor0. An independent Go probe on that host confirmed generic TCP binding
  of `0.0.0.0:8443` reports `[::]:8443`, failing the existing exact listener check;
  explicit `[::]:8443` reports the selected literal. See
  [Set82 hosted notes](../CHANGESET_82_NOTES.md),
  `desktop/pooloperator/runtime.go` and `desktop/pooloperator/command.go`.
- **Workaround:** Preserve original lifecycle evidence, inspect the concrete
  startup selection, and verify the suspected boundary independently. Select
  the explicit bound address on a fresh fixture. Do not print private error
  trees, relabel failed runs, weaken address matching or reset existing roots.
- **Revisit:** When improving operator diagnostics or before public operations
  need actionable startup alerts. Promote immediately if ambiguity prevents
  safe recovery of an active owner or a payout.

## EF-18 — Full-pool worker banner remains outcome_unknown

- **Trigger:** The selected recoverable worker completes both accepted pool rewards, then synchronizes its next local request and calls Issue after the original source reaches its derived intake maximum. The source returns capacity before a new source intent; the service conservatively exposes non-Busy Issue errors as outcome_unknown.
- **Player impact:** The mining status can show retrying/outcome_unknown even though prior records are delivered and the pool is correctly drained. It can confuse the player or an auxiliary monitor; it does not mean the accepted rewards or payout were lost.
- **Evidence:** `docs/evidence/set82/hosted-attempt5-financial-proof/client-evidence/miner-1/process-report.json` reports 2 records/0 pending and clean requested exit0; `operator-01-closed/stdout.jsonl` independently reports draining/pool_capacity, ordinary maximum 2, submissions 2 and rounds 2; `proof-replay-before.json` preserves two exact accepted rows, both fighter receipts, and the third retained null request. Source: `poolshare/operator_admission.go:operatorIntake`, `poolservice/daily_server.go` Issue failure classification, `prospectworker/pool_issue_run.go` same-request retry and `pool_worker.go` status projection. `review.json` pins the named completed public evidence.
- **Workaround:** Stop and join after actual delivered records and original operator full-intake status, then verify complete closed outbox/source evidence. Keep the exact active request retained; do not clear it, start a replacement request, require an invented CLI capacity code, or relabel uncertainty as a definite no-effect response. The auxiliary overly strict status watcher remains a recorded failed observation, separate from the selected protocol proof.
- **Revisit:** Improve the explanatory status once a trustworthy capacity signal can be correlated with the original request/owner without weakening conservative Issue semantics. This banner refinement is nonblocking for the bounded hosted payout test; financial correctness, exact replay and all owner cleanup remain mandatory.

## EF-19 — An existing Go cache can hide installed desktop packages

**Status:** Open build-environment follow-up; nonblocking with a fresh cache.

- **Trigger:** Building the desktop with the previously reused Go cache reports
  that Wails application/events packages are unavailable despite their pinned
  module and source files being installed.
- **Player impact:** A developer build can fail before compilation. No wallet,
  payout or player-data failure was observed.
- **Evidence:** Set83's [package lookup diagnostics](evidence/set83/wallet-integration/checks/wails-package-diagnostic.json)
  and [fresh-cache lookup](evidence/set83/wallet-integration/checks/wails-fresh-cache-diagnostic.json).
  Disabling the module index also restored lookup in a diagnostic. This isolates
  the cache/index boundary; the original cause of the stale entry is unproven.
- **Workaround:** Select a new build cache, retaining the failed cache and logs.
  The unchanged Go 1.27.4 no-CSE profile then passed tests, vet and both normal
  desktop/helper builds; see the [component report](evidence/set83/wallet-integration/report.json).
- **Revisit:** If it recurs, or during build-toolchain qualification before
  distribution. Do not alter product dependencies or compiler settings merely
  to repair this cache.
