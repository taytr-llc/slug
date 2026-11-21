# Set80 full affected regressions

Source-only runner: `run-regressions.py`. Root alone executes it. Use one fresh attempt per slice/mode, for example:

```
python3 set80-prep/run-regressions.py --attempt regressions-financial-normal-attempt1 --slice financial --mode normal
```

Slices use complete package suites with no `-run` filter: core (`registry`, `poolshare`); transport (`poolservice`, `poolingress`); worker (`prospectworker`); operator (`desktop/pooloperator`); desktop (`desktop/pilot`, `desktop/accountpilot`, `desktop/cmd/slugpilot`); financial (`poolsettle`, `poolschedule`). Modes are normal and race. Both use the original pinned build-profile wrapper, CGO1, no additional build tags, `GOMAXPROCS=2`, `-p=1`, `-count=1`, and `-timeout=600s` per package. Aggregate test capture is 600 seconds times package count plus 120 seconds for compilation. Dependency capture has its separate existing 120-second bound. Logs are bounded to 32 MiB for dependencies and 64 MiB for the full suite. No product timeout changes.

Only the financial slice applies `-skip ^TestRecoverableDailyFullIntakeExpiredWinnerPreservesPendingPayout$`. This real 300-second expiry test already passed normal and race in the pinned public reports `artifacts/set80/financial-normal-attempt2/report.json` and `artifacts/set80/financial-race-attempt1/report.json`; both are validated and recorded. The original old-mode/full-intake, pending-body, accounting, and all other tests remain selected. This runner neither reruns nor claims actual opt-in process/node acceptance.

The 54 exact natural skip admissions in `regression-skips.json` cover normal-profile cheap-only refusal controls, existing explicit node/fixture opt-ins, real-UTC midnight guards, Linux's Darwin-only ACL check, and three existing raw-parser-only wallet projection subcases. Each admission requires exact package, complete test name and reason text; the ACL admission also requires Linux. Every observed skip is reported separately and never supplies PASS. Unknown skips fail. All inherited `SLUG_*` environment variables are removed, so these runs cannot accidentally select a private/actual fixture from the parent environment. Actual-only build tags remain unselected.

The runner requires PASS for every selected package and a terminal PASS or admitted skip for every selected top-level test declaration from `go list -test`; missing tests and all failures fail the run. Nested skip admissions are exact, too. Full JSON logs, selected test declarations, top-level pass/skip inventory, all pass names and package PASS records are retained.

Source binding includes the selected complete package trees, public testdata directories named by their literal fixture paths, original modules/workspace files, the exact dependency/native/embed closure, the runner/skip inventory, capture/profile helpers and build profile/wrapper. It re-enumerates selected trees/testdata at the end. A preliminary inventory precedes dependency discovery; discovered dependencies are bound before tests. Source and profile comparisons and inherited process-group/log cleanup checks execute on failure as well. Source-only validation comprised AST parsing, exact skip source/reason checks and static fixture path review; no runner import, Go, tests or processes were executed by the author.
