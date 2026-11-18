# Set42 candidate fight engine — exact draft for review

Status: **unshipped engineering draft**, November 16, 2026. Root approved drafting and implementation planning, not these model constants, oracle output, calibration success or engine release. No Go implementation, independent goldens or new runtime is authorized by this document until root approves its exact reviewed hash. This draft makes the proposed choices executable rather than delegating them to an implementer.

The original input provenance is `set42-proposal-inputs.json`. Preserve the imported `sim/fight_sim.py` and all old DNA, manifest, condition, descriptor, receipt and stable contracts byte-for-byte. This is a new engine identity; it never changes a historical version. Statistical release gates in §13 apply even to the Candidate Pilot preview.

## 1. Scope and version

New pure package `fighterbout`. Constants:

```
Version1 uint32 = 1
EngineID = "slug-fighter-bout-candidate-v1"
Scale int64 = 1000000
Rounds uint32 = 3
TicksPerRound uint32 = 60
MaxInputBytes = 32768
MaxResultBytes = 196608
MaxJSONDepth = 8
MaxRawWords uint32 = 65536
MaxUniformAttempts uint32 = 256
```

Version1 accepts only `mode:"standard"` and `seed_kind:"explicit"`. It does not implement title/featured execution, future seed selection, challenges, entries, records, XP awards, ownership or eligibility. D10's five-round title rule remains required for a later explicit mode/version; do not accept a title flag and run three rounds. A supplied explicit seed is reproducible input, not proof of unpredictability, canonical inclusion or D12/D46 fairness.

The first integration remains an unrecorded Fight Preview of two verified same-gym candidates. It uses accounted XP=0 and training=0 until real result authorities exist. The pure engine supports the full already-approved XP/training ranges for exact testing; supplying them authenticates neither award. The engine does not read a system clock, network, filesystem or mutable global random state.

## 2. Exact public input and validation

Ordered Go DTOs/JSON fields (no `omitempty`):

```go
type FighterInput struct {
    ProspectID    string                   `json:"prospect_id"`
    Manifest      fightermanifest.Manifest `json:"manifest"`
    BornAtMS      int64                    `json:"born_at_ms"`
    ProXP         uint32                   `json:"pro_xp"`
    TrainingSkill uint32                   `json:"training_skill"`
}
type Input struct {
    Version  uint32       `json:"version"`
    EngineID string      `json:"engine_id"`
    Mode     string      `json:"mode"`
    SeedKind string      `json:"seed_kind"`
    AsOfMS   int64       `json:"as_of_ms"`
    Seed     string      `json:"seed"`
    A        FighterInput `json:"a"`
    B        FighterInput `json:"b"`
}
```

All hashes/IDs are exactly 64 lowercase hex characters encoding 32 bytes; zero is a valid encoding. `A.ProspectID < B.ProspectID` lexicographically is required; equal IDs fail. This fixes the corner ordering independently of the UI's selection order. `NewInput` below sorts the two complete fighter values; parsers and `ValidateInput` reject a noncanonical order. Names, abbreviated IDs and portraits never select a random stream.

Both manifests must pass the existing entire canonical `fightermanifest` validation/regeneration under version1, not just field-range checks. Their weight class must match exactly. Preserve the manifest's stored style; no age-based style recomputation. The pure package checks no proof of qualifying work; the stable adapter owns provenance. Distinct syntactically valid prospect IDs may have equal manifest inputs in model tests; that is not an admission assertion.

`0 < BornAtMS <= AsOfMS <= 8640000000000000` for both fighters. `ProXP <=15`, `TrainingSkill <=5`; reject invalid values, never clamp corrupt bonuses. Compute `fightercondition.Project(1, Manifest.DNA, BornAtMS, AsOfMS, int(ProXP))` internally for each fighter. Use its `AgedAttributesTenths`, not its XP-adjusted Skill, as the base for §5. This prevents double-counting XP. Both snapshots share the exact input as-of. The upper bound is the existing JavaScript Date limit, below MaxSafeInteger; the pure condition package retains its larger independent int64 API unchanged. No supplied condition object, birth rewrite or browser time override is accepted by the trusted integration.

The current condition/rating implementation remains unchanged. A retired condition is a valid model input; it does not become professional eligibility. No final attribute floor is added.

## 3. Fixed-point operations

Every combat quantity suffixed `Q` is signed int64 units of 1/1,000,000. All calculations use integer arithmetic. Every decimal constant in this document has at most six places and converts exactly to Q units. For example `.0022` is2200, `.45` is450000, `60` is60000000. Spaces in prose do not change these constants.

Define `R(n,d)` for signed n and strictly positive d as nearest integer division, exact ties to even: operate on magnitude, increment the quotient if `2*remainder>d`, or if equality and quotient is odd; restore the sign. Thus `R(-5,2)=-2`, `R(-7,2)=-4`. No negative zero exists. Checked arithmetic must reject an invalid denominator or overflow; implementations may use proven-safe int64 paths, but not wrap/saturate.

```
Mul(x,y) = R(x*y,Q)
Div(x,y) = R(x*Q,y)        // y must be positive
Clamp(x,lo,hi) = min(max(x,lo),hi)
```

An expression below is evaluated in its written nested order. `Mul(Mul(a,b),c)` is not `R(a*b*c,Q*Q)`. Plain addition/subtraction is exact. Multiplication by an unscaled integer such as a counter or inches is ordinary checked integer multiplication. The only clamps are those explicitly listed. No reassociation, float, FMA, platform `round` or random library is normative.

## 4. Seed identity and deterministic draws

`InputID` is lowercase hex SHA256 of:

```
ASCII("SLUG fighter bout input\0v1\0") || CanonicalJSON(Input)
```

It commits the complete validated input including seed, exact manifests, bonuses, birth/as-of and canonical corner order. The explicit seed has no special all-zero refusal.

Five independent streams have exact ASCII labels `form-a`, `form-b`, `chin-a`, `chin-b`, `combat`. Each starts counter0. The next raw word is:

```
h = SHA256(ASCII("SLUG fighter bout random\0v1\0")
           || raw32(InputID)
           || LE32(byte_length(label)) || ASCII(label)
           || LE64(counter))
word = BE64(h[0:8])
counter += 1
```

A shared raw-word count across all streams is incremented for every attempted word, including rejected words. Attempting word65537 fails `ErrStream` and returns no result. No streams are created from player text.

`Uniform(n)`, n>0, is unbiased on integers0..n−1: let `threshold = 2^64 mod n`; consume words until a word≥threshold, then return `word mod n`. Refuse after256 consecutive rejected words for a single call. Compute `2^64 mod n` without overflowing signed arithmetic. Each logical draw's stream is stated below. Probability `Chance(p)` is always `Uniform(Q) < p`, with p already in0..Q; it consumes a draw even if p=0 orQ. There is no float conversion.

Initialization, in this exact order:

1. form-a: `FormA = Uniform(200001)-100000`.
2. form-b: same for B.
3. chin-a: `JitterA = 920000+Uniform(160001)`.
4. chin-b: same for B.

Form lies inclusively in−.10..+.10 at one-millionth resolution. Chin jitter lies inclusively in.92..1.08. During all three rounds, `combat` continues from its previous counter; there is no round reset. The transition rules below define every conditional draw. Maximal normal logical draws are4 +180*6 +2*2 =1088; rejection/raw-word ceilings remain explicit fail-closed bounds.

This is an intentional versioned replacement for Python's MT/float draws, not a claim of byte parity with `random.Random`. The source equations are compared separately; statistical model compatibility is gated in §13.

## 5. Initial effective attributes and physical terms

For each aged tenths value t, `BaseQ = int64(t)*100000`. For each of the five attributes:

```
EffectiveQ[attr] = Mul(BaseQ[attr], Q+FormQ)
EffectiveQ[Skill] += int64(ProXP+TrainingSkill)*Q
MaxStaminaQ = 60*Q + Mul(EffectiveQ[Stamina],1400000)
KOThresholdQ = Mul(70*Q + Mul(EffectiveQ[Chin],450000), JitterQ)
```

Form applies to aged attributes, then the two separate approved bonuses are added once to Skill. XP is not aged again or added twice from the league projection. Neither bonus is multiplied by form. There is no unsupported final Skill120 clamp: the approved cap is+15 pro XP and+5 training, not a120 stat cap. The upper effective Skill endpoint is130. All five effective attributes, including Chin in both resistance equations, use the same form-adjusted values.

Initial stamina equals MaxStaminaQ; damage taken/dealt, thrown/landed/significant counts, scored knockdowns, control and per-round suffered knockdowns are0. Stamina never exceeds its initial maximum or goes below0.

Use these explicitly proposed minimal physical effects; they are engineering model changes requiring calibration, not previously approved multipliers:

```
ReachGap = Clamp(attacker.ReachIn-defender.ReachIn, -8, 8)   // inches
HeightGap = Clamp(attacker.HeightIn-defender.HeightIn,-8,8)  // inches
RangeHitQ = 2500*ReachGap + 1250*HeightGap
MassDamageQ = Q + 1000*Clamp(attacker.WeightLB-defender.WeightLB,-40,40)
```

The reach term is at most±.02 hit probability, height at most±.01, both before fatigue scaling. Mass changes damage by at most±4%. Natural weight also determines the existing same-class admission check; no new class boundaries. These terms do not relabel style, alter the fighter manifest, or change the league rating. There is no additional hidden style/size multiplier. Stance retains the source's open-stance hit term in §7.

## 6. Strike selection and initial per-tick order

Strike order, damage and power flag:

| index/name | DamageQ | power |
|---|---:|---|
| 0 `jab` |1200000|false|
| 1 `cross` |2300000|true|
| 2 `hook` |2800000|true|
| 3 `uppercut` |3300000|true|
| 4 `body` |1800000|false|

Style probability numerators out of100, in that same order:

| style | jab | cross | hook | uppercut | body |
|---|---:|---:|---:|---:|---:|
| slugger |25|25|25|15|10|
| outfighter |50|20|10|5|15|
| boxer_puncher |35|25|20|10|10|
| pressure |30|20|25|10|15|
| boxer |40|25|15|8|12|

At round start reset only both `RoundKnockdowns` counters to0. Preserve all other state. Each round has ticks1..60 unless a finish occurs earlier. At each tick:

```
fA=Div(A.StaminaQ,A.MaxStaminaQ)
fB=Div(B.StaminaQ,B.MaxStaminaQ)
wA=Mul(A.EffectiveSpeedQ,450000+Mul(550000,fA))
wB=Mul(B.EffectiveSpeedQ,450000+Mul(550000,fB))
pA=Div(wA,wA+wB)
```

Consume combat `Chance(pA)`; true chooses A as attacker, false B. This always has a strictly positive denominator (see bounds). Consume combat `Uniform(100)` and select the first cumulative style numerator strictly greater than the ticket. Increment attacker.Thrown. Deduct stamina1150000 for a power strike, otherwise800000, clamped at0. Hit calculation observes this post-cost stamina.

## 7. Hit, damage and significant shots

```
p = 300000 + Mul(att.EffectiveSkillQ-def.EffectiveSpeedQ,2200)
if att.Stance != def.Stance: p += 20000
p += RangeHitQ
p = Mul(p,720000+Mul(280000,Div(att.StaminaQ,att.MaxStaminaQ)))
p += Mul(max(0,550000-Div(def.StaminaQ,def.MaxStaminaQ)),100000)
p = Clamp(p,60000,780000)
```

Consume combat `Chance(p)`. On a miss, add500000 to attacker.ControlQ and end this tick. No damage, knockdown or referee draw occurs on a miss.

On a hit, increment attacker.Landed. Consume combat `Uniform(850001)` and set `DamageFactorQ=600000+ticket`. Compute exactly:

```
d = Mul(Mul(Mul(StrikeDamageQ,
                650000+R(att.EffectivePowerQ,100)),
            DamageFactorQ),MassDamageQ)
```

A jab adds500000 to attacker.ControlQ. A body hit removes `Mul(d,900000)` from defender.StaminaQ, clamped at0. `significant = power || d>=1600000`; if true increment attacker.Significant. Add d to attacker.DamageDealtQ and defender.DamageTakenQ. The `DamageQ` recorded for this tick is d, excluding the separate six-point knockdown addition.

The source's descriptive “huge strike” threshold4.2 has no state effect and is not a separate authoritative event. Presentation may derive emphasis from d; it must not add a second damage event.

## 8. Knockdowns and finish ordering

Preserve the source's significant-shot boundary for in-tick knockdown/KO/referee checks. A nonsignificant landed strike accumulates damage but consumes no knockdown/referee random word and does not itself cause an in-tick finish. This deliberate model choice is visible in calibration; do not silently move the checks.

For a significant hit, after the normal damage addition:

```
v = Div(def.DamageTakenQ,def.KOThresholdQ)
pKD = 8000 + Mul(max(0,d-3000000),18000)
             + Mul(Mul(v,v),220000)
             - Mul(def.EffectiveChinQ,120)
pKD = Clamp(pKD,0,450000)
```

Consume combat `Chance(pKD)`. If true: increment defender.RoundKnockdowns and attacker.Knockdowns; add3000000 attacker.ControlQ; add6000000 defender.DamageTakenQ and attacker.DamageDealtQ. If defender.RoundKnockdowns becomes3, finish immediately with the attacker winning, method`tko_three_knockdowns`; skip the remaining checks/draws.

Otherwise recompute `v=Div(def.DamageTakenQ,def.KOThresholdQ)`. If v≥Q, finish with attacker winning, method`ko`; consume no referee draw. If v≥850000 and v<Q, compute `pRef=100000+Mul(v-850000,1200000)` and consume combat `Chance(pRef)`; true finishes with attacker winning, method`tko_referee`. If v<850000 no referee draw occurs. Finish stops the tick/round/bout; later checks cannot overwrite it. Record the complete state after the finishing change.

At the end of a full 60-tick round, if no finish occurred:

- If this is round3, go directly to the scorecard. No post-final-bell recovery or corner stoppage.
- Otherwise for both fighters independently from their end-of-round state, apply:

```
recovery = Mul(MaxStaminaQ,100000+Mul(R(EffectiveStaminaQ,100),100000))
StaminaQ = min(MaxStaminaQ,StaminaQ+recovery)
DamageTakenQ = Mul(DamageTakenQ,970000)
```

Then determine corner outcomes without overwriting one with the other. Evaluate A, then B: if its post-recovery `Div(DamageTakenQ,KOThresholdQ)>=970000`, consume combat `Chance(350000)`; otherwise its stopped flag is false with no draw. Evaluate both before deciding. Exactly one flag true means the other fighter wins by`tko_corner`. Both true is a terminal draw with method`double_corner_stoppage`; it is a simultaneous finish, not a scorecard tie. Neither true continues the next round. This removes the source loop's last-writer winner and never runs after the final bell.

A terminal boolean is independent of winner identity. In particular, the double-corner draw must not later fall through to scorecard winner selection.

## 9. Whole-fight scores and result

Always compute both scorecards from final aggregate state, including on a finish. Damage-dealt totals never heal when damage-taken heals; no per-round score or round winner exists. Each displayed component is an integer number of tenths:

```
DamageTenths = R(DamageDealtQ,100000)
KnockdownTenths = 100*Knockdowns
AccuracyTenths = 0 if Thrown==0 else R(250*Landed,Thrown)
VolumeTenths = 6*Significant
ControlTenths = R(7*ControlQ,2*Q)
TotalTenths = DamageTenths+KnockdownTenths+AccuracyTenths+VolumeTenths+ControlTenths
```

Sum the rounded displayed components, matching the source's published-parts approach. Never round an unrounded sum instead. A non-finish ends after round3 tick60: larger TotalTenths wins by`decision`; equal totals give empty winner and method`decision_draw`. The old±2.0 margin is removed in accordance with D11. Do not conflate `decision_draw` and `double_corner_stoppage`.

Winner is the full canonical winning ProspectID, or the empty string only for the two draw methods. A KO/TKO winner cannot be empty. EndRound is1..3; EndTick is1..60. Corner finishes have EndTick60 and EndRound1 or2. A decision/draw has EndRound3 and EndTick60. The result never mutates input attributes, stable assignment, wallet, age, training, XP, rankings or records.

## 10. Transcript, codecs and public functions

All fields below are required, in listed order; arrays of length2 use corner order A,B. No arbitrary text logs enter normative bytes.

```go
type AttributesQ struct {
    Power int64 `json:"power"`
    Speed int64 `json:"speed"`
    Chin int64 `json:"chin"`
    Stamina int64 `json:"stamina"`
    Skill int64 `json:"skill"`
}
type Initial struct {
    FormQ int64 `json:"form_q"`
    JitterQ int64 `json:"jitter_q"`
    Effective AttributesQ `json:"effective"`
    MaxStaminaQ int64 `json:"max_stamina_q"`
    KOThresholdQ int64 `json:"ko_threshold_q"`
}
type State struct {
    StaminaQ int64 `json:"stamina_q"`
    DamageTakenQ int64 `json:"damage_taken_q"`
    Thrown uint32 `json:"thrown"`
    Landed uint32 `json:"landed"`
    Significant uint32 `json:"significant"`
    DamageDealtQ int64 `json:"damage_dealt_q"`
    Knockdowns uint32 `json:"knockdowns"`
    ControlQ int64 `json:"control_q"`
    RoundKnockdowns uint32 `json:"round_knockdowns"`
}
type Tick struct {
    Number uint32 `json:"number"`
    Actor uint32 `json:"actor"` // 0=A,1=B
    Strike string `json:"strike"`
    Hit bool `json:"hit"`
    DamageQ int64 `json:"damage_q"`
    Significant bool `json:"significant"`
    Knockdown bool `json:"knockdown"`
    Stoppage string `json:"stoppage"` // empty or one in-tick finish method
    States [2]State `json:"states"`
}
type Recovery struct {
    States [2]State `json:"states"`
    CornerStopped [2]bool `json:"corner_stopped"`
}
type Round struct {
    Number uint32 `json:"number"`
    Ticks []Tick `json:"ticks"`
    End [2]State `json:"end"` // after ticks, before recovery
    Recovery *Recovery `json:"recovery"`
}
type Score struct {
    DamageTenths uint32 `json:"damage_tenths"`
    KnockdownTenths uint32 `json:"knockdown_tenths"`
    AccuracyTenths uint32 `json:"accuracy_tenths"`
    VolumeTenths uint32 `json:"volume_tenths"`
    ControlTenths uint32 `json:"control_tenths"`
    TotalTenths uint32 `json:"total_tenths"`
}
type DrawCounts struct {
    FormA uint32 `json:"form_a"`
    FormB uint32 `json:"form_b"`
    ChinA uint32 `json:"chin_a"`
    ChinB uint32 `json:"chin_b"`
    Combat uint32 `json:"combat"`
}
type Result struct {
    Version uint32 `json:"version"`
    EngineID string `json:"engine_id"`
    InputID string `json:"input_id"`
    Initial [2]Initial `json:"initial"`
    Rounds []Round `json:"rounds"`
    Final [2]State `json:"final"`
    Winner string `json:"winner"`
    Method string `json:"method"`
    EndRound uint32 `json:"end_round"`
    EndTick uint32 `json:"end_tick"`
    Scores [2]Score `json:"scores"`
    DrawCounts DrawCounts `json:"draw_counts"`
}
```

One Tick is emitted for every executed tick, including misses and the finishing tick. Rounds are contiguous starting1; ticks contiguous starting1. End equals the last tick states. Recovery is non-null exactly after a complete non-final round without an in-tick finish, including a terminal corner finish. Final equals the final Recovery states if present, otherwise final End. The counts are raw words consumed in each stream including rejected words, not logical draw counts.

Canonical JSON is compact UTF-8 with exactly the declared field order, no whitespace/newline, all required fields, no HTML escaping, and no trailing data. Strings are canonical field/enum values and existing manifest strings; no alternative escaped spelling. Integers use minimal base10 (`0` or optional minus followed by nonzero digit and digits), no`-0`, float, exponent or plus. Negative values occur only in Initial.FormQ and the existing manifest shoulder_jitter. Arrays are non-null; Recovery alone may be null. Reject unknown/duplicate fields, unknown versions/identifiers, malformed UTF-8, depth>8 and bytes beyond caps before large decoding. Container depth counts each object/array from root1. A valid maximum result is below the cap per §11.

Go canonical encoding must use a fixed struct order and disable HTML escaping. The nested existing Manifest has its frozen field order; this contract does not modify its independent codec or claim condition v1 already had a digest. Parse functions require byte equality to canonical re-encoding after semantic validation, so whitespace/order variants are deliberately rejected.

Exact proposed API:

```
NewInput(asOfMS int64, seed string, a,b FighterInput) (Input,error)
ValidateInput(Input) (Input,error)
ParseInput([]byte) (Input,error)
MarshalInput(Input) ([]byte,error)
InputID(Input) (string,error)
Simulate(context.Context,Input) (Result,error)
ValidateResult(context.Context,Input,Result) (Result,error)
ParseResult(context.Context,Input,[]byte) (Result,error)
MarshalResult(context.Context,Input,Result) ([]byte,error)
Digest(context.Context,Input,Result) (string,error)
```

NewInput supplies the four fixed selectors and sorts A/B; it does not lowercase strings or repair invalid manifests/bonuses. ValidateInput returns an owned canonical value without sorting or other normalization. Result contains only InputID, not a second copy of Input or its condition snapshots. Every result validation/codec/digest call takes the separately retained Input, regenerates the complete simulation and requires exact semantic/byte equality; no caller `Verified` bypass, winner override, random callback or partial-result trust. MarshalResult/Digest validate; ParseResult bounds then validates/re-encodes. Digest is lowercase SHA256 of `ASCII("SLUG fighter bout result\0v1\0") || CanonicalJSON(Result)`. It authenticates no issuer or seed provenance.

Errors are exported sentinels `ErrInput`, `ErrResult`, `ErrStream`, `ErrArithmetic`; cancellation returns `ctx.Err()`. All failures return the zero output/string/nil bytes, never a partial transcript. Public simulation checks context before input work, after input validation, before each tick, between rounds and before publication. Context cancellation does not spawn detached work. Private test-only arithmetic/draw seams may exercise rare boundaries; no runtime exported arbitrary random function.

## 11. Proven bounds and absence of undefined arithmetic

Canonical aged attributes are positive: the smallest Speed is.6, Chin/Stamina.7, Power.8 and Skill.9. After minimum form, Speed≥.54, Chin/Stamina≥.63, Power≥.72, Skill≥.81. All non-Skill effective attributes≤110; Skill≤130. Thus MaxStamina is in[60,214] units and KOThreshold in[64,130] using conservative outward bounds. No stamina or hurt denominator can be0. Even at zero stamina, initiative weight≥.54*.45=.243, so wA+wB>0.

Damage is positive and less than9 units: worst factors are3.3*(.65+1.10)*1.45*1.04=8.7087 before at most three half-unit-in-Q rounding errors; the <9 bound is ample. Each of180 ticks adds at most damage<9 plus6 knockdown damage to aggregate totals, so DamageDealt/DamageTaken<2700 units. Healing only reduces taken damage. Control≤180*3.5=630 units. Thrown/Landed/Significant/total knockdowns≤180; RoundKnockdowns≤3. (Tighter real finish limits are not required for safety.)

Conservative largest hurt fraction is<2700/64<43, represented by<43,000,000. Squaring for Mul forms a product<1.849e15; its scaled result times220000 is<4.068e14. The largest generic Div numerator needed here is<2.7e9*1e6=2.7e15. Initiative numerator≤110e6*1e6=1.1e14. Damage chains and stamina/recovery products are below1e15. All are well below MaxInt64≈9.22e18; all additions/subtractions also fit. Signed hit/geometry terms are bounded; implement R's sign handling without ever negating MinInt64 (unreachable after validation, checked in the private primitive).

Score components fit uint32: conservative DamageTenths<27000, KnockdownTenths≤18000, AccuracyTenths≤250, VolumeTenths≤1080, ControlTenths≤2205; total<48535. Do not narrow until these checked operations complete. Input times are ordered before subtraction and bounded by the Date limit8640000000000000; fightercondition retains its own safe duration arithmetic.

Serialization is independently bounded: one State is<256 bytes at the listed numeric maxima. One Tick is<768 bytes including two states and all longest enums.180 ticks therefore consume<138240 bytes; round ends/recovery, initials, final state, counters, scores and fixed fields add<8192. The Result has no repeated Input or condition snapshots: its total is therefore<146432 bytes, below196608. A separately wrapped request/result may carry the exact two public input items once within the existing262144-byte helper frame; that adapter must prove and enforce its own envelope bound. Max nested container depth is7; cap8 is defensive. No unbounded logging/string arrays exist. These inequalities must get independent arithmetic/maximum-serialization tests before the final contract freeze; a failure requires revising the draft, not silently enlarging runtime caps.

## 12. Declared source/model changes and reference acceptance

The reviewed source intent retained: three rounds/60 ticks, style strike mixtures/damage, speed/fatigue initiative, skill/speed/stance hit model, fatigue clamps, body damage, significant shot definition, cumulative damage/control, source-style knockdown/referee checks, stamina/damage recovery and whole-fight displayed score components.

Explicit changes: canonical full identities/current manifests and exact aged values; fixed-point arithmetic; SHA256 integer-grid streams; bounded physical reach/height/mass effects; form-adjusted Chin in resistance; distinct validated bonuses and no Skill120 clamp; exact published-score ties; no post-final-round recovery/corner; simultaneous corner outcomes with explicit terminal draw. Each difference is measured, not disguised as exact source parity. The static D65 formula is not changed.

Before Go, an independent Python integer implementation must derive exact inputs/transcripts/digests from this frozen contract and immutable independent DNA/manifest/condition sources, never future Go output. Include every style/strike, both stances, geometry/mass clamp edges, form/jitter/damage endpoints, signed rounding ties, minimum retired values, zero stamina, all finish paths, double corner, exact/adjacent score ties and all conditional draw counters. Primitive rare-boundary vectors may use private model states to prove unreachable/rare branches; public complete-result vectors must begin with valid canonical input. Label these separately.

The prototype baseline remains byte-for-byte archived. The historical rating measurement correction is separately recorded in `set42-rating-baseline.md/.json`: old draw-order accounting awarded full credit to some draws; correcting the two scoring sites changes metrics but not the8,965 simulated bouts, fit weights or career records. It is not calibration of this new engine.

## 13. Predeclared calibration and release gates

This section must be reviewed/frozen **before running this engine or fitting outcomes**. It is a release gate, not a golden output to be filled from Go.

### Dataset construction

Use the frozen independent manifest and condition generators. A manifest seed is SHA256 of a framed calibration label: `ASCII("SLUG fight calibration\0v1\0") || LE32(label length)||ASCII(label) || LE32(index)||LE32(corner)||LE32(attempt)`. Label choices are listed below; corner0/1. Attempt starts0 and increases until the desired canonical weight class is obtained; stop/fail at4096 attempts. No outcome-based rejection. ProspectID is SHA256 of `ASCII("SLUG fight calibration identity\0v1\0") || raw32(manifest seed) || LE32(corner)`; NewInput sorts complete corners. Common birth/as-of is1700000000000 for prime cases. Bout seed is SHA256 of `ASCII("SLUG fight calibration bout\0v1\0") || LE32(label length)||ASCII(label)||LE32(index)`. All dataset IDs/inputs are frozen before simulations.

Prime labels are exactly `prime-light`, `prime-welter`, `prime-middle`, `prime-light_heavy`, `prime-heavy`. For each class generate1200 pairs indexed0..1199; within each class indices0..799 are training and800..1199 held out. This gives4000 training and2000 held-out same-class bouts, all XP/training0. No engine parameter is chosen from held-out outcomes. This is separate from the original6000 unconstrained prototype pairings; do not equate the two populations.

Decline labels are `decline-` followed by those five class strings,300 pairs per class indexed0..299. Side A before canonical sorting is observed at `Born+(PrimeWeeks+floor(index*DeclineWeeks/300))*604800000`; Side A birth is1700000000000. Side B birth equals that exact AsOfMS, making B age0 at the common observation. Use account XP values `index mod16` for A and0 for B, training0. These1500 cases are diagnostics, not additional fitting observations. Include a separate fixed endpoint grid of retired phase/XP0,15/training0,5/form-state primitive cases; do not discard unfavorable endpoints.

Bias labels are `mirror-` followed by class,400 pairs per class. Generate one manifest using generator corner0 and its first accepted attempt. Use that same manifest seed and identical manifest/birth/XP0/training0 for both sides; derive the two ProspectIDs from that one manifest seed with identity suffix corner0 andcorner1, then canonicalize the corners. Seeds differ by index as above. These2000 valid pure-model inputs measure corner-order bias independently of phenotype. They are not claimed distinct admitted fighters.

### Fitting and measurement

Use independent Decimal arithmetic precision50, round-half-even, in the fixed input order class order(light,welter,middle,light_heavy,heavy), then index. Attribute order is Power,Speed,Chin,Stamina,Skill. On each training pair use current league attributes (aged plus accounted XP, no training/form): each Decimal feature is exactly `(LeagueAttributesTenthsA[attr]-LeagueAttributesTenthsB[attr])/500`, which is the attribute-unit difference divided by50. Do not divide the raw tenths by50. Labels are1 for canonical A win,0 for B win,.5 for either draw reason. Fit the existing five-coefficient logistic model: start all weights0;400 full-batch iterations; learning rate.5; sigmoid `1/(1+exp(-clamp(dot,-30,30)))`; each operation rounded at the declared Decimal context; gradients accumulated as an explicit left fold in dataset order. Divide by sample count4000. No intercept, regularization, early stopping, hyperparameter search or holdout tuning.

Normalize by arithmetic mean of the five fitted weights; if mean≤0 or any normalized weight≤0, fail calibration. Round each normalized weight independently to the nearest tenth, ties-even; do not renormalize afterward. Canonical comparison order is Power,Speed,Chin,Stamina,Skill; current D65 is[1.0,2.5,.2,.4,.8]. Publish full precision, normalized and rounded coefficients.

Prediction scoring checks draws **first**: a drawn outcome scores.5 for every predictor; tied predicted ratings also score.5; otherwise correct favorite1/incorrect0. Report exact numerator in half-points and denominator, not only rounded percentages. On the new-engine outcome labels, score three distinct predictors: the continuous fitted coefficients, unchanged D65, and unweighted sum of the five current league attributes. The source-model baseline is a separate outcome series on these inputs, not a fourth predictor of the new outcomes. If predictor accuracy against source outcomes is also reported, label its outcome series explicitly and do not substitute it for the new-engine gate. Use the unchanged source equations with exactly the same immutable physical/attribute inputs as a labeled numerical baseline; do not call the old generator or feed current XP twice. Its own floats/draw margin remain historical comparison behavior, not accepted expected bytes.

### Pass/fail criteria before shipping

All are required; results and failures remain archived:

1. Exact independent-vs-Go transcript/digest agreement on the frozen corpus and all bounded arithmetic/decoder tests; no invalid state or stream failure in the fixed calibration inputs.
2. The rounded refit equals the current D65 tuple exactly. A different tuple does not silently replace fightercondition v1, even if it predicts well. Prepare the concrete evidence/version proposal for Mike or revise the unshipped model through a newly recorded contract/calibration iteration.
3. On the2000 held-out prime bouts, unchanged D65 scores at least75%, and is no more than2.0 percentage points worse than the continuous fitted predictor. Publish all class strata and 95% bootstrap uncertainty using a separately frozen resampling seed; uncertainty is diagnostic, not a hidden pass-rule override.
4. On the same held-out inputs, finish proportion differs by no more than10 percentage points from the source-model baseline and mean EndRound by no more than.25. This is the arithmetic mean of the integer last-entered round1..3 across all2,000 bouts, not elapsed ticks/60; the old source does not expose elapsed tick counts. New-engine tick duration may be an additional diagnostic only. Draw fraction is≤10%; no per-class draw fraction exceeds15%. These are predeclared broad model-compatibility gates, not a claim the old 50–53% finish figure applies to same-class/current manifests.
5. In the2000 identical-phenotype bias bouts, at least1500 are decisive and `abs(AWins-BWins)/Decisive <= .05`. Publish the exact counts and class strata. No input reordering or seed reroll may repair a failed bias result.
6. Physical-effect paired diagnostics, all finish reasons, form/bonus endpoints, primed/declined comparisons and retired bounds are published even if they are not fitting data. Calibration does not imply that every stronger single attribute wins each individual stochastic fight. No adding a post-hoc success threshold or omitting an adverse stratum.

### Exact remaining diagnostic recipes

Bootstrap uses 1,000 stratified replicates of the held-out prime set. For each replicate in order0..999, each class in the fixed order, draw400 indices from that class’s400 held-out cases with replacement. All predictor scores and paired differences use those identical sampled indices. The stream key is SHA256 of `ASCII("SLUG fight calibration bootstrap\0v1\0")`; raw word i is BE64 of the first8 bytes of SHA256(key||LE64(i)), counter starting0. Uniform(400) uses the rejection algorithm from §4 with256 attempts per selection; bootstrap has its own total ceiling10,000,000 raw words, not the engine's65536 ceiling. Sort the1,000 replicate statistics; the reported central95% interval endpoints are elements24 and974 (zero-based, nearest-rank convention). This diagnostic does not rerun fitting or override a pass criterion.

The physical paired diagnostic uses prime training indices0..39 of each class (200 inputs). Compare the full proposed model with three separately labelled private counterfactual calculations: reach contribution zero only; height contribution zero only; mass factorQ only. Keep each original InputID and random streams identical; changed branching may consume later draws differently. These counterfactuals are mathematical diagnostics, not alternate supported EngineIDs or public expected-result vectors. Report changed outcomes, finish/round statistics and scores, stratified by the signed natural reach/height/weight gap. No outcome filtering or coefficient fitting uses these cases. Independently test that each immediate hit/damage equation is monotone in its own proposed physical advantage with all other operands fixed, allowing the documented clamps/rounding plateaus; do not require monotone stochastic winners.

Canonical retired endpoint diagnostics use labels `endpoint-` plus the five class strings,32 pairs per class, the same generation/seed recipe. Both births are1700000000000. Define `CareerWeeksA=A.Manifest.DNA.PrimeWeeks+A.Manifest.DNA.DeclineWeeks` and likewise for B (these are local sums, not DNA fields); as-of is birth plus `max(CareerWeeksA,CareerWeeksB)*604800000`. Before corner sorting, A gets ProXP15 iff(index bit0), TrainingSkill5 iff(bit1); B gets ProXP15 iff(bit2), TrainingSkill5 iff(bit3), otherwise0. This supplies160 valid public input pairs; it is diagnostic only, not fitting data. Form/jitter endpoints are covered in independent private initial-state tests because public seeds may not conveniently draw exact endpoints.

The private initial-state grid is all32 combinations of aged tenths Power{8,1000},Speed{6,1000},Chin{7,1000},Stamina{7,1000},Skill{9,1000}, crossed with ProXP{0,15},TrainingSkill{0,5},FormQ{−100000,0,100000},JitterQ{920000,1000000,1080000}:1,152 exact arithmetic states. This is a primitive coverage grid, not hand-edited canonical fighter input. Test physical gaps at reach{−8,0,8},height{−8,0,8},weight{−40,0,40}, and exact adjacent values reach/height{−9,−7,7,9},weight{−41,−39,39,41}, with explicit expected equation results. No private state constructor becomes a production API.

The source-model baseline uses only the immutable imported `slug-sim-export/sim/fight_sim.py`, SHA256 `37ff08917ed720e3d9c4f3b14508c57b08d830f9f4cb01833fa412dbca9b6422`, under CPython3.14.3 as recorded in `set42-rating-baseline.json`; do not patch its simulator. Instantiate its Fighter with seed=Manifest.DNA.Seed, fid=full ProspectID, name=Manifest.Name.Display, tier mapped from Manifest.DNA.Rarity as club→Club, professional→Professional, contender→Contender, elite→Elite, generational→Generational, attrs copied from the original mined DNA in exact uppercase Power/Speed/Chin/Stamina/Skill keys, height_in/reach_in/weight_lb and career lengths copied from the manifest/DNA. Stance mapping is orthodox→Orthodox, southpaw→Southpaw. Class mapping is light→Light, welter→Welter, middle→Middle, light_heavy→Light Heavy, heavy→Heavy. Style mapping is slugger→Slugger, outfighter→Outfighter, boxer_puncher→Boxer-Puncher, pressure→Pressure, boxer→Boxer. The baseline must not call its generate_fighter/derive_style.

Call `simulate_bout(fa,fb,input.Seed,title=False,featured=False,skill_bonus_a=A.ProXP+A.TrainingSkill,skill_bonus_b=B.ProXP+B.TrainingSkill,verbose=False,base_a=agedA,base_b=agedB)` in the canonical engine corner order. Each aged base is the internally recomputed exact tenths divided by10 into Python float, with the same five uppercase keys. Retain original source float calculations, RNG, draw margin and conditional Skill120 clamp in this labelled baseline. No extra seed prefix is added. Baseline win labels are source winner fid matching A/B or.5 for no winner; finish means the source method is neither Decision nor Draw. New-model finish means method is neither decision nor decision_draw; double_corner_stoppage is both a finish and a draw. Every final condition/record seed remains unchanged. Original-Python historical runs and corrected metric-only reports remain distinct from these new-input baseline runs.

All raw inputs, label lists, partition membership, normalized weights and outputs must be archived before release review. After any model revision informed by held-out outcomes, record a new contract/iteration and commit a disjoint new held-out label namespace before the next fitting run; preserve the old failure and never raise a threshold post-hoc. Current D65 compiled code remains unchanged throughout planning. A new engine cannot ship under a candidate label while any gate is unresolved.

## 14. Integration handoff and explicit non-claims

The pure explicit seed boundary can later receive a seed selected by a separately trusted future-block protocol; that service must establish lock timing, canonicality/reorg behavior, per-round arrival and publication itself. No `fair`, `eligible`, `owned`, `final`, `official` or `xp_awarded` field exists here.

Before the preview implementation, freeze a small separate integration appendix: exact two-ID stable read and original-view lease; helper-derived preview seed recipe; bounded single-lane begin/status/cancel; current-view response correlation; one owned latest transcript and cache budget; no changes to stable operation/roster schema. It must preserve the existing frame versions and permit Stop/logout during input proof work. The engine contract alone does not authorize a broad command passthrough or a new demo namespace.

D74 review, permanent genesis/production activation, real provider/wallet resources, actual exhibition/pro records and training/XP/result authorities remain separate. Root approval of this draft will authorize only the explicitly stated unshipped model implementation and independent evidence generation.

## 15. Draft review evidence

The independent formula-only serialization check is archived at `set42-serialization-shape-check.py/.json`. It evaluates synthetic bounded shapes, not engine outputs or accepted bout goldens: conservative Input16,905 bytes, State183 bytes, Tick525 bytes, Result98,585 bytes and depth7. This is below the intentionally looser analytic bounds in §11 and the fixed192KiB result ceiling. It performs no simulation or model fitting. Independent model review and historical baseline evidence are separately recorded in `set42-fight-model-review.md/.json` and `set42-rating-baseline.md/.json`. These reviews do not establish statistical calibration or authorize a release.
