# Candidate fighter current-condition projection

Reviewed engineering candidate frozen November 16, 2026 Pacific from spec D16/D21–23,
D42/D43/D65/D68 and the supplied `sim/fight_sim.py`. This is a pure projection
candidate, not a new clock, admission authority, ownership record or fight
engine. It preserves the canonical DNA v1 record and every historical birth.
It does not activate a product rule or replace Mike's D74 joint review.

## Inputs and ownership of facts

Package `fightercondition` proposes `Version1 uint32 = 1`, identifier
`slug-fighter-condition-candidate-v1`, and a pure
`Project(version uint32, dna fighter.DNA, bornAtMS, asOfMS int64, proXP int) (Snapshot, error)`.
Unknown versions fail. Regenerate the entire DNA from its canonical seed and
version and require exact equality, including lowercase seed. Never accept
hand-edited attributes or career lengths. The function uses no system clock,
network, mutable state, caller-supplied rate, or floating point.

`bornAtMS` must be positive and `asOfMS >= bornAtMS`. Both are signed int64
Unix milliseconds. Subtract only after these checks, so elapsed time cannot
underflow or overflow. A caller must obtain birth from verified durable D68
admission history and as-of time from its own trusted operation boundary.
This pure function cannot authenticate either timestamp; arbitrary preview
inputs are not evidence of an admitted fighter. It never rewrites a birth,
updates a receipt, or grants eligibility because a claimant chose a time.

`proXP` is an already-accounted integer in [0,15]; reject out-of-range values
rather than silently normalizing corrupt state. D42 earns +1 per completed
professional fight (including a draw) or +2 for a title fight, capped at 15;
that event accounting is separate. No professional history, weekly training,
form, injury, wear, transfers, retirement flag or roster selection is supplied.

## Exact week and phase arithmetic

One elapsed age week is exactly 604800000 milliseconds (seven 24-hour days).
This is duration since job issuance, independent of local timezone, calendar
weeks, Saturday entry locks or DST. `age_weeks = floor((asOfMS-bornAtMS)/W)`.
Let P and D be canonical DNA prime and decline weeks. P is 26..52 and D is
13..39. Let `elapsed_decline_weeks = clamp(age_weeks-P, 0, D)`.

- Prime while `age_weeks < P`.
- Decline while `P <= age_weeks < P+D`.
- Retired when `age_weeks >= P+D`.

At the exact prime boundary phase becomes decline with zero accumulated loss;
the first decline step is one completed week later. At the exact career end,
all specified loss applies and professional retirement begins. This matches
the reference's week-by-week linear model. After retirement, retain final
condition for exhibitions; do not continue declining, add an attribute floor,
erase the fighter, or decide belt/transfer effects in this helper.

`prime_remaining_ms = max(0, P*W-elapsed_ms)` and
`prime_remaining_weeks = max(0, P-age_weeks)`; the latter is the ceiling of the
remaining partial-week duration. `career_remaining_ms = max(0,(P+D)*W-elapsed_ms)`.
Avoid adding durations to near-int64-limit timestamps; use elapsed arithmetic.
No value grows unbounded except the explicitly int64 elapsed age weeks.

## Current attributes and D65 rating

Store adjusted attributes as exact integer tenths, with JSON names
`power`, `speed`, `chin`, `stamina`, `skill` under an explicitly named
`aged_attributes_tenths` object. Original mined integers remain unchanged.
The losses by attribute are Power 20%, Speed 40%, Chin 30%, Stamina 30%,
Skill 10% of the original values at the end of decline.

For original integer A, loss integer L and completed decline weeks d:

```
aged_tenths = round_even(A * (100*D - L*d) / (10*D))
```

All arithmetic is exact integer/rational. Round to nearest integer, ties to
even; never use binary floating point. Validate all inputs before arithmetic.
The bounded DNA ranges keep intermediates well within signed int64. This
preserves the Python reference's one-decimal intent while eliminating binary
float rounding artifacts. Prime yields A*10; the final step gives precisely
the stated percentage loss. No extra minimum or maximum is imposed. Canonical mined attributes span
1..100, including values below 10. Original Speed=1 ends at 0.6 (six tenths),
for example; do not floor it to 1.0.

`league_attributes_tenths` copies aged attributes and adds `10*proXP` to
Skill only. XP is added after aging, so earned experience is not aged again.
It does not change mined Skill or become a sixth attribute. Do not cap Skill
at 100 after adding the approved bonus. No weekly training or fight form enters
this projection. Those belong to a later fight-input calculation; this helper
must not accidentally include them in the rating.

The D65 rating uses the league/current-condition attributes, represented as
an exact integer in hundredths:

```
rating_hundredths = 10*PowerTenths + 25*SpeedTenths
                  + 2*ChinTenths + 4*StaminaTenths + 8*SkillTenths
```

There is no secondary rounding. This is the existing published 1.0/2.5/0.2/0.4/0.8
formula, not a refit or calibration claim. Actual future fight rebalancing still
requires the spec's rating refit before that engine ships.

## Output and scope

`Snapshot` contains, in order: version, projector_id, dna_version, seed,
born_at_ms, as_of_ms, age_weeks, phase, elapsed_decline_weeks,
prime_remaining_weeks, prime_remaining_ms, career_remaining_ms, pro_xp,
aged_attributes_tenths, league_attributes_tenths, rating_hundredths.
Exact Go DTO types and JSON tags are:

```go
type AttributesTenths struct {
    Power   int `json:"power"`
    Speed   int `json:"speed"`
    Chin    int `json:"chin"`
    Stamina int `json:"stamina"`
    Skill   int `json:"skill"`
}
type Snapshot struct {
    Version              uint32           `json:"version"`
    ProjectorID          string           `json:"projector_id"`
    DNAVersion           uint32           `json:"dna_version"`
    Seed                 string           `json:"seed"`
    BornAtMS             int64            `json:"born_at_ms"`
    AsOfMS               int64            `json:"as_of_ms"`
    AgeWeeks             int64            `json:"age_weeks"`
    Phase                string           `json:"phase"`
    ElapsedDeclineWeeks   int              `json:"elapsed_decline_weeks"`
    PrimeRemainingWeeks  int              `json:"prime_remaining_weeks"`
    PrimeRemainingMS     int64            `json:"prime_remaining_ms"`
    CareerRemainingMS    int64            `json:"career_remaining_ms"`
    ProXP                int              `json:"pro_xp"`
    AgedAttributesTenths  AttributesTenths `json:"aged_attributes_tenths"`
    LeagueAttributesTenths AttributesTenths `json:"league_attributes_tenths"`
    RatingHundredths      int              `json:"rating_hundredths"`
}
```

The phase strings are exactly lowercase `prime`, `decline`, or `retired`.
There are no other phase values and no floating-point output fields. Bounded
`int` values fit even a 32-bit architecture; age weeks and milliseconds stay
`int64`. Even ignoring the canonical talent budget, rating is at most 50,200.
The positive-birth and ordered-time checks bound elapsed milliseconds by
`MaxInt64-1`; do not cast the potentially large age to `int` before comparing
or clamping. All duration multiplication uses an explicit `int64` operand.
No absolute end timestamp is formed.
This slice need not add a parser or claim canonical JSON/digest identity.
An error returns the zero snapshot, never partial condition.

Do not return a broad `eligible` or `owned` flag: professional eligibility also
needs admitted identity, current ownership, roster and event rules. Retired is
an age phase only. The same valid inputs always return an equal value without
mutating DNA. Presentation must label original, aged and XP-adjusted values;
no UI integration or historical trait reassignment is implied here.

## Independent verification before runtime acceptance

Create an oracle from this document and the independent DNA reference only,
never production Go outputs. Preserve both source hashes and a reference
snippet/hash of the original prototype lifecycle/rating functions. Compare
exact rational arithmetic of the aging model, documenting binary-float text
artifacts rather than blessing them as golden expectations.

Use all canonical DNA corpus seeds and both XP endpoints, plus representative
interior XP values. Check one millisecond before/at/after every prime, decline
week and retirement boundary; zero elapsed; many weeks after retirement;
positive near-int64-max birth/time pairs; invalid versions, mutated DNA,
noncanonical seeds, zero/negative/future births and invalid XP. Independently
exhaust all original attribute values 1..100, decline lengths 13..39 and steps
0..D for each loss percentage, including all round-even ties. Freeze expected
integer tenths and ratings. Show monotonic loss, exact endpoints, unchanged
originals, XP exclusion from aging and training/form exclusion from rating.
Run concurrent/race checks without mutable global scratch buffers.

Receipt binding, authoritative event XP, roster restrictions, title vacancy,
exhibitions, training-day boundaries, ranking windows and fight form are later
integration work. None can be inferred from a plain JSON receipt or a preview.

## Review resolution and provenance boundary

The review confirmed the D16/D42 XP cap and prototype draw accounting,
D21/D22 prime/decline endpoints, D23 no-floor behavior, D43 proportional losses,
D65 XP-after-aging rating without training/form, and D68 immutable issuance
birth. The seven-day millisecond conversion and lowercase/integer DTO are
explicit candidate engineering choices translating the prototype's integer
weeks; they add no admission or eligibility rule. The root approved the exact
output types and phase spelling during this review.

The draft's exhaustive-grid range 10..100 was corrected to canonical DNA's
1..100. No product-policy ambiguity remains in this pure projection. The
original draft, reviewed contract, relevant spec/prototype extracts, independent
DNA reference and checksums are retained with the independent oracle. This
review does not authorize runtime integration, official activation, or replace
D74. The oracle must never take expected values from a future Go projection.
