# SLUG launch readiness

Updated November 17, 2026. Numbered change sets are reviewable engineering
increments, not a fixed countdown to launch. This checklist tracks the actual
finish line and must be updated from evidence rather than a branch number.

**Required design review with Mike:** before the friends/tester or public
product release, hold a thorough joint design review and address or explicitly
agree to defer its findings. Mike requested this gate on November 16 (D74).
Mike wants one full joint design review once all major flows are ready, at the
end before release. Partial reviews do not replace that session.

Routine authorized engineering pushes and Phase-A seed verification continue;
they do not count as completion of this product-release gate.

## Current evidence

- Chain core, production-PoW tests and report-only signed checkpoints are built.
  The live seed is still disposable Phase A at the recorded height 61, with
  existing script upgrades activated from block 62. Permanent genesis has not
  been approved or launched.
- The Mac Account Pilot builds and exercises two published fixture wallets on
  disposable loopback chains. It is ad-hoc signed, not notarized. It is not
  real signup, unique player wallets or a production custody deployment.
- Sets 27–28 validate account enrollment, the complete approved 24-hour reset
  hold, signed identity assertions and durable reset ordering. Both seed
  deployments passed their live observation.
- Set 29's purpose-separated fixture vault, race/crash gates, frozen normal-PoW
  acceptance and packaged helper tests pass. Its exact wallet CI, seed deployment
  and 15-minute live observation are complete.
- D73 approves the production-profile design. Set 30 implements it with
  explicit public candidate fixtures while final allocation/genesis and real
  issuance remain gated. Set 31's fixture provider coordination, independent
  vectors, crash/replay and two-wallet normal-PoW acceptance pass locally.
  Sets 30 and 31 are deployed and both full observations passed;
  real-provider integration remains a launch gate.
- Set 32 implements bounded resumable candidate recovery with explicit
  schema 11, high-index address metadata and a separately verified full-node
  scanner. Frozen normal-PoW file-only restore/spend, full normal module tests
  and maximum watch measurements pass, including the complete final scanner
  race and process-kill suite. The packaged Mac pilot and per-set normal-PoW
  smoke also pass. Full remote wallet CI passed, and set 32 deployed successfully;
  its full 15-minute live observation passed. See `RESUMABLE_RECOVERY.md`.
- Set 33 adds independently checked candidate fighter manifests, including
  names, physical traits, styles and portrait selections. The original DNA
  preview stays compatible. These are offline previews; official assignment,
  lifecycle integration and fight gameplay remain open.
- Set 34 implements the candidate portrait renderer and offline CLI with exact
  independent SVG fixtures and bounded markup. A local visual gallery is ready;
  browser policy blocked local-file preview, so visual and actual screen-reader
  checks remain open; Mike chose to keep the rendering check pending. The
  opt-in candidate desktop integration is recorded below.
- Set 35 provides exact candidate aging/current-condition and D65 rating
  projections. It preserves original DNA, uses supplied birth/time/XP and
  does not authenticate those facts or establish roster eligibility. Trusted
  admission/history and current ownership remain required.
- Sets 33–35 are deployed as one engineering batch of offline candidate tools;
  each passed its own normal-PoW smoke. The full shared live observation passed.
  The Mac GUI remains the set32 fixture pilot and portrait review stays pending.
- Set 36 binds the exact candidate static appearance descriptor into a separate
  job version and typed payable-candidate builder. Production activation has
  no entries. Durable authority issuance/admission and trusted historical reads
  must still connect this binding to an accepted fighter.
- Set 36 is deployed and its full observation passed. Set 37 has implemented
  a separate pinned candidate authority that atomically stores immutable birth,
  eligibility and appearance hashes. Verified local reads remain distinct from
  public authenticated publication, current ownership and roster integration.
- Set 37 deployed, but its fresh observer exposed the inherited 24-hour idle-tip
  header-serving gate immediately after the unchanged tip aged out. Local
  reproduction and a separate read-only P2P inventory/block check confirmed
  the diagnosis and unchanged height61 hash. Set38 repairs this serving gate;
  fresh public sync, required smoke and the full combined 37–38 observation
  passed. No public block was mined.
- Set 39 connects candidate authority jobs and receipts to a pinned service and
  typed durable miner. Independent service/worker reviews and the two-wallet
  normal-PoW composition pass, including lost acknowledgments, reset-held mining
  and exact historical recovery after real expiry with nodes offline. Final
  integrated gates, seed deployment and full 15-minute observation passed. The
  default Mac GUI still uses the existing fixture pilot.
- Set 40 adds a separate Candidate Pilot bundle with automatic candidate cards
  and exact deterministic portraits after explicit Start. It uses two published
  fixture wallets and a separate data namespace, with typed public receipt
  export and bounded cold history verification. Source reviews and preliminary
  two-wallet normal-PoW composition pass. The exact package, integrated gates
  and full deployment observation passed. No window or native chooser
  rendering is claimed by helper/DOM tests. Mike's rendering check, actual
  screen-reader acceptance and D74 review remain pending.
- Set 41 implements a durable candidate inventory, trusted current condition
  and ten reversible roster slots in the same Candidate Pilot. Core, DOM and
  development helper checks pass, including both retained fixture wallets,
  roster replay/reopen, reset-held actions and process cleanup. Final integrated
  gates, exact packaged acceptance, seed deployment and the full 15-minute
  observation passed. Entry locking, fights, results/XP and transferable ownership
  remain open; this work does not close visual acceptance or D74.
- Set 42's isolated implementation adds the deterministic engine and unrecorded
  same-gym preview. Independent 55-bout conformance, core/helper, native and DOM
  checks pass. All ten module builds, full normal/cheap-PoW desktop tests,
  desktop vet, 111 frontend tests and formatting pass. The packaged integration
  harness compiles. Its first run failed during login when sandbox-denied
  loopback allocation exposed a typed-nil cleanup crash. The narrow startup
  repair passes focused and full account-component normal/race/tagged checks;
  fresh exact packaged acceptance passed with both fixture wallets and six
  independently checked previews. Calibration simulated 9,660
  fixed inputs; its rounded refit is
  [0.9, 2.5, 0.2, 0.6, 0.8], differing from D65. The exact-match release gate
  failed while the other numerical gates passed. Mike's D75 decision retains
  D65 and rescinds that exact-equality rule, preserving its original failed
  report. Future rating changes require positive paired-bootstrap evidence
  validated on prime, aging and XP-varied populations including close pairs;
  the incumbent wins ties. The 1,000 coefficient refits and 20,000 fresh matchup
  families passed independent audit. All eight primary paired improvement
  intervals include zero; the unchanged engine with D65 has explicit D75
  successor acceptance. Set 42 is deployed at master `fc672113`; its full
  15-minute live observation and restart passed. No remote CI run/check was
  available for the exact commit, so only the captured local tests are claimed.
  Condition v1, authority pins and saved rosters are unchanged.
  See `RATING_VALIDATION_POLICY.md` and `RATING_VALIDATION_REPORT.md`.
- Set 43 is implemented on `slug-saturday-entry`: reversible Saturday entry
  selection, save/replace/withdraw, history and cutoff reporting in the same
  candidate gym. The independent corpus, core/desktop tests, crash recovery
  and exact two-wallet Mac bundle flow pass. Deployment at master `e2761812`,
  the complete 15-minute live observation and clean restart passed. Event
  locking, future seed selection, matching and
  results/XP remain separate work; no official league event is enabled yet.
- Set 44 implements D76/D77's candidate foundation: noon-fixed roster/ownership/
  eligibility/condition; no seed substitution or automatic cancellation; and
  escalation at the following Saturday lock when the required seed remains
  unfinalized. Independent-reference, normal/race, crash-recovery and desktop
  composition checks have passed. The exact Mac bundle, independent evidence
  checks and deployment at master `1fc307e0` passed, followed by the complete
  15-minute live observation and restart. Closed
  desktop captures use the explicitly tagged test clock with normal PoW;
  successful real-clock closed-bundle coverage awaits an eligible Saturday.
  See `EVENT_SNAPSHOTS.md`.
  Its fixture assignment evidence cannot substitute for production ownership
  history, and its calendar threshold cannot substitute for chain finality.
  See `EVENT_LOCK_POLICY.md`; the D74 review remains pending.
- Set 45's candidate publication/seed journal, owned-node adapter and desktop
  controls are implemented on `slug-event-seed`, with frozen independent
  reference comparisons and actual normal-node composition passing, including
  retained finality/conflict, restart and next-Saturday escalation. Final source
  checks pass; packaged/deployment acceptance is still in progress. This is
  candidate engineering work, not an official event service. See `EVENT_SEEDS.md`.

The missing simulator and prototype sources arrived in `../slug-sim-export`
on November 16. `README_HANDOFF.md` records their provenance; workspace
`sim-handoff-intake.json` preserves archive and file checksums. They are reference
implementations and first-pass mockups. The canonical spec takes precedence,
and their intake does not replace the D74 review of the working product.

## Release milestones

| Milestone | Remaining acceptance |
| --- | --- |
| Production wallet and recovery | Freeze the approved profile and exact chain/coin identity; prove actual export, independent fresh restore and spending. Complete authoritative address metadata and bounded resumable scanning before claiming full evolving-wallet recovery. Real entropy and custody must use the operator-owned runtime, never assistant-visible keys. |
| Real accounts and hosted custody | Demonstrate provider authentication/reset ordering, original-authentication epochs, durable retry/reconciliation and abandoned-flow behavior. Provision the reviewed operator-owned provider/vault configuration; prove same-wallet recovery, hold enforcement, backups, key rotation and disaster recovery. |
| Desktop product | Integrate real signup/login, wallet views and controlled mining with the service. Complete native error/recovery flows, cross-device behavior, macOS signing/notarization and Windows runtime acceptance. Review distribution/update behavior. |
| Mining and fighter services | Complete the official pool/share validation and payout path in the build plan, authoritative fighter lifecycle/ownership and portable history. Preserve automatic jobs, birth, canonical-winning-block rules and report-only checkpoint doctrine. |
| League, marketplace and presentation | Wire the fight/league operation, entries, matching, results/finality, rankings and required gym/fighter flows. Complete the agreed launch marketplace/transfer behavior and reset-hold restrictions. Prototype/simulation output does not substitute for deployed acceptance. |
| Thorough design review with Mike (D74) | Review a working representative build together: onboarding, mining controls/status, wallet/recovery/reset holds, roster/fighter lifecycle, fights/league, marketplace/transfers, navigation, visual hierarchy, accessibility, loading/empty/error/offline states and trust/irreversible-action wording. Record findings and agreed changes. Recheck affected flows before product release. |
| Launch hardening and operations | Run end-to-end rehearsals and security review of the final system; verify capacity, monitoring, backups/recovery and rollback procedures. Complete the launch legal/operational items in the spec, including any external work requiring Mike's accounts or decisions. |
| Permanent genesis and coordinated launch | Mike approves the exact permanent genesis and final parameters. Freeze release sources/configuration and deploy compatible nodes/services/clients. Rehearse the genesis procedure and verify the first permanent history. D1/D2's public-from-genesis and permanent tester-history commitments remain in force. |

## Review session preparation

Prepare a working build, a short route through the major player journeys, and
examples of failure/recovery states. Bring a concise list of known design gaps
and unresolved choices. Mike and Codex review together; a solo code audit or
automated accessibility check does not replace this session. Capture outcomes
in a dated review record with owners and validation for each required change.

The D75 studies add two concrete league-design review topics: all 4,000 sampled
initial-band-edge favorites won, and the set 42 engine produces fewer draws
than the original simulator under its different score-tie rule. Review those
findings as matchmaking/gameplay questions; changing a summary rating alone
does not change a fight. Neither finding authorizes a policy or engine change.

The approved logo handoff is preserved byte-for-byte in the workspace's
`../brand-assets`: gray background, white front-facing fist, black linework and
SLUG across the knuckles, plus the transparent variant. Use those exact assets
in the forthcoming desktop design work rather than recreating the mark. Source
paths and checksums are recorded in `../brand-assets/handoff.json`.

Do not invent a calendar date or claim an automatic reminder has been scheduled.
Arrange the session when a representative build is ready, before user-facing
release. Any later scope change must keep this gate visible.
