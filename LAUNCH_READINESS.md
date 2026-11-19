# SLUG launch readiness

Updated November 18, 2026. Numbered change sets are reviewable engineering
increments, not a fixed countdown to launch. This checklist tracks the actual
finish line and must be updated from evidence rather than a branch number.

**Required design review with Mike:** before the friends/tester or public
product release, hold a thorough joint design review and address or explicitly
agree to defer its findings. Mike requested this gate on November 16 (D74).
Mike wants one full joint design review once all major flows are ready, at the
end before release. Partial reviews do not replace that session.
On November 18 Mike clarified that app organization and navigation changes
belong in that final session. The external navigation draft is stopped and
unapplied; it is not the next engineering set.

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
  retained finality/conflict, restart and next-Saturday escalation. Source checks,
  the exact original-icon package at `ca84a62d`, independent evidence checks and
  deployment at master `dcc44829` passed, followed by the complete 15-minute
  observation and clean restart. The normal-clock package covers setup, refusals
  and recovery; successful closed publication/finality uses separate scoped-clock
  normal-PoW evidence. GUI/icon rendering, accessibility, portrait review and the
  full joint D74 review remain open. This is candidate engineering work, not an
  official event service. See `CHANGESET_45_NOTES.md` and `EVENT_SEEDS.md`.
- Set 46 is deployed: candidate booked cards with unmatched counts/reasons
  under the scoped C1 two-gym exception. This does not amend league matchmaking
  or establish professional results/awards. The implementation and first actual
  normal-PoW scoped-clock run produced one bout from three entrants, with one
  unmatched fighter and its reason. Independent arithmetic and frontend checks
  pass. Final implementation `5e6b36d3`, its corrected normal-clock Mac package
  and deployment at master `5c11cbaf` passed, followed by 900 seconds of live
  observation, 31 samples and a clean restart at the unchanged height-61 tip.
  Successful closed-event booking uses separate scoped-clock normal-PoW evidence;
  the normal-clock package covers setup, open/pending/refusal and recovery flows.
  GitHub returned no runs/checks, so remote CI is not claimed passed. Earlier
  failed package/race evidence remains preserved. See
  `CANDIDATE_CARD_EXCEPTION.md`, `EVENT_CARDS.md` and `CHANGESET_46_NOTES.md`.
- Set 47 implements saved candidate bouts and immutable replay. Implementation
  checks and independent verification of the actual native first Run and
  historical conflict replay pass. The scoped-clock, normal-PoW test prepares
  its seed through real authenticated core actions without automatic admissions.
  Final normal-clock app packaging and seven independent package reports pass.
  Release `ba218f55` is deployed; the full 900-second observation, 31 samples,
  chain verification and clean restart pass. Railway configuration is unchanged.
  Remote CI returned no runs/checks and is not claimed passed;
  GUI/portrait/native chooser/accessibility and the joint D74 review stay open.
  Candidate transcripts grant no professional/exhibition records, XP, awards,
  titles or ownership. See `EVENT_BOUTS.md` and `CHANGESET_47_NOTES.md`.


- Set 48 completed candidate result publication, independent result finality
  and public proof export on `slug-result-publication`. Implementation
  `de4c1497` passed the exact-profile local and normal Mac package gates and
  was released on master at `cc4f41e4`. The final provider closeout and isolated
  900.0553332919953-second, 31-sample checkpoint observer
  passed, including restart and clean owned shutdown. Service configuration and
  volume settings stayed unchanged. `CHANGESET_48_NOTES.md` records the exact
  evidence and scope; this engineering deployment is not product-launch approval.
  Build qualification found a Go compiler lifetime
  defect in the original application regression. The documented unoptimized
  build option passes that test ten times and keeps full-capacity selected
  metadata below 500 ms, but fails the existing 25-second startup deadline.
  That option is not release-qualified. Disabling the two CSE passes instead
  passes the original regression ten times, selected metadata at 228–241 ms
  and full startup at 11.299 seconds in the preliminary diagnostic. Full core normal, scoped-clock, race and
  GC-pressure checks pass; a three-literal vet correction has independent
  equivalence review and passing final-source follow-up checks. Final-profile
  desktop normal/race, adapter, all-module, capacity and installer crash checks
  pass; selected metadata took 228–242 ms and full parent startup took 11.16
  seconds. The fresh actual run passed in 286.793 seconds with six proof exports,
  zero new admissions and graceful cleanup; independent verification checked
  279 unique proof headers and all twelve altered-evidence controls were refused.
  The temporary build profile is qualified for this recorded candidate scope;
  no upstream compiler fix or universal platform guarantee is claimed. Preserve
  the failed-build and timing evidence, the two original test-assertion findings
  and the forced-invalidation restart limitation. The two assertions were
  tightened after closeout and passed their affected normal/race suites without
  changing shipping code. Normal-calendar package acceptance
  does not establish a successful closed-event publication. D65, C1, production
  authority, permanent genesis, portrait/GUI/accessibility and D74 gates remain
  unchanged. This work adds no professional records, XP or awards.

- Set 49 completed on `slug-career-awards`. Mike approved D78 on November 18:
  permanent records/XP and all result-derived points/title changes take effect
  exactly once at result finality. Provisional outcomes stay labeled; later
  conflicts retain awards; noon snapshots use only awards effective at lock.
  Implementation `5b590f40` passed local gates and actual scoped-clock normal-PoW
  acceptance: one award event, twelve Career observations, six independent proof
  checks, and twenty-one actual-evidence corruption controls. The normal-calendar
  Mac package and nine independent package reports separately passed activation,
  zero-award current pages, held/refused views and restart; they do not establish
  real-calendar finalized awards. Release `0a4374de` deployed successfully and
  passed 900.03260950002 seconds of observation with 31 samples at unchanged
  height 61, sixty descendants and one peer, chain verification and clean restart.
  Railway configuration and volume remain unchanged. Remote CI returned no
  runs/checks and is assessed rather than claimed passed. Standard records,
  capped XP and base points are implemented; complete rankings/title allocation,
  production authority, portrait/GUI/accessibility and D74 remain open. C1 and
  D65 are unchanged. See `CHANGESET_49_NOTES.md` and `RESULT_AWARD_POLICY.md`.

- Set 50 implementation `99d36ef2` is released as `592e143c` on the seed. Its
  additive practice path captures finalized XP with original DNA and birth,
  retains that captured result across later awards, and grants no records,
  XP, points or titles. All 249 frontend tests, independent arithmetic and
  three scoped awarded-fighter jobs/two distinct compositions pass. The normal
  Mac package passed five XP0 jobs/ten typed portraits and all ten independent
  checks: nine passed reports were hash-reused after a corrected tenth checker.
  The initial missing-snapshot producer failure and checker-envelope failure
  remain preserved. The live observation and provider closeout passed: 900.03
  seconds, 31 successful samples at height 61 with 60 checkpoint descendants,
  and a clean restart.
  Railway remained healthy on the exact release, with configuration, volume
  and TCP endpoint unchanged. Remote CI absence is assessed, not
  passed. The legacy XP0 preview, fight engine, D65, C1 and D78 are unchanged.
  One local signed-out spot check showed the detailed header logo and one
  running instance. Mike explicitly confirmed the menu-bar fist visible after
  its already-on macOS Allow in Menu Bar setting was toggled off/on. No source
  fix or established root cause is claimed. This narrow logo/tray observation
  does not close full GUI, native chooser, screen-reader or D74 acceptance.
  See `CAREER_PREVIEW.md` and `CHANGESET_50_NOTES.md`; production authority
  and the remaining product gates stay open.

- Set 51 implementation `751b947b` passed local, cold-capacity, scoped actual,
  normal Mac package and eleven independent package checks for authenticated
  finalized fighter history. The candidate build is 0.4.11/build 51. Its first
  package attempt failed an inherited missing-source restart probe; a focused
  diagnostic did not reproduce it and the fresh full package passed without
  relaxed assertions or deadlines. The original cause remains unproven.
  Approved icon bytes are unchanged. No compiled seed-node source or deployment
  configuration changed, so the verified desktop build does not trigger a
  Set 51 seed rollout; master/live remain Set 50. Remote CI absence is not a
  pass. GUI/accessibility, production authority, rankings/titles, permanent
  genesis and the final joint D74 review remain open. See `CHANGESET_51_NOTES.md`.

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
| Fighter-name diversity and collisions | Before launch, substantially expand the candidate name pool and measure collision frequency at expected fighter populations. Fighter IDs remain the identity keys. Use deterministic, versioned generation so existing identities, assigned names and manifests remain stable; do not rewrite the frozen generator or historical fighters. Bring population assumptions and remaining duplicate-name behavior to the D74 review. |
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

The current frozen name tables contain 32 distinct first and 32 distinct last
names, yielding 1,024 display names. Under an independent-uniform draw model,
the probability of at least one shared name exceeds 50% at 38 fighters; at
100 fighters, the expected fraction with a namesake is about 9.22%. These are
analytic scenarios, not a production population forecast. D51 already permits
duplicates. Review acceptable prevalence and disambiguation at an explicit
population scope, then assess the expanded version against that target while
preserving existing assignments. The workspace's
`set48-name-collision-prelaunch-note.md` records the assumptions and calculations.

The approved logo handoff is preserved byte-for-byte in the workspace's
`../brand-assets`: gray background, white front-facing fist, black linework and
SLUG across the knuckles, plus the transparent variant. Use those exact assets
in the forthcoming desktop design work rather than recreating the mark. Source
paths and checksums are recorded in `../brand-assets/handoff.json`.

Do not invent a calendar date or claim an automatic reminder has been scheduled.
Arrange the session when a representative build is ready, before user-facing
release. Any later scope change must keep this gate visible.
