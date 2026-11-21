# AGENTS.md — SLUG chain repo (fork of btcd)

This repo is **SLUG**, a fork of btcsuite/btcd (v0.27.0) being turned into a
standalone cryptocurrency: Argon2id proof-of-work, 2-minute blocks, 10 SLUG
block subsidy. It is the chain layer for a fight-league product (fighters are
derived from block hashes; that layer comes later — see HANDOVER.md and
BRAWLCHAIN_SPEC.md in the handover package).

Read HANDOVER.md before starting. It has the full state, the remaining work
spec, and the verification gates. This file is the short standing-rules
version.

Mike's current work order (November 21) is: hosted pool and scheduled payouts;
then wallet balances and history; then sign-in/onboarding; then the full joint
D74 design review. Marketplace work is on hold. Preserve completed fighter
transfer/recovery work and D82's mandatory authentication requirements.

Fix a newly found issue immediately only if it blocks the agreed test flow,
risks funds or lost data, or breaks security or payout correctness. Record all
other known open edge cases in `docs/EDGE_CASE_FOLLOW_UPS.md`, with trigger,
player impact, evidence, workaround and revisit point. Public-launch-only
limits must be labeled and must not delay a safe, explicitly bounded test build.
This supersedes earlier sequencing that required general capacity continuation
before any hosted test qualification. Never bypass existing capacity, reset
histories, discard recovery rights or weaken D84 to make a test pass. The active
burn list is in `LAUNCH_READINESS.md`; keep it linked to the follow-up log.
Fixture proofs alone still do not establish hosted operation or launch approval.
D74 remains the final joint review before friends/tester or public release.

Mike explicitly excludes Wandercom and Growmaple from agent access. Do not
query or operate on either organization's repositories or other resources.
Keep GitHub work scoped to the authorized SLUG repositories. The broad GitHub
CLI workflow-scope authorization was canceled at his direction; any replacement
authorization must be repository-limited, not an account-wide OAuth expansion.

## Branch discipline (do not break this)

- Mike explicitly authorized the Phase A master merge and seed deployment
  on November 15, 2026. The closeout merge ends master's pristine-upstream
  role; upstream btcd remains available at `dd169fe0c32a56087d28b92fff3b8cf4fb43477a`.
- `master` is the release/deployment branch after closeout. Develop changes
  on topic branches and validate them before promotion. Treat any push to
  master as a live deployment; obtain authorization for a new rollout.
- On November 16 Mike authorized continued work toward the desktop mining
  pilot and deployment/verification of completed sets as needed, without
  per-set rollout questions. That authorization persists: continue until a
  decision needs him, and raise an explicit blocking question in the chat.
  Do not claim push-notification delivery; notification controls are unavailable
  to this session. Existing private-key and consensus rules remain in force.
- Work lands as numbered **change sets**, each on its own branch stacked on
  the previous one:
  `slug-pow-swap` → `slug-chain-params` → `slug-chain-fixes` → `slug-deploy`
  → `slug-rebrand` → `slug-module-rename` (set 6, `df655087`)
  → `slug-signed-checkpoints` (set 7, `6b3e9e90`)
  → `slug-pow-security` (set 8, `c8176de5`)
  → `slug-test-pow` (set 9, `273b5e17`)
  → `slug-checkpoint-authority` (set 10, `0ca7466a`)
  → `slug-first-checkpoint` (set 11, `af67093f`)
  → `slug-phase-a-closeout` (set 12, released on master at `e056146f`)
  → `slug-fighter-dna` (set 13, released on master at `0f44da5f`)
  → `slug-prospect-proof` (set 14, released on master at `fdeebae2`)
  → `slug-prospect-registry` (set 15, released on master at `6443d163`)
  → `slug-trusted-prospect-context` (set 16, released on master at `e94c1cfa`)
  → `slug-gym-identity` (set 17, released on master at `7968e0a0`)
  → `slug-prospect-birth` (set 18, released on master at `c88abe11`)
  → `slug-prospect-service` (set 19, released on master at `56a4889d`)
  → `slug-mining-templates` (set 20, released on master at `3ed3b71e`)
  → `slug-automatic-miner` (set 21, released on master at `59e14a8c`)
  → `slug-desktop-pilot` (set 22, released on master at `fad9cbdf`)
  → `slug-wallet-foundation` (set 23, released on master at `e1b25e7f`)
  → `slug-wallet-recovery` (set 24, released on master at `0b04cd1b`)
  → `slug-phase-a-activation` (set 25, approved activation at height 62)
  → `slug-desktop-wallet` (set 26, released on master at `f03f1ef6`)
  → `slug-account-enrollment` (set 27, released on master at `a541fb58`)
  → `slug-identity-recovery` (set 28, released on master at `bc92fd53`)
  → `slug-recovery-vault` (set 29, encrypted fixture retention)
  → `slug-production-profile` (set 30, approved D73 profile design)
  → `slug-provider-coordinator` (set 31, fixture authentication/reset journal)
  → `slug-resumable-recovery` (set 32, bounded candidate wallet recovery)
  → `slug-fighter-manifest` (set 33, static candidate fighter previews)
  → `slug-fighter-portraits` (set 34, deterministic preview SVG)
  → `slug-fighter-condition` (set 35, exact candidate aging projection)
  → `slug-manifest-jobs` (set 36, candidate appearance-bound job codecs)
  → `slug-manifest-authority` (set 37, pinned candidate admission store)
  → `slug-idle-headers` (set 38, idle-chain history serving)
  → `slug-manifest-miner` (set 39, candidate service and durable miner)
  → `slug-manifest-desktop` (set 40, opt-in candidate fixture desktop)
  → `slug-gym-stable` (set 41, durable candidate stable and ten-slot roster)
  → `slug-fight-preview` (set 42, released on master at `fc672113`)
  → `slug-saturday-entry` (set 43, released on master at `e2761812`)
  → `slug-event-snapshot` (set 44, released on master at `1fc307e0`)
  → `slug-event-seed` (set 45, released on master at `dcc44829`)
  → `slug-event-card` (set 46, released on master at `5c11cbaf`)
  → `slug-event-bouts` (set 47, released on master at `ba218f55`)
  → `slug-result-publication` (set 48, released on master at `cc4f41e4`)
  → `slug-career-awards` (set 49, released on master at `0a4374de`)
  → `slug-career-preview` (set 50, released on master at `592e143c`).
  Set 49 implementation is `5b590f40`, stacked from completed Set 48 topic
  `dd74828f`; its normal package and complete 900-second closeout passed.
  Set 50 implementation is `99d36ef2`, stacked from Set 49's completed topic
  maintenance tip `b3150b49`. Its independently frozen `CAREER_PREVIEW.md`
  family passed scoped awarded-fighter and normal XP0 package acceptance;
  practice remains unrecorded and grants no awards. Preserve the old XP0
  Preview family/engine and every failed acceptance/checker attempt. Final
  observation and provider closeout passed: 900.03 seconds, 31 successful
  samples at height 61 with 60 checkpoint descendants, and a clean restart.
  Railway remained healthy on the exact release, with configuration, volume
  and TCP endpoint unchanged.
  Remote CI absence is assessed, not a pass. One local signed-out header-logo
  and user-confirmed menu-bar fist check does not replace the full D74 review.
- Set 51 on `slug-fighter-history`, implementation `751b947b`, completes the
  authenticated finalized fighter-history candidate. Local, scoped actual,
  cold-capacity, normal Mac package and eleven independent package checks pass.
  Preserve its failed first package attempt and unresolved original cause;
  the focused diagnostic and fresh strict package pass are successor evidence.
  Version 0.4.11/build 51 is a verified desktop build. No compiled seed-node
  source or deployment configuration changed, so master and the live seed
  remain at Set 50 (`592e143c`). See `CHANGESET_51_NOTES.md`.
- Set 52 on `slug-pool-shares` adds immutable manifest pool-work verification
  with independent pool/prospect/block targets. Its focused normal/race/profile
  refusal tests and vet pass. No durable share acceptance, credits, payments or
  live pool are enabled, and no seed rollout is needed. See `CHANGESET_52_NOTES.md`.
- Set 53 on `slug-pool-acceptance` adds durable issued pool work, authenticated
  share acceptance, duplicate recovery and canonical-only D80 late credit.
  Normal/race/profile-refusal checks and vet pass. Use its dedicated manifest
  issuer and preserve exact pending block evidence on uncertainty. The journal
  grants no fighter admission, monetary balance or payment. Public pool/miner
  integration and round accounting remain ahead. See `CHANGESET_53_NOTES.md`.
- Set 54 on `slug-pool-rounds` adds operator-only accepted-work pages, fixed
  pool rounds, exact fractional gross allocations and fresh 60-descendant
  confirmation locks. D81 orphan/return never changes membership; later
  conflicts preserve confirmed gross earnings. Normal/race/profile-refusal
  checks and vet pass. This is not maturity, spendability or payout authority;
  no changed package is linked by the seed. See `CHANGESET_54_NOTES.md`.
- Set 55 on `slug-pool-settlement` completes D58's daily settlement contract
  with exact fractional carry, original mature funding and a durable signed
  batch before broadcast. Network fees use the operator's 1% only; deferrals
  stay visible and retries cannot create another payment. The isolated public
  fixture signer is not a production wallet. See `CHANGESET_55_NOTES.md` for
  validation and the remaining service integration boundary.
- Set 56 on `slug-pool-service` adds authenticated work/share/balance HTTP and
  the desktop's original-view credential adapter. Pin the complete pool source,
  transport and expected gym; preserve exact job/nonce recovery on uncertainty.
  The concrete pool and settlement take their own account leases: never nest
  those calls under a service/view lease. Pool receipts never grant fighter
  birth. Shared runtime and a dedicated durable pool worker remain next; do not
  silently switch the existing solo worker or rewrite its wallet payout marker.
  See `POOL_SERVICE.md` and `CHANGESET_56_NOTES.md`.
- Set 57 on `slug-pool-worker` adds a separate `PoolWorker`/`PoolOutbox` v1.
  Mine only exact issued templates with extra nonce zero; preserve independent
  pool, block and prospect outcomes before transport. Recover original job and
  nonce after uncertainty; a sequence-zero receipt is not credit. Broadcast
  blocks before signing, never sign new expired births, and allow historical
  signed-receipt recovery without changing birth. Use the admission-only v3
  service when sharing a pool issuer; never expose outside job issuance.
  Capacity pauses mining while preserving pending recovery. The shared desktop
  runtime and balance view follow; do not silently replace the solo route.
  See `POOL_WORKER.md` and `CHANGESET_57_NOTES.md`.
- Set 58 on `slug-pool-gym-binding` adds separately pinned native84/index0
  pool recipients for the two published fixture wallets. Preserve the old
  scope44 payout and markers. Persist account/source intent before allocating;
  a lost address-allocation response requires wallet close/reopen/reattachment
  and recovery of the original index0 address, never blind reallocation.
  READY requires synchronized wallet ownership/path checks and the original
  token/view recheck. Matching existing markers must be synchronized again
  after uncertain publication. Shared-source stable isolation and parent
  runtime follow; no current app route or production custody is activated.
  See `POOL_RECIPIENTS.md` and `CHANGESET_58_NOTES.md`.
- Set 59 on `slug-shared-gym-source` adds explicit Pin2 shared-source stables.
  Verify every global source row before filtering by gym; retain original
  sequences and complete assignment-or-skip audit coverage. The global cursor
  is not a fighter count: use SharedStatus.AssignmentCount. Follow Recent must
  select the local tail, preserving opaque career read/session fences. Keep
  legacy Pin1 namespaces and projection semantics unchanged; shared typed pages
  cannot publish a contiguous legacy projection. No current app route switches
  implicitly. See `SHARED_GYM_STABLE.md` and `CHANGESET_59_NOTES.md`.
- Set 60 on `slug-pool-session-fences` releases account leases during pool
  proof/node/source work and balance verification. Capture the original token
  and key; reacquire that same authority around each actual local commit and
  final disclosure. Do not weaken WithSession or retain its temporary recheck.
  Registry staged commits use TryLock; retry busy outside the account lease.
  The shared issuer must select NewFencedManifestAdmissionServer with one
  admission writer and pool-only job issuance. Preserve exact pending evidence,
  outcome-unknown recovery, old schemas and legacy isolated APIs. Next deliver
  the runnable explicit shared fixture app: parent-owned nodes/issuer/journals,
  two actual recipient-bound wallets, separate workers/outboxes/Pin2 stables,
  visible authenticated balances and bounded daily scheduling. Preserve the
  solo route and D74 gate. See `POOL_SESSION_FENCES.md` and
  `CHANGESET_60_NOTES.md`.
- Set 61 on `slug-shared-pool-desktop` adds the explicit `--candidate-pool`
  route and separate `SLUG Pool Pilot` / `LocalPoolPilot` namespace. One parent
  owns the nodes, shared issuer and accounting journals; original-view children
  own their wallet, worker, outbox and Pin2 stable. Both actual fixture recipients
  must verify before services start. Only Start mines; logout/reset joins the
  child while retaining the source, and Quit joins all owners. Preserve exact
  source/transport/recipient markers; missing established components fail closed.
  Shared counts are gym-local; balances retain exact fractions and observation
  timestamps. Preserve the solo route. Shared-wallet recovery follows in Set62,
  then explicit shared league attachment; never repin old histories or reorganize
  navigation. See `SHARED_POOL_DESKTOP.md` and `CHANGESET_61_NOTES.md`; D74 stays last.
- Set62 on `slug-shared-pool-recovery` connects native recovery export and
  file-only fresh verification to the actual Pool Pilot wallet. Capture the
  original view before the dialog, retain directory/file descriptors through
  helper handoff, and publish owner-only files exclusively under the original
  RecoveryExport capability. D72 blocks export during a hold but allows viewing
  and existing-file verification. Wait for actual synchronization even at
  genesis; retain complete confirmed-state comparison and retry when pending
  transactions or changing observations prevent a stable comparison. Uncertain
  publication must tell the user to inspect the destination before retrying.
  Cancel/Close join cleanup; verification never spends or starts mining. The
  funded file-only restore/spend, package, full desktop and smoke checks pass.
  Shared league attachment is next; preserve solo histories and the final D74
  gate. See `SHARED_POOL_RECOVERY.md` and `CHANGESET_62_NOTES.md`.
- Set63 on `slug-shared-pool-entries` connects shared mined fighters to Saturday
  entries and a parent-owned noon coordinator. Preserve exact Pin2 participants,
  original global receipt sequences, immutable setup intent/READY identities and
  child-owned entry handles. Missing established stores fail closed. Recheck the
  original view through native dispatch and commit; backup drains active lane
  work outside account leases and does not restart mining. A Recent page does
  not prove complete shared-source synchronization: roster/entry mutations still
  require caught-up authority. Normal-PoW noon/reset/restart acceptance and exact
  normal-package before-noon/recovery checks passed. Next attach the complete
  shared publication/card/result/career cycle, activating career authority before
  its new lock and first finality. Preserve prior snapshots without backdating
  activation. Keep publication funding separate from pool earnings. See
  `SHARED_POOL_LEAGUE.md` and `CHANGESET_63_NOTES.md`; D74 remains last.
- Set64 on `slug-shared-event-cycle` attaches the full shared event/career
  pipeline. Activate the parent-owned result reader before publishing children;
  persist the exact source/store/activation chain and fail closed on missing
  established components. Borrow the concrete existing node pair, retaining
  source pins, owner handles and original-view capabilities. Retry only transient
  issuer contention within the existing request/lifetime bounds, outside account
  leases. Keep scalar1/2 fixture publication funding separate from scalar3 pool
  settlement. Genuine seed/result finality, exact-once careers, restart and C1
  refusal pass on the normal-PoW fixture chain. Pre-enrollment never exempts an
  awarded fighter from C1's later zero-record cutoff. The normal package retains
  explicit Start-only mining, recovery and existing panels; practice stays off.
  See `SHARED_EVENT_CYCLE.md` and `CHANGESET_64_NOTES.md`. Continue functional
  blockers with frozen-history compatibility; D74 remains the final joint gate.
- Set65 on `slug-name-diversity` adds the explicitly selected name-v2 candidate
  with 512 first names and 512 surnames. Default constructors and existing
  sources remain v1; never infer a version from saved data or repin history.
  Resolve the complete original descriptor at admission, reconstruction and
  every transport/portrait boundary, including the manifest hash domain in a
  saved bout response. The frozen independent corpus covers all name indices;
  the reproducible collision sample is a scenario, not a population forecast.
  Normal-PoW v2 event/finality/career acceptance, v1/v2 package recovery and the
  full desktop suite pass. Preserve the failed first bout-response acceptance
  and independent diagnosis. See `NAME_DIVERSITY.md` and `CHANGESET_65_NOTES.md`.
  Continue portable public evidence without claiming ownership, authenticated
  admission time or complete authoritative history from unsigned summaries.
  Transfer timing/order remains a later policy question; D74 remains last.
- Set66 on `slug-public-fighter-evidence` adds bounded original-receipt/history
  export and a separate offline consistency checker. Require a separately
  established full expected source Pin; the file authenticates neither issuance
  time, actual awards/finality, current ownership nor global history completeness.
  Use one concrete source proof, the original opaque history capture and exact
  native action ID. Never recover a new export by adopting an older latest job.
  Stop/Close join proof and file cleanup; asynchronous account transition
  acknowledgments do not prove cleanup. Preserve possible-publication uncertainty
  and the old recovery-file limit. Normal/race/reference, six actual exports,
  complete desktop, normal package and smoke checks pass; retain the sandbox
  failure and explicit composite rerun evidence. See `CHANGESET_66_NOTES.md`.
  Own-payout history follows in Set67; no transfer policy or navigation redesign
  is implied.
- Set67 on `slug-payout-history` adds an authenticated own-payment capture,
  fixed local pages and a separate dated observation of a selected output in
  the owned wallet. Preserve the original recipient, source, view and exact
  native action through reads and uncertain replies. Pool-recorded state and
  wallet-reported confirmation are separate facts; neither authorizes another
  payment. Stop/backup/Close join the independent payout slot before reusing
  resources. Keep errors distinct from a verified empty history and keep the
  former capture after a failed refresh. See `POOL_PAYOUT_HISTORY.md`,
  `POOL_PAYOUT_DESKTOP.md` and `CHANGESET_67_NOTES.md` for qualification status.
  Continue remaining wallet/lifecycle work under settled policies; transfers
  and general league matching still need their own decisions. D74 stays last.
- Set68 on `slug-reviewed-withdrawals` adds separately reviewed, durable personal
  withdrawals to the fixture Pool Pilot. Keep exact intent/draft/signed bytes,
  wallet reservations and the paired journal across interrupted replies and
  reopen. Fresh original-view D72 authorization gates signing, explicit retry
  and unsigned abandonment; a send acknowledgment is not an observed payment.
  Never automatically resend or replace a signed transaction. Native review
  displays the full recipient and exact amount/fee/total. Keep the candidate
  fee ceiling, bounded selection, concrete resource provenance and paired-store
  audits; missing established components fail closed. The two-wallet actual
  rehearsal proves lost-ack retry with one debit. See the frozen contracts in
  `REVIEWED_WITHDRAWALS_ADOPTION.md` and qualification in `CHANGESET_68_NOTES.md`.
  This remains schema-9 public-fixture engineering, not production issuance or
  full evolving-wallet recovery. D74 and permanent identity/custody gates remain.
- Set69 on `slug-resumable-phase-a-backup` adds explicit schema-12 bounded
  restoration of the existing version-1 Phase-A recovery file. Preserve coin-0
  paths, mixed49 branches, root/gym identity and original imported manifest;
  never migrate installed wallets or recreate withdrawal authorization from a
  recovery file. Keep 128-attempt steps, clipped 4096-per-branch limits and exact
  private wallet/node tip comparisons. Failed cleanup retains its owner and
  scratch directory; a later no-op Close cannot clear the first failure. The
  completed scanner audit permits later live allocation and retained orphan
  discovery without equating historical rows to the current wallet ledger.
  `CHANGESET_69_NOTES.md` records qualification and its precise limits. Hosted
  evolving metadata, production custody and D74 remain separate gates.
- Set70 on `slug-wallet-activity` adds a bounded, immutable own-wallet activity
  capture, eight-row pages and selected transaction details in the existing
  wallet section. Keep integer amounts, the atomic public identity/history/sync
  snapshot, exact wallet/node hash and height checks, original View/source/gym
  binding, native-known job IDs and explicit refresh. Failed refresh retains the
  dated prior capture; old success never resolves a new uncertain action.
  Capture/detail cancellation must join before wallet unload or backup. Read no
  private recovery material; do not infer pool earnings, spendability, finality
  or a new send permission from activity. Preserve the independently frozen
  corpora and bounded storage/raw-transaction checks. See `WALLET_ACTIVITY.md`
  and `CHANGESET_70_NOTES.md` for qualification and limits. Hosted evolving
  recovery remains separate; D74 remains last.
- Set71 on `slug-hosted-current-recovery` adds the explicit
  `--candidate-hosted-pool` route in a separate selected namespace. Preserve
  legacy Pool defaults and stores. Pin authority/coordinator/vault/wallet-owner
  IDs before setup, retain ordered immutable receipts and fail closed on missing
  established storage. Signed published-fixture login/reset is not a real
  provider: retain the original authentication epoch, exact reset event and
  durable barrier before credential changes or child joins. Pending reset must
  remain reachable while signed out; replay never extends the hold.
  Current recovery captures only bounded public committed wallet metadata from
  the retained concrete schema-9 owner. Join captures before unloading; do not
  hold wallet/parent mutexes across account fences. Current checkpoints and
  exports are separate from old immutable vault APIs. Keep recovery and gym
  envelopes distinct, no local signer fallback, and no recovery unwrap for held
  mining. Prepare export outside account leases, then use the original
  RecoveryExport fence only for bounded publication to the pinned destination.
  Preserve uncertain outcomes and cleanup ownership. A current export is a dated
  capture, not continuous disaster recovery or permission to recreate missing
  payment journals. See `HOSTED_CURRENT_RECOVERY.md`, the portable independent
  references and `CHANGESET_71_NOTES.md` for qualification and limits. Real
  provider/custody and permanent identity remain gates; D74 stays last.
- Set72 on `slug-hosted-allocation-recovery` adds explicit
  `--candidate-hosted-recovery` / `LocalHostedRecoveryPilot` with fresh schema13.
  Reserve the complete supported address prefix before consumption/disclosure;
  burned, unused, uncertain and abandoned ranges remain covered. Keep exact
  Plan/Reserve/Consume/Ack/Finalize owner and operation identity, read-only lookup,
  original-view disclosure and fresh process-local Resume/Redisplay actions.
  Only exact wallet-file/directory absence with all other required authorities
  intact permits recovery-only export and schema12 inspection. Never recreate
  missing payment journals or install a replacement spending wallet through it.
  Preserve physically read-only archives, D72 holds and sticky cleanup ownership.
  Full wallet, funded loss/restore/spend, frontend, smoke and explicitly composed
  desktop/package qualification passed; retain the legacy typed-nil failure and
  its narrow successor. See `HOSTED_ALLOCATION_RECOVERY.md` and
  `CHANGESET_72_NOTES.md`. No seed input changed. Continue D83 ownership/direct
  transfer and independently restored-gym use; login and D74 remain deferred.
- Set73 on `slug-fighter-ownership` adds public ownership replay and the explicit
  owned-gym route, with schema14 hosted carriers and file-only schema12-to15
  restored-gym use. Preserve canonical first-valid transfer order, immutable
  birth/history, signed checkpoint plus 60-descendant effectiveness, original
  View/D72 fences and exact request/sign/delivery separation. Public source
  gaps refuse complete reconstruction; a saved delivery receipt is not current
  ownership. A pending chain publication whose named block is no longer
  canonical stays held; never retarget it. Reference recovery never grants ready
  ownership. Independent recovery uses the original exported ranges and fresh
  public history, never copied private authorization journals. No noon cutoff
  may be backdated or replaced by current ownership. Actual CLI recovery and both
  noon ownership variants passed, including immutable history and component
  reopen. Clean release packaging and the complete 900-second post-rollout
  observation passed. Master and Railway run release `e3eb4d49`; source,
  persistent volume and public endpoint were verified. Native/D74 product gates
  remain open. See
  `PUBLIC_FIGHTER_OWNERSHIP.md`, `OWNED_GYM_DESKTOP.md`, `RESTORED_GYM_CLI.md` and
  `CHANGESET_73_NOTES.md`. Public pool/scheduled payouts follow this milestone;
  sign-in/onboarding and the final D74 product review remain later.
- Set74 on `slug-public-pool-runtime` follows completed Set73 maintenance
  `1aba70ca`. Public operator service and scheduled payouts precede sign-in.
  Recovery-only settlement, original-store binding, operator admission/publication
  custody and ownership replay, recovery-first UTC scheduling and the concrete payout-node connection
  passed focused normal, race and profile-refusal checks. The dedicated schema16
  wallet and standalone TLS operator passed a two-miner normal-PoW scheduled
  payout/restart rehearsal at chain `4a3f20da` / wallet `40959793`: maturity deferral,
  one batched payment, accepted lost-response recovery, no duplicate and clean
  standalone CLI reopen. Offline provisioning and remote public configuration/
  credential custody now pass focused normal/race/refusal qualification. Only
  the operator runs real provisioning; no private key is an agent input. The
  existing schema14 wallet also passed actual TLS mining, accepted share/fighter
  receipt audit and exact stopped reopen. Native Linux qualification passed at
  chain `c41cc337` / wallet `6a69ecfc`, including normal/race custody checks and
  real ACL controls. GitHub CI repair is verified at `47e1ce84`: all six
  Build and Test jobs and the Linux operator job passed. Independent SLUG
  address vectors retain original payloads; full chain/race suites remain
  enabled under their explicit test-PoW profile, with the normal proof checked
  separately. See `docs/evidence/set74/ci-repair-remote.json`.
  The separate remote native app now passes visible fixture login, Start,
  native-menu Stop, sign out and Quit. Closed audits match an accepted share
  and separate fighter receipt; every owner joined, with original history
  unchanged. Preserve the two failed harness attempts and their corrections.
  See `docs/evidence/set74/remote-native.json`; this is not hosted login or D74.
  Concrete host handoff, remote history/payout presentation and
  continuous capacity/host qualification remain; do not call the pool deployed.
  Unseen canonical winners from original issued jobs can now cross the ordinary
  intake ceiling after a separate two-axis whole-history reservation, within
  unchanged V1 physical and financial limits. Normal/race regressions include
  full-owner reopen, one added credit and unchanged reserved payout bytes.
  Actual normal-PoW acceptance now passes full ordinary intake, real job expiry,
  unseen canonical recovery, exact receipt retry after original-owner restart,
  one additional B credit and unchanged signed/reserved payout A through lost
  reply and confirmation. Preserve all four failed harness attempts. Physically
  full or financially unbacked journals still hold; broader capacity continuation
  remains a release blocker. Never discard D80 rights, reset histories or
  bypass a reservation. See `docs/evidence/set74/late-winner-regression.json`
  and `docs/evidence/set74/late-winner-actual.json`. No host deployment is proved.
  Scheduling retains the original settlement;
  only completed daily batches advance its cursor. Node evidence grants no
  signing authority; uncertain delivery retries only the retained transaction.
  Operator ownership uses explicit issuer/replica roles and `/slug/ownership/v2`;
  retain the original issuer owner and keep replicas/audits without signing
  authority. Never migrate V1 history or load the offline checkpoint key online.
  Keep exact retained transaction recovery separate from daily preparation.
  See `CHANGESET_74_NOTES.md`; no public pool readiness is implied.
- Set75 on `slug-pool-capacity` follows Set74 `e0e71ac0`. Its first slice
  streams the concrete accepted prefix/round audit into settlement while keeping
  all V1 schemas, source pins, duplicate guards and ceilings. Authenticate the
  full source and stored namespaces before callbacks or commits; capacity must
  never mask corruption. Stage credit imports until complete source/old-credit
  validation succeeds. Preserve original pending payout bytes and owner handles.
  D84 (Mike, November 20): if the complete daily payout cannot fit, defer the
  entire batch with a visible transaction-capacity reason. No oldest-first
  subset, multiple payouts, miner surcharge or reserve advance. Inspecting
  funding in pages never selects only one page. A deferral occupies no daily
  slot. Waiting need not shrink an oversized batch; report the capacity hold.
  Six pool packages pass normal/race checks, thirteen cheap-profile refusals
  pass, and focused desktop normal/race/vet checks pass. Preserve the sandbox
  bind failure and unsupported vet-wrapper invocation as failed harness attempts.
  Streaming is a prerequisite, not continuous capacity: original V1 history,
  source/client, signed publication and funding-claim limits remain. Continue
  explicit durable adoption/append history, preserved old-job recovery and
  full-intake restart acceptance before public host qualification. See
  `POOL_CAPACITY_CONTINUATION.md` and `CHANGESET_75_NOTES.md`.
- Set76 adds a fresh, explicitly selected V2 daily foundation across source,
  rounds, indexed settlement, schema16 custody, node evidence and scheduling.
  Old V1 constructors, pins and roots stay strict; no migration or automatic
  adoption is implemented. Source2 still has finite lifetime caps: 8,192 works
  and 16,384 receipts. Daily settlement has 16,384 credits and 32,767 batches.
  Preserve one original Store/backend capability, exact pending bytes, global
  duplicate guards and D84 whole-payout deferral. Node/signing operation limits
  remain unchanged in time; the daily transaction must fit standard weight.
  Component normal/race/profile/vet checks pass; the complete ledger/scheduler
  suites pass 71 top-level tests per mode. The 17-target build/full operator
  normal/race closeout and normal-PoW seed smoke/restart pass. Actual attempt3
  passes the 721-input/64-payee financial proof in-process; its owner reopen
  retains the wire hash cache and is not fresh-process replay evidence.
  Fresh-process attempt4 passes with a 142.534-second cold source audit, exact
  A/B recovery and no additional broadcast; all owners and nodes join cleanly.
  Exact-tip GitHub CI passes all seven jobs on chain2ca9fc16/walletdfc5d9dd.
  See docs/evidence/set76/ci.json. The isolated proof does not model
  continuous mining by 64 miners, ordinary share traffic
  or job refresh/expiry. Do not treat it as public readiness. Explicit V2 public
  transport/operator/native integration, signed-authority capacity, original
  late-winner recovery at V2 intake boundaries, durable continuation and hosted
  scheduled operation remain gates. Never silently enlarge a signed profile or
  repin an old wallet. See `CHANGESET_76_NOTES.md` and
  `POOL_CAPACITY_CONTINUATION.md`; sign-in follows public pool work and D74 is last.
- Set77 on `slug-pool-live-rounds` adds explicitly selected live round List and
  Refresh on the original fully audited Store. Full Sync stays atomic over its
  complete captured tail; full financial audits and D84 remain unchanged. The
  live directory checks exact touched bytes and caches no chain observation.
  Unknown/uncertain writes and failed full audits revoke it; Close joins work.
  Original pre-Run runtime selection preserves defaults and permits only one
  exact-stale reanchor within the existing deadline. All 17 builds, full
  accounting/operator normal/race, profile refusal and vet checks pass. Preserve
  the first test-only two-gym balance assertion failure and its exact correction.
  This is not V2 public-runtime or continuous-capacity acceptance. Continue
  explicit daily intake/reservation and public integration before onboarding;
  D74 stays last. See `CHANGESET_77_NOTES.md` and `POOL_LIVE_ROUNDS.md`.
- Set78 adds finite daily intake reservation on the original Source2 owner.
  Derive ordinary capacity (at most 4,095), reserve a canonical receipt before
  issuer effects, audit all retained liabilities/indexes/membership and require
  fresh canonical proof for unseen post-drain recovery. Physical/work/byte and
  signed-authority limits remain; no rollover, V1 adoption or profile widening.
  Keep original source/round/ledger/backend bindings, exact pending payout bytes
  and D84 whole-payout deferral. Independent oracles, all 17 builds, accounting
  and operator normal/race, cheap refusal and vet pass. Preserve the first
  kernel test's zero-descendant tip-hash fixture failure and its test-only fix.
  Normal-node/schema16 fresh-process two-to-three receipt recovery and seed
  smoke/restart pass; preserve the actual runner/checker failures and exact
  checker-only correction. Set78 exact-tip CI passes all seven jobs at
  chain `1f086601647c` / wallet `dfc5d9dd7e0f`; see docs/evidence/set78/ci.json. This is not
  miner-load or public TLS/runtime acceptance. Set77's seven predecessor CI jobs
  are green at chain `21da8ea51de2` / wallet `dfc5d9dd7e0f`.
  Continue explicit V2 public integration and hosted payouts before onboarding;
  D74 remains last. See `POOL_DAILY_ADMISSION.md` and `CHANGESET_78_NOTES.md`.
- Set79 on `slug-public-daily-pool` explicitly connects DAILY public TLS,
  provisioning/runtime, paged money history, the existing-wallet worker and
  `slugpilot --daily`. V1 defaults, old roots, wallet pins and D84 remain strict.
  Complete affected accounting/operator normal/race checks and selected
  transport/worker/desktop/profile controls pass. Normal-PoW actual acceptance
  now passes all three paths: full ordinary intake plus a real-expiry canonical
  winner and fresh-process retry (one B credit, immutable reserved A, one send);
  automatic original-wallet mining to a scheduled payment; and the actual
  headless command with one SIGTERM, exit 0, joined cleanup and wallet receipt.
  Both automatic paths paid 19.8 SLUG from two rewards in one transaction.
  Preserve the failed attempts and exact corrections. Public source/evidence
  archives are path-bound, not a portable execution claim. Final builds/static
  and all seven exact-tip CI jobs pass at `fdd40d8e`; downloaded Linux evidence
  includes 339 required passing assertions with unchanged sources and clean
  joins. See `CHANGESET_79_NOTES.md` and `docs/evidence/set79/ci.json`; do not
  infer a public deployment from these proofs.
  DAILY contention retries only original-owner read/admission work within the
  fixed budget; never replay an entire quote/sign/broadcast on Busy. Completed
  daily slots may refresh accounting without preparing another payment.
  Unknown Issue outcomes retain a durable hold across restart; both automatic
  proofs retained that hold. Exact-request issuance recovery is next, followed
  by continuous history/capacity and real hosted operation. Never clear an
  unidentified hold, adopt the latest job, renew expiry or discard old rights.
  Source2 lifetime caps and signed-authority limits remain. The node closure
  is unchanged; avoid a redundant seed rollout. Sign-in follows public-pool
  work, and the joint D74 review remains last. See `POOL_PUBLIC_DAILY.md`.
- Set80 on `slug-pool-issue-recovery` adds explicit
  `--daily-recoverable-issue`: synchronize the exact request before network
  dispatch, resume the original issuer job/template, and retain validated
  completion or the original unused terminal without renewing expiry. V1 and
  plain DAILY roots stay strict; no implicit migration or latest-job adoption.
  Independent references, focused normal/race/profile controls, full affected
  normal/race suites, all-module builds/vet and normal-node smoke pass. The
  original cumulative desktop race timeout is preserved; four fixed complete
  pilot partitions pass within the unchanged per-process limit.
  Actual B proves real unused-job expiry after process crash with no birth or
  money; A3 proves the crash/reply-loss/replay path and one real scheduled
  9.9-SLUG payment to the original stopped wallet, with all owners joined.
  A1 is explicitly disqualified for checker source mutation during execution;
  A2 retains its omitted live error as unexplained. Never relabel either passed.
  A3's diagnostics do not change acceptance/retry/deadline rules. Exact-tip CI
  at `ce9eac74` exposed delayed lifetime cancellation in the gym-stable close
  path. A deterministic pre-fix reproduction is retained; direct original-life
  checks pass the full stable normal/race suites and vet. Preserve source/clock
  failure precedence and distinguish active cancellation from later ErrClosed.
  All seven follow-up exact-tip CI jobs pass at `1f559221`; downloaded Linux
  evidence verifies 428 required assertions, unchanged sources and clean joins.
  See `docs/evidence/set80/ci-closeout/` and
  `docs/evidence/set80/stable-close-ci/`, `CHANGESET_80_NOTES.md` and
  `POOL_ISSUE_RECOVERY.md`. The November 21 active work order above supersedes
  the previous general-capacity-before-host sequencing. Independent signed
  limits, D80's no-late-birth rule and D84 remain.
- Set81 on `slug-pool-retained-admission` preserves original-job admission at
  an ordinary DAILY pool-only drain, with authoritative expiry preflight and
  bounded exact-Busy round retries. Initial full affected normal/race suites
  pass; the successor passes all 69 operator normal tests, 16 targeted race
  tests, all 17 module builds/static checks and unchanged-node smoke. Actual A2
  passes full intake, restarts, lost admission reply, identical replay and one
  scheduled 19.8-SLUG payment. B1 fails extended offline publication readiness
  before expiry replay/final wallet verification; preserve it as EF-12, not a
  pass. All seven exact-tip CI jobs pass at `138553b1`; downloaded Linux evidence
  verifies 470 required assertions, unchanged sources and clean joins. See `CHANGESET_81_NOTES.md` and
  `docs/evidence/set81/`. Continue bounded hosted pool/payouts; general capacity
  and long-outage follow-ups do not block that test unless they meet the triage
  rule. Wallet balances/history follow, then sign-in and the full D74 review.
- Set82 on `slug-hosted-pool-test` packages the bounded operator with exact
  product source `138553b1` and wallet `dfc5d9dd`. Native image job `44919766385`
  passes at package commit `2d79de95`: ten required component tests plus five
  named Linux ACL subcases under UID10001, then both shipped command help paths
  in the final scratch image. Public source archives and source-before/after
  match. All seven exact-tip CI jobs pass at `2d79de95`, including the complete
  race job; see `docs/evidence/set82/ci.json`. This is not hosted mining,
  provider volume, full serve shutdown or payout qualification; continue those
  concrete host checks without general capacity expansion. See
  `CHANGESET_82_NOTES.md` and `deploy/pool/README.md`. Transport-pin renewal and
  rotation limits are logged as EF-13; do not fix them incidentally.
- Mike requires a thorough joint design review before friends/tester or public
  product release (D74, November 16). Keep this gate in LAUNCH_READINESS.md;
  a solo review does not replace the session with him. He wants one full review
  at the end once all major flows are ready, rather than partial design reviews.
  Continue authorized engineering deployments, but do not treat them as product
  launch approval.
  On November 18 Mike clarified that app organization and navigation changes
  also belong in that final session. The external Gym/Fighters/Saturday draft
  is stopped and unapplied; do not resume it as the next engineering set.
  Continue functional release blockers and avoid redundant seed rollouts for
  desktop-only work when compiled node source and deployment config are unchanged.
- Branch each next change set off the latest completed implementation tip,
  never the live seed branch by default. Use `slug-<topic>` names; set 7 is
  `slug-signed-checkpoints`. HANDOVER.md records the original takeover state.
- **Never open a pull request.** On a fork, GitHub's PR prompts can target
  btcsuite upstream. Push branches only.
- One change set = one branch = one (or a few) well-explained commits.
  Commit messages: short summary line, then a body that says what changed
  and why, including any deliberate non-changes.

## Hosted authentication requirement (D82)

- Mike deferred login-flow/provider/MFA implementation and account-recovery UX
  work until wallet recovery, fighter ownership/transfer and full gym/fighter
  restoration are done. Keep the requirements below recorded for later. Existing
  fixture authentication may support core tests; do not turn it into real-login
  work or resume screen/navigation design before the final joint D74 review.
- Mike requires strict mandatory MFA for email/password login and wants passkey
  support (November 19). Record and preserve these release requirements in
  `HOSTED_AUTHENTICATION.md`; the current signed fixture buttons do not prove MFA.
- Preserve D72's same-wallet reset, revocation and hold. Password reset must not
  remove the second factor or create an email-only bypass. Mike requires a backup
  factor/code with no support bypass. Separate single-use login recovery codes
  remain proposed; never assume the 24-word wallet phrase is a login reset code.
- Passkey user verification and password-plus-authenticator are proposed paths.
  Freeze a separately versioned trusted assurance contract before implementing;
  do not reinterpret old fixture assertions, freshness timestamps or schemas as
  MFA evidence. Provider selection and final D74 screens remain separate work.

## Fighter transfer order and finality (D83)

- Mike approved public chain order and checkpoint finality on November 19.
  First valid competing send in canonical block/transaction order wins; later
  competing sends are void. Pending transfers do not change the effective owner.
  Ownership changes at a signed checkpoint plus sixty descendant blocks; noon
  locks use the owner of record at that time. Later checkpoint conflict retains
  official history and follows the community doctrine, never consensus veto.
- See `FIGHTER_TRANSFER_POLICY.md`. This closes the previously open timing/order
  question, not implementation. Preserve immutable birth, old event/result gym
  attribution, D27 ownership/roster distinction and D72 held-transfer refusal.
  The next usable milestone includes actual transfer and independently restored
  gym ownership; login work remains deferred and the D74 review remains last.

## Build and test

- Sets 48–49 candidate qualification uses the exact local Go 1.27.4 no-CSE
  profile in `build-profile.json`; see `BUILD_TOOLCHAIN.md`. Use
  `sh scripts/candidate-go.sh build ...` and `test ...` for that profile.
  Set 49's ten-module builds, scoped local regressions, actual normal-PoW
  composition and normal Mac package passed under their recorded profile/source
  bindings. Plain `go build`/`go test` and the labelled minimum-Go compatibility
  job remain separate diagnostics. Preserve the compiler failure and all failed
  attempts; do not weaken assertions or runtime limits. Qualification is finite
  candidate evidence, not an upstream fix or universal platform guarantee.
  Mac native/Linux cross-build evidence is not a local Docker build. Remote CI
  returned no implementation/release runs or checks; absence was assessed, not
  counted as a pass. Provider deployment is separately evidenced.
- Go workspace: `go.work` is committed and resolves all 10 modules to local
  sources. Always build/test from the repo root in workspace mode.
- Full build: `go build ./...` at root covers the root module only. Build
  every module: root, address, btcec, btcutil, chaincfg, chainhash, psbt,
  txscript, v2transport, wire. A quick all-module check:
  `for d in . address btcec btcutil chaincfg chainhash psbt txscript v2transport wire; do (cd $d && go build ./...) || break; done`
- `gofmt -l .` must be clean (excluding nothing; upstream is formatted).
- Test suites are SLOW under Argon2id (256 MB per block hash): btcutil a
  minute or two, database ~4.5 minutes, netsync full package ~6 minutes.
  Slow is normal; a suite that is still hashing at timeout is not the same
  as a failing assertion. Report which.
- Set 9 closed the blockchain/integration execution debt: complete tagged
  suites passed, with production vectors and smoke checked separately.
  Run `make test-chain` for both complete suites. The explicit `slugtestpow`
  build tag selects 64 KiB Argon2 memory and incompatible test identities;
  default/release builds retain 256 MiB. Never add that tag to release or
  Railway builds. Tagged daemons require regtest/simnet and use separate
  default files. No runtime flag enables cheap PoW.
- Set 7's signed checkpoint deliverable is complete: Mike's public key was
  enrolled in set 10 and his signed height-1 record is archived in set 11 at
  `checkpoints/phase-a/height-1.json`. Explicit local import reported finalized
  at tip 61 with 60 descendants. Mike authorized checkpoint-aware deployment
  as part of set 12. See `PHASE_A_CLOSEOUT.md` for the rollout and rollback
  configuration. Production genesis remains a separate explicit decision.

## Consensus facts (Phase A testnet, disposable but internally consistent)

- PoW: `wire/blockheader.go` `BlockHash()` is Argon2id over the 80-byte
  serialized header (header is both password and salt; time 1, memory
  256 MB, threads 1, 32-byte output). ONLY the header hash changed.
  Txids, merkle roots, checksums, and sighashes remain double-SHA256.
- Mainnet params were redefined in place as the SLUG network in
  `chaincfg`: subsidy 10 SLUG, halving every 1,050,000 blocks, 2-minute
  target spacing, retarget window 2,016 blocks, coinbase maturity 100,
  PowLimit 2^255−1, genesis bits 0x207fffff. Total issuance works out to
  20,999,999.8635 SLUG (integer-shift truncation, same as Bitcoin).
- Genesis: timestamp 2026-11-15 00:00:00 UTC, coinbase text "SLUG genesis,
  November 15 2026. Every fighter starts somewhere.", nonce 0, hash
  `7893676099cf9985bb198dfb6a347d9422a19f2422cf3326a999a7230ce246cd`,
  subsidy paid to an OP_RETURN burn output.
- Network: magic 0x534c5547 ("SLUG"), P2P port 29333, RPC port 29332,
  bech32 HRP "slug", P2PKH byte 0x53, P2SH 0x47, WIF 0xd3, HD private
  version 0x736c7567, HD public 0x534c5547. DNS seeds empty, Bitcoin
  checkpoints cleared.
- Do not change any consensus parameter or the genesis on an existing
  branch. Parameter changes are a new change set, and after the Phase C
  genesis they become coordinated upgrades, never silent edits.
- Mike approved D71 on November 16: implement, validate and deploy the existing
  BIP34/65/66, CSV, SegWit and Taproot rules at Phase-A height 62 in set 25.
  Preserve genesis and blocks 0–61. Confirm the seed still has the recorded
  height-61 tip before promotion; stop if it has advanced. Update nodes before
  further mining. This does not authorize public-chain mining or production
  genesis. After height 62, a binary downgrade is not a sufficient rollback.

## Test-vector rule (learned the hard way)

When adapting Bitcoin test vectors to SLUG encodings: decode the expected
string, swap ONLY the version bytes (or the bech32 HRP), re-encode with a
fresh checksum. Payloads (keys, hash160s, witness programs) stay
byte-identical to the canonical vectors. Never paste the code's own output
in as the expected value without independent verification (e.g. hash the
fixture header with `golang.org/x/crypto/argon2` directly in a throwaway
test). Exception already in tree: `TestCloneWithVersion` in hdkeychain
expects BITCOIN strings on purpose, because the test clones to explicitly
passed version bytes.

## Deliberate non-renames (change set 5)

Left as-is on purpose, do not "fix" without a change-set decision:
internal identifiers (btcdLog etc.), the "BTCD:" log subsystem tag, the
btcjson `"btcdconnected"` RPC notification method (wire protocol), and the
integration/rpctest harness's internal binary name. The Go module path
rename is change set 6 (HANDOVER.md has the spec).

## Deployment coupling (Railway seed node)

A public seed node runs this code on Railway (project
`successful-cooperation`, service `slug`). Phase A closeout switches its
source from `slug-rebrand` to **`master`**. Check the live Railway configuration
during the transition; afterward any push to master auto-deploys. Its start
command adds `--signedcheckpoint=/usr/share/slug/checkpoints/phase-a/height-1.json`
to `slugd --notls --rpcuser=u --rpcpass=p --datadir=/data`, with a
persistent volume at /data; public TCP endpoint
`junction.proxy.rlwy.net:16023` → port 29333 (RPC is not exposed).
Implications: never push a broken build to master; if a future
change set renames the binary again, the Railway start command must be
changed in the same deploy or the seed will not start. Deploying a
different branch means switching the service's source branch in Railway
settings. Preserve `slug-rebrand` at `0574b8b` as the rollback source;
rollback also removes the signed-checkpoint flag, which that older client
does not understand. Never detach, replace, or clear the /data volume.

## Signed checkpoint policy (set 7, Mike's decision)

- Signed checkpoints are report-only official league declarations. Conflicts
  are publicly reported; normal chain selection and validation continue.
  Never feed signed records into btcd's consensus checkpoints or fast-add
  shortcuts. Enforced finality requires a separate explicit decision.
- D62 burial means 60 descendant blocks, excluding the checkpoint block.
- Mike runs the offline key/signing tool himself. Never create, read, or
  request his operator private seed; only his public key is compiled into
  chaincfg. Test fixture keys are not operator keys. MainNetParams now uses
  his public key `811349556ea792060edbb0518dd3a513c5906abe495a81eaf629abd5900c44a3`.
  Other networks remain unconfigured. The first signed Phase A checkpoint
  is archived and verified; load it explicitly using `--signedcheckpoint`.
  Publishing the file does not automatically configure every node.

## Style

- Fighter DNA is a separate off-chain component in `fighter`; its exact
  versioned byte contract is in `FIGHTER_DNA.md`. Keep independent reference
  vectors independently derived, preserve old DNA versions, and do not treat
  a preview hash as qualifying work or an ownership claim. Complete fighter
  generation and registration are subsequent work, not node consensus.
- Prospect proofs live in `prospect` and use `PROSPECT_PROOFS.md`'s exact
  byte contract. Verification is conditional on separately trusted context;
  never accept claimant-chosen targets or mistake a proof for account
  authentication, registered ownership, discovery time or full block validity.
  Production proof verification must reject the cheap test PoW profile.
- The local `registry` is conditional evidence storage, not ownership or a
  birth clock. Preserve first evidence and stable IDs across restart and
  later status changes; never reset age by resubmission. Use a separate
  explicitly initialized directory, synchronous atomic writes, and fail
  closed on corruption. A write error has an uncertain commit outcome;
  reopen before retrying. See `PROSPECT_REGISTRY.md`.
- `prospectnode` resolves current-parent context from an explicitly trusted
  node and policy. Online verification rechecks the node after hashing;
  guarded admission checks before writes and duplicate acknowledgment.
  Never describe this observation as permanent canonicality or fighter birth.
  Never accept a claimant-selected policy or expose RPC credentials in output.
  See `TRUSTED_PROSPECT_CONTEXT.md`; stored receipts remain supplied-context.

- `gymauth` binds self-certifying gym IDs to wallet-compatible secp256k1 keys
  and signs only a specific prospect admission. Signing is automatic client
  work, never a player prospect-selection step. CLI tools accept public inputs
  only; use fixture keys for tests, never real wallet or operator keys. Explicit
  schema-2 registries require and atomically persist authorization; do not
  upgrade schema-1 evidence or arbitrary gym IDs implicitly. Key control does
  not prove an account, ownership transfer, birth, or permanent canonicality.
  See `GYM_AUTHORIZATION.md` for exact bytes and integration boundaries.

- D68 jobs last exactly five minutes. Age starts at durable issuance; a new
  admission at or after expiry is rejected without grace. Job IDs/challenges
  enter proof-v2 work before hashing. Only explicitly created schema-3 or schema-4 stores
  issue and admit jobs; no retrospective birth is attached to older evidence.
  Keep accepted birth immutable across duplicate recovery, expiry and reorgs.
  Historical receipts do not assert renewed eligibility. Persist clock high
  water even for expiry rejections, fail closed on rollback/uncertain writes,
  and never accept claimant time or nonce on authority APIs. Client handling is
  automatic under D44. See `PROSPECT_BIRTH.md` and spec decision D68.

- `prospectservice` is a loopback-only authority boundary with one active
  operation, bounded connections/bytes/rates and durable configured capacities.
  API credentials bind access to a public gym key but never replace admission
  signatures. Preserve historical receipts after uncertain acknowledgments,
  drain real work before closing storage and never reopen a poisoned handle
  automatically. `slugregistryd` is not started by seed deployment. See
  `PROSPECT_SERVICE.md`. Protocol v1 requires schema 3; v2 requires explicit
  schema 4. Never silently change old eligibility or present context-only jobs
  as full templates.

- D69 broadcasts block-target wins immediately, before prospect signing or
  admission. Only the exact winning block on the canonical chain can use the
  stale-parent exception, within the original D68 lifetime. Authority guards
  take internally verified work, never claimant-provided hash summaries. Keep
  admission-time eligibility immutable with birth. The worker synchronizes
  found work before transport, and public authorization before admission;
  uncertain outcomes retain identical evidence for recovery. Retained-capacity
  exhaustion pauses mining, never drops pending work or receipts automatically.

- The desktop pilot has its own module/workspace under `desktop`; keep the
  ten-module chain workspace and Railway node build independent. Its fixture
  gym mines only a disposable loopback chain, with no production wallet or
  public-node option. Do not describe public receipt export as wallet export
  or hosted login. Mining requires an explicit Start; Quit drains owned work
  and processes. Real wallet custody, D48 export, calibrated policy and release
  signing remain separate integration gates. Never read operator/wallet keys.
  Set 26 uses a separate `LocalWalletPilot` v2 namespace and the published
  zero256 wallet fixture. Ordinary address/balance/history operations use actual
  btcwallet RPC with an owned minimal allowlist; signing/export stay inside the
  helper. Native dialogs select recovery paths, never the webview. Restore
  verification is asynchronous and excludes mining. Preserve old `LocalPilot`
  data; never adopt pre-upgrade cached chains or substitute a new wallet/gym
  when established data is missing. Runtime checks require activation height62,
  since the node's unchanged version string is insufficient. The explicit
  `--localwalletpilot` node option is restricted to connect-only literal
  loopback networking and RPC; it changes only old-tip template readiness.
  Wallet rescan readiness has the same private-pilot boundary. Never enable
  either path for the seed or replace the worker/job/outbox flow with hidden
  bootstrap mining. All consensus/proposal/PoW checks still apply.
  Mike confirmed the gym shell is new Go work against the slugd/btcwallet RPC
  surfaces, not a Paymetheus adaptation. Fork the separate btcwallet repository
  when its wallet workstream begins.
  That fork now exists at `taytr-llc/slugwallet`, checked out beside this repo.
  Its first set 23 branch starts from btcwallet v0.18.1. Keep node deployment
  separate from wallet work; no wallet service starts on Railway with this set.
  Mike approved D70: 24 English BIP39 words, no extra BIP39 passphrase, with
  versioned SLUG wallet/gym recovery metadata. Login/encryption passwords remain
  separate. New recovery profiles use explicit standard BIP32 derivation;
  existing wallets keep legacy derivation. The Phase-A fixture profile is not a
  production path allocation. Verify actual later export and fresh restore,
  never invent a phrase for an old raw-seed wallet or imply hosted login exists.
  Mike approved D72 on November 16: a verified password reset restores access
  to the same hosted wallet without the 24 words, with a post-reset security
  hold. Record this as closing D70's hosted-reset authority question. The
  service retains recovery authority; backup export does not revoke it. For
  24 hours after each verified reset, block withdrawals, fighter transfers,
  recovery phrase/file export, recovery-authority changes, marketplace purchases
  and listings. Viewing and mining into the same wallet remain available.
  Revoke existing sessions immediately. A distinct further reset restarts the
  hold; replaying the same verified event must not extend it.
  Set 27 starts with two explicitly published fixtures, not real registration,
  production derivation allocation, or a public wallet service.
  Set 28 adds an explicit schema-2 assertion/reset boundary. Preserve schema-1
  data and APIs; never allow legacy login/reset to bypass schema 2. Pin verifier
  trust, consume authority-issued nonces, and bind the signed credential epoch
  to the original authentication. Refresh issuance time is not fresh auth. A
  verified reset must establish its durable barrier before provider credential
  completion; pending state never clears merely on timeout or restart. Real
  provider deployment, arbitrary signup and production custody remain separate.
  Set 29 retains recovery and gym-signing material in separate purpose-bound
  envelopes in the wallet repository. Held gym operations must never unwrap
  recovery material or export a seed, scalar or data key. Full provisioning
  requires fresh authentication with no active hold; interrupted reservations
  keep their original identity. Keep provider I/O outside account/vault leases
  and reauthorize the original token/generation at publication. The initial
  fixture manifest is immutable, not a current address-cursor backup. There is
  no production KMS adapter, new-device wallet UI or real key issuance in this set.
  Mike approved D73 on November 16: a separate production recovery profile with
  dedicated SLUG coin type, four payment scopes, native84 receive/change defaults
  and a permanent-chain-bound gym key. Preserve all existing fixture and legacy
  paths/schemas. The exact proposed contract is in PRODUCTION_WALLET_PROFILE.md.
  Final coin allocation, descriptor bytes and permanent genesis remain issuance
  gates; a caller-provided finalized flag cannot bypass them. Use only explicit
  public fixtures until those gates are settled. Approval does not provision
  provider/KMS resources or authorize reading/creating real wallet keys here.

- Keep diffs minimal and close to upstream btcd structure; this fork tracks
  upstream and every gratuitous divergence makes merges harder.
- Where behavior intentionally differs from Bitcoin, leave a short comment
  saying so and why.
- Write a `CHANGESET_<n>_NOTES.md` per change set (format: what changed,
  what was deliberately not changed, how it was verified, known debt).

- Set 31's `gymprovider` accepts only the pinned published fixture issuer/key.
  Keep original-authentication epochs and provider-operation identities immutable.
  A reset may allow provider credential completion only after a synced, exact
  pending authority barrier; historical completion never grants another permit.
  Do not invent session replay after a lost acknowledgement or unlock abandoned
  pending resets on timeout. Keep JWTs/session tokens out of the journal. Real
  provider guarantees and product login integration require separate evidence;
  the Auth0-shaped fixture is not a deployed adapter. See `GYM_PROVIDER.md`.

- Set 32 uses explicit candidate wallet schema 11; never silently migrate
  schemas 8/9/10 or enable real issuance. Pending sessions exclusively own
  storage and cannot sign or spend. Preserve issued cursors across cancellation,
  restart, notification failure and reorg; unused lookahead is not issuance.
  Keep the aggregate 65,536-attempt quota and bounded node/watch requests.
  `RESUMABLE_RECOVERY.md` defines the scanner and ready-wallet limits; the
  fixture vault still does not track an evolving wallet snapshot.

- Set 33's `fightermanifest` preserves complete DNA v1 while adding versioned
  candidate names, physical traits, style and portrait selections. Keep the
  frozen tables, strict canonical codec and independently derived fixtures.
  A preview is not an official appearance assignment or fighter identity;
  current jobs bind only DNA version. Official assignment requires explicit
  issuance/version binding or an immutable committed mapping. Never change
  historical receipts or birth, or replace the existing DNA CLI route.

- Set 34's `fighterportrait` validates the complete manifest before drawing.
  Preserve exact versioned SVG bytes, pinned tables and independent reference
  fixtures. No uploaded markup, external assets, free text or mutable palettes
  enter rendering. Byte determinism does not imply cross-browser pixel identity
  or screen-reader verification. Portrait previews still carry no official
  assignment, identity, ownership or birth claim.

- Set 35's `fightercondition` is a pure projection of complete canonical DNA,
  supplied birth/time and already-accounted XP. It authenticates none of those
  external facts and grants no admission, ownership or roster eligibility.
  Keep exact duration weeks, nearest-even integer tenths, no added attribute
  floor, XP after aging and the D65 rating without training/form. The caller's
  later trusted event boundary must supply immutable birth and XP.

- D75 (Mike, November 17) keeps D65 unchanged and rescinds set 42's exact
  rounded-coefficient equality gate. Preserve the failed original report and
  contract; do not relabel them passed. Rating replacements require a positive
  paired bootstrap interval excluding zero and population validation covering
  prime, aging and XP-varied fighters, including close pairings. The incumbent
  wins ties or inconclusive comparisons. Run and publish coefficient-refitting
  bootstrap and population-coverage studies on the unchanged set 42 engine.
  These studies do not authorize new weights, condition v2, historical rewrites
  or gameplay rebalancing. See `RATING_VALIDATION_POLICY.md`. Other engineering
  and D74 product-release gates remain in force.
  The completed independent studies are in `RATING_VALIDATION_REPORT.md`:
  all eight primary paired improvement intervals include zero. Preserve both
  that evidence and the original failed equality result; D65 remains unchanged.

- D76 (Mike, November 17) freezes roster membership, ownership, eligibility
  and current condition at Saturday noon Pacific. Later roster changes,
  transfers or retirement do not alter that event. Keep birth/live aging and
  D39's immediate title vacancy intact. Candidate assignment is not proof of
  transferable production ownership; official snapshots need that authority.
- D77 (Mike, November 17) waits with visible delay/conflict, never substitutes
  a seed and never automatically cancels the event. If the required seed is
  unfinalized at the following Saturday noon-Pacific lock, escalate to the
  operator as a chain incident under checkpoint/community policy. Use calendar
  boundaries through DST, preserve original event/seed evidence, and keep node
  checkpoints report-only. No synthetic finalized flag or preview seed can
  stand in for the later trusted seed/finality authority. See EVENT_LOCK_POLICY.md.

- Candidate-card exception C1 (Mike, November 17) permits the existing two-gym,
  zero-professional-record candidate to book same-class opposing fighters in
  the inclusive half-to-double D65 band with fewer than five opponents. Waive
  only widen-to-five, not the band. Retain unmatched counts and per-fighter
  reasons. This does not amend D13/D28/D52 or settle general widening, record
  filters or next-week priority. See CANDIDATE_CARD_EXCEPTION.md.

- D78 (Mike, November 18) makes professional records, XP and all result-derived
  ranking points/title changes effective exactly once at the result's own signed
  checkpoint plus 60 descendant blocks. Label every provisional outcome as
  provisional wherever displayed, including fighter pages and rankings; no
  provisional award affects effective condition or standings. Retain finalized
  awards after a later reported chain conflict; report it under D62 without
  automatic rollback or consensus enforcement. Noon snapshots use only awards
  already effective at lock and never change retrospectively. Preserve D39's
  independent retirement/time-driven vacancy rules, D65 and C1's zero-record
  boundary. Do not infer transferable ownership, production authority or new
  matchmaking/title-selection policy from this timing decision.

- D79 (Mike, November 18) uses difficulty-weighted per-block pool rounds,
  including the winning share, with daily settlement only. Earnings lock at
  sixty descendants; payment waits for the funding coinbase's 100-block
  consensus maturity. Never advance pool reserves. Apply the 1% fee once,
  pay balances of at least 1 SLUG and carry the rest. A later loss of the
  funding coinbase blocks payment without erasing locked earnings. Resolve
  incident policy separately for conflicted funding;
  see `POOL_ACCOUNTING_POLICY.md`. Work classification alone is not credit.
- D80 (Mike, November 18) includes an expired-job winning share only when its
  exact block becomes canonical and pays the configured pool address. Count
  that share and distribute its block reward under D79; reject ordinary late
  shares. Never grant fighter admission or birth from the expired job. A node
  submission acknowledgment is not canonical membership, and observed
  canonicality is not irreversible finality.
- D81 (Mike) keeps an orphaned pool round's shares fixed, with no earnings
  before confirmation while its block is orphaned. The exact block's return
  resumes the same round; never reuse shares in another round. Preserve D79's
  already-confirmed earnings through later conflict and never advance reserves.

- Mike completed D58's settlement details on November 18: preserve exact
  fractional net balances, pay whole atoms at the 1-SLUG threshold, and batch
  the daily payments into one transaction. Network fees come only from the
  pool's earned 1%; defer visibly rather than surcharge miners or drop their
  change into fees. Pin recipient authority, retain original matured funding
  provenance and persist the exact signed transaction and all reservations
  before broadcast. Retry only those bytes; unknown delivery never releases
  funds or creates another payment. Do not label this a new daily-payout rule.
  Real wallet custody and product release gates remain separate.

- Set 46 retains immutable candidate cards in a separate explicit journal.
  Preserve the complete noon source, fixed seed, pairings, unmatched reasons and
  original creation time across replay, conflict and restart. Source health is
  separate from saved evidence; a caller-supplied source or finalized flag cannot
  authorize booking. Shared event/seed/card stores belong to the candidate parent,
  not the current login. Never infer professional record authority from XP or
  award results/training from a card. See EVENT_CARDS.md.

- Set 47's saved candidate bout transcripts grant no professional/exhibition
  records, XP, training, title or ownership. Preserve the booked noon inputs,
  fixed seed and complete first result across replay; report current source
  conflicts separately without rewriting history. See EVENT_BOUTS.md.

- Set 39 adds explicitly pinned candidate service protocol 3 and a separately
  typed worker/outbox version 2. Preserve old services, workers and journals;
  never infer trust from a marker or silently migrate old evidence. Bind the
  outbox to origin, full authority pin and transport trust identity without
  persisting credentials. Found work, authorization and receipts synchronize
  before their respective transport/acknowledgment boundaries. Reserve remaining
  record growth so full capacity cannot prevent recovery of retained work.
  Cancellation joins real work; historical recovery still requires a live
  account session and preserves original birth/binding. See MANIFEST_MINING.md
  and its separate reservation addendum. No official descriptor activation,
  ownership authority, public receipt import or default desktop route is implied.

- Set 36 separates structural appearance descriptors from compiled candidate
  support and production activation. Production resolution has no entries.
  Keep Job v1 and its readers unchanged; the separate ManifestJob v2 binds
  an authority namespace and exact descriptor before work. Neither field
  authenticates issuance. BuildManifest is a typed unmined-candidate helper,
  not an admission path. No schema migration, current-service activation or
  historical appearance assignment is implied. See MANIFEST_JOBS.md.

- Set 37's separate ManifestStore requires an explicit schema-5 authority pin.
  Unsigned marker inspection supports provisioning recovery; it does not make
  its output trusted configuration. Never migrate schemas 1–4 or accept caller
  receipt JSON as authoritative. Generate the static binding from internally
  verified work and commit it with original birth/eligibility in one synced
  batch. Historical reads retain their original facts through expiry/reorgs;
  they do not establish current ownership. Production activation stays closed.
  See MANIFEST_ADMISSION.md for strict bounds, poisoning and read guarantees.

- Set 38 separates serving known headers from deciding that the chain is fresh.
  Keep the existing bounded LocateHeaders semantics, including the special
  empty-locator indexed-header lookup. Serving headers is not a claim that a
  node is current or that every indexed header has a validated block body.
  Do not relax chain currentness, mining templates, transaction relay, compact
  filters, proof verification or consensus to make an idle seed synchronizable.

- Set 40 adds a separate Candidate Pilot bundle and explicit helper mode with
  its own LocalManifestPilot namespace. Preserve all earlier app/data routes.
  Login cold-verifies at most eight retained records while stopped; only Start
  mines. Live cards follow fully verified, synchronized acceptance, never a
  claimant receipt or an uncommitted response. View credentials and generation
  remain bound to the original login through cache reads, portraits and exports.
  Never substitute a new account session during a pending operation. Public
  receipt export remains distinct from recovery export and uses an exclusive
  native-selected destination. See MANIFEST_DESKTOP.md and its packaging and
  acceptance addenda. The candidate build does not activate an official
  descriptor, ownership, real accounts, product distribution or D74 approval.

- Set 41 consumes verified records from one concrete pinned ManifestStore into
  a separate candidate assignment authority. Assignment, consumed sequence and
  clock high water synchronize together; roster membership never controls
  inventory retention. Ten reversible slots use durable prepare/apply operation
  identities and revision checks. Preserve original birth and XP=0 until an
  explicit trusted results authority exists. Current held sessions may make
  reversible roster edits; D72's asset/recovery/market restrictions stay intact.
  Keep bounded serial verification, original account leases, uncertain-outcome
  reconciliation and fixed-page versus Follow Recent behavior. The existing
  candidate bundle uses the same account root with a recoverable explicit v2
  upgrade; missing established components fail closed. No public receipt import,
  transfer, discard, entry lock, training, official descriptor or real account
  is enabled. See GYM_STABLE.md and its engineering addenda.
