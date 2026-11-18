# Set47 Card dependency update — engineering draft

This draft updates the dependency assumptions in the original Set47 contracts,
frozen by `set47-contract-freeze.json` at
`e023ddb4c043ae09b6edd826e0fbd1765d3e8bd02c34bcf0ce0ed10205d8c2fe`.
The original contracts, freeze and reference input copies remain preserved.
This draft is not a replacement freeze or permission to start runtime work.

Set46's actual-clock package and populated history tests exposed costs omitted
by the earlier stationary-clock and test-lease measurements. Its reviewed
correction adds a prepared source-view sequence and a bounded private Card
index of ID, cutoff and SHA256 of the exact canonical stored record bytes.
The index derives only from a complete opening audit or a validated successful
synced insert. Selected current database bytes must match that private index
before owned decoding. No record payload, caller validation verdict or import
authority is cached. Current concrete seed evidence still receives its full
semantic validation outside the final account lease.

Accordingly, Set47's new source seam should reuse that existing corrected Card
read machinery. Statements in the original core sections2/5 and helper
sections4/5 that assumed a repeated full Card record audit, or forbade adding a
new Card audit cache in Set47, mean: do not add a second cache or bypass the
existing audited-byte check; retain the full current seed-source audit and
original/final session guards. Extract and hash compact bout projections from
the already validated owned Card result without repeating semantic validation
per bout. The strict prohibitions on engine execution in retained transcript
reads, replay, cached polls and private transport parsing remain intact.

Set46 separately revised only its Card metadata context budget from250ms to
500ms after the full account-and-history path still exceeded250ms following
both redundancy reductions. This does **not** revise Set47's separately frozen
250ms Bout metadata gate. That remains an unproven implementation acceptance
gate requiring actual full-path measurement and a separate explicit engineering
review if it fails. No other timeout, global lane, cache or frame cap changes
through this dependency update.

All Set47 DTOs, field order, digest recipes, candidate scope, original cutoff,
births, booked seed, unchanged engine, zero bonuses, immutable result policy,
new-run conflict refusal and historical replay meanings remain exactly as
originally frozen. Reference arithmetic must continue to come from the frozen
independent Python engine/source inputs. This dependency update supplies no
new expected fight output or authority assertion.

Before adoption, bind the final reviewed Set46 source and correction evidence,
obtain independent review of this limited addendum, and publish an additive
Set47 freeze preserving the original freeze and draft input inventory. Runtime
implementation still waits for Set46's verified release.
