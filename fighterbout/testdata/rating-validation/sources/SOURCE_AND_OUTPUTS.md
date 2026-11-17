# D75 study source and artifact schema

This is an execution index, not a result or permission to change D65. The effective protocol is the byte-pinned `PROTOCOL.md` plus `RESOURCE_AMENDMENT.md`. The original failed preparation and original set42 calibration remain preserved. No study fits or fights may run without `execution-release.json` approving the exact SHA256 of `source-input-freeze.json`.

## Source ownership and entry points

- `common.py`: immutable reference/protocol pins, canonical JSON, exclusive artifact writes and deterministic SHA256 streams. Every pretty JSON write checks its exact encoded byte count against the retained 1 GiB budget before writing and checks the resulting total afterward.
- `prepare_inputs.py`: coefficient resample bytes and complete population inputs only. No fight or fit invocation. Attempt 001 hit its original cap; attempt 002 repeats the same streams under the resource amendment.
- `audit_inputs.py`: recomputes all 4,000,000 selected training indices and all population schedules using the shared deterministic helpers; checks canonical inputs, quotas, fixed age/XP assignments, filters, disjointness and exact 14,151-row failed-attempt prefix. This is an integrity audit, not a second independent RNG implementation.
- `execution.py`: exact approval/source/input gate and Decimal50 reporting helpers. Each study records the entry freeze and checks the identical freeze at completion.
- `fit_accelerator.cpp` / `fit_accelerator`: separately authored scalar binary64 training bootstrap accelerator. `ACCELERATOR_IO.md` defines every byte of its input and JSONL output. Compiler source/version/flags/binary are pinned in `accelerator-build.json`.
- `coefficient_study.py` / `decimal_refit_worker.py`: runs all 1,000 refits and the original accelerator control, launches at most two owned Decimal checks, enforces numerical tolerances and reports all retained replicates. Children have individual deadlines and are terminated and joined on failure.
- `population_study.py`: `simulate` produces one frozen-engine and one unchanged-source outcome for each frozen input; `bootstrap` uses the fixed paired family resamples and produces all panel/class summaries. The original `generate_corpus.check_accounting` runs on every integer-engine transcript.
- `resources.py`: cumulative owned-directory artifact accounting, including preserved failed attempts. Budget failures retain partial artifacts.

All execution uses `/opt/homebrew/bin/python3` version 3.14.3. The original-source simulator uses its original floating-point/RNG behavior; it is a separate diagnostic series. No Go output supplies expectations.

## Frozen input formats

`data/training.dat` and `data/indices.dat` follow `ACCELERATOR_IO.md`: 96,016 and 8,000,016 bytes respectively. The selected-index payload is exactly 8,000,000 bytes plus its 16-byte header.

`data/population-inputs.jsonl.gz` is deterministic gzip (mtime 0), one canonical JSON row per unique family. Each row retains population/panel/class/edge side/unit and original row index; exact age/XP design cell and assignments; accepted class/pair attempt indices; canonical input and InputID; canonical conditions and incumbent rating ratio. `data/population-schedules.jsonl.gz` retains each unit's pre-rejection shuffled design and XP choices and raw draw counts. The archives contain 20,000 unique families and 40,000 unique fighter IDs, not admitted ownership evidence.

Input freeze records include SHA256 values, counters, timings, no-outcome declarations and exact executing source hashes. `data/input-audit.json` is required before the final execution release. Historical coefficient-input source hashes remain those actually used before the resource amendment; unchanged bytes are re-audited rather than relabelled as regenerated.

## Result formats and units

`coefficient-results/original.jsonl` and `bootstrap.jsonl` retain exact binary64 hexadecimal coefficients. Per-replica Decimal JSON and command logs retain all checks. `replicates.json` contains every raw vector, mean, normalized vector or null, rounded tuple or null, validity and numerical source. `report.json` contains raw/normalized medians, fixed percentile intervals, covariance/correlation, tuple frequencies, validity and all error checks. Nonfinite or undefined normalization is never silently dropped. These are training-fit uncertainty estimates, not held-out predictor improvements.

`population-results/full-results.jsonl.gz` retains InputID, the full integer-engine transcript and original-source winner/method/round/scores. `outcomes.jsonl.gz` retains the analysis inputs and predictions, exact half-point scores, outcome labels, result digest, actual ages/fractions, XP, classes, physical traits, styles and stances. Prediction gap values retain the original weighted league-tenths scale; displayed gap summaries divide by 10. Continuous raw-fit gaps have an arbitrary scale and are not directly comparable with D65 point gaps.

`bootstrap-replicas.json` retains all 10,000 raw paired totals for every panel. Predictor totals and paired improvements have denominator `2*n`; finish/draw/last-entered-round differences have denominator `n`. The report states these denominators and exact percentile indices. Improved/worse/unchanged counts compare each family's draw-first half-point scores, not just favourite labels. Panel and class summaries retain all zero-valued categories, fine age/XP cells and explicit XP-gap buckets.

`population-results/report.json` contains the 12 separate panels, class summaries, ordinary paired intervals, the eight approximate simultaneous challenger diagnostics and the disclosed eight-panel positive-evidence assessment. D65 remains authoritative regardless of the assessment. Edge intervals are explicitly descriptive and unstable where fixed design cells have one or two families.

Final result reports bind the exact entry source/input freeze, all raw artifact hashes, measured timings and unchanged sources. A compact final artifact index will point to these files; large transcript archives remain outside runtime testdata.

## Scope limits

These are synthetic class-balanced, standard three-round populations, not observed player prevalence. Primary age coverage excludes retirement; training is zero; title, featured and live matchmaker behavior are not validated. Fixed small age/XP cells limit bootstrap resolution. Coefficient uncertainty is conditional on the original training design and engine, not causal attribute importance. The amended input cap changes resource allowance only, and no study authorizes new weights or production release.
