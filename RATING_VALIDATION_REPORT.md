# D75 rating validation on the set 42 engine

D65 remains unchanged: `1.0 Power + 2.5 Speed + 0.2 Chin + 0.4 Stamina + 0.8 Skill`. No rating or combat parameters were changed by these studies. The original exact-equality calibration remains failed under its original contract; D75 explicitly rescinds that criterion.

## Coefficient stability

We refit the original declared logistic procedure on 1,000 class-stratified resamples of the 4,000 training pairs. Each fit retains 400 iterations and the original settings. This measures coefficient variability; it is separate from resampling predictions with fixed weights.

The intervals below are 95% percentile bootstrap intervals after normalizing each fitted tuple to mean one. For a comparable scale, the incumbent column divides the unchanged published coefficients by 0.98. These comparison values are not newly published rating weights.

| Attribute | Published D65 | D65 on mean-one scale | Original normalized fit | Bootstrap median | 95% coefficient interval | Frequency of original rounded value |
| --- | ---: | ---: | ---: | ---: | --- | ---: |
| Power | 1.0 | 1.020 | 0.891 | 0.891 | [0.829, 0.954] | 0.9: 87.5% |
| Speed | 2.5 | 2.551 | 2.457 | 2.459 | [2.374, 2.547] | 2.5: 55.6% |
| Chin | 0.2 | 0.204 | 0.237 | 0.235 | [0.163, 0.302] | 0.2: 65.7% |
| Stamina | 0.4 | 0.408 | 0.568 | 0.566 | [0.505, 0.626] | 0.6: 68.3% |
| Skill | 0.8 | 0.816 | 0.848 | 0.848 | [0.784, 0.912] | 0.8: 52.9% |

The exact rounded proposal `[0.9, 2.5, 0.2, 0.6, 0.8]` occurred in 161/1,000 refits (16.1%). The complete raw and normalized covariance/correlation matrices, tuple frequencies, validity counts and retained fits are preserved in the study artifacts. Coefficient uncertainty does not by itself establish predictive superiority or causal gameplay effects.

Numerical acceleration was checked against the original 50-digit Decimal fit and seven fixed bootstrap refits, plus 0 additional declared boundary fallbacks. Maximum normalized error was 4.257E-16; the original and all seven fixed reference rounded tuples agreed. Original Decimal reproduction was exact.

The Power and Stamina coefficient intervals lie respectively below and above the incumbent on the comparable mean-one scale. That supports a directional shift within this original prime training design, while the rounded values and joint tuple vary. It does not establish a causal explanation or superiority on fresh matches.

## Fresh population coverage

20,000 fresh pair families use 40,000 distinct fighter IDs, disjoint from the original calibration and conformance inputs. All five weight classes have equal representation. Each of four age/XP populations contributes 2,000 natural pairs, 2,000 close pairs and 1,000 initial-band-edge pairs. Close means the larger D65 rating is at most 1.05 times the smaller. The input populations, predictors, seeds, resampling cells and analysis rules were frozen before any new outcomes.

Mixed-age samples cover all 16 ordered combinations of prime, early decline, mid decline and late decline. XP-varied samples cover 0–15 professional XP, balancing gaps of 0, 1–5, 6–10 and 11–15. These are synthetic research populations, not estimates of future player prevalence.

Accuracy gives half credit to draws and rating ties. Improvement is the fixed rounded proposal minus unchanged D65, in percentage points. Each interval uses 10,000 paired resamples within the predeclared class/age/XP cells.

| Population | Pairing | D65 accuracy | Rounded proposal | Improvement | Paired 95% interval |
| --- | --- | ---: | ---: | ---: | --- |
| Prime, no XP | Natural | 85.625% | 85.650% | +0.025 | [-0.775, +0.800] |
| Prime, no XP | Close | 55.950% | 56.150% | +0.200 | [-1.950, +2.325] |
| Mixed age, no XP | Natural | 85.950% | 85.750% | -0.200 | [-0.900, +0.500] |
| Mixed age, no XP | Close | 54.000% | 54.825% | +0.825 | [-1.250, +2.900] |
| Prime, varied XP | Natural | 84.200% | 83.925% | -0.275 | [-1.000, +0.450] |
| Prime, varied XP | Close | 54.625% | 54.800% | +0.175 | [-1.925, +2.250] |
| Mixed age, varied XP | Natural | 86.300% | 86.600% | +0.300 | [-0.300, +0.900] |
| Mixed age, varied XP | Close | 57.100% | 57.700% | +0.600 | [-1.325, +2.525] |

0 of the eight primary comparisons have ordinary paired intervals wholly above zero. The prospective study rule requires all eight; the challenger therefore does not meet that condition. The incumbent remains in place. An interval containing zero is inconclusive, not proof that the predictors are equal. Approximate Bonferroni-adjusted intervals are also retained in the reports.

### Initial matchmaking-band edges

Each edge cohort has 500 pairs just inside the initial D52 limit (rating ratio 1.8–2.0) and 500 just outside (>2.0–2.2). The detailed tables retain both sides. Small and singleton age/XP cells make these intervals descriptive; they do not establish superiority or zero uncertainty.

| Population | D65 accuracy | Rounded proposal | Improvement | Descriptive paired 95% interval |
| --- | ---: | ---: | ---: | --- |
| Prime, no XP | 100.000% | 100.000% | +0.000 | [+0.000, +0.000] |
| Mixed age, no XP | 100.000% | 100.000% | +0.000 | [+0.000, +0.000] |
| Prime, varied XP | 100.000% | 100.000% | +0.000 | [+0.000, +0.000] |
| Mixed age, varied XP | 100.000% | 100.000% | +0.000 | [+0.000, +0.000] |

Both fixed ratings correctly favored the winner in all 4,000 sampled band-edge fights. The zero improvement intervals reflect identical predictions in these samples; they are not evidence of perfect future prediction or equivalence. These deliberately wide rating gaps produced one-sided outcomes and deserve attention in the joint matchmaking/design review, separately from this rating decision.

### Outcome distributions

The unchanged original simulator also ran on the identical input pairs with the documented adapter. The table compares set 42 to that source baseline. Finish and draw differences are percentage points; round difference is the mean last-entered round, not elapsed fight time. These are descriptive comparisons, not a formal equivalence test. They cannot attribute a shift to any one engine change.

| Population | Pairing | Finish difference [95% interval] | Draw difference [95% interval] | Mean round difference [95% interval] |
| --- | --- | --- | --- | --- |
| Prime, no XP | natural | +0.80 [-1.200, +2.800] | -1.35 [-1.900, -0.900] | +0.0160 [-0.0060, +0.0385] |
| Prime, no XP | close | -0.90 [-2.950, +1.150] | -2.35 [-3.050, -1.700] | -0.0055 [-0.0220, +0.0110] |
| Prime, no XP | d52-edge | +2.80 [+0.900, +4.700] | +0.00 [+0.000, +0.000] | -0.0220 [-0.0620, +0.0170] |
| Mixed age, no XP | natural | +0.05 [-1.950, +2.100] | -1.45 [-2.000, -0.950] | +0.0120 [-0.0095, +0.0330] |
| Mixed age, no XP | close | +2.20 [-0.100, +4.400] | -2.75 [-3.500, -2.050] | -0.0020 [-0.0190, +0.0150] |
| Mixed age, no XP | d52-edge | +0.80 [-0.900, +2.600] | +0.00 [+0.000, +0.000] | -0.0360 [-0.0740, +0.0020] |
| Prime, varied XP | natural | -1.05 [-3.050, +1.000] | -1.70 [-2.300, -1.150] | -0.0015 [-0.0245, +0.0220] |
| Prime, varied XP | close | +1.35 [-0.700, +3.500] | -2.80 [-3.550, -2.100] | -0.0085 [-0.0250, +0.0085] |
| Prime, varied XP | d52-edge | -0.70 [-2.300, +0.900] | +0.00 [+0.000, +0.000] | +0.0180 [-0.0220, +0.0570] |
| Mixed age, varied XP | natural | -0.35 [-2.250, +1.550] | -0.90 [-1.300, -0.550] | +0.0115 [-0.0095, +0.0330] |
| Mixed age, varied XP | close | +0.40 [-1.600, +2.450] | -2.75 [-3.450, -2.100] | -0.0190 [-0.0355, -0.0025] |
| Mixed age, varied XP | d52-edge | +0.30 [-0.600, +1.200] | +0.00 [+0.000, +0.000] | -0.0190 [-0.0430, +0.0050] |

Finish differences range from −1.05 to +2.80 percentage points and mean round differences from −0.0360 to +0.0180 across the twelve panels. Draw rates are lower in all eight natural/close panels, by 0.90–2.80 percentage points, with paired intervals wholly below zero. For decision draws, the original simulator uses a score difference margin of 2.0; the frozen set 42 model uses exact equality of its rounded score totals. Set 42 also permits a double-corner-stoppage draw, with none observed here. These are different rules, and this study does not isolate the contribution of any one model change. Outcome distributions should not be described as identical. No draw or gameplay rule was changed during D75 validation.

## Preservation and limits

- All original calibration inputs, outcomes, fit, failed report and the separate rounded-predictor diagnostic remain byte-identical. The original conformance corpus and runtime D65/condition v1 also remain unchanged.
- Input preparation initially stopped at its declared 2,000,000-derivation cap, before any fits or fights. A reviewed resource-only amendment raised that cap to 6,000,000. The successful retry had to reproduce all 14,151 preserved partial rows byte-for-byte; populations, seeds, selection rules and sample counts did not change.
- Reports retain class results, comparison changes, methods including zero counts, age/XP cross-tabs, physical/style/rarity coverage and outcome differences. No favorable pooled headline replaces a weak or adverse cohort.
- The study covers standard three-round bouts. It does not validate five-round title/featured bouts, training variation, fresh retired fighters, actual league prevalence or gameplay balance. The coefficient bootstrap describes uncertainty in the original prime training sample; it does not refit an aging-specific model.
- Keeping D65 under D75 does not waive package, normal-PoW smoke or deployment verification. The pending portrait rendering check, actual native/accessibility acceptance and joint D74 design review remain product-release gates.

## Exact evidence

- Source/input freeze SHA256: `91c30017bf982b2f446e3502fd0a7b5944530c282f095fc96fa1b9b6efde8ce8`.
- Coefficient report SHA256: `e435ab79a819156697829538d35ed0e3c4d4f4cb640e5120a1d8d2fc44cb15ca`.
- Population report SHA256: `48f5a230e319530a02ac31fa1dfca0e9d133fbb16a703a47108281912e9a2461`.
- The repository evidence index documents the retained study sources, protocols, compact report data and hashes of the larger workspace artifacts.
