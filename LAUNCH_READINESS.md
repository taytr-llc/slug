# SLUG launch readiness

Updated November 16, 2026. Numbered change sets are reviewable engineering
increments, not a fixed countdown to launch. This checklist tracks the actual
finish line and must be updated from evidence rather than a branch number.

**Required design review with Mike:** before the friends/tester or public
product release, hold a thorough joint design review and address or explicitly
agree to defer its findings. Mike requested this gate on November 16 (D74).
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
  acceptance and packaged helper tests pass. Its sources are published; exact
  wallet CI and seed promotion are tracked in the workspace release report.
- D73 approves the production-profile design. Set 30 implements it with
  explicit public candidate fixtures while final allocation/genesis and real
  issuance remain gated. Provider coordination is the next planned set 31.

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

The approved logo handoff is preserved byte-for-byte in the workspace's
`../brand-assets`: gray background, white front-facing fist, black linework and
SLUG across the knuckles, plus the transparent variant. Use those exact assets
in the forthcoming desktop design work rather than recreating the mark. Source
paths and checksums are recorded in `../brand-assets/handoff.json`.

Do not invent a calendar date or claim an automatic reminder has been scheduled.
Arrange the session when a representative build is ready, before user-facing
release. Any later scope change must keep this gate visible.
