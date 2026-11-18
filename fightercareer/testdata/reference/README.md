# Independent fighter-career arithmetic reference

This small Python-standard-library corpus derives `Award`, `Apply` and
`QualityBonus` expectations from the pinned `FIGHTER_CAREER.md` contract and
D41/D42/D78 spec. It was authored before any new Go implementation was read or
executed. No Go, engine, Argon, network, clock, private keys or prior corpus is
an input. `input_index.json` records exact original document hashes and sizes.

`generate.py` regenerates the exact compact, sorted-key JSON in `corpus.json`.
It verifies fixed arithmetic anchors, distinct delta shapes, both-corner
consistency, uint32 domain/overflow boundaries, XP caps and sequence totals.
To check without writing:

```sh
python3 generate.py --check corpus.json
```

Use `--write /fresh/output.json` to generate a new exclusive destination.
No existing corpus is overwritten. Outputs contain no timestamps, host paths,
Python versions or environment-dependent values. Independent execution evidence
and the two-interpreter byte comparison are in the external Set49 report.

The JSON test schema is not a proposed application codec. `award_cases`,
`apply_cases`, and `quality_cases` each contain `id`, `input`, and `expected`.
Expected errors are the symbolic package names `ErrInvalid` or `ErrOverflow`;
successful cases use `error: ""` plus `deltas` (A then B), `totals`, or `bonus`.
Delta/Totals fields are `wins`, `losses`, `draws`, `xp`, `base_points`.
`apply_sequences` contains an initial Totals and successive delta/total pairs.
Consumers must test the named error, not derive expected results from Go.

All numerical inputs are uint32-representable, matching the value API. JSON
booleans, negative/out-of-range numbers, unknown fields and malformed serialized
input are not this package's contract: there is no production JSON importer.
The contract does not prescribe output values on errors or precedence when an
input is both invalid and overflowing. The corpus does not invent either:
invalid cases isolate validation, and overflow cases use valid prior XP/deltas.

Modeled title/rank arithmetic confers no title status, rank, ownership,
professional-result authority or finality. Rank zero is rejected, not treated
as unranked; known-unranked handling belongs to an authenticated higher-level
profile. BasePoints exclude quality bonuses. Standard/title XP is +1/+2 total
for each completed fight, including draws, with applied XP capped at 15.
Zero applied XP never suppresses record/point accumulation. Repeated Apply
deliberately accumulates again; exactly-once D78 behavior is not claimed here.
No rolling ranking window, title allocation, C1 widening or condition rule is
introduced by these finite arithmetic cases.
