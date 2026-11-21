# Set81 qualification evidence

Set81 preserves valid retained fighter delivery when only ordinary DAILY pool
intake is full. It also reconciles original-job expiry without creating a late
birth. This evidence is scoped to synthetic fixtures and the selected mode;
public deployment, continuous capacity and the joint D74 review remain open.

## Completed local gates

- [Focused components](components): 16 required top-level tests and 62 passing
  assertions in each of normal and race mode, plus two cheap-profile refusals.
- [Initial complete affected suites](regressions): 361 passing top-level tests in each
  mode across core, operator, service and worker. Six expected profile skips
  remain recorded. The first core/operator normal runs failed because sandboxed
  IPv6 loopback binding was denied; their failed reports are preserved beside
  the unchanged-source successor passes.
- [Build and static checks](build/report.json): all 17 module builds, affected
  vet checks, repository formatting and whitespace checks pass.
- [Normal-node smoke](smoke/result.json): isolated mining and report-only
  checkpoint checks, read-only seed synchronization to height 61 and restart
  pass. [Compiled closure](smoke/release-compiled-closure.json) is unchanged from
  the deployed node. The initial broader diagnostic remains preserved.

[Actual A1](actual/A1-failed/disposition.json) failed the original 90-second
round-readiness gate after the CLI joined. Full intake and public readiness
were visible; sampled round synchronization returned `busy`. Sources remained
unchanged and cleanup joined. The failure is retained while the contention
path is corrected. The reviewed successor passes [all 69 operator tests](regressions/full-operator-normal-successor.json),
[16 targeted race tests](regressions/round-successor-race.json) and
[all 17 builds/static checks](build/successor-report.json). Earlier full core,
service and worker results cover their unchanged implementation.

Actual [A2 passes](actual/A2-qualified/execution/report.json): full intake,
original-owner restarts, lost successful admission reply, exact replay and
stopped-wallet reopen produce one target fighter and one scheduled 19.8-SLUG
payment from two rewards (209-atom fee). The operator remains active during
maturity. Sources/profile/binaries remain unchanged and all owners join.

Actual [B1 failed](actual/B1-failed/disposition.json) the original 90-second
public readiness gate after 100 blocks advanced while the operator was stopped.
Public replay made partial progress; rounds were ready. The expiry retry never
launched and final wallet/financial acceptance was not reached. Its one observed
send is not a qualified payout result. [EF-12](../../EDGE_CASE_FOLLOW_UPS.md#ef-12--extended-offline-catch-up-exceeded-the-restart-readiness-window)
tracks this gap under Mike's November 21 triage; it is not silently repaired or
relabeled passed. Component expiry coverage remains separate.

[Exact-tip CI](ci-closeout/) passes all seven jobs at `138553b1`. Downloaded
evidence verifies 470 required PASS assertions, 32 groups and 64 commands, with
unchanged sources and clean joins. The first external verifier's JSON-prelude
failure and its narrow passing parser successor are both retained. This does
not change B1's failed actual outcome or establish a hosted deployment.

The [initial archive map](archive-initial.json) records the first public copies.
The [source map](archive-source-map.json) and [installation check](archive-source-installation.json)
preserve source/oracle/checker identities, including superseded attempts.
Runtime trees, wallet databases and credentials are excluded. Absolute source
paths in historical evidence are provenance, not a portable execution claim.
