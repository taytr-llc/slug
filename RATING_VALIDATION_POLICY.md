# Rating validation policy — D75

Mike decided on November 17, 2026 to retain D65 and replace the exact-coefficient
equality gate used during set 42. This document records a post-result amendment;
it does not change the historical contract or declare its original failure a pass.

## Incumbent and decision

The published rating remains:

`1.0 × Power + 2.5 × Speed + 0.2 × Chin + 0.4 × Stamina + 0.8 × Skill`

Use the existing current-condition attributes and permanent professional XP,
excluding temporary form and weekly training bonuses. Condition version 1,
its exact snapshots, stable authority pins, stored assignments/rosters, original
birth and preview-v1 commitments remain unchanged. No rating version 2 is
authorized. A summary rating change would not change combat attribute effects.

Mike's instruction:

> Keep D65 unchanged. The exact-equality acceptance rule is rescinded and
> replaced: a rating change requires a refit improvement whose paired bootstrap
> interval excludes zero, validated on prime, aging, and XP-varied populations
> including close pairings. Run the coefficient-stability bootstrap and the
> population-coverage validation on the set 42 engine and report both. Record
> the failed refit, the rescinded rule, and this new policy as a new decision,
> preserving the original result.

The incumbent wins ties and inconclusive evidence. A proposed replacement must
show a positive improvement: its paired interval must lie wholly above zero.
Continue reporting 95% intervals, with population definitions, resampling unit,
fit settings, rounding and multiplicity treatment fixed before new outcomes.
Coverage must include prime, aging and XP-varied fighters and close pairings;
report band-edge behavior and adverse strata rather than hiding them in an
overall average. Evidence from one chosen distribution is not a claim about an
unknown future player population. Statistical evidence does not itself activate
a new rating; any future publication retains the explicit version/change process.

## Preserved original failure

The frozen set 42 contract required its rounded refit to equal D65 exactly.
That rule was criterion 2 in section 13 of `set42-fight-contract-draft.md`,
SHA256 `abb5b54edd921855b81dc5b99ef29b2ac94cadc9296dcd2981af4b57ad25040f`.
The immutable copy is in `fighterbout/testdata/reference/inputs/`.
D75 supersedes that criterion for current acceptance.
All other original numerical and source-conformance criteria remain in force.

The original study used 4,000 training and 2,000 held-out prime pairs, with
additional decline, mirror and endpoint diagnostics. Its normalized rounded
refit was `[0.9, 2.5, 0.2, 0.6, 0.8]` in Power/Speed/Chin/Stamina/Skill order.
The original report remains failed because that tuple is different from D65.
Unchanged D65 scored 85.05%, above the 75% floor and only 0.35 percentage points
below the continuous fit, within the existing 2-point tolerance. The separate
rounded-predictor diagnostic scored 85.65%, with paired improvement interval
[-0.10, +1.35] percentage points. This does not establish superiority.

Preserve these original evidence hashes:

| Evidence | SHA256 |
| --- | --- |
| Original numerical calibration report | `0f9f3261dac4585e646961eb504e036cc98e4a7d66f5eba073d2a5303a0e2b9e` |
| Original calibration capture | `4811071294df275d40011b4cca2f3e90f20fae87972915bab7f2f83b5ac966f7` |
| Separate rounded-predictor diagnostic | `1ee4624d8786fdc88e30830fecddddbf2008ff52b82c6db8c572cdd3786faf65` |

The original contract, source model, independent expected transcripts, inputs,
outcomes, fit history and bootstrap samples must not be regenerated or edited
to conceal that result. Reports of current acceptance must identify D75 and
the original rescinded criterion separately.

## Required follow-up studies

1. Coefficient stability: resample training fights and refit the same declared
   procedure on every bootstrap replicate. Report coefficient intervals,
   normalized/rounded variability and joint tuple frequencies. Validate any
   numerical acceleration against the exact original fit and specified
   high-precision reference replicates. Keep this distinct from bootstrapping
   predictions with fixed coefficients.
2. Population coverage: freeze fresh, disjoint, outcome-independent inputs for
   prime, aging and XP-varied populations, including close and band-edge pairs.
   Compare fixed D65, the existing rounded proposal and the original continuous
   fit on the unchanged set 42 engine. Report paired uncertainty, population
   and class strata, draws, finish/duration behavior and limitations. No new
   outcome-selected coefficient tuning is part of this validation.

The study protocol and results are separate artifacts. Their preparation does
not authorize changing runtime code, model parameters or published weights.
The original normal-PoW/source checks remain scoped to their captured sources;
package acceptance, smoke and deployment observation still require completion.
Actual visual/native interaction, accessibility and the joint D74 design review
remain product-release gates.

Both required studies are complete and independently audited. All eight
primary paired improvement intervals include zero, so the challenger has not
demonstrated superiority. D65 remains unchanged. The explicit successor
acceptance preserves the historical failed report and all other original
numerical gates. See [RATING_VALIDATION_REPORT.md](RATING_VALIDATION_REPORT.md)
and `fighterbout/testdata/rating-validation/` for results and retained evidence.
