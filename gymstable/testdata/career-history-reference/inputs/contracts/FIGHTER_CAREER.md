# Fighter career arithmetic v1

This is pure arithmetic for D41, D42 and D78. It grants no authority, finality,
ownership, title or ranking position. Only a trusted finality/accounting path
may apply its output. Historical DNA, saved bout inputs and D65 stay unchanged.

## Inputs and outputs

`fightercareer.Award(winner, method, kind)` returns two `Delta` values in fixed
corner-A/corner-B order. `winner` is exactly `a`, `b` or `draw`; `kind` is exactly
`standard` or `title`. Each Delta contains uint32 `Wins`, `Losses`, `Draws`,
`XP`, and `BasePoints`. No other outcome/kind is accepted.

- `decision` requires winner `a` or `b`: winner gets one win and ten base
  points; loser gets one loss and zero points.
- `ko`, `tko_referee`, `tko_three_knockdowns`, `tko_corner` require winner `a`
  or `b`: winner gets one win and fourteen base points; loser one loss/zero.
- `decision_draw`, `double_corner_stoppage` require winner `draw`: both get
  one draw and four base points. A double-corner stoppage is not two wins.
- Both corners earn one XP for standard and two XP total for title, regardless
  of win/loss/draw. This arithmetic does not classify a real bout as a title.

`fightercareer.Apply(prior, delta)` returns a `Totals` with uint32 Wins, Losses,
Draws, XP and BasePoints. Prior XP must be 0..15, and each delta must be exactly
one of the Award-produced per-corner shapes, including its base points and XP.
Reject rather than wrap any uint32 counter/point addition. Add the counters and
base points; set XP to min(15, prior.XP + delta.XP). Zero applied XP at the cap
does not suppress the bout record or points. This function does not deduplicate:
the trusted event history must include each original result exactly once.

`fightercareer.QualityBonus(opponentRank)` returns six for ranks 1..5, three
for 6..10 and zero for a known rank above ten. Rank zero is an error: absent
rank evidence is not known-unranked evidence. The caller uses this only for an
authenticated win and a trusted fight-time rank. Known-unranked opponents have
zero quality bonus by the higher-level authorized profile, not by guessing a
missing rank here. Quality points remain separate from BasePoints.

These functions return the package's `ErrInvalid` for invalid inputs and
`ErrOverflow` for arithmetic overflow. All inputs are values; outputs own their
values and have no mutable aliases. There is no clock, I/O, random choice,
hashing, JSON import, global state or engine simulation in this package.

## Independent verification

Generate expected deltas and totals independently before implementing Go.
Cover every supported method/corner/kind, malformed winner/method combinations,
prior XP 0/13/14/15 and invalid 16, replay accumulation as arithmetic rather
than an idempotency claim, maximum counters and points, malformed delta shapes,
and quality ranks 0/1/5/6/10/11/max-uint32. Preserve a clear distinction between
pure modeled title/rank arithmetic and actually authorized standard candidate
bouts. This contract makes no rolling-ranking window or title allocation choice.
