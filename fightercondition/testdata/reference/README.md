# Independent fighter condition candidate oracle

This is a pure candidate projection reference, not an admission authority, clock, ownership record, eligibility decision or fight engine. It preserves DNA v1 and supplied immutable birth facts. No official activation or D74 review is implied.

The reviewed contract is copied to `inputs/fighter-condition-contract.md`, SHA256 `8ee75162fcd631cae15b33804c787690e60bf4997fae6b7cd536bb771908e654`. The original pre-review draft is retained as `original-contract-draft.md` (SHA256 `4c12b919e34bb554554a7b47f698868c1464c4bff82fb419f8317d65cd9176ef`). The review corrected exhaustive attribute coverage from 10..100 to canonical 1..100 and froze the exact Go DTO types and lowercase phases. Root approved these engineering clarifications. No unresolved product-policy decision remained for this helper.

## Reproduce

Python 3.8 or newer, standard library only:

```sh
python3 reference_condition.py --check
```

`--check` independently regenerates all four complete output files and compares their bytes. It verifies pinned source hashes, regenerates every canonical DNA, checks whole-week and millisecond boundaries, validates exact integer ratios against high-precision Decimal half-even rounding, exhausts the aging grid, compares prototype numerical intent, verifies monotonic loss/endpoints/XP ordering, and repeats 128 projections concurrently. It writes no artifacts.

A deliberate reviewed regeneration uses:

```sh
python3 reference_condition.py --generate
python3 reference_condition.py --check
```

There are no Go dependencies, expected Go outputs, network calls, random draws, real clock reads, key operations or image tools. Original absolute paths in `input_hashes.json` are provenance only; copied inputs are resolved relative to this directory. The copied independent DNA oracle is the sole source of original fighter attributes and career lengths. Expected condition values are derived only by this oracle's integer formula.

## Files and exact schema

`condition_v1.json` has:

- `preview_only`, `version`, `projector_id`, `week_ms`, reviewed contract hash and pinned provenance;
- `dna_cases[{name,seed,dna}]`: all eight independent canonical baseline records plus four deterministic supplementary seeds that cover Prime=26/52, Decline=13/39 and original attribute=1/100;
- `cases[{name,dna_index,version,born_at_ms,as_of_ms,pro_xp,expected}]`: 13,950 exact full projections;
- `invalid_cases[{name,version,dna,born_at_ms,as_of_ms,pro_xp,expected_error,expected_zero}]`: 48 invalid API cases; error text is deliberately not frozen;
- coverage and validation metadata.

The API's version is `uint32`, so the negative-version fixture represents an invalid selector at the oracle boundary; a typed Go test can use its modulo-uint32 representation, which is also an unknown version. This corpus is not a new public JSON parser contract. Millisecond inputs include exact signed-int64 extremes; read them using integer-aware types, never through a JavaScript number or generic float-decoding path.

`aging_grid_v1.json` contains `loss_percentages=[10,20,30,40]` and 2,700 rows:

```text
{original, decline_weeks, aged_tenths_by_loss: [[step0..D], ...]}
```

Each inner array corresponds to the same-index loss percentage. This freezes every one of the **291,600** valid original-value/decline-length/step/loss cells: original 1..100, decline 13..39, completed step 0..D. Chin and Stamina share the 30% column; the oracle verifies the prototype's two paths agree. Direct grid vectors test the scalar arithmetic, not permission to project a hand-edited DNA object.

`ties_v1.json` records all **7,836** exact half-ties with original value, decline length, step, loss, numerator, denominator, floor and expected integer tenths. Tie-breaking is exact nearest-even; original Speed=1 at final 40% loss remains six tenths, with no floor to 1.0.

`prototype_artifacts_v1.json` is explicitly **nonnormative**. It records the prototype's binary-float disagreements separately from golden condition values:

- 2,698 aging cells differ, all at mathematically exact half-ties;
- 1,802 unique selected projection ratings differ because of those aged-value differences or binary arithmetic residue.

For example, original Skill=1, decline length14 and completed decline step7 gives exact 0.95, which rounds to **1.0** (ten tenths). The prototype returns **0.9** because its binary representation lies below the exact tie. The zero-seed prime rating is exactly **16,870 hundredths**; the prototype computes `168.70000000000002`. These artifacts do not alter the integer expectations. No output formula uses binary floating point; only the secondary prototype comparison does.

`inputs/lifecycle_rating_reference.py.txt` contains verbatim lifecycle/rating excerpts, hashed alongside the complete original simulation, rating study, current spec and independent DNA files. Old prototype comment decision numbers are not treated as authoritative; the reviewed spec supplies D16/D21–23/D42/D43/D65/D68.

## Boundaries covered

Every selected fighter is projected at -1/0/+1 milliseconds around every whole week from birth through two weeks after career end, using XP 0,1,7,14,15. Coverage includes zero elapsed, prime transition with no loss, first completed decline step, exact retirement, long post-retirement retention, near-MaxInt64 positive birth/time pairs and maximum elapsed age (15,250,284,452 weeks). Remaining duration is computed from elapsed time, without adding an expiry to birth. All XP values 0..15 also receive exact delta checks: aging is unchanged and rating increases by 80 hundredths per XP point.

Inputs are validated before arithmetic. Zero/negative/future births, unknown selectors, out-of-range XP, noncanonical/mismatched seeds and edited DNA all fail. An error corresponds to the zero snapshot; no partial projection is an accepted result. The oracle's full-DNA regeneration prevents a valid-looking attribute or career edit from becoming authoritative.

Phase strings are `prime`, `decline`, `retired`. They describe age only. No ownership/eligibility flag, training/form adjustment, extra floor, Skill cap after XP, fight history accounting or birth mutation exists. Timestamp trust and completed-fight XP accounting belong to the caller's later verified integration.

`verification.json` and `.log` record commands, durations, Python/platform details and exact final source/output hashes. They distinguish this independent numerical evidence from the future Go implementation/race checks and product integration.

## Portability correction

The initial evidence used Python 3.14.3. An integration check launched Python
3.9.6 and exposed a difference in the **nonnormative** rating diagnostics:
Python 3.12 changed the float-summation algorithm used by built-in `sum`.
The underlying 13,950 expected snapshots, all invalid cases, the entire aging
grid and all exact ties were unchanged. The initial diagnostic count was 558
under Python 3.14 and 1,802 under Python 3.9.

The oracle now explicitly evaluates the original prototype's legacy sequence:
start at binary64 0.0, multiply each weight/aged attribute and add each term in
`SOURCE.ATTRS` order. It does not use the host's built-in float sum. On Python
before 3.12 it additionally checks this result against `SOURCE.league_rating`.
This frozen diagnostic model is called `sequential-binary64-left-fold-v1` and
produces 1,802 selected rating differences on both tested interpreters. The
rating-artifact count was also removed from normative corpus metadata; the
full diagnostic report owns it. No normative snapshot value was regenerated
from a prototype float or Go output. Prior evidence is preserved separately at
`/Users/taytrrip/slug/set35-condition-reference-pre-portability`.
