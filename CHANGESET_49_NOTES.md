# Change set 49 — finality-timed career awards

In progress on `slug-career-awards`, stacked from completed Set48 topic
`dd74828f3957d252f16f0e4925e2bcbb518ed540`. No Set49 deployment or completed
runtime acceptance is claimed.

## Decision

D78 applies permanent records, XP and all result-derived points/title changes
exactly once at the result's own signed checkpoint plus sixty descendants.
Provisional outcomes remain labeled and grant no permanent effects. Later
reported chain conflicts retain finalized awards. Noon snapshots consume only
awards already effective at lock. See `RESULT_AWARD_POLICY.md`.

## Implementation and verification

The integration contract and independent reference are being prepared before
runtime changes. Existing result, stable and event authority boundaries need
explicit versioning and durable as-of accounting. C1's zero-record restriction,
D65, all existing evidence, production authority and D74 remain unchanged.
Validation results will be recorded here as they are actually obtained.
