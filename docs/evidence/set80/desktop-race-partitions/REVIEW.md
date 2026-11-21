# Complete desktop race partition successor

The retained first full race attempt failed the pilot package's cumulative
600-second alarm at `TestPoolWithdrawalSetupPopulatedBeforeReadyRefuses/wallet`.
That subtest had run eight seconds. The live stack was runnable in scrypt wallet
creation; the log shows no prior race warning or assertion failure. This does
not establish a deadlock. The failed package elapsed 600.593 seconds; its completed
pilot tests account for 591.74 seconds. The complete normal pilot package passed
in 208.244 seconds.

`run.py` is a separate, unexecuted successor. The active `run-regressions.py`,
actual helpers/checker and repositories are unchanged. Run from the exact staged
path after root review:

```
python3 /Users/taytrrip/slug/set80-prep/desktop-race-partitions/run.py --attempt regressions-desktop-race-partitions-attempt1
```

Four serial pilot processes use whole, anchored top-level names, with all their
subtests and fuzz seeds retained. They cover all 440 compiled Test functions and
both Fuzz targets exactly once. Each keeps `-race -count=1 -p=1 -timeout=600s`,
normal profile, CGO1 and GOMAXPROCS2; each capture permits 720 seconds including
build/cleanup. No test is excluded or retried and no product deadline changes.
The two Fuzz targets are seed-corpus tests under normal `go test`, not a fuzzing
campaign. Go's existing `-run` matching selects these names too.

The fixed partition plan uses observed normal/race durations only for scheduling:
max(observed completed race centiseconds, eight times normal centiseconds, one).
Longest-first assignment, name/partition ordinal tie breaks produce four totals
of about 433 seconds each. These estimates are not promises or PASS criteria.
Each actual process must independently finish all selected names within 600s.
The root can inspect every name in `partition-plan.json`; runtime `go list` must
reproduce the complete declaration inventory before any test starts.

The original accountpilot and slugpilot race packages already passed. The
successor reuses only those 122 and 31 complete package inventories. It requires
the exact prior report, log, profile, source-before/source-after and current bytes
of every prior captured input to match. It reparses every selected PASS/skip from
the original log. The original aggregate stays failed; its completed pilot tests
are not reused. New dependency/source capture and final rechecks cover all three
packages. Successful composite coverage is 595 top-level Test/Fuzz targets:
572 PASS and 23 natural top-level skips, plus three explicit projection subcase
skips. The unchanged exact skip reasons are copied and pinned; any missing,
additional or differently explained skip fails. The one normal-only accountpilot
test guarded by `!race` is documented as absent from the compiled race inventory,
not silently excluded.

All command groups/readers must join without forced signals, all logs remain
bounded, and source/profile/prior evidence are rechecked even on failure. A
failed partition stops subsequent execution and records the remaining names as
missing. No current runtime output was used to change expected product results.
Only source/JSON inspection and AST parsing were performed by this author.
