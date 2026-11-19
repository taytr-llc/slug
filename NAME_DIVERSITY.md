# Explicit candidate name diversity

Set65 adds an opt-in name generator with 512 first names and 512 surnames:
262,144 possible display strings, compared with v1's 1,024. Duplicate names
remain allowed under D51. Fighter IDs remain the identity keys. Existing
fighters, sources, receipts, portraits, defaults and v1 expected vectors keep
their original bytes.

The exact reviewed candidate is the complete descriptor
`cd02ae57b5918548244be2cabbe6c0c433feb5a1c165745e2e5c6bf235128e75`.
It retains descriptor schema1, DNA1 and portrait traits1, with manifest2 and
renderer2. Resolve the complete tuple; separately supported components cannot
be combined into another supported descriptor. Production resolution remains
unavailable.

## Frozen generation and appearance

The normative bundle is `fighterdescriptor/testdata/name-v2`. Its approval,
exact contracts, names-only table, inherited v1 inputs and independent reference
are archived together. `independent/reference-v2.json` is independently derived;
production outputs must never replace its expected values.

Only names use the new v2 random streams. For the same seed, complete DNA,
physical values, style and portrait traits equal v1 exactly. The renderer uses
the same geometry, palettes and integer arithmetic. Accessible name text and
renderer metadata change, with a separate v2 SVG digest domain. Renderer1
continues to refuse manifest2, and renderer2 refuses manifest1. Full manifest
regeneration remains mandatory before drawing or accepting assigned metadata.

The fight engine and its complete-input hashing contract remain unchanged.
Different manifest bytes change commitments and can change the fight RNG input;
equal traits do not imply equal simulated outcomes across a hypothetical
version substitution. Retained fighters and fight inputs are never substituted.

The fixed name components are ordinary names authored from general knowledge,
with no scraped roster or demographic inference. The bundle preserves content
review and provenance. Similar spellings, repeated components and accidental
real-name combinations remain possible. This is not exhaustive multilingual or
trademark clearance; no blacklist, reroll, suffix or uniqueness promise is added.

## Explicit source selection

Existing constructors and packaged defaults still select v1. `PoolOptions`
permits the exact v2 descriptor through additive constructors. It is a fresh
candidate source choice, not an upgrade of an existing issuer's history.

Native and helper command lines require all of the following for v2:

- An explicitly true `--candidate-pool` flag.
- `--candidate-name-version=2`.
- An explicitly supplied, clean absolute `--data-dir`.

Unknown selectors and non-pool combinations are rejected before storage setup.
An established v2 root requires the same explicit choice on reopen; a default
v1 launch cannot infer v2 from files. Opening an established v1 root with v2
also fails. Outer account, inner runtime and source markers are checked before
their locks or journals open, and checked again at the owning boundaries.
Missing established components cannot be recreated to make a mismatch pass.

Each issuer, job, receipt, recipient, worker, stable and event pipeline retains
its complete original source. Generation, delivery, card/bout reconstruction,
typed transport, native portrait checks and frontend validation select the
exact source-bound tuple. Catalog membership never replaces source equality.
The original solo practice routes stay v1; shared practice remains disabled.

## Collision evidence

The independent study fixes the same public seed sequence and population
scenarios before generation. It reports both an independent-uniform model and
observed counts from 100,000 distinct SHA256-derived seeds. These are scenarios,
not production forecasts or claims about admitted mining work. A probability
that rounds to 100% is distinguished from mathematical certainty.

The Go implementation reproduces the observed counts and seed-stream digest
exactly. At 100,000 sample fighters, v2 has 82,987 distinct names and 31,969
fighters with a namesake (31.969%); v1 has 1,024 distinct names and every sampled
fighter has a namesake. At 10,000, v2's namesake fraction is 3.99%. A larger pool
reduces repeated names; it does not eliminate them.

The full model, six observed scenarios, component frequencies and reproducible
sample definition are in `independent/collision-report.json` within the bundle.
The corpus also covers every first/surname index, actual apostrophes, complete
manifest and SVG bytes, unchanged v1 outputs, stream counter boundaries and
invalid tuples. No numeric collision tolerance or launch population is adopted.
The remaining name presentation and collision tradeoffs belong in the final
joint D74 review with Mike.
