# Change set 13: deterministic fighter DNA foundation

Branch: `slug-fighter-dna`, based on the released master merge `e056146f`.
Mike said to keep going after the chain-core rollout and the next fighter
workstream were explained. This set is pushed as its own branch; deploying
it to the Railway seed is not part of this change set.

## What changed

- Added a pure off-chain `fighter` package and offline `slugfighter` command.
  DNA version 1 maps a displayed 32-byte work hash to the settled rarity
  pyramid, exact talent budget, five attributes and prime/decline lengths.
- Specified byte order, domain separation, integer rejection sampling,
  within-tier budgets and uniform bounded-composition allocation in
  `FIGHTER_DNA.md`. A different mapping requires another DNA version.
- Added an independent Python standard-library oracle. Its eight fixtures
  cover all five rarity tiers, public chain hashes and byte-order edge cases.
  The oracle uses inclusion–exclusion; Go uses dynamic programming. Expected
  values were derived without reading Go source or copying Go outputs.
- Updated the work plan to distinguish the completed chain-core rollout
  milestone from full Phase A, which still needs registration and the pool.

Uniform budget selection and uniform feasible attribute tuples are explicit
engineering choices within the spec's open DNA tuning. Independent labeled
SHA-256 streams condition the work digest before sampling, so the raw
leading bits constrained by PoW do not directly select rarity. This does
not prevent miners from choosing which discoveries to reveal.

## Deliberately unchanged / remaining work

No consensus, genesis, PoW, node, checkpoint, ownership or payout behavior
changed. This is a DNA-component version, not a finished full fighter
generator. Names, physical traits, style, portraits and authenticated birth
anchors remain ahead. The preview accepts any 32 bytes; it neither proves
qualifying work nor registers or grants ownership of a fighter.

Before registration, define the prospect target, pre-work gym binding,
pool/solo evidence, freshness/replay handling, authenticated birth anchor,
ownership/transfer publications and checkpoint anchoring. A copied public
share must not become a valid competing ownership claim. No operator
private material is needed or accessed.

## Verification

- Independent reference self-check: eight complete records across all five
  tiers; enumeration checks for combinatorial counts and lexical unranking,
  rejection behavior, normalization and fixture regeneration passed.
- `go test -race ./fighter ./cmd/slugfighter -count=1`: passed (1.252s and
  1.452s package times). Includes fixed vectors, rarity boundaries,
  rejection thresholds, nonzero stream counters, bounds, exact sums,
  concurrent reproducibility, CLI errors and unsupported versions.
- `go vet ./fighter ./cmd/slugfighter`: passed during independent review.
- Normal builds of all ten modules: passed; no dependency changes.
- Required normal-PoW smoke: version 0.27.0; isolated node mined two blocks,
  confirmed the first block's 10 SLUG subsidy, reported the local checkpoint conflict,
  adopted the public seed's 61-block chain normally, reported finalized with
  60 descendants and no conflict, passed `verifychain 1 61`, then retained
  finality after a clean restart. Startup checkpoint logs matched.
- Smoke evidence: `/private/tmp/slug-set13-smoke.json`; temporary data/logs:
  `/private/tmp/slug-set13-smoke-_1yatgf0`.

The earlier complete tagged blockchain/integration suite evidence remains
in set 9. This off-chain addition does not require rerunning those expensive
unmodified suites. The known CI/address-vector debt remains separate.
