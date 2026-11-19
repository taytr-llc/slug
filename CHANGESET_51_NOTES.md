# Change set 51 — finalized fighter history

Work is on `slug-fighter-history`, stacked from completed Set 50 maintenance
`1cdfdb8a156624c97fbad512cb5386d1c5797f4c`. This first commit freezes the
independent contract and reference before runtime implementation.

`CAREER_HISTORY.md` defines an explicit authenticated capture of one fighter’s
already-effective awards, followed by immutable newest-first pages. It keeps
earned XP distinct from applied XP at the cap, preserves the complete original
result identity, and labels base points as history rather than rankings.
Its historical draft paragraph is retained byte-for-byte; the frozen approval
in `gymstable/testdata/career-history-freeze.json` supersedes that status.

The reference contains 28 histories, 35 core and four index controls, 65 typed
wire values, 67 codec controls, 45 protocol exchanges and 69 symbolic lifecycle
models. Two Python versions reproduced the exact output. Independent reviews
cover math/input provenance and the separately authored wire/model partition;
root verified all 140 indexed file hashes and reproduction evidence. Original
candidates and findings remain preserved, including the clarification that
native job discovery precedes wire response correlation.

Genuine inputs come from the already verified Set 49 public scoped-clock
fixture. Large histories, ties and capacity/lifecycle cases are explicitly
modeled. This reference repeats no proof hashing, signing, chain activity or
engine work and does not claim actual application authorization or latency.

D62/D78, D65, C1 eligibility, existing Event bytes and CareerPolicyID remain
unchanged. There is no new award authority, database schema, automatic mining,
ranking/title policy, permanent genesis or production wallet activation.
The thorough joint design review with Mike remains a separate release gate.

Runtime implementation, original-session/lifecycle tests, actual awarded
history and normal-package acceptance, source review and live deployment
verification remain pending at this reference-only checkpoint.
