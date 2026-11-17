# Deterministic candidate fight engine

`fighterbout` implements an explicit-seed, deterministic three-round fight
model. It accepts canonical fighter manifests and produces a bounded structured
transcript and whole-fight scorecard. It does not admit a fighter, authenticate
birth or bonuses, award XP/training, record a professional result, or establish
ownership, eligibility or seed fairness. The authenticated desktop use is the
separate [unrecorded fight preview](FIGHT_PREVIEW.md).

This Set42 documentation describes the frozen, integrated implementation.
Source conformance, independently audited D75 studies and successor acceptance
have passed. No Set42 package acceptance or deployment is claimed here.
D75 retains the existing D65 rating; there is no condition-v2 implementation.

## Versioned inputs and API

The supported tuple is `Version1 = 1`, engine ID
`slug-fighter-bout-candidate-v1`, mode `standard`, seed kind `explicit`.
Unknown versions or selectors fail. `FighterInput` contains a full lowercase
prospect ID, complete `fightermanifest.Manifest`, original `BornAtMS`, already
accounted `ProXP` in 0..15 and `TrainingSkill` in 0..5. The engine validates the
entire canonical manifest; arbitrary JSON, a supplied ID or a valid transcript
is not proof of admission.

`Input` contains the version/engine/mode/seed selectors, a common `AsOfMS`,
canonical 32-byte seed and complete A/B fighters. IDs must be distinct and
fighters must share the canonical manifest weight class. `NewInput` sorts
complete fighters by full prospect ID, so reversing a UI selection does not
change corners. Direct validation and parsing require that canonical order.
Birth is positive and not later than as-of; timestamps are bounded by
8,640,000,000,000,000 Unix milliseconds. No system clock is consulted.

```go
NewInput(asOfMS int64, seed string, a, b FighterInput) (Input, error)
ValidateInput(Input) (Input, error)
InputID(Input) (string, error)
MarshalInput(Input) ([]byte, error)
ParseInput([]byte) (Input, error)

Simulate(context.Context, Input) (Result, error)
ValidateResult(context.Context, Input, Result) (Result, error)
MarshalResult(context.Context, Input, Result) ([]byte, error)
ParseResult(context.Context, Input, []byte) (Result, error)
Digest(context.Context, Input, Result) (string, error)
```

`ValidateResult`, the result codecs and `Digest` re-simulate against the supplied
Input and compare the complete deterministic result. InputID alone cannot
reconstruct or authenticate that input. These functions are not cheap poll
parsers. The native desktop has a separate private bounded transport validator;
it never calls a re-simulating API to display a retained result.

## Arithmetic and model behavior

The model uses checked signed integer fixed point with scale 1,000,000 and
round-to-nearest, ties-even. Frozen operation ordering is part of the engine
identity. No floating-point calculation, unbounded public arithmetic helper,
platform PRNG or wall-clock value participates in a fight.

The engine obtains age-adjusted attributes from the unchanged condition-v1
projection. Form applies to aged attributes; the separately validated XP and
training bonuses enter Skill once in the specified order. It never reapplies
XP from `LeagueAttributesTenths`, and it does not silently cap effective Skill
at 100 or 120. The published D65 rating is not a combat input. Original DNA,
career lengths, style and stance remain unchanged.

The exact model includes stamina, initiative, strike selection, accuracy,
damage, resistance and knockdowns, together with bounded explicit relative
reach, height and mass effects. These are numerical-model inputs, not new
traits or new eligibility rules. Threshold jitter and form are explicit
integer grids. Chance comparisons and draw consumption follow the frozen
contract, including draws on the specified conditional branches.

Finish precedence is explicit. Tick stoppages end immediately; between-round
recovery/corner checks occur only when another round remains. There is no
post-final-bell recovery or corner stoppage. A simultaneous double-corner
stoppage has its own terminal method and no winner; it cannot fall through to
a scorecard decision. A `decision_draw` means exact equality of the published
whole-fight score totals. Each total sums the rounded displayed component
values, so the scorecard explains the decision.

The complete equations, constants, draw schedule and operation-specific bounds
are preserved in the immutable source contract under
`fighterbout/testdata/reference/inputs/set42-fight-contract-draft.md`, SHA256
`abb5b54edd921855b81dc5b99ef29b2ac94cadc9296dcd2981af4b57ad25040f`.
D75 supersedes only that contract's original exact-refit-equality release
criterion; its equations and normative expected bytes are unchanged.

## Random streams and commitments

InputID is the lowercase SHA256 of:

```
ASCII("SLUG fighter bout input\0v1\0") || canonical Input JSON
```

Five streams, labelled `form-a`, `form-b`, `chin-a`, `chin-b` and `combat`, are
constructed privately from this InputID. For each word:

```
SHA256(
  ASCII("SLUG fighter bout random\0v1\0") || raw32(InputID) ||
  LE32(label byte length) || ASCII(label) || LE64(counter)
)
```

The word is the first eight digest bytes interpreted big-endian. Each stream
counter starts at zero. Rejection sampling consumes every rejected word and
uses the fixed threshold that avoids modulo bias. At most 256 attempts are
allowed per uniform draw, with a shared limit of 65,536 raw words and refusal
before counter wrap. The private deterministic test seam is not an exported
randomness override. A chance succeeds when its uniform integer is strictly
less than the fixed-point threshold.

The result digest is the lowercase SHA256 of:

```
ASCII("SLUG fighter bout result\0v1\0") || canonical Result JSON
```

A changed seed, complete input, model selector or transcript changes its
appropriate commitment. These hashes do not authenticate an issuer, fair seed,
professional result or trusted timestamp.

## Transcript, codecs and bounds

Result contains version/engine/InputID, two initial states, rounds, two final
states, full winner ID or empty draw winner, finish method/round/tick, two
whole-fight scorecards and per-stream raw draw counts. Each tick records actor,
strike, hit, damage, significant-hit/knockdown flags, stoppage and both states.
A round distinguishes its tick-end state from optional between-round recovery.
The input and complete conditions are not repeated inside every tick or Result.

| Resource | Bound |
| --- | ---: |
| Canonical Input | 32,768 bytes |
| Canonical Result | 196,608 bytes |
| JSON container depth | 8 |
| Rounds | 3 |
| Ticks per round / total | 60 / 180 |
| Raw random words | 65,536 |
| Attempts per uniform draw | 256 |

Strict codecs enforce the complete typed schema, minimal integer spellings,
array lengths, selectors and canonical values. They refuse duplicates, unknown
or missing fields, malformed UTF-8, invalid nulls, trailing content and excess
sizes/depth before materializing unbounded data. Canonical engine parsing is
not the same grammar as the desktop transport: the latter accepts bounded
whitespace/key-order variations, validates values and privately reconstructs
canonical bytes for digest checks.

Cancellation, arithmetic-range or stream failures return no partial Result.
The public error families are `ErrInput`, `ErrResult`, `ErrStream` and
`ErrArithmetic`, plus context cancellation/deadline errors. Callers must not
convert failure into a valid draw or a partially recorded fight.

## Independent conformance

The independent Python integer model was authored from the frozen contract and
independent DNA/manifest/condition sources before Go simulation execution. Its
expected values are not captured Go output. Two absolute Python interpreters
regenerated the frozen corpus before execution was released.

The corpus contains 55 complete canonical Input/Result/ID/digest cases, plus
primitive arithmetic, all style/strike branches, stream/rejection/budget and
rare finish cases, initial/physical grids, invalid input/codec cases and result
mutations. Public complete results use valid canonical inputs; rare primitive
states are labelled as private boundary tests. All complete results matched
the Go implementation. Normal, race, cheap-profile, vet and parser fuzz checks
passed at captured source hashes.

Normative corpus SHA256:
`87bd70465fa8c4e83dd55783e4ea0ef8b7f6475ffc74dde86475fd07b0d6ee84`.
Independent model script SHA256:
`c3247a0d856270cc5043b5767ace9f628b2efb9cd120d786773f9221d928bd7f`.
See `fighterbout/testdata/reference` and the Set42 release evidence for the
frozen sources, commands, hashes and reviews. Never update those expectations
to make a runtime change pass.

## D65 calibration and D75 successor policy

The original predeclared calibration used 4,000 training fights and 2,000
held-out prime fights, with additional aging, endpoint, physical and mirror
diagnostics. Its rounded refit was `[0.9,2.5,0.2,0.6,0.8]`, rather than existing
D65 `[1.0,2.5,0.2,0.4,0.8]`. The report therefore **failed the original exact
rounded-tuple equality criterion**, while its other declared gates passed.
That failed report and the original criterion remain preserved as historical
evidence. They are not rewritten as if the original rule had passed.

After those results were known, the user made the new rating-validation
decision D75: retain D65, rescind that exact-equality criterion, and require
positive paired bootstrap evidence excluding zero before a future rating
change, with prime, aging and XP-varied coverage including close pairs. The
incumbent wins ties or insufficient evidence. This is an explicit post-result
policy decision, not a claim that the new criterion was predeclared before the
original experiment. No coefficient v2 is implemented or automatically adopted,
regardless of a study's result.

The additional D75 studies and independent review are complete.
Coefficient stability must resample training fights and refit each replicate;
a fixed-predictor accuracy bootstrap is not that evidence. Population coverage
uses fresh, disjoint, outcome-independent prime, aging and XP-varied inputs,
including close and band-edge pairs, on the unchanged engine. Compare the fixed
incumbent, existing rounded proposal and original continuous fit without new
outcome-selected tuning. Continue reporting 95% paired intervals and adverse
strata under a separately frozen protocol. Their exact populations, splits,
resampling unit, multiplicity handling, close-pair definition and limitations
belong in that protocol and the successor acceptance record, not in a
retrospectively adjusted original report. All other original numerical and
source-conformance criteria remain in force. See
[RATING_VALIDATION_POLICY.md](RATING_VALIDATION_POLICY.md) for the decision and
required evidence. The 1,000 actual coefficient refits reproduced the original
rounded tuple in 161 cases. All eight primary paired improvement intervals
from 20,000 fresh matchups include zero. The complete coefficient, population,
class, edge and outcome-distribution findings are in
[RATING_VALIDATION_REPORT.md](RATING_VALIDATION_REPORT.md). Existing D65,
condition v1, stable pins/status/store identity and preview-v1 seed recipe
remain unchanged. Any later actual coefficient change needs its own explicit
versioned decision and implementation contract.

With the studies and explicit D75 successor acceptance complete, this engine may
proceed through the remaining engineering/package/deployment gates without a
coefficient change. Engineering promotion/deploy/observation remains authorized
subject to those gates. Actual visual/native chooser/screenreader acceptance
and joint D74 review remain required before friends/tester/public product
release; they are not a veto on the authorized engineering deployment process.
