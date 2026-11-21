# Set80 qualification evidence

Local qualification is complete. [Final CI closeout](ci-closeout/README.md)
records all seven jobs passing at maintenance tip `1f559221`, paired with wallet
`dfc5d9dd`, while preserving the original `ce9eac74` shutdown failure. These
are finite synthetic-fixture tests, not a deployed public pool or release.

- [Payment recovery A3](actual-A3/execution/report.json): PASS, 440.362 seconds.
  Real operator crash, successful reply loss, same-request replay, original
  stopped-wallet reopen, maturity and one scheduled 9.9-SLUG payment. The fee is
  141 atoms from the pool cut. All 39 public artifacts and source/profile/cleanup
  bindings pass. One separate pending outbox row remains retained.
- [Unused-job expiry B1](actual/B1-qualified/report.json): PASS, 342.741 seconds.
  Original request/job survives crash and real five-minute expiry; no new work,
  share, birth, credit or payment. The stopped null local request is retained;
  worker terminal consumption has separate focused coverage.
- [Desktop race completion](desktop-race-partitions/execution/report.json):
  PASS. Four fixed pilot groups plus original source-bound complete account/CLI
  packages cover 595 targets:572 PASS and 23 documented top-level skips. The
  original cumulative 600-second timeout remains failed in the regression archive.
- [Full affected regressions](regressions/reports): all six normal groups and
  five race groups pass directly; the desktop race successor above completes
  the sixth. The real-expiry financial case is separately run once per mode.
- [Focused components](components/reports), [normal-node smoke](smoke/result.json)
  and the [independent reference qualification](../../../testdata/issue-recovery/qualification.json)
  retain their original outcomes, exact selections and bounded execution data.

## Preserved failures

[A1's root disposition](actual/A1-rejected/qualification-disposition.json)
**overrides its mechanical PASS**: checker source changed and was restored during
execution. It is not qualified. [A2](actual/A2-failed/report.json) failed a live
lookup whose generic assertion omitted the error code. Its separate read-only
diagnostic validates the saved rows but cannot reconstruct that missing error.
A3 adds client/wire observations without changing acceptance, retry or deadlines;
it does not retroactively qualify either prior attempt.

## Exact copies and scope

[Base map](archive-base.json), [supplement](archive-supplement.json) and
[final extension](archive-final.json) bind every selected source/target by byte
length and SHA256. Historical source preimages, patch context and reports retain
their qualified bytes. Narrow archive whitespace attributes preserve those
bytes; product code remains subject to normal whitespace checks.

The maps copy only explicit public files, never recursive runtime trees,
wallet databases, credentials or private fixture files. Historical absolute
metadata paths are evidence, not files to follow. Helpers and source locks are
path-bound; the archive does not claim a portable one-command replay. Existing
limits, original authority, strict expiry, D80, reserved payments and D84 remain.
Hosted operation, continuous capacity and final joint D74 review remain open.
