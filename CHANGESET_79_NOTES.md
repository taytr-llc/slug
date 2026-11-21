# Change set 79: explicit public DAILY integration

Set79 implements the explicit daily public surface over the finite Source2,
daily round/ledger/wallet/scheduler and admission work from Sets76–78. It adds
Policy2/Setup2 composition, daily miner credentials, selected TLS ingress and
HTTP clients, durable daily workers/outboxes, and a direct existing-wallet
headless launcher. V1 APIs, defaults, formats and roots remain selected as V1.
There is no implicit migration, new signing authority or payout-policy change.

See [the implemented surface and remaining gates](POOL_PUBLIC_DAILY.md).

## Implemented behavior

- `--daily` explicitly selects provisioning and operator commands. Construction
  verifies the original daily source/round/ledger/wallet/scheduler bindings,
  derives the complete finite liability budget and attaches paused intake before
  exposing services. The ordinary bound is not a caller-selected quota.
- `/pool/v2/` successful work, receipt and money responses bind source,
  settlement configuration and native recipient. Daily trust and credential
  domains are separate; original pool/admission audiences and session fences
  remain. Embedded jobs/submissions and admission HTTP v3 are unchanged.
- Exact daily money reads use nonnegative int64 decimal strings for cumulative
  paid/reserved totals and reduced fractions for earnings. Payout pages scan
  eight indexed batch slots under one captured revision, preserve full balance
  totals and reject stale continuation. Empty pages are not silently treated
  as complete history.
- Daily Issue uncertainty is durable in the original outbox before dispatch.
  Scan publication and hold-clear are atomic under the original attempt;
  unknown results cannot cause replacement Issue on loop or restart. Original
  pending receipt/fighter recovery remains available. A pre-dispatch crash can
  leave an unresolved hold because Issue has no request-identity recovery API.
- The explicit headless command retains the original local schema14 session,
  recipient, Mining lease, recovery/vault authority and outbox. It opens no
  replacement wallet or node, exposes no key getter and introduces no helper
  wire or native DAILY UI. Cleanup uncertainty retains original owners.
- D84 still defers the whole ready transaction at capacity. Original reserved
  payout bytes recover first; no subset, surcharge, advance or new daily slot
  is authorized by intake or read operations.
- DAILY scheduler cycles can refresh later confirmations after the day's
  original payment has completed and its cursor is occupied. The original
  ledger fully audits first, then checks the durable admitted day against its
  own clock, the occupied original slot and absence of a pending batch before
  backend or credit effects. Refresh can update credits/balances but cannot
  quote, sign, broadcast or open another daily slot. Expired admission is a
  retryable no-effect refusal. V1 due decisions and all stored codecs remain
  unchanged; original pending recovery still precedes refresh.
- Intake coordination now retains the original readiness and capacity cause
  across exact source Busy only, under one 30-second budget and cancellable
  10-millisecond waits. It joins the original intake mutex without renewing the
  budget; cancellation, expiry and mixed fatal errors stop the transition. It
  never clears a drain cause, changes a payout or grants replacement authority.

- A subsequently installed DAILY-only admission change queues on the original
  ledger gate and retries only pure initial-audit/read-only source Busy within
  the same invocation. A post-sign source read cannot restart quoting, signing
  or broadcast. The first contention starts one shared 30-second waiting budget
  under the original caller; mixed fatal/canceled/expired results still refuse.
  V1 and public read gate behavior remain unchanged. This change is installed
  and passes focused plus full affected-suite qualification and complete public
  Paths A, B and C. Final builds/static checks and all seven exact-tip CI jobs pass.

## Qualification snapshot — 2026-11-21

- Independent public DAILY corpus: **PASS**, reproduced by Python 3.14.3 and
  3.9.6. The selected serialization successor is 3,176,583 bytes, SHA256
  `a5c65673442846249b500d0db05c80652b5350ed1c032a586d275265698d25e3`.
  Portable inputs, producer and qualification are under
  [testdata/public-daily](testdata/public-daily/).
- Root core/history checks: **PASS** for the communicated normal/race and cheap
  refusal scope, including original daily indexed payout capture and the
  stronger sparse 18-batch traversal. These are finite component checks.
- Transport/miner/ingress normal/race and selected cheap refusals: **PASS** as
  reported by the root qualification lane. Existing V1 suites remain included.
- Worker normal/race and selected cheap refusals: **PASS**, with unchanged
  selected sources. Two earlier dependency-cache failures remain preserved;
  they did not require a product change.
- Desktop pilot/account controller, provisioning/runtime and headless command
  normal/race and selected cheap refusals: **PASS** after the named public
  fixture correction described below. These are component checks, not actual
  worker/payment or headless-process acceptance.
- Full accounting/source/scheduler suites: **PASS** normally (79.175 seconds)
  and under race (276.489 seconds), plus selected cheap refusals. These runs
  precede the narrow completed-day refresh change.
- Completed-day accounting refresh: **PASS** for all six required focused tests
  normally (4.038 seconds) and under race (8.791 seconds), plus the explicit
  profile refusal (2.318 seconds). Selected sources stayed unchanged and all
  command groups joined. A subsequent complete settlement/scheduler run also
  passed normally (39.602 seconds), under race (324.463 seconds) and selected
  cheap refusals (2.407 seconds), with unchanged sources and clean joins. Those
  runs precede the exact source/round contention successor.
- Exact original source/round contention: **PASS** for all three required
  normal (7.482 seconds) and race (15.676 seconds) tests, plus two pure cheap
  controls (1.839 seconds). The real original-round-gate regression retained A
  and imported later B without another payment. These are focused fixture
  checks; the later complete Path B actual also passes.
- Full accounting after source/round contention: **PASS**, normally (66.393
  seconds), under race (299.936 seconds) and selected cheap refusals (3.920
  seconds). All nine required refresh/contention controls passed in both full
  normal/race runs, with unchanged sources and clean joins. This covers
  `poolshare`, `poolround`, `poolsettle` and `poolschedule`, before the subsequent
  desktop intake retry; [the portable record](docs/evidence/set79/accounting-after-contention.json)
  binds the exact reports and logs.
- Original-source intake contention: **PASS** for four required focused tests
  normally (7.471 seconds) and under race (16.276 seconds), plus the two pure
  cheap controls (5.301 seconds). The original issuer gate remains held during
  repeated census Busy and cancellation; the same capacity cause survives,
  both controllers drain without re-enable, and the entire pending A ledger,
  exact signed bytes and backend counters remain unchanged. These are fixture
  controls, not the complete public actual or headless shutdown acceptance.
- DAILY CLI cancellation provenance: **PASS**, normally (21.262 seconds),
  under race (26.494 seconds) and in the selected cheap scope (19.506 seconds).
  All 15 required normal/race tests and 13 cheap controls passed with unchanged
  sources and joined groups. These cover original account/Mining/file-owner
  cancellation, retained independent failures and real pipe framing/interrupts.
  They are component proof; the separate complete attached headless-process
  actual also passes as recorded below.
- Path A's independent target/config/economic fixture: **PASS** with identical
  46,498-byte outputs from both interpreters, SHA256
  `a48a854d20298fced726b816d11690aa45e9f4184e3b161cbc7c258b74897386`.
  This is reference qualification only; the separately qualified complete Path A2
  actual is recorded below, with its first failed attempt preserved.
- Set79 normal-node smoke: **PASS**. Two isolated blocks check the 10-SLUG
  reward and report-only checkpoint conflict; read-only public synchronization
  reaches height 61/60 checkpoint descendants, followed by joined restart. No
  public-chain mining occurred.
- Pre-admission-contention paired closeout: **PASS** for all 17 module builds,
  accounting vet (6.239 seconds), full operator normal/race (58.726/397.767),
  mininglease normal/race (0.796/2.025), selected remote desktop normal/race
  (23.925/60.600), desktop/operator vet (4.124), gofmt and diff checks. All 7,599
  bound inputs and the profile stayed unchanged; every command group joined.
  Earlier discovery failures 1 and 2 remain preserved. This third closeout
  predates the installed DAILY admission-contention change and is not its final
  exact-source qualification.
- New DAILY admission-contention focused qualification: **PASS**, normal
  7.815 seconds, race 21.084 seconds and selected cheap controls 2.711 seconds.
  All six required normal/race tests and both cheap controls passed without
  skips, with unchanged sources and joined command groups. These exercise
  original queued ledger admission against recurring reads, bounded cancellation
  and Close, post-sign source contention without a repeated quote/sign/send,
  and an initially empty scheduler cursor importing B once with A unchanged.
  The subsequent complete affected suites also pass as recorded below;
  the later actual and final static passes are recorded below; exact-tip CI remains a gate.
- Full accounting/operator regression after DAILY admission contention: **PASS**,
  core normal/race 73.165/301.935 seconds, operator normal/race 57.263/364.430
  seconds, and selected cheap controls 6.004 seconds. All six admission controls
  actually passed in both full normal/race runs, sources remained unchanged and
  every command group joined. This complements the earlier 17-module/desktop
  closeout; it is not complete public actual or exact-tip CI acceptance.
- Public DAILY normal-node/TLS/worker/real-UTC payout acceptance: **PASS,
  Path A attempt 2**. The outer actual command took 345.672 seconds; the Go
  scenario recorded 345.011106042 seconds. Three original vault authorizations
  produced three accepted fighters with zero pending delivery rows. Two pool
  rewards produced one scheduled payment of 1,980,000,000 atoms (19.8 SLUG),
  with a 209-atom fee and one send. The exact original-wallet increase, stopped
  reopen, authenticated money/history and final closed audit all pass. All
  3,079 selected sources remain unchanged, as do the original 268-file fixture
  catalog and 226 immutable copied paths. The retained outbox still has
  `issue_pending=true`; that uncertainty is not erased by zero pending deliveries.
  [Portable public evidence](docs/evidence/set79/attachment-actual.json) binds
  all 14 checker-selected public files and source/profile/fixture-hash metadata.
  The first failed attempt remains preserved; this direct controller proof
  does not qualify the separate headless command process.
- Public DAILY full-intake, real-expiry, fresh-process late-winner and unchanged
  reserved-payout acceptance: **PASS, Path B attempt 5**. The outer actual command
  took 673.947 seconds; the Go scenario recorded 673.486473334 seconds. The
  derived ordinary intake of two becomes three accepted receipts after real
  expiry. Two fresh production DAILY operator lifetimes retain the exact late
  receipt and original A body. B remains uncredited at 59 descendants and gains
  exactly one credit at 60, at tip 163, while its coin remains immature. A is the
  sole signed batch and is broadcast once; zero fighter births are recorded.
  All original owners, nodes and groups joined, and all 2,513 bound source files,
  copied inputs and normal-profile artifacts stayed unchanged. The complete
  [portable public evidence](docs/evidence/set79/public-actual.json) binds all
  11 checker-selected phase/report files plus source/build/lock metadata.
  Failed Path B attempts 1–4 retain their original statuses and evidence.
- Headless DAILY command process/cancellation acceptance: **PASS, Path C
  attempt 1**. The outer command took 290.406 seconds; the Go scenario recorded
  289.811943875 seconds. The actual normal CGO1 `slugpilot --daily` child retained
  its original PID/process group, received one SIGTERM, exited 0 and joined both
  output streams without forced kill. All 26 status records are complete
  canonical frames; stderr is empty. Ten original signatures and ten accepted
  fighters, with zero pending deliveries, produced two pool rewards and one
  19.8-SLUG wallet payment with a 209-atom fee. Stopped reopen and the full closed
  audit pass; `issue_pending=true` remains retained. All 3,111 sources, the
  original 268-file catalog and 226 immutable copied paths stay unchanged.
  [The portable proof](docs/evidence/set79/headless-actual.json) binds all 16
  checker-selected public phases/logs plus exact source/build/lock metadata.
- Final 17-module/static closeout: **PASS, attempt 2**. All 17 module builds,
  accounting vet (0.586 seconds), operator/desktop vet (3.272 seconds), formatting
  and diff checks pass with all 7,696 bound inputs/profile unchanged and every
  group joined. Attempt 1's 17 build successes followed by a test `lostcancel`
  vet failure remain preserved. The exact test-only successor adds two
  `defer cancel()` calls; its original contention test also passes normally
  (3.164 seconds) and under race (18.395 seconds), without skips.
  [Source-delta evidence](docs/evidence/set79/actual-test-only-source-successor.json)
  links the captured normal dependency selection to the unchanged actual reports:
  their full inventories still bind test hash `285f41c4…`, while the final test
  is `7be0587f…`. That file is in `TestGoFiles`, not compiled normal
  `GoFiles`/`CgoFiles`; no actual rerun or production change is claimed.
- Set79 exact-tip chain/wallet CI and downloaded Linux evidence: **PASS** at
  chain `fdd40d8edd9a685613f1f667f0074e56ec4e7da5` and wallet
  `dfc5d9dd7e0f604486fbac812d0413fce1f45cd6`. All six Build and Test jobs
  (run `44895338644`) and operator-linux (run `44895338636`) succeeded.
  The downloaded Linux report records 339 required passing assertions,
  1,260.502 seconds, unchanged before/after sources and clean process joins.
  Its scope excludes actual daily-runtime and deployed-host qualification;
  the three local normal-PoW actuals above supply their separate runtime scope.
  See [exact-tip CI evidence](docs/evidence/set79/ci.json). This post-push result
  is recorded on the next stacked branch; no seed rollout was necessary.

[Component evidence](docs/evidence/set79/components.json) binds each completed
report's byte count and SHA256, its source/profile metadata and command results.
It distinguishes pre-refresh, focused refresh, subsequent full accounting and
focused intake regressions, and retains failed attempts separately from passing
successors. The exact [original component record](docs/evidence/set79/components-before-busy.json)
and [record before the latest intake results](docs/evidence/set79/components-before-intake-result.json)
are preserved, followed by the exact [pre-B4 result snapshot](docs/evidence/set79/components-before-public-b4-result.json).
The intermediate predecessor chain remains intact, including the exact
[pre-Path-B5 snapshot](docs/evidence/set79/components-before-public-b5-result.json)
and the [pre-Path-A2 snapshot](docs/evidence/set79/components-before-attachment-a2-result.json),
followed by the [pre-Path-C1 snapshot](docs/evidence/set79/components-before-headless-c1-result.json)
and [pre-final-static snapshot](docs/evidence/set79/components-before-final-static-result.json).

The [first public actual failure](docs/evidence/set79/public-actual-attempt1-failure.json)
records zero payout attempts and zero fresh CLI lifetimes: all outer groups and
nodes joined, but the runtime itself returned a fatal original round Busy.

Source review traces that contention through the original settlement binding
and round full-audit gate into a scheduler that did not classify round Busy as
retryable. Exact source Busy could also be collapsed to round SourceUnavailable.
The installed successor maps only exact source Busy and retries only error trees
whose every leaf is an allowed transient; any fatal sibling still stops. It
changes no payout, due-time, reservation, codec or timeout. Its focused qualification passes;
the later complete Path B attempt 5 passes. The failed run does not establish
the precise competing maintenance call or instant.

The [second public actual failure](docs/evidence/set79/public-actual-attempt2-failure.json)
retains one attempted and accepted A broadcast and clean runtime/node/group
joins. Its combined startup-readiness/Issue allowance was 20 seconds while
maintenance uses a 30-second interval, but no final paused status was recorded;
that mismatch does not establish the precise failure cause. The source-reviewed
harness successor permits up to 90 seconds of read-only readiness on the same
runtime, then the unchanged 20-second Issue operation, retaining the latest
successful status and checking original Run completion. It changes no product
code, selected configuration, unknown-result retry rule, 900-second scenario or
1,080-second outer bound. Complete Path B attempt 5 later passes with these
bounds unchanged.

The [third public actual failure](docs/evidence/set79/public-actual-attempt3-failure.json)
retains one A send and its original reserved audit. The reopened runtime returned
raw `poolshare.ErrBusy`; its dated last status was paused with one submission.
Runtime completion and owners joined and both nodes stopped, but the runtime
returned a fatal error, so the actual failed. Source review traces this path
through direct intake Status/Enable and the full DAILY census to the original
issuer's nonwaiting reads. It does not identify the precise competing call.
The narrowly qualified retry preserves the original capacity cause across that
contention; the fourth actual advanced further but also failed as recorded below.

The [fourth public actual failure](docs/evidence/set79/public-actual-attempt4-failure.json)
retains the complete public phases through `07-B59-restart.json`: the derived
two-row ordinary intake was full, real job expiry was observed, and the unseen
canonical B became exactly the third accepted receipt and second round. A fresh
production DAILY process returned the exact saved receipt for duplicate and
lookup. At B59 the original A credit was still the sole credit, B was unconfirmed,
and A's original signed body remained byte-identical while delivery advanced
from prepared to confirmed. Exactly one attempted and one successful send used
the same A raw transaction. The second fresh process failed its bounded final
B60 wait after 783.039 seconds overall; no final B60 credit audit exists.

The actual report records `clean_join=true`, no cleanup errors, two joined CLI
lifetimes and clean node shutdowns. Every outer command group is empty. The
outer report's `clean_join=false` accompanies actual exit 1 and must not be read
as a reported owner leak. Sources, copied inputs and profile artifacts stayed
unchanged. Source review identified original-gate contention paths, but the
failed evidence does not identify one exclusive competing call. The separately
qualified DAILY admission successor and complete Path B attempt 5 pass; neither
changes the failed fourth run into a pass.

The [first automatic-wallet actual failure](docs/evidence/set79/attachment-actual-attempt1-failure.json)
retains two accepted pool rewards, original authorizations/outbox evidence,
stopped reopen and a scheduled payment that was validated and mined. Its first
authenticated balance read returned `unavailable`, before the exact final wallet
delta and full closed-ledger audit. The underlying read cause was not captured
and is not a proven Busy. Runtime, owners and nodes joined cleanly and the
original copied fixture stayed unchanged. A bounded read-only harness successor
still requires the complete authenticated values and closed audit. The second
actual passes those full checks; partial progress does not change the first
failed attempt into a pass.

The [first two closeout failures](docs/evidence/set79/closeout-attempts1-2-failures.json)
retain the same Wails import-discovery error before module builds, with unchanged
source/profile inventories and clean command joins. Root restored the incomplete
Wails v3.1.0 extraction from its exact checksum-matched cached archive, preserving
the partial tree; a stale module index in the old build cache still reproduced
the second refusal. Root's same-input fresh-cache probe resolved the application,
and the third closeout used that cache and passed. No product, dependency version
or financial expectation changed. Neither failed attempt is a build pass; the
passing third run remains pre-admission-contention qualification.

The [first final-static failure](docs/evidence/set79/final-builds-attempt1-failure.json)
preserves all 17 successful module builds and the following `lostcancel` refusal.
The test reused a cancellation variable; an early failure path could miss the
second cleanup. The exact two-defer-only successor retains its original explicit
stop/join and financial assertions. The actual source catalogs and public proof
files remain unchanged, and final test/static qualification is recorded separately.

The original qualified corpus was 3,175,720 bytes with SHA256
`0ee0cfc5946987e7f2f1d739fccfda3735591863c8ad8e769edea52f19d8de5a`.
Its inherited template recipe retained semantic values but did not match the
pre-existing canonical Go field order. Product parsing correctly rejected the
work fixture. The failed qualification and entire predecessor corpus remain
preserved under `testdata/public-daily-pre-work-wire-order` and external evidence.
The independently reproduced successor changes only inherited work serialization
order (including transaction hash/txid and template time/commitment positions).
It checks unchanged decoded values and original job/context/hash identities.
No parser was relaxed and no economic, composition or credential expectation
was replaced with product output.

The first desktop qualification also preserved a test-fixture selection failure.
The frozen independent composition explicitly selects
`official-leading-zero-account`, while the new daily tests inherited the V1
helper's first vector, `zero256`. That paired daily expected Setup/backup bytes
with the wrong original material, capsule and wallet derivation. The narrow
test-only successor selects the unique named original vector and its matching
capsule/time before applying daily public expectations; the V1 helper still
selects its original first vector. No producer, product rule, expected economic
value or equality assertion changed. The corrected desktop qualification passed.

The Set78 direct-owner actual and its seven green CI jobs qualify that
predecessor, not this public composition. Likewise Set76's 721-input/64-payee
proof is not continuous 64-miner acceptance. Set79 retains explicit source,
profile, process-join and immutable-public-evidence bindings for its own actual
and passing final CI results; none qualifies continuous load or a deployed host.

## Remaining boundaries

This is finite integration. Source/work/byte limits, signed-authority capacity,
round-window bounds, full-history financial reservation and outbox limits remain.
Work issuance and renewal consume retained jobs even without a winning block.
There is no automatic retirement/rollover, physical-full continuation or trusted
persisted audit cache. An ambiguous Issue hold has no automatic recovery route.
Accepted pool work can still leave a later fighter admission pending when
ordinary intake drains; exact signed/interrupted rows must remain retained.
The passed automatic-worker actual separately records authenticated earnings,
original vault signatures and three completed fighter births; this is not a
universal capacity-race completion claim. The separate delayed-client
actual now proves the full-intake/real-expiry/reserved-payout path and later
same-day credit refresh. Neither proves continuous traffic or the headless
command's process boundary merely by composing its controller directly; the
separate Path C actual now qualifies that concrete process and shutdown path.

No host is provisioned or deployed by this change. No public-chain mining,
production genesis, hosted authentication/MFA or native navigation is approved
by the component results. Finish actual public operation and scheduled payouts,
then later onboarding; preserve the final joint D74 product review.
