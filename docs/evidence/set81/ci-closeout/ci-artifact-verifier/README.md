# Set81 downloaded Linux CI expectations

These are source-derived expectations for chain `138553b1c6c3b5e9b1fd13d17861912f7ce1cd4c`, paired with wallet `dfc5d9dd7e0f604486fbac812d0413fce1f45cd6`. They are **not a CI result**. The verifier has only been parsed as source; no downloaded Set81 artifact has been verified here, and no Go, test, runtime, or network command was run to prepare this directory.

The selected gate requires **470 PASS assertions in 32 test groups across 64 commands**. This counts required assertions, including the five named Linux ACL subcases and repeated normal/race/cheap checks; it is not the total number of tests that the suites execute. There are 214 normal assertions including the one wire guard, 213 race assertions, and 43 cheap assertions.

| Group suffix | Normal | Race |
| --- | ---: | ---: |
| authority | 2 | 2 |
| wallet | 7 | 7 |
| operator | 38 | 38 |
| daily-public-transport | 21 | 21 |
| daily-public-worker | 15 | 15 |
| daily-public-desktop | 26 | 26 |
| daily-mining-lease | 3 | 3 |
| daily-source-round | 39 | 39 |
| daily-source-fences | 9 | 9 |
| retained-admission | 10 | 10 |
| daily-node-shape | 9 | 9 |
| daily-ledger-schedule | 34 | 34 |

`normal-wire-guard` adds one assertion. The seven cheap groups require: daily-core 16, retained-admission 2, daily-wallet 2, daily-public 5, daily-desktop 2, intake-controls 3, and cancellation-controls 13. `expected.json` records the exact ordered required names, package identities, selectors, build tags, commands, cwd/workspace templates and maximum command bounds.

## Exact Set80 delta

The preserved Set80 final maintenance-tip verification records 428 assertions, 29 groups and 61 commands. All inherited required names remain. All 61 inherited command labels remain in the same relative order; only the normal/race operator `-run` selector gains `DailyRetained`. Set81 adds three retained-admission commands and 42 required assertions:

- Six operator retained-admission tests in each normal/race run: `TestDailyRetainedAdmissionSelectionReadinessAndDirectDrain`, `TestDailyRetainedAdmissionFullSourceReopenPreservesPayment`, `TestDailyRetainedAdmissionHardEscalationAndConcurrentReadiness`, `TestDailyRetainedAdmissionHardCauseSurvivesSourceBusy`, `TestDailyRetainedAdmissionEnableErrorsKeepOriginalOwner`, `TestDailyRetainedAdmissionCanceledBeforeTransition`.
- Four operator round-contention tests in each normal/race run: `TestDailyRetainedRoundOriginalIssuerReconciles`, `TestDailyRetainedRoundCancellationJoinsWhileIssuerHeld`, `TestDailyRetainedRoundRetryClassifiesExactBusy`, `TestDailyRetainedRoundRetryKeepsOriginalDeadline`.
- Ten dedicated retained-admission tests in each normal/race run: `TestManifestAdmissionPreflightOriginalOwnerAndHistory`, `TestManifestAdmissionPreflightExpiryAndRestart`, `TestManifestAdmissionPreflightClockCapacityAndUncertainty`, `TestManifestAdmissionPreflightPublicCounts`, `TestDailyFencedAdmissionExpiryBeforeIntake`, `TestDailyFencedAdmissionSessionAndDuplicateFences`, `TestDailyFencedAdmissionHistoricalAndLegacy`, `TestDailyFencedAdmissionErrorProvenance`, `TestDailyFencedAdmissionLiveCapacityReopen`, `TestRetainedPoolAdmissionDefiniteExpiryAndUnknownReopen`.
- Two cheap refusals: `TestManifestAdmissionPreflightCheapRefuses` and `TestDailyFencedAdmissionCheapRefuses`.

Every required name was matched to its source declaration and selected package, and its top-level name matches the selected `-run` expression. The expectation file pins 91 declaration files plus the gate, workflow, candidate wrapper and build profile. The source expectation was derived by restricted AST inspection, not by importing or executing the qualification script.

## Command and scope boundaries

The 64 commands comprise 32 test commands, 12 before/after Git inventory commands, seven profile/environment/compiler/workspace/wire selection commands, nine build/profile/symbol commands for the three normal binaries, and four vet commands. The binaries are `slugd` and `slugpoold` with CGO 0, and `slugpilot` with CGO 1. All use native Linux amd64 and the unchanged `slug-go1274-no-cse-v1` candidate profile. Test commands retain `-p=1 -parallel=1 -count=1 -timeout=10m`, with `-race` only for race groups and `slugtestpow` only for the named cheap groups. The total gate budget is 2700 seconds; the workflow budget is 55 minutes.

The existing full-intake real-expiry financial test remains in the full ledger/scheduler suite once per normal/race mode. Local actual A/B/L tags are absent. The CI report must retain `provisioning_qualified=false`, `daily_actual_qualified=false`, and `deployed_host_qualified=false`. A downloaded operator result does not qualify a hosted deployment or the separate actual acceptance scenarios.

## Future verifier use

Root may run the following only after downloading the exact artifact; the command below is a recipe, not evidence of execution:

```text
python3 /Users/taytrrip/slug/set81-prep/ci-artifact-verifier/verify.py \
  /absolute/downloaded/operator-linux-directory \
  --ci-report /absolute/root-watch/report.json \
  --out /absolute/new/set81-linux-verification.json
```

`--ci-report` is optional. Without it, the result explicitly leaves the overall seven-job CI claim unverified. With it, the watcher report must identify this exact chain head and branch, both completed successful workflows, and exactly these seven successful jobs: Build, Unit coverage, Unit race, Unit rpctest, Minimum Go version (compatibility), In-tree modules, and operator-linux.

Required downloaded inputs are `report.json`, byte-identical `sources-before.json` and `sources-after.json`, all 64 exact named logs, and all three normal binaries. The verifier checks exact command arguments/environments, package-qualified required PASS events against the log and report (SKIP is insufficient), every selected package result, source commits/status and pinned source identities, complete tracked-file-list evidence, original process-group joins, no termination signals or cleanup failures, temporary cleanup, hashes/lengths, native normal/cheap wire selection and normal binary profile/symbol assertions. It then rehashes only those allowlisted inputs. It neither walks arbitrary artifact subtrees nor accesses a wallet/runtime database.

The output must be a fresh path outside the downloaded artifact tree; existing reports are never rewritten. Failed verification is preserved as a separate failure result. Original source inventories and compiled artifacts retain their recorded provenance. This checker is an integrity/consistency check of downloaded evidence, not independent authentication of GitHub or a replacement for the exact-tip job observation.
